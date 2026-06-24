package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// stagingCleanupRetention is the default retention applied by the periodic
// cleanup job. Orphan staging blobs older than this are deleted on each run.
const stagingCleanupRetention = 7 * 24 * time.Hour

// stagingCleanupInterval is how often the periodic cleanup job runs.
const stagingCleanupInterval = time.Hour

// GenerateReportArgs holds the River job arguments for async report generation.
type GenerateReportArgs struct {
	ProjectID     int64  `json:"project_id"`
	Slug          string `json:"slug"`
	StorageKey    string `json:"storage_key"`
	BatchID       string `json:"batch_id"`
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
func (GenerateReportArgs) Kind() string { return "generate_report" }

// PlaywrightIngestArgs holds the River job arguments for async Playwright report ingestion.
type PlaywrightIngestArgs struct {
	ProjectID     int64  `json:"project_id"`
	Slug          string `json:"slug"`
	StorageKey    string `json:"storage_key"`
	ExecName      string `json:"exec_name"`
	ExecFrom      string `json:"exec_from"`
	CIBranch      string `json:"ci_branch"`
	CICommitSHA   string `json:"ci_commit_sha"`
	CIPipelineID  string `json:"ci_pipeline_id"`
	CIPipelineURL string `json:"ci_pipeline_url"`
}

// Kind returns the River job kind identifier.
func (PlaywrightIngestArgs) Kind() string { return "playwright_ingest" }

// GenerateReportWorker is a River worker that executes Allure report generation.
type GenerateReportWorker struct {
	river.WorkerDefaults[GenerateReportArgs]
	generator    ReportGenerator
	buildStore   store.BuildStorer
	webhookStore store.WebhookStorer
	externalURL  string
	riverClient  *river.Client[pgx.Tx]
	jobTimeout   time.Duration
	progress     riverProgressWriter
	logger       *zap.Logger
}

// riverProgressWriter persists per-job phase/progress. It is satisfied by
// *RiverJobManager; tests inject a no-op or capturing writer.
type riverProgressWriter interface {
	upsertJobProgress(ctx context.Context, jobID int64, phase JobPhase, done, total int)
}

// Work implements river.Worker.
func (w *GenerateReportWorker) Work(ctx context.Context, job *river.Job[GenerateReportArgs]) error {
	a := job.Args
	gen := w.generator
	if pr, ok := w.generator.(progressAwareGenerator); ok && w.progress != nil {
		gen = pr.WithProgressReporter(func(phase JobPhase, done, total int) {
			w.progress.upsertJobProgress(ctx, job.ID, phase, done, total)
		})
	}
	reportID, err := gen.GenerateReport(ctx, a.ProjectID, a.Slug, a.StorageKey, a.BatchID, a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults, a.CIBranch, a.CICommitSHA, a.CIPipelineID, a.CIPipelineURL)
	if err != nil {
		w.logger.Error("river: report generation failed",
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
	recordReportID(ctx, w.logger, job.ID, reportID)
	if w.progress != nil {
		w.progress.upsertJobProgress(ctx, job.ID, JobPhaseCompleted, 0, 0)
	}
	w.logger.Info("river: report generation completed",
		zap.Int64("job_id", job.ID),
		zap.Int64("project_id", a.ProjectID),
		zap.String("slug", a.Slug),
	)
	// Fire-and-forget: enqueue webhook notifications for this report.
	if err := w.enqueueWebhooks(ctx, a.ProjectID); err != nil {
		w.logger.Warn("river: failed to enqueue webhook notifications",
			zap.String("slug", a.Slug), zap.Error(err))
	}
	return nil
}

// Timeout overrides River's 1-minute default. Report generation downloads
// all Allure results from storage, runs the Allure CLI locally, and
// uploads the generated report back. Large projects (>1 GiB of results or
// heavy attachments such as .webm) easily exceed the default.
func (w *GenerateReportWorker) Timeout(*river.Job[GenerateReportArgs]) time.Duration {
	return w.jobTimeout
}

// enqueueWebhooks constructs a WebhookPayload from the latest build and enqueues
// delivery jobs for all active webhooks. Errors are non-fatal.
func (w *GenerateReportWorker) enqueueWebhooks(ctx context.Context, projectID int64) error {
	return enqueueWebhooksForProject(ctx, projectID, w.buildStore, w.webhookStore, w.riverClient, w.externalURL, w.logger)
}

// PlaywrightIngestWorker is a River worker that processes Playwright report ingestion.
type PlaywrightIngestWorker struct {
	river.WorkerDefaults[PlaywrightIngestArgs]
	runner       *PlaywrightRunner
	buildStore   store.BuildStorer
	webhookStore store.WebhookStorer
	externalURL  string
	riverClient  *river.Client[pgx.Tx]
	jobTimeout   time.Duration
	logger       *zap.Logger
}

// Work implements river.Worker for Playwright report ingestion.
func (w *PlaywrightIngestWorker) Work(ctx context.Context, job *river.Job[PlaywrightIngestArgs]) error {
	a := job.Args
	reportID, err := w.runner.IngestReport(ctx, a.ProjectID, a.Slug, a.StorageKey, a.ExecName, a.ExecFrom, a.CIBranch, a.CICommitSHA, a.CIPipelineID, a.CIPipelineURL)
	if err != nil {
		w.logger.Error("river: playwright ingest failed",
			zap.Int64("job_id", job.ID),
			zap.Int64("project_id", a.ProjectID),
			zap.String("slug", a.Slug),
			zap.Error(err),
		)
		return err
	}
	recordReportID(ctx, w.logger, job.ID, reportID)
	w.logger.Info("river: playwright ingest completed",
		zap.Int64("job_id", job.ID),
		zap.Int64("project_id", a.ProjectID),
		zap.String("slug", a.Slug),
	)
	// Fire-and-forget: enqueue webhook notifications.
	if err := w.enqueueWebhooks(ctx, a.ProjectID); err != nil {
		w.logger.Warn("river: failed to enqueue webhook notifications",
			zap.String("slug", a.Slug), zap.Error(err))
	}
	return nil
}

// Timeout overrides River's 1-minute default for Playwright ingestion,
// which can run long for reports with many trace and video attachments.
func (w *PlaywrightIngestWorker) Timeout(*river.Job[PlaywrightIngestArgs]) time.Duration {
	return w.jobTimeout
}

func (w *PlaywrightIngestWorker) enqueueWebhooks(ctx context.Context, projectID int64) error {
	return enqueueWebhooksForProject(ctx, projectID, w.buildStore, w.webhookStore, w.riverClient, w.externalURL, w.logger)
}

// webhookEventPriority defines the specificity order for event selection.
// Higher index = more specific. Used to pick the most specific triggered event
// that a webhook subscribes to.
var webhookEventPriority = []string{
	store.WebhookEventReportCompleted,
	store.WebhookEventReportFailed,
	store.WebhookEventRegressionDetected,
}

// enqueueWebhooksForProject constructs a WebhookPayload from the latest build,
// derives the set of triggered events, and enqueues ONE delivery per active
// webhook whose subscribed events intersect the triggered set. Each webhook
// receives the most specific triggered event it subscribes to.
func enqueueWebhooksForProject(ctx context.Context, projectID int64, buildStore store.BuildStorer, webhookStore store.WebhookStorer, riverClient *river.Client[pgx.Tx], externalURL string, logger *zap.Logger) error {
	// Get latest build for stats
	build, err := buildStore.GetLatestBuild(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get latest build: %w", err)
	}

	// Construct base payload
	payload := WebhookPayload{
		Event:       store.WebhookEventReportCompleted,
		ProjectID:   projectID,
		BuildNumber: build.BuildNumber,
		Timestamp:   time.Now(),
	}

	// Dashboard URL — link directly to the report, not just the project.
	if externalURL != "" {
		payload.DashboardURL = externalURL + "/projects/" + strconv.FormatInt(projectID, 10) + "/reports/" + strconv.Itoa(build.BuildNumber)
	}

	// Stats
	if build.StatTotal != nil && *build.StatTotal > 0 {
		total := derefInt(build.StatTotal)
		passed := derefInt(build.StatPassed)
		skipped := derefInt(build.StatSkipped)
		payload.Stats = WebhookStats{
			Total:   total,
			Passed:  passed,
			Failed:  derefInt(build.StatFailed),
			Broken:  derefInt(build.StatBroken),
			Skipped: skipped,
		}
		denom := total - skipped
		if denom > 0 {
			payload.Stats.PassRate = float64(passed) / float64(denom) * 100
		}
	}

	// Delta vs previous build
	prev, err := buildStore.GetPreviousBuild(ctx, projectID, build.BuildNumber)
	if err == nil && prev.StatTotal != nil && *prev.StatTotal > 0 {
		prevPassed := derefInt(prev.StatPassed)
		prevTotal := derefInt(prev.StatTotal)
		prevSkipped := derefInt(prev.StatSkipped)
		prevDenom := prevTotal - prevSkipped
		prevPassRate := 0.0
		if prevDenom > 0 {
			prevPassRate = float64(prevPassed) / float64(prevDenom) * 100
		}
		payload.Delta = &WebhookDelta{
			PassRateChange: payload.Stats.PassRate - prevPassRate,
			NewFailures:    derefInt(build.StatFailed) - derefInt(prev.StatFailed),
			FixedTests:     derefInt(prev.StatFailed) - derefInt(build.StatFailed),
		}
		if payload.Delta.NewFailures < 0 {
			payload.Delta.NewFailures = 0
		}
		if payload.Delta.FixedTests < 0 {
			payload.Delta.FixedTests = 0
		}
	}

	// CI metadata
	if build.CIProvider != nil || build.CIBranch != nil {
		payload.CI = &WebhookCI{
			Provider:  derefStr(build.CIProvider),
			BuildURL:  derefStr(build.CIBuildURL),
			Branch:    derefStr(build.CIBranch),
			CommitSHA: derefStr(build.CICommitSHA),
		}
	}

	// Derive the set of triggered events for this build.
	triggeredSet := map[string]bool{
		store.WebhookEventReportCompleted: true,
	}
	if derefInt(build.StatFailed) > 0 {
		triggeredSet[store.WebhookEventReportFailed] = true
	}
	if payload.Delta != nil && payload.Delta.NewFailures > 0 {
		triggeredSet[store.WebhookEventRegressionDetected] = true
	}

	// Collect all active webhooks for this project from each triggered event,
	// deduplicating by webhook ID so a webhook subscribed to multiple triggered
	// events is only delivered once.
	seen := make(map[string]bool)
	type webhookWithEvent struct {
		id    string
		event string
	}
	var deliveries []webhookWithEvent

	// Iterate priority order (least→most specific) so the last assignment wins,
	// giving us the most specific event when a webhook subscribes to several.
	for _, event := range webhookEventPriority {
		if !triggeredSet[event] {
			continue
		}
		whs, err := webhookStore.ListActiveForEvent(ctx, projectID, event)
		if err != nil {
			logger.Warn("river: list active webhooks failed",
				zap.Int64("project_id", projectID), zap.String("event", event), zap.Error(err))
			continue
		}
		for i := range whs {
			id := whs[i].ID
			// Overwrite: most specific event seen last wins.
			seen[id] = true
			deliveries = append(deliveries, webhookWithEvent{id: id, event: event})
		}
	}

	// Deduplicate: keep only the last entry per webhook ID (most specific event).
	finalDeliveries := make(map[string]string) // webhookID → event
	for _, d := range deliveries {
		finalDeliveries[d.id] = d.event
	}

	if len(finalDeliveries) == 0 {
		return nil
	}

	// Enqueue one job per webhook with its most specific event.
	count := 0
	for whID, event := range finalDeliveries {
		p := payload
		p.Event = event
		_, err := riverClient.Insert(ctx, SendWebhookArgs{
			WebhookID: whID,
			Payload:   p,
		}, &river.InsertOpts{
			Queue:       "webhooks",
			MaxAttempts: 5,
		})
		if err != nil {
			logger.Warn("river: failed to enqueue webhook",
				zap.String("webhook_id", whID), zap.Error(err))
			continue
		}
		count++
	}

	logger.Info("river: enqueued webhook notifications",
		zap.Int64("project_id", projectID),
		zap.Int("count", count))
	return nil
}

// enqueueFailureWebhook enqueues a minimal report_failed webhook delivery for
// all webhooks in the project that subscribe to report_failed or report_completed.
// Best-effort: errors are logged and not returned.
func enqueueFailureWebhook(ctx context.Context, projectID int64, webhookStore store.WebhookStorer, riverClient *river.Client[pgx.Tx], logger *zap.Logger) {
	payload := WebhookPayload{
		Event:     store.WebhookEventReportFailed,
		ProjectID: projectID,
		Timestamp: time.Now(),
	}

	// Collect webhooks subscribed to report_failed or report_completed (fallback).
	seen := make(map[string]bool)

	for _, event := range []string{store.WebhookEventReportCompleted, store.WebhookEventReportFailed} {
		whs, err := webhookStore.ListActiveForEvent(ctx, projectID, event)
		if err != nil {
			logger.Warn("river: enqueueFailureWebhook: list failed",
				zap.Int64("project_id", projectID), zap.String("event", event), zap.Error(err))
			continue
		}
		for i := range whs {
			id := whs[i].ID
			if seen[id] {
				continue
			}
			seen[id] = true
			p := payload
			_, err := riverClient.Insert(ctx, SendWebhookArgs{
				WebhookID: id,
				Payload:   p,
			}, &river.InsertOpts{
				Queue:       "webhooks",
				MaxAttempts: 5,
			})
			if err != nil {
				logger.Warn("river: enqueueFailureWebhook: insert failed",
					zap.String("webhook_id", id), zap.Error(err))
			}
		}
	}
}

// derefInt safely dereferences an *int, returning 0 if nil.
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// derefStr safely dereferences a *string, returning "" if nil.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// jobFailedErrorHandler is a River ErrorHandler that increments the
// aldeck_jobs_failed_total counter and fires a report_failed webhook when a
// job exhausts all its retry attempts (final discard).
type jobFailedErrorHandler struct {
	counter      metric.Int64Counter
	webhookStore store.WebhookStorer
	riverClient  *river.Client[pgx.Tx]
	logger       *zap.Logger
}

// compile-time check
var _ river.ErrorHandler = (*jobFailedErrorHandler)(nil)

// HandleError is called by River after each failed attempt. We only act when
// the job has exhausted all attempts (Attempt >= MaxAttempts) — that is the
// final discard that operators need to know about.
func (h *jobFailedErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	if job.Attempt < job.MaxAttempts {
		return nil // still has retries remaining
	}
	h.recordFailure(ctx, job)
	return nil
}

// HandlePanic is called by River when a job panics. Treat a panic on the final
// attempt the same as a hard failure.
func (h *jobFailedErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, _ any, _ string) *river.ErrorHandlerResult {
	if job.Attempt < job.MaxAttempts {
		return nil
	}
	h.recordFailure(ctx, job)
	return nil
}

// recordFailure increments the counter and fires a failure webhook.
func (h *jobFailedErrorHandler) recordFailure(ctx context.Context, job *rivertype.JobRow) {
	h.counter.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", job.Kind)))

	projectID := projectIDFromJobRow(job)
	if projectID == 0 || h.webhookStore == nil || h.riverClient == nil {
		return
	}
	// Fire-and-forget: use a background context so River's cancellation of the
	// job context doesn't prevent the webhook insert.
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		enqueueFailureWebhook(bgCtx, projectID, h.webhookStore, h.riverClient, h.logger)
	}()
}

// projectIDFromJobRow decodes the project_id from any known job arg type.
// Returns 0 if the job kind is unknown or the field is absent.
func projectIDFromJobRow(job *rivertype.JobRow) int64 {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if json.Unmarshal(job.EncodedArgs, &args) == nil && args.ProjectID != 0 {
		return args.ProjectID
	}
	return 0
}

// RiverJobManager implements JobQueuer using River backed by PostgreSQL.
// It is safe for concurrent use across multiple instances (pods).
type RiverJobManager struct {
	client  *river.Client[pgx.Tx]
	pool    *pgxpool.Pool
	logger  *zap.Logger
	running atomic.Bool // true after Start, false after Shutdown
}

// upsertJobProgress writes the latest phase/progress for a job using last-write-wins
// semantics. Errors are logged and swallowed: progress reporting must never fail
// the underlying report-generation work.
func (jm *RiverJobManager) upsertJobProgress(ctx context.Context, jobID int64, phase JobPhase, done, total int) {
	const stmt = `
		INSERT INTO job_progress (job_id, phase, progress_done, progress_total, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (job_id) DO UPDATE
		   SET phase = EXCLUDED.phase,
		       progress_done = EXCLUDED.progress_done,
		       progress_total = EXCLUDED.progress_total,
		       updated_at = now()
	`
	if _, err := jm.pool.Exec(ctx, stmt, jobID, string(phase), done, total); err != nil {
		jm.logger.Warn("failed to write job_progress",
			zap.Int64("job_id", jobID), zap.String("phase", string(phase)), zap.Error(err))
	}
}

// readJobProgressBatch fetches phase/progress for a batch of jobs in a single
// query. Jobs with no progress row are simply absent from the returned map.
// Errors are logged and an empty (non-nil) map is returned so callers can
// proceed without progress data rather than failing.
func (jm *RiverJobManager) readJobProgressBatch(ctx context.Context, jobIDs []int64) map[int64]struct {
	phase JobPhase
	done  int
	total int
} {
	result := make(map[int64]struct {
		phase JobPhase
		done  int
		total int
	}, len(jobIDs))
	if len(jobIDs) == 0 {
		return result
	}
	rows, err := jm.pool.Query(ctx,
		`SELECT job_id, phase, progress_done, progress_total FROM job_progress WHERE job_id = ANY($1)`,
		jobIDs)
	if err != nil {
		jm.logger.Warn("failed to batch-read job_progress", zap.Error(err))
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id    int64
			phase string
			done  int
			total int
		)
		if err := rows.Scan(&id, &phase, &done, &total); err != nil {
			jm.logger.Warn("failed to scan job_progress row", zap.Error(err))
			continue
		}
		result[id] = struct {
			phase JobPhase
			done  int
			total int
		}{JobPhase(phase), done, total}
	}
	if err := rows.Err(); err != nil {
		jm.logger.Warn("failed to iterate job_progress rows", zap.Error(err))
	}
	return result
}

// readJobProgress fetches the most recent phase/progress for a job. Returns
// ("", 0, 0, false) when no row exists. Errors other than no-rows are logged.
func (jm *RiverJobManager) readJobProgress(ctx context.Context, jobID int64) (JobPhase, int, int, bool) {
	var (
		phase string
		done  int
		total int
	)
	err := jm.pool.QueryRow(ctx,
		`SELECT phase, progress_done, progress_total FROM job_progress WHERE job_id = $1`,
		jobID).Scan(&phase, &done, &total)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			jm.logger.Warn("failed to read job_progress",
				zap.Int64("job_id", jobID), zap.Error(err))
		}
		return "", 0, 0, false
	}
	return JobPhase(phase), done, total, true
}

// deleteJobProgress removes the progress row associated with a deleted job.
// Errors are logged and swallowed.
func (jm *RiverJobManager) deleteJobProgress(ctx context.Context, jobID int64) {
	if _, err := jm.pool.Exec(ctx, `DELETE FROM job_progress WHERE job_id = $1`, jobID); err != nil {
		jm.logger.Warn("failed to delete job_progress",
			zap.Int64("job_id", jobID), zap.Error(err))
	}
}

// compile-time check
var _ JobQueuer = (*RiverJobManager)(nil)

// NewRiverJobManager creates a new RiverJobManager. Call Start to begin processing jobs.
// The storage store and config are used to wire the staged tar.gz worker and
// the periodic staging-cleanup job; both may be nil during tests that don't
// exercise the async upload path.
func NewRiverJobManager(pool *pgxpool.Pool, generator ReportGenerator, pwRunner *PlaywrightRunner, webhookStore store.WebhookStorer, buildStore store.BuildStorer, dataStore storage.Store, cfg *config.Config, encKey []byte, externalURL string, maxWorkers int, jobTimeout time.Duration, logger *zap.Logger) (*RiverJobManager, error) {
	jm := &RiverJobManager{pool: pool, logger: logger}

	workers := river.NewWorkers()
	reportWorker := &GenerateReportWorker{
		generator:    generator,
		buildStore:   buildStore,
		webhookStore: webhookStore,
		externalURL:  externalURL,
		jobTimeout:   jobTimeout,
		progress:     jm,
		logger:       logger,
	}
	river.AddWorker(workers, reportWorker)
	river.AddWorker(workers, &SendWebhookWorker{
		webhookStore: webhookStore,
		httpClient:   newWebhookHTTPClient(10 * time.Second),
		encKey:       encKey,
		logger:       logger,
	})

	var pwWorker *PlaywrightIngestWorker
	if pwRunner != nil {
		pwWorker = &PlaywrightIngestWorker{
			runner:       pwRunner,
			buildStore:   buildStore,
			webhookStore: webhookStore,
			externalURL:  externalURL,
			jobTimeout:   jobTimeout,
			logger:       logger,
		}
		river.AddWorker(workers, pwWorker)
	}

	var stagedWorker *ParseStagedTarGzWorker
	if dataStore != nil {
		stagedWorker = &ParseStagedTarGzWorker{
			store:        dataStore,
			cfg:          cfg,
			generator:    generator,
			buildStore:   buildStore,
			webhookStore: webhookStore,
			externalURL:  externalURL,
			jobTimeout:   jobTimeout,
			progress:     jm,
			logger:       logger,
		}
		river.AddWorker(workers, stagedWorker)
		river.AddWorker(workers, &StagingCleanupWorker{
			store:  dataStore,
			logger: logger,
		})
	}

	var periodicJobs []*river.PeriodicJob
	if dataStore != nil {
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(stagingCleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return StagingCleanupArgs{OlderThanSeconds: int64(stagingCleanupRetention.Seconds())}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		))
	}

	// Register the aldeck_jobs_failed_total counter via the global OTel meter.
	meter := otel.GetMeterProvider().Meter("aldeck")
	jobsFailedCounter, err := meter.Int64Counter(
		"aldeck_jobs_failed_total",
		metric.WithDescription("Total number of River jobs that have exhausted all retry attempts"),
	)
	if err != nil {
		return nil, fmt.Errorf("create jobs_failed counter: %w", err)
	}

	// The ErrorHandler references the River client — we wire it back after
	// client construction below.
	errHandler := &jobFailedErrorHandler{
		counter:      jobsFailedCounter,
		webhookStore: webhookStore,
		logger:       logger,
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
			"webhooks":         {MaxWorkers: 5},
		},
		Workers: workers,
		WorkerMiddleware: []rivertype.WorkerMiddleware{
			NewOTelWorkerMiddleware(),
		},
		ErrorHandler: errHandler,
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		return nil, fmt.Errorf("create River client: %w", err)
	}

	// Wire the client back into workers so they can enqueue webhook jobs.
	reportWorker.riverClient = client
	if pwWorker != nil {
		pwWorker.riverClient = client
	}
	if stagedWorker != nil {
		stagedWorker.riverClient = client
	}
	// Wire client into the error handler so it can enqueue failure webhooks.
	errHandler.riverClient = client

	jm.client = client
	return jm, nil
}

// Start begins processing River jobs using the given context.
func (jm *RiverJobManager) Start(ctx context.Context) {
	if err := jm.client.Start(ctx); err != nil {
		jm.logger.Error("river client start failed", zap.Error(err))
		return
	}
	jm.running.Store(true)
}

// Shutdown gracefully stops the River client, waiting for running jobs to complete.
func (jm *RiverJobManager) Shutdown() {
	jm.running.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := jm.client.Stop(ctx); err != nil {
		jm.logger.Error("river client stop failed", zap.Error(err))
	}
}

// Healthy returns nil when the job queue is operational.
// It returns an error if the manager has not been started or has been shut
// down, or if the underlying connection pool cannot be pinged.
func (jm *RiverJobManager) Healthy(ctx context.Context) error {
	if !jm.running.Load() {
		return errors.New("job queue not running")
	}
	if err := jm.pool.Ping(ctx); err != nil {
		return fmt.Errorf("job queue pool ping: %w", err)
	}
	return nil
}

// Submit enqueues a new report generation job via River and returns its initial state.
func (jm *RiverJobManager) Submit(ctx context.Context, projectID int64, slug string, params JobParams) *Job {
	args := GenerateReportArgs{
		ProjectID:     projectID,
		Slug:          slug,
		StorageKey:    params.StorageKey,
		BatchID:       params.BatchID,
		ExecName:      params.ExecName,
		ExecFrom:      params.ExecFrom,
		ExecType:      params.ExecType,
		StoreResults:  params.StoreResults,
		CIBranch:      params.CIBranch,
		CICommitSHA:   params.CICommitSHA,
		CIPipelineID:  params.CIPipelineID,
		CIPipelineURL: params.CIPipelineURL,
	}
	res, err := jm.client.Insert(ctx, args, &river.InsertOpts{Metadata: InjectTraceContextIntoMetadata(ctx)})
	if err != nil {
		jm.logger.Error("river insert failed", zap.Int64("project_id", projectID), zap.String("slug", slug), zap.Error(err))
		return &Job{
			ProjectID: projectID,
			Slug:      slug,
			Status:    JobStatusFailed,
			CreatedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	return riverRowToJob(res.Job)
}

// SubmitStagedTarGz enqueues a job that processes a staged tar.gz blob.
func (jm *RiverJobManager) SubmitStagedTarGz(ctx context.Context, projectID int64, slug string, params StagedTarGzParams) *Job {
	args := ParseStagedTarGzArgs{
		ProjectID:     projectID,
		Slug:          slug,
		StorageKey:    params.StorageKey,
		BatchID:       params.BatchID,
		StagingKey:    params.StagingKey,
		ExecName:      params.ExecName,
		ExecFrom:      params.ExecFrom,
		ExecType:      params.ExecType,
		StoreResults:  params.StoreResults,
		CIBranch:      params.CIBranch,
		CICommitSHA:   params.CICommitSHA,
		CIPipelineID:  params.CIPipelineID,
		CIPipelineURL: params.CIPipelineURL,
	}
	res, err := jm.client.Insert(ctx, args, &river.InsertOpts{Metadata: InjectTraceContextIntoMetadata(ctx)})
	if err != nil {
		jm.logger.Error("river insert (staged tar.gz) failed",
			zap.Int64("project_id", projectID), zap.String("slug", slug), zap.Error(err))
		return &Job{
			ProjectID: projectID,
			Slug:      slug,
			Status:    JobStatusFailed,
			CreatedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	return riverRowToJob(res.Job)
}

// SubmitPlaywright enqueues a new Playwright report ingestion job.
func (jm *RiverJobManager) SubmitPlaywright(ctx context.Context, projectID int64, slug, storageKey string, execName, execFrom, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) *Job {
	args := PlaywrightIngestArgs{
		ProjectID:     projectID,
		Slug:          slug,
		StorageKey:    storageKey,
		ExecName:      execName,
		ExecFrom:      execFrom,
		CIBranch:      ciBranch,
		CICommitSHA:   ciCommitSHA,
		CIPipelineID:  ciPipelineID,
		CIPipelineURL: ciPipelineURL,
	}
	res, err := jm.client.Insert(ctx, args, &river.InsertOpts{MaxAttempts: 3, Metadata: InjectTraceContextIntoMetadata(ctx)})
	if err != nil {
		jm.logger.Error("river insert failed", zap.Int64("project_id", projectID), zap.String("slug", slug), zap.Error(err))
		return &Job{
			ProjectID: projectID,
			Slug:      slug,
			Status:    JobStatusFailed,
			CreatedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	return riverRowToJob(res.Job)
}

// Get returns the job with the given string ID (River int64 rendered as decimal string), or nil.
func (jm *RiverJobManager) Get(ctx context.Context, jobID string) *Job {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return nil
	}
	row, err := jm.client.JobGet(ctx, id)
	if err != nil {
		return nil
	}
	j := riverRowToJob(row)
	if phase, done, total, ok := jm.readJobProgress(ctx, id); ok {
		j.Phase = phase
		if done > 0 || total > 0 {
			j.Progress = &JobProgress{Done: done, Total: total}
		}
	}
	return j
}

// ListJobs returns all generate_report and playwright_ingest jobs known to River, newest first (capped at 200).
func (jm *RiverJobManager) ListJobs(ctx context.Context) []*Job {
	params := river.NewJobListParams().
		Kinds("generate_report", "playwright_ingest", "parse_staged_targz").
		First(200)

	res, err := jm.client.JobList(ctx, params)
	if err != nil {
		jm.logger.Error("river job list failed", zap.Error(err))
		return []*Job{}
	}

	// Collect all River IDs for a single batch progress query (avoids N+1).
	ids := make([]int64, 0, len(res.Jobs))
	for _, r := range res.Jobs {
		ids = append(ids, r.ID)
	}
	progressByID := jm.readJobProgressBatch(ctx, ids)

	jobs := make([]*Job, 0, len(res.Jobs))
	for _, r := range res.Jobs {
		j := riverRowToJob(r)
		if p, ok := progressByID[r.ID]; ok {
			j.Phase = p.phase
			if p.done > 0 || p.total > 0 {
				j.Progress = &JobProgress{Done: p.done, Total: p.total}
			}
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// Delete removes a terminal job (completed, failed, or cancelled) from River.
// Returns ErrJobNotFound if the job does not exist,
// or ErrJobNotTerminal if the job is still active.
func (jm *RiverJobManager) Delete(ctx context.Context, jobID string) error {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}

	row, err := jm.client.JobGet(ctx, id)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}

	status := riverStateToJobStatus(row.State)
	if status != JobStatusCompleted && status != JobStatusFailed && status != JobStatusCancelled {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotTerminal)
	}

	if _, err := jm.client.JobDelete(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
		}
		return fmt.Errorf("delete job %q: %w", jobID, err)
	}
	// Best-effort progress cleanup; failures are logged inside the helper.
	jm.deleteJobProgress(ctx, id)
	return nil
}

// Retry makes a failed or discarded River job immediately available for
// re-execution. Returns ErrJobNotFound if no job with that ID exists.
func (jm *RiverJobManager) Retry(ctx context.Context, jobID string) error {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}
	_, err = jm.client.JobRetry(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
		}
		return fmt.Errorf("retry job %q: %w", jobID, err)
	}
	return nil
}

// Cancel cancels the River job with the given string ID.
// Returns ErrJobNotFound if the job does not exist.
func (jm *RiverJobManager) Cancel(ctx context.Context, jobID string) error {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}
	_, err = jm.client.JobCancel(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
		}
		return fmt.Errorf("cancel job %q: %w", jobID, err)
	}
	return nil
}

// recordReportID durably persists the report id onto the River job via
// RecordOutput, so the job-status endpoint can surface it from any pod (the
// previous in-memory map was lost across replicas and restarts). A record
// failure is logged but deliberately not fatal: RecordOutput can only fail on a
// JSON-marshal error (impossible for a string) or when called outside a worker
// context, and failing the job here would force a costly re-run of report
// generation. Do not change this to return the error.
func recordReportID(ctx context.Context, logger *zap.Logger, jobID int64, reportID string) {
	if reportID == "" {
		return
	}
	if err := river.RecordOutput(ctx, reportID); err != nil {
		logger.Warn("river: failed to record report id", zap.Int64("job_id", jobID), zap.Error(err))
	}
}

// reportIDFromMetadata extracts the report ID stored by river.RecordOutput
// from River job metadata. The value lives under the "output" key and is a
// JSON-encoded string. Returns "" for any missing or malformed input.
func reportIDFromMetadata(meta []byte) string {
	if len(meta) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(meta, &m) != nil {
		return ""
	}
	raw, ok := m[rivertype.MetadataKeyOutput]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// riverRowToJob converts a rivertype.JobRow to our Job struct.
func riverRowToJob(r *rivertype.JobRow) *Job {
	j := &Job{
		ID:        strconv.FormatInt(r.ID, 10),
		Status:    riverStateToJobStatus(r.State),
		CreatedAt: r.CreatedAt,
	}
	if r.AttemptedAt != nil {
		t := *r.AttemptedAt
		j.StartedAt = &t
	}
	if r.FinalizedAt != nil {
		t := *r.FinalizedAt
		j.CompletedAt = &t
	}
	// Try to decode project_id, slug, and storage_key from any of the
	// known job arg types. The encoded args are JSON-tagged, so the first
	// successful decode with a non-zero ProjectID wins.
	var genArgs GenerateReportArgs
	switch {
	case json.Unmarshal(r.EncodedArgs, &genArgs) == nil && genArgs.ProjectID != 0:
		j.ProjectID = genArgs.ProjectID
		j.Slug = genArgs.Slug
		j.StorageKey = genArgs.StorageKey
	default:
		var stagedArgs ParseStagedTarGzArgs
		if err := json.Unmarshal(r.EncodedArgs, &stagedArgs); err == nil && stagedArgs.ProjectID != 0 {
			j.ProjectID = stagedArgs.ProjectID
			j.Slug = stagedArgs.Slug
			j.StorageKey = stagedArgs.StorageKey
			break
		}
		var pwArgs PlaywrightIngestArgs
		if err := json.Unmarshal(r.EncodedArgs, &pwArgs); err == nil {
			j.ProjectID = pwArgs.ProjectID
			j.Slug = pwArgs.Slug
			j.StorageKey = pwArgs.StorageKey
		}
	}
	// Use the last attempt error message if present.
	if len(r.Errors) > 0 {
		j.Error = r.Errors[len(r.Errors)-1].Error
	}
	j.ReportID = reportIDFromMetadata(r.Metadata)
	return j
}

// riverStateToJobStatus maps River job states to our JobStatus values.
func riverStateToJobStatus(state rivertype.JobState) JobStatus {
	switch state {
	case rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateScheduled:
		return JobStatusPending
	case rivertype.JobStateRunning:
		return JobStatusRunning
	case rivertype.JobStateRetryable:
		return JobStatusRetrying
	case rivertype.JobStateCompleted:
		return JobStatusCompleted
	case rivertype.JobStateCancelled:
		return JobStatusCancelled
	case rivertype.JobStateDiscarded:
		return JobStatusFailed
	default:
		return JobStatusPending
	}
}
