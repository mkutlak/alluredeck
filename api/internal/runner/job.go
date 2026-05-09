package runner

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for job operations.
var (
	ErrJobNotFound    = errors.New("job not found")
	ErrJobNotTerminal = errors.New("job is not in a terminal state")
)

// JobStatus represents the current state of an async report generation job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusRetrying  JobStatus = "retrying"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// JobPhase identifies which step of GenerateReport is currently executing.
// It is a finer-grained signal than JobStatus and is intended for polling
// CI clients that want to surface progress to a user. Phases other than
// "completed" and "failed" only occur while JobStatus is "running".
type JobPhase string

const (
	JobPhasePending          JobPhase = "pending"
	JobPhasePreparingLocal   JobPhase = "preparing_local"
	JobPhaseGeneratingReport JobPhase = "generating_report"
	JobPhasePublishingReport JobPhase = "publishing_report"
	JobPhaseFinalizing       JobPhase = "finalizing"
	JobPhaseCompleted        JobPhase = "completed"
	JobPhaseFailed           JobPhase = "failed"
)

// JobProgress carries an optional per-phase counter.
// Both fields are zero when the active phase has no meaningful per-item
// progress (e.g. running the Allure CLI, finalizing).
type JobProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// JobProgressReporter is invoked by the runner whenever the active phase or
// per-phase counter changes. Implementations must be safe for concurrent
// calls from multiple goroutines and should return promptly — they are
// invoked from inside hot loops (storage download/upload).
type JobProgressReporter func(phase JobPhase, done, total int)

// JobParams holds the parameters for a report generation job.
type JobParams struct {
	StorageKey    string
	BatchID       string
	ExecName      string
	ExecFrom      string
	ExecType      string
	StoreResults  bool
	CIBranch      string
	CICommitSHA   string
	CIPipelineID  string
	CIPipelineURL string
}

// Job represents a single async report generation task.
type Job struct {
	ID          string       `json:"job_id"`
	ProjectID   int64        `json:"project_id"`
	Slug        string       `json:"slug"`
	StorageKey  string       `json:"storage_key"`
	Status      JobStatus    `json:"status"`
	Phase       JobPhase     `json:"phase,omitempty"`
	Progress    *JobProgress `json:"progress,omitempty"`
	ReportID    string       `json:"report_id,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	StartedAt   *time.Time   `json:"started_at,omitempty"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	Output      string       `json:"output,omitempty"`
	Error       string       `json:"error,omitempty"`
	Params      JobParams    `json:"-"`
}

// JobQueuer is the interface for async report generation job queues.
// Implemented by RiverJobManager (PostgreSQL-backed).
type JobQueuer interface {
	Submit(ctx context.Context, projectID int64, slug string, params JobParams) *Job
	SubmitPlaywright(ctx context.Context, projectID int64, slug, storageKey string, execName, execFrom, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) *Job
	ListJobs(ctx context.Context) []*Job
	Cancel(ctx context.Context, jobID string) error
	Delete(ctx context.Context, jobID string) error
	Get(ctx context.Context, jobID string) *Job
	Start(ctx context.Context)
	Shutdown()
}
