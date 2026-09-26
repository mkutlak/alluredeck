package tools_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestMatchKnownIssues(t *testing.T) {
	ctx := context.Background()
	mocks := testutil.New()
	conn, err := mocks.KnownIssues.Create(ctx, 1, "Connection test", `connection refused`, "", "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	// An uncompilable pattern is skipped, not fatal to the whole match.
	for name, pattern := range map[string]string{"Timeout test": `timeout exceeded`, "Bad pattern": `[invalid`} {
		if _, err := mocks.KnownIssues.Create(ctx, 1, name, pattern, "", ""); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	cs := setupTestServer(t, &bootstrap.Stores{KnownIssue: mocks.KnownIssues})

	out, _ := call[tools.MatchKnownIssuesOutput](t, cs, "match_known_issues",
		map[string]any{"project_id": 1, "error_message": "dial tcp: connection refused after 30s"})
	want := []tools.KnownIssueMatch{{KnownIssueID: conn.ID, Name: "Connection test", RegexPattern: `connection refused`, MatchedSubstring: "connection refused"}}
	if !reflect.DeepEqual(out.Items, want) {
		t.Errorf("matches = %+v, want %+v", out.Items, want)
	}

	none, _ := call[tools.MatchKnownIssuesOutput](t, cs, "match_known_issues", map[string]any{"project_id": 1, "error_message": "completely unrelated error"})
	if len(none.Items) != 0 {
		t.Errorf("matches = %+v, want none", none.Items)
	}
	for _, args := range []map[string]any{{"project_id": 1, "error_message": ""}, {"project_id": 0, "error_message": "some error"}} {
		callErr(t, cs, "match_known_issues", args)
	}
}
