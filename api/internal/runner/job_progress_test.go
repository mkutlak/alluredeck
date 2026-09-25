package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// gatedProgressGenerator is a ReportGenerator fake that publishes each step's
// phase/progress through the captured reporter, then blocks on the step's gate
// until the test releases it, so mid-job state can be inspected without sleeps.
// It opts in to MemJobManager's progressReceiver path.
type gatedProgressGenerator struct {
	reporter JobProgressReporter
	steps    []gatedStep
	out      string
	err      error
}

type gatedStep struct {
	phase       JobPhase
	done, total int
	gate        chan struct{}
}

func (g *gatedProgressGenerator) SetProgressReporter(r JobProgressReporter) { g.reporter = r }

func (g *gatedProgressGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	for _, s := range g.steps {
		g.reporter(s.phase, s.done, s.total)
		<-s.gate
	}
	return g.out, g.err
}

// waitForPhase polls the in-memory job until its phase matches want or the
// deadline expires.
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

// TestMemJobManager_PhaseAndProgress verifies MemJobManager surfaces every
// phase/progress update through Get() while the job runs — including the
// preparing_local and publishing_report file counts, whose callbacks were once
// never wired — drops progress back to nil when both counters are zero, and
// records the terminal status, phase, report id and error.
func TestMemJobManager_PhaseAndProgress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		steps     []gatedStep
		err       error
		wantPhase JobPhase
		wantState JobStatus
	}{
		{
			name: "progress surfaced per phase, then completed",
			steps: []gatedStep{
				{JobPhasePreparingLocal, 7500, 15919, nil},
				{JobPhaseGeneratingReport, 0, 0, nil},
				{JobPhasePublishingReport, 320, 1024, nil},
			},
			wantPhase: JobPhaseCompleted, wantState: JobStatusCompleted,
		},
		{
			name:  "generator error fails the job",
			steps: []gatedStep{{JobPhasePreparingLocal, 100, 5000, nil}},
			err:   errors.New("boom"), wantPhase: JobPhaseFailed, wantState: JobStatusFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i := range tc.steps {
				tc.steps[i].gate = make(chan struct{})
			}
			mgr := NewMemJobManager(&gatedProgressGenerator{steps: tc.steps, out: "42", err: tc.err}, 1, nil)
			ctx := t.Context()
			mgr.Start(ctx)
			defer mgr.Shutdown()
			released := 0 // release any still-blocked step on early exit so Shutdown can drain
			defer func() {
				for _, s := range tc.steps[released:] {
					close(s.gate)
				}
			}()
			job := mgr.Submit(ctx, 7, "demo", JobParams{StorageKey: "demo"})

			for _, s := range tc.steps {
				j := waitForPhase(t, mgr, job.ID, s.phase, 2*time.Second)
				switch {
				case s.done == 0 && s.total == 0:
					if j.Progress != nil {
						t.Errorf("%s: progress should be nil when both counters are zero, got %+v", s.phase, j.Progress)
					}
				case j.Progress == nil || j.Progress.Done != s.done || j.Progress.Total != s.total:
					t.Errorf("%s: progress = %+v, want %d/%d", s.phase, j.Progress, s.done, s.total)
				}
				close(s.gate)
				released++
			}

			final := waitForPhase(t, mgr, job.ID, tc.wantPhase, 2*time.Second)
			if final.Status != tc.wantState {
				t.Fatalf("status: got %q, want %q", final.Status, tc.wantState)
			}
			if tc.err == nil && final.ReportID != "42" {
				t.Errorf("report id: got %q, want %q", final.ReportID, "42")
			}
			if tc.err != nil && final.Error != tc.err.Error() {
				t.Errorf("error: got %q, want %q", final.Error, tc.err.Error())
			}
		})
	}
}
