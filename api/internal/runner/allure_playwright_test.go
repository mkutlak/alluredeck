package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestAllureWithAttachments constructs an Allure instance with mock attachment
// and test-result stores for Playwright copy tests.
func newTestAllureWithAttachments(t *testing.T, projectsDir string, mocks *testutil.MockStores) *Allure {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	return NewAllure(AllureDeps{
		Config:          cfg,
		Store:           st,
		BuildStore:      mocks.Builds,
		Locker:          mocks.Locker,
		TestResultStore: mocks.TestResults,
		AttachmentStore: mocks.Attachments,
		Logger:          zap.NewNop(),
	})
}

// TestCopyPlaywrightReport_NoLatest verifies that copyPlaywrightReport is a no-op
// when playwright-reports/latest/ does not exist.
func TestCopyPlaywrightReport_NoLatest(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(1)
	slug := "pw-no-latest"
	buildNumber := 1

	mocks := testutil.New()
	var setHasCalled bool
	mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, _ int, _ bool) error {
		setHasCalled = true
		return nil
	}

	a := newTestAllureWithAttachments(t, dir, mocks)
	a.copyPlaywrightReport(context.Background(), projectID, slug, buildNumber)

	if setHasCalled {
		t.Error("SetHasPlaywrightReport should not be called when latest/ does not exist")
	}

	// No numbered build directory should be created.
	buildDir := filepath.Join(dir, slug, "playwright-reports", "1")
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		t.Error("playwright-reports/1/ should not exist when latest/ was absent")
	}
}

// TestCopyPlaywrightReport_EmptyLatest verifies that copyPlaywrightReport is a no-op
// when playwright-reports/latest/ exists but is empty.
func TestCopyPlaywrightReport_EmptyLatest(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(2)
	slug := "pw-empty-latest"
	buildNumber := 1

	latestDir := filepath.Join(dir, slug, "playwright-reports", "latest")
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	var setHasCalled bool
	mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, _ int, _ bool) error {
		setHasCalled = true
		return nil
	}

	a := newTestAllureWithAttachments(t, dir, mocks)
	a.copyPlaywrightReport(context.Background(), projectID, slug, buildNumber)

	if setHasCalled {
		t.Error("SetHasPlaywrightReport should not be called when latest/ is empty")
	}
}

// TestCopyPlaywrightReport_CopiesReport verifies that copyPlaywrightReport copies
// playwright-reports/latest/ to playwright-reports/{buildNumber}/, sets
// has_playwright_report=true, and cleans latest/.
func TestCopyPlaywrightReport_CopiesReport(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(3)
	slug := "pw-copy-test"
	buildNumber := 3

	// Populate latest/ with a minimal Playwright report.
	latestDir := filepath.Join(dir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "index.html"), "<html>pw report</html>")
	mustWriteFile(t, filepath.Join(latestDir, "data", "abc123.png"), "\x89PNG")

	mocks := testutil.New()
	var capturedHasReport bool
	var capturedBuildNumber int
	mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, bo int, value bool) error {
		capturedBuildNumber = bo
		capturedHasReport = value
		return nil
	}
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, _ int) (int64, error) {
		return 99, nil
	}
	var capturedAttachments []store.TestAttachment
	mocks.Attachments.InsertBuildAttachmentsFn = func(_ context.Context, _ int64, _ int64, atts []store.TestAttachment) error {
		capturedAttachments = atts
		return nil
	}

	a := newTestAllureWithAttachments(t, dir, mocks)
	a.copyPlaywrightReport(context.Background(), projectID, slug, buildNumber)

	// Verify report was copied to numbered build directory.
	buildIndex := filepath.Join(dir, slug, "playwright-reports", "3", "index.html")
	if _, err := os.Stat(buildIndex); err != nil {
		t.Errorf("index.html not copied to build dir: %v", err)
	}
	buildPNG := filepath.Join(dir, slug, "playwright-reports", "3", "data", "abc123.png")
	if _, err := os.Stat(buildPNG); err != nil {
		t.Errorf("data/abc123.png not copied to build dir: %v", err)
	}

	// Verify SetHasPlaywrightReport was called with correct args.
	if !capturedHasReport {
		t.Error("SetHasPlaywrightReport: value should be true")
	}
	if capturedBuildNumber != buildNumber {
		t.Errorf("SetHasPlaywrightReport: build_number = %d, want %d", capturedBuildNumber, buildNumber)
	}

	// Verify attachment metadata was extracted and inserted.
	if len(capturedAttachments) != 1 {
		t.Fatalf("InsertBuildAttachments: got %d attachments, want 1", len(capturedAttachments))
	}
	att := capturedAttachments[0]
	if att.Source != "data/abc123.png" {
		t.Errorf("attachment Source = %q, want %q", att.Source, "data/abc123.png")
	}
	if att.MimeType != "image/png" {
		t.Errorf("attachment MimeType = %q, want %q", att.MimeType, "image/png")
	}
	if att.Name != "abc123.png" {
		t.Errorf("attachment Name = %q, want %q", att.Name, "abc123.png")
	}

	// Verify latest/ was cleaned.
	entries, err := os.ReadDir(latestDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir latest/: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("latest/ should be empty after copy, got %d entries", len(entries))
	}
}

// TestCopyPlaywrightReport_SkipsDatFiles verifies that .dat files are not inserted
// as attachments (they are allure step metadata, not useful attachments).
func TestCopyPlaywrightReport_SkipsDatFiles(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(4)
	slug := "pw-skip-dat"
	buildNumber := 1

	latestDir := filepath.Join(dir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "index.html"), "<html></html>")
	mustWriteFile(t, filepath.Join(latestDir, "data", "abc.dat"), "metadata")
	mustWriteFile(t, filepath.Join(latestDir, "data", "trace.zip"), "zipdata")

	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, _ int) (int64, error) {
		return 1, nil
	}
	var capturedAttachments []store.TestAttachment
	mocks.Attachments.InsertBuildAttachmentsFn = func(_ context.Context, _ int64, _ int64, atts []store.TestAttachment) error {
		capturedAttachments = atts
		return nil
	}

	a := newTestAllureWithAttachments(t, dir, mocks)
	a.copyPlaywrightReport(context.Background(), projectID, slug, buildNumber)

	// Only trace.zip should be inserted; abc.dat should be skipped.
	if len(capturedAttachments) != 1 {
		t.Fatalf("InsertBuildAttachments: got %d attachments, want 1 (dat skipped)", len(capturedAttachments))
	}
	if capturedAttachments[0].Source != "data/trace.zip" {
		t.Errorf("expected data/trace.zip, got %q", capturedAttachments[0].Source)
	}
	if capturedAttachments[0].MimeType != "application/zip" {
		t.Errorf("MimeType = %q, want application/zip", capturedAttachments[0].MimeType)
	}
}

// TestCopyPlaywrightReport_NoDataDir verifies that copyPlaywrightReport succeeds
// even when no data/ directory exists in the Playwright report.
func TestCopyPlaywrightReport_NoDataDir(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(5)
	slug := "pw-no-data"
	buildNumber := 2

	latestDir := filepath.Join(dir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "index.html"), "<html></html>")

	mocks := testutil.New()
	var setHasCalled bool
	mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, _ int, _ bool) error {
		setHasCalled = true
		return nil
	}
	var insertCalled bool
	mocks.Attachments.InsertBuildAttachmentsFn = func(_ context.Context, _ int64, _ int64, _ []store.TestAttachment) error {
		insertCalled = true
		return nil
	}

	a := newTestAllureWithAttachments(t, dir, mocks)
	a.copyPlaywrightReport(context.Background(), projectID, slug, buildNumber)

	if !setHasCalled {
		t.Error("SetHasPlaywrightReport should be called when index.html is present")
	}
	if insertCalled {
		t.Error("InsertBuildAttachments should not be called when data/ is absent")
	}
}

// TestMimeTypeFromExt verifies the MIME type mapping for known extensions.
func TestMimeTypeFromExt(t *testing.T) {
	cases := []struct {
		ext      string
		wantMime string
	}{
		{".png", "image/png"},
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".gif", "image/gif"},
		{".svg", "image/svg+xml"},
		{".zip", "application/zip"},
		{".webm", "video/webm"},
		{".mp4", "video/mp4"},
		{".txt", "text/plain"},
		{".log", "text/plain"},
		{".dat", ""},          // skip
		{".bin", ""},          // unknown → skip
		{".PNG", "image/png"}, // case-insensitive
	}
	for _, tc := range cases {
		got := mimeTypeFromExt(tc.ext)
		if got != tc.wantMime {
			t.Errorf("mimeTypeFromExt(%q) = %q, want %q", tc.ext, got, tc.wantMime)
		}
	}
}
