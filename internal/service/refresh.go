package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

const (
	ttlOK       = 60 * time.Second
	ttlNegative = 30 * time.Second
)

type RefreshEngine struct {
	store     store.Querier
	registry  *provider.Registry
	credStore domain.CredentialStore
	sf        singleflight.Group
	mu        sync.RWMutex
	entries   map[string]time.Time
}

func NewRefreshEngine(q store.Querier, reg *provider.Registry, credStore domain.CredentialStore) *RefreshEngine {
	return &RefreshEngine{
		store:     q,
		registry:  reg,
		credStore: credStore,
		entries:   make(map[string]time.Time),
	}
}

func (e *RefreshEngine) GetBindingStatus(ctx context.Context, bindingID string) (*domain.Binding, error) {
	if b, ok := e.hit(ctx, bindingID); ok {
		return b, nil
	}
	result, err, _ := e.sf.Do(bindingID, func() (interface{}, error) {
		return e.doFetch(ctx, bindingID)
	})
	if err != nil {
		return nil, err
	}
	return result.(*domain.Binding), nil
}

func (e *RefreshEngine) Refresh(ctx context.Context, bindingID string) (*domain.Binding, error) {
	e.mu.Lock()
	delete(e.entries, bindingID)
	e.mu.Unlock()
	return e.GetBindingStatus(ctx, bindingID)
}

func (e *RefreshEngine) Invalidate(keys ...string) {
	e.mu.Lock()
	for _, k := range keys {
		delete(e.entries, k)
	}
	e.mu.Unlock()
}

func (e *RefreshEngine) FanOutRefresh(ctx context.Context, connectionID, externalID string) error {
	bindings, err := e.store.FanOutBindingsByAccountExternal(ctx, store.FanOutBindingsByAccountExternalParams{
		ConnectionID: connectionID,
		ExternalID:   externalID,
	})
	if err != nil {
		return fmt.Errorf("fanout bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil
	}

	conn, err := e.store.GetProviderConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("fanout connection: %w", err)
	}

	cred, err := e.credStore.Resolve(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("fanout credential: %w", err)
	}

	p := e.registry.Get(conn.Provider)
	if p == nil {
		return fmt.Errorf("fanout provider %s not found", conn.Provider)
	}
	insp, ok := p.(provider.Inspector)
	if !ok {
		return fmt.Errorf("fanout provider %s not an Inspector", conn.Provider)
	}

	provConn := &domain.ProviderConnection{
		ID:       conn.ID,
		Provider: domain.ProviderType(conn.Provider),
		Label:    conn.Label,
		Endpoint: conn.Endpoint,
	}

	ext, err := insp.GetResource(ctx, provConn, cred, externalID)
	if err != nil {
		return e.fanoutError(ctx, bindings, err)
	}

	metaJSON, _ := json.Marshal(ext.Meta)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, b := range bindings {
		_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
			ID:             b.ID,
			SyncStatus:     string(domain.SyncStatusOK),
			CachedMetaJson: string(metaJSON),
			LastSyncedAt:   &now,
		})
		e.setTTL(b.ID, ttlOK)
	}
	return nil
}

func (e *RefreshEngine) hit(ctx context.Context, bindingID string) (*domain.Binding, bool) {
	e.mu.RLock()
	expiresAt, ok := e.entries[bindingID]
	e.mu.RUnlock()
	if !ok || time.Now().After(expiresAt) {
		return nil, false
	}
	b, err := e.store.GetBinding(ctx, bindingID)
	if err != nil {
		return nil, false
	}
	return bindingToDomain(b), true
}

func (e *RefreshEngine) doFetch(ctx context.Context, bindingID string) (*domain.Binding, error) {
	b, err := e.store.GetBinding(ctx, bindingID)
	if err != nil {
		return nil, fmt.Errorf("get binding: %w", err)
	}

	conn, err := e.store.GetProviderConnection(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	cred, err := e.credStore.Resolve(ctx, b.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	p := e.registry.Get(conn.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", conn.Provider)
	}
	insp, ok := p.(provider.Inspector)
	if !ok {
		return nil, fmt.Errorf("provider %s not an Inspector", conn.Provider)
	}

	provConn := &domain.ProviderConnection{
		ID:       conn.ID,
		Provider: domain.ProviderType(conn.Provider),
		Label:    conn.Label,
		Endpoint: conn.Endpoint,
	}

	ext, err := insp.GetResource(ctx, provConn, cred, b.ExternalID)
	if err != nil {
		var pErr *provider.Error
		if errors.As(err, &pErr) {
			switch pErr.Kind {
			case provider.KindNotFound:
				now := time.Now().UTC().Format(time.RFC3339)
				_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
					ID:             b.ID,
					SyncStatus:     string(domain.SyncStatusOrphaned),
					CachedMetaJson: b.CachedMetaJson,
					LastSyncedAt:   &now,
				})
				e.setTTL(bindingID, ttlOK)
				db := bindingToDomain(b)
				db.SyncStatus = domain.SyncStatusOrphaned
				db.LastSyncedAt = now
				return db, nil
			case provider.KindUnauthorized:
				now := time.Now().UTC().Format(time.RFC3339)
				_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
					ID:             b.ID,
					SyncStatus:     string(domain.SyncStatusAuthError),
					CachedMetaJson: b.CachedMetaJson,
					LastSyncedAt:   &now,
				})
				e.setTTL(bindingID, ttlOK)
				db := bindingToDomain(b)
				db.SyncStatus = domain.SyncStatusAuthError
				db.LastSyncedAt = now
				return db, nil
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
			ID:             b.ID,
			SyncStatus:     string(domain.SyncStatusError),
			CachedMetaJson: b.CachedMetaJson,
			LastSyncedAt:   &now,
		})
		e.setTTL(bindingID, ttlNegative)
		db := bindingToDomain(b)
		db.SyncStatus = domain.SyncStatusError
		db.LastSyncedAt = now
		return db, nil
	}

	metaJSON, _ := json.Marshal(ext.Meta)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
		ID:             b.ID,
		SyncStatus:     string(domain.SyncStatusOK),
		CachedMetaJson: string(metaJSON),
		LastSyncedAt:   &now,
	})
	e.setTTL(bindingID, ttlOK)
	db := bindingToDomain(b)
	db.SyncStatus = domain.SyncStatusOK
	db.CachedMeta = ext.Meta
	db.LastSyncedAt = now
	return db, nil
}

func (e *RefreshEngine) fanoutError(ctx context.Context, bindings []store.Binding, err error) error {
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		now := time.Now().UTC().Format(time.RFC3339)
		for _, b := range bindings {
			_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
				ID:             b.ID,
				SyncStatus:     string(domain.SyncStatusError),
				CachedMetaJson: b.CachedMetaJson,
				LastSyncedAt:   &now,
			})
			e.setTTL(b.ID, ttlNegative)
		}
		return err
	}

	var status domain.SyncStatus
	switch pErr.Kind {
	case provider.KindNotFound:
		status = domain.SyncStatusOrphaned
	case provider.KindUnauthorized:
		status = domain.SyncStatusAuthError
	default:
		status = domain.SyncStatusError
	}

	var ttl time.Duration
	if status == domain.SyncStatusError {
		ttl = ttlNegative
	} else {
		ttl = ttlOK
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, b := range bindings {
		_ = e.store.UpdateBindingSyncStatus(ctx, store.UpdateBindingSyncStatusParams{
			ID:             b.ID,
			SyncStatus:     string(status),
			CachedMetaJson: b.CachedMetaJson,
			LastSyncedAt:   &now,
		})
		e.setTTL(b.ID, ttl)
	}
	if status == domain.SyncStatusError {
		return err
	}
	return nil
}

func (e *RefreshEngine) setTTL(bindingID string, d time.Duration) {
	e.mu.Lock()
	e.entries[bindingID] = time.Now().Add(d)
	e.mu.Unlock()
}

func bindingToDomain(b store.Binding) *domain.Binding {
	meta := make(map[string]any)
	if b.CachedMetaJson != "" && b.CachedMetaJson != "{}" {
		json.Unmarshal([]byte(b.CachedMetaJson), &meta)
	}
	db := &domain.Binding{
		ID:           b.ID,
		SlotID:       b.SlotID,
		ConnectionID: b.ConnectionID,
		Product:      domain.ProductType(b.Product),
		ExternalID:   b.ExternalID,
		CachedMeta:   meta,
		SyncStatus:   domain.SyncStatus(b.SyncStatus),
		CreatedAt:    b.CreatedAt,
	}
	if b.LastSyncedAt != nil {
		db.LastSyncedAt = *b.LastSyncedAt
	}
	return db
}


