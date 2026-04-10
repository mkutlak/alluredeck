package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// ---- GetReportCategories ----------------------------------------------------

func TestGetReportCategories_LatestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "catproject"
	widgetsDir := filepath.Join(projectsDir, projectSlug, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catJSON := `[{"name":"Product defects","matchedStatistic":{"failed":3,"broken":0,"known":0,"unknown":0,"total":3}},{"name":"Test defects","matchedStatistic":{"failed":0,"broken":2,"known":0,"unknown":0,"total":2}}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "categories.json"), []byte(catJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	projectSlug := "nocat"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	h, _ := newTestReportHandler(t, projectsDir)

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
	projectSlug := "emptycat"
	widgetsDir := filepath.Join(projectsDir, projectSlug, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "categories.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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

// ---- GetReportEnvironment ---------------------------------------------------

func TestGetReportEnvironment_LatestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "myproject"
	widgetsDir := filepath.Join(projectsDir, projectSlug, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envJSON := `[{"name":"Browser","values":["Chrome 120"]},{"name":"OS","values":["Linux","macOS"]},{"name":"Java","values":["17.0.8"]}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	projectSlug := "proj"
	widgetsDir := filepath.Join(projectsDir, projectSlug, "reports", "5", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envJSON := `[{"name":"Version","values":["1.2.3"]}]`
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(envJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/5/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	projectSlug := "noreports"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	h, _ := newTestReportHandler(t, projectsDir)

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
	projectSlug := "empty"
	widgetsDir := filepath.Join(projectsDir, projectSlug, "reports", "latest", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "environment.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
