package store_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestLockManager_SameProjectSerialized(t *testing.T) {
	lm := store.NewLockManager()
	ctx := context.Background()

	var counter int32
	var wg sync.WaitGroup
	results := make([]int32, 0, 10)
	var mu sync.Mutex

	for range 5 {
		wg.Go(func() {
			unlock, err := lm.Acquire(ctx, "proj", "op")
			if err != nil {
				t.Errorf("Acquire: %v", err)
				return
			}
			defer unlock()
			val := atomic.AddInt32(&counter, 1)
			mu.Lock()
			results = append(results, val)
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(results) != 5 {
		t.Errorf("expected 5 completions, got %d", len(results))
	}
}

func TestLockManager_DifferentProjectConcurrent(t *testing.T) {
	lm := store.NewLockManager()
	ctx := context.Background()

	done := make(chan string, 2)
	for _, id := range []string{"proj-a", "proj-b"} {
		go func(id string) {
			unlock, err := lm.Acquire(ctx, id, "op")
			if err != nil {
				t.Errorf("Acquire %s: %v", id, err)
				done <- id
				return
			}
			time.Sleep(20 * time.Millisecond)
			unlock()
			done <- id
		}(id)
	}

	// Both should complete; if they deadlock this test will time out.
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout: different-project locks deadlocked")
		}
	}
}

func TestLockManager_ContextCancellation(t *testing.T) {
	lm := store.NewLockManager()

	// Hold the lock.
	ctx := context.Background()
	unlock, err := lm.Acquire(ctx, "blocked", "holder")
	if err != nil {
		t.Fatalf("Acquire holder: %v", err)
	}
	defer unlock()

	// Try to acquire with a cancelled context.
	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = lm.Acquire(cancelCtx, "blocked", "waiter")
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}
