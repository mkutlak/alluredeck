package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"
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
	generator ReportGenerator
	logger    *zap.Logger
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
	return nil
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
func NewRiverJobManager(pool *pgxpool.Pool, generator ReportGenerator, maxWorkers int, logger *zap.Logger) (*RiverJobManager, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &GenerateReportWorker{generator: generator, logger: logger})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("create River client: %w", err)
	}

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
