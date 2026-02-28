package runner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockGenerator provides deterministic control over report generation timing.
// Close ch to unblock GenerateReport; set err to simulate failure.
type mockGenerator struct {
	ch  chan struct{}
	err error
}

func (m *mockGenerator) GenerateReport(ctx context.Context, projectID, execName, execFrom, execType string, storeResults bool, ciBranch, ciCommitSHA string) (string, error) {
	<-m.ch
	return "ok", m.err
}

func newMockGen() *mockGenerator {
	return &mockGenerator{ch: make(chan struct{})}
}

func newTestJobManager(t *testing.T, gen ReportGenerator, poolSize int) *JobManager {
	t.Helper()
	ctx := context.Background()
	jm := NewJobManager(gen, poolSize, zap.NewNop())
	jm.Start(ctx)
	t.Cleanup(func() { jm.Shutdown() })
	return jm
}

// waitForStatus polls until the job reaches the target status or times out.
func waitForStatus(t *testing.T, jm *JobManager, jobID string, want JobStatus, timeout time.Duration) *Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j := jm.Get(jobID)
		if j != nil && j.Status == want {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j := jm.Get(jobID)
	if j != nil {
		t.Fatalf("timed out waiting for status %q; current status: %q", want, j.Status)
	} else {
		t.Fatalf("timed out waiting for status %q; job not found", want)
	}
	return nil
}

// TestSubmit_CreatesPendingJob verifies that submitting a job creates it with pending status.
func TestSubmit_CreatesPendingJob(t *testing.T) {
	gen := newMockGen()
	defer close(gen.ch)

	jm := newTestJobManager(t, gen, 2)

	params := JobParams{ExecName: "CI", StoreResults: true}
	job := jm.Submit("proj1", params)

	if job == nil {
		t.Fatal("Submit returned nil")
	}
	if job.ID == "" {
		t.Error("job ID should not be empty")
	}
	if job.ProjectID != "proj1" {
		t.Errorf("ProjectID: got %q, want %q", job.ProjectID, "proj1")
	}
	// Status may be pending or running depending on goroutine scheduling,
	// but should be one of the active states.
	if job.Status != JobStatusPending && job.Status != JobStatusRunning {
		t.Errorf("expected pending or running, got %q", job.Status)
	}
}

// TestGet_ReturnsJob verifies Get returns the job by ID.
func TestGet_ReturnsJob(t *testing.T) {
	gen := newMockGen()
	defer close(gen.ch)

	jm := newTestJobManager(t, gen, 2)

	job := jm.Submit("proj-get", JobParams{})
	got := jm.Get(job.ID)

	if got == nil {
		t.Fatal("Get returned nil for existing job")
	}
	if got.ID != job.ID {
		t.Errorf("ID: got %q, want %q", got.ID, job.ID)
	}
}

// TestGet_NilForUnknownID verifies Get returns nil for unknown IDs.
func TestGet_NilForUnknownID(t *testing.T) {
	gen := newMockGen()
	defer close(gen.ch)

	jm := newTestJobManager(t, gen, 2)

	if got := jm.Get("does-not-exist"); got != nil {
		t.Errorf("expected nil for unknown ID, got %+v", got)
	}
}

// TestJob_TransitionsToCompleted verifies successful generation sets status=completed.
func TestJob_TransitionsToCompleted(t *testing.T) {
	gen := newMockGen()
	jm := newTestJobManager(t, gen, 2)

	job := jm.Submit("proj-ok", JobParams{})
	close(gen.ch) // unblock the worker

	completed := waitForStatus(t, jm, job.ID, JobStatusCompleted, 2*time.Second)

	if completed.Output != "ok" {
		t.Errorf("Output: got %q, want %q", completed.Output, "ok")
	}
	if completed.Error != "" {
		t.Errorf("Error should be empty, got %q", completed.Error)
	}
	if completed.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	if completed.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

// TestJob_TransitionsToFailed verifies error from generator sets status=failed.
func TestJob_TransitionsToFailed(t *testing.T) {
	gen := newMockGen()
	gen.err = errors.New("allure crashed")
	jm := newTestJobManager(t, gen, 2)

	job := jm.Submit("proj-fail", JobParams{})
	close(gen.ch) // unblock the worker

	failed := waitForStatus(t, jm, job.ID, JobStatusFailed, 2*time.Second)

	if failed.Error != "allure crashed" {
		t.Errorf("Error: got %q, want %q", failed.Error, "allure crashed")
	}
	if failed.Output != "" {
		t.Errorf("Output should be empty on failure, got %q", failed.Output)
	}
	if failed.CompletedAt == nil {
		t.Error("CompletedAt should be set on failure")
	}
}

// TestConcurrencyLimit verifies pool size is respected: 3 jobs with pool=2
// means first 2 go to running, 3rd stays pending until a slot opens.
func TestConcurrencyLimit(t *testing.T) {
	// blockCh blocks all workers until closed. unblockOne releases one worker at a time.
	blockCh := make(chan struct{})
	// running counts how many GenerateReport calls are active simultaneously.
	var (
		mu         sync.Mutex
		maxRunning int
		running    int
	)

	countingGen := &countingMockGenerator{
		blockCh: blockCh,
		onStart: func() {
			mu.Lock()
			running++
			if running > maxRunning {
				maxRunning = running
			}
			mu.Unlock()
		},
		onDone: func() {
			mu.Lock()
			running--
			mu.Unlock()
		},
	}

	jm := NewJobManager(countingGen, 2, zap.NewNop())
	jm.Start(context.Background())
	defer jm.Shutdown()

	j1 := jm.Submit("proj-c1", JobParams{})
	j2 := jm.Submit("proj-c2", JobParams{})

	// Wait for first two to be running.
	waitForStatus(t, jm, j1.ID, JobStatusRunning, 2*time.Second)
	waitForStatus(t, jm, j2.ID, JobStatusRunning, 2*time.Second)

	// Submit third — pool is full, it should not yet be running.
	j3 := jm.Submit("proj-c3", JobParams{})

	// Give goroutine scheduler a moment; j3 must not have acquired a slot yet.
	time.Sleep(50 * time.Millisecond)
	snap3 := jm.Get(j3.ID)
	if snap3 == nil {
		t.Fatal("j3 not found")
	}
	if snap3.Status == JobStatusRunning || snap3.Status == JobStatusCompleted || snap3.Status == JobStatusFailed {
		t.Errorf("j3 should be pending while pool is full, got %q", snap3.Status)
	}

	// Unblock all workers and wait for all jobs to complete.
	close(blockCh)
	waitForStatus(t, jm, j1.ID, JobStatusCompleted, 2*time.Second)
	waitForStatus(t, jm, j2.ID, JobStatusCompleted, 2*time.Second)
	waitForStatus(t, jm, j3.ID, JobStatusCompleted, 2*time.Second)

	// Verify pool limit was never exceeded.
	mu.Lock()
	observed := maxRunning
	mu.Unlock()
	if observed > 2 {
		t.Errorf("pool size 2 exceeded: max concurrent workers was %d", observed)
	}
}

// countingMockGenerator tracks concurrency and blocks until blockCh is closed.
type countingMockGenerator struct {
	blockCh chan struct{}
	onStart func()
	onDone  func()
}

func (c *countingMockGenerator) GenerateReport(ctx context.Context, projectID, execName, execFrom, execType string, storeResults bool, ciBranch, ciCommitSHA string) (string, error) {
	c.onStart()
	<-c.blockCh
	c.onDone()
	return "ok", nil
}

// TestDuplicateSubmit_ReturnsSameJob verifies a second submit for the same
// projectID while a job is pending/running returns the existing job.
func TestDuplicateSubmit_ReturnsSameJob(t *testing.T) {
	gen := newMockGen()
	defer close(gen.ch)

	jm := newTestJobManager(t, gen, 2)

	j1 := jm.Submit("proj-dup", JobParams{ExecName: "first"})
	j2 := jm.Submit("proj-dup", JobParams{ExecName: "second"})

	if j1.ID != j2.ID {
		t.Errorf("expected same job ID; j1=%q j2=%q", j1.ID, j2.ID)
	}
}

// TestDuplicateSubmit_AllowsNewJobAfterCompletion verifies that once a job
// completes, a new job can be submitted for the same projectID.
func TestDuplicateSubmit_AllowsNewJobAfterCompletion(t *testing.T) {
	gen := newMockGen()
	jm := newTestJobManager(t, gen, 2)

	j1 := jm.Submit("proj-seq", JobParams{})
	close(gen.ch) // unblock
	waitForStatus(t, jm, j1.ID, JobStatusCompleted, 2*time.Second)

	// New generator channel for the second job.
	gen2 := newMockGen()
	defer close(gen2.ch)
	// Replace generator — we need a new manager or a way to swap.
	// Instead, verify via the manager's projectJobs map being cleared.
	j2 := jm.Submit("proj-seq", JobParams{})
	if j2.ID == j1.ID {
		t.Error("expected a new job after completion, got the same ID")
	}
}

// TestCleanupExpired removes completed/failed jobs older than maxAge.
func TestCleanupExpired(t *testing.T) {
	gen := newMockGen()
	jm := newTestJobManager(t, gen, 2)

	// Submit and complete a job.
	j := jm.Submit("proj-cleanup", JobParams{})
	close(gen.ch)
	waitForStatus(t, jm, j.ID, JobStatusCompleted, 2*time.Second)

	// Job should still be present.
	if jm.Get(j.ID) == nil {
		t.Fatal("job should still be present before cleanup")
	}

	// Run cleanup with 0 maxAge — should remove all completed jobs.
	jm.cleanupExpired(0)

	if jm.Get(j.ID) != nil {
		t.Error("job should have been removed by cleanupExpired")
	}
}

// TestCleanupExpired_KeepsActiveJobs verifies pending/running jobs are not removed.
func TestCleanupExpired_KeepsActiveJobs(t *testing.T) {
	gen := newMockGen()
	defer close(gen.ch)

	jm := newTestJobManager(t, gen, 2)

	j := jm.Submit("proj-active", JobParams{})

	// Run cleanup — active job must survive.
	jm.cleanupExpired(0)

	if jm.Get(j.ID) == nil {
		t.Error("active job should not be removed by cleanupExpired")
	}
}

// TestShutdown_WaitsForInFlightWorkers verifies Shutdown blocks until workers finish.
func TestShutdown_WaitsForInFlightWorkers(t *testing.T) {
	gen := newMockGen()

	jm := NewJobManager(gen, 2, zap.NewNop())
	jm.Start(context.Background())

	j := jm.Submit("proj-shutdown", JobParams{})

	// Unblock the worker shortly after Shutdown is called.
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		close(gen.ch)
	}()

	jm.Shutdown() // must block until worker completes

	final := jm.Get(j.ID)
	if final == nil {
		t.Fatal("job not found after shutdown")
	}
	if final.Status != JobStatusCompleted && final.Status != JobStatusFailed {
		t.Errorf("expected terminal status after shutdown, got %q", final.Status)
	}
	<-done
}
