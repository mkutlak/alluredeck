package tools_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

const publicBase = "https://app.example.com"

// proposeFn runs one propose_* tool core with an explicit TokenInfo; the
// in-memory transport bypasses the auth middleware that sets it in production.
type proposeFn func(req *mcpsdk.CallToolRequest, info *mcpauth.TokenInfo, stores *bootstrap.Stores, key []byte) (*mcpsdk.CallToolResult, error)

func classify(in tools.ProposeClassifyDefectInput) proposeFn {
	return func(req *mcpsdk.CallToolRequest, info *mcpauth.TokenInfo, stores *bootstrap.Stores, key []byte) (*mcpsdk.CallToolResult, error) {
		res, _, err := tools.ExecProposeClassifyDefectForTest(context.Background(), req, in, info, stores, zap.NewNop(), publicBase, key)
		return res, err
	}
}

func knownIssue(in tools.ProposeKnownIssueInput) proposeFn {
	return func(req *mcpsdk.CallToolRequest, info *mcpauth.TokenInfo, stores *bootstrap.Stores, key []byte) (*mcpsdk.CallToolResult, error) {
		res, _, err := tools.ExecProposeKnownIssueForTest(context.Background(), req, in, info, stores, zap.NewNop(), publicBase, key)
		return res, err
	}
}

func markFlaky(in tools.ProposeMarkFlakyInput) proposeFn {
	return func(req *mcpsdk.CallToolRequest, info *mcpauth.TokenInfo, stores *bootstrap.Stores, key []byte) (*mcpsdk.CallToolResult, error) {
		res, _, err := tools.ExecProposeMarkFlakyForTest(context.Background(), req, in, info, stores, zap.NewNop(), publicBase, key)
		return res, err
	}
}

// TestPropose_Refusals: every refused write explains itself, prompts for
// nothing and writes nothing.
func TestPropose_Refusals(t *testing.T) {
	defectIn := tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "abc", ProposedCategory: "product_bug"}
	// An API key owned by a configuration-file user has no users row, and
	// proposals.proposer_user_id is NOT NULL with a FK to users(id) (e655699).
	envUser := tokenInfo("editor", "true", "admin", 0)

	tests := []struct {
		name    string
		propose proposeFn
		info    *mcpauth.TokenInfo
		req     *mcpsdk.CallToolRequest
		key     []byte
		dupErr  bool
		wantErr []string
	}{
		// Even a viewer key that carries allow_mcp_writes is refused on role.
		{"viewer is forbidden", classify(defectIn), tokenInfo("viewer", "true", "1", 1), nil, nil, false, []string{"forbidden"}},
		{"editor without allow_mcp_writes is forbidden", classify(defectIn), tokenInfo("editor", "false", "2", 2), nil, nil, false, []string{"forbidden"}},
		// "23503" tells an agent nothing; the message must name the cause.
		{"caller without a users row is refused by name", markFlaky(markFlakyInput()), envUser, requestWithoutElicitation(), nil, false,
			[]string{"admin", "registered user"}},
		// Asking a user to approve a write that can never be recorded wastes their decision.
		{"caller without a users row is refused before confirming", markFlaky(markFlakyInput()), envUser, requestWithElicitation(),
			confirmSigningKey, false, []string{"registered user"}},
		// The more specific error first: a mistyped regex, not attribution.
		{"invalid regex is reported before attribution", knownIssue(tools.ProposeKnownIssueInput{ProjectID: 7, RegexPattern: "[unclosed"}),
			envUser, requestWithoutElicitation(), nil, false, []string{"regex"}},
		{"empty history_id", markFlaky(tools.ProposeMarkFlakyInput{ProjectID: 1, TestFullName: "pkg.TestFoo"}), editorInfo(), nil, nil, false,
			[]string{"history_id"}},
		// A failing duplicate check must not be read as "no duplicate".
		{"duplicate check failure is surfaced", markFlaky(markFlakyInput()), editorInfo(), nil, nil, true, []string{"duplicate"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stores, counter := countingStores(t, nil)
			if tc.dupErr {
				stores.FlakyProposals.(*testutil.MockFlakyProposalStore).FindPendingDuplicateFn = func(context.Context, int, string) (*store.FlakyProposal, error) {
					return nil, context.DeadlineExceeded
				}
			}
			res, err := tc.propose(tc.req, tc.info, stores, tc.key)
			if err == nil {
				t.Fatal("want a refusal, got success")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if res != nil && len(res.InputRequests) > 0 {
				t.Error("prompted for confirmation of a refused write")
			}
			if counter.creates() != 0 {
				t.Errorf("wrote %d proposals", counter.creates())
			}
		})
	}
}

func TestProposeClassifyDefect_Writes(t *testing.T) {
	stores, counter := countingStores(t, nil)
	var got *store.DefectProposal
	stores.DefectProposals.(*testutil.MockDefectProposalStore).CreateFn = func(_ context.Context, p *store.DefectProposal) (int64, error) {
		got = p
		return 99, nil
	}
	in := tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "deadbeef", ProposedCategory: "test_bug", Rationale: "test says so"}

	// nil request and signing key: no MCP client, so no confirmation gate.
	_, out, err := tools.ExecProposeClassifyDefectForTest(context.Background(), nil, in, editorInfo(), stores, zap.NewNop(), publicBase, nil)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	want := store.DefectProposal{ProjectID: 1, FingerprintHash: "deadbeef", ProposedCategory: "test_bug", Rationale: "test says so",
		ProposerUserID: 42, ProposerAPIKeyID: 7, Status: store.ProposalStatusPending}
	if got == nil || !reflect.DeepEqual(*got, want) {
		t.Errorf("stored proposal = %+v, want %+v", got, want)
	}
	if out != (tools.ProposeClassifyDefectOutput{ProposalID: 99, ReviewURL: publicBase + "/admin/proposals/defect/99"}) {
		t.Errorf("output = %+v, want proposal 99 with its review URL and no duplicate_of", out)
	}
	if ev := counter.audit.EventsByAction(store.AuditActionMCPProposeDefectClassify); len(ev) != 1 || ev[0].TargetType != "proposal" || ev[0].TargetID != "99" {
		t.Errorf("audit events = %+v, want one proposal/99 event", ev)
	}

	// The proposal is already stored when the audit write fails; the caller
	// must still hear about it.
	counter.audit.RecordErr = errors.New("audit db down")
	if _, _, err := tools.ExecProposeClassifyDefectForTest(context.Background(), nil, in, editorInfo(), stores, zap.NewNop(), publicBase, nil); err == nil || !strings.Contains(err.Error(), "audit") {
		t.Errorf("err = %v, want the audit failure reported", err)
	}
}

// TestProposeMarkFlaky_APIKeyCallerWritesResolvedUserID pins e655699: the
// API-key path used to parse the username — an email — as the proposer id,
// so every API-key proposal wrote 0 and failed the users(id) FK with 23503.
// The numeric id resolved at authentication must reach the store unchanged.
func TestProposeMarkFlaky_APIKeyCallerWritesResolvedUserID(t *testing.T) {
	stores, _ := countingStores(t, nil)
	var got *store.FlakyProposal
	stores.FlakyProposals = &testutil.MockFlakyProposalStore{CreateFn: func(_ context.Context, p *store.FlakyProposal) (int64, error) {
		got = p
		return 1, nil
	}}
	_, _, err := tools.ExecProposeMarkFlakyForTest(context.Background(), requestWithoutElicitation(), markFlakyInput(),
		tokenInfo("editor", "true", "engineer@example.com", 4242), stores, zap.NewNop(), publicBase, nil)
	if err != nil {
		t.Fatalf("propose_mark_flaky: %v", err)
	}
	if got == nil || got.ProposerUserID != 4242 || got.ProposerAPIKeyID != 7 {
		t.Fatalf("stored proposal = %+v, want proposer_user_id 4242 and api key 7", got)
	}
}

func TestProposeKnownIssue_DryRunCount(t *testing.T) {
	tenMixed := []string{
		"NullPointerException in foo", "timeout connecting to db", "NullPointerException in bar",
		"assertion failed: expected 1 got 2", "NullPointerException in baz", "connection refused",
		"index out of range", "unexpected EOF", "no such file or directory", "deadline exceeded",
	}
	fiveThousand := make([]string, 5000)
	for i := range fiveThousand {
		fiveThousand[i] = "fatal error: out of memory"
	}
	tests := []struct {
		name     string
		messages []string
		pattern  string
		want     int
	}{
		{"counts matching recent messages", tenMixed, "NullPointerException", 3},
		{"stops counting at the cap", fiveThousand, "fatal error", 1000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stores, _ := countingStores(t, tc.messages)
			_, out, err := tools.ExecProposeKnownIssueForTest(context.Background(), nil,
				tools.ProposeKnownIssueInput{ProjectID: 1, RegexPattern: tc.pattern}, editorInfo(), stores, zap.NewNop(), "", nil)
			if err != nil {
				t.Fatalf("propose: %v", err)
			}
			if out.DryRunMatchCount != tc.want {
				t.Errorf("dry_run_match_count = %d, want %d", out.DryRunMatchCount, tc.want)
			}
		})
	}
}

// TestPropose_DuplicatePending: an identical pending proposal means the call
// is a re-run, not a new finding. It is echoed via duplicate_of and nothing is
// written, audited, or (for known issues) dry-run scanned.
func TestPropose_DuplicatePending(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	type result struct {
		id  int64
		url string
		dup *tools.DuplicateProposal
	}
	tests := []struct {
		name    string
		arm     func(t *testing.T, s *bootstrap.Stores)
		propose func(s *bootstrap.Stores) (result, error)
		want    result
	}{
		{
			name: "classify defect",
			arm: func(t *testing.T, s *bootstrap.Stores) {
				s.DefectProposals.(*testutil.MockDefectProposalStore).FindPendingDuplicateFn = func(_ context.Context, projectID int, hash, category string) (*store.DefectProposal, error) {
					if projectID != 1 || hash != "deadbeef" || category != "test_bug" {
						t.Errorf("FindPendingDuplicate(%d, %q, %q), want (1, deadbeef, test_bug)", projectID, hash, category)
					}
					return &store.DefectProposal{ID: 77, CreatedAt: created}, nil
				}
			},
			propose: func(s *bootstrap.Stores) (result, error) {
				_, out, err := tools.ExecProposeClassifyDefectForTest(context.Background(), nil,
					tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "deadbeef", ProposedCategory: "test_bug"},
					editorInfo(), s, zap.NewNop(), publicBase, nil)
				return result{out.ProposalID, out.ReviewURL, out.DuplicateOf}, err
			},
			want: result{77, publicBase + "/admin/proposals/defect/77", nil},
		},
		{
			name: "known issue",
			arm: func(t *testing.T, s *bootstrap.Stores) {
				s.KnownIssueProposals.(*testutil.MockKnownIssueProposalStore).FindPendingDuplicateFn = func(_ context.Context, projectID int, pattern string) (*store.KnownIssueProposal, error) {
					if projectID != 1 || pattern != "NullPointerException" {
						t.Errorf("FindPendingDuplicate(%d, %q), want (1, NullPointerException)", projectID, pattern)
					}
					return &store.KnownIssueProposal{ID: 42, CreatedAt: created}, nil
				}
			},
			propose: func(s *bootstrap.Stores) (result, error) {
				_, out, err := tools.ExecProposeKnownIssueForTest(context.Background(), nil,
					tools.ProposeKnownIssueInput{ProjectID: 1, RegexPattern: "NullPointerException"},
					editorInfo(), s, zap.NewNop(), publicBase, nil)
				return result{out.ProposalID, out.ReviewURL, out.DuplicateOf}, err
			},
			want: result{42, publicBase + "/admin/proposals/known_issue/42", nil},
		},
		{
			name: "mark flaky",
			arm: func(t *testing.T, s *bootstrap.Stores) {
				s.FlakyProposals.(*testutil.MockFlakyProposalStore).FindPendingDuplicateFn = func(_ context.Context, projectID int, historyID string) (*store.FlakyProposal, error) {
					if projectID != 7 || historyID != "h-login" {
						t.Errorf("FindPendingDuplicate(%d, %q), want (7, h-login)", projectID, historyID)
					}
					return &store.FlakyProposal{ID: 88, CreatedAt: created}, nil
				}
			},
			propose: func(s *bootstrap.Stores) (result, error) {
				_, out, err := tools.ExecProposeMarkFlakyForTest(context.Background(), nil, markFlakyInput(),
					editorInfo(), s, zap.NewNop(), publicBase, nil)
				return result{out.ProposalID, out.ReviewURL, out.DuplicateOf}, err
			},
			want: result{88, publicBase + "/admin/proposals/flaky/88", nil},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stores, counter := countingStores(t, nil)
			tc.arm(t, stores)
			got, err := tc.propose(stores)
			if err != nil {
				t.Fatalf("propose: %v", err)
			}
			wantDup := tools.DuplicateProposal{ProposalID: tc.want.id, ReviewURL: tc.want.url, CreatedAt: created}
			if got.id != tc.want.id || got.url != tc.want.url || got.dup == nil || !reflect.DeepEqual(*got.dup, wantDup) {
				t.Errorf("output = (%d, %q, %+v), want (%d, %q, %+v)", got.id, got.url, got.dup, tc.want.id, tc.want.url, wantDup)
			}
			if counter.creates() != 0 || len(counter.audit.Events()) != 0 || counter.scans != 0 {
				t.Errorf("duplicate call wrote %d proposals, %d audit events, ran %d dry-run scans; want none",
					counter.creates(), len(counter.audit.Events()), counter.scans)
			}
		})
	}
}
