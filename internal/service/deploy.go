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
	store    store.Querier
	registry *provider.Registry
	engine   *RefreshEngine

	mu      sync.RWMutex
	deploys map[string][]domain.DeployEvent
	expires map[string]time.Time
}

func NewDeployService(q store.Querier, reg *provider.Registry, eng *RefreshEngine) *DeployService {
	return &DeployService{
		store:    q,
		registry: reg,
		engine:   eng,
		deploys:  make(map[string][]domain.DeployEvent),
		expires:  make(map[string]time.Time),
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

	acct, err := s.store.GetProviderAccount(ctx, b.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	p := s.registry.Get(b.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", b.Provider)
	}

	deployer, ok := p.(provider.Deployer)
	if !ok {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support deployments"}
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	event, err := deployer.TriggerDeploy(ctx, provAcct, bindingToDomain(b), storeSlotToPtr(slot))
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

	acct, err := s.store.GetProviderAccount(ctx, b.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	p := s.registry.Get(b.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", b.Provider)
	}

	deployer, ok := p.(provider.Deployer)
	if !ok {
		return nil, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support deployments"}
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	events, err := deployer.ListDeployments(ctx, provAcct, bindingToDomain(b))
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

	acct, err := s.store.GetProviderAccount(ctx, b.AccountID)
	if err != nil {
		return domain.LogChunk{}, fmt.Errorf("get account: %w", err)
	}

	p := s.registry.Get(b.Provider)
	if p == nil {
		return domain.LogChunk{}, fmt.Errorf("provider %s not found", b.Provider)
	}

	fetcher, ok := p.(provider.LogFetcher)
	if !ok {
		return domain.LogChunk{}, &provider.Error{Kind: provider.KindUnsupported, ProviderMsg: "provider does not support log fetching"}
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	chunk, err := fetcher.GetBuildLogs(ctx, provAcct, bindingToDomain(b), deployID, tail)
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
		Kind:      domain.ResourceKind(s.Type),
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