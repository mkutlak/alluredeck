package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func buildStoresKnownIssue(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		KnownIssue: mocks.KnownIssues,
	}
}

func decodeMatchKnownIssues(t *testing.T, res *mcpsdk.CallToolResult) tools.MatchKnownIssuesOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.MatchKnownIssuesOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal MatchKnownIssuesOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// match_known_issues
// ---------------------------------------------------------------------------

func TestMatchKnownIssues_HappyPath(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()

	// Seed active known issues via the stateful MemKnownIssueStore.
	if _, err := mocks.KnownIssues.Create(ctx, 1, "Connection test", `connection refused`, "", ""); err != nil {
		t.Fatalf("seed ki1: %v", err)
	}
	if _, err := mocks.KnownIssues.Create(ctx, 1, "Timeout test", `timeout exceeded`, "", ""); err != nil {
		t.Fatalf("seed ki2: %v", err)
	}

	cs := setupTestServer(t, buildStoresKnownIssue(mocks))

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "match_known_issues",
		Arguments: map[string]any{"project_id": 1, "error_message": "dial tcp: connection refused after 30s"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeMatchKnownIssues(t, res)
	if len(out.Items) != 1 {
		t.Errorf("want 1 match, got %d", len(out.Items))
	}
	if out.Items[0].MatchedSubstring != "connection refused" {
		t.Errorf("want matched_substring=connection refused, got %q", out.Items[0].MatchedSubstring)
	}
}

func TestMatchKnownIssues_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresKnownIssue(mocks))
	ctx := context.Background()

	// Missing error_message.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "match_known_issues",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty error_message")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "match_known_issues",
		Arguments: map[string]any{"error_message": "some error"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestMatchKnownIssues_NoMatch(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()

	if _, err := mocks.KnownIssues.Create(ctx, 1, "Timeout test", `timeout exceeded`, "", ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cs := setupTestServer(t, buildStoresKnownIssue(mocks))

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "match_known_issues",
		Arguments: map[string]any{"project_id": 1, "error_message": "completely unrelated error"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeMatchKnownIssues(t, res)
	if len(out.Items) != 0 {
		t.Errorf("want 0 matches, got %d", len(out.Items))
	}
}

func TestMatchKnownIssues_BadRegex_FailSoft(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()

	// Seed one invalid regex and one valid one.
	if _, err := mocks.KnownIssues.Create(ctx, 1, "Bad pattern", `[invalid`, "", ""); err != nil {
		t.Fatalf("seed bad: %v", err)
	}
	if _, err := mocks.KnownIssues.Create(ctx, 1, "Good pattern", `timeout`, "", ""); err != nil {
		t.Fatalf("seed good: %v", err)
	}

	cs := setupTestServer(t, buildStoresKnownIssue(mocks))

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "match_known_issues",
		Arguments: map[string]any{"project_id": 1, "error_message": "timeout after 30s"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	// Should not be an error — bad regex is skipped silently.
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeMatchKnownIssues(t, res)
	// Only the valid pattern should match.
	if len(out.Items) != 1 {
		t.Errorf("want 1 match (bad regex skipped), got %d", len(out.Items))
	}
	if out.Items[0].MatchedSubstring != "timeout" {
		t.Errorf("want matched_substring=timeout, got %q", out.Items[0].MatchedSubstring)
	}
}
