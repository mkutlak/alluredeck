package store_test

import (
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestOpen_CreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Verify DB is usable.
	var n int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&n); err != nil {
		t.Fatalf("schema_version not accessible: %v", err)
	}
}

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "wal.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	var mode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected WAL mode, got %q", mode)
	}
}

func TestOpen_IdempotentMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "idempotent.db")

	// Open twice — second open should not fail on already-applied migrations.
	s1, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_ = s1.Close()

	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("second Open (idempotent): %v", err)
	}
	defer func() { _ = s2.Close() }()

	// schema_version should have exactly one entry per migration file.
	var count int
	if err := s2.DB().QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 schema_version rows, got %d", count)
	}
}

func TestOpen_TablesExist(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "tables.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	tables := []string{"projects", "builds", "jwt_blacklist", "schema_version", "test_results"}
	for _, tbl := range tables {
		var name string
		err := s.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", tbl, err)
		}
	}
}

func TestOpen_BuildStabilityColumns(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "cols.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	cols := []string{"flaky_count", "retried_count", "new_failed_count", "new_passed_count"}
	for _, col := range cols {
		var exists int
		err := s.DB().QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('builds') WHERE name=?", col,
		).Scan(&exists)
		if err != nil {
			t.Errorf("checking column %q: %v", col, err)
		} else if exists == 0 {
			t.Errorf("column %q not found in builds table", col)
		}
	}
}
