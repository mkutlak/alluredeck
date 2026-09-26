package tools_test

import (
	"context"
	"strings"
	"testing"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// confirmSigningKey authenticates RequestState; a non-empty key is what
// enables the confirmation gate at all.
var confirmSigningKey = []byte("confirmation-test-key")

// tokenInfo mirrors what mcp/auth.go's verifiers put in TokenInfo.Extra:
// "user_id" is a display label (an email on the API-key path) and
// "user_db_id" the numeric users.id, 0 for an env-config user.
func tokenInfo(role, allowWrites, label string, userDBID int64) *mcpauth.TokenInfo {
	return &mcpauth.TokenInfo{UserID: label, Scopes: []string{role}, Extra: map[string]any{
		"role": role, "api_key_id": int64(7), "allow_mcp_writes": allowWrites,
		"username": label, "user_id": label, "user_db_id": userDBID,
	}}
}

func editorInfo() *mcpauth.TokenInfo { return tokenInfo("editor", "true", "42", 42) }

// writeCounter tallies what actually reached the stores, so a test can assert
// that a pending or refused write touched nothing.
type writeCounter struct {
	defectCreates, kiCreates, flakyCreates, scans int
	audit                                         *testutil.MockAuditLogger
}

func (c *writeCounter) creates() int { return c.defectCreates + c.kiCreates + c.flakyCreates }

// countingStores builds proposal stores whose creates (ids 99/55/99) and
// dry-run scans are tallied; messages is the dry-run sample.
func countingStores(t *testing.T, messages []string) (*bootstrap.Stores, *writeCounter) {
	t.Helper()
	c := &writeCounter{audit: testutil.NewMockAuditLogger()}
	return &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{CreateFn: func(context.Context, *store.DefectProposal) (int64, error) {
			c.defectCreates++
			return 99, nil
		}},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{CreateFn: func(context.Context, *store.KnownIssueProposal) (int64, error) {
			c.kiCreates++
			return 55, nil
		}},
		FlakyProposals: &testutil.MockFlakyProposalStore{CreateFn: func(context.Context, *store.FlakyProposal) (int64, error) {
			c.flakyCreates++
			return 99, nil
		}},
		TestResult: &testutil.MockTestResultStore{ListRecentMessagesFn: func(context.Context, int64, int) ([]string, error) {
			c.scans++
			return messages, nil
		}},
		Audit: c.audit,
	}, c
}

// requestWithElicitation is a request from a client that advertises
// elicitation, which is what engages the confirmation gate. Capabilities ride
// in _meta, the 2026-07-28 stateless protocol's stand-in for initialize.
func requestWithElicitation() *mcpsdk.CallToolRequest {
	return &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Meta: mcpsdk.Meta{
		mcpsdk.MetaKeyProtocolVersion:    "2026-07-28",
		mcpsdk.MetaKeyClientCapabilities: map[string]any{"elicitation": map[string]any{}},
	}}}
}

// requestWithoutElicitation is a client that cannot answer a prompt — a CI
// pipeline driving the server with an API key.
func requestWithoutElicitation() *mcpsdk.CallToolRequest {
	return &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Meta: mcpsdk.Meta{
		mcpsdk.MetaKeyProtocolVersion:    "2026-07-28",
		mcpsdk.MetaKeyClientCapabilities: map[string]any{},
	}}}
}

// answer is the retry leg carrying the user's response to a confirmation.
func answer(state, action string) *mcpsdk.CallToolRequest {
	req := requestWithElicitation()
	req.Params.RequestState = state
	req.Params.InputResponses = mcpsdk.InputResponseMap{"confirm": &mcpsdk.ElicitResult{Action: action}}
	return req
}

func markFlakyInput() tools.ProposeMarkFlakyInput {
	return tools.ProposeMarkFlakyInput{ProjectID: 7, TestFullName: "pkg.LoginTest", HistoryID: "h-login"}
}

func callMarkFlaky(req *mcpsdk.CallToolRequest, in tools.ProposeMarkFlakyInput, stores *bootstrap.Stores, key []byte) (*mcpsdk.CallToolResult, tools.ProposeMarkFlakyOutput, error) {
	return tools.ExecProposeMarkFlakyForTest(context.Background(), req, in, editorInfo(), stores,
		zap.NewNop(), "https://app.example.com", key)
}

// promptText extracts the confirmation message from an input-required result.
func promptText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("result is nil, want a confirmation prompt")
	}
	elicit, ok := res.InputRequests["confirm"].(*mcpsdk.ElicitParams)
	if !ok {
		t.Fatalf("input requests = %v, want a %q *mcp.ElicitParams", res.InputRequests, "confirm")
	}
	return elicit.Message
}

// TestConfirmation_FirstCallAsksAndWritesNothing is the core guarantee: a write
// tool that has not been confirmed must not touch the database.
func TestConfirmation_FirstCallAsksAndWritesNothing(t *testing.T) {
	stores, counter := countingStores(t, nil)
	res, _, err := callMarkFlaky(requestWithElicitation(), markFlakyInput(), stores, confirmSigningKey)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	prompt := promptText(t, res)
	if res.RequestState == "" {
		t.Fatal("no RequestState, so the answer could not be authenticated")
	}
	if counter.creates() != 0 || len(counter.audit.Events()) != 0 {
		t.Fatalf("first call wrote %d proposals and %d audit events, want none", counter.creates(), len(counter.audit.Events()))
	}
	// Content and inputRequests are mutually exclusive on the wire.
	if len(res.Content) != 0 || res.StructuredContent != nil {
		t.Fatalf("confirmation result carries content (%d blocks, structured=%v), want none", len(res.Content), res.StructuredContent != nil)
	}
	for _, want := range []string{"pkg.LoginTest", "h-login"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt %q does not mention %q, so the user cannot see what they approve", prompt, want)
		}
	}
}

// TestConfirmation_Answer: an accept writes exactly once; a veto writes nothing
// and is reported as success, not as a retryable error.
func TestConfirmation_Answer(t *testing.T) {
	for action, wantCreates := range map[string]int{"accept": 1, "decline": 0, "cancel": 0} {
		t.Run(action, func(t *testing.T) {
			stores, counter := countingStores(t, nil)
			first, _, err := callMarkFlaky(requestWithElicitation(), markFlakyInput(), stores, confirmSigningKey)
			if err != nil {
				t.Fatalf("first call: %v", err)
			}
			res, out, err := callMarkFlaky(answer(first.RequestState, action), markFlakyInput(), stores, confirmSigningKey)
			if err != nil {
				t.Fatalf("retry after %s: %v", action, err)
			}
			if counter.flakyCreates != wantCreates {
				t.Fatalf("created %d proposals, want %d", counter.flakyCreates, wantCreates)
			}
			if wantCreates == 0 {
				if res == nil || res.IsError {
					t.Errorf("%s produced an error result; a user veto is not a failure", action)
				}
				return
			}
			// The review link is handed to a human, so it must be absolute.
			if out.ProposalID != 99 || out.ReviewURL != "https://app.example.com/admin/proposals/flaky/99" {
				t.Errorf("output = %+v, want proposal 99 with an absolute review URL", out)
			}
		})
	}
}

// TestConfirmation_ForgedStateRejected is the security case. RequestState makes
// a round trip through a client that could alter it and the server is
// stateless, so the signature is all that stops a caller from skipping the
// prompt or swapping the payload after approval.
func TestConfirmation_ForgedStateRejected(t *testing.T) {
	stores, counter := countingStores(t, nil)
	first, _, err := callMarkFlaky(requestWithElicitation(), markFlakyInput(), stores, confirmSigningKey)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	valid := first.RequestState
	otherStores, _ := countingStores(t, nil)
	otherTool, _, err := tools.ExecProposeClassifyDefectForTest(context.Background(), requestWithElicitation(),
		tools.ProposeClassifyDefectInput{ProjectID: 7, FingerprintHash: "abc", ProposedCategory: "product_bug"},
		editorInfo(), otherStores, zap.NewNop(), "https://app.example.com", confirmSigningKey)
	if err != nil {
		t.Fatalf("minting a foreign-tool state: %v", err)
	}

	tests := []struct {
		name  string
		state string
		in    tools.ProposeMarkFlakyInput
		key   []byte
	}{
		{"fabricated state", "not-a-real-token", markFlakyInput(), confirmSigningKey},
		{"truncated state", valid[:len(valid)-8], markFlakyInput(), confirmSigningKey},
		{"state for another tool", otherTool.RequestState, markFlakyInput(), confirmSigningKey},
		// Approve a narrow change, then retry with a different one.
		{"arguments swapped after approval", valid, tools.ProposeMarkFlakyInput{ProjectID: 7, TestFullName: "pkg.PaymentTest", HistoryID: "h-pay"}, confirmSigningKey},
		// Another deployment, or a replica rolled to a new signing key.
		{"state signed with another key", valid, markFlakyInput(), []byte("a-different-key")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := callMarkFlaky(answer(tc.state, "accept"), tc.in, stores, tc.key); err == nil {
				t.Fatal("forged confirmation accepted, want error")
			}
			if counter.flakyCreates != 0 {
				t.Fatalf("forged confirmation wrote %d proposals", counter.flakyCreates)
			}
		})
	}
}

// TestConfirmation_WritesDirectly: a headless client cannot answer a prompt
// and keeps the pre-confirmation behaviour, and a deployment with no signing
// key cannot verify an answer so it never asks.
func TestConfirmation_WritesDirectly(t *testing.T) {
	tests := []struct {
		name string
		req  *mcpsdk.CallToolRequest
		key  []byte
	}{
		{"headless client", requestWithoutElicitation(), confirmSigningKey},
		{"no signing key", requestWithElicitation(), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stores, counter := countingStores(t, nil)
			res, out, err := callMarkFlaky(tc.req, markFlakyInput(), stores, tc.key)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if res != nil && len(res.InputRequests) > 0 {
				t.Fatal("issued a confirmation prompt")
			}
			if counter.flakyCreates != 1 || out.ProposalID != 99 {
				t.Fatalf("created %d proposals (id %d), want exactly 1 (id 99)", counter.flakyCreates, out.ProposalID)
			}
		})
	}
}

// TestConfirmation_KnownIssuePromptShowsDryRunCount: the operator must see how
// much a pattern would sweep up before agreeing to it.
func TestConfirmation_KnownIssuePromptShowsDryRunCount(t *testing.T) {
	messages := []string{
		"NullPointerException at A", "NullPointerException at B", "NullPointerException at C",
		"NullPointerException at D", "NullPointerException at E", "NullPointerException at F",
		"timeout waiting for element", "connection refused",
	}
	stores, counter := countingStores(t, messages)
	res, _, err := tools.ExecProposeKnownIssueForTest(context.Background(), requestWithElicitation(),
		tools.ProposeKnownIssueInput{ProjectID: 7, RegexPattern: "NullPointerException", ProposedCategory: "product_bug"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", confirmSigningKey)
	if err != nil {
		t.Fatalf("propose_known_issue: %v", err)
	}
	if counter.kiCreates != 0 {
		t.Fatalf("prompt stage created %d proposals, want 0", counter.kiCreates)
	}
	// Six of eight sampled messages match, so the breadth warning must fire.
	if prompt := promptText(t, res); !strings.Contains(prompt, "6") || !strings.Contains(prompt, "over half") {
		t.Errorf("prompt %q omits the dry-run count or the breadth warning", prompt)
	}
}
