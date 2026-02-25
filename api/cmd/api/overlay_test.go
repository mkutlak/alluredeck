package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile creates parent dirs and writes content to the given path.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: test helper needs readable temp directory
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test helper uses standard permissions
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
	if err := os.MkdirAll(filepath.Join(dir, "myproject", "reports", "3"), 0o755); err != nil { //nolint:gosec // G301: test setup needs readable temp directory
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

// TestOverlay_BackwardCompat_FullCopyBuild verifies that old builds that
// contain a full copy of the report (including index.html) are served directly
// without touching latest/.
func TestOverlay_BackwardCompat_FullCopyBuild(t *testing.T) {
	dir := t.TempDir()
	// Old build with full copy.
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "2", "index.html"),
		"<html>build2</html>")
	// latest/ also has index.html — must NOT be used.
	writeTestFile(t,
		filepath.Join(dir, "myproject", "reports", "latest", "index.html"),
		"<html>latest</html>")

	h := newOverlayHandler(dir)
	rr := getOverlay(t, h, "/myproject/reports/2/index.html")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "<html>build2</html>" {
		t.Errorf("expected build2 content, got: %q", body)
	}
}
