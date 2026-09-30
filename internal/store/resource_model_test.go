package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOldEpochRefusedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	schema, e := os.ReadFile("migrations/0001_resource_model.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(strings.Split(string(schema), "-- +goose Down")[0]); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO project VALUES('original','keep me','', '2020-01-01','2020-01-01')`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if _, e = Open(path); !errors.Is(e, ErrSchemaResetRequired) {
		t.Fatalf("old epoch accepted: %v", e)
	}
	db, e = sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var epoch, name string
	if e = db.QueryRow(`SELECT value FROM schema_meta WHERE key='schema_epoch'`).Scan(&epoch); e != nil || epoch != "5" {
		t.Fatal("epoch changed")
	}
	if e = db.QueryRow(`SELECT name FROM project WHERE id='original'`).Scan(&name); e != nil || name != "keep me" {
		t.Fatal("old user data mutated")
	}
}
func TestForeignKeysJSONAndEpoch(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "new.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var fk int
	if e = db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); e != nil || fk != 1 {
		t.Fatal("foreign keys disabled")
	}
	if _, e = db.Exec(`INSERT INTO provider_connection VALUES('c','missing','label','token','encrypted',1,1,'{}','valid',0,0)`); e == nil {
		t.Fatal("invalid instance FK accepted")
	}
	if _, e = db.Exec(`INSERT INTO provider_instance VALUES('i','fixture','cloud',0,'{}',0)`); e == nil {
		t.Fatal("version zero accepted")
	}
	if _, e = db.Exec(`INSERT INTO provider_instance VALUES('i','fixture','cloud',1,'{broken',0)`); e == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, e = db.Exec(`INSERT INTO provider_instance VALUES('i','fixture','cloud',1,'{}',0)`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO provider_instance VALUES('j','fixture','enterprise',1,'{}',0)`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO provider_connection VALUES('c','i','label','token','encrypted',1,1,'{}','valid',0,0)`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO provider_scope(id,provider_instance_id,scope_type,scope_key,locator_version,locator_json,label) VALUES('s','j','account','["A"]',1,'{}','account')`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO connection_scope VALUES('b','c','s',1,'{}','valid',0)`); e == nil {
		t.Fatal("cross-instance binding accepted")
	}
	var epoch string
	db.QueryRow(`SELECT value FROM schema_meta WHERE key='schema_epoch'`).Scan(&epoch)
	if epoch != "6" {
		t.Fatal(epoch)
	}
}
