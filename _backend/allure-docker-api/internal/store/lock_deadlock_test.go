package store_test

// TestLockManager_NoDeadlockAfterCancellation verifies that when a context-cancelled
// waiter's background goroutine races for the lock after the waiter gave up,
// other waiters are not permanently blocked.
//
// Scenario:
//
//	holder      → holds the lock
//	long-waiter → waiting with no timeout (must eventually succeed)
//	short-waiter → waiting with short timeout (will cancel)
//
// Bug: if short-waiter's background goroutine acquires pl.mu after short-waiter
// gave up (ctx.Done), it closes `locked` without calling Unlock. long-waiter
// then blocks forever on pl.mu.Lock().
import (
	"context"
	"testing"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

func TestLockManager_NoDeadlockAfterCancellation(t *testing.T) {
	lm := store.NewLockManager()

	// Step 1: holder acquires the lock and holds it.
	holderCtx := context.Background()
	holderUnlock, err := lm.Acquire(holderCtx, "proj", "holder")
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	// Step 2: long-waiter starts waiting with no timeout.
	longDone := make(chan error, 1)
	longUnlock := make(chan func(), 1)
	go func() {
		unlock, acquireErr := lm.Acquire(context.Background(), "proj", "long-waiter")
		longDone <- acquireErr
		if acquireErr == nil {
			longUnlock <- unlock
		}
	}()

	// Step 3: short-waiter starts with a very short timeout so it cancels quickly.
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer shortCancel()

	shortDone := make(chan struct{}, 1)
	go func() {
		_, _ = lm.Acquire(shortCtx, "proj", "short-waiter")
		shortDone <- struct{}{}
	}()

	// Wait for short-waiter to time out.
	select {
	case <-shortDone:
		// short-waiter cancelled as expected
	case <-time.After(2 * time.Second):
		t.Fatal("short-waiter did not cancel within 2s")
	}

	// Step 4: release the holder — long-waiter should now be able to acquire.
	holderUnlock()

	// Step 5: assert long-waiter completes within 500ms.
	select {
	case err := <-longDone:
		if err != nil {
			t.Fatalf("long-waiter expected success, got: %v", err)
		}
		// Clean up the long-waiter's lock.
		unlock := <-longUnlock
		unlock()
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock detected: long-waiter did not complete within 500ms after holder released")
	}
}
