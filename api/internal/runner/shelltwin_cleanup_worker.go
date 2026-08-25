package runner

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// shellTwinCleanupInterval is how often the periodic shell-twin cleanup job
// runs. Once the legacy twins are gone each run converges after one empty
// batch, so a daily cadence costs a single bounded scan per day.
const shellTwinCleanupInterval = 24 * time.Hour

// shellTwinCleanupBatchSize is how many twin rows one DeleteShellTwinBatch
// call may delete. Small enough that each batch's transaction (and the row
// locks it takes) stays short; large enough that a backlog of hundreds of
// thousands of twins drains in minutes.
const shellTwinCleanupBatchSize = 5000

// shellTwinCleanupBatchPause is the idle gap between consecutive batches so
// the cleanup never monopolizes the pool against live ingestion traffic.
const shellTwinCleanupBatchPause = 200 * time.Millisecond

// ShellTwinCleanupArgs is the River job arg for the shell-twin cleanup job.
//
// The job replaces migration 0049's single set-based DELETE: that statement
// ran during startup migrations — before the HTTP listener binds — and
// exceeded DB_STATEMENT_TIMEOUT on production-sized tables, crash-looping the
// pod. As a background job the same cleanup runs in bounded batches with
// nobody waiting on it.
type ShellTwinCleanupArgs struct {
	// BatchSize overrides the per-batch row cap; <=0 uses the default.
	BatchSize int `json:"batch_size,omitempty"`
}

// Kind implements river.JobArgs.
func (ShellTwinCleanupArgs) Kind() string { return "shell_twin_cleanup" }

// ShellTwinCleanupWorker deletes legacy shell-twin test_results rows in
// bounded batches until a batch comes back empty.
type ShellTwinCleanupWorker struct {
	river.WorkerDefaults[ShellTwinCleanupArgs]

	testResults store.TestResultStorer
	logger      *zap.Logger
}

var _ river.Worker[ShellTwinCleanupArgs] = (*ShellTwinCleanupWorker)(nil)

// Work runs cleanup batches until the table has converged (an empty batch) or
// the job context is cancelled. Cancellation mid-backlog is safe: every batch
// commits independently, and the next run resumes exactly where this one
// stopped because the predicate is state-derived, not cursor-derived.
func (w *ShellTwinCleanupWorker) Work(ctx context.Context, job *river.Job[ShellTwinCleanupArgs]) error {
	batch := job.Args.BatchSize
	if batch <= 0 {
		batch = shellTwinCleanupBatchSize
	}

	var total int64
	for {
		n, err := w.testResults.DeleteShellTwinBatch(ctx, batch)
		if err != nil {
			return err
		}
		total += n
		if n < int64(batch) {
			break
		}
		select {
		case <-ctx.Done():
			w.logger.Info("shell-twin cleanup interrupted; remaining twins picked up next run",
				zap.Int64("deleted", total))
			return ctx.Err()
		case <-time.After(shellTwinCleanupBatchPause):
		}
	}

	if total > 0 {
		w.logger.Info("shell-twin cleanup complete", zap.Int64("deleted", total))
	}
	return nil
}
