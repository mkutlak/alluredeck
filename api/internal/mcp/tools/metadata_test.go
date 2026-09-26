package tools_test

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// writeTools are the only tools permitted to declare themselves non-read-only.
// Adding a tool here is a deliberate act: it makes clients prompt the user.
var writeTools = map[string]bool{
	"propose_classify_defect": true,
	"propose_known_issue":     true,
	"propose_mark_flaky":      true,
}

// TestRegisteredToolMetadata checks what a client sees in tools/list. A
// missing Title renders as a raw snake_case identifier. Annotations decide
// auto-approval: a query tool without ReadOnlyHint prompts on every call, and
// a write tool claiming ReadOnlyHint would be auto-approved.
func TestRegisteredToolMetadata(t *testing.T) {
	cs := connect(t, func(s *mcpsdk.Server) {
		tools.RegisterAll(s, &bootstrap.Stores{
			DefectProposals:     &testutil.MockDefectProposalStore{},
			KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
			FlakyProposals:      &testutil.MockFlakyProposalStore{},
			TestResult:          &testutil.MockTestResultStore{},
			Audit:               testutil.NewMockAuditLogger(),
		}, zap.NewNop(), "", nil)
	})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	writes := 0
	for _, tool := range res.Tools {
		// House style "<Verb> AllureDeck <object>": self-identifying wherever
		// a client shows it without server context.
		if !strings.Contains(tool.Title, "AllureDeck") {
			t.Errorf("tool %q title %q does not name AllureDeck", tool.Name, tool.Title)
		}
		a := tool.Annotations
		if a == nil {
			t.Errorf("tool %q has no Annotations; clients cannot tell whether it mutates", tool.Name)
			continue
		}
		if a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("tool %q should declare OpenWorldHint=false; it reaches only this deployment's database", tool.Name)
		}
		if !writeTools[tool.Name] {
			if !a.ReadOnlyHint {
				t.Errorf("query tool %q does not declare ReadOnlyHint", tool.Name)
			}
			continue
		}
		writes++
		// A proposal only inserts a pending row, and calling twice queues two.
		if a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint || a.IdempotentHint {
			t.Errorf("write tool %q annotations = %+v, want ReadOnly=false Destructive=false Idempotent=false", tool.Name, a)
		}
	}
	if writes != len(writeTools) {
		t.Errorf("registered %d of the %d write tools", writes, len(writeTools))
	}
}
