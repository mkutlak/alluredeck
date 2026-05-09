package handlers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestPlaywrightHandler creates a PlaywrightHandler backed by a real local
// store and stateful in-memory project/build stores, suitable for handler tests.
func newTestPlaywrightHandler(t *testing.T, projectsDir string) (*PlaywrightHandler, *testutil.MockStores) {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir, MaxUploadSizeMB: 100}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	h := NewPlaywrightHandler(st, mocks.Projects, mocks.Builds, nil, cfg, logger)
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
// and a data/ attachment returns 200 with status "uploaded" and files go to
// playwright-reports/latest/.
func TestPlaywrightUpload_Success(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-success"
	latestDir := filepath.Join(projectsDir, projectID, "playwright-reports", "latest")
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"index.html":    []byte("<html><body>Playwright Report</body></html>"),
		"data/test.png": []byte("\x89PNG\r\n"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Status != "uploaded" {
		t.Errorf("expected status=uploaded in response, got %q", resp.Data.Status)
	}

	// Verify index.html was written to playwright-reports/latest/.
	if _, err := os.Stat(filepath.Join(latestDir, "index.html")); err != nil {
		t.Errorf("index.html not written to playwright-reports/latest/: %v", err)
	}
}

// TestPlaywrightUpload_MissingIndex verifies that an archive without index.html
// is rejected with a 400 response.
func TestPlaywrightUpload_MissingIndex(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-no-index"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "playwright-reports", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"data/test.png": []byte("\x89PNG\r\n"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
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

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html><body>Report</body></html>"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	// Pre-create parent project so we can pass its numeric ID.
	parentProj, err := mocks.Projects.CreateProject(context.Background(), "pw-parent")
	if err != nil {
		t.Fatal(err)
	}
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	req := makePlaywrightTarGzRequest(t, projectID, archive)
	q := req.URL.Query()
	q.Set("force_project_creation", "true")
	q.Set("parent_id", parentIDStr)
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify parent project was registered (ID 1 — created before the handler call).
	parentExists, err := mocks.Projects.ProjectExists(context.Background(), parentProj.ID)
	if err != nil {
		t.Fatalf("unexpected error checking parent project in store: %v", err)
	}
	if !parentExists {
		t.Error("parent project was not registered in project store")
	}

	// Verify child project was registered in the project store with parent link (ID 2).
	childIntID := int64(2)
	exists, err := mocks.Projects.ProjectExists(context.Background(), childIntID)
	if err != nil {
		t.Fatalf("unexpected error checking project in store: %v", err)
	}
	if !exists {
		t.Error("child project was not registered in project store after force_project_creation=true")
	}
}

// TestPlaywrightUpload_InvalidGzip verifies that non-gzip data is rejected with 400.
func TestPlaywrightUpload_InvalidGzip(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-badgzip"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "playwright-reports", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
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
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "playwright-reports", "latest"), 0o755); err != nil {
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

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for path traversal, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_PreservesSubdirectory verifies that data/ subdirectory
// files are written with their relative paths preserved under playwright-reports/latest/.
func TestPlaywrightUpload_PreservesSubdirectory(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-subdir"
	latestDir := filepath.Join(projectsDir, projectID, "playwright-reports", "latest")
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pngContent := []byte("\x89PNG\r\n\x1a\n")
	archive := makeTarGz(t, map[string][]byte{
		"index.html":      []byte("<html/>"),
		"data/screen.png": pngContent,
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	req := makePlaywrightTarGzRequest(t, projectID, archive)
	w := httptest.NewRecorder()

	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Files should be written to playwright-reports/latest/data/screen.png.
	got, err := os.ReadFile(filepath.Join(latestDir, "data", "screen.png"))
	if err != nil {
		t.Fatalf("data/screen.png not written to playwright-reports/latest/: %v", err)
	}
	if !bytes.Equal(got, pngContent) {
		t.Errorf("data/screen.png content mismatch: got %q, want %q", got, pngContent)
	}
}

// TestPlaywrightUpload_WithBuildNumber verifies that when build_number is provided,
// the report is written directly to playwright-reports/{buildNumber}/ and
// has_playwright_report is set. The latest/ directory is not touched.
func TestPlaywrightUpload_WithBuildNumber(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-build-num"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID, "playwright-reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html><body>Direct Report</body></html>"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}

	// Simulate build 5 existing.
	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, _ int64, bn int) (store.Build, error) {
		if bn == 5 {
			return store.Build{ProjectID: 1, BuildNumber: 5}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}

	var setHasPWReportCalled bool
	mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, bn int, value bool) error {
		if bn == 5 && value {
			setHasPWReportCalled = true
		}
		return nil
	}

	req := makePlaywrightTarGzRequest(t, projectID, archive)
	q := req.URL.Query()
	q.Set("build_number", "5")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.UploadReport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the report was written directly to playwright-reports/5/.
	buildDir := filepath.Join(projectsDir, projectID, "playwright-reports", "5")
	if _, err := os.Stat(filepath.Join(buildDir, "index.html")); err != nil {
		t.Errorf("index.html not written to playwright-reports/5/: %v", err)
	}

	// Verify has_playwright_report was set.
	if !setHasPWReportCalled {
		t.Error("SetHasPlaywrightReport was not called with (projectID, 5, true)")
	}

	// Verify latest/ was NOT created.
	latestDir := filepath.Join(projectsDir, projectID, "playwright-reports", "latest")
	if _, err := os.Stat(latestDir); !os.IsNotExist(err) {
		t.Errorf("playwright-reports/latest/ should not exist when build_number is provided")
	}
}

// TestPlaywrightUpload_WithBuildNumber_NotFound verifies that uploading with a
// build_number that doesn't exist returns 404.
func TestPlaywrightUpload_WithBuildNumber_NotFound(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-build-404"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html/>"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, _ int64, _ int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	req := makePlaywrightTarGzRequest(t, projectID, archive)
	q := req.URL.Query()
	q.Set("build_number", "99")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.UploadReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightUpload_WithBuildNumber_Invalid verifies that a non-numeric
// build_number returns 400.
func TestPlaywrightUpload_WithBuildNumber_Invalid(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "pw-build-bad"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	archive := makeTarGz(t, map[string][]byte{
		"index.html": []byte("<html/>"),
	})

	h, mocks := newTestPlaywrightHandler(t, projectsDir)
	if _, err := mocks.Projects.CreateProject(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}

	req := makePlaywrightTarGzRequest(t, projectID, archive)
	q := req.URL.Query()
	q.Set("build_number", "abc")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.UploadReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPlaywrightExtract_DiskSpool verifies that extractPlaywrightArchive correctly
// spools all entries to disk and streams them to storage. It uses a MockStore so
// no real filesystem project layout is required, and includes a ~10 MB file to
// confirm large entries are handled without buffering in memory.
func TestPlaywrightExtract_DiskSpool(t *testing.T) {
	large := make([]byte, 10<<20) // 10 MB
	for i := range large {
		large[i] = byte(i % 251)
	}

	files := map[string][]byte{
		"index.html":          []byte("<html><body>Playwright</body></html>"),
		"data/trace.zip":      large,
		"data/screenshot.png": []byte("\x89PNG\r\n"),
		"data/video.webm":     []byte("WEBM"),
		"assets/app.js":       []byte("console.log('hi')"),
	}
	archive := makeTarGz(t, files)

	// Collect what was written via the mock store.
	type written struct {
		subPath string
		content []byte
	}
	var mu sync.Mutex
	var uploads []written

	mock := &storage.MockStore{
		WritePlaywrightFileFn: func(_ context.Context, _, subPath string, r io.Reader) error {
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			mu.Lock()
			uploads = append(uploads, written{subPath: subPath, content: data})
			mu.Unlock()
			return nil
		},
	}

	cfg := &config.Config{MaxUploadSizeMB: 200, UploadWriteConcurrency: 4}
	h := &PlaywrightHandler{store: mock, cfg: cfg, logger: zap.NewNop()}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/gzip")

	if err := h.extractPlaywrightArchive(req, "proj-key", "latest"); err != nil {
		t.Fatalf("extractPlaywrightArchive: %v", err)
	}

	if len(uploads) != len(files) {
		t.Fatalf("expected %d uploads, got %d", len(files), len(uploads))
	}

	// Build a map for easy lookup.
	got := make(map[string][]byte, len(uploads))
	for _, u := range uploads {
		got[u.subPath] = u.content
	}

	for name, want := range files {
		key := "latest/" + name
		content, ok := got[key]
		if !ok {
			t.Errorf("missing upload for %q", key)
			continue
		}
		if !bytes.Equal(content, want) {
			t.Errorf("content mismatch for %q: got %d bytes, want %d bytes", key, len(content), len(want))
		}
	}
}

// TestPlaywrightExtract_MidBatchFailure verifies that when one WritePlaywrightFile
// call returns an error, extractPlaywrightArchive propagates that error and the
// context passed to remaining goroutines is cancelled so they can detect it.
func TestPlaywrightExtract_MidBatchFailure(t *testing.T) {
	const failTarget = "data/fail-me.bin"

	files := map[string][]byte{
		"index.html":   []byte("<html/>"),
		failTarget:     []byte("bad"),
		"data/ok1.bin": []byte("ok1"),
		"data/ok2.bin": []byte("ok2"),
	}
	archive := makeTarGz(t, files)

	errTarget := fmt.Errorf("injected store error")

	mock := &storage.MockStore{
		WritePlaywrightFileFn: func(ctx context.Context, _, subPath string, r io.Reader) error {
			if strings.HasSuffix(subPath, failTarget) {
				return errTarget
			}
			// Respect context cancellation so goroutines queued after the
			// failure do not silently succeed.
			return ctx.Err()
		},
	}

	cfg := &config.Config{MaxUploadSizeMB: 10, UploadWriteConcurrency: 4}
	h := &PlaywrightHandler{store: mock, cfg: cfg, logger: zap.NewNop()}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/gzip")

	extractErr := h.extractPlaywrightArchive(req, "proj-key", "latest")
	if extractErr == nil {
		t.Fatal("expected an error from extractPlaywrightArchive, got nil")
	}
	// The injected error must be reachable via errors.Is (errgroup returns the
	// first non-nil error from the group).
	if !errors.Is(extractErr, errTarget) {
		t.Errorf("expected injected store error in chain, got: %v", extractErr)
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
