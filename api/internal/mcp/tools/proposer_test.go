package tools_test

import (
	"context"
	"strings"
	"testing"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// apiKeyInfo models what mcp/auth.go's verifyAPIKey puts in TokenInfo.Extra:
// "user_id" is the api_keys.username, which is an email address, while
// "user_db_id" carries the numeric users.id resolved during authentication.
func apiKeyInfo(email string, userDBID int64) *mcpauth.TokenInfo {
	return &mcpauth.TokenInfo{
		UserID: email,
		Scopes: []string{"editor"},
		Extra: map[string]any{
			"role":             "editor",
			"api_key_id":       int64(7),
			"allow_mcp_writes": "true",
			"username":         email,
			"user_id":          email,
			"user_db_id":       userDBID,
		},
	}
}

// TestProposal_APIKeyCaller_WritesResolvedUserID is the regression test for the
// bug this fixes: proposals.proposer_user_id is NOT NULL with a foreign key to
// users(id), and the API-key path used to derive it by parsing the username —
// an email — as an integer. That yielded 0 for every key, so every API-key
// proposal failed with a 23503 foreign-key violation.
//
// The numeric id must now reach the store unchanged.
func TestProposal_APIKeyCaller_WritesResolvedUserID(t *testing.T) {
	var got *store.FlakyProposal
	stores, _ := countingStores(t, nil)
	stores.FlakyProposals = &flakyCapture{onCreate: func(p *store.FlakyProposal) { got = p }}

	_, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), requestWithoutElicitation(), markFlakyInput(),
		apiKeyInfo("engineer@example.com", 4242), stores,
		zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("propose_mark_flaky: %v", err)
	}
	if got == nil {
		t.Fatal("no proposal reached the store")
	}
	if got.ProposerUserID != 4242 {
		t.Errorf("ProposerUserID = %d, want 4242; a non-matching value violates the users(id) foreign key",
			got.ProposerUserID)
	}
	if got.ProposerAPIKeyID != 7 {
		t.Errorf("ProposerAPIKeyID = %d, want 7", got.ProposerAPIKeyID)
	}
}

// TestProposal_UnresolvableUser_ReportsClearly covers the caller that genuinely
// has no users row: an API key owned by an env-config user. The write cannot
// succeed, so it must be refused with an explanation rather than left to fail
// as an opaque database constraint error.
func TestProposal_UnresolvableUser_ReportsClearly(t *testing.T) {
	stores, counter := countingStores(t, nil)

	_, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), requestWithoutElicitation(), markFlakyInput(),
		apiKeyInfo("admin", 0), stores,
		zap.NewNop(), "https://app.example.com", nil,
	)
	if err == nil {
		t.Fatal("a caller with no users row was allowed to propose, want an error")
	}
	if counter.flakyCreates != 0 {
		t.Errorf("wrote %d proposals despite having no user to attribute them to", counter.flakyCreates)
	}
	// The message has to name the cause; "23503" tells an agent nothing.
	for _, want := range []string{"admin", "registered user"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestProposal_UnresolvableUser_RefusedBeforeConfirming verifies the check runs
// before the confirmation prompt. Asking a user to approve a write that cannot
// be recorded wastes their decision.
func TestProposal_UnresolvableUser_RefusedBeforeConfirming(t *testing.T) {
	stores, _ := countingStores(t, nil)

	res, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), requestWithElicitation(), markFlakyInput(),
		apiKeyInfo("admin", 0), stores,
		zap.NewNop(), "https://app.example.com", confirmSigningKey,
	)
	if err == nil {
		t.Fatal("expected refusal, got success")
	}
	if res != nil && len(res.InputRequests) > 0 {
		t.Error("prompted for confirmation of a write that can never be recorded")
	}
}

// TestProposal_InvalidInput_ReportedBeforeAttribution keeps the more specific
// error first: a caller who mistyped a regex should hear about the regex, not
// about attribution.
func TestProposal_InvalidInput_ReportedBeforeAttribution(t *testing.T) {
	stores, _ := countingStores(t, nil)

	_, _, err := tools.ExecProposeKnownIssueForTest(
		context.Background(), requestWithoutElicitation(),
		tools.ProposeKnownIssueInput{ProjectID: 7, RegexPattern: "[unclosed"},
		apiKeyInfo("admin", 0), stores,
		zap.NewNop(), "https://app.example.com", nil,
	)
	if err == nil {
		t.Fatal("expected an error for an uncompilable regex")
	}
	if !strings.Contains(err.Error(), "regex") {
		t.Errorf("error %q does not report the regex problem", err)
	}
}

// flakyCapture records the proposal handed to Create so a test can assert on
// the exact row that would have been written.
type flakyCapture struct {
	onCreate func(*store.FlakyProposal)
}

var _ store.FlakyProposalStorer = (*flakyCapture)(nil)

func (m *flakyCapture) Create(_ context.Context, p *store.FlakyProposal) (int64, error) {
	m.onCreate(p)
	return 1, nil
}

func (m *flakyCapture) Get(context.Context, int64) (*store.FlakyProposal, error) { return nil, nil }

func (m *flakyCapture) ListPending(context.Context, int, int, string) ([]*store.FlakyProposal, string, error) {
	return nil, "", nil
}

func (m *flakyCapture) MarkReviewed(context.Context, int64, int64, store.ProposalStatus) error {
	return nil
}
