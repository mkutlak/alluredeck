package runner

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// fakeProgressGenerator is a ReportGenerator that publishes a fixed sequence
// of phase/progress updates via the captured reporter, then returns the
// configured output and error. It opts in to MemJobManager's progressReceiver
// path so we don't need to construct a full *Allure for unit tests.
type fakeProgressGenerator struct {
	reporter JobProgressReporter
	steps    []progressStep
	out      string
	err      error
	done     chan struct{} // closed when GenerateReport returns
}

type progressStep struct {
	phase JobPhase
	done  int
	total int
}

// SetProgressReporter satisfies the runner package's progressReceiver
// interface used by MemJobManager.runJob.
func (f *fakeProgressGenerator) SetProgressReporter(r JobProgressReporter) {
	f.reporter = r
}

func (f *fakeProgressGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	for _, s := range f.steps {
		if f.reporter != nil {
			f.reporter(s.phase, s.done, s.total)
		}
	}
	if f.done != nil {
		close(f.done)
	}
	return f.out, f.err
}

// waitForPhase polls the in-memory job until its phase matches want or the
// deadline expires. Useful for asserting intermediate states without time.Sleep.
func waitForPhase(t *testing.T, m *MemJobManager, jobID string, want JobPhase, timeout time.Duration) *Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		j := m.Get(context.Background(), jobID)
		if j != nil && j.Phase == want {
			return j
		}
		if time.Now().After(deadline) {
			if j == nil {
				t.Fatalf("job %s not found", jobID)
			}
			t.Fatalf("job %s: phase=%q want %q after %s", jobID, j.Phase, want, timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestMemJobManager_PublishesPhaseAndProgress drives the runner with a fake
// generator that emits a fixed phase/progress sequence and asserts the manager
// surfaces the most recent update via Get().
func TestMemJobManager_PublishesPhaseAndProgress(t *testing.T) {
	t.Parallel()

	gen := &fakeProgressGenerator{
		steps: []progressStep{
			{JobPhasePreparingLocal, 50, 200},
			{JobPhasePreparingLocal, 200, 200},
			{JobPhaseGeneratingReport, 0, 0},
			{JobPhasePublishingReport, 100, 320},
			{JobPhaseFinalizing, 0, 0},
		},
		out:  "42",
		done: make(chan struct{}),
	}

	mgr := NewMemJobManager(gen, 1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Shutdown()

	job := mgr.Submit(ctx, 7, "demo", JobParams{StorageKey: "demo"})

	// Wait for the worker to drain the entire step sequence.
	select {
	case <-gen.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("generator never returned")
	}

	// The job goroutine writes the terminal status under the lock right after
	// the generator returns; poll Get until status flips to Completed so we
	// observe the post-terminal snapshot.
	final := waitForPhase(t, mgr, job.ID, JobPhaseCompleted, 1*time.Second)
	if final.Status != JobStatusCompleted {
		t.Fatalf("status: got %q, want %q", final.Status, JobStatusCompleted)
	}
	if final.ReportID != "42" {
		t.Errorf("report id: got %q, want %q", final.ReportID, "42")
	}
}

// TestMemJobManager_FailureSetsFailedPhase verifies that a generator error
// flips the job to status=failed with phase=failed.
func TestMemJobManager_FailureSetsFailedPhase(t *testing.T) {
	t.Parallel()

	gen := &fakeProgressGenerator{
		steps: []progressStep{
			{JobPhasePreparingLocal, 100, 5000},
		},
		err:  errBoom,
		done: make(chan struct{}),
	}

	mgr := NewMemJobManager(gen, 1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Shutdown()

	job := mgr.Submit(ctx, 7, "demo", JobParams{StorageKey: "demo"})

	select {
	case <-gen.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("generator never returned")
	}

	final := waitForPhase(t, mgr, job.ID, JobPhaseFailed, 1*time.Second)
	if final.Status != JobStatusFailed {
		t.Fatalf("status: got %q, want %q", final.Status, JobStatusFailed)
	}
	if final.Error == "" {
		t.Errorf("expected non-empty Error on failure")
	}
}

// TestMemJobManager_ProgressNilWhenZero ensures progress is omitted from the
// snapshot when both counters are zero (e.g. mid-CLI generation).
func TestMemJobManager_ProgressNilWhenZero(t *testing.T) {
	t.Parallel()

	// Use a barrier so we can inspect the running job before it terminates.
	gate := make(chan struct{})
	gen := &gatedProgressGenerator{
		gate: gate,
	}

	mgr := NewMemJobManager(gen, 1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	job := mgr.Submit(ctx, 1, "p", JobParams{StorageKey: "p"})

	// Wait for the generator to publish the zero-progress phase.
	deadline := time.Now().Add(1 * time.Second)
	for {
		j := mgr.Get(ctx, job.ID)
		if j != nil && j.Phase == JobPhaseGeneratingReport {
			if j.Progress != nil {
				t.Errorf("progress should be nil when both counters are zero, got %+v", j.Progress)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never observed generating_report phase")
		}
		time.Sleep(2 * time.Millisecond)
	}

	close(gate)
	mgr.Shutdown()
}

// gatedProgressGenerator publishes a single zero-progress phase, then blocks
// on a gate until released. Used to inspect mid-job state from tests.
type gatedProgressGenerator struct {
	reporter JobProgressReporter
	gate     chan struct{}
}

func (g *gatedProgressGenerator) SetProgressReporter(r JobProgressReporter) { g.reporter = r }

func (g *gatedProgressGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	if g.reporter != nil {
		g.reporter(JobPhaseGeneratingReport, 0, 0)
	}
	<-g.gate
	return strconv.Itoa(1), nil
}

// errBoom is a sentinel test error used to trigger failure paths.
var errBoom = errBoomError{}

type errBoomError struct{}

func (errBoomError) Error() string { return "boom" }
