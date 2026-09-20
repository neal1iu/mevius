package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
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
		if err := q.InsertProjectResource(ctx, InsertProjectResourceParams{ID: "pr-" + id, ProjectID: id, ResourceInstanceID: "repo", Alias: "repo", CreatedAt: now, UpdatedAt: now}); err != nil {
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

func TestOAuthMigrationRoundTripAndTokenDefault(t *testing.T) {
	db, _ := openTest(t)
	if err := goose.Down(db, "migrations"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Down(db, "migrations"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO provider_connection (
		id, provider_id, label, endpoint, scope_type, scope_id, scope_label,
		config_json, encrypted_credential, remote_identity_json, permissions_json,
		created_at, updated_at
	) VALUES (?, ?, ?, '', ?, ?, ?, '{}', ?, '{}', '{}', ?, ?)`,
		"legacy-token", "github", "Legacy token", "organization", "org", "Org", "encrypted", now, now)
	if err != nil {
		t.Fatalf("epoch 2 insert: %v", err)
	}
	if err = goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	row, err := New(db).GetProviderConnection(context.Background(), "legacy-token")
	if err != nil {
		t.Fatal(err)
	}
	if row.AuthMethod != "token" {
		t.Fatalf("auth_method=%q, want token", row.AuthMethod)
	}
	var epoch string
	if err = db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'schema_epoch'`).Scan(&epoch); err != nil || epoch != "4" {
		t.Fatalf("epoch=%q err=%v", epoch, err)
	}
}
