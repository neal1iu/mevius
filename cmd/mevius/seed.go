package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"mevius/internal/store"
)

func seedRun(args []string) int {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	reset := fs.Bool("reset", false, "clear database before seeding")
	fs.Parse(args)

	dbPath := os.Getenv("MEVIUS_DB_PATH")
	if dbPath == "" {
		dbPath = "./mevius.db"
	}

	db, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	ctx := context.Background()

	if !*reset {
		hasData, err := hasUserData(ctx, db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed: check data: %v\n", err)
			return 1
		}
		if hasData {
			fmt.Fprintf(os.Stderr, "seed: database has existing data; use --reset to overwrite\n")
			return 1
		}
	}

	if *reset {
		if err := clearAll(ctx, db); err != nil {
			fmt.Fprintf(os.Stderr, "seed: clear: %v\n", err)
			return 1
		}
	}

	if err := insertFixtures(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "seed: insert: %v\n", err)
		return 1
	}

	fmt.Println("seed: done")
	return 0
}

func hasUserData(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM provider_account")
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	row = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM project")
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func clearAll(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM binding"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM slot"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM provider_account"); err != nil {
		return err
	}

	return tx.Commit()
}

func insertFixtures(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	accounts := []struct {
		id, provider, label, encryptedToken, metaJSON string
	}{
		{"fixture-gh", "github", "Fixture GitHub", "env:v1:fixture:gh:token", `{}`},
		{"fixture-cf", "cloudflare", "Fixture Cloudflare", "env:v1:fixture:cf:token", `{}`},
		{"fixture-vc", "vercel", "Fixture Vercel", "env:v1:fixture:vc:token", `{}`},
	}
	for _, a := range accounts {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO provider_account (id, provider, label, encrypted_token, meta_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			a.id, a.provider, a.label, a.encryptedToken, a.metaJSON, now)
		if err != nil {
			return fmt.Errorf("insert account %s: %w", a.id, err)
		}
	}

	projects := []struct {
		id, name, desc string
	}{
		{"fixture-proj-shop", "demo-shop", "Demo e-commerce shop"},
		{"fixture-proj-blog", "demo-blog", "Demo personal blog"},
	}
	for _, p := range projects {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO project (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			p.id, p.name, p.desc, now, now)
		if err != nil {
			return fmt.Errorf("insert project %s: %w", p.id, err)
		}
	}

	slots := []struct {
		id, projectID, slotType, name, configJSON string
	}{
		{"fixture-slot-repo", "fixture-proj-shop", "repo", "shop-repo", `{"repo":"mevius/demo-shop"}`},
		{"fixture-slot-compute", "fixture-proj-shop", "compute", "shop-worker", `{"runtime":"node18"}`},
		{"fixture-slot-ss", "fixture-proj-blog", "static-site", "blog-site", `{"build_command":"npm run build"}`},
		{"fixture-slot-dns", "fixture-proj-blog", "dns-domain", "blog-domain", `{"domain":"blog.example.com"}`},
	}
	for _, s := range slots {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO slot (id, project_id, type, name, config_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			s.id, s.projectID, s.slotType, s.name, s.configJSON, now)
		if err != nil {
			return fmt.Errorf("insert slot %s: %w", s.id, err)
		}
	}

	// Bindings with all 4 sync_status values
	bindings := []struct {
		id, slotID, accountID, provider, externalID, cachedMetaJSON, syncStatus string
		lastSyncedAt                                                             *string
	}{
		{"fixture-b-ok", "fixture-slot-repo", "fixture-gh", "github", "repo-ok-1", `{"name":"demo-shop","private":false}`, "ok", strPtr(now)},
		{"fixture-b-err", "fixture-slot-repo", "fixture-gh", "github", "repo-err-1", `{}`, "error", nil},
		{"fixture-b-auth", "fixture-slot-ss", "fixture-vc", "vercel", "proj-auth-1", `{}`, "auth_error", nil},
		{"fixture-b-orph", "fixture-slot-dns", "fixture-cf", "cloudflare", "zone-orph-1", `{"zone":"orphaned.example.com"}`, "orphaned", strPtr(now)},
	}
	for _, b := range bindings {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO binding (id, slot_id, account_id, provider, external_id, cached_meta_json, sync_status, last_synced_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			b.id, b.slotID, b.accountID, b.provider, b.externalID, b.cachedMetaJSON, b.syncStatus, b.lastSyncedAt, now)
		if err != nil {
			return fmt.Errorf("insert binding %s: %w", b.id, err)
		}
	}

	return tx.Commit()
}

func strPtr(s string) *string { return &s }