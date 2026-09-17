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

func TestBindSlotNotFound(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

	_, err := svc.Bind(context.Background(), "nonexistent-slot", "acct-1", "product", "ext-1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for nonexistent slot, got %v", err)
	}
}

func TestBindAccountNotFound(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

	q.addBinding(store.Binding{ID: "dummy"})
	q.addSlot(store.Slot{ID: "slot-1", Role: "repo"})

	_, err := svc.Bind(context.Background(), "slot-1", "nonexistent-connection", "product", "ext-1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for nonexistent connection, got %v", err)
	}
}

func TestBindDuplicate(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	reg.Register(&providerAdapter{
		typeFn: func() string { return "cloudflare" },
		descriptorFn: func() domain.ProviderDescriptor {
			return provider.CloudflareDescriptor
		},
	})
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

	q.addSlot(store.Slot{ID: "slot-1", Role: "frontend"})
	q.addConnection(store.ProviderConnection{ID: "acct-1", Provider: "cloudflare"})

	_, err := svc.Bind(context.Background(), "slot-1", "acct-1", "cloudflare.pages", "ext-1")
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}

	q.setInsertBindingErr(errors.New("UNIQUE constraint failed"))
	_, err = svc.Bind(context.Background(), "slot-1", "acct-1", "cloudflare.pages", "ext-1")
	if !errors.Is(err, ErrDuplicateBinding) {
		t.Fatalf("expected ErrDuplicateBinding, got %v", err)
	}
}

func TestUnbind(t *testing.T) {
	q := newFakeQuerier()
	reg := provider.NewRegistry()
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

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

	q.addConnection(store.ProviderConnection{ID: "acct-1", Provider: "fake", EncryptedCredential: "enc"})
	q.addBinding(store.Binding{ID: "bnd-1", SlotID: "slot-1", ConnectionID: "acct-1", Product: "fake", ExternalID: "ext-1"})

	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

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
	eng := NewRefreshEngine(q, reg, &stubCredStore{})
	svc := NewBindingService(q, reg, &stubCredStore{}, eng)

	bindings, err := svc.ListBySlot(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ListBySlot: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("expected 0 bindings, got %d", len(bindings))
	}
}