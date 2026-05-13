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
	tools.RegisterAll(srv, stores, zap.NewNop())

	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
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
	if out.NextCursor != "" {
		t.Errorf("want empty next_cursor, got %q", out.NextCursor)
	}
	if out.Items[1].FullName != "pkg.Test2" {
		t.Errorf("want pkg.Test2, got %q", out.Items[1].FullName)
	}
	if !out.Items[1].Flaky {
		t.Error("want Items[1].Flaky=true")
	}
}

// TestListFailingTests_Pagination seeds 5 failures, uses limit=2, and walks all pages.
func TestListFailingTests_Pagination(t *testing.T) {
	allResults := []store.TestResult{
		{BuildID: 1, HistoryID: "h1", FullName: "Test1", Status: "failed"},
		{BuildID: 1, HistoryID: "h2", FullName: "Test2", Status: "failed"},
		{BuildID: 1, HistoryID: "h3", FullName: "Test3", Status: "failed"},
		{BuildID: 1, HistoryID: "h4", FullName: "Test4", Status: "failed"},
		{BuildID: 1, HistoryID: "h5", FullName: "Test5", Status: "failed"},
	}

	mocks := testutil.New()
	mocks.Builds.GetLatestBuildFn = func(_ context.Context, _ int64) (store.Build, error) {
		return store.Build{ID: 1, BuildNumber: 1}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
		// Return up to limit items (handler requests limit+1 for has-more detection).
		if limit > len(allResults) {
			limit = len(allResults)
		}
		return allResults[:limit], nil
	}

	cs := setupTestServer(t, buildStores(mocks))
	ctx := context.Background()

	// Page 1: limit=2, no cursor → expect 2 items + cursor.
	res1, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "build_id": 1, "limit": 2},
	})
	if err != nil {
		t.Fatalf("page1 CallTool: %v", err)
	}
	if res1.IsError {
		t.Fatalf("page1 unexpected error: %v", res1.Content)
	}
	out1 := decodeOutput(t, res1)
	if len(out1.Items) != 2 {
		t.Errorf("page1: want 2 items, got %d", len(out1.Items))
	}
	if out1.NextCursor == "" {
		t.Error("page1: want non-empty next_cursor")
	}

	// Page 2: use cursor from page 1, expect 2 items + cursor.
	// We simulate the store returning items [2..4] by adjusting the mock.
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
		offset := 2 // page2 offset
		end := offset + limit
		if end > len(allResults) {
			end = len(allResults)
		}
		return allResults[offset:end], nil
	}
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "build_id": 1, "limit": 2, "cursor": out1.NextCursor},
	})
	if err != nil {
		t.Fatalf("page2 CallTool: %v", err)
	}
	if res2.IsError {
		t.Fatalf("page2 unexpected error: %v", res2.Content)
	}
	out2 := decodeOutput(t, res2)
	if len(out2.Items) != 2 {
		t.Errorf("page2: want 2 items, got %d", len(out2.Items))
	}
	if out2.NextCursor == "" {
		t.Error("page2: want non-empty next_cursor")
	}

	// Page 3: last page, expect 1 item + empty cursor.
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
		offset := 4 // page3 offset
		end := offset + limit
		if end > len(allResults) {
			end = len(allResults)
		}
		return allResults[offset:end], nil
	}
	res3, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_failing_tests",
		Arguments: map[string]any{"project_id": 1, "build_id": 1, "limit": 2, "cursor": out2.NextCursor},
	})
	if err != nil {
		t.Fatalf("page3 CallTool: %v", err)
	}
	if res3.IsError {
		t.Fatalf("page3 unexpected error: %v", res3.Content)
	}
	out3 := decodeOutput(t, res3)
	if len(out3.Items) != 1 {
		t.Errorf("page3: want 1 item, got %d", len(out3.Items))
	}
	if out3.NextCursor != "" {
		t.Errorf("page3: want empty next_cursor, got %q", out3.NextCursor)
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
	if out.NextCursor != "" {
		t.Errorf("want empty next_cursor, got %q", out.NextCursor)
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
