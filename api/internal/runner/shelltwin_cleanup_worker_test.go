package runner

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestShellTwinCleanupWorker pins the drain loop: the worker keeps deleting
// batches while each comes back full and stops at the first short batch (the
// backlog has converged), so an empty table costs one round-trip; a failing
// batch surfaces to River so the job is retried, not reported successful.
func TestShellTwinCleanupWorker(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name       string
		batchSize  int
		deleted    []int64 // rows removed per successive call
		err        error
		wantLimits []int // limit passed on each call; nil = don't check
		wantCalls  int
	}{
		{name: "drains until a short batch", batchSize: 2, deleted: []int64{2, 2, 1}, wantLimits: []int{2, 2, 2}, wantCalls: 3},
		{name: "empty table is one call", deleted: []int64{0}, wantCalls: 1},
		{name: "store error propagates", deleted: []int64{0}, err: boom, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var limits []int
			mock := &testutil.MockTestResultStore{
				DeleteShellTwinBatchFn: func(_ context.Context, limit int) (int64, error) {
					limits = append(limits, limit)
					return tc.deleted[len(limits)-1], tc.err
				},
			}
			w := &ShellTwinCleanupWorker{testResults: mock, logger: zap.NewNop()}
			job := &river.Job[ShellTwinCleanupArgs]{
				JobRow: &rivertype.JobRow{ID: 7, Attempt: 1, CreatedAt: time.Now()},
				Args:   ShellTwinCleanupArgs{BatchSize: tc.batchSize},
			}
			if err := w.Work(context.Background(), job); !errors.Is(err, tc.err) {
				t.Fatalf("Work error = %v, want %v", err, tc.err)
			}
			if len(limits) != tc.wantCalls {
				t.Errorf("store called %d times, want %d", len(limits), tc.wantCalls)
			}
			if tc.wantLimits != nil && !slices.Equal(limits, tc.wantLimits) {
				t.Errorf("limits = %v, want %v", limits, tc.wantLimits)
			}
		})
	}
}
