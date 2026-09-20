package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"mevius/internal/crypto"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type ConnectionService struct {
	q        *store.Queries
	key      [32]byte
	registry *provider.Registry
}
type CreateConnectionInput struct {
	ProviderID domain.ProviderID    `json:"provider_id"`
	Label      string               `json:"label"`
	Endpoint   string               `json:"endpoint,omitempty"`
	Scope      domain.ProviderScope `json:"scope"`
	Config     map[string]any       `json:"config,omitempty"`
	Credential string               `json:"credential"`
}

func NewConnectionService(q *store.Queries, key [32]byte, registry *provider.Registry) *ConnectionService {
	return &ConnectionService{q: q, key: key, registry: registry}
}
func (s *ConnectionService) Probe(ctx context.Context, id domain.ProviderID, endpoint string, credential []byte) (*domain.ProbeResult, error) {
	p := s.registry.Provider(id)
	if p == nil {
		return nil, fmt.Errorf("%w: unknown provider %s", ErrInvalid, id)
	}
	return p.Probe(ctx, endpoint, credential)
}
func (s *ConnectionService) Create(ctx context.Context, input CreateConnectionInput) (*domain.ProviderConnection, error) {
	if input.Credential == "" || input.Scope.ID == "" {
		return nil, fmt.Errorf("%w: credential and scope are required", ErrInvalid)
	}
	return s.createValidated(ctx, input, []byte(input.Credential), domain.AuthMethodToken, "", nil)
}

func (s *ConnectionService) createValidated(ctx context.Context, input CreateConnectionInput, credential []byte, authMethod domain.AuthMethod, encryptedCredential string, oauthMeta map[string]any) (*domain.ProviderConnection, error) {
	p := s.registry.Provider(input.ProviderID)
	if p == nil {
		return nil, fmt.Errorf("%w: unknown provider", ErrInvalid)
	}
	probe, err := p.ValidateScope(ctx, input.Endpoint, credential, input.Scope)
	if err != nil {
		return nil, err
	}
	scope := probe.Scopes[0]
	now := time.Now().UTC().Format(time.RFC3339)
	config := input.Config
	if config == nil {
		config = map[string]any{}
	}
	if oauthMeta != nil {
		config["oauth"] = oauthMeta
	}
	if encryptedCredential == "" {
		encryptedCredential = crypto.Encrypt(s.key, string(credential))
	}
	v := domain.ProviderConnection{ID: uuid.NewString(), ProviderID: input.ProviderID, Label: input.Label, Endpoint: input.Endpoint, Scope: scope, AuthMethod: authMethod, Config: config, RemoteIdentity: probe.Identity, Permissions: probe.Permissions, PermissionsCheckedAt: now, CreatedAt: now, UpdatedAt: now}
	err = s.q.InsertProviderConnection(ctx, store.InsertProviderConnectionParams{ID: v.ID, ProviderID: string(v.ProviderID), Label: v.Label, Endpoint: v.Endpoint, ScopeType: scope.Type, ScopeID: scope.ID, ScopeLabel: scope.Label, AuthMethod: string(authMethod), ConfigJson: encode(v.Config), EncryptedCredential: encryptedCredential, RemoteIdentityJson: encode(v.RemoteIdentity), PermissionsJson: encode(v.Permissions), PermissionsCheckedAt: &now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return nil, fmt.Errorf("%w: connection scope already exists: %v", ErrConflict, err)
	}
	return &v, nil
}
func (s *ConnectionService) RotateCredential(ctx context.Context, id, credential string) (*domain.ProviderConnection, error) {
	conn, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	p := s.registry.Provider(conn.ProviderID)
	probe, err := p.ValidateScope(ctx, conn.Endpoint, []byte(credential), conn.Scope)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	err = s.q.UpdateProviderConnectionCredential(ctx, store.UpdateProviderConnectionCredentialParams{EncryptedCredential: crypto.Encrypt(s.key, credential), RemoteIdentityJson: encode(probe.Identity), PermissionsJson: encode(probe.Permissions), PermissionsCheckedAt: &now, UpdatedAt: now, ID: id})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}
func (s *ConnectionService) List(ctx context.Context) ([]domain.ProviderConnection, error) {
	rows, err := s.q.ListProviderConnections(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProviderConnection, 0, len(rows))
	for _, row := range rows {
		result = append(result, connectionFromStore(row))
	}
	return result, nil
}
func (s *ConnectionService) Get(ctx context.Context, id string) (*domain.ProviderConnection, error) {
	row, err := s.q.GetProviderConnection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v := connectionFromStore(row)
	return &v, nil
}
func (s *ConnectionService) Delete(ctx context.Context, id string) error {
	count, err := s.q.CountResourceInstancesByConnection(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: connection has resource instances", ErrConflict)
	}
	affected, err := s.q.DeleteProviderConnection(ctx, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
