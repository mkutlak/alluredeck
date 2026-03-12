package pg

import (
	"context"
	"fmt"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// compile-time check that PGStore implements store.Locker.
var _ store.Locker = (*PGStore)(nil)

// AcquireLock acquires a PostgreSQL session-level advisory lock for the given key,
// serialising concurrent operations (e.g. report generation) per project across
// all application instances. A dedicated connection is held until the returned
// release function is called, which unlocks and returns the connection to the pool.
func (s *PGStore) AcquireLock(ctx context.Context, key string) (func(), error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire pg connection for advisory lock: %w", err)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", key); err != nil {
		conn.Release()
		return nil, fmt.Errorf("pg_advisory_lock(%q): %w", key, err)
	}

	return func() {
		// Use Background context: unlock must complete even if the caller's ctx is cancelled.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", key)
		conn.Release()
	}, nil
}
