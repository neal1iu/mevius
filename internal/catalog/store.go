package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"mevius/internal/crypto"
	"strings"
	"time"
)

type Service struct {
	DB       *sql.DB
	Registry *Registry
	key      [32]byte
}

func NewService(db *sql.DB, r *Registry, key [32]byte) *Service {
	return &Service{DB: db, Registry: r, key: key}
}
func safeError(e error) error {
	if e == nil {
		return nil
	}
	for _, v := range []error{ErrInvalid, ErrConflict, ErrNotFound, ErrForbidden, ErrUnknown} {
		if errors.Is(e, v) {
			return e
		}
	}
	return errors.New("catalog operation failed")
}
func dbError(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil && strings.Contains(e.Error(), "constraint") {
		return ErrConflict
	}
	return e
}
func decode(raw string, v any) error {
	if e := json.Unmarshal([]byte(raw), v); e != nil {
		return fmt.Errorf("catalog storage JSON is corrupt: %w", e)
	}
	return nil
}
func (s *Service) CreateInstance(ctx context.Context, providerID, key string, endpoint Document) (*Instance, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrInvalid
	}
	if e := s.Registry.ValidateInstance(providerID, endpoint); e != nil {
		return nil, e
	}
	v := &Instance{ID: uuid.NewString(), ProviderID: providerID, Key: key, Endpoint: endpoint, CreatedAt: time.Now().UTC()}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO provider_instance VALUES(?,?,?,?,?,?)`, v.ID, v.ProviderID, v.Key, endpoint.Version, string(endpoint.Data), millis(v.CreatedAt))
	return v, dbError(e)
}
func scanInstance(row interface{ Scan(...any) error }) (*Instance, error) {
	v := &Instance{}
	var raw string
	var ms int64
	e := row.Scan(&v.ID, &v.ProviderID, &v.Key, &v.Endpoint.Version, &raw, &ms)
	v.Endpoint.Data = JSON(raw)
	v.CreatedAt = timestamp(ms)
	if e == nil && (!json.Valid(v.Endpoint.Data) || v.Endpoint.Version < 1) {
		return nil, ErrInvalid
	}
	return v, dbError(e)
}
func (s *Service) Instance(ctx context.Context, id string) (*Instance, error) {
	v, e := scanInstance(s.DB.QueryRowContext(ctx, `SELECT * FROM provider_instance WHERE id=?`, id))
	if e == nil && s.Registry.providers[v.ProviderID] != nil {
		e = s.Registry.ValidateInstance(v.ProviderID, v.Endpoint)
	}
	return v, e
}
func (s *Service) Instances(ctx context.Context) ([]Instance, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM provider_instance ORDER BY provider_id,instance_key`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Instance{}
	for rows.Next() {
		v, e := scanInstance(rows)
		if e != nil {
			return nil, e
		}
		if s.Registry.providers[v.ProviderID] != nil {
			if e = s.Registry.ValidateInstance(v.ProviderID, v.Endpoint); e != nil {
				return nil, e
			}
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (s *Service) CreateConnection(ctx context.Context, instanceID, label, scheme string, credential []byte) (*Connection, error) {
	i, e := s.Instance(ctx, instanceID)
	if e != nil {
		return nil, e
	}
	if len(credential) == 0 || !member([]string{"token", "oauth"}, scheme) {
		return nil, ErrInvalid
	}
	auth, e := s.Registry.providers[i.ProviderID].Validate(ctx, *i, credential)
	if e != nil {
		return nil, safeError(e)
	}
	if !json.Valid(auth.Principal) {
		return nil, ErrInvalid
	}
	// Only a public identity allowlist crosses the authorization boundary.
	var principal map[string]any
	if e = decode(string(auth.Principal), &principal); e != nil {
		return nil, e
	}
	public := map[string]any{}
	for _, k := range []string{"id", "login", "name", "email"} {
		if v, ok := principal[k]; ok {
			switch v.(type) {
			case string, float64:
				public[k] = v
			}
		}
	}
	now := time.Now().UTC()
	v := &Connection{ID: uuid.NewString(), InstanceID: i.ID, Label: label, AuthScheme: scheme, Revision: 1, Principal: Document{1, jsonBytes(public)}, State: "valid", CreatedAt: now, UpdatedAt: now}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO provider_connection VALUES(?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.InstanceID, v.Label, v.AuthScheme, crypto.Encrypt(s.key, string(credential)), v.Revision, 1, string(v.Principal.Data), v.State, millis(now), millis(now))
	return v, dbError(e)
}
func scanConnection(row interface{ Scan(...any) error }) (*Connection, error) {
	v := &Connection{}
	var enc, raw string
	var c, u int64
	e := row.Scan(&v.ID, &v.InstanceID, &v.Label, &v.AuthScheme, &enc, &v.Revision, &v.Principal.Version, &raw, &v.State, &c, &u)
	v.Principal.Data = JSON(raw)
	v.CreatedAt = timestamp(c)
	v.UpdatedAt = timestamp(u)
	if e == nil && (!json.Valid(v.Principal.Data) || v.Principal.Version != 1) {
		return nil, ErrInvalid
	}
	return v, dbError(e)
}
func (s *Service) Connection(ctx context.Context, id string) (*Connection, error) {
	return scanConnection(s.DB.QueryRowContext(ctx, `SELECT * FROM provider_connection WHERE id=?`, id))
}
func (s *Service) Connections(ctx context.Context) ([]Connection, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM provider_connection ORDER BY created_at,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		v, e := scanConnection(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (s *Service) credential(ctx context.Context, id string) ([]byte, error) {
	var enc string
	if e := s.DB.QueryRowContext(ctx, `SELECT encrypted_credential FROM provider_connection WHERE id=?`, id).Scan(&enc); e != nil {
		return nil, dbError(e)
	}
	value, e := crypto.Decrypt(s.key, enc)
	if e != nil {
		return nil, e
	}
	return []byte(value), nil
}
func (s *Service) DiscoverScopes(ctx context.Context, id string) ([]ScopeCandidate, error) {
	c, e := s.Connection(ctx, id)
	if e != nil {
		return nil, e
	}
	i, e := s.Instance(ctx, c.InstanceID)
	if e != nil {
		return nil, e
	}
	cred, e := s.credential(ctx, c.ID)
	if e != nil {
		return nil, e
	}
	a, e := s.Registry.providers[i.ProviderID].Validate(ctx, *i, cred)
	return a.Scopes, safeError(e)
}
func (s *Service) BindScope(ctx context.Context, id string, candidate ScopeCandidate) (*Binding, error) {
	c, e := s.Connection(ctx, id)
	if e != nil {
		return nil, e
	}
	i, e := s.Instance(ctx, c.InstanceID)
	if e != nil {
		return nil, e
	}
	key, e := IdentityKey(candidate.IdentityParts)
	if e != nil {
		return nil, e
	}
	if candidate.Type == "" || !json.Valid(candidate.Locator) {
		return nil, ErrInvalid
	}
	sc := Scope{ID: uuid.NewString(), InstanceID: i.ID, Type: candidate.Type, Key: key, Locator: Document{1, candidate.Locator}, Label: candidate.Label}
	cred, e := s.credential(ctx, id)
	if e != nil {
		return nil, e
	}
	verified, e := s.Registry.providers[i.ProviderID].ValidateScope(ctx, AccessContext{Instance: *i, Connection: *c, Scope: sc, Credential: cred})
	if e != nil {
		return nil, safeError(e)
	}
	key, e = IdentityKey(verified.IdentityParts)
	if e != nil {
		return nil, e
	}
	if verified.Type == "" || !json.Valid(verified.Locator) {
		return nil, ErrInvalid
	}
	sc.Type = verified.Type
	sc.Key = key
	sc.Locator = Document{1, verified.Locator}
	sc.Label = verified.Label
	public := verified.Context
	if e = validateCapabilities(public); e != nil {
		return nil, e
	}
	var projected Capabilities
	if e = decode(string(public), &projected); e != nil {
		return nil, e
	}
	public = jsonBytes(projected)
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	// Rotation may occur while validation is in flight. Never bind stale permissions.
	var revision int
	if e = tx.QueryRowContext(ctx, `SELECT credential_revision FROM provider_connection WHERE id=?`, id).Scan(&revision); e != nil {
		return nil, dbError(e)
	}
	if revision != c.Revision {
		return nil, ErrConflict
	}
	sc.ID, e = s.upsertScope(ctx, tx, i.ID, verified, 0)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	b := &Binding{ID: uuid.NewString(), ConnectionID: id, ScopeID: sc.ID, Context: Document{1, public}, State: "valid", ValidatedAt: &now}
	_, e = tx.ExecContext(ctx, `INSERT INTO connection_scope VALUES(?,?,?,?,?,?,?) ON CONFLICT(connection_id,scope_id) DO UPDATE SET context_json=excluded.context_json,validation_state='valid',validated_at=excluded.validated_at`, b.ID, id, sc.ID, 1, string(public), b.State, millis(now))
	if e != nil {
		return nil, dbError(e)
	}
	if e = tx.QueryRowContext(ctx, `SELECT id FROM connection_scope WHERE connection_id=? AND scope_id=?`, id, sc.ID).Scan(&b.ID); e != nil {
		return nil, e
	}
	return b, tx.Commit()
}
func validateCapabilities(raw JSON) error {
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return ErrInvalid
	}
	var caps Capabilities
	if e := json.Unmarshal(raw, &caps); e != nil {
		return ErrInvalid
	}
	for _, v := range caps {
		if !member([]string{"available", "unavailable", "unknown"}, v.Availability) {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Service) Bindings(ctx context.Context, connectionID string) ([]Binding, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM connection_scope WHERE connection_id=? ORDER BY id`, connectionID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		v, e := scanBinding(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func scanBinding(row interface{ Scan(...any) error }) (*Binding, error) {
	v := &Binding{}
	var raw string
	var ms sql.NullInt64
	e := row.Scan(&v.ID, &v.ConnectionID, &v.ScopeID, &v.Context.Version, &raw, &v.State, &ms)
	v.Context.Data = JSON(raw)
	if ms.Valid {
		t := timestamp(ms.Int64)
		v.ValidatedAt = &t
	}
	if e == nil && (v.Context.Version != 1 || validateCapabilities(v.Context.Data) != nil) {
		return nil, ErrInvalid
	}
	return v, dbError(e)
}
func (s *Service) Context(ctx context.Context, bindingID string) (*AccessContext, error) {
	b, e := scanBinding(s.DB.QueryRowContext(ctx, `SELECT * FROM connection_scope WHERE id=?`, bindingID))
	if e != nil {
		return nil, e
	}
	if b.State != "valid" {
		return nil, ErrForbidden
	}
	c, e := s.Connection(ctx, b.ConnectionID)
	if e != nil {
		return nil, e
	}
	if c.State != "valid" {
		return nil, ErrForbidden
	}
	i, e := s.Instance(ctx, c.InstanceID)
	if e != nil {
		return nil, e
	}
	sc := Scope{}
	var raw string
	e = s.DB.QueryRowContext(ctx, `SELECT id,provider_instance_id,scope_type,scope_key,locator_version,locator_json,label,parent_scope_id FROM provider_scope WHERE id=?`, b.ScopeID).Scan(&sc.ID, &sc.InstanceID, &sc.Type, &sc.Key, &sc.Locator.Version, &raw, &sc.Label, &sc.ParentID)
	if e != nil {
		return nil, dbError(e)
	}
	sc.Locator.Data = JSON(raw)
	if sc.InstanceID != i.ID || s.Registry.ValidateScopeLocator(i.ProviderID, sc.Type, sc.Locator) != nil {
		return nil, ErrInvalid
	}
	cred, e := s.credential(ctx, c.ID)
	if e != nil {
		return nil, e
	}
	return &AccessContext{Instance: *i, Connection: *c, Scope: sc, Binding: *b, Credential: cred}, nil
}
func (s *Service) RotateCredential(ctx context.Context, id string, credential []byte) (*Connection, error) {
	c, e := s.Connection(ctx, id)
	if e != nil {
		return nil, e
	}
	i, e := s.Instance(ctx, c.InstanceID)
	if e != nil {
		return nil, e
	}
	if len(credential) == 0 {
		return nil, ErrInvalid
	}
	a, e := s.Registry.providers[i.ProviderID].Validate(ctx, *i, credential)
	if e != nil {
		return nil, safeError(e)
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var active int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE connection_id=? AND status='in_progress'`, id).Scan(&active); e != nil {
		return nil, e
	}
	if active > 0 {
		return nil, ErrConflict
	}
	var principal map[string]any
	if e = decode(string(a.Principal), &principal); e != nil {
		return nil, e
	}
	public := map[string]any{}
	for _, k := range []string{"id", "login", "name", "email"} {
		if v, ok := principal[k]; ok {
			switch v.(type) {
			case string, float64:
				public[k] = v
			}
		}
	}
	result, e := tx.ExecContext(ctx, `UPDATE provider_connection SET encrypted_credential=?,principal_json=?,credential_revision=credential_revision+1,auth_scheme='token',authorization_state='valid',updated_at=? WHERE id=? AND credential_revision=?`, crypto.Encrypt(s.key, string(credential)), string(jsonBytes(public)), time.Now().UnixMilli(), id, c.Revision)
	if e != nil {
		return nil, e
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return nil, ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE connection_scope SET validation_state='unknown',context_json='{}',validated_at=NULL WHERE connection_id=?`, id); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE resource_access SET capabilities_json='{}',error_code='credential_rotated' WHERE connection_scope_id IN (SELECT id FROM connection_scope WHERE connection_id=?)`, id); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return s.Connection(ctx, id)
}
func (s *Service) DeleteConnection(ctx context.Context, id string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var active int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE connection_id=? AND status='in_progress'`, id).Scan(&active); e != nil {
		return e
	}
	if active > 0 {
		return ErrConflict
	}
	res, e := tx.ExecContext(ctx, `DELETE FROM provider_connection WHERE id=?`, id)
	if e != nil {
		return dbError(e)
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}
func (s *Service) Scope(ctx context.Context, id string) (*Scope, error) {
	v := &Scope{}
	var raw string
	e := s.DB.QueryRowContext(ctx, `SELECT * FROM provider_scope WHERE id=?`, id).Scan(&v.ID, &v.InstanceID, &v.Type, &v.Key, &v.Locator.Version, &raw, &v.Label, &v.ParentID)
	v.Locator.Data = JSON(raw)
	if e == nil && (!json.Valid(v.Locator.Data) || v.Locator.Version < 1) {
		return nil, ErrInvalid
	}
	return v, dbError(e)
}

func (s *Service) upsertScope(ctx context.Context, tx *sql.Tx, instanceID string, candidate ScopeCandidate, depth int) (string, error) {
	if depth > 8 || candidate.Type == "" || !json.Valid(candidate.Locator) {
		return "", ErrInvalid
	}
	key, e := IdentityKey(candidate.IdentityParts)
	if e != nil {
		return "", e
	}
	var parent *string
	if candidate.Parent != nil {
		id, e := s.upsertScope(ctx, tx, instanceID, *candidate.Parent, depth+1)
		if e != nil {
			return "", e
		}
		parent = &id
	}
	var providerID string
	if e = tx.QueryRowContext(ctx, `SELECT provider_id FROM provider_instance WHERE id=?`, instanceID).Scan(&providerID); e != nil {
		return "", e
	}
	schema := s.Registry.providers[providerID].ScopeSchemas()[candidate.Type]
	if e = s.Registry.ValidateScopeLocator(providerID, candidate.Type, Document{schema.Version, candidate.Locator}); e != nil {
		return "", e
	}
	id := uuid.NewString()
	_, e = tx.ExecContext(ctx, `INSERT INTO provider_scope VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(provider_instance_id,scope_type,scope_key) DO UPDATE SET locator_json=excluded.locator_json,label=excluded.label,parent_scope_id=excluded.parent_scope_id`, id, instanceID, candidate.Type, key, schema.Version, string(candidate.Locator), candidate.Label, parent)
	if e != nil {
		return "", dbError(e)
	}
	e = tx.QueryRowContext(ctx, `SELECT id FROM provider_scope WHERE provider_instance_id=? AND scope_type=? AND scope_key=?`, instanceID, candidate.Type, key).Scan(&id)
	return id, e
}
