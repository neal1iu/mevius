package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"mevius/internal/domain"
	"mevius/internal/provider"
	"mevius/internal/store"
)

type testCredentials struct{}

func (testCredentials) Resolve(context.Context, string) ([]byte, error) { return []byte("token"), nil }

type testProvider struct{ product *testProduct }
type testProduct struct{ creates, deletes int }

func (p *testProvider) ID() domain.ProviderID { return "test" }
func (p *testProvider) Descriptor() domain.ProviderDescriptor {
	return domain.ProviderDescriptor{ID: "test", DisplayName: "Test", Products: []domain.ProductDescriptor{p.product.Descriptor()}}
}
func (p *testProvider) Products() []provider.ProductDriver {
	return []provider.ProductDriver{p.product}
}
func (p *testProvider) Probe(context.Context, string, []byte) (*domain.ProbeResult, error) {
	return &domain.ProbeResult{Identity: map[string]any{"login": "tester"}, Scopes: []domain.ProviderScope{{Type: "account", ID: "one", Label: "One"}}, Permissions: map[domain.Capability]domain.CapabilityState{domain.CapInspect: {Availability: domain.CapabilityAvailable}}}, nil
}
func (p *testProvider) ValidateScope(ctx context.Context, endpoint string, credential []byte, scope domain.ProviderScope) (*domain.ProbeResult, error) {
	result, _ := p.Probe(ctx, endpoint, credential)
	result.Scopes = []domain.ProviderScope{scope}
	return result, nil
}
func (p *testProduct) Descriptor() domain.ProductDescriptor {
	return domain.ProductDescriptor{ID: "test.repositories", ProviderID: "test", ResourceKind: domain.ResourceKindGitRepo, Capabilities: []domain.Capability{domain.CapCreate, domain.CapInspect, domain.CapDelete}}
}
func (p *testProduct) Create(_ context.Context, _ *domain.ProviderConnection, _ []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	p.creates++
	var spec domain.RepoSpec
	_ = json.Unmarshal(req.Spec, &spec)
	return &domain.ExternalResource{ExternalID: "org/" + spec.Name, DisplayName: spec.Name}, nil
}
func (p *testProduct) Delete(context.Context, *domain.ProviderConnection, []byte, *domain.ResourceInstance) error {
	p.deletes++
	return nil
}
func (p *testProduct) Inspect(_ context.Context, _ *domain.ProviderConnection, _ []byte, v *domain.ResourceInstance) (*domain.ExternalResource, error) {
	return &domain.ExternalResource{ExternalID: v.ExternalID, DisplayName: v.DisplayName}, nil
}
func (p *testProduct) RecoverCreate(_ context.Context, _ *domain.ProviderConnection, _ []byte, req domain.CreateResourceRequest) (*domain.ExternalResource, error) {
	var spec domain.RepoSpec
	_ = json.Unmarshal(req.Spec, &spec)
	return &domain.ExternalResource{ExternalID: "org/" + spec.Name, DisplayName: spec.Name}, nil
}

func setupServices(t *testing.T) (*store.Queries, *ResourceService, *LinkService, *testProduct) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	q := store.New(db)
	now := time.Now().UTC().Format(time.RFC3339)
	if err = q.InsertProviderConnection(context.Background(), store.InsertProviderConnectionParams{ID: "conn", ProviderID: "test", Label: "Test", ScopeType: "account", ScopeID: "one", ScopeLabel: "One", AuthMethod: "token", ConfigJson: "{}", EncryptedCredential: "x", RemoteIdentityJson: "{}", PermissionsJson: `{"create":{"availability":"available"},"inspect":{"availability":"available"},"delete":{"availability":"available"}}`, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	product := &testProduct{}
	reg := provider.NewRegistry()
	reg.Register(&testProvider{product: product})
	resources := NewResourceService(q, reg, testCredentials{})
	return q, resources, NewLinkService(q), product
}

func TestManagedCreateIsIdempotent(t *testing.T) {
	_, resources, _, product := setupServices(t)
	spec, _ := json.Marshal(domain.RepoSpec{Name: "repo"})
	input := CreateResourceInput{ConnectionID: "conn", ProviderProductID: "test.repositories", Spec: spec}
	first, err := resources.Create(context.Background(), input, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := resources.Create(context.Background(), input, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || product.creates != 1 {
		t.Fatalf("idempotency failed: ids %s %s calls %d", first.ID, second.ID, product.creates)
	}
	input.Spec, _ = json.Marshal(domain.RepoSpec{Name: "other"})
	if _, err = resources.Create(context.Background(), input, "same-key"); err == nil {
		t.Fatal("expected key/hash conflict")
	}
}

func TestProjectCanShareInstanceAndPipelineRequiresRepo(t *testing.T) {
	q, resources, links, _ := setupServices(t)
	spec, _ := json.Marshal(domain.RepoSpec{Name: "repo"})
	repo, err := resources.Create(context.Background(), CreateResourceInput{ConnectionID: "conn", ProviderProductID: "test.repositories", Spec: spec}, "repo-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, name := range []string{"one", "two"} {
		now := time.Now().UTC().Format(time.RFC3339)
		if err = q.InsertProject(ctx, store.InsertProjectParams{ID: name, Name: name, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if _, err = links.Attach(ctx, name, repo.ID, "repo", ""); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err = q.InsertResourceInstance(ctx, store.InsertResourceInstanceParams{ID: "pipeline", ConnectionID: "conn", ProviderProductID: "test.repositories", ResourceKind: "ci_pipeline", ExternalID: "pipe", DisplayName: "pipe", LifecycleMode: "imported", SpecJson: "{}", ProviderConfigJson: "{}", CachedMetaJson: "{}", SyncStatus: "ok", CapabilityStateJson: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err = links.CreateRelation(ctx, "pipeline", repo.ID, domain.RelationSourceRepo, domain.RelationSystem, nil); err != nil {
		t.Fatal(err)
	}
	if err = q.InsertProject(ctx, store.InsertProjectParams{ID: "empty", Name: "empty", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err = links.Attach(ctx, "empty", "pipeline", "ci", ""); err == nil {
		t.Fatal("expected source repo prerequisite conflict")
	}
	if _, err = links.Attach(ctx, "one", "pipeline", "ci", ""); err != nil {
		t.Fatal(err)
	}
}

func TestStaleCreateOperationRecoversWithoutCreatingAgain(t *testing.T) {
	q, resources, _, product := setupServices(t)
	spec, _ := json.Marshal(domain.RepoSpec{Name: "recovered"})
	input := CreateResourceInput{ConnectionID: "conn", ProviderProductID: "test.repositories", Spec: spec}
	requestHash := hash(struct {
		Operation string
		Input     CreateResourceInput
	}{"create", input})
	old := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := q.InsertOperationRequest(context.Background(), store.InsertOperationRequestParams{ID: "op", IdempotencyKey: "recover-key", OperationType: "create_resource", RequestHash: requestHash, Status: "in_progress", ResponseJson: "{}", ErrorJson: "{}", CreatedAt: old, UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	instance, err := resources.Create(context.Background(), input, "recover-key")
	if err != nil {
		t.Fatal(err)
	}
	if instance.ExternalID != "org/recovered" || product.creates != 0 {
		t.Fatalf("unexpected recovery: %#v creates=%d", instance, product.creates)
	}
}

func TestRemoteDeleteReplaySucceedsAfterLocalRowIsGone(t *testing.T) {
	_, resources, _, product := setupServices(t)
	spec, _ := json.Marshal(domain.RepoSpec{Name: "temporary"})
	instance, err := resources.Create(context.Background(), CreateResourceInput{ConnectionID: "conn", ProviderProductID: "test.repositories", Spec: spec}, "create-delete")
	if err != nil {
		t.Fatal(err)
	}
	if err = resources.DeleteRemote(context.Background(), instance.ID, "delete-key"); err != nil {
		t.Fatal(err)
	}
	if err = resources.DeleteRemote(context.Background(), instance.ID, "delete-key"); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if product.deletes != 1 {
		t.Fatalf("remote delete calls=%d", product.deletes)
	}
}
