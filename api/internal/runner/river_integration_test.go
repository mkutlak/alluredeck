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
type riverMockGen struct{}

func (riverMockGen) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	return "ok", nil
}

// TestRiverJobManager exercises the River-backed job manager against a real
// Postgres (TEST_POSTGRES_URL): submitted jobs are retrievable and listed, and
// unknown or malformed IDs yield nil / ErrJobNotFound (which admin maps to 404).
func TestRiverJobManager(t *testing.T) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping River integration test")
	}
	ctx := context.Background()
	s, err := pg.Open(ctx, &config.Config{DatabaseURL: url, RunMigrations: true})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	logger := zap.NewNop()
	jm, err := runner.NewRiverJobManager(s.Pool(), riverMockGen{}, nil, pg.NewWebhookStore(s, nil, logger),
		pg.NewBuildStore(s, logger), nil, nil, nil, nil, nil, "", 1, 5*time.Minute, logger)
	if err != nil {
		t.Fatalf("NewRiverJobManager: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	jm.Start(runCtx)
	t.Cleanup(jm.Shutdown)

	before := len(jm.ListJobs(ctx))
	job := jm.Submit(ctx, int64(1), "river-test-project", runner.JobParams{ExecName: "CI"})
	if job == nil || job.ID == "" || job.ProjectID != 1 {
		t.Fatalf("Submit = %+v, want a job with an ID for project 1", job)
	}
	if got := jm.Get(ctx, job.ID); got == nil || got.ID != job.ID {
		t.Errorf("Get(%q) = %+v, want the submitted job", job.ID, got)
	}
	jm.Submit(ctx, int64(2), "river-list-proj", runner.JobParams{})

	// Jobs are inserted asynchronously; poll until both appear.
	deadline := time.Now().Add(5 * time.Second)
	for len(jm.ListJobs(ctx)) < before+2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(jm.ListJobs(ctx)); n < before+2 {
		t.Errorf("ListJobs: got %d jobs, want at least %d", n, before+2)
	}

	for _, id := range []string{"99999999999", "not-a-number"} {
		if got := jm.Get(ctx, id); got != nil {
			t.Errorf("Get(%q) = %+v, want nil", id, got)
		}
	}
	if err := jm.Cancel(ctx, "99999999998"); !errors.Is(err, runner.ErrJobNotFound) {
		t.Errorf("Cancel(unknown) error = %v, want one wrapping ErrJobNotFound", err)
	}
}
