package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// mockReportGenerator is a test double for runner.ReportGenerator.
type mockReportGenerator struct {
	out string
	err error
}

func (m *mockReportGenerator) GenerateReport(_ context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
	return m.out, m.err
}

// newTestAllureHandlerWithJobManager builds an AllureHandler with a real JobManager
// backed by the provided generator, for handler-level tests.
func newTestAllureHandlerWithJobManager(t *testing.T, projectsDir string, gen runner.ReportGenerator) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir, KeepHistory: false}

	db, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	st := storage.NewLocalStore(cfg)
	buildStore := store.NewBuildStore(db, zap.NewNop())
	lockManager := store.NewLockManager()
	r := runner.NewAllure(cfg, st, buildStore, lockManager, nil, zap.NewNop())

	jm := runner.NewJobManager(gen, 2, zap.NewNop())
	jm.Start(context.Background())
	t.Cleanup(func() { jm.Shutdown() })

	return NewAllureHandler(cfg, r, jm, store.NewProjectStore(db, zap.NewNop()), buildStore, store.NewKnownIssueStore(db), nil, nil, st)
}

func makeGenerateReportReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/projects/"+projectID+"/reports",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	return req
}

func makeGetJobStatusReq(t *testing.T, projectID, jobID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/projects/"+projectID+"/jobs/"+jobID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("job_id", jobID)
	return req
}

// TestGenerateReport_Returns202WithJobID verifies async queuing returns 202 + job_id.
func TestGenerateReport_Returns202WithJobID(t *testing.T) {
	projectsDir := t.TempDir()
	gen := &mockReportGenerator{out: "ok"}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	rr := httptest.NewRecorder()
	h.GenerateReport(rr, makeGenerateReportReq(t, "myproject"))

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object in response")
	}
	jobID, ok := data["job_id"].(string)
	if !ok || jobID == "" {
		t.Errorf("expected non-empty job_id in data, got %v", data)
	}
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if msg, _ := meta["message"].(string); msg != "Report generation queued" {
		t.Errorf("unexpected metadata message: %q", msg)
	}
}

// TestGenerateReport_ReservedProjectID_Returns400 verifies validation rejects reserved names.
func TestGenerateReport_ReservedProjectID_Returns400(t *testing.T) {
	projectsDir := t.TempDir()
	gen := &mockReportGenerator{out: "ok"}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	rr := httptest.NewRecorder()
	h.GenerateReport(rr, makeGenerateReportReq(t, "login"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for reserved project_id, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGenerateReport_InvalidProjectID_Returns400 verifies path traversal is rejected.
func TestGenerateReport_InvalidProjectID_Returns400(t *testing.T) {
	projectsDir := t.TempDir()
	gen := &mockReportGenerator{out: "ok"}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	rr := httptest.NewRecorder()
	h.GenerateReport(rr, makeGenerateReportReq(t, "../evil"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetJobStatus_Returns200ForValidJob verifies 200 with job details.
func TestGetJobStatus_Returns200ForValidJob(t *testing.T) {
	projectsDir := t.TempDir()
	// Use a blocking generator so the job stays in pending/running state.
	blockCh := make(chan struct{})
	defer close(blockCh)
	gen := &blockingMockGenerator{ch: blockCh}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	// First, queue a job.
	genRR := httptest.NewRecorder()
	h.GenerateReport(genRR, makeGenerateReportReq(t, "myproject"))
	if genRR.Code != http.StatusAccepted {
		t.Fatalf("generate: expected 202, got %d", genRR.Code)
	}
	var genResp map[string]any
	if err := json.Unmarshal(genRR.Body.Bytes(), &genResp); err != nil {
		t.Fatal(err)
	}
	jobID := genResp["data"].(map[string]any)["job_id"].(string)

	// Now get its status.
	rr := httptest.NewRecorder()
	h.GetJobStatus(rr, makeGetJobStatusReq(t, "myproject", jobID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object")
	}
	if data["ID"] == "" && data["id"] == "" {
		t.Errorf("expected job ID in data, got %v", data)
	}
}

// TestGetJobStatus_Returns404ForUnknownJobID verifies 404 for missing job.
func TestGetJobStatus_Returns404ForUnknownJobID(t *testing.T) {
	projectsDir := t.TempDir()
	gen := &mockReportGenerator{out: "ok"}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	rr := httptest.NewRecorder()
	h.GetJobStatus(rr, makeGetJobStatusReq(t, "myproject", "nonexistent-job-id"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetJobStatus_Returns404WhenProjectIDMismatch verifies that a job belonging
// to a different project is not returned.
func TestGetJobStatus_Returns404WhenProjectIDMismatch(t *testing.T) {
	projectsDir := t.TempDir()
	blockCh := make(chan struct{})
	defer close(blockCh)
	gen := &blockingMockGenerator{ch: blockCh}
	h := newTestAllureHandlerWithJobManager(t, projectsDir, gen)

	// Queue a job for "project-a".
	genRR := httptest.NewRecorder()
	h.GenerateReport(genRR, makeGenerateReportReq(t, "project-a"))
	if genRR.Code != http.StatusAccepted {
		t.Fatalf("generate: expected 202, got %d", genRR.Code)
	}
	var genResp map[string]any
	if err := json.Unmarshal(genRR.Body.Bytes(), &genResp); err != nil {
		t.Fatal(err)
	}
	jobID := genResp["data"].(map[string]any)["job_id"].(string)

	// Request job status under "project-b" — must return 404.
	rr := httptest.NewRecorder()
	h.GetJobStatus(rr, makeGetJobStatusReq(t, "project-b", jobID))

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for project mismatch, got %d: %s", rr.Code, rr.Body.String())
	}
}

// blockingMockGenerator blocks until ch is closed, used to hold jobs in-flight.
type blockingMockGenerator struct {
	ch chan struct{}
}

func (b *blockingMockGenerator) GenerateReport(_ context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
	<-b.ch
	return "ok", nil
}
