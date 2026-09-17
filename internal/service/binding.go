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
	ErrUnsupportedCombo = errors.New("slot role and product combination not supported")
	ErrDuplicateBinding = errors.New("binding already exists for this slot, connection, and external resource")
)

type BindingService struct {
	store    store.Querier
	registry *provider.Registry
	credStore domain.CredentialStore
	engine   *RefreshEngine
}

func NewBindingService(q store.Querier, reg *provider.Registry, credStore domain.CredentialStore, eng *RefreshEngine) *BindingService {
	return &BindingService{store: q, registry: reg, credStore: credStore, engine: eng}
}

func (s *BindingService) Discover(ctx context.Context, connectionID, product string) ([]domain.ExternalResource, error) {
	conn, err := s.store.GetProviderConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	p := s.registry.Get(conn.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", conn.Provider)
	}

	kind, err := productResourceKind(s.registry, product)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", product, err)
	}

	lister, ok := p.(provider.ResourceLister)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support resource listing", conn.Provider)
	}

	cred, err := s.resolveCredential(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	provConn := &domain.ProviderConnection{
		ID:       conn.ID,
		Provider: domain.ProviderType(conn.Provider),
		Label:    conn.Label,
		Endpoint: conn.Endpoint,
	}

	resources, err := lister.ListExternalResources(ctx, provConn, cred, kind)
	if err != nil {
		return nil, err
	}
	return resources, nil
}

func (s *BindingService) Bind(ctx context.Context, slotID, connectionID, product, externalID string) (*domain.Binding, error) {
	slot, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}

	conn, err := s.store.GetProviderConnection(ctx, connectionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}

	if err := validateBindingCombo(s.registry, domain.SlotRole(slot.Role), domain.ProviderType(conn.Provider), product); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()

	err = s.store.InsertBinding(ctx, store.InsertBindingParams{
		ID:             id,
		SlotID:         slotID,
		ConnectionID:   connectionID,
		Product:        product,
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

	cred, credErr := s.resolveCredential(ctx, connectionID)
	if credErr == nil {
		p := s.registry.Get(conn.Provider)
		insp, ok := p.(provider.Inspector)
		if ok {
			provConn := &domain.ProviderConnection{
				ID:       conn.ID,
				Provider: domain.ProviderType(conn.Provider),
				Label:    conn.Label,
				Endpoint: conn.Endpoint,
			}
			ext, inspectErr := insp.GetResource(ctx, provConn, cred, externalID)
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
					ConnectionID: connectionID,
					Product:      domain.ProductType(product),
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
	}

	db := &domain.Binding{
		ID:           id,
		SlotID:       slotID,
		ConnectionID: connectionID,
		Product:      domain.ProductType(product),
		ExternalID:   externalID,
		SyncStatus:   domain.SyncStatusNever,
		CreatedAt:    now,
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
		result[i] = *bindingToDomain(b)
	}
	return result, nil
}

func (s *BindingService) Refresh(ctx context.Context, bindingID string) (*domain.Binding, error) {
	return s.engine.Refresh(ctx, bindingID)
}

func (s *BindingService) resolveCredential(ctx context.Context, connectionID string) ([]byte, error) {
	return s.credStore.Resolve(ctx, connectionID)
}

func validateBindingCombo(reg *provider.Registry, role domain.SlotRole, providerType domain.ProviderType, productID string) error {
	p := reg.Get(string(providerType))
	if p == nil {
		return fmt.Errorf("%w: provider %s not found", ErrUnsupportedCombo, providerType)
	}
	desc := p.Descriptor()
	for _, prod := range desc.Products {
		if prod.ID != productID {
			continue
		}
		for _, r := range prod.Roles {
			if r == role {
				return nil
			}
		}
		return fmt.Errorf("%w: product %s does not support role %s", ErrUnsupportedCombo, productID, role)
	}
	return fmt.Errorf("%w: product %s not found for provider %s", ErrUnsupportedCombo, productID, providerType)
}

func productResourceKind(reg *provider.Registry, productID string) (domain.ResourceKind, error) {
	for _, p := range reg.All() {
		desc := p.Descriptor()
		for _, prod := range desc.Products {
			if prod.ID == productID {
				return prod.ResourceKind, nil
			}
		}
	}
	return "", ErrUnsupportedCombo
}

func isUniqueConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint")
}
