package runner_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// riverMockGen is a simple ReportGenerator for integration tests.
type riverMockGen struct {
	out string
}

func (m *riverMockGen) GenerateReport(_ context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
	return m.out, nil
}

// openRiverTestStore opens a PGStore (including River migrations) using TEST_POSTGRES_URL.
// Skips the test if the env var is not set.
func openRiverTestStore(t *testing.T) *pg.PGStore {
	t.Helper()
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping River integration test")
	}
	s, err := pg.Open(context.Background(), &config.Config{DatabaseURL: url})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newTestRiverJM creates a started RiverJobManager backed by the given store.
func newTestRiverJM(t *testing.T, s *pg.PGStore, gen runner.ReportGenerator) *runner.RiverJobManager {
	t.Helper()
	logger := zap.NewNop()
	jm, err := runner.NewRiverJobManager(s.Pool(), gen, 1, logger)
	if err != nil {
		t.Fatalf("NewRiverJobManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jm.Start(ctx)
	t.Cleanup(func() { jm.Shutdown() })
	return jm
}

// TestRiverJobManager_SubmitAndGet verifies that a submitted job can be
// retrieved by the ID returned from Submit.
func TestRiverJobManager_SubmitAndGet(t *testing.T) {
	s := openRiverTestStore(t)
	jm := newTestRiverJM(t, s, &riverMockGen{out: "ok"})

	job := jm.Submit("river-test-project", runner.JobParams{ExecName: "CI"})
	if job == nil {
		t.Fatal("Submit returned nil")
		return // unreachable, but satisfies staticcheck SA5011
	}
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}
	if job.ProjectID != "river-test-project" {
		t.Errorf("ProjectID: got %q, want %q", job.ProjectID, "river-test-project")
	}

	got := jm.Get(job.ID)
	if got == nil {
		t.Fatal("Get returned nil for existing job ID")
		return // unreachable, but satisfies staticcheck SA5011
	}
	if got.ID != job.ID {
		t.Errorf("Get ID mismatch: got %q, want %q", got.ID, job.ID)
	}
}

// TestRiverJobManager_ListJobs verifies that submitted jobs appear in ListJobs.
func TestRiverJobManager_ListJobs(t *testing.T) {
	s := openRiverTestStore(t)
	jm := newTestRiverJM(t, s, &riverMockGen{out: "ok"})

	before := len(jm.ListJobs())

	jm.Submit("river-list-proj-a", runner.JobParams{})
	jm.Submit("river-list-proj-b", runner.JobParams{})

	// Poll until both jobs appear in River's DB (inserted asynchronously).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(jm.ListJobs()) >= before+2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	jobs := jm.ListJobs()
	if len(jobs) < before+2 {
		t.Errorf("expected at least %d jobs, got %d", before+2, len(jobs))
	}
}

// TestRiverJobManager_GetUnknownID returns nil for an unknown ID.
func TestRiverJobManager_GetUnknownID(t *testing.T) {
	s := openRiverTestStore(t)
	jm := newTestRiverJM(t, s, &riverMockGen{out: "ok"})

	if got := jm.Get("99999999999"); got != nil {
		t.Errorf("expected nil for unknown ID, got %+v", got)
	}
	if got := jm.Get("not-a-number"); got != nil {
		t.Errorf("expected nil for non-numeric ID, got %+v", got)
	}
}

// TestRiverJobManager_CancelUnknownID returns a "not found" error.
func TestRiverJobManager_CancelUnknownID(t *testing.T) {
	s := openRiverTestStore(t)
	jm := newTestRiverJM(t, s, &riverMockGen{out: "ok"})

	err := jm.Cancel("99999999998")
	if err == nil {
		t.Fatal("expected error cancelling unknown job, got nil")
	}
	// Error must wrap ErrJobNotFound — required by admin to return 404.
	if !errors.Is(err, runner.ErrJobNotFound) {
		t.Errorf("expected error wrapping ErrJobNotFound, got: %q", err)
	}
}
