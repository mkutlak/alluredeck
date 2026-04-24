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

func TestGetReportStability_DBPath_NumericReportID(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "stability-numeric-proj"
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

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildNumber int) (store.Build, error) {
		if pid == projectID && buildNumber == 5 {
			return store.Build{
				ID:             200,
				ProjectID:      projectID,
				BuildNumber:    5,
				FlakyCount:     intPtr(2),
				RetriedCount:   intPtr(1),
				NewFailedCount: intPtr(1),
				NewPassedCount: intPtr(1),
				StatTotal:      intPtr(100),
			}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}

	mocks.TestResults.ListStabilityByBuildFn = func(_ context.Context, pid int64, buildID int64) ([]store.TestResult, error) {
		return []store.TestResult{
			{TestName: "test-flaky", FullName: "suite.test-flaky", Status: "passed", Flaky: true, Retries: 2},
			{TestName: "test-new-fail", FullName: "suite.test-new-fail", Status: "failed", NewFailed: true},
			{TestName: "test-new-pass", FullName: "suite.test-new-pass", Status: "passed", NewPassed: true},
		}, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/5/stability", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "5")

	rr := httptest.NewRecorder()
	h.GetReportStability(rr, req)

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

	flakyTests, ok := data["flaky_tests"].([]any)
	if !ok {
		t.Fatalf("expected flaky_tests to be array, got %T", data["flaky_tests"])
	}
	if len(flakyTests) != 1 {
		t.Errorf("flaky_tests length = %d, want 1", len(flakyTests))
	}

	newFailed, ok := data["new_failed"].([]any)
	if !ok {
		t.Fatalf("expected new_failed to be array, got %T", data["new_failed"])
	}
	if len(newFailed) != 1 {
		t.Errorf("new_failed length = %d, want 1", len(newFailed))
	}

	newPassed, ok := data["new_passed"].([]any)
	if !ok {
		t.Fatalf("expected new_passed to be array, got %T", data["new_passed"])
	}
	if len(newPassed) != 1 {
		t.Errorf("new_passed length = %d, want 1", len(newPassed))
	}

	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be object, got %T", data["summary"])
	}
	if int(summary["flaky_count"].(float64)) != 2 {
		t.Errorf("flaky_count = %v, want 2", summary["flaky_count"])
	}
	if int(summary["total"].(float64)) != 100 {
		t.Errorf("total = %v, want 100", summary["total"])
	}
}

func TestGetReportStability_DBPath_Latest(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "stability-latest-proj"
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
			ID:          50,
			ProjectID:   projectID,
			BuildNumber: 7,
			StatTotal:   intPtr(42),
		}, nil
	}

	mocks.TestResults.ListStabilityByBuildFn = func(_ context.Context, pid int64, buildID int64) ([]store.TestResult, error) {
		return []store.TestResult{}, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/stability", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportStability(rr, req)

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

	flakyTests, ok := data["flaky_tests"].([]any)
	if !ok {
		t.Fatalf("expected flaky_tests to be array, got %T", data["flaky_tests"])
	}
	if len(flakyTests) != 0 {
		t.Errorf("flaky_tests length = %d, want 0", len(flakyTests))
	}

	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be object, got %T", data["summary"])
	}
	if int(summary["total"].(float64)) != 42 {
		t.Errorf("total = %v, want 42", summary["total"])
	}
}

func TestGetReportStability_BuildNotFound(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "stability-notfound-proj"
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

	mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, buildNumber int) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/99/stability", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "99")

	rr := httptest.NewRecorder()
	h.GetReportStability(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportStability_EmptyResults(t *testing.T) {
	projectsDir := t.TempDir()
	projectSlug := "stability-empty-proj"
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
			ID:             10,
			ProjectID:      projectID,
			BuildNumber:    1,
			FlakyCount:     intPtr(0),
			RetriedCount:   intPtr(0),
			NewFailedCount: intPtr(0),
			NewPassedCount: intPtr(0),
			StatTotal:      intPtr(0),
		}, nil
	}

	mocks.TestResults.ListStabilityByBuildFn = func(_ context.Context, pid int64, buildID int64) ([]store.TestResult, error) {
		return []store.TestResult{}, nil
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/latest/stability", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportStability(rr, req)

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

	flakyTests, ok := data["flaky_tests"].([]any)
	if !ok {
		t.Fatalf("expected flaky_tests to be []any (not nil), got %T", data["flaky_tests"])
	}
	if len(flakyTests) != 0 {
		t.Errorf("flaky_tests length = %d, want 0", len(flakyTests))
	}

	newFailed, ok := data["new_failed"].([]any)
	if !ok {
		t.Fatalf("expected new_failed to be []any (not nil), got %T", data["new_failed"])
	}
	if len(newFailed) != 0 {
		t.Errorf("new_failed length = %d, want 0", len(newFailed))
	}

	newPassed, ok := data["new_passed"].([]any)
	if !ok {
		t.Fatalf("expected new_passed to be []any (not nil), got %T", data["new_passed"])
	}
	if len(newPassed) != 0 {
		t.Errorf("new_passed length = %d, want 0", len(newPassed))
	}
}
