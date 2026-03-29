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

func newTestHistoryHandler(t *testing.T, mocks *testutil.MockStores) *TestHistoryHandler {
	t.Helper()
	return NewTestHistoryHandler(mocks.TestResults, mocks.Builds, mocks.Branches, t.TempDir())
}

func TestTestHistoryHandler_MissingHistoryID(t *testing.T) {
	mocks := testutil.New()
	h := newTestHistoryHandler(t, mocks)

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
	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{}, nil
	}

	h := newTestHistoryHandler(t, mocks)

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
	mocks := testutil.New()
	projectID := "hist-handler-proj"
	now := time.Now().UTC().Truncate(time.Second)
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildOrder: 1, BuildID: 101, Status: "passed", DurationMs: 400, CreatedAt: now},
			{BuildOrder: 2, BuildID: 102, Status: "passed", DurationMs: 800, CreatedAt: now},
			{BuildOrder: 3, BuildID: 103, Status: "failed", DurationMs: 1200, CreatedAt: now},
		}, nil
	}

	h := newTestHistoryHandler(t, mocks)

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
	mocks := testutil.New()
	projectID := "hist-branch-proj"
	mocks.Branches.GetByNameFn = func(_ context.Context, _, _ string) (*store.Branch, error) {
		return nil, fmt.Errorf("%w: branch=nonexistent-branch project=%s", store.ErrBranchNotFound, projectID)
	}

	h := newTestHistoryHandler(t, mocks)

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
