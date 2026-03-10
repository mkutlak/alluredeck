package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractProjectID_ValidID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("project_id", "my-project")
	w := httptest.NewRecorder()

	got, ok := h.extractProjectID(w, req)
	if !ok {
		t.Fatalf("extractProjectID returned false, body: %s", w.Body.String())
	}
	if got != "my-project" {
		t.Errorf("projectID = %q, want %q", got, "my-project")
	}
}

func TestExtractProjectID_EmptyDefaultsToDefault(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("project_id", "")
	w := httptest.NewRecorder()

	got, ok := h.extractProjectID(w, req)
	if !ok {
		t.Fatalf("extractProjectID returned false, body: %s", w.Body.String())
	}
	if got != "default" {
		t.Errorf("projectID = %q, want %q", got, "default")
	}
}

func TestExtractProjectID_TraversalRejected(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("project_id", "../evil")
	w := httptest.NewRecorder()

	_, ok := h.extractProjectID(w, req)
	if ok {
		t.Fatal("extractProjectID returned true for traversal attack")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExtractProjectID_InvalidEncoding(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("project_id", "%zz")
	w := httptest.NewRecorder()

	_, ok := h.extractProjectID(w, req)
	if ok {
		t.Fatal("extractProjectID returned true for invalid encoding")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	if msg, _ := meta["message"].(string); msg != "invalid project_id encoding" {
		t.Errorf("message = %q, want %q", msg, "invalid project_id encoding")
	}
}

func TestExtractReportID_ValidNumeric(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("report_id", "42")
	w := httptest.NewRecorder()

	got, ok := extractReportID(w, req)
	if !ok {
		t.Fatalf("extractReportID returned false")
	}
	if got != "42" {
		t.Errorf("reportID = %q, want %q", got, "42")
	}
}

func TestExtractReportID_ValidLatest(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("report_id", "latest")
	w := httptest.NewRecorder()

	got, ok := extractReportID(w, req)
	if !ok {
		t.Fatalf("extractReportID returned false")
	}
	if got != "latest" {
		t.Errorf("reportID = %q, want %q", got, "latest")
	}
}

func TestExtractReportID_TraversalRejected(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("report_id", "../evil")
	w := httptest.NewRecorder()

	_, ok := extractReportID(w, req)
	if ok {
		t.Fatal("extractReportID returned true for traversal attack")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExtractReportID_EmptyRejected(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.SetPathValue("report_id", "")
	w := httptest.NewRecorder()

	_, ok := extractReportID(w, req)
	if ok {
		t.Fatal("extractReportID returned true for empty report_id")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
