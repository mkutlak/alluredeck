package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// buildTestPlaywrightHTML returns a minimal Playwright HTML report: an embedded
// base64 ZIP holding report.json (metadata + stats) and the per-file detail
// JSON with one passed, one failed and one skipped test. Parser field mapping
// is covered in internal/parser; this fixture only drives the runner pipeline.
func buildTestPlaywrightHTML(t *testing.T) []byte {
	t.Helper()
	pwTest := func(id, title, outcome, status string) map[string]any {
		return map[string]any{"testId": id, "title": title, "outcome": outcome, "path": []string{"Login"},
			"results": []map[string]any{{"startTime": "2023-11-14T12:00:00.000Z", "status": status}}}
	}
	report := map[string]any{
		"metadata": map[string]any{
			"ci":        map[string]any{"branch": "main", "commitHash": "abc123def", "buildHref": "https://ci.example.com/jobs/42"},
			"gitCommit": map[string]any{"hash": "abc123def456", "branch": "main"},
		},
		"startTime": 1700000000000, "duration": 5000,
		"files": []map[string]any{{"fileId": "file1", "fileName": "tests/login.spec.ts"}},
		"stats": map[string]any{"total": 3, "expected": 1, "unexpected": 1, "flaky": 0, "skipped": 1},
	}
	detail := map[string]any{"fileId": "file1", "fileName": "tests/login.spec.ts", "tests": []map[string]any{
		pwTest("t-pass-1", "should login", "expected", "passed"),
		pwTest("t-fail-1", "should show error", "unexpected", "failed"),
		pwTest("t-skip-1", "should reset password", "skipped", "skipped"),
	}}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	for name, v := range map[string]any{"report.json": report, "file1.json": detail} {
		data, _ := json.Marshal(v)
		f, _ := zw.Create(name)
		_, _ = f.Write(data)
	}
	_ = zw.Close()
	return []byte(`<html><body><script>window.playwrightReportBase64 = "data:application/zip;base64,` +
		base64.StdEncoding.EncodeToString(zipBuf.Bytes()) + `";</script></body></html>`)
}

// TestPlaywrightRunner_IngestReport verifies the full Playwright ingestion
// pipeline: HTML parsing → build reservation → stats and CI metadata → one
// test_results row per test → report published and latest/ cleaned. The
// Playwright-only path has no Allure results to reconcile against, so its
// stability (InsertBatch) and enrichment (InsertBatchFull) writes must share a
// history_id, or each test lands on two rows.
func TestPlaywrightRunner_IngestReport(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "pw-ingest-test"
	pwLatestDir := filepath.Join(projectsDir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(pwLatestDir, "index.html"), string(buildTestPlaywrightHTML(t)))
	mustWriteFile(t, filepath.Join(pwLatestDir, "data", "fail-screenshot.png"), "\x89PNG")

	cfg := &config.Config{ProjectsPath: projectsDir, KeepHistory: true, KeepHistoryLatest: 20}
	mocks := testutil.New()
	var mu sync.Mutex
	var reserved bool
	var stats *store.BuildStats
	var ci *store.CIMetadata
	var batch []store.TestResult
	var full []*parser.Result
	mocks.Builds.ReserveBuildFn = func(context.Context, int64) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		reserved = true
		return 1, nil
	}
	mocks.Builds.UpdateBuildStatsFn = func(_ context.Context, _ int64, _ int, s store.BuildStats) error {
		mu.Lock()
		defer mu.Unlock()
		stats = &s
		return nil
	}
	mocks.Builds.UpdateBuildCIMetadataFn = func(_ context.Context, _ int64, _ int, c store.CIMetadata) error {
		mu.Lock()
		defer mu.Unlock()
		ci = &c
		return nil
	}
	mocks.TestResults.GetBuildIDFn = func(context.Context, int64, int) (int64, error) { return 42, nil }
	mocks.TestResults.InsertBatchFn = func(_ context.Context, rs []store.TestResult) error {
		mu.Lock()
		defer mu.Unlock()
		batch = rs
		return nil
	}
	mocks.TestResults.InsertBatchFullFn = func(_ context.Context, _, _ int64, rs []*parser.Result) error {
		mu.Lock()
		defer mu.Unlock()
		full = rs
		return nil
	}
	mocks.Branches.GetOrCreateFn = func(context.Context, int64, string) (*store.Branch, bool, error) {
		return &store.Branch{ID: 1, Name: "main"}, false, nil
	}

	pr := NewPlaywrightRunner(PlaywrightRunnerDeps{
		Config: cfg, Store: storage.NewLocalStore(cfg), BuildStore: mocks.Builds, Locker: mocks.Locker,
		TestResultStore: mocks.TestResults, BranchStore: mocks.Branches, DefectStore: mocks.Defects, Logger: zap.NewNop(),
	})
	msg, err := pr.IngestReport(context.Background(), 20, slug, slug, "CI Runner", "https://ci.example.com", "", "", "", "")
	if err != nil {
		t.Fatalf("IngestReport: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty success message")
	}

	mu.Lock()
	defer mu.Unlock()
	if !reserved {
		t.Error("ReserveBuild was not called")
	}
	wantStats := store.BuildStats{Passed: 1, Failed: 1, Skipped: 1, Total: 3, DurationMs: 5000}
	if stats == nil || *stats != wantStats {
		t.Errorf("UpdateBuildStats = %+v, want %+v", stats, wantStats)
	}
	// Report metadata fills what the request left blank; gitCommit wins over ci.
	wantCI := store.CIMetadata{Provider: "CI Runner", BuildURL: "https://ci.example.com", Branch: "main", CommitSHA: "abc123def456"}
	if ci == nil || *ci != wantCI {
		t.Errorf("UpdateBuildCIMetadata = %+v, want %+v", ci, wantCI)
	}

	gotStatus := map[string]string{}
	stabilityIDs := map[string]string{}
	for _, r := range batch {
		gotStatus[r.TestName] = r.Status
		stabilityIDs[r.FullName] = r.HistoryID
	}
	wantStatus := map[string]string{
		"Login > should login": "passed", "Login > should show error": "failed", "Login > should reset password": "skipped",
	}
	if !maps.Equal(gotStatus, wantStatus) {
		t.Errorf("InsertBatch test statuses = %v, want %v", gotStatus, wantStatus)
	}
	if len(batch) != 3 || len(full) != 3 {
		t.Fatalf("InsertBatch rows = %d, InsertBatchFull rows = %d, want 3 each", len(batch), len(full))
	}
	for _, r := range full {
		if id, ok := stabilityIDs[r.FullName]; !ok || id != r.HistoryID {
			t.Errorf("%q: stability history_id %q != enrichment history_id %q — would create a second row", r.FullName, id, r.HistoryID)
		}
	}

	for _, rel := range []string{"index.html", "data/fail-screenshot.png"} {
		if _, err := os.Stat(filepath.Join(projectsDir, slug, "playwright-reports", "1", rel)); err != nil {
			t.Errorf("report file %s not published: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(pwLatestDir, "index.html")); !os.IsNotExist(err) {
		t.Error("expected playwright-reports/latest/ to be cleaned up")
	}
}
