package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// SQLiteStore is the main database handle wrapping an SQLite connection.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at dbPath, applies WAL pragma,
// foreign keys pragma, and runs pending migrations. Returns ready-to-use store.
func Open(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Use a single connection to ensure pragmas apply to the same connection.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), `PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close() // error intentionally ignored in recovery path
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close() // error intentionally ignored in recovery path
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.applyMigrations(context.Background()); err != nil {
		_ = db.Close() // error intentionally ignored in recovery path
		return nil, fmt.Errorf("applying migrations: %w", err)
	}
	return s, nil
}

// DB returns the underlying *sql.DB for use by sub-stores.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
