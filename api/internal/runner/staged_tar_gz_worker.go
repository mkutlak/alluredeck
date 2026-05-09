package runner

import (
	"context"
	"fmt"
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

	concurrency := uploadConcurrencyFromCfg(w.cfg)
	maxFiles := DefaultMaxArchiveFileCount
	if w.cfg != nil && w.cfg.MaxArchiveFileCount > 0 {
		maxFiles = w.cfg.MaxArchiveFileCount
	}

	reporter := func(phase JobPhase, done, total int) {
		if w.progress != nil {
			w.progress.upsertJobProgress(ctx, job.ID, phase, done, total)
		}
	}

	_, extractErr := ExtractTarGzToStorage(ctx, w.store, a.StorageKey, a.BatchID, blob, TarExtractOptions{
		MaxDecompressedBytes: DefaultMaxDecompressedBytes,
		MaxFileCount:         maxFiles,
		Concurrency:          concurrency,
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
		return extractErr
	}

	// Chain report generation in-process so the same River job tracks the full
	// async upload lifecycle (single job_id surfaces extracting_staged →
	// preparing_local → … → completed via the existing reporter).
	gen := w.generator
	if pr, ok := w.generator.(progressAwareGenerator); ok && w.progress != nil {
		gen = pr.WithProgressReporter(reporter)
	}
	reportID, err := gen.GenerateReport(ctx, a.ProjectID, a.Slug, a.StorageKey, a.BatchID,
		a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults,
		a.CIBranch, a.CICommitSHA, a.CIPipelineID, a.CIPipelineURL)
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

// uploadConcurrencyFromCfg returns the configured upload write concurrency,
// defaulting to 32 when unset. Mirrors handlers.uploadWriteConcurrency but
// lives in the runner package so the staged worker can reuse it without
// importing handlers.
func uploadConcurrencyFromCfg(cfg *config.Config) int {
	if cfg != nil && cfg.UploadWriteConcurrency > 0 {
		return cfg.UploadWriteConcurrency
	}
	return 32
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
