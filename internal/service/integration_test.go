package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

func realDB(t *testing.T) *store.Queries {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.New(db)
}

func seedProject(t *testing.T, q *store.Queries) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	err := q.InsertProject(ctx, store.InsertProjectParams{
		ID: "proj-seed", Name: "seed-proj", Description: "", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return "proj-seed"
}

func TestProjectServiceCreate(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	p, err := svc.Create(ctx, "my-project", "A test project")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Name != "my-project" {
		t.Errorf("expected name 'my-project', got %q", p.Name)
	}
	if p.Description != "A test project" {
		t.Errorf("expected description, got %q", p.Description)
	}
	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestProjectServiceDuplicateName(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	_, err := svc.Create(ctx, "dup-name", "")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = svc.Create(ctx, "dup-name", "")
	if err != ErrDuplicateName {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestProjectServiceList(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	projects, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}

	svc.Create(ctx, "p1", "")
	svc.Create(ctx, "p2", "")

	projects, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestProjectServiceGet(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}

	p, err := svc.Create(ctx, "get-me", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "get-me" {
		t.Errorf("expected name 'get-me', got %q", got.Name)
	}
}

func TestProjectServiceGetDetail(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := svc.GetDetail(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent detail")
	}

	p, err := svc.Create(ctx, "detail-proj", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	q.InsertSlot(ctx, store.InsertSlotParams{
		ID: "slot-d1", ProjectID: p.ID, Type: "repo", Name: "repo1", ConfigJson: `{"name":"repo1"}`, CreatedAt: now,
	})
	q.InsertSlot(ctx, store.InsertSlotParams{
		ID: "slot-d2", ProjectID: p.ID, Type: "static-site", Name: "site1", ConfigJson: `{"name":"site1"}`, CreatedAt: now,
	})

	q.InsertProviderAccount(ctx, store.InsertProviderAccountParams{
		ID: "pa-det", Provider: "cloudflare", Label: "CF", EncryptedToken: "enc", MetaJson: `{}`, CreatedAt: now,
	})

	q.InsertBinding(ctx, store.InsertBindingParams{
		ID: "b-det", SlotID: "slot-d1", AccountID: "pa-det", Provider: "cloudflare",
		ExternalID: "ext-det", CachedMetaJson: `{}`, SyncStatus: "ok", LastSyncedAt: nil, CreatedAt: now,
	})

	detail, err := svc.GetDetail(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if len(detail.Slots) != 2 {
		t.Errorf("expected 2 slots, got %d", len(detail.Slots))
	}
	if len(detail.Bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(detail.Bindings))
	}
}

func TestProjectServiceUpdate(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	p, err := svc.Create(ctx, "update-me", "old desc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(ctx, p.ID, "updated-name", "new desc")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got %q", updated.Name)
	}
	if updated.Description != "new desc" {
		t.Errorf("expected desc 'new desc', got %q", updated.Description)
	}
}

func TestProjectServiceDelete(t *testing.T) {
	q := realDB(t)
	svc := NewProjectService(q)
	ctx := context.Background()

	p, err := svc.Create(ctx, "delete-me", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.Delete(ctx, p.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = svc.Get(ctx, p.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSlotServiceCreate(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	p, _ := projSvc.Create(ctx, "slot-proj", "")

	slot, err := slotSvc.Create(ctx, p.ID, string(domain.ResourceKindRepo), "my-repo", json.RawMessage(`{"name":"my-repo"}`))
	if err != nil {
		t.Fatalf("Create slot: %v", err)
	}
	if slot.Name != "my-repo" {
		t.Errorf("expected name 'my-repo', got %q", slot.Name)
	}
	if slot.Kind != domain.ResourceKindRepo {
		t.Errorf("expected kind repo, got %s", slot.Kind)
	}
}

func TestSlotServiceInvalidConfig(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	p, _ := projSvc.Create(ctx, "invalid-slot-proj", "")

	_, err := slotSvc.Create(ctx, p.ID, string(domain.ResourceKindRepo), "bad-repo", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestSlotServiceListByProject(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	p, _ := projSvc.Create(ctx, "list-slot-proj", "")

	slots, err := slotSvc.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(slots) != 0 {
		t.Errorf("expected 0 slots, got %d", len(slots))
	}

	slotSvc.Create(ctx, p.ID, string(domain.ResourceKindDNSDomain), "dns1", json.RawMessage(`{}`))
	slotSvc.Create(ctx, p.ID, string(domain.ResourceKindDNSDomain), "dns2", json.RawMessage(`{}`))

	slots, err = slotSvc.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(slots) != 2 {
		t.Errorf("expected 2 slots, got %d", len(slots))
	}
}

func TestSlotServiceGet(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	_, err := slotSvc.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slot")
	}

	p, _ := projSvc.Create(ctx, "get-slot-proj", "")
	s, _ := slotSvc.Create(ctx, p.ID, string(domain.ResourceKindCompute), "compute1", json.RawMessage(`{"name":"compute1"}`))

	got, err := slotSvc.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "compute1" {
		t.Errorf("expected name 'compute1', got %q", got.Name)
	}
}

func TestSlotServiceUpdate(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	p, _ := projSvc.Create(ctx, "upd-slot-proj", "")
	s, _ := slotSvc.Create(ctx, p.ID, string(domain.ResourceKindStaticSite), "old", json.RawMessage(`{"name":"old"}`))

	updated, err := slotSvc.Update(ctx, s.ID, "new-name", json.RawMessage(`{"name":"new-name","framework":"react"}`))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	_ = updated

	got, err := slotSvc.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Name != "new-name" {
		t.Errorf("expected name 'new-name', got %q", got.Name)
	}
}

func TestSlotServiceDelete(t *testing.T) {
	q := realDB(t)
	projSvc := NewProjectService(q)
	slotSvc := NewSlotService(q)
	ctx := context.Background()

	err := slotSvc.Delete(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent slot")
	}

	p, _ := projSvc.Create(ctx, "del-slot-proj", "")
	s, _ := slotSvc.Create(ctx, p.ID, string(domain.ResourceKindRepo), "del-me", json.RawMessage(`{"name":"del-me"}`))

	err = slotSvc.Delete(ctx, s.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = slotSvc.Get(ctx, s.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestAccountServiceIntegration(t *testing.T) {
	q := realDB(t)
	reg := provider.NewRegistry()
	reg.Register(&stubProviderSvc{
		pType: "cloudflare",
		validateMeta: domain.AccountMeta{AccountID: "cf-int-123"},
	})
	var key [32]byte
	svc := NewAccountService(q, key, reg)
	ctx := context.Background()

	acct, err := svc.AddAccount(ctx, "cloudflare", "integration-test", "test-token-valid")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if acct.Label != "integration-test" {
		t.Errorf("expected label 'integration-test', got %q", acct.Label)
	}
	if acct.TokenEncrypted != "" {
		t.Error("expected empty TokenEncrypted in response")
	}

	accounts, err := svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("expected 1 account, got %d", len(accounts))
	}

	got, err := svc.GetAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.ID != acct.ID {
		t.Errorf("expected ID %s, got %s", acct.ID, got.ID)
	}

	err = svc.DeleteAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	accounts, err = svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts after delete: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", len(accounts))
	}
}

type stubProviderSvc struct {
	pType       string
	validateMeta domain.AccountMeta
	validateErr  error
}

func (s *stubProviderSvc) Type() string { return s.pType }

func (s *stubProviderSvc) ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	if s.validateErr != nil {
		return domain.AccountMeta{}, s.validateErr
	}
	return s.validateMeta, nil
}

func (s *stubProviderSvc) ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	return nil, nil
}

func (s *stubProviderSvc) GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error) {
	return &domain.ExternalResource{ExternalID: externalID, Meta: map[string]any{}}, nil
}

var _ provider.Provider = (*stubProviderSvc)(nil)
var _ provider.Inspector = (*stubProviderSvc)(nil)

type cfCountingProvider struct {
	cp *countingProvider
}

func (c *cfCountingProvider) Type() string { return "cloudflare" }

func (c *cfCountingProvider) ValidateCredentials(ctx context.Context, account *domain.ProviderAccount) (domain.AccountMeta, error) {
	return c.cp.ValidateCredentials(ctx, account)
}

func (c *cfCountingProvider) ListExternalResources(ctx context.Context, account *domain.ProviderAccount, kind domain.ResourceKind) ([]domain.ExternalResource, error) {
	return c.cp.ListExternalResources(ctx, account, kind)
}

func (c *cfCountingProvider) GetResource(ctx context.Context, account *domain.ProviderAccount, externalID string) (*domain.ExternalResource, error) {
	return c.cp.GetResource(ctx, account, externalID)
}

var _ provider.Provider = (*cfCountingProvider)(nil)
var _ provider.Inspector = (*cfCountingProvider)(nil)

func TestDiscoverResources(t *testing.T) {
	q := realDB(t)
	reg := provider.NewRegistry()
	reg.Register(&stubProviderSvc{
		pType: "cloudflare",
		validateMeta: domain.AccountMeta{AccountID: "cf-123"},
	})
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	q.InsertProviderAccount(ctx, store.InsertProviderAccountParams{
		ID: "pa-disc", Provider: "cloudflare", Label: "CF", EncryptedToken: "enc", MetaJson: `{}`, CreatedAt: now,
	})

	resources, err := svc.Discover(ctx, "pa-disc", domain.ResourceKindRepo)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("expected empty resources, got %d", len(resources))
	}
}

func TestFanoutRefreshWithDB(t *testing.T) {
	q := realDB(t)
	reg := provider.NewRegistry()
	cp := &countingProvider{
		result: &domain.ExternalResource{ExternalID: "shared-ext", Meta: map[string]any{"updated": true}},
	}
	reg.Register(cp)
	cp2 := &cfCountingProvider{
		cp: cp,
	}
	reg.Register(cp2)
	eng := NewRefreshEngine(q, reg)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	if err := q.InsertProviderAccount(ctx, store.InsertProviderAccountParams{
		ID: "pa-fo", Provider: "cloudflare", Label: "CF", EncryptedToken: "enc", MetaJson: `{}`, CreatedAt: now,
	}); err != nil {
		t.Fatalf("InsertProviderAccount: %v", err)
	}
	if err := q.InsertProject(ctx, store.InsertProjectParams{
		ID: "proj-fo", Name: "fo-proj", Description: "", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := q.InsertSlot(ctx, store.InsertSlotParams{
			ID: fmt.Sprintf("slot-fo-%d", i), ProjectID: "proj-fo",
			Type: "repo", Name: fmt.Sprintf("fo-slot-%d", i), ConfigJson: `{}`, CreatedAt: now,
		}); err != nil {
			t.Fatalf("InsertSlot %d: %v", i, err)
		}
		if err := q.InsertBinding(ctx, store.InsertBindingParams{
			ID: fmt.Sprintf("bnd-fo-%d", i), SlotID: fmt.Sprintf("slot-fo-%d", i),
			AccountID: "pa-fo", Provider: "fake", ExternalID: "shared-ext",
			CachedMetaJson: `{}`, SyncStatus: "never", LastSyncedAt: nil, CreatedAt: now,
		}); err != nil {
			t.Fatalf("InsertBinding %d: %v", i, err)
		}
	}

	err := eng.FanOutRefresh(ctx, "pa-fo", "shared-ext")
	if err != nil {
		t.Fatalf("FanOutRefresh: %v", err)
	}

	for i := 0; i < 2; i++ {
		b, err := q.GetBinding(ctx, fmt.Sprintf("bnd-fo-%d", i))
		if err != nil {
			t.Fatalf("GetBinding %d: %v", i, err)
		}
		if b.SyncStatus != string(domain.SyncStatusOK) {
			t.Errorf("binding %d expected ok, got %s", i, b.SyncStatus)
		}
	}
}