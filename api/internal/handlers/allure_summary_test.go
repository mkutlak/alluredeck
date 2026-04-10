package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func ptr(v string) *string    { return &v }

func TestGetReportSummary_NumericReportID(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "summary-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := proj.ID
	projectIDStr := strconv.FormatInt(projectID, 10)

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		if pid == projectID && buildOrder == 3 {
			return store.Build{
				ID:             100,
				ProjectID:      projectID,
				BuildNumber:    3,
				IsLatest:       true,
				CIProvider:     ptr("GitHub Actions"),
				StatPassed:     intPtr(85),
				StatFailed:     intPtr(10),
				StatBroken:     intPtr(3),
				StatSkipped:    intPtr(2),
				StatTotal:      intPtr(100),
				DurationMs:     int64Ptr(45000),
				FlakyCount:     intPtr(2),
				NewFailedCount: intPtr(3),
				NewPassedCount: intPtr(1),
			}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.Builds.GetPreviousBuildFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, pid int64, buildID int64, limit int) ([]store.TestResult, error) {
		return []store.TestResult{
			{TestName: "Login timeout", Status: "failed", DurationMs: 30000, NewFailed: true},
			{TestName: "API broken", Status: "broken", DurationMs: 5000, Flaky: true},
		}, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/3/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	if int(build["build_number"].(float64)) != 3 {
		t.Errorf("build_number = %v, want 3", build["build_number"])
	}
	if int64(build["project_id"].(float64)) != projectID {
		t.Errorf("project_id = %v, want %d", build["project_id"], projectID)
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
	projectSlug := "latest-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := proj.ID
	projectIDStr := strconv.FormatInt(projectID, 10)

	mocks.Builds.GetLatestBuildFn = func(_ context.Context, pid int64) (store.Build, error) {
		return store.Build{
			ID:          2,
			ProjectID:   projectID,
			BuildNumber: 2,
			IsLatest:    true,
			StatPassed:  intPtr(50),
			StatTotal:   intPtr(50),
		}, nil
	}
	mocks.Builds.GetPreviousBuildFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	if int(build["build_number"].(float64)) != 2 {
		t.Errorf("latest resolved to build_number %v, want 2", build["build_number"])
	}
}

func TestGetReportSummary_TrendDelta(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "trend-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := proj.ID
	projectIDStr := strconv.FormatInt(projectID, 10)

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		if buildOrder == 2 {
			return store.Build{
				ID:          2,
				ProjectID:   projectID,
				BuildNumber: 2,
				StatPassed:  intPtr(85),
				StatFailed:  intPtr(10),
				StatBroken:  intPtr(3),
				StatSkipped: intPtr(2),
				StatTotal:   intPtr(100),
				DurationMs:  int64Ptr(45000),
			}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.Builds.GetPreviousBuildFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		return store.Build{
			ID:          1,
			ProjectID:   projectID,
			BuildNumber: 1,
			StatPassed:  intPtr(90),
			StatFailed:  intPtr(5),
			StatBroken:  intPtr(2),
			StatSkipped: intPtr(3),
			StatTotal:   intPtr(100),
			DurationMs:  int64Ptr(40000),
		}, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/2/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
	if int(trend["previous_build_number"].(float64)) != 1 {
		t.Errorf("previous_build_number = %v, want 1", trend["previous_build_number"])
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
	projectSlug := "notfound-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/99/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "99")

	rr := httptest.NewRecorder()
	h.GetReportSummary(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportSummary_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()

	h, _ := newTestReportHandler(t, projectsDir)
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
	projectSlug := "limit-proj"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectSlug), 0o755); err != nil {
		t.Fatal(err)
	}

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), projectSlug)
	if err != nil {
		t.Fatal(err)
	}
	projectID := proj.ID
	projectIDStr := strconv.FormatInt(projectID, 10)

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		if buildOrder == 1 {
			return store.Build{
				ID:          1,
				ProjectID:   projectID,
				BuildNumber: 1,
				StatFailed:  intPtr(15),
				StatTotal:   intPtr(30),
			}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.Builds.GetPreviousBuildFn = func(_ context.Context, pid int64, buildOrder int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	// Return 15 failures; handler caps at topFailuresLimit (10).
	var batch []store.TestResult
	for i := range 15 {
		batch = append(batch, store.TestResult{
			TestName:   "FailTest" + string(rune('A'+i)),
			FullName:   "pkg.FailTest" + string(rune('A'+i)),
			Status:     "failed",
			DurationMs: int64((15 - i) * 1000),
			HistoryID:  "h-" + string(rune('a'+i)),
		})
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, pid int64, buildID int64, limit int) ([]store.TestResult, error) {
		if limit < len(batch) {
			return batch[:limit], nil
		}
		return batch, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/1/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
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
