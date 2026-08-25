package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// setupTestServer wires a real MCP server with in-memory transport and returns
// a connected ClientSession for tool calls.
func setupTestServer(t *testing.T, stores *bootstrap.Stores) *mcpsdk.ClientSession {
	t.Helper()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "v0"}, nil)
	// Empty publicURL and nil signing key: these tests exercise read tools,
	// and a nil key leaves the write-confirmation gate disabled.
	tools.RegisterAll(srv, stores, zap.NewNop(), "", nil)

	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// buildStores assembles a *bootstrap.Stores from testutil mocks.
func buildStores(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
	}
}

// decodeOutput JSON-round-trips StructuredContent into ListFailingTestsOutput.
func decodeOutput(t *testing.T, res *mcpsdk.CallToolResult) tools.ListFailingTestsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	var out tools.ListFailingTestsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListFailingTestsOutput: %v", err)
	}
	return out
}

// TestListFailingTests_HappyPath seeds 3 failures and verifies they all come back.
func TestListFailingTests_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, projectID int64) (store.Build, error) {
		return store.Build{ID: 42, ProjectID: projectID, BuildNumber: 7}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, projectID, buildID int64, limit int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 42, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", Retries: 0, Flaky: false},
			{BuildID: 42, ProjectID: projectID, HistoryID: "h2", FullName: "pkg.Test2", Status: "broken", Retries: 1, Flaky: true},
			{BuildID: 42, ProjectID: projectID, HistoryID: "h3", FullName: "pkg.Test3", Status: "failed", Retries: 0, Flaky: false},
		}, nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "limit": 50},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if len(out.Items) != 3 {
		t.Errorf("want 3 items, got %d", len(out.Items))
	}
	if out.Items[1].FullName != "pkg.Test2" {
		t.Errorf("want pkg.Test2, got %q", out.Items[1].FullName)
	}
	if !out.Items[1].Flaky {
		t.Error("want Items[1].Flaky=true")
	}
}

// TestListFailingTests_TestResultIDPopulated verifies test_result_id is taken
// from the store row's ID (previously always zero — a fabricated value a
// caller could mistake for a real id).
func TestListFailingTests_TestResultIDPopulated(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, projectID int64) (store.Build, error) {
		return store.Build{ID: 42, ProjectID: projectID, BuildNumber: 7}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{ID: 501, BuildID: 42, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed"},
		}, nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if len(out.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(out.Items))
	}
	if out.Items[0].TestResultID != 501 {
		t.Errorf("want test_result_id=501, got %d", out.Items[0].TestResultID)
	}
}

// TestListFailingTests_CursorIgnored verifies that a cursor value never
// affects the request the tool issues to the store — list_failing_tests has
// no offset pagination, so the input schema accepts and ignores it rather
// than lying about a "next page" that would just re-return page one.
func TestListFailingTests_CursorIgnored(t *testing.T) {
	var gotLimit int
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, _ int64) (store.Build, error) {
		return store.Build{ID: 1, BuildNumber: 1}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
		gotLimit = limit
		return []store.TestResult{{BuildID: 1, HistoryID: "h1", FullName: "Test1", Status: "failed"}}, nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	// An arbitrary, non-base64 cursor must not cause a validation error.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "limit": 2, "cursor": "not-a-real-cursor!!"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error (cursor should be ignored, not validated): %v", res.Content)
	}
	if gotLimit != 2 {
		t.Errorf("want store called with limit=2 regardless of cursor, got %d", gotLimit)
	}

	out := decodeOutput(t, res)
	if len(out.Items) != 1 {
		t.Errorf("want 1 item, got %d", len(out.Items))
	}
}

// TestListFailingTests_NoBuilds verifies that a project with no builds returns empty items, no error.
func TestListFailingTests_NoBuilds(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, _ int64) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if len(out.Items) != 0 {
		t.Errorf("want 0 items, got %d", len(out.Items))
	}
}

// TestListFailingTests_InvalidInput verifies that project_id=0 causes IsError=true.
func TestListFailingTests_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 0},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

// TestListFailingTests_StoreError verifies that a store error surfaces as IsError=true
// with the error message embedded in the content.
func TestListFailingTests_StoreError(t *testing.T) {
	storeErr := errors.New("db connection reset")

	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, _ int64) (store.Build, error) {
		return store.Build{ID: 99, BuildNumber: 1}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, _ int) ([]store.TestResult, error) {
		return nil, storeErr
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for store error")
	}
	// Verify the error message is embedded in Content.
	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			if contains(tc.Text, storeErr.Error()) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected store error message %q in content, got: %v", storeErr.Error(), res.Content)
	}
}

// TestListFailingTests_BuildIDNotInProject verifies that a build_id belonging to
// a different project returns IsError=true with the hint message.
func TestListFailingTests_BuildIDNotInProject(t *testing.T) {
	mocks := testutil.New()
	// GetBuildByID returns ErrBuildNotFound — build_id=28 not in project=1.
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, _ int64) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "build_id": 28},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for build_id not in project")
	}
	// Hint message must be present.
	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			if contains(tc.Text, "resolve_url") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("want hint about resolve_url in error message")
	}
}

// TestListFailingTests_SummaryOnly_FallsBackToCount verifies summary_only=true
// returns counts without an item list, and that total_failed falls back to
// CountFailedByBuild's distinct count when the build has no stat_failed/
// stat_broken (predates stat tracking).
func TestListFailingTests_SummaryOnly_FallsBackToCount(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, projectID int64) (store.Build, error) {
		return store.Build{ID: 10, ProjectID: projectID, BuildNumber: 3}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{Status: "failed"},
			{Status: "failed"},
			{Status: "broken"},
		}, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _, _ int64) (int, error) {
		return 3, nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "summary_only": true},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if out.Summary == nil {
		t.Fatal("want non-nil summary")
	}
	if out.Summary.TotalFailed != 3 {
		t.Errorf("want total_failed=3 (fallback to distinct count), got %d", out.Summary.TotalFailed)
	}
	if out.Summary.DistinctFailed != 3 {
		t.Errorf("want distinct_failed=3, got %d", out.Summary.DistinctFailed)
	}
	if out.Summary.Statuses["failed"] != 2 {
		t.Errorf("want statuses.failed=2, got %d", out.Summary.Statuses["failed"])
	}
	if out.Summary.Statuses["broken"] != 1 {
		t.Errorf("want statuses.broken=1, got %d", out.Summary.Statuses["broken"])
	}
}

// TestListFailingTests_SummaryOnly_UsesBuildStats verifies total_failed
// prefers the build's own stat_failed+stat_broken over the (possibly
// row-capped) distinct count, while distinct_failed always reflects
// CountFailedByBuild — the two legitimately differ when a test is ingested
// twice under two history_id schemes (see DistinctFailed doc comment).
func TestListFailingTests_SummaryOnly_UsesBuildStats(t *testing.T) {
	failed, broken := 5, 2
	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, projectID int64) (store.Build, error) {
		return store.Build{ID: 10, ProjectID: projectID, BuildNumber: 3, StatFailed: &failed, StatBroken: &broken}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{{Status: "failed"}}, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _, _ int64) (int, error) {
		return 4, nil // dedup-aware count differs from the raw stat sum (7)
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "summary_only": true},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if out.Summary == nil {
		t.Fatal("want non-nil summary")
	}
	if out.Summary.TotalFailed != 7 {
		t.Errorf("want total_failed=7 (stat_failed+stat_broken), got %d", out.Summary.TotalFailed)
	}
	if out.Summary.DistinctFailed != 4 {
		t.Errorf("want distinct_failed=4, got %d", out.Summary.DistinctFailed)
	}
}

// TestListFailingTests_BuildRefForGreenBuild verifies that the top-level
// `build` hoist is populated even when the failing-tests list is empty, as
// long as an explicit build_id resolved to a real build. This matches
// compare_builds' always-hoist behavior and saves the LLM a round-trip.
func TestListFailingTests_BuildRefForGreenBuild(t *testing.T) {
	mocks := testutil.New()
	branch := "main"
	sha := "abc123"
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, _ int64) (store.Build, error) {
		return store.Build{
			ID:          164,
			ProjectID:   1,
			BuildNumber: 28,
			CIBranch:    &branch,
			CICommitSHA: &sha,
		}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{}, nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "build_id": 164},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}

	out := decodeOutput(t, res)
	if out.Build == nil {
		t.Fatal("want out.Build populated for green build, got nil")
	}
	if out.Build.BuildNumber != 28 {
		t.Errorf("want build_number=28, got %d", out.Build.BuildNumber)
	}
	if out.Build.Branch != "main" {
		t.Errorf("want branch=main, got %q", out.Build.Branch)
	}
	if out.Build.CommitSHA != "abc123" {
		t.Errorf("want commit_sha=abc123, got %q", out.Build.CommitSHA)
	}
	if len(out.Items) != 0 {
		t.Errorf("want empty items, got %d", len(out.Items))
	}
}

// contains is a simple substring check.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
