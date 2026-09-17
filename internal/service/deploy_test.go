package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type fakeDeployProvider struct {
	*providerAdapter
	deployTriggerFn func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error)
	deployListFn    func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error)
	logFetchFn      func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error)
}

func (a *fakeDeployProvider) TriggerDeploy(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
	if a.deployTriggerFn != nil {
		return a.deployTriggerFn(ctx, conn, credential, binding, slot)
	}
	return &domain.DeployEvent{ID: "evt-1", Status: "queued"}, nil
}

func (a *fakeDeployProvider) ListDeployments(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error) {
	if a.deployListFn != nil {
		return a.deployListFn(ctx, conn, credential, binding)
	}
	return nil, nil
}

func (a *fakeDeployProvider) GetBuildLogs(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
	if a.logFetchFn != nil {
		return a.logFetchFn(ctx, conn, credential, binding, deployID, tail)
	}
	return domain.LogChunk{}, nil
}

func deploySetUp(t *testing.T) (*DeployService, *fakeDeployProvider, *fakeQuerier, string) {
	t.Helper()
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	dp := &fakeDeployProvider{
		providerAdapter: &providerAdapter{typeFn: func() string { return "cloudflare" }},
	}
	reg.Register(dp)
	q.addConnection(store.ProviderConnection{
		ID: "acct-1", Provider: "cloudflare", Label: "cf", EncryptedCredential: "encrypted",
	})
	q.addBinding(store.Binding{
		ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "cloudflare",
		ExternalID: "ext-1", CachedMetaJson: `{}`, SyncStatus: "ok",
	})
	q.addSlot(store.Slot{
		ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "site",
		ConfigJson: `{"name":"site"}`, CreatedAt: "now",
	})
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewDeployService(q, reg, &stubCredStore{}, eng)
	return svc, dp, q, "bnd-1"
}

func TestTriggerDeploySuccess(t *testing.T) {
	svc, dp, _, bindingID := deploySetUp(t)
	dp.deployTriggerFn = func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
		return &domain.DeployEvent{ID: "evt-1", Status: "queued", CreatedAt: "now", UpdatedAt: "now"}, nil
	}

	evt, err := svc.TriggerDeploy(context.Background(), bindingID)
	if err != nil {
		t.Fatalf("TriggerDeploy: %v", err)
	}
	if evt.ID != "evt-1" {
		t.Fatalf("expected event ID evt-1, got %s", evt.ID)
	}
	if evt.Status != "queued" {
		t.Fatalf("expected status queued, got %s", evt.Status)
	}
}

func TestTriggerDeployBindingNotFound(t *testing.T) {
	svc, _, _, _ := deploySetUp(t)
	_, err := svc.TriggerDeploy(context.Background(), "nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestTriggerDeploySlotNotFound(t *testing.T) {
	svc, _, q, bindingID := deploySetUp(t)
	q.mu.Lock()
	b := q.bindings[bindingID]
	b.SlotID = "nonexistent-slot"
	q.bindings[bindingID] = b
	q.mu.Unlock()

	_, err := svc.TriggerDeploy(context.Background(), bindingID)
	if err == nil {
		t.Fatal("expected error for nonexistent slot")
	}
}

func TestTriggerDeployAccountNotFound(t *testing.T) {
	svc, _, q, bindingID := deploySetUp(t)
	q.mu.Lock()
	b := q.bindings[bindingID]
	b.ConnectionID = "nonexistent-connection"
	q.bindings[bindingID] = b
	q.mu.Unlock()

	_, err := svc.TriggerDeploy(context.Background(), bindingID)
	if err == nil {
		t.Fatal("expected error for nonexistent connection")
	}
}

func TestTriggerDeployProviderNotFound(t *testing.T) {
	svc, _, q, bindingID := deploySetUp(t)
	q.mu.Lock()
	b := q.bindings[bindingID]
	b.Product = "nonexistent"
	q.bindings[bindingID] = b
	conn := q.connections["acct-1"]
	conn.Provider = "nonexistent"
	q.connections["acct-1"] = conn
	q.mu.Unlock()

	_, err := svc.TriggerDeploy(context.Background(), bindingID)
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestTriggerDeployUnsupported(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	reg.Register(&providerAdapter{
		typeFn: func() string { return "nondep" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			return &domain.ExternalResource{ExternalID: externalID, Meta: map[string]any{}}, nil
		},
	})
	q.addConnection(store.ProviderConnection{ID: "acct-1", Provider: "nondep", EncryptedCredential: "enc"})
	q.addBinding(store.Binding{ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "nondep", ExternalID: "ext-1"})
	q.addSlot(store.Slot{ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "site", ConfigJson: `{}`})
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewDeployService(q, reg, &stubCredStore{}, eng)

	_, err := svc.TriggerDeploy(context.Background(), "bnd-1")
	if err == nil {
		t.Fatal("expected error for unsupported deploy capability")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("expected provider.Error, got %T", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Fatalf("expected KindUnsupported, got %s", pErr.Kind)
	}
}

func TestTriggerDeployProviderErrScrubbed(t *testing.T) {
	svc, dp, _, bindingID := deploySetUp(t)
	leakedToken := "ghp_[A-Za-z0-9]{36}"
	dp.deployTriggerFn = func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, slot *domain.Slot) (*domain.DeployEvent, error) {
		return nil, &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "upstream error: " + leakedToken}
	}

	_, err := svc.TriggerDeploy(context.Background(), bindingID)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), leakedToken) {
		t.Errorf("leaked token %q found in error message: %s", leakedToken, err.Error())
	}
}

func TestListDeploymentsSuccess(t *testing.T) {
	svc, dp, _, bindingID := deploySetUp(t)
	dp.deployListFn = func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error) {
		return []domain.DeployEvent{{ID: "evt-1", Status: "ready", CreatedAt: "now"}}, nil
	}

	events, err := svc.ListDeployments(context.Background(), bindingID)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestListDeploymentsBindingNotFound(t *testing.T) {
	svc, _, _, _ := deploySetUp(t)
	_, err := svc.ListDeployments(context.Background(), "nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestListDeploymentsUnsupported(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	reg.Register(&providerAdapter{
		typeFn: func() string { return "nondep2" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			return &domain.ExternalResource{ExternalID: externalID, Meta: map[string]any{}}, nil
		},
	})
	q.addConnection(store.ProviderConnection{ID: "acct-1", Provider: "nondep2", EncryptedCredential: "enc"})
	q.addBinding(store.Binding{ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "nondep2", ExternalID: "ext-1"})
	q.addSlot(store.Slot{ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "site", ConfigJson: `{}`})
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewDeployService(q, reg, &stubCredStore{}, eng)

	_, err := svc.ListDeployments(context.Background(), "bnd-1")
	if err == nil {
		t.Fatal("expected error for unsupported list capability")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("expected provider.Error, got %T", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Fatalf("expected KindUnsupported, got %s", pErr.Kind)
	}
}

func TestGetLogsSuccess(t *testing.T) {
	svc, dp, _, bindingID := deploySetUp(t)
	dp.logFetchFn = func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding, deployID string, tail int) (domain.LogChunk, error) {
		return domain.LogChunk{Lines: "build log output", Truncated: false}, nil
	}

	chunk, err := svc.GetLogs(context.Background(), bindingID, "deploy-1", 100)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if chunk.Lines != "build log output" {
		t.Fatalf("expected 'build log output', got %q", chunk.Lines)
	}
}

func TestGetLogsUnsupported(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	reg.Register(&providerAdapter{
		typeFn: func() string { return "nondep3" },
		getResourceFn: func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, externalID string) (*domain.ExternalResource, error) {
			return &domain.ExternalResource{ExternalID: externalID, Meta: map[string]any{}}, nil
		},
	})
	q.addConnection(store.ProviderConnection{ID: "acct-1", Provider: "nondep3", EncryptedCredential: "enc"})
	q.addBinding(store.Binding{ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "nondep3", ExternalID: "ext-1"})
	q.addSlot(store.Slot{ID: "slot-1", ProjectID: "proj-1", Role: "static-site", Name: "site", ConfigJson: `{}`})
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewDeployService(q, reg, &stubCredStore{}, eng)

	_, err := svc.GetLogs(context.Background(), "bnd-1", "deploy-1", 100)
	if err == nil {
		t.Fatal("expected error for unsupported log fetching")
	}
	var pErr *provider.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("expected provider.Error, got %T", err)
	}
	if pErr.Kind != provider.KindUnsupported {
		t.Fatalf("expected KindUnsupported, got %s", pErr.Kind)
	}
}

func TestGetLogsBindingNotFound(t *testing.T) {
	svc, _, _, _ := deploySetUp(t)
	_, err := svc.GetLogs(context.Background(), "nonexistent", "deploy-1", 100)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestListDeploymentsFromCache(t *testing.T) {
	svc, dp, _, bindingID := deploySetUp(t)
	callCount := 0
	dp.deployListFn = func(ctx context.Context, conn *domain.ProviderConnection, credential []byte, binding *domain.Binding) ([]domain.DeployEvent, error) {
		callCount++
		return []domain.DeployEvent{{ID: "evt-1", Status: "ready"}}, nil
	}

	events1, err := svc.ListDeployments(context.Background(), bindingID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	events2, err := svc.ListDeployments(context.Background(), bindingID)
	if err != nil {
		t.Fatalf("second call (cached): %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected 1 upstream call (cached), got %d", callCount)
	}
	if len(events1) != len(events2) {
		t.Fatalf("expected same number of events from cache, got %d vs %d", len(events1), len(events2))
	}
}

func TestScrubProviderErr(t *testing.T) {
	tests := []struct {
		name    string
		input   error
		checkFn func(error) bool
	}{
		{
			name:  "provider error scrubbed",
			input: &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "ghp_[A-Za-z0-9]{36} in response"},
			checkFn: func(err error) bool {
				return !strings.Contains(err.Error(), "ghp_[A-Za-z0-9]{36}") && strings.Contains(err.Error(), "ghp_***")
			},
		},
		{
			name:  "bearer token scrubbed",
			input: &provider.Error{Kind: provider.KindUpstream, ProviderMsg: "Bearer [A-Za-z0-9\\-_.]+ invalid"},
			checkFn: func(err error) bool {
				return !strings.Contains(err.Error(), "Bearer [A-Za-z0-9\\-_.]+") && strings.Contains(err.Error(), "Bearer ***")
			},
		},
		{
			name:  "plain error wrapped",
			input: errors.New("raw upstream failure: ghp_[A-Za-z0-9]{36}"),
			checkFn: func(err error) bool {
				return !strings.Contains(err.Error(), "ghp_[A-Za-z0-9]{36}") && strings.Contains(err.Error(), "ghp_***")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubProviderErr(tt.input)
			if !tt.checkFn(result) {
				t.Errorf("scrubProviderErr(%v) = %v, didn't pass check", tt.input, result)
			}
		})
	}
}