package runner

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// MemJobManager is a simple in-process goroutine-based job manager.
// It implements JobQueuer and is intended for use in tests.
type MemJobManager struct {
	gen      ReportGenerator
	poolSize int

	mu      sync.Mutex
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc

	workCh chan *Job
	wg     sync.WaitGroup
}

// NewMemJobManager creates a new in-process job manager backed by the given generator.
func NewMemJobManager(gen ReportGenerator, poolSize int, _ any) *MemJobManager {
	return &MemJobManager{
		gen:      gen,
		poolSize: poolSize,
		jobs:     make(map[string]*Job),
		cancels:  make(map[string]context.CancelFunc),
		workCh:   make(chan *Job, 100),
	}
}

// Start launches poolSize worker goroutines.
func (m *MemJobManager) Start(ctx context.Context) {
	for range m.poolSize {
		m.wg.Add(1)
		go m.runWorker(ctx)
	}
}

// Shutdown waits for all in-flight jobs to complete.
func (m *MemJobManager) Shutdown() {
	close(m.workCh)
	m.wg.Wait()
}

// Submit enqueues a new job and returns it immediately with Pending status.
// If ctx is cancelled before the job can be enqueued, it returns a failed job.
func (m *MemJobManager) Submit(ctx context.Context, projectID int64, slug string, params JobParams) *Job {
	j := &Job{
		ID:         newMemJobID(),
		ProjectID:  projectID,
		Slug:       slug,
		StorageKey: params.StorageKey,
		Status:     JobStatusPending,
		CreatedAt:  time.Now(),
		Params:     params,
	}

	m.mu.Lock()
	m.jobs[j.ID] = j
	m.mu.Unlock()

	select {
	case m.workCh <- j:
	case <-ctx.Done():
		m.mu.Lock()
		j.Status = JobStatusFailed
		j.Error = ctx.Err().Error()
		m.mu.Unlock()
	}
	return j
}

// SubmitStagedTarGz records a staged tar.gz job. MemJobManager does not
// actually exercise the staged extraction logic; it returns an immediately-
// completed job suitable for handler-level tests. End-to-end staged worker
// tests use a River-backed manager (or call ParseStagedTarGzWorker.Work
// directly).
func (m *MemJobManager) SubmitStagedTarGz(_ context.Context, projectID int64, slug string, params StagedTarGzParams) *Job {
	now := time.Now()
	j := &Job{
		ID:          newMemJobID(),
		ProjectID:   projectID,
		Slug:        slug,
		StorageKey:  params.StorageKey,
		Status:      JobStatusCompleted,
		CreatedAt:   now,
		CompletedAt: &now,
		Params: JobParams{
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
		},
	}
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.mu.Unlock()
	return j
}

// SubmitPlaywright enqueues a new Playwright ingestion job.
// MemJobManager does not execute Playwright ingestion; it records the job as completed immediately.
func (m *MemJobManager) SubmitPlaywright(_ context.Context, projectID int64, slug, storageKey string, execName, execFrom, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) *Job {
	now := time.Now()
	j := &Job{
		ID:          newMemJobID(),
		ProjectID:   projectID,
		Slug:        slug,
		StorageKey:  storageKey,
		Status:      JobStatusCompleted,
		CreatedAt:   now,
		CompletedAt: &now,
		Params: JobParams{
			StorageKey:    storageKey,
			ExecName:      execName,
			ExecFrom:      execFrom,
			CIBranch:      ciBranch,
			CICommitSHA:   ciCommitSHA,
			CIPipelineID:  ciPipelineID,
			CIPipelineURL: ciPipelineURL,
		},
	}
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.mu.Unlock()
	return j
}

// ListJobs returns a snapshot of all known jobs.
// Each element is a deep copy so callers can safely read fields without holding the lock.
func (m *MemJobManager) ListJobs(_ context.Context) []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, copyJob(j))
	}
	return jobs
}

// Cancel cancels a pending or running job.
func (m *MemJobManager) Cancel(_ context.Context, jobID string) error {
	m.mu.Lock()
	j, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}
	status := j.Status
	cancel := m.cancels[jobID]
	m.mu.Unlock()

	if status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCancelled {
		return fmt.Errorf("job %q is already in terminal state", jobID)
	}

	if cancel != nil {
		cancel()
	}

	m.mu.Lock()
	j.Status = JobStatusCancelled
	m.mu.Unlock()
	return nil
}

// Delete removes a terminal job (completed, failed, or cancelled) by ID.
// Returns an error ending with "not found" if the job does not exist,
// or ending with "not in a terminal state" if the job is still active.
func (m *MemJobManager) Delete(_ context.Context, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotFound)
	}
	if j.Status != JobStatusCompleted && j.Status != JobStatusFailed && j.Status != JobStatusCancelled {
		return fmt.Errorf("job %q: %w", jobID, ErrJobNotTerminal)
	}
	delete(m.jobs, jobID)
	return nil
}

// Get returns a snapshot of a job by ID, or nil if not found.
// The returned value is a deep copy so callers can safely read fields without holding the lock.
func (m *MemJobManager) Get(_ context.Context, jobID string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[jobID]
	if !ok {
		return nil
	}
	return copyJob(j)
}

// copyJob returns a deep copy of j safe to expose to readers outside the lock.
// Pointer fields (StartedAt, CompletedAt, Progress) are duplicated so that
// subsequent mutations to the original do not race with readers.
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
	if j.Progress != nil {
		p := *j.Progress
		cp.Progress = &p
	}
	return &cp
}

func (m *MemJobManager) runWorker(ctx context.Context) {
	defer m.wg.Done()
	for j := range m.workCh {
		m.runJob(ctx, j)
	}
}

func (m *MemJobManager) runJob(parentCtx context.Context, j *Job) {
	jobCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	m.mu.Lock()
	m.cancels[j.ID] = cancel
	now := time.Now()
	j.Status = JobStatusRunning
	j.Phase = JobPhasePending
	j.StartedAt = &now
	m.mu.Unlock()

	// Build a per-job reporter that writes phase/progress under m.mu so concurrent
	// Get/ListJobs see consistent snapshots.
	reporter := func(phase JobPhase, done, total int) {
		m.mu.Lock()
		j.Phase = phase
		if total > 0 || done > 0 {
			j.Progress = &JobProgress{Done: done, Total: total}
		} else {
			j.Progress = nil
		}
		m.mu.Unlock()
	}

	gen := m.gen
	if pr, ok := m.gen.(progressAwareGenerator); ok {
		gen = pr.WithProgressReporter(reporter)
	} else if rec, ok := m.gen.(progressReceiver); ok {
		// Test fakes that capture the reporter without copying state implement
		// SetProgressReporter directly.
		rec.SetProgressReporter(reporter)
	}

	p := j.Params
	output, err := gen.GenerateReport(jobCtx,
		j.ProjectID, j.Slug, j.StorageKey, p.BatchID, p.ExecName, p.ExecFrom, p.ExecType, p.StoreResults, p.CIBranch, p.CICommitSHA, p.CIPipelineID, p.CIPipelineURL)

	completed := time.Now()
	m.mu.Lock()
	j.CompletedAt = &completed
	if j.Status != JobStatusCancelled {
		if err != nil {
			j.Status = JobStatusFailed
			j.Phase = JobPhaseFailed
			j.Error = err.Error()
		} else {
			j.Status = JobStatusCompleted
			j.Phase = JobPhaseCompleted
			j.ReportID = output
		}
	}
	delete(m.cancels, j.ID)
	m.mu.Unlock()
}

// progressAwareGenerator is the interface satisfied by ReportGenerator
// implementations that can produce a derived runner bound to a per-job
// progress reporter. *Allure satisfies it.
type progressAwareGenerator interface {
	ReportGenerator
	WithProgressReporter(JobProgressReporter) *Allure
}

// progressReceiver is an alternative shape used by lightweight test fakes:
// they install the reporter directly on themselves rather than returning a
// derived value. Real production runners should prefer progressAwareGenerator.
type progressReceiver interface {
	SetProgressReporter(JobProgressReporter)
}

// newMemJobID generates a random hex string suitable as a job ID.
func newMemJobID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
