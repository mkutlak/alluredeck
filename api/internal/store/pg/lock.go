package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// compile-time check that Store implements store.Locker.
var _ store.Locker = (*PGStore)(nil)

// AcquireLock acquires a PostgreSQL session-level advisory lock for the given key,
// serialising concurrent operations (e.g. report generation) per project across
// all application instances. A dedicated connection is held until the returned
// release function is called, which unlocks and returns the connection to the pool.
//
// Acquisition polls the non-blocking pg_try_advisory_lock with exponential
// backoff rather than blocking on pg_advisory_lock, because pooled connections
// have a lock_timeout GUC set (see store.go AfterConnect): a blocking wait
// longer than that timeout would be cancelled by Postgres even though the
// caller's ctx is still valid. Polling with pg_try_advisory_lock returns
// immediately each attempt, so lock_timeout never trips; the only bound on
// total wait time is the caller's ctx.
func (s *PGStore) AcquireLock(ctx context.Context, key string) (func(), error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire pg connection for advisory lock: %w", err)
	}

	backoff := 25 * time.Millisecond
	const maxBackoff = time.Second
	for {
		var got bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", key).Scan(&got); err != nil {
			conn.Release()
			return nil, fmt.Errorf("pg_try_advisory_lock(%q): %w", key, err)
		}
		if got {
			break
		}
		select {
		case <-ctx.Done():
			conn.Release()
			return nil, fmt.Errorf("acquire advisory lock %q: %w", key, ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return func() {
		// Use Background context: unlock must complete even if the caller's ctx is cancelled.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", key)
		conn.Release()
	}, nil
}
