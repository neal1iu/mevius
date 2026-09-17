package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type stubCredStore struct{}

func (s *stubCredStore) Resolve(ctx context.Context, ref string) ([]byte, error) {
	return []byte("test-credential"), nil
}

type fakeQuerier struct {
	mu               sync.Mutex
	bindings         map[string]store.Binding
	connections      map[string]store.ProviderConnection
	slots            map[string]store.Slot
	insertBindingErr error
	updateStatusFn   func(ctx context.Context, arg store.UpdateBindingSyncStatusParams) error
}

func newFakeQuerier() *fakeQuerier {
	return &fakeQuerier{
		bindings:    make(map[string]store.Binding),
		connections: make(map[string]store.ProviderConnection),
		slots:       make(map[string]store.Slot),
	}
}

func (f *fakeQuerier) addBinding(b store.Binding) {
	f.mu.Lock()
	f.bindings[b.ID] = b
	f.mu.Unlock()
}

func (f *fakeQuerier) addConnection(a store.ProviderConnection) {
	f.mu.Lock()
	f.connections[a.ID] = a
	f.mu.Unlock()
}

func (f *fakeQuerier) addSlot(s store.Slot) {
	f.mu.Lock()
	f.slots[s.ID] = s
	f.mu.Unlock()
}

func (f *fakeQuerier) setInsertBindingErr(err error) {
	f.mu.Lock()
	f.insertBindingErr = err
	f.mu.Unlock()
}

func (f *fakeQuerier) FanOutBindingsByAccountExternal(ctx context.Context, arg store.FanOutBindingsByAccountExternalParams) ([]store.Binding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []store.Binding
	for _, b := range f.bindings {
		if b.ConnectionID == arg.ConnectionID && b.ExternalID == arg.ExternalID {
			result = append(result, b)
		}
	}
	return result, nil
}

func (f *fakeQuerier) GetBinding(ctx context.Context, id string) (store.Binding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.bindings[id]
	if !ok {
		return store.Binding{}, sql.ErrNoRows
	}
	return b, nil
}

func (f *fakeQuerier) GetProviderConnection(ctx context.Context, id string) (store.ProviderConnection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.connections[id]
	if !ok {
		return store.ProviderConnection{}, sql.ErrNoRows
	}
	return a, nil
}

func (f *fakeQuerier) GetProviderConnectionCredential(ctx context.Context, id string) (string, error) {
	return "encrypted", nil
}

func (f *fakeQuerier) UpdateBindingSyncStatus(ctx context.Context, arg store.UpdateBindingSyncStatusParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateStatusFn != nil {
		return f.updateStatusFn(ctx, arg)
	}
	b, ok := f.bindings[arg.ID]
	if !ok {
		return sql.ErrNoRows
	}
	b.SyncStatus = arg.SyncStatus
	b.CachedMetaJson = arg.CachedMetaJson
	b.LastSyncedAt = arg.LastSyncedAt
	f.bindings[arg.ID] = b
	return nil
}

func (f *fakeQuerier) DeleteBinding(ctx context.Context, id string) error          { return nil }
func (f *fakeQuerier) DeleteProject(ctx context.Context, id string) error          { return nil }
func (f *fakeQuerier) DeleteProviderConnection(ctx context.Context, id string) error { return nil }
func (f *fakeQuerier) DeleteSlot(ctx context.Context, id string) error              { return nil }
func (f *fakeQuerier) GetProject(ctx context.Context, id string) (store.Project, error) {
	return store.Project{}, nil
}
func (f *fakeQuerier) GetProjectByName(ctx context.Context, name string) (store.Project, error) {
	return store.Project{}, nil
}
func (f *fakeQuerier) GetSlot(ctx context.Context, id string) (store.Slot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.slots[id]
	if !ok {
		return store.Slot{}, sql.ErrNoRows
	}
	return s, nil
}
func (f *fakeQuerier) InsertBinding(ctx context.Context, arg store.InsertBindingParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertBindingErr != nil {
		return f.insertBindingErr
	}
	f.bindings[arg.ID] = store.Binding{
		ID: arg.ID, SlotID: arg.SlotID, ConnectionID: arg.ConnectionID,
		Product: arg.Product, ExternalID: arg.ExternalID,
		CachedMetaJson: arg.CachedMetaJson, SyncStatus: arg.SyncStatus,
		LastSyncedAt: arg.LastSyncedAt, CreatedAt: arg.CreatedAt,
	}
	return nil
}
func (f *fakeQuerier) InsertProject(ctx context.Context, arg store.InsertProjectParams) error {
	return nil
}
func (f *fakeQuerier) InsertProviderConnection(ctx context.Context, arg store.InsertProviderConnectionParams) error {
	return nil
}
func (f *fakeQuerier) InsertSlot(ctx context.Context, arg store.InsertSlotParams) error { return nil }
func (f *fakeQuerier) ListBindingsBySlot(ctx context.Context, slotID string) ([]store.Binding, error) {
	return nil, nil
}
func (f *fakeQuerier) ListBindingsBySlots(ctx context.Context, slotIDs []string) ([]store.Binding, error) {
	return nil, nil
}
func (f *fakeQuerier) ListProjects(ctx context.Context) ([]store.Project, error) { return nil, nil }
func (f *fakeQuerier) ListProviderConnections(ctx context.Context) ([]store.ProviderConnection, error) {
	return nil, nil
}
func (f *fakeQuerier) ListSlotsByProject(ctx context.Context, projectID string) ([]store.Slot, error) {
	return nil, nil
}
func (f *fakeQuerier) UpdateProject(ctx context.Context, arg store.UpdateProjectParams) error {
	return nil
}
func (f *fakeQuerier) UpdateSlot(ctx context.Context, arg store.UpdateSlotParams) error  { return nil }
func (f *fakeQuerier) UpdateSlotConfig(ctx context.Context, arg store.UpdateSlotConfigParams) error {
	return nil
}

var _ store.Querier = (*fakeQuerier)(nil)

type countingProvider struct {
	callCount atomic.Int64
	result    *domain.ExternalResource
	err       error
}

func (p *countingProvider) Type() string { return "fake" }

func (p *countingProvider) Descriptor() domain.ProviderDescriptor {
	return domain.ProviderDescriptor{Type: domain.ProviderType("fake")}
}

func (p *countingProvider) ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	return json.RawMessage(`{"account_id":"fake-account"}`), nil
}

func (p *countingProvider) ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	return nil, nil
}

func (p *countingProvider) GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
	p.callCount.Add(1)
	if p.err != nil {
		return nil, p.err
	}
	return p.result, nil
}

func setUp(t *testing.T) (*RefreshEngine, *countingProvider, *fakeQuerier, string) {
	t.Helper()
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	cp := &countingProvider{
		result: &domain.ExternalResource{
			ExternalID:  "ext-1",
			DisplayName: "test-resource",
			Meta:        map[string]any{"key": "value"},
		},
	}
	reg.Register(cp)

	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{"old":"data"}`, SyncStatus: string(domain.SyncStatusOK),
	})

	return NewRefreshEngine(q, reg, &stubCredStore{}), cp, q, "bnd-1"
}

type providerAdapter struct {
	typeFn        func() string
	descriptorFn  func() domain.ProviderDescriptor
	validateFn    func(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error)
	listFn        func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, kind domain.ResourceKind) ([]domain.ExternalResource, error)
	getResourceFn func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error)
}

func (a *providerAdapter) Type() string {
	if a.typeFn != nil { return a.typeFn() }
	return "mock"
}

func (a *providerAdapter) Descriptor() domain.ProviderDescriptor {
	if a.descriptorFn != nil { return a.descriptorFn() }
	return domain.ProviderDescriptor{Type: domain.ProviderType("mock")}
}

func (a *providerAdapter) ValidateCredentials(ctx context.Context, conn *domain.ProviderConnection, credential []byte) (json.RawMessage, error) {
	if a.validateFn != nil { return a.validateFn(ctx, conn, credential) }
	return json.RawMessage(`{}`), nil
}

func (a *providerAdapter) ListExternalResources(ctx context.Context, conn *domain.ProviderConnection, credential []byte, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	if a.listFn != nil { return a.listFn(ctx, conn, credential, kind) }
	return nil, nil
}

func (a *providerAdapter) GetResource(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
	if a.getResourceFn != nil { return a.getResourceFn(ctx, conn, credential, externalID) }
	return nil, errors.New("not implemented")
}

var _ provider.Provider = (*providerAdapter)(nil)
var _ provider.Inspector = (*providerAdapter)(nil)

func TestRefreshCacheHit(t *testing.T) {
	eng, cp, _, bindingID := setUp(t)
	ctx := context.Background()

	b, err := eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }
	if n := cp.callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
	if b.CachedMeta == nil || b.CachedMeta["key"] != "value" {
		t.Fatalf("expected fresh meta, got %+v", b.CachedMeta)
	}

	b2, err := eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }
	if n := cp.callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call total (cache hit), got %d", n)
	}
	if b2.SyncStatus != domain.SyncStatusOK {
		t.Fatalf("expected sync_status=ok, got %s", b2.SyncStatus)
	}
}

func TestRefreshCacheMiss(t *testing.T) {
	eng, cp, q, bindingID := setUp(t)
	ctx := context.Background()

	eng.mu.Lock()
	eng.entries[bindingID] = time.Now().Add(-1 * time.Second)
	eng.mu.Unlock()
	q.mu.Lock()
	b := q.bindings[bindingID]
	b.CachedMetaJson = `{"old":"data"}`
	q.bindings[bindingID] = b
	q.mu.Unlock()
	cp.callCount.Store(0)

	b2, err := eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }
	if n := cp.callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
	if b2.CachedMeta["key"] != "value" {
		t.Fatalf("expected refreshed meta, got %+v", b2.CachedMeta)
	}
}

func TestRefreshConcurrent(t *testing.T) {
	q := newFakeQuerier()
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusNever),
	})

	reg := provider.NewRegistry()
	callCount := atomic.Int64{}
	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			callCount.Add(1)
			time.Sleep(10 * time.Millisecond)
			return &domain.ExternalResource{
				ExternalID: "ext-1", DisplayName: "test",
				Meta: map[string]any{"key": "value"},
			}, nil
		},
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := eng.GetBindingStatus(ctx, "bnd-1")
			if err != nil { errs <- err }
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected exactly 1 upstream call for 20 concurrent, got %d", n)
	}
	dbB, _ := q.GetBinding(ctx, "bnd-1")
	if dbB.SyncStatus != string(domain.SyncStatusOK) {
		t.Fatalf("expected sync_status=ok, got %s", dbB.SyncStatus)
	}
}

func TestRefreshNegativeTTL(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()

	callCount := atomic.Int64{}
	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			callCount.Add(1)
			return nil, fmt.Errorf("upstream error: %w", &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "internal error"})
		},
	})

	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusNever),
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	b, err := eng.GetBindingStatus(ctx, "bnd-1")
	if err != nil { t.Fatal(err) }
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
	if b.SyncStatus != domain.SyncStatusError {
		t.Fatalf("expected sync_status=error, got %s", b.SyncStatus)
	}

	b2, err := eng.GetBindingStatus(ctx, "bnd-1")
	if err != nil { t.Fatal(err) }
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected 0 additional upstream calls within negative TTL, got %d", n)
	}
	if b2.SyncStatus != domain.SyncStatusError {
		t.Fatalf("expected sync_status=error, got %s", b2.SyncStatus)
	}
}

func TestRefreshManual(t *testing.T) {
	eng, cp, _, bindingID := setUp(t)
	ctx := context.Background()

	_, err := eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }

	cp.callCount.Store(0)
	b, err := eng.Refresh(ctx, bindingID)
	if err != nil { t.Fatal(err) }
	if n := cp.callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call after manual refresh, got %d", n)
	}
	if b.SyncStatus != domain.SyncStatusOK {
		t.Fatalf("expected sync_status=ok, got %s", b.SyncStatus)
	}
}

func TestRefreshFanOut(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()

	callCount := atomic.Int64{}
	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			callCount.Add(1)
			return &domain.ExternalResource{
				ExternalID: externalID, DisplayName: "shared-resource",
				Meta: map[string]any{"updated": true},
			}, nil
		},
	})

	for i := 0; i < 3; i++ {
		q.addBinding(store.Binding{
			ID: fmt.Sprintf("bnd-%d", i), SlotID: fmt.Sprintf("slot-%d", i),
			ConnectionID: "acct-1", Product: "fake", ExternalID: "shared-ext",
			CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusNever),
		})
	}
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	err := eng.FanOutRefresh(ctx, "acct-1", "shared-ext")
	if err != nil { t.Fatal(err) }
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected exactly 1 upstream call for fanout, got %d", n)
	}
	for i := 0; i < 3; i++ {
		b, err := q.GetBinding(ctx, fmt.Sprintf("bnd-%d", i))
		if err != nil { t.Fatal(err) }
		if b.SyncStatus != string(domain.SyncStatusOK) {
			t.Fatalf("binding %d expected sync_status=ok, got %s", i, b.SyncStatus)
		}
	}
}

func TestRefreshNotFound(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	callCount := atomic.Int64{}

	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			callCount.Add(1)
			return nil, &provider.Error{Kind: provider.KindNotFound}
		},
	})
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusOK),
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	b, err := eng.GetBindingStatus(ctx, "bnd-1")
	if err != nil { t.Fatal(err) }
	if b.SyncStatus != domain.SyncStatusOrphaned {
		t.Fatalf("expected sync_status=orphaned, got %s", b.SyncStatus)
	}
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
}

func TestRefreshUnauthorized(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	callCount := atomic.Int64{}

	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			callCount.Add(1)
			return nil, &provider.Error{Kind: provider.KindUnauthorized}
		},
	})
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusOK),
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	b, err := eng.GetBindingStatus(ctx, "bnd-1")
	if err != nil { t.Fatal(err) }
	if b.SyncStatus != domain.SyncStatusAuthError {
		t.Fatalf("expected sync_status=auth_error, got %s", b.SyncStatus)
	}
	if n := callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
}

func TestRefreshInvalidate(t *testing.T) {
	eng, cp, _, bindingID := setUp(t)
	ctx := context.Background()

	_, err := eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }

	cp.callCount.Store(0)
	eng.Invalidate(bindingID)
	_, err = eng.GetBindingStatus(ctx, bindingID)
	if err != nil { t.Fatal(err) }
	if n := cp.callCount.Load(); n != 1 {
		t.Fatalf("expected 1 upstream call after invalidate, got %d", n)
	}
}

func TestRefreshConcurrentRace(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()

	reg.Register(&providerAdapter{
		typeFn: func() string { return "fake" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			time.Sleep(10 * time.Millisecond)
			return &domain.ExternalResource{
				ExternalID: "ext-1", DisplayName: "test",
				Meta: map[string]any{"key": "value"},
			}, nil
		},
	})
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "fake", Label: "test", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: string(domain.SyncStatusNever),
	})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eng.GetBindingStatus(ctx, "bnd-1")
			eng.Invalidate("bnd-1", "other-key")
			eng.Refresh(ctx, "bnd-1")
			eng.FanOutRefresh(ctx, "acct-1", "ext-1")
		}()
	}
	wg.Wait()
}