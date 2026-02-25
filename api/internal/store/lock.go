package store

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	stateWaiting   int32 = 0
	stateLocked    int32 = 1
	stateAbandoned int32 = 2
)

// LockManager provides per-project mutex coordination to serialize concurrent operations.
type LockManager struct {
	mu    sync.Mutex
	locks sync.Map // projectID -> *projectLock
}

type projectLock struct {
	mu       sync.Mutex
	refCount int
}

// NewLockManager creates a new LockManager.
func NewLockManager() *LockManager {
	return &LockManager{}
}

// Acquire locks the given projectID for the specified operation.
// It blocks until the lock is available or ctx is cancelled.
// Returns an unlock function that must be called when the operation completes.
func (lm *LockManager) Acquire(ctx context.Context, projectID, operation string) (func(), error) {
	// Get or create the per-project lock.
	actual, _ := lm.locks.LoadOrStore(projectID, &projectLock{})
	pl, ok := actual.(*projectLock)
	if !ok {
		panic("store: LockManager: unexpected value type in locks map")
	}

	// Increment reference count under the manager-level mutex.
	lm.mu.Lock()
	pl.refCount++
	lm.mu.Unlock()

	// Acquire the project-level lock, respecting context cancellation.
	//
	// We use an atomic state variable to coordinate ownership between this
	// goroutine and the background goroutine so that exactly one of them is
	// responsible for the mutex — never both, never neither.
	state := stateWaiting
	locked := make(chan struct{}, 1) // buffered: goroutine can send without a receiver

	go func() {
		pl.mu.Lock()
		if atomic.CompareAndSwapInt32(&state, stateWaiting, stateLocked) {
			// Caller is still waiting; hand off lock ownership via channel.
			locked <- struct{}{}
		} else {
			// Caller already abandoned (set stateAbandoned); release immediately.
			pl.mu.Unlock()
		}
	}()

	select {
	case <-ctx.Done():
		if !atomic.CompareAndSwapInt32(&state, stateWaiting, stateAbandoned) {
			// Goroutine already acquired and set stateLocked before we could
			// mark stateAbandoned. The lock is sitting in the buffered channel;
			// drain it and release so the mutex is not permanently held.
			<-locked
			pl.mu.Unlock()
		}
		// Decrement our reference.
		lm.mu.Lock()
		pl.refCount--
		if pl.refCount == 0 {
			lm.locks.Delete(projectID)
		}
		lm.mu.Unlock()
		return nil, fmt.Errorf("acquire lock for %s (%s): %w", projectID, operation, ctx.Err())
	case <-locked:
		// Lock acquired successfully; ownership is ours.
	}

	unlock := func() {
		pl.mu.Unlock()
		lm.mu.Lock()
		pl.refCount--
		if pl.refCount == 0 {
			lm.locks.Delete(projectID)
		}
		lm.mu.Unlock()
	}
	return unlock, nil
}
