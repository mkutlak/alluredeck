package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// buildTestPlaywrightHTML creates a minimal but realistic Playwright HTML report with
// embedded base64 ZIP containing report.json and a detail file. Returns the HTML bytes.
func buildTestPlaywrightHTML(t *testing.T) []byte {
	t.Helper()

	report := map[string]any{
		"metadata": map[string]any{
			"ci": map[string]any{
				"branch":     "main",
				"commitHash": "abc123def",
				"buildHref":  "https://ci.example.com/jobs/42",
			},
			"gitCommit": map[string]any{
				"hash":   "abc123def456",
				"branch": "main",
			},
		},
		"startTime": 1700000000000,
		"duration":  5000,
		"files": []map[string]any{
			{
				"fileId":   "file1",
				"fileName": "tests/login.spec.ts",
				"tests": []map[string]any{
					{
						"testId":      "t-pass-1",
						"title":       "should login",
						"projectName": "UI Tests",
						"location":    map[string]any{"file": "tests/login.spec.ts", "line": 10, "column": 5},
						"duration":    1200,
						"tags":        []string{"@smoke", "@auth"},
						"outcome":     "expected",
						"path":        []string{"Login"},
						"ok":          true,
						"results":     []map[string]any{{"attachments": []any{}}},
					},
					{
						"testId":      "t-fail-1",
						"title":       "should show error",
						"projectName": "UI Tests",
						"location":    map[string]any{"file": "tests/login.spec.ts", "line": 25, "column": 5},
						"duration":    3000,
						"tags":        []string{"@smoke"},
						"outcome":     "unexpected",
						"path":        []string{"Login"},
						"ok":          false,
						"results":     []map[string]any{{"attachments": []any{}}},
					},
					{
						"testId":      "t-skip-1",
						"title":       "should reset password",
						"projectName": "UI Tests",
						"location":    map[string]any{"file": "tests/login.spec.ts", "line": 40, "column": 5},
						"duration":    0,
						"tags":        []string{},
						"outcome":     "skipped",
						"path":        []string{"Login"},
						"ok":          true,
						"results":     []map[string]any{{"attachments": []any{}}},
					},
				},
				"stats": map[string]any{"total": 3, "expected": 1, "unexpected": 1, "flaky": 0, "skipped": 1, "ok": false},
			},
		},
		"stats":        map[string]any{"total": 3, "expected": 1, "unexpected": 1, "flaky": 0, "skipped": 1, "ok": false},
		"projectNames": []string{"UI Tests"},
		"errors":       []any{},
	}

	detail := map[string]any{
		"fileId":   "file1",
		"fileName": "tests/login.spec.ts",
		"tests": []map[string]any{
			{
				"testId": "t-pass-1", "title": "should login", "projectName": "UI Tests",
				"location": map[string]any{"file": "tests/login.spec.ts", "line": 10, "column": 5},
				"duration": 1200, "tags": []string{"@smoke", "@auth"}, "outcome": "expected",
				"path": []string{"Login"}, "ok": true,
				"results": []map[string]any{{
					"duration": 1200, "startTime": "2023-11-14T12:00:00.000Z", "retry": 0,
					"status": "passed", "steps": []any{}, "errors": []any{}, "attachments": []any{},
				}},
			},
			{
				"testId": "t-fail-1", "title": "should show error", "projectName": "UI Tests",
				"location": map[string]any{"file": "tests/login.spec.ts", "line": 25, "column": 5},
				"duration": 3000, "tags": []string{"@smoke"}, "outcome": "unexpected",
				"path": []string{"Login"}, "ok": false,
				"results": []map[string]any{{
					"duration": 3000, "startTime": "2023-11-14T12:00:01.200Z", "retry": 0,
					"status": "failed",
					"steps": []map[string]any{{
						"title": "Click login button", "startTime": "2023-11-14T12:00:01.200Z",
						"duration": 2500, "steps": []any{}, "attachments": []any{},
					}},
					"errors": []string{"TimeoutError: locator.click: Timeout 10000ms exceeded"},
					"attachments": []map[string]any{{
						"name": "screenshot", "contentType": "image/png", "path": "data/fail-screenshot.png",
					}},
				}},
			},
			{
				"testId": "t-skip-1", "title": "should reset password", "projectName": "UI Tests",
				"location": map[string]any{"file": "tests/login.spec.ts", "line": 40, "column": 5},
				"duration": 0, "tags": []string{}, "outcome": "skipped",
				"path": []string{"Login"}, "ok": true,
				"results": []map[string]any{{
					"duration": 0, "startTime": "2023-11-14T12:00:04.200Z", "retry": 0,
					"status": "skipped", "steps": []any{}, "errors": []any{}, "attachments": []any{},
				}},
			},
		},
	}

	reportJSON, _ := json.Marshal(report)
	detailJSON, _ := json.Marshal(detail)

	// Build ZIP
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f1, _ := zw.Create("report.json")
	_, _ = f1.Write(reportJSON)
	f2, _ := zw.Create("file1.json")
	_, _ = f2.Write(detailJSON)
	_ = zw.Close()

	// Build HTML
	encoded := base64.StdEncoding.EncodeToString(zipBuf.Bytes())
	var html bytes.Buffer
	html.WriteString(`<html><head></head><body><script>window.playwrightReportBase64 = "data:application/zip;base64,`)
	html.WriteString(encoded)
	html.WriteString(`";</script></body></html>`)
	return html.Bytes()
}

// TestPlaywrightRunner_IngestReport is an integration test that verifies the full
// Playwright ingestion pipeline: HTML parsing → report directory creation → build
// stats storage → test result insertion → CI metadata extraction.
func TestPlaywrightRunner_IngestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := int64(20)
	slug := "pw-ingest-test"

	// Set up project directory with a Playwright HTML report in playwright-reports/latest/
	pwLatestDir := filepath.Join(projectsDir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(pwLatestDir, "index.html"), string(buildTestPlaywrightHTML(t)))
	mustWriteFile(t, filepath.Join(pwLatestDir, "data", "fail-screenshot.png"), "\x89PNG")

	cfg := &config.Config{
		ProjectsPath:          projectsDir,
		KeepHistory:           true,
		KeepHistoryLatest:     20,
		KeepHistoryMaxAgeDays: 0,
	}
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()

	// Track calls to verify the pipeline executed correctly.
	var mu sync.Mutex
	var capturedStats *store.BuildStats
	var capturedCI *store.CIMetadata
	var capturedTestResults []store.TestResult
	var insertBuildCalled bool

	mocks.Builds.NextBuildNumberFn = func(_ context.Context, _ int64) (int, error) {
		return 1, nil
	}
	mocks.Builds.InsertBuildFn = func(_ context.Context, _ int64, _ int) error {
		mu.Lock()
		insertBuildCalled = true
		mu.Unlock()
		return nil
	}
	mocks.Builds.UpdateBuildStatsFn = func(_ context.Context, _ int64, _ int, stats store.BuildStats) error {
		mu.Lock()
		capturedStats = &stats
		mu.Unlock()
		return nil
	}
	mocks.Builds.UpdateBuildCIMetadataFn = func(_ context.Context, _ int64, _ int, ci store.CIMetadata) error {
		mu.Lock()
		capturedCI = &ci
		mu.Unlock()
		return nil
	}
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, _ int) (int64, error) {
		return 42, nil
	}
	mocks.TestResults.InsertBatchFn = func(_ context.Context, results []store.TestResult) error {
		mu.Lock()
		capturedTestResults = results
		mu.Unlock()
		return nil
	}
	mocks.Branches.GetOrCreateFn = func(_ context.Context, _ int64, _ string) (*store.Branch, bool, error) {
		return &store.Branch{ID: 1, Name: "main"}, false, nil
	}

	pr := NewPlaywrightRunner(PlaywrightRunnerDeps{
		Config:          cfg,
		Store:           st,
		BuildStore:      mocks.Builds,
		Locker:          mocks.Locker,
		TestResultStore: mocks.TestResults,
		BranchStore:     mocks.Branches,
		DefectStore:     mocks.Defects,
		Logger:          zap.NewNop(),
	})

	msg, err := pr.IngestReport(context.Background(), projectID, slug, slug, "CI Runner", "https://ci.example.com", "", "")
	if err != nil {
		t.Fatalf("IngestReport: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty success message")
	}

	// Verify build was inserted.
	mu.Lock()
	defer mu.Unlock()

	if !insertBuildCalled {
		t.Error("InsertBuild was not called")
	}

	// Verify stats: 1 passed, 1 failed, 1 skipped = 3 total.
	if capturedStats == nil {
		t.Fatal("UpdateBuildStats was not called")
	}
	if capturedStats.Passed != 1 {
		t.Errorf("stats.Passed: got %d, want 1", capturedStats.Passed)
	}
	if capturedStats.Failed != 1 {
		t.Errorf("stats.Failed: got %d, want 1", capturedStats.Failed)
	}
	if capturedStats.Skipped != 1 {
		t.Errorf("stats.Skipped: got %d, want 1", capturedStats.Skipped)
	}
	if capturedStats.Total != 3 {
		t.Errorf("stats.Total: got %d, want 3", capturedStats.Total)
	}
	if capturedStats.DurationMs != 5000 {
		t.Errorf("stats.DurationMs: got %d, want 5000", capturedStats.DurationMs)
	}

	// Verify CI metadata was extracted from the report.
	if capturedCI == nil {
		t.Fatal("UpdateBuildCIMetadata was not called")
	}
	if capturedCI.Branch != "main" {
		t.Errorf("CI.Branch: got %q, want %q", capturedCI.Branch, "main")
	}
	if capturedCI.CommitSHA != "abc123def456" {
		t.Errorf("CI.CommitSHA: got %q, want %q", capturedCI.CommitSHA, "abc123def456")
	}

	// Verify per-test results were inserted.
	if len(capturedTestResults) != 3 {
		t.Fatalf("InsertBatch: got %d results, want 3", len(capturedTestResults))
	}

	// Find each test by name.
	byName := make(map[string]store.TestResult)
	for _, tr := range capturedTestResults {
		byName[tr.TestName] = tr
	}

	passed, ok := byName["Login > should login"]
	if !ok {
		t.Fatal("missing test result for 'Login > should login'")
	}
	if passed.Status != "passed" {
		t.Errorf("passed test status: got %q, want %q", passed.Status, "passed")
	}

	failed, ok := byName["Login > should show error"]
	if !ok {
		t.Fatal("missing test result for 'Login > should show error'")
	}
	if failed.Status != "failed" {
		t.Errorf("failed test status: got %q, want %q", failed.Status, "failed")
	}

	skipped, ok := byName["Login > should reset password"]
	if !ok {
		t.Fatal("missing test result for 'Login > should reset password'")
	}
	if skipped.Status != "skipped" {
		t.Errorf("skipped test status: got %q, want %q", skipped.Status, "skipped")
	}

	// Verify report files were copied to playwright-reports/1/.
	reportIndex := filepath.Join(projectsDir, slug, "playwright-reports", "1", "index.html")
	if _, err := os.Stat(reportIndex); err != nil {
		t.Errorf("report index.html not published: %v", err)
	}
	reportAttach := filepath.Join(projectsDir, slug, "playwright-reports", "1", "data", "fail-screenshot.png")
	if _, err := os.Stat(reportAttach); err != nil {
		t.Errorf("report attachment not published: %v", err)
	}

	// Verify playwright-reports/latest/ was cleaned up.
	latestIndex := filepath.Join(projectsDir, slug, "playwright-reports", "latest", "index.html")
	if _, err := os.Stat(latestIndex); !os.IsNotExist(err) {
		t.Error("expected playwright-reports/latest/ to be cleaned up")
	}
}
