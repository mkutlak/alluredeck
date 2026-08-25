package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func fakeShellTwinJob(args ShellTwinCleanupArgs) *river.Job[ShellTwinCleanupArgs] {
	return &river.Job[ShellTwinCleanupArgs]{
		JobRow: &rivertype.JobRow{ID: 7, Attempt: 1, CreatedAt: time.Now()},
		Args:   args,
	}
}

// TestShellTwinCleanupWorker_DrainsUntilShortBatch pins the loop contract: the
// worker keeps deleting batches while each one comes back full, and stops at
// the first short batch — the sign the backlog has converged.
func TestShellTwinCleanupWorker_DrainsUntilShortBatch(t *testing.T) {
	returns := []int64{2, 2, 1} // batch size 2: two full batches, one short
	var calls int
	var limits []int
	mock := &testutil.MockTestResultStore{
		DeleteShellTwinBatchFn: func(_ context.Context, limit int) (int64, error) {
			limits = append(limits, limit)
			n := returns[calls]
			calls++
			return n, nil
		},
	}

	w := &ShellTwinCleanupWorker{testResults: mock, logger: zap.NewNop()}
	if err := w.Work(context.Background(), fakeShellTwinJob(ShellTwinCleanupArgs{BatchSize: 2})); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if calls != 3 {
		t.Errorf("store called %d times, want 3", calls)
	}
	for i, l := range limits {
		if l != 2 {
			t.Errorf("call %d used limit %d, want 2", i, l)
		}
	}
}

// TestShellTwinCleanupWorker_EmptyTableIsOneCall verifies the converged case
// costs exactly one store round-trip.
func TestShellTwinCleanupWorker_EmptyTableIsOneCall(t *testing.T) {
	var calls int
	mock := &testutil.MockTestResultStore{
		DeleteShellTwinBatchFn: func(_ context.Context, _ int) (int64, error) {
			calls++
			return 0, nil
		},
	}

	w := &ShellTwinCleanupWorker{testResults: mock, logger: zap.NewNop()}
	if err := w.Work(context.Background(), fakeShellTwinJob(ShellTwinCleanupArgs{})); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if calls != 1 {
		t.Errorf("store called %d times, want 1", calls)
	}
}

// TestShellTwinCleanupWorker_PropagatesStoreError verifies a failing batch
// surfaces to River so the job is retried rather than reported successful.
func TestShellTwinCleanupWorker_PropagatesStoreError(t *testing.T) {
	boom := errors.New("boom")
	mock := &testutil.MockTestResultStore{
		DeleteShellTwinBatchFn: func(_ context.Context, _ int) (int64, error) {
			return 0, boom
		},
	}

	w := &ShellTwinCleanupWorker{testResults: mock, logger: zap.NewNop()}
	if err := w.Work(context.Background(), fakeShellTwinJob(ShellTwinCleanupArgs{})); !errors.Is(err, boom) {
		t.Fatalf("Work error = %v, want %v", err, boom)
	}
}
