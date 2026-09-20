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
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM provider_connection")
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

	if _, err := tx.ExecContext(ctx, "DELETE FROM operation_request"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM resource_relation"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_resource"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM resource_instance"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM provider_connection"); err != nil {
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
		id, provider, label, scopeType, scopeID, scopeLabel, encryptedCredential string
	}{
		{"fixture-gh", "github", "Fixture GitHub", "organization", "mevius", "Mevius", "env:v1:fixture:gh:token"},
		{"fixture-cf", "cloudflare", "Fixture Cloudflare", "account", "cf-account", "Cloudflare Demo", "env:v1:fixture:cf:token"},
		{"fixture-vc", "vercel", "Fixture Vercel", "team", "vc-team", "Vercel Demo", "env:v1:fixture:vc:token"},
	}
	for _, a := range accounts {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO provider_connection (id, provider_id, label, endpoint, scope_type, scope_id, scope_label, config_json, encrypted_credential, remote_identity_json, permissions_json, permissions_checked_at, created_at, updated_at) VALUES (?, ?, ?, '', ?, ?, ?, '{}', ?, '{}', '{}', ?, ?, ?)`,
			a.id, a.provider, a.label, a.scopeType, a.scopeID, a.scopeLabel, a.encryptedCredential, now, now, now)
		if err != nil {
			return fmt.Errorf("insert connection %s: %w", a.id, err)
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

	resources := []struct {
		id, connectionID, product, kind, externalID, displayName, lifecycle, meta, status string
	}{
		{"fixture-repo", "fixture-gh", "github.repositories", "git_repo", "mevius/demo-shop", "demo-shop", "imported", `{}`, "ok"},
		{"fixture-page", "fixture-vc", "vercel.projects", "page", "page-demo", "demo-page", "imported", `{}`, "ok"},
		{"fixture-zone", "fixture-cf", "cloudflare.dns", "dns_zone", "zone-demo", "example.com", "imported", `{}`, "never"},
	}
	for _, resource := range resources {
		_, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO resource_instance (id, connection_id, provider_product_id, resource_kind, external_id, external_url, display_name, lifecycle_mode, spec_json, provider_config_json, cached_meta_json, sync_status, capability_state_json, last_synced_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, '', ?, ?, '{}', '{"version":1}', ?, ?, '{}', ?, ?, ?)`,
			resource.id, resource.connectionID, resource.product, resource.kind, resource.externalID, resource.displayName, resource.lifecycle, resource.meta, resource.status, now, now, now)
		if err != nil {
			return fmt.Errorf("insert resource %s: %w", resource.id, err)
		}
	}

	links := []struct{ id, projectID, instanceID, alias, role, purpose string }{
		{"fixture-pr-repo", "fixture-proj-shop", "fixture-repo", "repo", "source", "source"},
		{"fixture-pr-blog-repo", "fixture-proj-blog", "fixture-repo", "repo", "source", "source"},
		{"fixture-pr-page", "fixture-proj-blog", "fixture-page", "site", "frontend", "production"},
		{"fixture-pr-zone", "fixture-proj-blog", "fixture-zone", "dns", "infrastructure", "public dns"},
	}
	for _, link := range links {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO project_resource (id, project_id, resource_instance_id, alias, role, purpose, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, link.id, link.projectID, link.instanceID, link.alias, link.role, link.purpose, now, now)
		if err != nil {
			return fmt.Errorf("insert project resource %s: %w", link.id, err)
		}
	}
	relations := []struct{ id, from, to, relationType, origin string }{
		{"fixture-rel-source", "fixture-page", "fixture-repo", "source_repo", "system"},
	}
	for _, relation := range relations {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO resource_relation (id, from_resource_instance_id, to_resource_instance_id, relation_type, origin, config_json, created_at) VALUES (?, ?, ?, ?, ?, '{}', ?)`, relation.id, relation.from, relation.to, relation.relationType, relation.origin, now)
		if err != nil {
			return fmt.Errorf("insert relation %s: %w", relation.id, err)
		}
	}

	return tx.Commit()
}
