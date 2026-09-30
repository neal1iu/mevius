package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

func (s *Service) relationCheck(from *Resource, typ, origin string, ref Reference, attributes Document) (RelationDescriptor, error) {
	d, ok := s.Registry.relations[typ]
	if t := s.Registry.types[from.TypeID]; t != nil && origin == "adapter" {
		ok = false
		for _, local := range t.Descriptor.Relations {
			if local.ID == typ {
				d = local
				ok = true
				break
			}
		}
	}
	if !ok || d.Origin != origin || !member(d.From, from.TypeID) || !member(d.To, ref.TypeID) || attributes.Version != d.Attributes.Version {
		return d, ErrInvalid
	}
	target := s.Registry.types[ref.TypeID]
	if target == nil || target.Descriptor.ProviderID != ref.ProviderID {
		return d, ErrInvalid
	}
	if e := s.Registry.Validate(ref.TypeID, "reference", ref.Remote); e != nil {
		return d, e
	}
	var v any
	if e := json.Unmarshal(attributes.Data, &v); e != nil {
		return d, ErrInvalid
	}
	if e := s.Registry.relationSchemas[typ].Validate(v); e != nil {
		return d, ErrInvalid
	}
	return d, nil
}
func (s *Service) insertRelation(ctx context.Context, tx *sql.Tx, from *Resource, aid *string, typ, origin string, ref Reference, attrs Document) error {
	d, e := s.relationCheck(from, typ, origin, ref, attrs)
	if e != nil {
		return e
	}
	ref.ResolvedID = nil
	var key *string
	if len(ref.IdentityParts) > 0 {
		if ref.InstanceID == nil {
			return ErrInvalid
		}
		k, e := IdentityKey(ref.IdentityParts)
		if e != nil {
			return e
		}
		key = &k
		var resolved string
		e = tx.QueryRowContext(ctx, `SELECT id FROM resource_instance WHERE provider_instance_id=? AND resource_type_id=? AND identity_key=?`, *ref.InstanceID, ref.TypeID, k).Scan(&resolved)
		if e == nil {
			ref.ResolvedID = &resolved
		} else if e != sql.ErrNoRows {
			return e
		} else if origin == "user" {
			return ErrInvalid
		}
	} else {
		ref.ResolvedID = nil
	}
	if ref.ResolvedID != nil && *ref.ResolvedID == from.ID {
		return ErrInvalid
	}
	var active int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE status='in_progress' AND target_id IN (?,?)`, from.ID, ref.ResolvedID).Scan(&active); e != nil {
		return e
	}
	if active > 0 {
		return ErrConflict
	}
	if ref.InstanceID != nil {
		var p string
		if e = tx.QueryRowContext(ctx, `SELECT provider_id FROM provider_instance WHERE id=?`, *ref.InstanceID).Scan(&p); e != nil {
			return dbError(e)
		}
		if p != ref.ProviderID {
			return ErrInvalid
		}
	}
	ref.ID = uuid.NewString()
	var existing string
	e = tx.QueryRowContext(ctx, `SELECT ref.id FROM resource_reference ref JOIN resource_relation rel ON rel.reference_id=ref.id WHERE rel.from_resource_id=? AND rel.relation_type=? AND rel.origin=? AND rel.observed_access_id IS ? AND ref.provider_id=? AND ref.provider_instance_id IS ? AND ref.resource_type_id=? AND ref.reference_json=? AND ref.identity_key IS ?`, from.ID, typ, origin, aid, ref.ProviderID, ref.InstanceID, ref.TypeID, string(ref.Remote.Data), key).Scan(&existing)
	if e == nil {
		_, e = tx.ExecContext(ctx, `UPDATE resource_relation SET attributes_version=?,attributes_json=?,blocks_deletion=? WHERE from_resource_id=? AND reference_id=? AND relation_type=? AND origin=?`, attrs.Version, string(attrs.Data), d.BlocksDeletion, from.ID, existing, typ, origin)
		return e
	}
	if e != sql.ErrNoRows {
		return e
	}
	if d.Cardinality == "one" {
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM resource_relation WHERE from_resource_id=? AND relation_type=? AND origin=? AND observed_access_id IS ?`, from.ID, typ, origin, aid).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return ErrConflict
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO resource_reference VALUES(?,?,?,?,?,?,?,?,?)`, ref.ID, ref.ProviderID, ref.InstanceID, ref.TypeID, ref.Remote.Version, string(ref.Remote.Data), key, ref.ResolvedID, time.Now().UnixMilli())
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO resource_relation VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), from.ID, ref.ID, typ, origin, aid, attrs.Version, string(attrs.Data), d.BlocksDeletion, time.Now().UnixMilli())
	return e
}
func (s *Service) updateRelations(ctx context.Context, tx *sql.Tx, r *Resource, aid string, v Observation) error {
	for _, typ := range v.CompleteRelations {
		d, ok := s.Registry.relations[typ]
		if !ok || d.Origin != "adapter" || !member(d.From, r.TypeID) {
			return ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM resource_relation WHERE from_resource_id=? AND observed_access_id=? AND relation_type=? AND origin='adapter'`, r.ID, aid, typ); e != nil {
			return e
		}
	}
	for _, rel := range v.Relations {
		attrs := rel.Attributes
		if len(attrs) == 0 {
			attrs = JSON(`{}`)
		}
		d := s.Registry.relations[rel.Type]
		if e := s.insertRelation(ctx, tx, r, &aid, rel.Type, "adapter", rel.Reference, Document{d.Attributes.Version, attrs}); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) AddRelation(ctx context.Context, fromID, typ string, ref Reference, attributes Document) error {
	r, e := s.Resource(ctx, fromID)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var active int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE status='in_progress' AND target_id IN (?,?)`, fromID, ref.ResolvedID).Scan(&active); e != nil {
		return e
	}
	if active > 0 {
		return ErrConflict
	}
	if e = s.insertRelation(ctx, tx, r, nil, typ, "user", ref, attributes); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) RemoveRelation(ctx context.Context, id string) error {
	res, e := s.DB.ExecContext(ctx, `DELETE FROM resource_relation WHERE id=? AND origin='user'`, id)
	if e != nil {
		return dbError(e)
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return ErrForbidden
	}
	return nil
}
func (s *Service) Relations(ctx context.Context, id string) ([]Relation, error) {
	resource, e := s.Resource(ctx, id)
	if e != nil {
		return nil, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT rel.id,rel.from_resource_id,rel.relation_type,rel.origin,rel.observed_access_id,rel.attributes_version,rel.attributes_json,rel.blocks_deletion,ref.id,ref.provider_id,ref.provider_instance_id,ref.resource_type_id,ref.reference_version,ref.reference_json,ref.identity_key,ref.resolved_resource_id FROM resource_relation rel JOIN resource_reference ref ON ref.id=rel.reference_id WHERE rel.from_resource_id=? ORDER BY rel.id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Relation{}
	for rows.Next() {
		var v Relation
		var attrs, remote string
		var key *string
		if e = rows.Scan(&v.ID, &v.FromID, &v.Type, &v.Origin, &v.AccessID, &v.Attributes.Version, &attrs, &v.BlocksDeletion, &v.Reference.ID, &v.Reference.ProviderID, &v.Reference.InstanceID, &v.Reference.TypeID, &v.Reference.Remote.Version, &remote, &key, &v.Reference.ResolvedID); e != nil {
			return nil, e
		}
		v.Attributes.Data = JSON(attrs)
		v.Reference.Remote.Data = JSON(remote)
		if !json.Valid(v.Attributes.Data) || !json.Valid(v.Reference.Remote.Data) {
			return nil, ErrInvalid
		}
		if s.Registry.types[resource.TypeID] != nil && s.Registry.types[v.Reference.TypeID] != nil {
			if _, e = s.relationCheck(resource, v.Type, v.Origin, v.Reference, v.Attributes); e != nil {
				return nil, e
			}
		}
		if key != nil {
			if e = decode(*key, &v.Reference.IdentityParts); e != nil {
				return nil, e
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type ProjectResource struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	ResourceID  string    `json:"resource_id"`
	Alias       string    `json:"alias"`
	Role        string    `json:"role"`
	Purpose     string    `json:"purpose"`
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	v := &Project{}
	var c, u int64
	e := row.Scan(&v.ID, &v.Name, &v.Description, &c, &u)
	v.CreatedAt = timestamp(c)
	v.UpdatedAt = timestamp(u)
	return v, dbError(e)
}
func (s *Service) Projects(ctx context.Context) ([]Project, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM project ORDER BY name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		v, e := scanProject(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (s *Service) Project(ctx context.Context, id string) (*Project, error) {
	return scanProject(s.DB.QueryRowContext(ctx, `SELECT * FROM project WHERE id=?`, id))
}
func (s *Service) SaveProject(ctx context.Context, id, name, description string) (*Project, error) {
	if name == "" {
		return nil, ErrInvalid
	}
	now := time.Now().UnixMilli()
	if id == "" {
		id = uuid.NewString()
		_, e := s.DB.ExecContext(ctx, `INSERT INTO project VALUES(?,?,?,?,?)`, id, name, description, now, now)
		if e != nil {
			return nil, dbError(e)
		}
	} else {
		res, e := s.DB.ExecContext(ctx, `UPDATE project SET name=?,description=?,updated_at=? WHERE id=?`, name, description, now, id)
		if e != nil {
			return nil, dbError(e)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return nil, ErrNotFound
		}
	}
	return s.Project(ctx, id)
}
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	res, e := s.DB.ExecContext(ctx, `DELETE FROM project WHERE id=?`, id)
	if e != nil {
		return dbError(e)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Service) ProjectResources(ctx context.Context, id string) ([]ProjectResource, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM project_resource WHERE project_id=? ORDER BY alias`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ProjectResource{}
	for rows.Next() {
		v := ProjectResource{}
		var c, u int64
		if e = rows.Scan(&v.ID, &v.ProjectID, &v.ResourceID, &v.Alias, &v.Role, &v.Purpose, &v.Environment, &c, &u); e != nil {
			return nil, e
		}
		v.CreatedAt = timestamp(c)
		v.UpdatedAt = timestamp(u)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Attach(ctx context.Context, v ProjectResource) (*ProjectResource, error) {
	r, e := s.Resource(ctx, v.ResourceID)
	if e != nil {
		return nil, e
	}
	t := s.Registry.types[r.TypeID]
	if t == nil || !member(t.Descriptor.Roles, v.Role) || v.Alias == "" {
		return nil, ErrInvalid
	}
	v.ID = uuid.NewString()
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	res, e := s.DB.ExecContext(ctx, `INSERT INTO project_resource SELECT ?,?,?,?,?,?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM operation_request WHERE target_id=? AND status='in_progress')`, v.ID, v.ProjectID, v.ResourceID, v.Alias, v.Role, v.Purpose, v.Environment, millis(v.CreatedAt), millis(v.UpdatedAt), v.ResourceID)
	if e != nil {
		return nil, dbError(e)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrConflict
	}
	return &v, nil
}
func (s *Service) Detach(ctx context.Context, projectID, linkID string) error {
	res, e := s.DB.ExecContext(ctx, `DELETE FROM project_resource WHERE project_id=? AND id=?`, projectID, linkID)
	if e != nil {
		return dbError(e)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Service) UpdateAttachment(ctx context.Context, projectID, linkID string, v ProjectResource) error {
	var resourceID string
	if e := s.DB.QueryRowContext(ctx, `SELECT resource_id FROM project_resource WHERE project_id=? AND id=?`, projectID, linkID).Scan(&resourceID); e != nil {
		return dbError(e)
	}
	r, e := s.Resource(ctx, resourceID)
	if e != nil {
		return e
	}
	t := s.Registry.types[r.TypeID]
	if t == nil || !member(t.Descriptor.Roles, v.Role) || v.Alias == "" {
		return ErrInvalid
	}
	_, e = s.DB.ExecContext(ctx, `UPDATE project_resource SET alias=?,role=?,purpose=?,environment=?,updated_at=? WHERE project_id=? AND id=?`, v.Alias, v.Role, v.Purpose, v.Environment, time.Now().UnixMilli(), projectID, linkID)
	return dbError(e)
}

// ResolveReference requires provider proof. Locator equality never resolves a reference.
func (s *Service) ResolveReference(ctx context.Context, id, bindingID string) (*Resource, error) {
	var ref Reference
	var raw string
	e := s.DB.QueryRowContext(ctx, `SELECT provider_id,provider_instance_id,resource_type_id,reference_version,reference_json FROM resource_reference WHERE id=?`, id).Scan(&ref.ProviderID, &ref.InstanceID, &ref.TypeID, &ref.Remote.Version, &raw)
	if e != nil {
		return nil, dbError(e)
	}
	ref.Remote.Data = JSON(raw)
	c, e := s.Context(ctx, bindingID)
	if e != nil {
		return nil, e
	}
	if c.Instance.ProviderID != ref.ProviderID || (ref.InstanceID != nil && *ref.InstanceID != c.Instance.ID) {
		return nil, ErrInvalid
	}
	t := s.Registry.types[ref.TypeID]
	if t == nil {
		return nil, ErrForbidden
	}
	if e = s.Registry.Validate(ref.TypeID, "reference", ref.Remote); e != nil {
		return nil, e
	}
	resolver, ok := t.Handler.(ReferenceResolver)
	if !ok {
		return nil, ErrForbidden
	}
	observation, e := resolver.ResolveReference(ctx, *c, ref.Remote.Data)
	if e != nil {
		return nil, safeError(e)
	}
	ref.ID = id
	return s.persistWithPolicy(ctx, c, t, observation, "external", "", t.Descriptor.Identity.Natural, &ref)
}
