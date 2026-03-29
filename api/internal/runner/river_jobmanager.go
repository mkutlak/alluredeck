package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

// GenerateReportWorker is a River worker that executes Allure report generation.
type GenerateReportWorker struct {
	river.WorkerDefaults[GenerateReportArgs]
	generator    ReportGenerator
	buildStore   store.BuildStorer
	webhookStore store.WebhookStorer
	externalURL  string
	riverClient  *river.Client[pgx.Tx]
	logger       *zap.Logger
}

// Work implements river.Worker.
func (w *GenerateReportWorker) Work(ctx context.Context, job *river.Job[GenerateReportArgs]) error {
	a := job.Args
	if _, err := w.generator.GenerateReport(ctx, a.ProjectID, a.ExecName, a.ExecFrom, a.ExecType, a.StoreResults, a.CIBranch, a.CICommitSHA); err != nil {
		w.logger.Error("river: report generation failed",
			zap.Int64("job_id", job.ID),
			zap.String("project_id", a.ProjectID),
			zap.Error(err),
		)
		return err
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
	// Get active webhooks for this project
	webhooks, err := w.webhookStore.ListActiveForEvent(ctx, projectID, "report_completed")
	if err != nil {
		return fmt.Errorf("list active webhooks: %w", err)
	}
	if len(webhooks) == 0 {
		return nil
	}

	// Get latest build for stats
	build, err := w.buildStore.GetLatestBuild(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get latest build: %w", err)
	}

	// Construct payload
	payload := store.WebhookPayload{
		Event:      "report_completed",
		ProjectID:  projectID,
		BuildOrder: build.BuildOrder,
		Timestamp:  time.Now(),
	}

	// Dashboard URL
	if w.externalURL != "" {
		payload.DashboardURL = w.externalURL + "/projects/" + projectID
	}

	// Stats
	if build.StatTotal != nil && *build.StatTotal > 0 {
		payload.Stats = store.WebhookStats{
			Total:   derefInt(build.StatTotal),
			Passed:  derefInt(build.StatPassed),
			Failed:  derefInt(build.StatFailed),
			Broken:  derefInt(build.StatBroken),
			Skipped: derefInt(build.StatSkipped),
		}
		payload.Stats.PassRate = float64(payload.Stats.Passed) / float64(payload.Stats.Total) * 100
	}

	// Delta vs previous build
	prev, err := w.buildStore.GetPreviousBuild(ctx, projectID, build.BuildOrder)
	if err == nil && prev.StatTotal != nil && *prev.StatTotal > 0 {
		prevPassRate := float64(derefInt(prev.StatPassed)) / float64(derefInt(prev.StatTotal)) * 100
		payload.Delta = &store.WebhookDelta{
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
		payload.CI = &store.WebhookCI{
			Provider:  derefStr(build.CIProvider),
			BuildURL:  derefStr(build.CIBuildURL),
			Branch:    derefStr(build.CIBranch),
			CommitSHA: derefStr(build.CICommitSHA),
		}
	}

	// Enqueue one job per webhook
	for i := range webhooks {
		_, err := w.riverClient.Insert(ctx, SendWebhookArgs{
			WebhookID: webhooks[i].ID,
			Payload:   payload,
		}, &river.InsertOpts{
			Queue:       "webhooks",
			MaxAttempts: 5,
		})
		if err != nil {
			w.logger.Warn("river: failed to enqueue webhook",
				zap.String("webhook_id", webhooks[i].ID), zap.Error(err))
		}
	}

	w.logger.Info("river: enqueued webhook notifications",
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
	client *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
	ctx    context.Context // set by Start
	logger *zap.Logger
}

// compile-time check
var _ JobQueuer = (*RiverJobManager)(nil)

// NewRiverJobManager creates a new RiverJobManager. Call Start to begin processing jobs.
func NewRiverJobManager(pool *pgxpool.Pool, generator ReportGenerator, webhookStore store.WebhookStorer, buildStore store.BuildStorer, encKey []byte, externalURL string, maxWorkers int, logger *zap.Logger) (*RiverJobManager, error) {
	workers := river.NewWorkers()
	reportWorker := &GenerateReportWorker{
		generator:    generator,
		buildStore:   buildStore,
		webhookStore: webhookStore,
		externalURL:  externalURL,
		logger:       logger,
	}
	river.AddWorker(workers, reportWorker)
	river.AddWorker(workers, &SendWebhookWorker{
		webhookStore: webhookStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		encKey:       encKey,
		logger:       logger,
	})

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

	// Wire the client back into the report worker so it can enqueue webhook jobs.
	reportWorker.riverClient = client

	return &RiverJobManager{client: client, pool: pool, logger: logger}, nil
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
	return riverRowToJob(row)
}

// ListJobs returns all generate_report jobs known to River, newest first (capped at 200).
func (jm *RiverJobManager) ListJobs() []*Job {
	params := river.NewJobListParams().
		Kinds("generate_report").
		First(200)

	res, err := jm.client.JobList(jm.ctx, params)
	if err != nil {
		jm.logger.Error("river job list failed", zap.Error(err))
		return []*Job{}
	}

	jobs := make([]*Job, 0, len(res.Jobs))
	for _, r := range res.Jobs {
		jobs = append(jobs, riverRowToJob(r))
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
	// Decode project_id from the encoded job args.
	var args GenerateReportArgs
	if err := json.Unmarshal(r.EncodedArgs, &args); err == nil {
		j.ProjectID = args.ProjectID
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
	case rivertype.JobStateRunning, rivertype.JobStateRetryable:
		return JobStatusRunning
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
