package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"mevius/internal/store"
	"os"
	"time"
)

func databasePath() string {
	if p := os.Getenv("MEVIUS_DB_PATH"); p != "" {
		return p
	}
	return "./mevius.db"
}

// Seed creates an offline directory snapshot, never usable remote credentials.
func seedRun(args []string) int {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	if fs.Parse(args) != nil {
		return 1
	}
	db, e := store.Open(databasePath())
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	defer db.Close()
	ctx := context.Background()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM project)+(SELECT COUNT(*) FROM provider_instance)`).Scan(&n); e != nil || n != 0 {
		fmt.Fprintln(os.Stderr, "seed requires an empty epoch 6 database; use the explicit reset command first")
		return 1
	}
	now := time.Now().UnixMilli()
	instance, scope, resource, project := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO provider_instance VALUES(?,?,?,?,?,?)`, []any{instance, "github", "public", 1, `{"url":""}`, now}},
		{`INSERT INTO provider_scope(id,provider_instance_id,scope_type,scope_key,locator_version,locator_json,label) VALUES(?,?,?,?,?,?,?)`, []any{scope, instance, "org", `["org","demo"]`, 1, `{"id":"demo"}`, "Demo (offline)"}},
		{`INSERT INTO resource_instance VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, []any{resource, instance, "github.repositories", `["demo-offline-repository"]`, 1, scope, 1, `{"external_id":"demo/application"}`, "Demo repository (offline)", "external", true, now, now}},
		{`INSERT INTO project VALUES(?,?,?,?,?)`, []any{project, "Example application", "Offline snapshot. Bind a real authorization to import remote resources.", now, now}},
		{`INSERT INTO project_resource VALUES(?,?,?,?,?,?,?,?,?)`, []any{uuid.NewString(), project, resource, "source", "source", "Application code", "development", now, now}},
	}
	for _, q := range statements {
		if _, e = tx.ExecContext(ctx, q.sql, q.args...); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
	}
	if e = tx.Commit(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	fmt.Println("seed: created offline directory snapshot")
	return 0
}
