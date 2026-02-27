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

func TestGetReportCategories_LatestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "catproject"
	widgetsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catJSON := `[{"name":"Product defects","matchedStatistic":{"failed":3,"broken":0,"known":0,"unknown":0,"total":3}},{"name":"Test defects","matchedStatistic":{"failed":0,"broken":2,"known":0,"unknown":0,"total":2}}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "categories.json"), []byte(catJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/catproject/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportCategories(rr, req)

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
	if len(data) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(data))
	}
}

func TestGetReportCategories_MissingFile(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "nocat"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/nocat/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportCategories(rr, req)

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

func TestGetReportCategories_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "../evil")
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportCategories(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestGetReportCategories_EmptyCategories(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "emptycat"
	widgetsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "categories.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/emptycat/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportCategories(rr, req)

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
