package main

import (
	"database/sql"
	"flag"
	"fmt"
	"mevius/internal/store"
	"os"
	"path/filepath"
)

func resetRun(args []string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	backup := fs.String("backup", "", "new SQLite backup file (required)")
	confirm := fs.Bool("confirm", false, "explicitly authorize resetting this database")
	if fs.Parse(args) != nil {
		return 1
	}
	if !*confirm || *backup == "" {
		fmt.Fprintln(os.Stderr, "reset requires --confirm --backup <new-file>; stop the server first")
		return 1
	}
	source, e := filepath.Abs(databasePath())
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	destination, e := filepath.Abs(*backup)
	if e != nil || destination == source {
		fmt.Fprintln(os.Stderr, "backup must be a different path")
		return 1
	}
	if _, e = os.Stat(destination); e == nil || !os.IsNotExist(e) {
		fmt.Fprintln(os.Stderr, "backup path must not already exist")
		return 1
	}
	if _, e = os.Stat(source); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	db, e := sql.Open("sqlite", "file:"+source+"?_busy_timeout=5000")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if _, e = db.Exec(`VACUUM INTO ?`, destination); e != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, "backup failed:", e)
		return 1
	}
	db.Close()
	if e = os.Chmod(destination, 0600); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	for _, p := range []string{source, source + "-wal", source + "-shm"} {
		if e = os.Remove(p); e != nil && !os.IsNotExist(e) {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
	}
	fresh, e := store.Open(source)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	fresh.Close()
	fmt.Println("reset: epoch 6 initialized; complete backup at", destination)
	return 0
}
