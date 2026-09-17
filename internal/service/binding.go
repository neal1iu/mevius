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

var (
	ErrUnsupportedCombo = errors.New("product does not support this slot role")
	ErrBindingExists    = errors.New("slot already has a binding")
)

type BindingService struct {
	store     store.Querier
	registry  *provider.Registry
	credStore domain.CredentialStore
	engine    *RefreshEngine
}

func NewBindingService(q store.Querier, reg *provider.Registry, credStore domain.CredentialStore, eng *RefreshEngine) *BindingService {
	return &BindingService{store: q, registry: reg, credStore: credStore, engine: eng}
}

func (s *BindingService) Discover(ctx context.Context, connectionID string, product domain.ProductType) ([]domain.ExternalResource, error) {
	connRow, err := s.store.GetProviderConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	p := s.registry.Get(connRow.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", connRow.Provider)
	}

	disc, ok := p.(provider.Discoverer)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support discovery", connRow.Provider)
	}

	cred, err := s.credStore.Resolve(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	conn := storeConnectionToDomain(connRow)
	resources, err := disc.ListExternalResources(ctx, &conn, cred, product)
	if err != nil {
		return nil, err
	}
	return resources, nil
}

func (s *BindingService) Bind(ctx context.Context, slotID, connectionID string, product domain.ProductType, externalID string) (*domain.Binding, error) {
	slot, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}

	connRow, err := s.store.GetProviderConnection(ctx, connectionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}

	p := s.registry.Get(connRow.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", connRow.Provider)
	}

	desc := p.Descriptor()
	var matchedProduct *domain.ProductDescriptor
	for i := range desc.Products {
		if desc.Products[i].ID == string(product) {
			matchedProduct = &desc.Products[i]
			break
		}
	}
	if matchedProduct == nil {
		return nil, fmt.Errorf("provider %s does not offer product %s", connRow.Provider, product)
	}

	roleSupported := false
	for _, r := range matchedProduct.Roles {
		if r == domain.SlotRole(slot.Role) {
			roleSupported = true
			break
		}
	}
	if !roleSupported {
		return nil, ErrUnsupportedCombo
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()

	err = s.store.InsertBinding(ctx, store.InsertBindingParams{
		ID:             id,
		SlotID:         slotID,
		ConnectionID:   connectionID,
		Product:        string(product),
		ExternalID:     externalID,
		CachedMetaJson: "{}",
		SyncStatus:     string(domain.SyncStatusNever),
		LastSyncedAt:   nil,
		CreatedAt:      now,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, ErrBindingExists
		}
		return nil, fmt.Errorf("insert binding: %w", err)
	}

	cred, credErr := s.credStore.Resolve(ctx, connectionID)
	insp, inspOK := p.(provider.Inspector)
	if inspOK && credErr == nil {
		conn := storeConnectionToDomain(connRow)
		ext, inspectErr := insp.GetResource(ctx, &conn, cred, externalID)
		if inspectErr == nil && ext != nil {
			metaJSON, _ := json.Marshal(ext.Meta)
			_ = s.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
				ID:             id,
				SyncStatus:     string(domain.SyncStatusOK),
				CachedMetaJson: string(metaJSON),
				LastSyncedAt:   &now,
			})

			return &domain.Binding{
				ID:           id,
				SlotID:       slotID,
				ConnectionID: connectionID,
				ExternalID:   externalID,
				ExternalURL:  ext.ExternalID,
				Product:      product,
				CachedMeta:   ext.Meta,
				SyncStatus:   domain.SyncStatusOK,
				LastSyncedAt: now,
				CreatedAt:    now,
			}, nil
		}
	}

	return &domain.Binding{
		ID:           id,
		SlotID:       slotID,
		ConnectionID: connectionID,
		ExternalID:   externalID,
		Product:      product,
		SyncStatus:   domain.SyncStatusNever,
		CreatedAt:    now,
	}, nil
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