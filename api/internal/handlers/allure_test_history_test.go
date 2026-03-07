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

// seedTestHistory creates a project with builds and test results for history tests.
func seedTestHistory(t *testing.T, db *store.SQLiteStore, projectID string) {
	t.Helper()
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	bs := store.NewBuildStore(db, zap.NewNop())
	ts := store.NewTestResultStore(db, zap.NewNop())

	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if err := bs.InsertBuild(ctx, projectID, i); err != nil {
			t.Fatalf("InsertBuild %d: %v", i, err)
		}
		bid, err := ts.GetBuildID(ctx, projectID, i)
		if err != nil {
			t.Fatalf("GetBuildID %d: %v", i, err)
		}
		_ = ts.InsertBatch(ctx, []store.TestResult{
			{
				BuildID: bid, ProjectID: projectID,
				TestName: "LoginTest", FullName: "pkg.LoginTest",
				Status: "passed", DurationMs: int64(i * 400),
				HistoryID: "abc123",
			},
		})
	}
}

// newTestHistoryHandler creates a TestHistoryHandler backed by a real SQLite store.
func newTestHistoryHandler(t *testing.T, db *store.SQLiteStore) *TestHistoryHandler {
	t.Helper()
	logger := zap.NewNop()
	ts := store.NewTestResultStore(db, logger)
	bs := store.NewBuildStore(db, logger)
	brs := store.NewBranchStore(db)
	return NewTestHistoryHandler(ts, bs, brs)
}

func TestTestHistoryHandler_MissingHistoryID(t *testing.T) {
	db := openTestStore(t)
	h := newTestHistoryHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/test-history", nil)
	req.SetPathValue("project_id", "myproj")

	rr := httptest.NewRecorder()
	h.GetTestHistory(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	msg, _ := meta["message"].(string)
	if msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

func TestTestHistoryHandler_NoResults(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	bs := store.NewBuildStore(db, zap.NewNop())
	_ = ps.CreateProject(ctx, "empty-proj")
	_ = bs.InsertBuild(ctx, "empty-proj", 1)

	h := newTestHistoryHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/empty-proj/test-history?history_id=nonexistent", nil)
	req.SetPathValue("project_id", "empty-proj")

	rr := httptest.NewRecorder()
	h.GetTestHistory(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	history, ok := data["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", data["history"])
	}
	if len(history) != 0 {
		t.Errorf("expected empty history array, got %d entries", len(history))
	}
}

func TestTestHistoryHandler_WithResults(t *testing.T) {
	db := openTestStore(t)
	projectID := "hist-handler-proj"
	seedTestHistory(t, db, projectID)

	h := newTestHistoryHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/test-history?history_id=abc123", projectID), nil)
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.GetTestHistory(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}

	history, ok := data["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", data["history"])
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}

	// Verify first entry has expected fields.
	entry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected entry object, got %T", history[0])
	}
	for _, field := range []string{"build_order", "build_id", "status", "duration_ms", "created_at"} {
		if _, exists := entry[field]; !exists {
			t.Errorf("missing field %q in history entry", field)
		}
	}

	// Verify history_id in data.
	if hid, _ := data["history_id"].(string); hid != "abc123" {
		t.Errorf("history_id = %q, want %q", hid, "abc123")
	}

	// Verify metadata.
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %T", resp["metadata"])
	}
	if msg, _ := meta["message"].(string); msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

func TestTestHistoryHandler_BranchFilter_NotFound(t *testing.T) {
	db := openTestStore(t)
	projectID := "hist-branch-proj"
	seedTestHistory(t, db, projectID)

	h := newTestHistoryHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/test-history?history_id=abc123&branch=nonexistent-branch", projectID), nil)
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.GetTestHistory(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	msg, _ := meta["message"].(string)
	if msg == "" {
		t.Error("expected non-empty metadata.message for 404")
	}
}
