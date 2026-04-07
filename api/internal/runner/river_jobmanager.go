package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// GenerateReportArgs holds the River job arguments for async report generation.
type GenerateReportArgs struct {
	ProjectID    string `json:"project_id"`
	ExecName     string `json:"exec_name"`
	ExecFrom     string `json:"exec_from"`
	ExecType     string `json:"exec_type"`
	StoreResults bool   `json:"store_results"`
	CIBranch     string `json:"ci_branch"`
	CICommitSHA  string `json:"ci_commit_sha"`
}

// Kind returns the River job kind identifier.
func (GenerateReportArgs) Kind() string { return "generate_report" }

// PlaywrightIngestArgs holds the River job arguments for async Playwright report ingestion.
type PlaywrightIngestArgs struct {
	ProjectID   string `json:"project_id"`
	ExecName    string `json:"exec_name"`
	ExecFrom    string `json:"exec_from"`
	CIBranch    string `json:"ci_branch"`
	CICommitSHA string `json:"ci_commit_sha"`
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
	reportIDs    *sync.Map
	logger       *zap.Logger
}

// Work implements river.Worker.
func (w *GenerateReportWorker) Work(ctx context.Context, job *river.Job[GenerateReportArgs]) error {
	a := job.Args
	reportID, err := w.generator.GenerateReport(ctx, a.ProjectID, a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults, a.CIBranch, a.CICommitSHA)
	if err != nil {
		w.logger.Error("river: report generation failed",
			zap.Int64("job_id", job.ID),
			zap.String("project_id", a.ProjectID),
			zap.Error(err),
		)
		return err
	}
	if reportID != "" {
		w.reportIDs.Store(job.ID, reportID)
	}
	w.logger.Info("river: report generation completed",
		zap.Int64("job_id", job.ID),
		zap.String("project_id", a.ProjectID),
	)
	// Fire-and-forget: enqueue webhook notifications for this report.
	if err := w.enqueueWebhooks(ctx, a.ProjectID); err != nil {
		w.logger.Warn("river: failed to enqueue webhook notifications",
			zap.String("project_id", a.ProjectID), zap.Error(err))
	}
	return nil
}

// enqueueWebhooks constructs a WebhookPayload from the latest build and enqueues
// delivery jobs for all active webhooks. Errors are non-fatal.
func (w *GenerateReportWorker) enqueueWebhooks(ctx context.Context, projectID string) error {
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
	reportIDs    *sync.Map
	logger       *zap.Logger
}

// Work implements river.Worker for Playwright report ingestion.
func (w *PlaywrightIngestWorker) Work(ctx context.Context, job *river.Job[PlaywrightIngestArgs]) error {
	a := job.Args
	reportID, err := w.runner.IngestReport(ctx, a.ProjectID, a.ExecName, a.ExecFrom, a.CIBranch, a.CICommitSHA)
	if err != nil {
		w.logger.Error("river: playwright ingest failed",
			zap.Int64("job_id", job.ID),
			zap.String("project_id", a.ProjectID),
			zap.Error(err),
		)
		return err
	}
	if reportID != "" {
		w.reportIDs.Store(job.ID, reportID)
	}
	w.logger.Info("river: playwright ingest completed",
		zap.Int64("job_id", job.ID),
		zap.String("project_id", a.ProjectID),
	)
	// Fire-and-forget: enqueue webhook notifications.
	if err := w.enqueueWebhooks(ctx, a.ProjectID); err != nil {
		w.logger.Warn("river: failed to enqueue webhook notifications",
			zap.String("project_id", a.ProjectID), zap.Error(err))
	}
	return nil
}

func (w *PlaywrightIngestWorker) enqueueWebhooks(ctx context.Context, projectID string) error {
	return enqueueWebhooksForProject(ctx, projectID, w.buildStore, w.webhookStore, w.riverClient, w.externalURL, w.logger)
}

// enqueueWebhooksForProject constructs a WebhookPayload from the latest build
// and enqueues delivery jobs for all active webhooks.
func enqueueWebhooksForProject(ctx context.Context, projectID string, buildStore store.BuildStorer, webhookStore store.WebhookStorer, riverClient *river.Client[pgx.Tx], externalURL string, logger *zap.Logger) error {
	// Get active webhooks for this project
	webhooks, err := webhookStore.ListActiveForEvent(ctx, projectID, "report_completed")
	if err != nil {
		return fmt.Errorf("list active webhooks: %w", err)
	}
	if len(webhooks) == 0 {
		return nil
	}

	// Get latest build for stats
	build, err := buildStore.GetLatestBuild(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get latest build: %w", err)
	}

	// Construct payload
	payload := WebhookPayload{
		Event:       "report_completed",
		ProjectID:   projectID,
		BuildNumber: build.BuildNumber,
		Timestamp:   time.Now(),
	}

	// Dashboard URL — link directly to the report, not just the project.
	if externalURL != "" {
		payload.DashboardURL = externalURL + "/projects/" + projectID + "/reports/" + strconv.Itoa(build.BuildNumber)
	}

	// Stats
	if build.StatTotal != nil && *build.StatTotal > 0 {
		payload.Stats = WebhookStats{
			Total:   derefInt(build.StatTotal),
			Passed:  derefInt(build.StatPassed),
			Failed:  derefInt(build.StatFailed),
			Broken:  derefInt(build.StatBroken),
			Skipped: derefInt(build.StatSkipped),
		}
		payload.Stats.PassRate = float64(payload.Stats.Passed) / float64(payload.Stats.Total) * 100
	}

	// Delta vs previous build
	prev, err := buildStore.GetPreviousBuild(ctx, projectID, build.BuildNumber)
	if err == nil && prev.StatTotal != nil && *prev.StatTotal > 0 {
		prevPassRate := float64(derefInt(prev.StatPassed)) / float64(derefInt(prev.StatTotal)) * 100
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

	// Enqueue one job per webhook
	for i := range webhooks {
		_, err := riverClient.Insert(ctx, SendWebhookArgs{
			WebhookID: webhooks[i].ID,
			Payload:   payload,
		}, &river.InsertOpts{
			Queue:       "webhooks",
			MaxAttempts: 5,
		})
		if err != nil {
			logger.Warn("river: failed to enqueue webhook",
				zap.String("webhook_id", webhooks[i].ID), zap.Error(err))
		}
	}

	logger.Info("river: enqueued webhook notifications",
		zap.String("project_id", projectID),
		zap.Int("count", len(webhooks)))
	return nil
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

// RiverJobManager implements JobQueuer using River backed by PostgreSQL.
// It is safe for concurrent use across multiple instances (pods).
type RiverJobManager struct {
	client    *river.Client[pgx.Tx]
	pool      *pgxpool.Pool
	ctx       context.Context // set by Start
	reportIDs sync.Map        // river job ID (int64) -> report ID (string)
	logger    *zap.Logger
}

// compile-time check
var _ JobQueuer = (*RiverJobManager)(nil)

// NewRiverJobManager creates a new RiverJobManager. Call Start to begin processing jobs.
func NewRiverJobManager(pool *pgxpool.Pool, generator ReportGenerator, pwRunner *PlaywrightRunner, webhookStore store.WebhookStorer, buildStore store.BuildStorer, encKey []byte, externalURL string, maxWorkers int, logger *zap.Logger) (*RiverJobManager, error) {
	jm := &RiverJobManager{pool: pool, logger: logger}

	workers := river.NewWorkers()
	reportWorker := &GenerateReportWorker{
		generator:    generator,
		buildStore:   buildStore,
		webhookStore: webhookStore,
		externalURL:  externalURL,
		reportIDs:    &jm.reportIDs,
		logger:       logger,
	}
	river.AddWorker(workers, reportWorker)
	river.AddWorker(workers, &SendWebhookWorker{
		webhookStore: webhookStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
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
			reportIDs:    &jm.reportIDs,
			logger:       logger,
		}
		river.AddWorker(workers, pwWorker)
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
			"webhooks":         {MaxWorkers: 5},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("create River client: %w", err)
	}

	// Wire the client back into workers so they can enqueue webhook jobs.
	reportWorker.riverClient = client
	if pwWorker != nil {
		pwWorker.riverClient = client
	}

	jm.client = client
	return jm, nil
}

// Start stores the server context and starts the River client.
func (jm *RiverJobManager) Start(ctx context.Context) {
	jm.ctx = ctx
	if err := jm.client.Start(ctx); err != nil {
		jm.logger.Error("river client start failed", zap.Error(err))
	}
}

// Shutdown gracefully stops the River client, waiting for running jobs to complete.
func (jm *RiverJobManager) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := jm.client.Stop(ctx); err != nil {
		jm.logger.Error("river client stop failed", zap.Error(err))
	}
}

// Submit enqueues a new report generation job via River and returns its initial state.
func (jm *RiverJobManager) Submit(projectID string, params JobParams) *Job {
	args := GenerateReportArgs{
		ProjectID:    projectID,
		ExecName:     params.ExecName,
		ExecFrom:     params.ExecFrom,
		ExecType:     params.ExecType,
		StoreResults: params.StoreResults,
		CIBranch:     params.CIBranch,
		CICommitSHA:  params.CICommitSHA,
	}
	res, err := jm.client.Insert(jm.ctx, args, nil)
	if err != nil {
		jm.logger.Error("river insert failed", zap.String("project_id", projectID), zap.Error(err))
		return &Job{
			ProjectID: projectID,
			Status:    JobStatusFailed,
			CreatedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	return riverRowToJob(res.Job)
}

// SubmitPlaywright enqueues a new Playwright report ingestion job.
func (jm *RiverJobManager) SubmitPlaywright(projectID string, execName, execFrom, ciBranch, ciCommitSHA string) *Job {
	args := PlaywrightIngestArgs{
		ProjectID:   projectID,
		ExecName:    execName,
		ExecFrom:    execFrom,
		CIBranch:    ciBranch,
		CICommitSHA: ciCommitSHA,
	}
	res, err := jm.client.Insert(jm.ctx, args, &river.InsertOpts{MaxAttempts: 3})
	if err != nil {
		jm.logger.Error("river insert failed", zap.String("project_id", projectID), zap.Error(err))
		return &Job{
			ProjectID: projectID,
			Status:    JobStatusFailed,
			CreatedAt: time.Now(),
			Error:     err.Error(),
		}
	}
	return riverRowToJob(res.Job)
}

// Get returns the job with the given string ID (River int64 rendered as decimal string), or nil.
func (jm *RiverJobManager) Get(jobID string) *Job {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return nil
	}
	row, err := jm.client.JobGet(jm.ctx, id)
	if err != nil {
		return nil
	}
	j := riverRowToJob(row)
	if v, ok := jm.reportIDs.Load(row.ID); ok {
		j.ReportID = v.(string)
	}
	return j
}

// ListJobs returns all generate_report and playwright_ingest jobs known to River, newest first (capped at 200).
func (jm *RiverJobManager) ListJobs() []*Job {
	params := river.NewJobListParams().
		Kinds("generate_report", "playwright_ingest").
		First(200)

	res, err := jm.client.JobList(jm.ctx, params)
	if err != nil {
		jm.logger.Error("river job list failed", zap.Error(err))
		return []*Job{}
	}

	jobs := make([]*Job, 0, len(res.Jobs))
	for _, r := range res.Jobs {
		j := riverRowToJob(r)
		if v, ok := jm.reportIDs.Load(r.ID); ok {
			j.ReportID = v.(string)
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// Delete removes a terminal job (completed, failed, or cancelled) from River.
// Returns ErrJobNotFound if the job does not exist,
// or ErrJobNotTerminal if the job is still active.
func (jm *RiverJobManager) Delete(jobID string) error {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}

	row, err := jm.client.JobGet(jm.ctx, id)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}

	status := riverStateToJobStatus(row.State)
	if status != JobStatusCompleted && status != JobStatusFailed && status != JobStatusCancelled {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotTerminal)
	}

	if _, err := jm.client.JobDelete(jm.ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
		}
		return fmt.Errorf("delete job %q: %w", jobID, err)
	}
	return nil
}

// Cancel cancels the River job with the given string ID.
// Returns ErrJobNotFound if the job does not exist.
func (jm *RiverJobManager) Cancel(jobID string) error {
	id, err := strconv.ParseInt(jobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}
	_, err = jm.client.JobCancel(context.Background(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
		}
		return fmt.Errorf("cancel job %q: %w", jobID, err)
	}
	return nil
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
	// Try to decode project_id from either job arg type.
	var genArgs GenerateReportArgs
	if err := json.Unmarshal(r.EncodedArgs, &genArgs); err == nil && genArgs.ProjectID != "" {
		j.ProjectID = genArgs.ProjectID
	} else {
		var pwArgs PlaywrightIngestArgs
		if err := json.Unmarshal(r.EncodedArgs, &pwArgs); err == nil {
			j.ProjectID = pwArgs.ProjectID
		}
	}
	// Use the last attempt error message if present.
	if len(r.Errors) > 0 {
		j.Error = r.Errors[len(r.Errors)-1].Error
	}
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
