package handlers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestPlaywrightHandler creates a PlaywrightHandler backed by a real local
// store and stateful in-memory project/job stores, suitable for handler tests.
func newTestPlaywrightHandler(t *testing.T, projectsDir string) (*PlaywrightHandler, *testutil.MockStores) {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir, MaxUploadSizeMB: 100}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	jm := runner.NewMemJobManager(nil, 0, logger)
	h := NewPlaywrightHandler(st, mocks.Projects, jm, cfg, logger)
	return h, mocks
}

// makePlaywrightTarGzRequest creates a POST request with Content-Type: application/gzip
// for the playwright upload endpoint.
func makePlaywrightTarGzRequest(t *testing.T, projectID string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/projects/"+projectID+"/playwright",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.Header.Set("Content-Type", "application/gzip")
	return req
}

// TestPlaywrightUpload_Success verifies that a valid tar.gz containing index.html
// and a data/ attachment returns 200 with a job_id.
func TestPlaywrightUpload_Success(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-success"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"index.html":    []byte("<html><body>Playwright Report</body></html>"),
		"data/test.png": []byte("\x89PNG\r\n"),
	})

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.JobID == "" {
		t.Error("expected non-empty job_id in response")
	}

	// Verify index.html was written to the results directory.
	if _, err := os.Stat(filepath.Join(resultsDir, "index.html")); err != nil {
		t.Errorf("index.html not written to results dir: %v", err)
	}
}

// TestPlaywrightUpload_MissingIndex verifies that an archive without index.html
// is rejected with a 400 response.
func TestPlaywrightUpload_MissingIndex(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-no-index"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "results"), 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"data/test.png": []byte("\x89PNG\r\n"),
	})

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_ProjectNotFound verifies that uploading to a non-existent
// project without force_project_creation returns 404.
func TestPlaywrightUpload_ProjectNotFound(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-nonexistent"

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html/>"),
	})

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_AutoCreateProject verifies that force_project_creation=true
// with a parent_id creates the project and registers it in the project store.
func TestPlaywrightUpload_AutoCreateProject(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-autocreate"
	parentID := "pw-parent"

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html><body>Report</body></html>"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	q := req.URL.Query()
	q.Set("force_project_creation", "true")
	q.Set("parent_id", parentID)
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify child project was registered in the project store with parent link.
	exists, err := mocks.Projects.ProjectExists(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error checking project in store: %v", err)
	}
	if !exists {
		t.Error("child project was not registered in project store after force_project_creation=true")
	}

	// Verify parent project was also registered.
	parentExists, err := mocks.Projects.ProjectExists(context.Background(), parentID)
	if err != nil {
		t.Fatalf("unexpected error checking parent project in store: %v", err)
	}
	if !parentExists {
		t.Error("parent project was not registered in project store")
	}
}

// TestPlaywrightUpload_InvalidGzip verifies that non-gzip data is rejected with 400.
func TestPlaywrightUpload_InvalidGzip(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-badgzip"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "results"), 0o755); err != nil {
		t.Fatal(err)
	}

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, []byte("this is not gzip data"))
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_PathTraversal verifies that path traversal entries in the
// archive are rejected with a 400 response.
func TestPlaywrightUpload_PathTraversal(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-traversal"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "results"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []tarEntry{
		{
			Header:  tarHeader("../../etc/passwd", 4),
			Content: []byte("root"),
		},
		{
			Header:  tarHeader("index.html", 7),
			Content: []byte("<html/>"),
		},
	}
	archive := makeTarGzWithOpts(t, entries)

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for path traversal, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_PreservesSubdirectory verifies that data/ subdirectory
// files are written with their relative paths preserved.
func TestPlaywrightUpload_PreservesSubdirectory(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-subdir"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pngContent := []byte("\x89PNG\r\n\x1a\n")
	archive := makeTarGz(t, map[string][]byte{
		"index.html":      []byte("<html/>"),
		"data/screen.png": pngContent,
	})

	h, _ := newTestPlaywrightHandler(t, projectsDir)
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// The local store writes to results/<filename>. For nested paths the filename
	// includes the subdirectory component (data/screen.png).
	got, err := os.ReadFile(filepath.Join(resultsDir, "data", "screen.png"))
	if err != nil {
		t.Fatalf("data/screen.png not written to results dir: %v", err)
	}
	if !bytes.Equal(got, pngContent) {
		t.Errorf("data/screen.png content mismatch: got %q, want %q", got, pngContent)
	}
}

// tarHeader is a helper that builds a tar.Header for a regular file.
func tarHeader(name string, size int64) tar.Header {
	return tar.Header{
		Name:     name,
		Size:     size,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}
}
