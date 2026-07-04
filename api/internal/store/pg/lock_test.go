package pg_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// openLockTestStore opens a PGStore using TEST_POSTGRES_URL; skips if unset.
func openLockTestStore(t *testing.T) *pg.PGStore {
	t.Helper()
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping advisory lock integration test")
	}
	s, err := pg.Open(context.Background(), &config.Config{DatabaseURL: url, RunMigrations: true})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// openLockTestStoreWithLockTimeout opens a PGStore using TEST_POSTGRES_URL,
// same as openLockTestStore, but sets a short lock_timeout GUC on every
// pooled connection so tests can reproduce a wait that outlasts lock_timeout.
func openLockTestStoreWithLockTimeout(t *testing.T, lockTimeout time.Duration) *pg.PGStore {
	t.Helper()
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping advisory lock integration test")
	}
	s, err := pg.Open(context.Background(), &config.Config{
		DatabaseURL:   url,
		RunMigrations: true,
		DBLockTimeout: lockTimeout,
	})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestAcquireLock_SerializesAccess verifies that two goroutines acquiring the
// same key are serialised: the second blocks until the first releases.
func TestAcquireLock_SerializesAccess(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	const key = "test_advisory_lock"

	unlock1, err := s.AcquireLock(ctx, key)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		unlock2, err := s.AcquireLock(ctx, key)
		if err != nil {
			return
		}
		close(acquired)
		unlock2()
	}()

	// Second lock must not be acquired while first is held.
	select {
	case <-acquired:
		t.Fatal("second lock acquired before first was released")
	case <-time.After(150 * time.Millisecond):
		// expected
	}

	unlock1()

	select {
	case <-acquired:
		// expected: second acquired after first released
	case <-time.After(3 * time.Second):
		t.Fatal("second lock not acquired after first was released")
	}
}

// TestAcquireLock_DifferentKeysDoNotBlock verifies that locks on different keys
// are independent and do not block each other.
func TestAcquireLock_DifferentKeysDoNotBlock(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	unlock1, err := s.AcquireLock(ctx, "key_alpha")
	if err != nil {
		t.Fatalf("AcquireLock(key_alpha): %v", err)
	}
	defer unlock1()

	done := make(chan struct{})
	go func() {
		unlock2, err := s.AcquireLock(ctx, "key_beta")
		if err != nil {
			return
		}
		unlock2()
		close(done)
	}()

	select {
	case <-done:
		// expected: different keys do not block
	case <-time.After(3 * time.Second):
		t.Fatal("lock on different key blocked unexpectedly")
	}
}

// TestAcquireLock_CancelledContextReturnsError verifies that cancelling the
// context while waiting returns an error without leaving a phantom lock.
func TestAcquireLock_CancelledContextReturnsError(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	const key = "test_cancel_lock"

	unlock1, err := s.AcquireLock(ctx, key)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Go(func() {
		_, err := s.AcquireLock(cancelCtx, key)
		if err == nil {
			t.Error("expected error from cancelled context, got nil")
		}
	})

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	// Original lock must still be releasable.
	unlock1()
}

// TestAcquireLock_NotCancelledByLockTimeout reproduces the original bug: a
// legitimate holder keeps an advisory lock for longer than the connection's
// lock_timeout GUC. Under the old blocking pg_advisory_lock implementation
// the waiter's acquire would be cancelled by Postgres ("canceling statement
// due to lock timeout") even though the caller's ctx was still valid. With
// the pg_try_advisory_lock poll, the wait is bounded only by ctx, so the
// waiter must eventually succeed.
func TestAcquireLock_NotCancelledByLockTimeout(t *testing.T) {
	s := openLockTestStoreWithLockTimeout(t, 500*time.Millisecond)
	ctx := context.Background()

	const key = "test_lock_timeout_key"

	holderAcquired := make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		unlock, err := s.AcquireLock(ctx, key)
		if err != nil {
			close(holderAcquired)
			return
		}
		close(holderAcquired)
		// Hold the lock well past the 500ms lock_timeout.
		time.Sleep(1200 * time.Millisecond)
		unlock()
	}()

	<-holderAcquired

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	unlock2, err := s.AcquireLock(waitCtx, key)
	if err != nil {
		t.Fatalf("AcquireLock should not be cancelled by the 500ms lock_timeout: %v", err)
	}
	if unlock2 == nil {
		t.Fatal("expected non-nil unlock function")
	}
	unlock2()

	<-holderDone
}
