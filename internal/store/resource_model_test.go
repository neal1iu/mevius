package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, New(db)
}
func seedConnection(t *testing.T, q *Queries) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.InsertProviderConnection(context.Background(), InsertProviderConnectionParams{ID: "conn", ProviderID: "github", Label: "GitHub", ScopeType: "organization", ScopeID: "org", ScopeLabel: "Org", AuthMethod: "token", ConfigJson: "{}", EncryptedCredential: "secret", RemoteIdentityJson: "{}", PermissionsJson: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}
func insertInstance(t *testing.T, q *Queries, id, external, kind, product string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.InsertResourceInstance(context.Background(), InsertResourceInstanceParams{ID: id, ConnectionID: "conn", ProviderProductID: product, ResourceKind: kind, ExternalID: external, DisplayName: external, LifecycleMode: "imported", SpecJson: "{}", ProviderConfigJson: "{}", CachedMetaJson: "{}", SyncStatus: "never", CapabilityStateJson: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func TestResourceUniquenessAndProjectSharing(t *testing.T) {
	_, q := openTest(t)
	seedConnection(t, q)
	insertInstance(t, q, "repo", "org/repo", "git_repo", "github.repositories")
	now := time.Now().UTC().Format(time.RFC3339)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if err := q.InsertProject(ctx, InsertProjectParams{ID: id, Name: id, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := q.InsertProjectResource(ctx, InsertProjectResourceParams{ID: "pr-" + id, ProjectID: id, ResourceInstanceID: "repo", Alias: "repo", Role: "source", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.DeleteProject(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetResourceInstance(ctx, "repo"); err != nil {
		t.Fatalf("shared instance deleted with project: %v", err)
	}
	err := q.InsertResourceInstance(ctx, InsertResourceInstanceParams{ID: "dup", ConnectionID: "conn", ProviderProductID: "github.repositories", ResourceKind: "git_repo", ExternalID: "org/repo", DisplayName: "dup", LifecycleMode: "imported", SpecJson: "{}", ProviderConfigJson: "{}", CachedMetaJson: "{}", SyncStatus: "never", CapabilityStateJson: "{}", CreatedAt: now, UpdatedAt: now})
	if err == nil {
		t.Fatal("expected remote identity uniqueness violation")
	}
	insertInstance(t, q, "custom", "external-custom", "future_product_kind", "future.product")
	if err := q.InsertProjectResource(ctx, InsertProjectResourceParams{ID: "bad-role", ProjectID: "b", ResourceInstanceID: "custom", Alias: "custom", Role: "unknown", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("expected invalid project role to be rejected")
	}
}

func TestRelationConstraintsAndRestrict(t *testing.T) {
	_, q := openTest(t)
	seedConnection(t, q)
	insertInstance(t, q, "repo", "org/repo", "git_repo", "github.repositories")
	insertInstance(t, q, "pipe", "org/repo:1", "ci_pipeline", "github.actions")
	now := time.Now().UTC().Format(time.RFC3339)
	ctx := context.Background()
	if err := q.InsertResourceRelation(ctx, InsertResourceRelationParams{ID: "rel", FromResourceInstanceID: "pipe", ToResourceInstanceID: "repo", RelationType: "source_repo", Origin: "system", ConfigJson: "{}", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertResourceRelation(ctx, InsertResourceRelationParams{ID: "bad", FromResourceInstanceID: "pipe", ToResourceInstanceID: "repo", RelationType: "deploys_to", Origin: "user", ConfigJson: "{}", CreatedAt: now}); err == nil {
		t.Fatal("expected deploys_to relation to be rejected")
	}
	if _, err := q.DeleteResourceInstance(ctx, "repo"); err == nil {
		t.Fatal("expected referenced target deletion to be restricted")
	}
	if _, err := q.DeleteResourceInstance(ctx, "pipe"); err != nil {
		t.Fatal(err)
	}
	if rows, err := q.ListRelationsTo(ctx, "repo"); err != nil || len(rows) != 0 {
		t.Fatalf("outgoing relation did not cascade: rows=%d err=%v", len(rows), err)
	}
}

func TestOpenRejectsLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE slot(id TEXT);`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err = Open(path); err == nil {
		t.Fatal("expected legacy schema rejection")
	}
}

func TestOpenRejectsUnversionedNonEmptySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unversioned.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE historical_project(id TEXT);`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err = Open(path); !errors.Is(err, ErrSchemaResetRequired) {
		t.Fatalf("got %v, want reset required", err)
	}
}

func TestOpenRejectsPreviousResourceSchemaEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "epoch4.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL); INSERT INTO schema_meta VALUES ('schema_epoch', '4');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err = Open(path); !errors.Is(err, ErrSchemaResetRequired) {
		t.Fatalf("got %v, want reset required", err)
	}
}
