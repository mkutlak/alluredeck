//go:build integration

package pg_test

import (
	"context"
	"os"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestMigrationIdempotency opens two PGStore instances against the same database
// in sequence to verify that goose + River migrations are idempotent: the second
// Open must succeed even though all migrations have already been applied.
func TestMigrationIdempotency(t *testing.T) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping migration idempotency smoke test")
	}

	ctx := context.Background()
	cfg := &config.Config{DatabaseURL: url, RunMigrations: true}

	// First open: applies all pending goose + River migrations.
	s1, err := pg.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("first pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s1.Close() })

	// Second open: all migrations already applied — must be a no-op and succeed.
	s2, err := pg.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("second pg.Open (idempotency check): %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	// Sanity-check that the schema is present by querying the projects table.
	row := s2.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM projects")
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("schema sanity check (SELECT COUNT(*) FROM projects): %v", err)
	}
}
