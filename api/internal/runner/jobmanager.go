package runner

import (
	"context"
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

	// Acquire semaphore (blocks if pool is full).
	jm.semaphore <- struct{}{}
	defer func() { <-jm.semaphore }()

	// Mark job as running.
	now := time.Now()
	jm.mu.Lock()
	job.Status = JobStatusRunning
	job.StartedAt = &now
	jm.mu.Unlock()

	// Run the generator using the server context (not the HTTP request context).
	out, err := jm.generator.GenerateReport(
		jm.ctx,
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
		job.Status = JobStatusFailed
		job.Error = err.Error()
		jm.logger.Error("async report generation failed",
			zap.String("job_id", job.ID),
			zap.String("project_id", job.ProjectID),
			zap.Error(err),
		)
	} else {
		job.Status = JobStatusCompleted
		job.Output = out
		jm.logger.Info("async report generation completed",
			zap.String("job_id", job.ID),
			zap.String("project_id", job.ProjectID),
		)
	}
	// Remove from projectJobs so new jobs can be submitted for this project.
	delete(jm.projectJobs, job.ProjectID)
	jm.mu.Unlock()
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

// cleanupExpired removes completed/failed jobs older than maxAge from both maps.
func (jm *JobManager) cleanupExpired(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for id, j := range jm.jobs {
		if j.Status != JobStatusCompleted && j.Status != JobStatusFailed {
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
