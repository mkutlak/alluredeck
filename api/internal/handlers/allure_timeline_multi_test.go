package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// buildMocksForMultiTimeline sets up mock stores for the multi-build timeline tests.
// It returns a MockStores configured with default data suitable for most tests.
func buildMocksForMultiTimeline() *testutil.MockStores {
	mocks := testutil.New()

	createdAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	// Default: ListBuildsPaginatedBranch returns one build (latest).
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ string, _, _ int, _ *int64) ([]store.Build, int, error) {
		return []store.Build{
			{ID: 100, ProjectID: "proj1", BuildOrder: 42, CreatedAt: createdAt},
		}, 1, nil
	}

	// GetBuildID maps build order → build ID.
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ string, buildOrder int) (int64, error) {
		if buildOrder == 42 {
			return 100, nil
		}
		if buildOrder == 41 {
			return 99, nil
		}
		return 0, fmt.Errorf("not found")
	}

	// ListTimelineMulti returns test data.
	mocks.TestResults.ListTimelineMultiFn = func(_ context.Context, _ string, buildIDs []int64, _ int) ([]store.MultiTimelineRow, error) {
		var rows []store.MultiTimelineRow
		for _, bid := range buildIDs {
			if bid == 100 {
				rows = append(rows,
					store.MultiTimelineRow{BuildID: 100, BuildOrder: 42, TestName: "Login", FullName: "com.Login", Status: "passed", StartMs: 170000, StopMs: 170005, Thread: "w1", Host: "h1"},
					store.MultiTimelineRow{BuildID: 100, BuildOrder: 42, TestName: "Logout", FullName: "com.Logout", Status: "failed", StartMs: 170010, StopMs: 170020, Thread: "w2", Host: "h1"},
				)
			}
			if bid == 99 {
				rows = append(rows,
					store.MultiTimelineRow{BuildID: 99, BuildOrder: 41, TestName: "Signup", FullName: "com.Signup", Status: "passed", StartMs: 160000, StopMs: 160010, Thread: "w1", Host: "h1"},
				)
			}
		}
		return rows, nil
	}

	return mocks
}

func TestGetProjectTimeline_SingleBuild_Default(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()
	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)

	// Should have 1 build returned.
	builds := data["builds"].([]any)
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}

	b0 := builds[0].(map[string]any)
	if int(b0["build_order"].(float64)) != 42 {
		t.Errorf("expected build_order=42, got %v", b0["build_order"])
	}
	testCases := b0["test_cases"].([]any)
	if len(testCases) != 2 {
		t.Fatalf("expected 2 test_cases, got %d", len(testCases))
	}
	summary := b0["summary"].(map[string]any)
	if int(summary["total"].(float64)) != 2 {
		t.Errorf("expected total=2, got %v", summary["total"])
	}

	if int(data["builds_returned"].(float64)) != 1 {
		t.Errorf("expected builds_returned=1, got %v", data["builds_returned"])
	}
}

func TestGetProjectTimeline_BranchFilter(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()

	// Branch store resolves "main" to branch ID 10.
	mocks.Branches.GetByNameFn = func(_ context.Context, _, name string) (*store.Branch, error) {
		if name == "main" {
			return &store.Branch{ID: 10, Name: "main"}, nil
		}
		return nil, store.ErrBranchNotFound
	}

	// Override ListBuildsPaginatedBranch to verify branchID is passed.
	var capturedBranchID *int64
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ string, _, _ int, branchID *int64) ([]store.Build, int, error) {
		capturedBranchID = branchID
		return []store.Build{
			{ID: 100, ProjectID: "proj1", BuildOrder: 42, CreatedAt: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)},
		}, 1, nil
	}

	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline?branch=main", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if capturedBranchID == nil || *capturedBranchID != 10 {
		t.Errorf("expected branchID=10 to be passed to ListBuildsPaginatedBranch, got %v", capturedBranchID)
	}
}

func TestGetProjectTimeline_DateRange(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()

	createdAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	// Override ListBuildsInRange for date range queries.
	mocks.Builds.ListBuildsInRangeFn = func(_ context.Context, _ string, _ *int64, from, to time.Time, limit int) ([]store.Build, int, error) {
		expectedFrom := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
		expectedTo := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC) // end-of-day inclusive
		if !from.Equal(expectedFrom) {
			return nil, 0, fmt.Errorf("unexpected from: %v", from)
		}
		if !to.Equal(expectedTo) {
			return nil, 0, fmt.Errorf("unexpected to: %v", to)
		}
		return []store.Build{
			{ID: 100, ProjectID: "proj1", BuildOrder: 42, CreatedAt: createdAt},
			{ID: 99, ProjectID: "proj1", BuildOrder: 41, CreatedAt: createdAt.Add(-24 * time.Hour)},
		}, 5, nil
	}

	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline?from=2026-03-20&to=2026-03-25&limit=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)

	builds := data["builds"].([]any)
	if len(builds) != 2 {
		t.Fatalf("expected 2 builds, got %d", len(builds))
	}

	if int(data["total_builds_in_range"].(float64)) != 5 {
		t.Errorf("expected total_builds_in_range=5, got %v", data["total_builds_in_range"])
	}
}

func TestGetProjectTimeline_MaxBuildsWarning(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()

	// Return 1 build but report total=15 to show "showing N of M".
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ string, _, _ int, _ *int64) ([]store.Build, int, error) {
		return []store.Build{
			{ID: 100, ProjectID: "proj1", BuildOrder: 42, CreatedAt: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)},
		}, 15, nil
	}

	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline?limit=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)

	if int(data["total_builds_in_range"].(float64)) != 15 {
		t.Errorf("expected total_builds_in_range=15, got %v", data["total_builds_in_range"])
	}
	if int(data["builds_returned"].(float64)) != 1 {
		t.Errorf("expected builds_returned=1, got %v", data["builds_returned"])
	}
}

func TestGetProjectTimeline_InvalidDateFormat(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()
	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline?from=not-a-date", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetProjectTimeline_LimitCap(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := buildMocksForMultiTimeline()

	// Track the perPage passed to verify capping.
	var capturedPerPage int
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ string, _, perPage int, _ *int64) ([]store.Build, int, error) {
		capturedPerPage = perPage
		return []store.Build{
			{ID: 100, ProjectID: "proj1", BuildOrder: 42, CreatedAt: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)},
		}, 1, nil
	}

	h := newTestProjectTimelineHandler(t, projectsDir, mocks)

	// Request limit=50 — should be capped to 10.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/timeline?limit=50", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "proj1")

	rr := httptest.NewRecorder()
	h.GetProjectTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if capturedPerPage != 10 {
		t.Errorf("expected limit capped to 10, but ListBuildsPaginatedBranch received perPage=%d", capturedPerPage)
	}
}
