package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"

	"github.com/google/uuid"
)

var capabilityMatrix = map[domain.ResourceKind][]domain.ProviderType{
	domain.ResourceKindRepo:       {domain.ProviderTypeGitHub},
	domain.ResourceKindCompute:    {domain.ProviderTypeCloudflare},
	domain.ResourceKindStaticSite: {domain.ProviderTypeCloudflare, domain.ProviderTypeVercel},
	domain.ResourceKindDNSDomain:  {domain.ProviderTypeCloudflare, domain.ProviderTypeVercel},
}

var (
	ErrUnsupportedCombo = errors.New("slot type and provider combination not supported")
	ErrDuplicateBinding = errors.New("binding already exists for this slot, account, and external resource")
)

type BindingService struct {
	store    store.Querier
	registry *provider.Registry
	engine   *RefreshEngine
}

func NewBindingService(q store.Querier, reg *provider.Registry, eng *RefreshEngine) *BindingService {
	return &BindingService{store: q, registry: reg, engine: eng}
}

func ValidateCapability(kind domain.ResourceKind, providerType domain.ProviderType) error {
	providers, ok := capabilityMatrix[kind]
	if !ok {
		return ErrUnsupportedCombo
	}
	for _, p := range providers {
		if p == providerType {
			return nil
		}
	}
	return ErrUnsupportedCombo
}

func (s *BindingService) Discover(ctx context.Context, accountID string, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	acct, err := s.store.GetProviderAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	p := s.registry.Get(acct.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", acct.Provider)
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	resources, err := p.ListExternalResources(ctx, provAcct, kind)
	if err != nil {
		return nil, err
	}
	return resources, nil
}

func (s *BindingService) Bind(ctx context.Context, slotID, accountID, externalID string) (*domain.Binding, error) {
	slot, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}

	acct, err := s.store.GetProviderAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get account: %w", err)
	}

	if err := ValidateCapability(domain.ResourceKind(slot.Type), domain.ProviderType(acct.Provider)); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()

	err = s.store.InsertBinding(ctx, store.InsertBindingParams{
		ID:             id,
		SlotID:         slotID,
		AccountID:      accountID,
		Provider:       acct.Provider,
		ExternalID:     externalID,
		CachedMetaJson: "{}",
		SyncStatus:     string(domain.SyncStatusNever),
		LastSyncedAt:   nil,
		CreatedAt:      now,
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateBinding
		}
		return nil, fmt.Errorf("insert binding: %w", err)
	}

	p := s.registry.Get(acct.Provider)
	insp, ok := p.(provider.Inspector)
	if ok {
		provAcct := &domain.ProviderAccount{
			ID:             acct.ID,
			Provider:       domain.ProviderType(acct.Provider),
			Label:          acct.Label,
			TokenEncrypted: acct.EncryptedToken,
		}

		ext, inspectErr := insp.GetResource(ctx, provAcct, externalID)
		if inspectErr == nil && ext != nil {
			metaJSON, _ := json.Marshal(ext.Meta)
			_ = s.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
				ID:             id,
				SyncStatus:     string(domain.SyncStatusOK),
				CachedMetaJson: string(metaJSON),
				LastSyncedAt:   &now,
			})

			db := &domain.Binding{
				ID:           id,
				SlotID:       slotID,
				AccountID:    accountID,
				ExternalID:   externalID,
				CachedMeta:   ext.Meta,
				SyncStatus:   domain.SyncStatusOK,
				LastSyncedAt: now,
				CreatedAt:    now,
			}
			if ext.ExternalID != "" {
				db.ExternalURL = ext.ExternalID
			}
			return db, nil
		}
	}

	db := &domain.Binding{
		ID:         id,
		SlotID:     slotID,
		AccountID:  accountID,
		ExternalID: externalID,
		SyncStatus: domain.SyncStatusNever,
		CreatedAt:  now,
	}
	return db, nil
}

func (s *BindingService) Unbind(ctx context.Context, bindingID string) error {
	err := s.store.DeleteBinding(ctx, bindingID)
	if err != nil {
		return fmt.Errorf("delete binding: %w", err)
	}
	return nil
}

func (s *BindingService) ListBySlot(ctx context.Context, slotID string) ([]domain.Binding, error) {
	bindings, err := s.store.ListBindingsBySlot(ctx, slotID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	result := make([]domain.Binding, len(bindings))
	for i, b := range bindings {
		result[i] = storeBindingToDomain(b)
	}
	return result, nil
}

func (s *BindingService) Refresh(ctx context.Context, bindingID string) (*domain.Binding, error) {
	return s.engine.Refresh(ctx, bindingID)
}

func isUniqueConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint")
}