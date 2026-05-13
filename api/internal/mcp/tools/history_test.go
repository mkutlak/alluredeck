package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func buildStoresHistory(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
		Attachment: mocks.Attachments,
		Defect:     mocks.Defects,
		KnownIssue: mocks.KnownIssues,
	}
}

func decodeGetTestFailure(t *testing.T, res *mcpsdk.CallToolResult) tools.GetTestFailureOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetTestFailureOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetTestFailureOutput: %v", err)
	}
	return out
}

func decodeGetTestHistory(t *testing.T, res *mcpsdk.CallToolResult) tools.GetTestHistoryOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetTestHistoryOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetTestHistoryOutput: %v", err)
	}
	return out
}

func decodeCompareBuilds(t *testing.T, res *mcpsdk.CallToolResult) tools.CompareBuildsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.CompareBuildsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal CompareBuildsOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// get_test_failure
// ---------------------------------------------------------------------------

func TestGetTestFailure_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 1234},
		}, nil
	}
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{
			{ID: 99, Name: "screenshot.png", MimeType: "image/png", SizeBytes: 4096},
		}, 1, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.Status != "failed" {
		t.Errorf("want status=failed, got %q", out.Status)
	}
	if out.DurationMs != 1234 {
		t.Errorf("want duration_ms=1234, got %d", out.DurationMs)
	}
	if len(out.Attachments) == 0 {
		t.Error("want at least one attachment")
	}
	if out.Attachments[0].ResourceURI != "alluredeck://attachment/99" {
		t.Errorf("want resource_uri=alluredeck://attachment/99, got %q", out.Attachments[0].ResourceURI)
	}
}

func TestGetTestFailure_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing history_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty history_id")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestGetTestFailure_NotFound(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, HistoryID: "other-id", Status: "failed"},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for missing history_id")
	}
}

// ---------------------------------------------------------------------------
// get_test_history
// ---------------------------------------------------------------------------

func TestGetTestHistory_HappyPath(t *testing.T) {
	sha := "deadbeef"
	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildNumber: 5, BuildID: 50, Status: "passed", DurationMs: 500, CreatedAt: time.Now(), CICommitSHA: &sha},
			{BuildNumber: 4, BuildID: 40, Status: "failed", DurationMs: 800, CreatedAt: time.Now()},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].Status != "passed" {
		t.Errorf("want status=passed, got %q", out.Items[0].Status)
	}
	if out.Items[0].CommitSHA != "deadbeef" {
		t.Errorf("want commit_sha=deadbeef, got %q", out.Items[0].CommitSHA)
	}
}

func TestGetTestHistory_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing history_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty history_id")
	}
}

func TestGetTestHistory_Pagination(t *testing.T) {
	entries := make([]store.TestHistoryEntry, 5)
	for i := range entries {
		entries[i] = store.TestHistoryEntry{BuildNumber: i + 1, BuildID: int64(i + 1), Status: "passed", CreatedAt: time.Now()}
	}

	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, limit int) ([]store.TestHistoryEntry, error) {
		if limit > len(entries) {
			return entries, nil
		}
		return entries[:limit], nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// limit=2 → handler requests 3, gets 3, returns 2 with cursor.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.NextCursor == "" {
		t.Error("want non-empty next_cursor")
	}
}

// ---------------------------------------------------------------------------
// compare_builds
// ---------------------------------------------------------------------------

func TestCompareBuilds_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
			{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
			{TestName: "T3", FullName: "pkg.T3", HistoryID: "h3", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 1, "target_build_id": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if len(out.Regressed) != 1 {
		t.Errorf("want 1 regressed, got %d", len(out.Regressed))
	}
	if len(out.Fixed) != 1 {
		t.Errorf("want 1 fixed, got %d", len(out.Fixed))
	}
	if len(out.NewFailed) != 1 {
		t.Errorf("want 1 new_failed, got %d", len(out.NewFailed))
	}
	if out.Regressed[0].HistoryID != "h1" {
		t.Errorf("want regressed history_id=h1, got %q", out.Regressed[0].HistoryID)
	}
}

func TestCompareBuilds_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing base_build_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "target_build_id": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for missing base_build_id")
	}
}
