//go:build integration

package pg_test

import (
	"context"
	"testing"
)

// TestMigrationIdempotency opens two PGStores on the same database in
// sequence: the first applies every pending goose + River migration, and the
// second, finding them all applied, must be a no-op that still succeeds with
// the schema in place.
func TestMigrationIdempotency(t *testing.T) {
	openTestStore(t)
	s := openTestStore(t)
	var n int
	if err := s.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM projects").Scan(&n); err != nil {
		t.Fatalf("schema sanity check (SELECT COUNT(*) FROM projects): %v", err)
	}
}
