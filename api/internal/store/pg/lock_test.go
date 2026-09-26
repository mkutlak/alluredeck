package pg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// TestAcquireLock covers the advisory lock on a pool whose connections carry
// a 500ms lock_timeout GUC. Different keys never block each other; a waiter on
// a held key stays blocked until release, bounded only by its ctx; and — the
// regression — a wait outlasting lock_timeout is not cancelled by Postgres, as
// the old blocking pg_advisory_lock was ("canceling statement due to lock
// timeout") while the caller's ctx was still valid.
func TestAcquireLock(t *testing.T) {
	s := openTestStore(t, config.Config{DBLockTimeout: 500 * time.Millisecond})
	ctx := context.Background()
	key, other := unique("lock"), unique("lock")

	unlock, err := s.AcquireLock(ctx, key)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	held := true
	defer func() {
		if held { // a failed check must not leave the connection held: pool.Close would hang
			unlock()
		}
	}()

	otherCtx, cancelOther := context.WithTimeout(ctx, 3*time.Second)
	defer cancelOther()
	unlockOther, err := s.AcquireLock(otherCtx, other)
	if err != nil {
		t.Fatalf("lock on a different key blocked: %v", err)
	}
	unlockOther()

	// A waiter whose ctx ends while the key is held gets the ctx error back.
	shortCtx, cancelShort := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancelShort()
	if release, err := s.AcquireLock(shortCtx, key); !errors.Is(err, context.DeadlineExceeded) {
		if err == nil {
			release()
		}
		t.Fatalf("AcquireLock on a held key with an expiring ctx = %v, want context.DeadlineExceeded", err)
	}

	done := make(chan error, 1)
	go func() {
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		release, err := s.AcquireLock(waitCtx, key)
		if err == nil {
			release()
		}
		done <- err
	}()
	time.Sleep(1200 * time.Millisecond) // hold well past the 500ms lock_timeout
	select {
	case err := <-done:
		t.Fatalf("waiter returned while the lock was held: %v", err)
	default:
	}
	unlock()
	held = false
	if err := <-done; err != nil {
		t.Fatalf("waiter must acquire after release, not be cancelled by lock_timeout: %v", err)
	}
}
