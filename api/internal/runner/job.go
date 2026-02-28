package runner

import "time"

// JobStatus represents the current state of an async report generation job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
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
	ID          string
	ProjectID   string
	Status      JobStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Output      string
	Error       string
	Params      JobParams
}
