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
	projectID := "myproject"

	// Create project directory structure.
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "results"), 0o755); err != nil { //nolint:gosec // G301: test fixture
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "reports"), 0o755); err != nil { //nolint:gosec // G301: test fixture
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	meta, ok := resp["meta_data"].(map[string]any)
	if !ok {
		t.Fatal("expected meta_data in response")
	}
	if meta["message"] != "Project successfully deleted" {
		t.Errorf("unexpected message: %v", meta["message"])
	}

	// Project directory must be gone.
	if _, err := os.Stat(filepath.Join(projectsDir, projectID)); !os.IsNotExist(err) {
		t.Error("project directory should be removed after DeleteProject")
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, "ghost"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteProject_InvalidID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.DeleteProject(rr, makeDeleteProjectReq(t, "../evil"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
