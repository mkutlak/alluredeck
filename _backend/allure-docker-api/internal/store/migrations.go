package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

var allMigrations []migration

func init() {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		panic("store: cannot read migrations directory: " + err.Error())
	}
	for _, e := range entries {
		if e.IsDir() || len(e.Name()) < 3 {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			panic("store: cannot read migration " + e.Name() + ": " + err.Error())
		}
		var version int
		if n, err := fmt.Sscanf(e.Name(), "%03d", &version); n != 1 || err != nil {
			continue
		}
		allMigrations = append(allMigrations, migration{
			version: version,
			name:    e.Name(),
			sql:     string(data),
		})
	}
	sort.Slice(allMigrations, func(i, j int) bool {
		return allMigrations[i].version < allMigrations[j].version
	})
}

// applyMigrations ensures schema_version table exists, then applies all pending migrations.
func (s *SQLiteStore) applyMigrations(ctx context.Context) error {
	// Bootstrap: create schema_version if it doesn't exist yet.
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		)
	`); err != nil {
		return fmt.Errorf("creating schema_version: %w", err)
	}

	for _, m := range allMigrations {
		var count int
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_version WHERE version = ?", m.version,
		).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		if _, err := s.db.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := s.db.ExecContext(ctx,
			"INSERT INTO schema_version(version) VALUES(?)", m.version,
		); err != nil {
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
	}
	return nil
}
