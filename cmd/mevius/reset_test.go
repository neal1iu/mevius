package main

import (
	"database/sql"
	"mevius/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitResetBacksUpOldEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	backup := path + ".backup"
	t.Setenv("MEVIUS_DB_PATH", path)
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`CREATE TABLE schema_meta(key TEXT PRIMARY KEY,value TEXT);INSERT INTO schema_meta VALUES('schema_epoch','5');CREATE TABLE old_resource(id TEXT);INSERT INTO old_resource VALUES('keep-in-backup')`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if resetRun([]string{"--backup", backup}) == 0 {
		t.Fatal("reset lacked explicit confirmation")
	}
	db, e = sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	var value string
	db.QueryRow(`SELECT id FROM old_resource`).Scan(&value)
	db.Close()
	if value != "keep-in-backup" {
		t.Fatal("implicit reset mutated source")
	}
	if resetRun([]string{"--confirm", "--backup", backup}) != 0 {
		t.Fatal("reset failed")
	}
	db, e = store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = sql.Open("sqlite", backup)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.QueryRow(`SELECT id FROM old_resource`).Scan(&value); e != nil || value != "keep-in-backup" {
		t.Fatal("backup lost old resources")
	}
	info, e := os.Stat(backup)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions")
	}
	if resetRun([]string{"--confirm", "--backup", backup}) == 0 {
		t.Fatal("existing backup overwritten")
	}
}
