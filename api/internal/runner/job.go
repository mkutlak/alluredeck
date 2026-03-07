package runner

import (
	"context"
	"time"
)

// JobStatus represents the current state of an async report generation job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// JobParams holds the parameters for a report generation job.
type JobParams struct {
	ExecName     string
	ExecFrom     string
	ExecType     string
	StoreResults bool
	CIBranch     string
	CICommitSHA  string
}

// Job represents a single async report generation task.
type Job struct {
	ID          string             `json:"job_id"`
	ProjectID   string             `json:"project_id"`
	Status      JobStatus          `json:"status"`
	CreatedAt   time.Time          `json:"created_at"`
	StartedAt   *time.Time         `json:"started_at,omitempty"`
	CompletedAt *time.Time         `json:"completed_at,omitempty"`
	Output      string             `json:"output,omitempty"`
	Error       string             `json:"error,omitempty"`
	Params      JobParams          `json:"-"`
	cancel      context.CancelFunc `json:"-"` //nolint:unused // set by runWorker, called by Cancel
}
