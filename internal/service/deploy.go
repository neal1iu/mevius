package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"

	"github.com/google/uuid"
)

const deployListTTL = 30 * time.Second

type DeployService struct {
	store     store.Querier
	registry  *provider.Registry
	credStore domain.CredentialStore
	engine    *RefreshEngine

	mu      sync.RWMutex
	deploys map[string][]domain.DeployEvent
	expires map[string]time.Time
}

func NewDeployService(q store.Querier, reg *provider.Registry, credStore domain.CredentialStore, eng *RefreshEngine) *DeployService {
	return &DeployService{
		store:     q,
		registry:  reg,
		credStore: credStore,
		engine:    eng,
		deploys:   make(map[string][]domain.DeployEvent),
		expires:   make(map[string]time.Time),
	}
}

func (s *DeployService) TriggerDeploy(ctx context.Context, bindingID string) (*domain.DeployEvent, error) {
	b, err := s.store.GetBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get binding: %w", err)
	}

	slot, err := s.store.GetSlot(ctx, b.SlotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("slot not found: %w", err)
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}

	connRow, err := s.store.GetProviderConnection(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	cred, err := s.credStore.Resolve(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	p := s.registry.Get(connRow.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", connRow.Provider)
	}

	deployer, ok := p.(provider.Deployer)
	if !ok {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support deployments"}
	}

	conn := storeConnectionToDomain(connRow)
	db := storeBindingToDomain(b)
	event, err := deployer.TriggerDeploy(ctx, &conn, cred, &db, storeSlotToPtr(slot))
	if err != nil {
		return nil, scrubProviderErr(err)
	}

	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if event.UpdatedAt == "" {
		event.UpdatedAt = event.CreatedAt
	}

	s.engine.Invalidate(bindingID)

	return event, nil
}

func (s *DeployService) ListDeployments(ctx context.Context, bindingID string) ([]domain.DeployEvent, error) {
	s.mu.RLock()
	expiresAt, ok := s.expires[bindingID]
	s.mu.RUnlock()
	if ok && time.Now().Before(expiresAt) {
		s.mu.RLock()
		evts := s.deploys[bindingID]
		s.mu.RUnlock()
		return evts, nil
	}

	b, err := s.store.GetBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get binding: %w", err)
	}

	connRow, err := s.store.GetProviderConnection(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	cred, err := s.credStore.Resolve(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	p := s.registry.Get(connRow.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", connRow.Provider)
	}

	deployer, ok := p.(provider.Deployer)
	if !ok {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support deployments"}
	}

	conn := storeConnectionToDomain(connRow)
	db := storeBindingToDomain(b)
	events, err := deployer.ListDeployments(ctx, &conn, cred, &db)
	if err != nil {
		return nil, scrubProviderErr(err)
	}

	s.mu.Lock()
	s.deploys[bindingID] = events
	s.expires[bindingID] = time.Now().Add(deployListTTL)
	s.mu.Unlock()

	return events, nil
}

func (s *DeployService) GetLogs(ctx context.Context, bindingID, deployID string, tail int) (domain.LogChunk, error) {
	b, err := s.store.GetBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LogChunk{}, sql.ErrNoRows
		}
		return domain.LogChunk{}, fmt.Errorf("get binding: %w", err)
	}

	connRow, err := s.store.GetProviderConnection(ctx, b.ConnectionID)
	if err != nil {
		return domain.LogChunk{}, fmt.Errorf("get connection: %w", err)
	}

	cred, err := s.credStore.Resolve(ctx, b.ConnectionID)
	if err != nil {
		return domain.LogChunk{}, fmt.Errorf("resolve credential: %w", err)
	}

	p := s.registry.Get(connRow.Provider)
	if p == nil {
		return domain.LogChunk{}, fmt.Errorf("provider %s not found", connRow.Provider)
	}

	fetcher, ok := p.(provider.LogFetcher)
	if !ok {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support log fetching"}
	}

	conn := storeConnectionToDomain(connRow)
	db := storeBindingToDomain(b)
	chunk, err := fetcher.GetBuildLogs(ctx, &conn, cred, &db, deployID, tail)
	if err != nil {
		return domain.LogChunk{}, scrubProviderErr(err)
	}

	return chunk, nil
}

func storeSlotToPtr(s store.Slot) *domain.Slot {
	ds := &domain.Slot{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		Name:      s.Name,
		Role:      domain.SlotRole(s.Role),
		CreatedAt: s.CreatedAt,
	}
	if s.ConfigJson != "" && s.ConfigJson != "{}" {
		ds.Config = json.RawMessage(s.ConfigJson)
	}
	return ds
}

func scrubProviderErr(err error) error {
	var pErr *provider.Error
	if errors.As(err, &pErr) {
		pErr.ProviderMsg = provider.ScrubTokens(pErr.ProviderMsg)
		return pErr
	}
	return fmt.Errorf("upstream error: %s", provider.ScrubTokens(err.Error()))
}