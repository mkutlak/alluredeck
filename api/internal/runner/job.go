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
	ID          string     `json:"job_id"`
	ProjectID   int64      `json:"project_id"`
	Slug        string     `json:"slug"`
	StorageKey  string     `json:"storage_key"`
	Status      JobStatus  `json:"status"`
	ReportID    string     `json:"report_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Output      string     `json:"output,omitempty"`
	Error       string     `json:"error,omitempty"`
	Params      JobParams  `json:"-"`
}

// JobQueuer is the interface for async report generation job queues.
// Implemented by RiverJobManager (PostgreSQL-backed).
type JobQueuer interface {
	Submit(projectID int64, slug string, params JobParams) *Job
	SubmitPlaywright(projectID int64, slug, storageKey string, execName, execFrom, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) *Job
	ListJobs() []*Job
	Cancel(jobID string) error
	Delete(jobID string) error
	Get(jobID string) *Job
	Start(ctx context.Context)
	Shutdown()
}
