package runner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// JobManager runs a bounded worker pool for async report generation.
// It is safe for concurrent use.
type JobManager struct {
	generator   ReportGenerator
	semaphore   chan struct{} // buffered channel as semaphore
	mu          sync.RWMutex
	jobs        map[string]*Job
	projectJobs map[string]string // projectID -> jobID for in-progress jobs
	ctx         context.Context
	wg          sync.WaitGroup
	logger      *zap.Logger
}

// NewJobManager creates a new JobManager with the given pool size.
func NewJobManager(generator ReportGenerator, poolSize int, logger *zap.Logger) *JobManager {
	return &JobManager{
		generator:   generator,
		semaphore:   make(chan struct{}, poolSize),
		jobs:        make(map[string]*Job),
		projectJobs: make(map[string]string),
		logger:      logger,
	}
}

// Start stores the server context and starts a background cleanup ticker.
func (jm *JobManager) Start(ctx context.Context) {
	jm.ctx = ctx
	go jm.runCleanupLoop(ctx, 5*time.Minute, time.Hour)
}

// runCleanupLoop periodically removes expired terminal jobs.
func (jm *JobManager) runCleanupLoop(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			jm.cleanupExpired(maxAge)
		case <-ctx.Done():
			return
		}
	}
}

// Submit enqueues a new report generation job for projectID.
// If a pending or running job already exists for that project, it is returned unchanged.
func (jm *JobManager) Submit(projectID string, params JobParams) *Job {
	jm.mu.Lock()

	// Deduplicate: return existing active job for this project.
	if existingID, ok := jm.projectJobs[projectID]; ok {
		if existing, exists := jm.jobs[existingID]; exists {
			cp := copyJob(existing) // copy while holding lock
			jm.mu.Unlock()
			return cp
		}
	}

	job := &Job{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		Params:    params,
	}
	jm.jobs[job.ID] = job
	jm.projectJobs[projectID] = job.ID
	jm.wg.Add(1)
	cp := copyJob(job) // copy before worker can modify job
	jm.mu.Unlock()

	go jm.runWorker(job)

	return cp
}

// runWorker acquires a semaphore slot, runs the generator, and updates job state.
func (jm *JobManager) runWorker(job *Job) {
	defer jm.wg.Done()

	// Create per-job context so Cancel() can stop GenerateReport.
	jobCtx, jobCancel := context.WithCancel(jm.ctx)
	defer jobCancel()

	// Store cancel func under lock so Cancel() can call it.
	jm.mu.Lock()
	job.cancel = jobCancel
	jm.mu.Unlock()

	// Acquire semaphore (blocks if pool is full).
	jm.semaphore <- struct{}{}
	defer func() { <-jm.semaphore }()

	// Check under lock whether the job was cancelled before acquiring the slot.
	now := time.Now()
	jm.mu.Lock()
	if job.Status == JobStatusCancelled {
		if job.CompletedAt == nil {
			job.CompletedAt = &now
		}
		jm.mu.Unlock()
		return
	}

	// Mark job as running.
	job.Status = JobStatusRunning
	job.StartedAt = &now
	jm.mu.Unlock()

	// Run the generator using the per-job context (not the server context).
	out, err := jm.generator.GenerateReport(
		jobCtx,
		job.ProjectID,
		job.Params.ExecName,
		job.Params.ExecFrom,
		job.Params.ExecType,
		job.Params.StoreResults,
		job.Params.CIBranch,
		job.Params.CICommitSHA,
	)

	completedAt := time.Now()
	jm.mu.Lock()
	job.CompletedAt = &completedAt
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Context was cancelled by Cancel() — status is already set; just ensure it's marked.
			job.Status = JobStatusCancelled
			// projectJobs was already cleaned up by Cancel(); do not delete here.
		} else {
			job.Status = JobStatusFailed
			job.Error = err.Error()
			jm.logger.Error("async report generation failed",
				zap.String("job_id", job.ID),
				zap.String("project_id", job.ProjectID),
				zap.Error(err),
			)
			delete(jm.projectJobs, job.ProjectID)
		}
	} else {
		job.Status = JobStatusCompleted
		job.Output = out
		jm.logger.Info("async report generation completed",
			zap.String("job_id", job.ID),
			zap.String("project_id", job.ProjectID),
		)
		delete(jm.projectJobs, job.ProjectID)
	}
	jm.mu.Unlock()
}

// ListJobs returns copies of all known jobs sorted by creation time (newest first).
// The returned slice is never nil.
func (jm *JobManager) ListJobs() []*Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	jobs := make([]*Job, 0, len(jm.jobs))
	for _, j := range jm.jobs {
		jobs = append(jobs, copyJob(j))
	}
	sort.Slice(jobs, func(i, k int) bool {
		return jobs[i].CreatedAt.After(jobs[k].CreatedAt)
	})
	return jobs
}

// Cancel cancels the job with the given ID.
// Returns an error if the job is not found or is already in a terminal state.
func (jm *JobManager) Cancel(jobID string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	j, ok := jm.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %q not found", jobID)
	}
	if j.Status == JobStatusCompleted || j.Status == JobStatusFailed || j.Status == JobStatusCancelled {
		return fmt.Errorf("job %q is already in terminal state %q", jobID, j.Status)
	}

	if j.cancel != nil {
		j.cancel()
	}
	j.Status = JobStatusCancelled
	delete(jm.projectJobs, j.ProjectID)
	return nil
}

// Get returns a copy of the job with the given ID, or nil if not found.
func (jm *JobManager) Get(jobID string) *Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	j, ok := jm.jobs[jobID]
	if !ok {
		return nil
	}
	return copyJob(j)
}

// Shutdown blocks until all in-flight workers complete.
func (jm *JobManager) Shutdown() {
	jm.wg.Wait()
}

// cleanupExpired removes completed, failed, and cancelled jobs older than maxAge.
func (jm *JobManager) cleanupExpired(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for id, j := range jm.jobs {
		if j.Status != JobStatusCompleted && j.Status != JobStatusFailed && j.Status != JobStatusCancelled {
			continue
		}
		if j.CompletedAt != nil && !j.CompletedAt.After(cutoff) {
			delete(jm.jobs, id)
		}
	}
}

// copyJob returns a shallow copy of j so callers cannot mutate internal state.
// Pointer fields (StartedAt, CompletedAt) are copied by value.
func copyJob(j *Job) *Job {
	cp := *j
	if j.StartedAt != nil {
		t := *j.StartedAt
		cp.StartedAt = &t
	}
	if j.CompletedAt != nil {
		t := *j.CompletedAt
		cp.CompletedAt = &t
	}
	return &cp
}
