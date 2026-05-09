package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestSendResults_Async_Returns202 verifies that POST with ?async=true streams
// the body to a staging blob and returns 202 + {job_id, batch_id}.
func TestSendResults_Async_Returns202(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestResultUploadHandler(t, projectsDir)

	// Pre-create a project so resolveProjectID succeeds without
	// force_project_creation. testutil.MemProjectStore exposes CreateProject
	// directly.
	if _, err := mocks.Projects.CreateProject(context.Background(), "asyncproj"); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Build a minimal valid gzip-magic body. The async path peeks the first
	// two bytes; the worker isn't exercised here so the rest can be arbitrary.
	body := append([]byte{0x1f, 0x8b}, make([]byte, 32)...)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/asyncproj/results?async=true", bytes.NewReader(body))
	req.SetPathValue("project_id", "asyncproj")
	req.Header.Set("Content-Type", "application/gzip")

	rec := httptest.NewRecorder()
	h.SendResults(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status: got %d body=%q, want 202", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing 'data' object in response: %v", got)
	}
	if _, ok := data["job_id"]; !ok {
		t.Errorf("missing job_id in response data: %v", data)
	}
	batchID, ok := data["batch_id"].(string)
	if !ok || batchID == "" {
		t.Errorf("missing/empty batch_id in response data: %v", data)
	}

	// Verify the staging blob ended up on disk via the LocalStore — the test
	// harness wires a LocalStore over projectsDir.
	stagingPath := projectsDir + "/staging/" + batchID + ".tar.gz"
	if _, err := os.Stat(stagingPath); err != nil {
		t.Errorf("staging blob not written: %v", err)
	}
}

// TestSendResults_Async_RejectsBadGzipMagic confirms the magic-byte sniff at
// the upload edge rejects non-gzip bodies before any storage write.
func TestSendResults_Async_RejectsBadGzipMagic(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestResultUploadHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), "asyncproj"); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Use a body with a non-gzip prefix.
	body := []byte("PK\x03\x04 zip header not gzip")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/asyncproj/results?async=true", bytes.NewReader(body))
	req.SetPathValue("project_id", "asyncproj")
	req.Header.Set("Content-Type", "application/gzip")

	rec := httptest.NewRecorder()
	h.SendResults(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%q, want 400", rec.Code, rec.Body.String())
	}
}

// TestSendResults_Async_NoOpWhenContentTypeIsJSON verifies that the async flag
// is silently ignored for JSON uploads — the existing sync path handles them.
// This is the regression guard for the back-compat decision in the brief.
func TestSendResults_Async_NoOpWhenContentTypeIsJSON(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestResultUploadHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), "asyncproj"); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	body := `{"results":[{"file_name":"r.json","content_base64":"YQ=="}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/asyncproj/results?async=true", bytes.NewReader([]byte(body)))
	req.SetPathValue("project_id", "asyncproj")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.SendResults(rec, req)

	// Sync JSON path returns 200, not 202.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected sync JSON path (200), got %d body=%q", rec.Code, rec.Body.String())
	}
}
