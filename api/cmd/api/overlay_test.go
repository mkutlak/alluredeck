package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// writeTestFile creates parent dirs and writes content to the given path.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// getOverlay sends a GET request to the overlay handler and returns the recorder.
func getOverlay(t *testing.T, handler http.Handler, urlPath string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, urlPath, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// TestOverlay_ServesFromBuildDir verifies that a file present in the numbered
// build directory is served directly without falling back to latest/.
func TestOverlay_ServesFromBuildDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "3", "data", "test.json"),
		`{"ok":true}`)

	h := newOverlayHandler(dir)
	rr := getOverlay(t, h, "/myproject/reports/3/data/test.json")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != `{"ok":true}` {
		t.Errorf("unexpected body: %q", body)
	}
}

// TestOverlay_FallsBackToLatest verifies that when a file is missing from a
// numbered build dir, the overlay serves it from reports/latest/ instead.
func TestOverlay_FallsBackToLatest(t *testing.T) {
	dir := t.TempDir()
	// Build dir 3 exists but has no index.html (partial-copy build).
	if err := os.MkdirAll(filepath.Join(dir, "myproject", "reports", "3"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "latest", "index.html"),
		"<html>latest</html>")

	h := newOverlayHandler(dir)
	rr := getOverlay(t, h, "/myproject/reports/3/index.html")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body)
	}
	if body := rr.Body.String(); body != "<html>latest</html>" {
		t.Errorf("expected latest content, got: %q", body)
	}
}

// TestOverlay_LatestServedDirectly verifies that requests targeting
// reports/latest/ are served directly (no overlay logic applied).
func TestOverlay_LatestServedDirectly(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "latest", "app.js"),
		"js content")

	h := newOverlayHandler(dir)
	rr := getOverlay(t, h, "/myproject/reports/latest/app.js")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "js content" {
		t.Errorf("unexpected body: %q", body)
	}
}

// TestOverlay_NonNumericDir_NoFallback verifies that paths not matching the
// {projectID}/reports/{N}/ pattern never fall back to latest/.
func TestOverlay_NonNumericDir_NoFallback(t *testing.T) {
	dir := t.TempDir()
	// latest/ exists but must not be used as fallback here.
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "latest", "index.html"),
		"<html>latest</html>")

	h := newOverlayHandler(dir)
	// Path goes through "emailable-report-render", not reports/{N}.
	rr := getOverlay(t, h, "/myproject/emailable-report-render/index.html")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// TestOverlay_404_WhenNeitherExists verifies that a 404 is returned when the
// file is absent from both the build dir and latest/.
func TestOverlay_404_WhenNeitherExists(t *testing.T) {
	dir := t.TempDir()
	h := newOverlayHandler(dir)
	rr := getOverlay(t, h, "/myproject/reports/5/index.html")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// stubProjectStorer is a minimal store.ProjectStorer for overlay handler tests.
// Only GetProject and GetProjectBySlugAny are implemented; all other methods
// return store.ErrProjectNotFound so the interface is satisfied without panics.
// ---------------------------------------------------------------------------

type stubProjectStorer struct {
	getByID   func(ctx context.Context, id int64) (*store.Project, error)
	getBySlug func(ctx context.Context, slug string) (*store.Project, error)
}

func (s *stubProjectStorer) GetProject(ctx context.Context, id int64) (*store.Project, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return nil, store.ErrProjectNotFound
}

func (s *stubProjectStorer) GetProjectBySlugAny(ctx context.Context, slug string) (*store.Project, error) {
	if s.getBySlug != nil {
		return s.getBySlug(ctx, slug)
	}
	return nil, store.ErrProjectNotFound
}

func (s *stubProjectStorer) CreateProject(_ context.Context, _ string) (*store.Project, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) CreateProjectWithParent(_ context.Context, _ string, _ int64) (*store.Project, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) GetProjectBySlug(_ context.Context, _ string) (*store.Project, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) ListProjects(_ context.Context) ([]store.Project, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) ListProjectsPaginated(_ context.Context, _, _ int) ([]store.Project, int, error) {
	return nil, 0, store.ErrProjectNotFound
}
func (s *stubProjectStorer) ListProjectsPaginatedTopLevel(_ context.Context, _, _ int) ([]store.Project, int, error) {
	return nil, 0, store.ErrProjectNotFound
}
func (s *stubProjectStorer) ListChildren(_ context.Context, _ int64) ([]store.Project, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) ListChildIDs(_ context.Context, _ int64) ([]string, error) {
	return nil, store.ErrProjectNotFound
}
func (s *stubProjectStorer) HasChildren(_ context.Context, _ int64) (bool, error) {
	return false, store.ErrProjectNotFound
}
func (s *stubProjectStorer) SetParent(_ context.Context, _, _ int64) error {
	return store.ErrProjectNotFound
}
func (s *stubProjectStorer) ClearParent(_ context.Context, _ int64) error {
	return store.ErrProjectNotFound
}
func (s *stubProjectStorer) DeleteProject(_ context.Context, _ int64) error {
	return store.ErrProjectNotFound
}
func (s *stubProjectStorer) RenameProject(_ context.Context, _ int64, _ string) error {
	return store.ErrProjectNotFound
}
func (s *stubProjectStorer) ProjectExists(_ context.Context, _ int64) (bool, error) {
	return false, store.ErrProjectNotFound
}
func (s *stubProjectStorer) SetReportType(_ context.Context, _ int64, _ string) error {
	return store.ErrProjectNotFound
}

var _ store.ProjectStorer = (*stubProjectStorer)(nil)

// ---------------------------------------------------------------------------
// Playwright report handler tests
// ---------------------------------------------------------------------------

// TestPlaywrightReportHandler_SlugResolvesToStorageKey verifies that a slug-based
// URL is correctly resolved to the project's storage_key when reading files.
func TestPlaywrightReportHandler_SlugResolvesToStorageKey(t *testing.T) {
	const (
		slug       = "ui-permissions"
		storageKey = "82"
		reportID   = "1"
		wantBody   = "<html>playwright</html>"
	)

	ps := &stubProjectStorer{
		getBySlug: func(_ context.Context, s string) (*store.Project, error) {
			if s == slug {
				return &store.Project{ID: 82, Slug: slug, StorageKey: storageKey}, nil
			}
			return nil, store.ErrProjectNotFound
		},
	}

	ms := &storage.MockStore{
		ReadPlaywrightFileFn: func(_ context.Context, projectID, subPath string) (io.ReadCloser, string, error) {
			if projectID != storageKey {
				t.Errorf("expected projectID=%q, got %q", storageKey, projectID)
			}
			wantSubPath := reportID + "/index.html"
			if subPath != wantSubPath {
				t.Errorf("expected subPath=%q, got %q", wantSubPath, subPath)
			}
			return io.NopCloser(strings.NewReader(wantBody)), "text/html", nil
		},
	}

	h := newPlaywrightReportHandler(ms, ps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+slug+"/playwright-reports/"+reportID+"/index.html", nil)
	req.SetPathValue("projectID", slug)
	req.SetPathValue("reportID", reportID)
	req.SetPathValue("rest", "index.html")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != wantBody {
		t.Errorf("unexpected body: %q", got)
	}
}

// TestPlaywrightReportHandler_UnknownSlug_404 verifies that an unknown slug
// returns 404 without attempting to read any file.
func TestPlaywrightReportHandler_UnknownSlug_404(t *testing.T) {
	ps := &stubProjectStorer{} // all lookups return ErrProjectNotFound

	ms := &storage.MockStore{
		ReadPlaywrightFileFn: func(_ context.Context, _, _ string) (io.ReadCloser, string, error) {
			t.Error("ReadPlaywrightFile should not be called for unknown slug")
			return nil, "", store.ErrProjectNotFound
		},
	}

	h := newPlaywrightReportHandler(ms, ps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/no-such-slug/playwright-reports/1/index.html", nil)
	req.SetPathValue("projectID", "no-such-slug")
	req.SetPathValue("reportID", "1")
	req.SetPathValue("rest", "index.html")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// Ensure time import is used (it is referenced via store.Project.CreatedAt zero value indirectly,
// but we include it explicitly to avoid an unused-import error — remove if compiler complains).
var _ = time.Time{}
