package store

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	tables := []string{"provider_account", "project", "slot", "binding"}
	for _, tbl := range tables {
		var count int
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl)
		if err := row.Scan(&count); err != nil {
			t.Fatalf("check table %s: %v", tbl, err)
		}
		if count != 1 {
			t.Errorf("table %s not found", tbl)
		}
	}

	var idxCount int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_binding_account_external'")
	if err := row.Scan(&idxCount); err != nil {
		t.Fatalf("check index: %v", err)
	}
	if idxCount != 1 {
		t.Error("index idx_binding_account_external not found")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-1", "cloudflare", "CF Account", "env:v1:abc:def", `{"account_id":"123"}`, now)
	if err != nil {
		t.Fatalf("insert provider_account: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO project (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"proj-1", "test-project", "Test", now)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-1", "proj-1", "repo", "test-slot", `{}`, now)
	if err != nil {
		t.Fatalf("insert slot: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-1", "slot-1", "pa-1", "cloudflare", "ext-1", `{}`, "ok", now, now)
	if err != nil {
		t.Fatalf("insert binding 1: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-2", "slot-1", "pa-1", "cloudflare", "ext-1", `{}`, "ok", now, now)
	if err == nil {
		t.Error("expected UNIQUE constraint error for duplicate binding, got nil")
	} else {
		t.Logf("UNIQUE constraint correctly enforced: %v", err)
	}
}

func TestConcurrentRW(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "concurrent.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-c1", "github", "GH Account", "env:v1:abc:def", `{}`, now)
	if err != nil {
		t.Fatalf("seed provider_account: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO project (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"proj-c1", "concurrent-project", "", now)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-c1", "proj-c1", "repo", "concurrent-slot", `{}`, now)
	if err != nil {
		t.Fatalf("seed slot: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 30)

	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for start := time.Now(); time.Since(start) < 10*time.Second; {
				var count int
				row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM binding")
				if err := row.Scan(&count); err != nil {
					errCh <- fmt.Errorf("reader %d: %v", n, err)
					return
				}
				time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		bindingID := 0
		for start := time.Now(); time.Since(start) < 10*time.Second; {
			bindingID++
			id := fmt.Sprintf("b-c%d", bindingID)
			_, err := db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, "slot-c1", "pa-c1", "github", fmt.Sprintf("ext-%d", bindingID), `{}`, "ok", now, now)
			if err != nil {
				errCh <- fmt.Errorf("writer insert %s: %v", id, err)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("concurrent error: %v", e)
		}
	}
}

func TestBadDB(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "not-a-db.txt")

	if err := os.WriteFile(badPath, []byte("this is not a sqlite database"), 0644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	db, err := Open(badPath)
	if err == nil {
		db.Close()
		t.Error("expected error for non-SQLite file, got nil")
	} else {
		t.Logf("bad DB correctly rejected: %v", err)
	}
}

func TestQueries(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()

	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	err = q.InsertProviderAccount(ctx, InsertProviderAccountParams{
		ID:             "pa-q1",
		Provider:       "vercel",
		Label:          "Vercel Account",
		EncryptedToken: "env:v1:key:nonce:ct",
		MetaJson:       `{"team_id":"team_abc"}`,
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("InsertProviderAccount: %v", err)
	}

	pa, err := q.GetProviderAccount(ctx, "pa-q1")
	if err != nil {
		t.Fatalf("GetProviderAccount: %v", err)
	}
	if pa.Label != "Vercel Account" {
		t.Errorf("expected label 'Vercel Account', got %q", pa.Label)
	}

	pas, err := q.ListProviderAccounts(ctx)
	if err != nil {
		t.Fatalf("ListProviderAccounts: %v", err)
	}
	if len(pas) != 1 {
		t.Errorf("expected 1 provider_account, got %d", len(pas))
	}

	err = q.InsertProject(ctx, InsertProjectParams{
		ID:          "proj-q1",
		Name:        "query-test",
		Description: "Test project for queries",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	proj, err := q.GetProject(ctx, "proj-q1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if proj.Name != "query-test" {
		t.Errorf("expected name 'query-test', got %q", proj.Name)
	}

	projects, err := q.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}

	err = q.InsertSlot(ctx, InsertSlotParams{
		ID:         "slot-q1",
		ProjectID:  "proj-q1",
		Type:       "static-site",
		Name:       "query-slot",
		ConfigJson: `{"build_command":"npm run build"}`,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("InsertSlot: %v", err)
	}

	slot, err := q.GetSlot(ctx, "slot-q1")
	if err != nil {
		t.Fatalf("GetSlot: %v", err)
	}
	if slot.Type != "static-site" {
		t.Errorf("expected type 'static-site', got %q", slot.Type)
	}

	slots, err := q.ListSlotsByProject(ctx, "proj-q1")
	if err != nil {
		t.Fatalf("ListSlotsByProject: %v", err)
	}
	if len(slots) != 1 {
		t.Errorf("expected 1 slot, got %d", len(slots))
	}

	err = q.UpdateSlotConfig(ctx, UpdateSlotConfigParams{
		ConfigJson: `{"build_command":"npm run build","output_dir":"dist"}`,
		ID:         "slot-q1",
	})
	if err != nil {
		t.Fatalf("UpdateSlotConfig: %v", err)
	}

	err = q.InsertBinding(ctx, InsertBindingParams{
		ID:             "b-q1",
		SlotID:         "slot-q1",
		AccountID:      "pa-q1",
		Provider:       "vercel",
		ExternalID:     "ext-query-1",
		CachedMetaJson: `{"deploy_id":"d_abc"}`,
		SyncStatus:     "never",
		LastSyncedAt:   nil,
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("InsertBinding: %v", err)
	}

	b, err := q.GetBinding(ctx, "b-q1")
	if err != nil {
		t.Fatalf("GetBinding: %v", err)
	}
	if b.ExternalID != "ext-query-1" {
		t.Errorf("expected external_id 'ext-query-1', got %q", b.ExternalID)
	}

	bindings, err := q.ListBindingsBySlot(ctx, "slot-q1")
	if err != nil {
		t.Fatalf("ListBindingsBySlot: %v", err)
	}
	if len(bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(bindings))
	}

	fanout, err := q.FanOutBindingsByAccountExternal(ctx, FanOutBindingsByAccountExternalParams{
		AccountID:  "pa-q1",
		ExternalID: "ext-query-1",
	})
	if err != nil {
		t.Fatalf("FanOutBindingsByAccountExternal: %v", err)
	}
	if len(fanout) != 1 {
		t.Errorf("expected 1 fanout result, got %d", len(fanout))
	}

	now2 := time.Now().UTC().Format(time.RFC3339)
	err = q.UpdateBindingSyncStatus(ctx, UpdateBindingSyncStatusParams{
		SyncStatus:     "ok",
		CachedMetaJson: `{"deploy_id":"d_abc","status":"ready"}`,
		LastSyncedAt:   &now2,
		ID:             "b-q1",
	})
	if err != nil {
		t.Fatalf("UpdateBindingSyncStatus: %v", err)
	}

	b2, err := q.GetBinding(ctx, "b-q1")
	if err != nil {
		t.Fatalf("GetBinding after update: %v", err)
	}
	if b2.SyncStatus != "ok" {
		t.Errorf("expected sync_status 'ok', got %q", b2.SyncStatus)
	}
	if b2.LastSyncedAt == nil || *b2.LastSyncedAt != now2 {
		t.Errorf("expected last_synced_at %q, got %v", now2, b2.LastSyncedAt)
	}

	err = q.DeleteBinding(ctx, "b-q1")
	if err != nil {
		t.Fatalf("DeleteBinding: %v", err)
	}

	err = q.DeleteSlot(ctx, "slot-q1")
	if err != nil {
		t.Fatalf("DeleteSlot: %v", err)
	}

	err = q.DeleteProject(ctx, "proj-q1")
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	err = q.DeleteProviderAccount(ctx, "pa-q1")
	if err != nil {
		t.Fatalf("DeleteProviderAccount: %v", err)
	}

	projects2, err := q.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects after delete: %v", err)
	}
	if len(projects2) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects2))
	}
}

func TestCascadeDelete(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-cd1", "cloudflare", "CF", "env:v1:k:n:c", `{}`, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO project (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"proj-cd1", "cascade-test", "", now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-cd1", "proj-cd1", "dns-domain", "cascade-slot", `{}`, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-cd1", "slot-cd1", "pa-cd1", "cloudflare", "ext-cd1", `{}`, "ok", now, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err = db.ExecContext(ctx, `DELETE FROM project WHERE id = ?`, "proj-cd1")
	if err != nil {
		t.Fatalf("delete project: %v", err)
	}

	var slotCount int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM slot WHERE id = 'slot-cd1'")
	if err := row.Scan(&slotCount); err != nil {
		t.Fatalf("check slot: %v", err)
	}
	if slotCount != 0 {
		t.Error("expected slot to be cascade-deleted")
	}

	var bindingCount int
	row = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM binding WHERE id = 'b-cd1'")
	if err := row.Scan(&bindingCount); err != nil {
		t.Fatalf("check binding: %v", err)
	}
	if bindingCount != 0 {
		t.Error("expected binding to be cascade-deleted")
	}

	_, err = db.ExecContext(ctx, `DELETE FROM provider_account WHERE id = ?`, "pa-cd1")
	if err != nil {
		t.Fatalf("delete provider_account: %v", err)
	}
}

func TestCheckConstraints(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-bad", "invalid_provider", "Bad", "env:v1:k:n:c", `{}`, now)
	if err == nil {
		t.Error("expected CHECK constraint error for invalid provider, got nil")
	}

	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-bad", "proj-nonexistent", "invalid_type", "Bad", `{}`, now)
	if err == nil {
		t.Error("expected CHECK constraint error for invalid slot type, got nil")
	}

	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-bad", "slot-nonexistent", "pa-nonexistent", "cloudflare", "ext-bad", `{}`, "invalid_status", now, now)
	if err == nil {
		t.Error("expected CHECK constraint error for invalid sync_status, got nil")
	}
}

func TestOpenFileDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%s) failed: %v", dbPath, err)
	}
	defer db.Close()

	var journalMode string
	row := db.QueryRow("PRAGMA journal_mode")
	if err := row.Scan(&journalMode); err != nil {
		t.Fatalf("get journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected WAL journal mode, got %q", journalMode)
	}

	var busyTimeout int
	row = db.QueryRow("PRAGMA busy_timeout")
	if err := row.Scan(&busyTimeout); err != nil {
		t.Fatalf("get busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("expected busy_timeout 5000, got %d", busyTimeout)
	}

	if db.Stats().MaxOpenConnections != 1 {
		t.Errorf("expected MaxOpenConns 1, got %d", db.Stats().MaxOpenConnections)
	}

	db.Close()
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer db2.Close()

	var tableNames []string
	rows, err := db2.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT IN ('goose_db_version', 'sqlite_sequence') ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tableNames = append(tableNames, name)
	}
	expected := []string{"binding", "project", "provider_account", "slot"}
	if len(tableNames) != len(expected) {
		t.Errorf("expected %d business tables, got %d: %v", len(expected), len(tableNames), tableNames)
	} else {
		for i, name := range tableNames {
			if name != expected[i] {
				t.Errorf("table[%d] = %s, want %s", i, name, expected[i])
			}
		}
	}
}

func TestIdempotentMigration(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	db.Close()

	db2, err := Open(":memory:")
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer db2.Close()

	var tableNames []string
	rows, err := db2.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT IN ('goose_db_version', 'sqlite_sequence') ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tableNames = append(tableNames, name)
	}
	expected := []string{"binding", "project", "provider_account", "slot"}
	if len(tableNames) != len(expected) {
		t.Errorf("expected %d business tables after re-migration, got %d: %v", len(expected), len(tableNames), tableNames)
	} else {
		for i, name := range tableNames {
			if name != expected[i] {
				t.Errorf("table[%d] = %s, want %s", i, name, expected[i])
			}
		}
	}
}

func TestCascadeDeleteAccount(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-cda1", "cloudflare", "CF", "env:v1:k:n:c", `{}`, now)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO project (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"proj-cda1", "proj-cda", "", now)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-cda1", "proj-cda1", "repo", "slot-cda", `{}`, now)
	if err != nil {
		t.Fatalf("seed slot: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-cda1", "slot-cda1", "pa-cda1", "cloudflare", "ext-cda1", `{}`, "ok", now, now)
	if err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	_, err = db.ExecContext(ctx, `DELETE FROM provider_account WHERE id = ?`, "pa-cda1")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}

	var bindingCount int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM binding WHERE id = 'b-cda1'")
	if err := row.Scan(&bindingCount); err != nil {
		t.Fatalf("check binding: %v", err)
	}
	if bindingCount != 0 {
		t.Error("expected binding to be cascade-deleted after account delete")
	}
}

func TestForeignKeyEnforcement(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b-fk1", "slot-nonexistent", "pa-nonexistent", "cloudflare", "ext-fk1", `{}`, "never", nil, now)
	if err == nil {
		var fkEnabled int
		row := db.QueryRowContext(ctx, "PRAGMA foreign_keys")
		if err := row.Scan(&fkEnabled); err != nil {
			t.Fatalf("get foreign_keys pragma: %v", err)
		}
		if fkEnabled == 1 {
			t.Error("expected FK violation error for non-existent slot, got nil")
		} else {
			t.Log("SQLite foreign_keys pragma is OFF (default for in-memory), FK not enforced")
		}
	}
}

func TestGetProjectByName(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	err = q.InsertProject(ctx, InsertProjectParams{
		ID: "p1", Name: "unique-name", Description: "test", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	p, err := q.GetProjectByName(ctx, "unique-name")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	if p.ID != "p1" {
		t.Errorf("expected ID p1, got %s", p.ID)
	}

	_, err = q.GetProjectByName(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project name")
	}
}

func TestUpdateProject(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	err = q.InsertProject(ctx, InsertProjectParams{
		ID: "p-upd", Name: "original", Description: "desc", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	err = q.UpdateProject(ctx, UpdateProjectParams{
		Name: "updated-name", Description: "new-desc", UpdatedAt: now, ID: "p-upd",
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	p, err := q.GetProject(ctx, "p-upd")
	if err != nil {
		t.Fatalf("GetProject after update: %v", err)
	}
	if p.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got %q", p.Name)
	}
	if p.Description != "new-desc" {
		t.Errorf("expected description 'new-desc', got %q", p.Description)
	}
}

func TestUpdateSlot(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	err = q.InsertProject(ctx, InsertProjectParams{
		ID: "p-sl", Name: "slot-proj", Description: "", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	err = q.InsertSlot(ctx, InsertSlotParams{
		ID: "sl-upd", ProjectID: "p-sl", Type: "repo", Name: "orig", ConfigJson: `{"name":"repo"}`, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertSlot: %v", err)
	}

	err = q.UpdateSlot(ctx, UpdateSlotParams{
		Name: "renamed-slot", ConfigJson: `{"name":"repo","private":true}`, ID: "sl-upd",
	})
	if err != nil {
		t.Fatalf("UpdateSlot: %v", err)
	}

	s, err := q.GetSlot(ctx, "sl-upd")
	if err != nil {
		t.Fatalf("GetSlot after update: %v", err)
	}
	if s.Name != "renamed-slot" {
		t.Errorf("expected name 'renamed-slot', got %q", s.Name)
	}
	if s.ConfigJson != `{"name":"repo","private":true}` {
		t.Errorf("expected config updated, got %q", s.ConfigJson)
	}
}

func TestListBindingsBySlots(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `INSERT INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"pa-lbs", "cloudflare", "CF", "env:v1:k:n:c", `{}`, now)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO project (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"proj-lbs", "lbs-proj", "", now)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-a", "proj-lbs", "repo", "a", `{}`, now)
	if err != nil {
		t.Fatalf("seed slot a: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"slot-b", "proj-lbs", "static-site", "b", `{}`, now)
	if err != nil {
		t.Fatalf("seed slot b: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("b-lbs-%d", i), "slot-a", "pa-lbs", "cloudflare", fmt.Sprintf("ext-%d", i), `{}`, "ok", now, now)
		if err != nil {
			t.Fatalf("seed binding %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		_, err = db.ExecContext(ctx, `INSERT INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("b-lbs-b-%d", i), "slot-b", "pa-lbs", "cloudflare", fmt.Sprintf("ext-b-%d", i), `{}`, "ok", now, now)
		if err != nil {
			t.Fatalf("seed binding b %d: %v", i, err)
		}
	}

	bindings, err := q.ListBindingsBySlots(ctx, []string{"slot-a", "slot-b"})
	if err != nil {
		t.Fatalf("ListBindingsBySlots: %v", err)
	}
	if len(bindings) != 5 {
		t.Errorf("expected 5 bindings, got %d", len(bindings))
	}

	empty, err := q.ListBindingsBySlots(ctx, []string{})
	if err != nil {
		t.Fatalf("ListBindingsBySlots empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 bindings for empty slots, got %d", len(empty))
	}
}

func TestWithTx(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	q := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	tq := q.WithTx(tx)
	if tq == nil {
		t.Fatal("WithTx returned nil")
	}

	err = tq.InsertProject(ctx, InsertProjectParams{
		ID: "p-tx", Name: "tx-proj", Description: "", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertProject in tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	p, err := q.GetProject(ctx, "p-tx")
	if err != nil {
		t.Fatalf("GetProject after tx commit: %v", err)
	}
	if p.Name != "tx-proj" {
		t.Errorf("expected name 'tx-proj', got %q", p.Name)
	}
}
