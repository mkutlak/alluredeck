package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ---- DeleteProject ----------------------------------------------------------

func makeDeleteProjectReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodDelete,
		"/projects/"+projectID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	return req
}

func TestDeleteProject_OK(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "myproject"

	h, mocks := newTestProjectHandler(t, projectsDir)

	// Register project in DB to get numeric ID.
	proj, err := mocks.Projects.CreateProject(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	// Create project directory structure using the slug.
	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "results"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if meta["message"] != "Project successfully deleted" {
		t.Errorf("unexpected message: %v", meta["message"])
	}

	// Project directory must be gone.
	if _, err := os.Stat(filepath.Join(projectsDir, slug)); !os.IsNotExist(err) {
		t.Error("project directory should be removed after DeleteProject")
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestProjectHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, "999"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteProject_StaleDBRecord_Cleaned verifies that a project which exists
// in the database but not on the filesystem (half-synced state) is cleaned up
// and a 200 is returned rather than a 404 that leaves the stale DB record.
func TestDeleteProject_StaleDBRecord_Cleaned(t *testing.T) {
	projectsDir := t.TempDir()

	// Project is in DB but NOT on filesystem — the half-synced state.
	h, mocks := newTestProjectHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), "stale-proj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	exists, err := mocks.Projects.ProjectExists(context.Background(), proj.ID)
	if err != nil {
		t.Fatalf("unexpected error checking projectStore: %v", err)
	}
	if exists {
		t.Error("stale DB record was not cleaned up by DeleteProject")
	}
}

func TestDeleteProject_InvalidID(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestProjectHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, "../evil"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---- DeleteReport -----------------------------------------------------------

func makeDeleteReportReq(t *testing.T, projectID, reportID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodDelete,
		"/api/v1/projects/"+projectID+"/reports/"+reportID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", reportID)
	return req
}

func TestDeleteReport_OK(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "myproject"

	h, mocks := newTestReportHandler(t, projectsDir)

	// Register project in DB to get numeric ID.
	proj, err := mocks.Projects.CreateProject(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	// Create project with a numbered report directory using the slug.
	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "reports", "3"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.DeleteReport(rr, makeDeleteReportReq(t, projectID, "3"))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if msg, _ := meta["message"].(string); msg != `Report "3" successfully deleted` {
		t.Errorf("unexpected message: %v", meta["message"])
	}

	// Report directory must be gone.
	if _, err := os.Stat(filepath.Join(projectsDir, slug, "reports", "3")); !os.IsNotExist(err) {
		t.Error("report directory should be removed after DeleteReport")
	}
}

func TestDeleteReport_NotFound(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "myproject"

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	// Create project dir but no report "999".
	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.DeleteReport(rr, makeDeleteReportReq(t, projectID, "999"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteReport_InvalidID(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "myproject"

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.DeleteReport(rr, makeDeleteReportReq(t, projectID, "abc"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteReport_MissingReportID(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "myproject"

	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	if err := os.MkdirAll(filepath.Join(projectsDir, slug, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// No report_id param
	h.DeleteReport(rr, makeDeleteReportReq(t, projectID, ""))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
