package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetDashboard_Empty(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetDashboardDataFn = func(_ context.Context, sparklineDepth int) ([]store.DashboardProject, error) {
		return []store.DashboardProject{}, nil
	}

	h := newTestAllureHandlerWithMocks(t, t.TempDir(), mocks)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	h.GetDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]any)

	projects, ok := data["projects"].([]any)
	if !ok {
		t.Fatalf("expected projects to be an array, got %T", data["projects"])
	}
	if len(projects) != 0 {
		t.Errorf("expected empty projects array, got %d entries", len(projects))
	}

	summary := data["summary"].(map[string]any)
	if int(summary["total_projects"].(float64)) != 0 {
		t.Errorf("total_projects = %v, want 0", summary["total_projects"])
	}
}

func TestGetDashboard_SingleProject(t *testing.T) {
	projID := "single-proj"
	mocks := testutil.New()
	mocks.Builds.GetDashboardDataFn = func(_ context.Context, sparklineDepth int) ([]store.DashboardProject, error) {
		return []store.DashboardProject{
			{
				ProjectID: projID,
				CreatedAt: time.Now(),
	
				Latest: &store.Build{
					ID:         3,
					ProjectID:  projID,
					BuildOrder: 3,
					IsLatest:   true,
					StatPassed: intPtr(90),
					StatTotal:  intPtr(100),
				},
				Sparkline: []store.SparklinePoint{
					{BuildOrder: 1, PassRate: 80.0},
					{BuildOrder: 2, PassRate: 85.0},
					{BuildOrder: 3, PassRate: 90.0},
				},
			},
		}, nil
	}

	h := newTestAllureHandlerWithMocks(t, t.TempDir(), mocks)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	h.GetDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]any)

	projects := data["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	proj := projects[0].(map[string]any)
	if proj["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", proj["project_id"], projID)
	}

	latestBuild, ok := proj["latest_build"].(map[string]any)
	if !ok {
		t.Fatalf("expected latest_build to be an object, got %T", proj["latest_build"])
	}
	if int(latestBuild["build_order"].(float64)) != 3 {
		t.Errorf("latest_build.build_order = %v, want 3", latestBuild["build_order"])
	}
	if latestBuild["pass_rate"].(float64) != 90.0 {
		t.Errorf("latest_build.pass_rate = %v, want 90.0", latestBuild["pass_rate"])
	}

	sparkline, ok := proj["sparkline"].([]any)
	if !ok {
		t.Fatalf("expected sparkline to be an array, got %T", proj["sparkline"])
	}
	if len(sparkline) != 3 {
		t.Errorf("expected 3 sparkline entries, got %d", len(sparkline))
	}
}

func TestGetDashboard_HealthSummary(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetDashboardDataFn = func(_ context.Context, sparklineDepth int) ([]store.DashboardProject, error) {
		return []store.DashboardProject{
			{
				ProjectID: "proj-healthy",
				CreatedAt: time.Now(),
	
				Latest: &store.Build{
					BuildOrder: 1,
					StatPassed: intPtr(95),
					StatTotal:  intPtr(100),
				},
				Sparkline: []store.SparklinePoint{},
			},
			{
				ProjectID: "proj-degraded",
				CreatedAt: time.Now(),
	
				Latest: &store.Build{
					BuildOrder: 1,
					StatPassed: intPtr(80),
					StatTotal:  intPtr(100),
				},
				Sparkline: []store.SparklinePoint{},
			},
			{
				ProjectID: "proj-failing",
				CreatedAt: time.Now(),
	
				Latest: &store.Build{
					BuildOrder: 1,
					StatPassed: intPtr(50),
					StatTotal:  intPtr(100),
				},
				Sparkline: []store.SparklinePoint{},
			},
		}, nil
	}

	h := newTestAllureHandlerWithMocks(t, t.TempDir(), mocks)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	h.GetDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]any)

	summary := data["summary"].(map[string]any)
	if int(summary["total_projects"].(float64)) != 3 {
		t.Errorf("total_projects = %v, want 3", summary["total_projects"])
	}
	if int(summary["healthy"].(float64)) != 1 {
		t.Errorf("healthy = %v, want 1", summary["healthy"])
	}
	if int(summary["degraded"].(float64)) != 1 {
		t.Errorf("degraded = %v, want 1", summary["degraded"])
	}
	if int(summary["failing"].(float64)) != 1 {
		t.Errorf("failing = %v, want 1", summary["failing"])
	}
}

func TestGetDashboard_ProjectWithNoBuilds(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetDashboardDataFn = func(_ context.Context, sparklineDepth int) ([]store.DashboardProject, error) {
		return []store.DashboardProject{
			{
				ProjectID: "no-builds-proj",
				CreatedAt: time.Now(),
	
				Latest:    nil,
				Sparkline: []store.SparklinePoint{},
			},
		}, nil
	}

	h := newTestAllureHandlerWithMocks(t, t.TempDir(), mocks)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	h.GetDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]any)

	projects := data["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	proj := projects[0].(map[string]any)

	// latest_build must be JSON null.
	if proj["latest_build"] != nil {
		t.Errorf("expected latest_build == null, got %v", proj["latest_build"])
	}

	// sparkline must be [] (empty array, not null).
	sparkline, ok := proj["sparkline"].([]any)
	if !ok {
		t.Fatalf("expected sparkline to be an array (even if empty), got %T", proj["sparkline"])
	}
	if len(sparkline) != 0 {
		t.Errorf("expected empty sparkline, got %d entries", len(sparkline))
	}
}
