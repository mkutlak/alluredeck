package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

func newDashboardTestHandler(t *testing.T, db *store.SQLiteStore) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: t.TempDir()}
	st := storage.NewLocalStore(cfg)
	bs := store.NewBuildStore(db, zap.NewNop())
	lockManager := store.NewLockManager()
	ts := store.NewTestResultStore(db, zap.NewNop())
	r := runner.NewAllure(cfg, st, bs, lockManager, ts, zap.NewNop())
	return NewAllureHandler(cfg, r, nil, store.NewProjectStore(db, zap.NewNop()), bs, store.NewKnownIssueStore(db), ts, nil, st)
}

func openDashboardDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestGetDashboard_Empty(t *testing.T) {
	db := openDashboardDB(t)
	ctx := context.Background()

	h := newDashboardTestHandler(t, db)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/dashboard", nil)
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
	db := openDashboardDB(t)
	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()

	projID := "single-proj"
	_ = ps.CreateProject(ctx, projID)

	// Insert 3 builds.
	_ = bs.InsertBuild(ctx, projID, 1)
	_ = bs.UpdateBuildStats(ctx, projID, 1, store.BuildStats{Passed: 80, Total: 100})
	_ = bs.InsertBuild(ctx, projID, 2)
	_ = bs.UpdateBuildStats(ctx, projID, 2, store.BuildStats{Passed: 85, Total: 100})
	_ = bs.InsertBuild(ctx, projID, 3)
	_ = bs.UpdateBuildStats(ctx, projID, 3, store.BuildStats{Passed: 90, Total: 100})
	_ = bs.SetLatest(ctx, projID, 3)

	h := newDashboardTestHandler(t, db)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/dashboard", nil)
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
	db := openDashboardDB(t)
	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()

	// proj-healthy: pass_rate=95% (>= 90%)
	_ = ps.CreateProject(ctx, "proj-healthy")
	_ = bs.InsertBuild(ctx, "proj-healthy", 1)
	_ = bs.UpdateBuildStats(ctx, "proj-healthy", 1, store.BuildStats{Passed: 95, Total: 100})
	_ = bs.SetLatest(ctx, "proj-healthy", 1)

	// proj-degraded: pass_rate=80% (>= 70% and < 90%)
	_ = ps.CreateProject(ctx, "proj-degraded")
	_ = bs.InsertBuild(ctx, "proj-degraded", 1)
	_ = bs.UpdateBuildStats(ctx, "proj-degraded", 1, store.BuildStats{Passed: 80, Total: 100})
	_ = bs.SetLatest(ctx, "proj-degraded", 1)

	// proj-failing: pass_rate=50% (< 70%)
	_ = ps.CreateProject(ctx, "proj-failing")
	_ = bs.InsertBuild(ctx, "proj-failing", 1)
	_ = bs.UpdateBuildStats(ctx, "proj-failing", 1, store.BuildStats{Passed: 50, Total: 100})
	_ = bs.SetLatest(ctx, "proj-failing", 1)

	h := newDashboardTestHandler(t, db)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/dashboard", nil)
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
	db := openDashboardDB(t)
	ps := store.NewProjectStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "no-builds-proj")

	h := newDashboardTestHandler(t, db)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/dashboard", nil)
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
