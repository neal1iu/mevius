package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

func TestValidateCapabilityMatrix(t *testing.T) {
	tests := []struct {
		kind     domain.ResourceKind
		provider domain.ProviderType
		wantErr  bool
	}{
		{kind: domain.ResourceKindRepo, provider: domain.ProviderTypeGitHub, wantErr: false},
		{kind: domain.ResourceKindRepo, provider: domain.ProviderTypeCloudflare, wantErr: true},
		{kind: domain.ResourceKindRepo, provider: domain.ProviderTypeVercel, wantErr: true},
		{kind: domain.ResourceKindCompute, provider: domain.ProviderTypeCloudflare, wantErr: false},
		{kind: domain.ResourceKindCompute, provider: domain.ProviderTypeGitHub, wantErr: true},
		{kind: domain.ResourceKindCompute, provider: domain.ProviderTypeVercel, wantErr: true},
		{kind: domain.ResourceKindStaticSite, provider: domain.ProviderTypeCloudflare, wantErr: false},
		{kind: domain.ResourceKindStaticSite, provider: domain.ProviderTypeVercel, wantErr: false},
		{kind: domain.ResourceKindStaticSite, provider: domain.ProviderTypeGitHub, wantErr: true},
		{kind: domain.ResourceKindDNSDomain, provider: domain.ProviderTypeCloudflare, wantErr: false},
		{kind: domain.ResourceKindDNSDomain, provider: domain.ProviderTypeVercel, wantErr: false},
		{kind: domain.ResourceKindDNSDomain, provider: domain.ProviderTypeGitHub, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+"_"+string(tt.provider), func(t *testing.T) {
			err := ValidateCapability(tt.kind, tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCapability(%s, %s) error = %v, wantErr = %v", tt.kind, tt.provider, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrUnsupportedCombo) {
				t.Errorf("expected ErrUnsupportedCombo, got %v", err)
			}
		})
	}
}

func TestBindSlotNotFound(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	_, err := svc.Bind(context.Background(), "nonexistent-slot", "acct-1", "ext-1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for nonexistent slot, got %v", err)
	}
}

func TestBindAccountNotFound(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	q.addBinding(store.Binding{ID: "dummy"})
	q.addSlot(store.Slot{ID: "slot-1", Type: "repo"})

	_, err := svc.Bind(context.Background(), "slot-1", "nonexistent-account", "ext-1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for nonexistent account, got %v", err)
	}
}

func TestBindUnsupportedCombo(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	q.addSlot(store.Slot{ID: "slot-1", Type: "repo"})
	q.addAccount(store.ProviderAccount{ID: "acct-1", Provider: "cloudflare"})

	_, err := svc.Bind(context.Background(), "slot-1", "acct-1", "ext-1")
	if !errors.Is(err, ErrUnsupportedCombo) {
		t.Fatalf("expected ErrUnsupportedCombo for repo+cloudflare, got %v", err)
	}
}

func TestBindDuplicate(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	q.addSlot(store.Slot{ID: "slot-1", Type: "static-site"})
	q.addAccount(store.ProviderAccount{ID: "acct-1", Provider: "cloudflare"})

	_, err := svc.Bind(context.Background(), "slot-1", "acct-1", "ext-1")
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}

	q.setInsertBindingErr(errors.New("UNIQUE constraint failed"))
	_, err = svc.Bind(context.Background(), "slot-1", "acct-1", "ext-1")
	if !errors.Is(err, ErrDuplicateBinding) {
		t.Fatalf("expected ErrDuplicateBinding, got %v", err)
	}
}

func TestUnbind(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	err := svc.Unbind(context.Background(), "bnd-1")
	if err != nil {
		t.Fatalf("Unbind: %v", err)
	}
}

func TestRefreshBinding(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	cp := &countingProvider{result: &domain.ExternalResource{ExternalID: "ext-1", Meta: map[string]any{"key": "val"}}}
	reg.Register(cp)

	q.addAccount(store.ProviderAccount{ID: "acct-1", Provider: "fake", EncryptedToken: "enc"})
	q.addBinding(store.Binding{ID: "bnd-1", SlotID: "slot-1", AccountID: "acct-1", Provider: "fake", ExternalID: "ext-1"})

	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	b, err := svc.Refresh(context.Background(), "bnd-1")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if b.SyncStatus != domain.SyncStatusOK {
		t.Fatalf("expected sync_status=ok, got %s", b.SyncStatus)
	}
}

func TestListBindingsBySlotEmpty(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg)
	svc := NewBindingService(q, reg, eng)

	bindings, err := svc.ListBySlot(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ListBySlot: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings, got %d", len(bindings))
	}
}