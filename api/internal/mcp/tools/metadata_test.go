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

// listRegisteredTools registers every tool on a real server and returns the
// tools/list result a client would see.
func listRegisteredTools(t *testing.T) []*mcpsdk.Tool {
	t.Helper()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "v0"}, nil)
	tools.RegisterAll(srv, &bootstrap.Stores{
		DefectProposals:     &testutil.MockDefectProposalStore{},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
		TestResult:          &testutil.MockTestResultStore{},
		Audit:               testutil.NewMockAuditLogger(),
	}, zap.NewNop(), "", nil)

	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connecting test client: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	res, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	return res.Tools
}

// TestEveryToolHasADisplayTitle guards the reason clients render raw
// snake_case identifiers: a missing Title. A tool added without one shows up
// as "alluredeck_some_tool" in the client's tool list.
func TestEveryToolHasADisplayTitle(t *testing.T) {
	for _, tool := range listRegisteredTools(t) {
		if tool.Title == "" {
			t.Errorf("tool %q has no Title; clients will display its raw identifier", tool.Name)
			continue
		}
		// The house style is "<Verb> AllureDeck <object>", so that a title is
		// self-identifying wherever a client shows it without server context.
		if !strings.Contains(tool.Title, "AllureDeck") {
			t.Errorf("tool %q title %q does not name AllureDeck", tool.Name, tool.Title)
		}
	}
}

// TestToolAnnotationsMatchBehaviour is what earns read-only tools their
// auto-approval in clients. A query tool missing ReadOnlyHint prompts the user
// as though it wrote something; a write tool claiming ReadOnlyHint is worse,
// because it would be auto-approved.
func TestToolAnnotationsMatchBehaviour(t *testing.T) {
	all := listRegisteredTools(t)
	if len(all) != 16 {
		t.Fatalf("registered %d tools, want 16; update this test if the set changed", len(all))
	}

	var readOnly, writes int
	for _, tool := range all {
		if tool.Annotations == nil {
			t.Errorf("tool %q has no Annotations; clients cannot tell whether it mutates", tool.Name)
			continue
		}
		isWrite := writeTools[tool.Name]
		switch {
		case isWrite:
			writes++
			if tool.Annotations.ReadOnlyHint {
				t.Errorf("write tool %q claims ReadOnlyHint; clients would auto-approve a mutation", tool.Name)
			}
			if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Errorf("tool %q should declare DestructiveHint=false; it only inserts a pending proposal", tool.Name)
			}
			if tool.Annotations.IdempotentHint {
				t.Errorf("tool %q claims idempotence, but calling it twice queues two proposals", tool.Name)
			}
		default:
			readOnly++
			if !tool.Annotations.ReadOnlyHint {
				t.Errorf("query tool %q does not declare ReadOnlyHint; clients will prompt on every call", tool.Name)
			}
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Errorf("tool %q should declare OpenWorldHint=false; it reaches only this deployment's database", tool.Name)
		}
	}

	if writes != len(writeTools) {
		t.Errorf("found %d write tools, want %d", writes, len(writeTools))
	}
	if readOnly != 13 {
		t.Errorf("found %d read-only tools, want 13", readOnly)
	}
}
