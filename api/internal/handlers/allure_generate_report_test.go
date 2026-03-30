package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockReportGenerator is a test double for the ReportGenerator interface.
type mockReportGenerator struct {
	out string
	err error
}

func (m *mockReportGenerator) GenerateReport(_ context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
	return m.out, m.err
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
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
	// The JSON response must use snake_case field names to match the frontend's
	// JobData type (job_id, project_id, status, created_at, etc.).
	if id, _ := data["job_id"].(string); id == "" {
		t.Errorf("expected non-empty 'job_id' (snake_case) in data, got %v", data)
	}
	if status, _ := data["status"].(string); status == "" {
		t.Errorf("expected non-empty 'status' (snake_case) in data, got %v", data)
	}
	if _, ok := data["project_id"]; !ok {
		t.Errorf("expected 'project_id' (snake_case) in data, got %v", data)
	}
	if _, ok := data["created_at"]; !ok {
		t.Errorf("expected 'created_at' (snake_case) in data, got %v", data)
	}
}

// TestGetJobStatus_Returns404ForUnknownJobID verifies 404 for missing job.
func TestGetJobStatus_Returns404ForUnknownJobID(t *testing.T) {
	projectsDir := t.TempDir()
	gen := &mockReportGenerator{out: "ok"}
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
	h := newTestReportHandlerWithJobManager(t, projectsDir, gen)

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
