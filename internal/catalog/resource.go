package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
)

func scanResource(row interface{ Scan(...any) error }) (*Resource, error) {
	v := &Resource{}
	var raw string
	var c, u int64
	e := row.Scan(&v.ID, &v.InstanceID, &v.TypeID, &v.IdentityKey, &v.IdentityVersion, &v.ScopeID, &v.Locator.Version, &raw, &v.Name, &v.Origin, &v.Protected, &c, &u)
	v.Locator.Data = JSON(raw)
	v.CreatedAt = timestamp(c)
	v.UpdatedAt = timestamp(u)
	return v, dbError(e)
}
func (s *Service) validateResource(v *Resource) error {
	if !json.Valid(v.Locator.Data) || v.IdentityVersion != 1 {
		return ErrInvalid
	}
	if s.Registry.types[v.TypeID] != nil {
		return s.Registry.Validate(v.TypeID, "locator", v.Locator)
	}
	return nil
}
func (s *Service) Resource(ctx context.Context, id string) (*Resource, error) {
	v, e := scanResource(s.DB.QueryRowContext(ctx, `SELECT * FROM resource_instance WHERE id=?`, id))
	if e != nil {
		return nil, e
	}
	return v, s.validateResource(v)
}
func (s *Service) Resources(ctx context.Context) ([]Resource, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM resource_instance ORDER BY display_name,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		v, e := scanResource(rows)
		if e != nil {
			return nil, e
		}
		if e = s.validateResource(v); e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func scanAccess(row interface{ Scan(...any) error }) (*Access, error) {
	v := &Access{}
	var raw, caps string
	var attempt int64
	var success sql.NullInt64
	e := row.Scan(&v.ID, &v.ResourceID, &v.BindingID, &v.Revision, &v.Observation.Version, &raw, &caps, &attempt, &success, &v.ErrorCode, &v.ConnectionID, &v.ConnectionLabel, &v.ScopeLabel, &v.ValidationState, &v.CurrentCredentialRevision)
	if e != nil {
		return nil, dbError(e)
	}
	v.Observation.Data = JSON(raw)
	if e = decode(caps, &v.Capabilities); e != nil {
		return nil, e
	}
	if e = validateCapabilities(JSON(caps)); e != nil {
		return nil, e
	}
	v.LastAttempt = timestamp(attempt)
	if success.Valid {
		t := timestamp(success.Int64)
		v.LastSuccess = &t
	}
	return v, nil
}
func (s *Service) Accesses(ctx context.Context, id string) ([]Access, error) {
	r, e := s.Resource(ctx, id)
	if e != nil {
		return nil, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT a.*, c.connection_id,p.label,sc.label,c.validation_state,p.credential_revision FROM resource_access a JOIN connection_scope c ON c.id=a.connection_scope_id JOIN provider_connection p ON p.id=c.connection_id JOIN provider_scope sc ON sc.id=c.scope_id WHERE a.resource_id=? ORDER BY a.id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Access{}
	for rows.Next() {
		a, e := scanAccess(rows)
		if e != nil {
			return nil, e
		}
		if s.Registry.types[r.TypeID] != nil {
			if e = s.Registry.Validate(r.TypeID, "observation", a.Observation); e != nil {
				return nil, e
			}
		} else if !json.Valid(a.Observation.Data) {
			return nil, ErrInvalid
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
func (s *Service) SelectAccess(ctx context.Context, r *Resource, id string) (*Access, *AccessContext, error) {
	return s.selectAccess(ctx, r, id, false)
}
func (s *Service) selectAccess(ctx context.Context, r *Resource, id string, refresh bool) (*Access, *AccessContext, error) {
	if s.Registry.types[r.TypeID] == nil {
		return nil, nil, ErrForbidden
	}
	all, e := s.Accesses(ctx, r.ID)
	if e != nil {
		return nil, nil, e
	}
	valid := []struct {
		a Access
		c *AccessContext
	}{}
	for _, a := range all {
		if id != "" && a.ID != id {
			continue
		}
		c, e := s.Context(ctx, a.BindingID)
		if e != nil {
			continue
		}
		if !refresh && (a.Revision != c.Connection.Revision || a.ErrorCode != "") {
			continue
		}
		valid = append(valid, struct {
			a Access
			c *AccessContext
		}{a, c})
	}
	if len(valid) == 0 {
		return nil, nil, ErrForbidden
	}
	if len(valid) > 1 {
		return nil, nil, fmt.Errorf("%w: access_id is required for multiple access paths", ErrConflict)
	}
	return &valid[0].a, valid[0].c, nil
}
func (s *Service) Discover(ctx context.Context, bindingID, typeID string, input Document) ([]Observation, error) {
	c, e := s.Context(ctx, bindingID)
	if e != nil {
		return nil, e
	}
	t := s.Registry.types[typeID]
	if t == nil || t.Handler == nil || t.Descriptor.ProviderID != c.Instance.ProviderID || !member(t.Descriptor.ScopeTypes, c.Scope.Type) {
		return nil, ErrInvalid
	}
	if input.Version != 1 || !json.Valid(input.Data) {
		return nil, ErrInvalid
	}
	values, e := t.Handler.Discover(ctx, *c, input.Data)
	if e != nil {
		return nil, safeError(e)
	}
	for _, v := range values {
		if e = s.validateObservation(t, c, v); e != nil {
			return nil, e
		}
	}
	return values, nil
}
func (s *Service) validateObservation(t *registeredType, c *AccessContext, v Observation) error {
	if _, e := IdentityKey(v.IdentityParts); e != nil {
		return e
	}
	if t.Descriptor.Identity.Scope == "scope" {
		var scopeParts []string
		if e := decode(c.Scope.Key, &scopeParts); e != nil {
			return e
		}
		if len(v.IdentityParts) <= len(scopeParts) {
			return ErrInvalid
		}
		for i, p := range scopeParts {
			if v.IdentityParts[i] != p {
				return fmt.Errorf("%w: identity must include scope namespace", ErrInvalid)
			}
		}
	}
	if t.Descriptor.Identity.Scope == "parent" {
		found := false
		for _, rel := range v.Relations {
			if rel.Type == "parent" && rel.Reference.TypeID == t.Descriptor.Identity.ParentType && len(rel.Reference.IdentityParts) > 0 {
				if rel.Reference.InstanceID == nil || *rel.Reference.InstanceID != c.Instance.ID || rel.Reference.ProviderID != c.Instance.ProviderID {
					return ErrInvalid
				}
				found = true
				if len(v.IdentityParts) <= len(rel.Reference.IdentityParts) {
					return ErrInvalid
				}
				for i, p := range rel.Reference.IdentityParts {
					if v.IdentityParts[i] != p {
						return ErrInvalid
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("%w: verified parent reference required", ErrInvalid)
		}
	}
	for _, kv := range []struct {
		k       string
		raw     JSON
		version int
	}{{"locator", v.Locator, t.Descriptor.Locator.Version}, {"observation", v.Public, t.Descriptor.Observation.Version}} {
		if e := s.Registry.Validate(t.Descriptor.ID, kv.k, Document{kv.version, kv.raw}); e != nil {
			return e
		}
	}
	if e := validateCapabilities(jsonBytes(v.Capabilities)); e != nil {
		return e
	}
	return nil
}
func (s *Service) Import(ctx context.Context, bindingID, typeID string, locator Document) (*Resource, error) {
	return s.ImportWithConfirmation(ctx, bindingID, typeID, locator, "")
}
func (s *Service) ImportWithConfirmation(ctx context.Context, bindingID, typeID string, locator Document, confirmedID string) (*Resource, error) {
	c, e := s.Context(ctx, bindingID)
	if e != nil {
		return nil, e
	}
	t := s.Registry.types[typeID]
	if t == nil || t.Handler == nil || t.Descriptor.ProviderID != c.Instance.ProviderID || !member(t.Descriptor.ScopeTypes, c.Scope.Type) {
		return nil, ErrInvalid
	}
	if e = s.Registry.Validate(typeID, "locator", locator); e != nil {
		return nil, e
	}
	v, e := t.Handler.Inspect(ctx, *c, locator.Data)
	if e != nil {
		return nil, safeError(e)
	}
	if t.Descriptor.Identity.Natural {
		key, e := IdentityKey(v.IdentityParts)
		if e != nil {
			return nil, e
		}
		var existingID string
		e = s.DB.QueryRowContext(ctx, `SELECT id FROM resource_instance WHERE provider_instance_id=? AND resource_type_id=? AND identity_key=?`, c.Instance.ID, typeID, key).Scan(&existingID)
		if e == nil && confirmedID != existingID {
			return nil, fmt.Errorf("%w: natural identity requires explicit confirm_resource_id before merging", ErrConflict)
		}
		if e != nil && e != sql.ErrNoRows {
			return nil, e
		}
		if e == sql.ErrNoRows && confirmedID != "" {
			return nil, ErrInvalid
		}
	}
	return s.persistWithPolicy(ctx, c, t, v, "external", confirmedID, t.Descriptor.Identity.Natural, nil)
}
func (s *Service) persist(ctx context.Context, c *AccessContext, t *registeredType, v Observation, origin string) (*Resource, error) {
	return s.persistWithPolicy(ctx, c, t, v, origin, "", origin == "mevius" && t.Descriptor.Identity.Natural, nil)
}
func (s *Service) persistWithPolicy(ctx context.Context, c *AccessContext, t *registeredType, v Observation, origin, confirmedID string, preventMerge bool, resolution *Reference) (*Resource, error) {
	if e := s.validateObservation(t, c, v); e != nil {
		return nil, e
	}
	key, e := IdentityKey(v.IdentityParts)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var revision int
	var state string
	e = tx.QueryRowContext(ctx, `SELECT p.credential_revision,c.validation_state FROM connection_scope c JOIN provider_connection p ON p.id=c.connection_id WHERE c.id=?`, c.Binding.ID).Scan(&revision, &state)
	if e != nil {
		return nil, dbError(e)
	}
	if revision != c.Connection.Revision || state != "valid" {
		return nil, ErrConflict
	}
	if preventMerge {
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM resource_instance WHERE provider_instance_id=? AND resource_type_id=? AND identity_key=?`, c.Instance.ID, t.Descriptor.ID, key).Scan(&existing)
		if err == nil && confirmedID != existing {
			return nil, ErrConflict
		}
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == sql.ErrNoRows && confirmedID != "" {
			return nil, ErrInvalid
		}
	}
	id := uuid.NewString()
	_, e = tx.ExecContext(ctx, `INSERT INTO resource_instance VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider_instance_id,resource_type_id,identity_key) DO UPDATE SET locator_version=excluded.locator_version,locator_json=excluded.locator_json,display_name=excluded.display_name,updated_at=excluded.updated_at`, id, c.Instance.ID, t.Descriptor.ID, key, 1, c.Scope.ID, t.Descriptor.Locator.Version, string(v.Locator), v.Name, origin, false, millis(now), millis(now))
	if e != nil {
		return nil, dbError(e)
	}
	// Explicit columns avoid accidental schema drift. The origin of an existing row is never upgraded by adoption.
	r, e := scanResource(tx.QueryRowContext(ctx, `SELECT * FROM resource_instance WHERE provider_instance_id=? AND resource_type_id=? AND identity_key=?`, c.Instance.ID, t.Descriptor.ID, key))
	if e != nil {
		return nil, e
	}
	aid := uuid.NewString()
	_, e = tx.ExecContext(ctx, `INSERT INTO resource_access VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(resource_id,connection_scope_id) DO UPDATE SET credential_revision=excluded.credential_revision,observation_version=excluded.observation_version,observation_json=excluded.observation_json,capabilities_json=excluded.capabilities_json,last_attempt_at=excluded.last_attempt_at,last_success_at=excluded.last_success_at,error_code=''`, aid, r.ID, c.Binding.ID, c.Connection.Revision, t.Descriptor.Observation.Version, string(v.Public), string(jsonBytes(v.Capabilities)), millis(now), millis(now), "")
	if e != nil {
		return nil, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT id FROM resource_access WHERE resource_id=? AND connection_scope_id=?`, r.ID, c.Binding.ID).Scan(&aid); e != nil {
		return nil, e
	}
	if e = s.updateRelations(ctx, tx, r, aid, v); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM resource_reference WHERE NOT EXISTS(SELECT 1 FROM resource_relation WHERE reference_id=resource_reference.id)`); e != nil {
		return nil, e
	}
	if resolution != nil {
		var active int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE target_id=? AND status='in_progress'`, r.ID).Scan(&active); e != nil {
			return nil, e
		}
		if active > 0 {
			return nil, ErrConflict
		}
		result, e := tx.ExecContext(ctx, `UPDATE resource_reference SET provider_instance_id=?,identity_key=?,resolved_resource_id=? WHERE id=? AND reference_version=? AND reference_json=?`, r.InstanceID, r.IdentityKey, r.ID, resolution.ID, resolution.Remote.Version, string(resolution.Remote.Data))
		if e != nil {
			return nil, e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return nil, ErrConflict
		}
	}
	// Resolve only references with a verified canonical identity in the same instance.
	_, e = tx.ExecContext(ctx, `UPDATE resource_reference SET resolved_resource_id=? WHERE provider_instance_id=? AND resource_type_id=? AND identity_key=?`, r.ID, r.InstanceID, r.TypeID, r.IdentityKey)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return r, nil
}
func (s *Service) Refresh(ctx context.Context, id, accessID string) (*Resource, error) {
	r, e := s.Resource(ctx, id)
	if e != nil {
		return nil, e
	}
	a, c, e := s.selectAccess(ctx, r, accessID, true)
	if e != nil {
		return nil, e
	}
	t := s.Registry.types[r.TypeID]
	v, e := t.Handler.Inspect(ctx, *c, r.Locator.Data)
	if e != nil {
		_, dbErr := s.DB.ExecContext(ctx, `UPDATE resource_access SET last_attempt_at=?,error_code='inspect_failed' WHERE id=?`, time.Now().UnixMilli(), a.ID)
		if dbErr != nil {
			return nil, dbErr
		}
		return nil, safeError(e)
	}
	key, e := IdentityKey(v.IdentityParts)
	if e != nil {
		return nil, e
	}
	if key != r.IdentityKey {
		return nil, fmt.Errorf("%w: remote identity changed; import as a different resource", ErrConflict)
	}
	return s.persist(ctx, c, t, v, r.Origin)
}
func (s *Service) SetProtection(ctx context.Context, id string, value bool) (*Resource, error) {
	res, e := s.DB.ExecContext(ctx, `UPDATE resource_instance SET delete_protection=?,updated_at=? WHERE id=? AND NOT EXISTS(SELECT 1 FROM operation_request WHERE target_id=? AND status='in_progress')`, value, time.Now().UnixMilli(), id, id)
	if e != nil {
		return nil, e
	}
	n, e := res.RowsAffected()
	if e != nil || n != 1 {
		return nil, ErrConflict
	}
	return s.Resource(ctx, id)
}
func deletionCheck(ctx context.Context, tx *sql.Tx, r *Resource, remote bool) error {
	if remote && (r.Origin != "mevius" || r.Protected) {
		return ErrForbidden
	}
	var count int
	e := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM project_resource WHERE resource_id=?) + (SELECT COUNT(*) FROM resource_relation rel JOIN resource_reference ref ON ref.id=rel.reference_id WHERE ref.resolved_resource_id=? AND rel.blocks_deletion=1)`, r.ID, r.ID).Scan(&count)
	if e != nil {
		return e
	}
	if count > 0 {
		return ErrConflict
	}
	return nil
}
func (s *Service) Forget(ctx context.Context, id string) error {
	r, e := s.Resource(ctx, id)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var active int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE target_id=? AND status='in_progress'`, id).Scan(&active); e != nil {
		return e
	}
	if active > 0 {
		return ErrConflict
	}
	if e = deletionCheck(ctx, tx, r, false); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM resource_instance WHERE id=?`, id); e != nil {
		return dbError(e)
	}
	return tx.Commit()
}
func (s *Service) View(ctx context.Context, id, accessID, viewID string, input Document) (Document, error) {
	r, e := s.Resource(ctx, id)
	if e != nil {
		return Document{}, e
	}
	a, c, e := s.SelectAccess(ctx, r, accessID)
	if e != nil {
		return Document{}, e
	}
	t := s.Registry.types[r.TypeID]
	for _, v := range t.Descriptor.Views {
		if v.ID == viewID {
			if a.Capabilities[v.Capability].Availability == "unavailable" {
				return Document{}, ErrForbidden
			}
			if e = s.Registry.Validate(r.TypeID, "view:"+viewID, input); e != nil {
				return Document{}, e
			}
			out, e := t.Views[viewID](ctx, *c, *r, *a, input.Data)
			if e != nil {
				return Document{}, safeError(e)
			}
			doc := Document{v.Output.Version, out}
			return doc, s.Registry.Validate(r.TypeID, "view-output:"+viewID, doc)
		}
	}
	return Document{}, ErrNotFound
}
