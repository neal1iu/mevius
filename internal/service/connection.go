package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mevius/internal/crypto"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"

	"github.com/google/uuid"
)

type ConnectionService struct {
	store    store.Querier
	key      [32]byte
	registry *provider.Registry
}

func NewConnectionService(q store.Querier, key [32]byte, reg *provider.Registry) *ConnectionService {
	return &ConnectionService{store: q, key: key, registry: reg}
}

func (s *ConnectionService) AddConnection(ctx context.Context, providerType, label, endpoint string, rawToken []byte, config map[string]any) (*domain.ProviderConnection, error) {
	if len(rawToken) == 0 {
		return nil, fmt.Errorf("token is required")
	}

	p := s.registry.Get(providerType)
	if p == nil {
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()

	conn := &domain.ProviderConnection{
		ID:        id,
		Provider:  domain.ProviderType(providerType),
		Label:     label,
		Endpoint:  endpoint,
		Config:    config,
		CreatedAt: now,
	}

	meta, err := p.ValidateCredentials(ctx, conn, rawToken)
	if err != nil {
		return nil, err
	}

	var remoteIdentity domain.AccountMeta
	if err := json.Unmarshal(meta, &remoteIdentity); err != nil {
		return nil, fmt.Errorf("parse remote identity: %w", err)
	}
	conn.RemoteIdentity = remoteIdentity

	encrypted := crypto.Encrypt(s.key, string(rawToken))

	configJSON, _ := json.Marshal(config)
	metaJSON, _ := json.Marshal(remoteIdentity)

	if err := s.store.InsertProviderConnection(ctx, store.InsertProviderConnectionParams{
		ID:                  id,
		Provider:            providerType,
		Label:               label,
		Endpoint:            endpoint,
		ConfigJson:          string(configJSON),
		EncryptedCredential: encrypted,
		RemoteIdentityJson:  string(metaJSON),
		CreatedAt:           now,
	}); err != nil {
		return nil, fmt.Errorf("insert connection: %w", err)
	}

	return conn, nil
}

func (s *ConnectionService) ListConnections(ctx context.Context) ([]domain.ProviderConnection, error) {
	rows, err := s.store.ListProviderConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	connections := make([]domain.ProviderConnection, 0, len(rows))
	for _, r := range rows {
		connections = append(connections, storeConnectionToDomain(r))
	}
	return connections, nil
}

func (s *ConnectionService) GetConnection(ctx context.Context, id string) (*domain.ProviderConnection, error) {
	row, err := s.store.GetProviderConnection(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	conn := storeConnectionToDomain(row)
	return &conn, nil
}

func (s *ConnectionService) DeleteConnection(ctx context.Context, id string) error {
	if err := s.store.DeleteProviderConnection(ctx, id); err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	return nil
}

func storeConnectionToDomain(r store.ProviderConnection) domain.ProviderConnection {
	var remoteIdentity domain.AccountMeta
	json.Unmarshal([]byte(r.RemoteIdentityJson), &remoteIdentity)

	return domain.ProviderConnection{
		ID:              r.ID,
		Provider:        domain.ProviderType(r.Provider),
		Label:           r.Label,
		Endpoint:        r.Endpoint,
		RemoteIdentity:  remoteIdentity,
		CreatedAt:       r.CreatedAt,
	}
}