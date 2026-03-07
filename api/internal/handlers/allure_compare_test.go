package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// seedCompareBuilds creates a project with two builds pre-seeded in the DB.
func seedCompareBuilds(t *testing.T, db *store.SQLiteStore, projectID string) {
	t.Helper()
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	bs := store.NewBuildStore(db, zap.NewNop())
	ts := store.NewTestResultStore(db, zap.NewNop())

	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, order := range []int{1, 2} {
		if err := bs.InsertBuild(ctx, projectID, order); err != nil {
			t.Fatalf("InsertBuild %d: %v", order, err)
		}
	}

	idA, err := ts.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID 1: %v", err)
	}
	idB, err := ts.GetBuildID(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("GetBuildID 2: %v", err)
	}

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: idA, ProjectID: projectID, TestName: "LoginTest", FullName: "pkg.LoginTest", Status: "passed", DurationMs: 1000, HistoryID: "h1"},
		{BuildID: idA, ProjectID: projectID, TestName: "OldTest", FullName: "pkg.OldTest", Status: "passed", DurationMs: 500, HistoryID: "h2"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: idB, ProjectID: projectID, TestName: "LoginTest", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 2000, HistoryID: "h1"},
		{BuildID: idB, ProjectID: projectID, TestName: "NewTest", FullName: "pkg.NewTest", Status: "passed", DurationMs: 300, HistoryID: "h3"},
	})
}

func TestCompareBuilds_Success(t *testing.T) {
	projectsDir := t.TempDir()
	db := openTestStore(t)
	projectID := "cmp-handler"
	seedCompareBuilds(t, db, projectID)

	h := newTestAllureHandlerWithDB(t, projectsDir, db)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/compare?a=1&b=2", projectID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}

	// Verify summary fields exist
	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be object, got %T", data["summary"])
	}
	if total, _ := summary["total"].(float64); int(total) == 0 {
		t.Errorf("expected non-zero total in summary")
	}

	// Verify tests array
	tests, ok := data["tests"].([]any)
	if !ok {
		t.Fatalf("expected tests to be array, got %T", data["tests"])
	}
	if len(tests) == 0 {
		t.Error("expected non-empty tests array")
	}

	// Verify build_a and build_b are present
	if ba, _ := data["build_a"].(float64); int(ba) != 1 {
		t.Errorf("expected build_a=1, got %v", data["build_a"])
	}
	if bb, _ := data["build_b"].(float64); int(bb) != 2 {
		t.Errorf("expected build_b=2, got %v", data["build_b"])
	}

	// Verify test entry structure
	entry, ok := tests[0].(map[string]any)
	if !ok {
		t.Fatalf("expected test entry to be object, got %T", tests[0])
	}
	for _, field := range []string{"test_name", "full_name", "history_id", "status_a", "status_b", "duration_a", "duration_b", "duration_delta", "category"} {
		if _, exists := entry[field]; !exists {
			t.Errorf("missing field %q in test entry", field)
		}
	}

	// metadata
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata to be object, got %T", resp["metadata"])
	}
	if msg, _ := meta["message"].(string); msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

func TestCompareBuilds_MissingParams(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	cases := []struct {
		name  string
		query string
	}{
		{"missing both", ""},
		{"missing b", "?a=1"},
		{"missing a", "?b=2"},
		{"a not integer", "?a=foo&b=2"},
		{"b not integer", "?a=1&b=bar"},
		{"a zero", "?a=0&b=2"},
		{"b zero", "?a=1&b=0"},
		{"a negative", "?a=-1&b=2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
				"/api/v1/projects/proj/compare"+tc.query, nil)
			req.SetPathValue("project_id", "proj")

			rr := httptest.NewRecorder()
			h.CompareBuilds(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestCompareBuilds_SameBuild(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj/compare?a=1&b=1", nil)
	req.SetPathValue("project_id", "proj")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	msg, _ := meta["message"].(string)
	if msg != "build_a and build_b must be different" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestCompareBuilds_BuildNotFound(t *testing.T) {
	projectsDir := t.TempDir()
	db := openTestStore(t)
	projectID := "cmp-notfound"

	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	bs := store.NewBuildStore(db, zap.NewNop())
	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 1)

	h := newTestAllureHandlerWithDB(t, projectsDir, db)

	// Build 99 does not exist
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/compare?a=1&b=99", projectID), nil)
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCompareBuilds_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/compare?a=1&b=2", nil)
	req.SetPathValue("project_id", "../evil")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCompareBuilds_NoStore(t *testing.T) {
	// When testResultStore is nil, handler returns empty data (same pattern as analytics)
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir) // newTestAllureHandler sets testResultStore=nil

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/someproj/compare?a=1&b=2", nil)
	req.SetPathValue("project_id", "someproj")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	// nil store returns empty response, not an error
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 with nil store, got %d: %s", rr.Code, rr.Body.String())
	}
}
