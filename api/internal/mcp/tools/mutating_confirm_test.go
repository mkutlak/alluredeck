package tools_test

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// confirmSigningKey is the HMAC key used to authenticate RequestState in these
// tests. A non-empty key is what enables the confirmation gate at all.
var confirmSigningKey = []byte("confirmation-test-key")

// writeCounter records how many proposals and audit events actually reached the
// store, so a test can assert that a pending confirmation wrote nothing.
type writeCounter struct {
	flakyCreates int
	kiCreates    int
	audit        *testutil.MockAuditLogger
}

// countingStores builds a Stores whose proposal and audit writes are tallied.
func countingStores(t *testing.T, messages []string) (*bootstrap.Stores, *writeCounter) {
	t.Helper()
	audit := testutil.NewMockAuditLogger()
	c := &writeCounter{audit: audit}
	return &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{
			CreateFn: func(context.Context, *store.KnownIssueProposal) (int64, error) {
				c.kiCreates++
				return 55, nil
			},
		},
		FlakyProposals: &testutil.MockFlakyProposalStore{
			CreateFn: func(context.Context, *store.FlakyProposal) (int64, error) {
				c.flakyCreates++
				return 99, nil
			},
		},
		TestResult: &testutil.MockTestResultStore{
			ListRecentMessagesFn: func(context.Context, int64, int) ([]string, error) {
				return messages, nil
			},
		},
		Audit: audit,
	}, c
}

// requestWithElicitation builds a tool request from a client that advertises
// elicitation support, which is what makes the confirmation gate engage.
//
// Capabilities are supplied through the per-request _meta field, the transport
// the 2026-07-28 stateless protocol uses in place of an initialize handshake.
func requestWithElicitation() *mcpsdk.CallToolRequest {
	return &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Meta: mcpsdk.Meta{
				mcpsdk.MetaKeyProtocolVersion: "2026-07-28",
				mcpsdk.MetaKeyClientCapabilities: map[string]any{
					"elicitation": map[string]any{},
				},
			},
		},
	}
}

// requestWithoutElicitation builds a request from a client that cannot answer a
// prompt — a CI pipeline driving the server with an API key.
func requestWithoutElicitation() *mcpsdk.CallToolRequest {
	return &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Meta: mcpsdk.Meta{
				mcpsdk.MetaKeyProtocolVersion:    "2026-07-28",
				mcpsdk.MetaKeyClientCapabilities: map[string]any{},
			},
		},
	}
}

// answer builds the retry-leg request carrying the user's response to a
// previously issued confirmation.
func answer(state, action string) *mcpsdk.CallToolRequest {
	req := requestWithElicitation()
	req.Params.RequestState = state
	req.Params.InputResponses = mcpsdk.InputResponseMap{
		"confirm": &mcpsdk.ElicitResult{Action: action},
	}
	return req
}

// markFlakyInput is the standard argument set used across these tests.
func markFlakyInput() tools.ProposeMarkFlakyInput {
	return tools.ProposeMarkFlakyInput{
		ProjectID:    7,
		TestFullName: "pkg.LoginTest",
		HistoryID:    "h-login",
	}
}

func callMarkFlaky(t *testing.T, req *mcpsdk.CallToolRequest, in tools.ProposeMarkFlakyInput, stores *bootstrap.Stores) (*mcpsdk.CallToolResult, tools.ProposeMarkFlakyOutput, error) {
	t.Helper()
	return tools.ExecProposeMarkFlakyForTest(
		context.Background(), req, in, editorInfo(), stores,
		zap.NewNop(), "https://app.example.com", confirmSigningKey,
	)
}

// TestConfirmation_FirstCallAsksAndWritesNothing is the core guarantee: a write
// tool that has not been confirmed must not touch the database.
func TestConfirmation_FirstCallAsksAndWritesNothing(t *testing.T) {
	stores, counter := countingStores(t, nil)

	res, _, err := callMarkFlaky(t, requestWithElicitation(), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if res == nil || len(res.InputRequests) == 0 {
		t.Fatal("first call returned no input request, want a confirmation prompt")
	}
	if res.RequestState == "" {
		t.Fatal("first call returned no RequestState, so the answer could not be authenticated")
	}
	if counter.flakyCreates != 0 {
		t.Fatalf("first call created %d proposals, want 0", counter.flakyCreates)
	}
	if n := len(counter.audit.Events()); n != 0 {
		t.Fatalf("first call recorded %d audit events, want 0", n)
	}

	// Content and inputRequests are mutually exclusive on the wire; the SDK
	// rejects a result carrying both as a server bug.
	if len(res.Content) != 0 || res.StructuredContent != nil {
		t.Fatalf("confirmation result carries content (%d blocks, structured=%v), want none",
			len(res.Content), res.StructuredContent != nil)
	}

	prompt := promptText(t, res)
	for _, want := range []string{"pkg.LoginTest", "h-login"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt %q does not mention %q, so the user cannot see what they are approving", prompt, want)
		}
	}
}

// TestConfirmation_AcceptWritesExactlyOnce covers the happy path.
func TestConfirmation_AcceptWritesExactlyOnce(t *testing.T) {
	stores, counter := countingStores(t, nil)

	first, _, err := callMarkFlaky(t, requestWithElicitation(), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	_, out, err := callMarkFlaky(t, answer(first.RequestState, "accept"), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("retry after accept: %v", err)
	}
	if counter.flakyCreates != 1 {
		t.Fatalf("created %d proposals, want exactly 1", counter.flakyCreates)
	}
	if out.ProposalID != 99 {
		t.Errorf("ProposalID = %d, want 99", out.ProposalID)
	}
	// The review link must be absolute: it is handed to a human to open.
	if !strings.HasPrefix(out.ReviewURL, "https://app.example.com/") {
		t.Errorf("ReviewURL = %q, want an absolute URL under the public base", out.ReviewURL)
	}
}

// TestConfirmation_DeclineWritesNothing verifies that a user veto is honoured
// and reported as success rather than as a retryable error.
func TestConfirmation_DeclineWritesNothing(t *testing.T) {
	for _, action := range []string{"decline", "cancel"} {
		t.Run(action, func(t *testing.T) {
			stores, counter := countingStores(t, nil)

			first, _, err := callMarkFlaky(t, requestWithElicitation(), markFlakyInput(), stores)
			if err != nil {
				t.Fatalf("first call: %v", err)
			}

			res, _, err := callMarkFlaky(t, answer(first.RequestState, action), markFlakyInput(), stores)
			if err != nil {
				t.Fatalf("retry after %s: unexpected error %v", action, err)
			}
			if counter.flakyCreates != 0 {
				t.Fatalf("%s created %d proposals, want 0", action, counter.flakyCreates)
			}
			if res == nil || res.IsError {
				t.Fatalf("%s produced an error result; a user veto is not a failure", action)
			}
		})
	}
}

// TestConfirmation_ForgedStateRejected is the security case. RequestState makes
// a round trip through a client that could alter it, and the server is
// stateless, so the signature is the only thing preventing a caller from
// skipping the prompt or swapping the payload after approval.
func TestConfirmation_ForgedStateRejected(t *testing.T) {
	stores, counter := countingStores(t, nil)
	first, _, err := callMarkFlaky(t, requestWithElicitation(), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	valid := first.RequestState

	// A state minted for a different tool, which must not authorise this one.
	otherStores, _ := countingStores(t, nil)
	otherTool, _, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(), requestWithElicitation(),
		tools.ProposeClassifyDefectInput{ProjectID: 7, FingerprintHash: "abc", ProposedCategory: "product_bug"},
		editorInfo(), otherStores, zap.NewNop(), "https://app.example.com", confirmSigningKey,
	)
	if err != nil {
		t.Fatalf("minting a foreign-tool state: %v", err)
	}

	tests := []struct {
		name  string
		state string
		in    tools.ProposeMarkFlakyInput
	}{
		{"fabricated state", "not-a-real-token", markFlakyInput()},
		{"truncated state", valid[:len(valid)-8], markFlakyInput()},
		{"state for another tool", otherTool.RequestState, markFlakyInput()},
		{
			// Approve a narrow change, then retry with a different one.
			name:  "arguments swapped after approval",
			state: valid,
			in:    tools.ProposeMarkFlakyInput{ProjectID: 7, TestFullName: "pkg.PaymentTest", HistoryID: "h-pay"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := counter.flakyCreates
			_, _, err := callMarkFlaky(t, answer(tc.state, "accept"), tc.in, stores)
			if err == nil {
				t.Fatal("forged confirmation accepted, want error")
			}
			if counter.flakyCreates != before {
				t.Fatalf("forged confirmation wrote a proposal (%d -> %d)", before, counter.flakyCreates)
			}
		})
	}
}

// TestConfirmation_ForeignKeyRejected covers a state signed by a different
// deployment — or a replica rolled to a new signing key.
func TestConfirmation_ForeignKeyRejected(t *testing.T) {
	stores, counter := countingStores(t, nil)
	first, _, err := callMarkFlaky(t, requestWithElicitation(), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	_, _, err = tools.ExecProposeMarkFlakyForTest(
		context.Background(), answer(first.RequestState, "accept"), markFlakyInput(),
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", []byte("a-different-key"),
	)
	if err == nil {
		t.Fatal("state signed with another key was accepted, want error")
	}
	if counter.flakyCreates != 0 {
		t.Fatalf("created %d proposals, want 0", counter.flakyCreates)
	}
}

// TestConfirmation_HeadlessClientWritesDirectly is the compatibility guarantee.
// A CI pipeline holding an API key cannot answer a prompt, so it must keep the
// pre-confirmation behaviour rather than failing.
func TestConfirmation_HeadlessClientWritesDirectly(t *testing.T) {
	stores, counter := countingStores(t, nil)

	res, out, err := callMarkFlaky(t, requestWithoutElicitation(), markFlakyInput(), stores)
	if err != nil {
		t.Fatalf("headless call: %v", err)
	}
	if res != nil && len(res.InputRequests) > 0 {
		t.Fatal("headless client was asked to confirm, which it cannot answer")
	}
	if counter.flakyCreates != 1 {
		t.Fatalf("created %d proposals, want exactly 1", counter.flakyCreates)
	}
	if out.ProposalID != 99 {
		t.Errorf("ProposalID = %d, want 99", out.ProposalID)
	}
}

// TestConfirmation_DisabledWithoutSigningKey verifies that a deployment with no
// signing key writes directly rather than issuing prompts it cannot verify.
func TestConfirmation_DisabledWithoutSigningKey(t *testing.T) {
	stores, counter := countingStores(t, nil)

	res, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), requestWithElicitation(), markFlakyInput(),
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("call without signing key: %v", err)
	}
	if res != nil && len(res.InputRequests) > 0 {
		t.Fatal("issued a confirmation prompt with no key to authenticate the answer")
	}
	if counter.flakyCreates != 1 {
		t.Fatalf("created %d proposals, want exactly 1", counter.flakyCreates)
	}
}

// TestConfirmation_KnownIssuePromptShowsDryRunCount checks the reason this gate
// exists for propose_known_issue: the operator must see how much the pattern
// would sweep up before agreeing to it.
func TestConfirmation_KnownIssuePromptShowsDryRunCount(t *testing.T) {
	// Six of eight sampled messages match, so the breadth warning must fire.
	messages := []string{
		"NullPointerException at A", "NullPointerException at B",
		"NullPointerException at C", "NullPointerException at D",
		"NullPointerException at E", "NullPointerException at F",
		"timeout waiting for element", "connection refused",
	}
	stores, counter := countingStores(t, messages)

	res, _, err := tools.ExecProposeKnownIssueForTest(
		context.Background(), requestWithElicitation(),
		tools.ProposeKnownIssueInput{ProjectID: 7, RegexPattern: "NullPointerException", ProposedCategory: "product_bug"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", confirmSigningKey,
	)
	if err != nil {
		t.Fatalf("propose_known_issue: %v", err)
	}
	if counter.kiCreates != 0 {
		t.Fatalf("prompt stage created %d proposals, want 0", counter.kiCreates)
	}

	prompt := promptText(t, res)
	if !strings.Contains(prompt, "6") {
		t.Errorf("prompt %q omits the dry-run match count", prompt)
	}
	if !strings.Contains(prompt, "over half") {
		t.Errorf("prompt %q omits the breadth warning for a pattern matching 6 of 8", prompt)
	}
}

// promptText extracts the confirmation message from an input-required result.
func promptText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("result is nil, want a confirmation prompt")
	}
	req, ok := res.InputRequests["confirm"]
	if !ok {
		t.Fatalf("result has no %q input request (keys: %v)", "confirm", keysOf(res.InputRequests))
	}
	elicit, ok := req.(*mcpsdk.ElicitParams)
	if !ok {
		t.Fatalf("input request is %T, want *mcp.ElicitParams", req)
	}
	return elicit.Message
}

func keysOf(m mcpsdk.InputRequestMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
