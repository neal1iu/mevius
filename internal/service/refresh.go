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
	store    store.Querier
	registry *provider.Registry
	sf       singleflight.Group
	mu       sync.RWMutex
	entries  map[string]time.Time
}

func NewRefreshEngine(q store.Querier, reg *provider.Registry) *RefreshEngine {
	return &RefreshEngine{
		store:    q,
		registry: reg,
		entries:  make(map[string]time.Time),
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

func (e *RefreshEngine) FanOutRefresh(ctx context.Context, accountID, externalID string) error {
	bindings, err := e.store.FanOutBindingsByAccountExternal(ctx, store.FanOutBindingsByAccountExternalParams{
		AccountID:  accountID,
		ExternalID: externalID,
	})
	if err != nil {
		return fmt.Errorf("fanout bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil
	}

	acct, err := e.store.GetProviderAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("fanout account: %w", err)
	}

	p := e.registry.Get(acct.Provider)
	if p == nil {
		return fmt.Errorf("fanout provider %s not found", acct.Provider)
	}
	insp, ok := p.(provider.Inspector)
	if !ok {
		return fmt.Errorf("fanout provider %s not an Inspector", acct.Provider)
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	ext, err := insp.GetResource(ctx, provAcct, externalID)
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

	acct, err := e.store.GetProviderAccount(ctx, b.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	p := e.registry.Get(b.Provider)
	if p == nil {
		return nil, fmt.Errorf("provider %s not found", b.Provider)
	}
	insp, ok := p.(provider.Inspector)
	if !ok {
		return nil, fmt.Errorf("provider %s not an Inspector", b.Provider)
	}

	provAcct := &domain.ProviderAccount{
		ID:             acct.ID,
		Provider:       domain.ProviderType(acct.Provider),
		Label:          acct.Label,
		TokenEncrypted: acct.EncryptedToken,
	}

	ext, err := insp.GetResource(ctx, provAcct, b.ExternalID)
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
	db := &domain.Binding{
		ID:           b.ID,
		SlotID:       b.SlotID,
		AccountID:    b.AccountID,
		ExternalID:   b.ExternalID,
		SyncStatus:   domain.SyncStatus(b.SyncStatus),
		CreatedAt:    b.CreatedAt,
	}
	if b.LastSyncedAt != nil {
		db.LastSyncedAt = *b.LastSyncedAt
	}
	if b.CachedMetaJson != "" && b.CachedMetaJson != "{}" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(b.CachedMetaJson), &meta); err == nil {
			db.CachedMeta = meta
		}
	}
	return db
}