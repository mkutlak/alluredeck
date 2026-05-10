package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ParseStagedTarGzArgs holds the River job arguments for processing a staged
// tar.gz blob produced by the async upload path. The blob lives at StagingKey
// in the storage backend; the worker opens it, runs the shared extraction
// helper, then chains report generation in-process via the AllureDeck runner.
type ParseStagedTarGzArgs struct {
	ProjectID     int64  `json:"project_id"`
	Slug          string `json:"slug"`
	StorageKey    string `json:"storage_key"`
	BatchID       string `json:"batch_id"`
	StagingKey    string `json:"staging_key"`
	ExecName      string `json:"exec_name"`
	ExecFrom      string `json:"exec_from"`
	ExecType      string `json:"exec_type"`
	StoreResults  bool   `json:"store_results"`
	CIBranch      string `json:"ci_branch"`
	CICommitSHA   string `json:"ci_commit_sha"`
	CIPipelineID  string `json:"ci_pipeline_id"`
	CIPipelineURL string `json:"ci_pipeline_url"`
}

// Kind returns the River job kind identifier.
func (ParseStagedTarGzArgs) Kind() string { return "parse_staged_targz" }

// ParseStagedTarGzWorker processes a staged tar.gz blob: opens the storage
// stream, extracts entries to projectID/results/{batchID}/, chains the existing
// report generator, and finally deletes the staging blob on success. On any
// extraction failure the staging blob is left in place for inspection (the
// periodic cleanup job eventually GCs it).
type ParseStagedTarGzWorker struct {
	river.WorkerDefaults[ParseStagedTarGzArgs]
	store        storage.Store
	cfg          *config.Config
	generator    ReportGenerator
	buildStore   store.BuildStorer
	webhookStore store.WebhookStorer
	externalURL  string
	riverClient  *river.Client[pgx.Tx]
	reportIDs    *sync.Map
	jobTimeout   time.Duration
	progress     riverProgressWriter
	logger       *zap.Logger
}

// Work implements river.Worker.
//
// The staged worker extracts the tar.gz directly to a pod-local temp dir and
// runs report generation against that dir. The legacy implementation wrote
// each entry to MinIO at storageKey/results/{batchID}/ and then immediately
// downloaded the same files back via PrepareLocal — observed in prod as ~4 min
// extracting_staged + ~4 min preparing_local for a 15919-file batch.
//
// The staging tar.gz blob remains the durability source of truth: on pod
// failure River retries from there, so the per-file MinIO write was pure
// dead weight. Atomicity: if extraction fails partway, defer os.RemoveAll
// removes the temp dir and no MinIO state was touched (a strict improvement
// over the old behavior, which left half-written objects under
// storageKey/results/{batchID}/).
func (w *ParseStagedTarGzWorker) Work(ctx context.Context, job *river.Job[ParseStagedTarGzArgs]) error {
	a := job.Args
	if w.progress != nil {
		w.progress.upsertJobProgress(ctx, job.ID, JobPhaseExtractingStaged, 0, 0)
	}

	blob, err := w.store.OpenBlob(ctx, a.StagingKey)
	if err != nil {
		w.logger.Error("river: open staging blob failed",
			zap.Int64("job_id", job.ID),
			zap.String("staging_key", a.StagingKey),
			zap.Error(err),
		)
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, JobPhaseFailed, 0, 0)
		}
		return fmt.Errorf("open staging blob %q: %w", a.StagingKey, err)
	}

	maxFiles := DefaultMaxArchiveFileCount
	if w.cfg != nil && w.cfg.MaxArchiveFileCount > 0 {
		maxFiles = w.cfg.MaxArchiveFileCount
	}

	reporter := func(phase JobPhase, done, total int) {
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, phase, done, total)
		}
	}

	tmpDir, err := os.MkdirTemp("", "alluredeck-staged-*")
	if err != nil {
		_ = blob.Close()
		w.logger.Error("river: create temp dir failed",
			zap.Int64("job_id", job.ID),
			zap.Error(err),
		)
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, JobPhaseFailed, 0, 0)
		}
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	localResultsDir := filepath.Join(tmpDir, "results", a.BatchID)
	//nolint:gosec // G301: 0o755 required for the allure CLI to read inputs
	if err := os.MkdirAll(localResultsDir, 0o755); err != nil {
		_ = blob.Close()
		w.logger.Error("river: create local results dir failed",
			zap.Int64("job_id", job.ID),
			zap.String("dir", localResultsDir),
			zap.Error(err),
		)
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, JobPhaseFailed, 0, 0)
		}
		return fmt.Errorf("create local results dir %q: %w", localResultsDir, err)
	}

	// TODO(progress): plumb reporter into ExtractTarGzToDir already wired —
	// the second-pass progress agent will refine its cadence.
	_, extractErr := ExtractTarGzToDir(ctx, blob, localResultsDir, TarExtractOptions{
		MaxDecompressedBytes: DefaultMaxDecompressedBytes,
		MaxFileCount:         maxFiles,
		Reporter:             reporter,
	})
	_ = blob.Close()

	if extractErr != nil {
		w.logger.Error("river: staged tar.gz extraction failed",
			zap.Int64("job_id", job.ID),
			zap.Int64("project_id", a.ProjectID),
			zap.String("slug", a.Slug),
			zap.String("staging_key", a.StagingKey),
			zap.Error(extractErr),
		)
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, JobPhaseFailed, 0, 0)
		}
		// Leave the staging blob in place for inspection — cleanup job GCs eventually.
		// The temp dir is removed on defer above; no MinIO state was touched.
		return extractErr
	}

	// Chain report generation in-process so the same River job tracks the full
	// async upload lifecycle (single job_id surfaces extracting_staged →
	// preparing_local → … → completed via the existing reporter).
	//
	// Prefer the local-dir path when the configured generator supports it
	// (production *Allure does). Fall back to the legacy GenerateReport for
	// test doubles that only satisfy ReportGenerator.
	var reportID string
	if local, ok := w.generator.(LocalReportGenerator); ok {
		gen := local
		if pr, ok := w.generator.(progressAwareGenerator); ok && w.progress != nil {
			// *Allure satisfies LocalReportGenerator (see iface.go); the implicit
			// interface conversion keeps the local path bound to the per-job reporter.
			gen = pr.WithProgressReporter(reporter)
		}
		// TODO(progress): the local path emits preparing_local from inside
		// generateReportFromLocal; wiring will be refined by the progress agent.
		reportID, err = gen.GenerateReportFromLocalDir(ctx, a.ProjectID, a.Slug, a.StorageKey, a.BatchID,
			a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults,
			a.CIBranch, a.CICommitSHA, a.CIPipelineID, a.CIPipelineURL,
			tmpDir)
	} else {
		gen := w.generator
		if pr, ok := w.generator.(progressAwareGenerator); ok && w.progress != nil {
			gen = pr.WithProgressReporter(reporter)
		}
		reportID, err = gen.GenerateReport(ctx, a.ProjectID, a.Slug, a.StorageKey, a.BatchID,
			a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults,
			a.CIBranch, a.CICommitSHA, a.CIPipelineID, a.CIPipelineURL)
	}
	if err != nil {
		w.logger.Error("river: staged report generation failed",
			zap.Int64("job_id", job.ID),
			zap.Int64("project_id", a.ProjectID),
			zap.String("slug", a.Slug),
			zap.Error(err),
		)
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, JobPhaseFailed, 0, 0)
		}
		return err
	}
	if reportID != "" && w.reportIDs != nil {
		w.reportIDs.Store(job.ID, reportID)
	}

	// Successful extract + generate — drop the staging blob.
	if delErr := w.store.DeleteBlob(ctx, a.StagingKey); delErr != nil {
		w.logger.Warn("river: failed to delete staging blob after success",
			zap.Int64("job_id", job.ID),
			zap.String("staging_key", a.StagingKey),
			zap.Error(delErr),
		)
	}

	if w.progress != nil {
		w.progress.upsertJobProgress(ctx, job.ID, JobPhaseCompleted, 0, 0)
	}
	w.logger.Info("river: staged tar.gz job completed",
		zap.Int64("job_id", job.ID),
		zap.Int64("project_id", a.ProjectID),
		zap.String("slug", a.Slug),
	)

	// Webhook fan-out is best-effort and only meaningful when the worker is
	// fully wired (River client + webhook/build stores). Skip in tests that
	// run Work directly with a partial worker.
	if w.webhookStore != nil && w.buildStore != nil && w.riverClient != nil {
		if err := enqueueWebhooksForProject(ctx, a.ProjectID, w.buildStore, w.webhookStore, w.riverClient, w.externalURL, w.logger); err != nil {
			w.logger.Warn("river: failed to enqueue webhook notifications", zap.String("slug", a.Slug), zap.Error(err))
		}
	}
	return nil
}

// Timeout overrides River's 1-minute default. The staged path runs both
// extraction and report generation, so it inherits the same ceiling as
// GenerateReportWorker.
func (w *ParseStagedTarGzWorker) Timeout(*river.Job[ParseStagedTarGzArgs]) time.Duration {
	return w.jobTimeout
}

// StagingCleanupArgs is the River job arg for the periodic staging GC job.
// It carries the retention window so operators can adjust it without rebuilding
// the binary.
type StagingCleanupArgs struct {
	OlderThanSeconds int64 `json:"older_than_seconds"`
}

// Kind returns the River job kind identifier.
func (StagingCleanupArgs) Kind() string { return "staging_cleanup" }

// StagingCleanupWorker deletes orphan blobs under the storage backend's
// "staging/" prefix that are older than OlderThanSeconds. Runs hourly.
type StagingCleanupWorker struct {
	river.WorkerDefaults[StagingCleanupArgs]
	store  storage.Store
	logger *zap.Logger
}

// Work implements river.Worker.
func (w *StagingCleanupWorker) Work(ctx context.Context, job *river.Job[StagingCleanupArgs]) error {
	older := time.Duration(job.Args.OlderThanSeconds) * time.Second
	if older <= 0 {
		older = 7 * 24 * time.Hour
	}
	keys, err := w.store.ListStagingBlobs(ctx, older)
	if err != nil {
		return fmt.Errorf("list staging blobs: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	var failed int
	for _, k := range keys {
		if delErr := w.store.DeleteBlob(ctx, k); delErr != nil {
			w.logger.Warn("staging cleanup: delete failed", zap.String("key", k), zap.Error(delErr))
			failed++
			continue
		}
	}
	w.logger.Info("staging cleanup: pass complete",
		zap.Int("listed", len(keys)),
		zap.Int("deleted", len(keys)-failed),
		zap.Int("failed", failed),
		zap.Duration("retention", older),
	)
	return nil
}
