package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetReportEnvironment_LatestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "myproject"
	widgetsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envJSON := `[{"name":"Browser","values":["Chrome 120"]},{"name":"OS","values":["Linux","macOS"]},{"name":"Java","values":["17.0.8"]}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproject/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T: %v", resp["data"], resp["data"])
	}
	if len(data) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(data))
	}
}

func TestGetReportEnvironment_SpecificBuild(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj"
	widgetsDir := filepath.Join(projectsDir, projectID, "reports", "5", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envJSON := `[{"name":"Version","values":["1.2.3"]}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj/reports/5/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "5")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(data))
	}
}

func TestGetReportEnvironment_MissingFile(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "noreports"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/noreports/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}

func TestGetReportEnvironment_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "../evil")
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestGetReportEnvironment_EmptyJSON(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "empty"
	widgetsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/empty/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}
