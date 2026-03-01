package handlers

import (
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
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// newSummaryTestHandler creates an AllureHandler wired to a real SQLite store.
func newSummaryTestHandler(t *testing.T, projectsDir string, db *store.SQLiteStore) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir}
	st := storage.NewLocalStore(cfg)
	bs := store.NewBuildStore(db, zap.NewNop())
	lockManager := store.NewLockManager()
	ts := store.NewTestResultStore(db, zap.NewNop())
	r := runner.NewAllure(cfg, st, bs, lockManager, ts, zap.NewNop())
	return NewAllureHandler(cfg, r, nil, store.NewProjectStore(db, zap.NewNop()), bs, store.NewKnownIssueStore(db), ts, nil, st)
}

func TestGetReportSummary_NumericReportID(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "summary-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ts := store.NewTestResultStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 3)
	_ = bs.UpdateBuildStats(ctx, projectID, 3, store.BuildStats{
		Passed: 85, Failed: 10, Broken: 3, Skipped: 2, Unknown: 0, Total: 100,
		DurationMs: 45000, FlakyCount: 2, RetriedCount: 1, NewFailedCount: 3, NewPassedCount: 1,
	})
	_ = bs.SetLatest(ctx, projectID, 3)
	_ = bs.UpdateBuildCIMetadata(ctx, projectID, 3, store.CIMetadata{
		Provider: "GitHub Actions", BuildURL: "https://github.com/org/repo/actions/runs/1", Branch: "main", CommitSHA: "abc1234",
	})

	buildID, _ := ts.GetBuildID(ctx, projectID, 3)
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildID, ProjectID: projectID, TestName: "Login timeout", FullName: "com.example.LoginTest#testTimeout", Status: "failed", DurationMs: 30000, HistoryID: "h1", NewFailed: true},
		{BuildID: buildID, ProjectID: projectID, TestName: "API broken", FullName: "com.example.APITest#testBroken", Status: "broken", DurationMs: 5000, HistoryID: "h2", Flaky: true},
		{BuildID: buildID, ProjectID: projectID, TestName: "Passing test", FullName: "com.example.PassTest#test", Status: "passed", DurationMs: 100, HistoryID: "h3"},
	})

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/summary-proj/reports/3/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "3")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}

	// Verify build metadata.
	build := data["build"].(map[string]any)
	if int(build["build_order"].(float64)) != 3 {
		t.Errorf("build_order = %v, want 3", build["build_order"])
	}
	if build["project_id"] != projectID {
		t.Errorf("project_id = %v, want %s", build["project_id"], projectID)
	}
	if build["is_latest"] != true {
		t.Errorf("is_latest = %v, want true", build["is_latest"])
	}
	if build["ci_provider"] != "GitHub Actions" {
		t.Errorf("ci_provider = %v, want GitHub Actions", build["ci_provider"])
	}

	// Verify statistics.
	stats := data["statistics"].(map[string]any)
	if int(stats["passed"].(float64)) != 85 {
		t.Errorf("passed = %v, want 85", stats["passed"])
	}
	if int(stats["total"].(float64)) != 100 {
		t.Errorf("total = %v, want 100", stats["total"])
	}
	if stats["passed_pct"].(float64) != 85.0 {
		t.Errorf("passed_pct = %v, want 85.0", stats["passed_pct"])
	}

	// Verify timing.
	timing := data["timing"].(map[string]any)
	if int64(timing["duration_ms"].(float64)) != 45000 {
		t.Errorf("duration_ms = %v, want 45000", timing["duration_ms"])
	}

	// Verify quality.
	quality := data["quality"].(map[string]any)
	if int(quality["flaky_count"].(float64)) != 2 {
		t.Errorf("flaky_count = %v, want 2", quality["flaky_count"])
	}
	if int(quality["new_failed_count"].(float64)) != 3 {
		t.Errorf("new_failed_count = %v, want 3", quality["new_failed_count"])
	}

	// Verify top failures.
	failures := data["top_failures"].([]any)
	if len(failures) != 2 {
		t.Fatalf("expected 2 top failures, got %d", len(failures))
	}
	f0 := failures[0].(map[string]any)
	if f0["test_name"] != "Login timeout" {
		t.Errorf("first failure = %v, want Login timeout", f0["test_name"])
	}
	if f0["new_failed"] != true {
		t.Errorf("new_failed = %v, want true", f0["new_failed"])
	}
}

func TestGetReportSummary_Latest(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "latest-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 1)
	_ = bs.InsertBuild(ctx, projectID, 2)
	_ = bs.UpdateBuildStats(ctx, projectID, 2, store.BuildStats{Passed: 50, Total: 50})
	_ = bs.SetLatest(ctx, projectID, 2)

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/latest-proj/reports/latest/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	build := data["build"].(map[string]any)
	if int(build["build_order"].(float64)) != 2 {
		t.Errorf("latest resolved to build_order %v, want 2", build["build_order"])
	}
}

func TestGetReportSummary_TrendDelta(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "trend-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 1)
	_ = bs.UpdateBuildStats(ctx, projectID, 1, store.BuildStats{
		Passed: 90, Failed: 5, Broken: 2, Skipped: 3, Unknown: 0, Total: 100, DurationMs: 40000,
	})
	_ = bs.InsertBuild(ctx, projectID, 2)
	_ = bs.UpdateBuildStats(ctx, projectID, 2, store.BuildStats{
		Passed: 85, Failed: 10, Broken: 3, Skipped: 2, Unknown: 0, Total: 100, DurationMs: 45000,
	})
	_ = bs.SetLatest(ctx, projectID, 2)

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/trend-proj/reports/2/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "2")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	trend := data["trend"].(map[string]any)
	if trend == nil {
		t.Fatal("expected trend to be present")
	}
	if int(trend["previous_build_order"].(float64)) != 1 {
		t.Errorf("previous_build_order = %v, want 1", trend["previous_build_order"])
	}
	if int(trend["passed_delta"].(float64)) != -5 {
		t.Errorf("passed_delta = %v, want -5", trend["passed_delta"])
	}
	if int(trend["failed_delta"].(float64)) != 5 {
		t.Errorf("failed_delta = %v, want 5", trend["failed_delta"])
	}
	if int64(trend["duration_delta_ms"].(float64)) != 5000 {
		t.Errorf("duration_delta_ms = %v, want 5000", trend["duration_delta_ms"])
	}
}

func TestGetReportSummary_BuildNotFound(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "notfound-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()
	_ = ps.CreateProject(ctx, projectID)

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/notfound-proj/reports/99/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "99")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportSummary_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/reports/1/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "../evil")
	req.SetPathValue("report_id", "1")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportSummary_TopFailuresLimit(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "limit-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ts := store.NewTestResultStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 1)
	_ = bs.UpdateBuildStats(ctx, projectID, 1, store.BuildStats{Failed: 15, Total: 30})
	_ = bs.SetLatest(ctx, projectID, 1)

	buildID, _ := ts.GetBuildID(ctx, projectID, 1)

	// Insert 15 failures.
	var batch []store.TestResult
	for i := range 15 {
		batch = append(batch, store.TestResult{
			BuildID: buildID, ProjectID: projectID,
			TestName: "FailTest" + string(rune('A'+i)), FullName: "pkg.FailTest" + string(rune('A'+i)),
			Status: "failed", DurationMs: int64((15 - i) * 1000), HistoryID: "h-" + string(rune('a'+i)),
		})
	}
	_ = ts.InsertBatch(ctx, batch)

	h := newSummaryTestHandler(t, projectsDir, db)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/limit-proj/reports/1/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "1")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	failures := data["top_failures"].([]any)
	if len(failures) != 10 {
		t.Errorf("expected max 10 top failures, got %d", len(failures))
	}
}
