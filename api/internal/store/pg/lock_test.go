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
	s, err := pg.Open(context.Background(), &config.Config{DatabaseURL: url})
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
