package tools_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// editorInfo returns a *mcpauth.TokenInfo with editor role and allow_mcp_writes=true.
func editorInfo() *mcpauth.TokenInfo {
	return &mcpauth.TokenInfo{
		UserID: "user-42",
		Scopes: []string{"editor"},
		Extra: map[string]any{
			"role":             "editor",
			"api_key_id":       int64(7),
			"allow_mcp_writes": "true",
			"username":         "editor-user",
			"user_id":          "42",
		},
	}
}

// viewerInfo returns a *mcpauth.TokenInfo with viewer role.
func viewerInfo() *mcpauth.TokenInfo {
	return &mcpauth.TokenInfo{
		UserID: "user-1",
		Scopes: []string{"viewer"},
		Extra: map[string]any{
			"role":             "viewer",
			"api_key_id":       int64(1),
			"allow_mcp_writes": "false",
			"username":         "viewer-user",
			"user_id":          "1",
		},
	}
}

// editorNoWriteInfo returns a *mcpauth.TokenInfo with editor role but allow_mcp_writes=false.
func editorNoWriteInfo() *mcpauth.TokenInfo {
	return &mcpauth.TokenInfo{
		UserID: "user-2",
		Scopes: []string{"editor"},
		Extra: map[string]any{
			"role":             "editor",
			"api_key_id":       int64(2),
			"allow_mcp_writes": "false",
			"username":         "editor-nowrite",
			"user_id":          "2",
		},
	}
}

// buildMutatingStores assembles a *bootstrap.Stores with the given mock fields.
func buildMutatingStores(
	defectProposals *testutil.MockDefectProposalStore,
	kiProposals *testutil.MockKnownIssueProposalStore,
	flakyProposals *testutil.MockFlakyProposalStore,
	testResults *testutil.MockTestResultStore,
	audit *testutil.MockAuditLogger,
) *bootstrap.Stores {
	return &bootstrap.Stores{
		DefectProposals:     defectProposals,
		KnownIssueProposals: kiProposals,
		FlakyProposals:      flakyProposals,
		TestResult:          testResults,
		Audit:               audit,
	}
}

// ---------------------------------------------------------------------------
// propose_classify_defect tests
// ---------------------------------------------------------------------------

// TestProposeClassifyDefect_RBAC_DeniesViewer verifies that a viewer role
// is rejected with a forbidden error.
func TestProposeClassifyDefect_RBAC_DeniesViewer(t *testing.T) {
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, _, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(),
		tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "abc", ProposedCategory: "product_bug"},
		viewerInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err == nil {
		t.Fatal("want error for viewer role, got nil")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("want error containing 'forbidden', got: %q", err.Error())
	}
}

// TestProposeClassifyDefect_RBAC_DeniesEditorWithoutFlag verifies that an
// editor without allow_mcp_writes is rejected.
func TestProposeClassifyDefect_RBAC_DeniesEditorWithoutFlag(t *testing.T) {
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, _, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(),
		tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "abc", ProposedCategory: "product_bug"},
		editorNoWriteInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err == nil {
		t.Fatal("want error for editor without allow_mcp_writes, got nil")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("want error containing 'forbidden', got: %q", err.Error())
	}
}

// TestProposeClassifyDefect_HappyPath_EditorWithFlag verifies that an editor
// with allow_mcp_writes=true successfully creates a proposal and records an
// audit event.
func TestProposeClassifyDefect_HappyPath_EditorWithFlag(t *testing.T) {
	var insertedProposal *store.DefectProposal

	defectStore := &testutil.MockDefectProposalStore{
		CreateFn: func(_ context.Context, p *store.DefectProposal) (int64, error) {
			cp := *p
			insertedProposal = &cp
			return 99, nil
		},
	}
	audit := testutil.NewMockAuditLogger()
	stores := buildMutatingStores(
		defectStore,
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		audit,
	)

	_, out, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(),
		tools.ProposeClassifyDefectInput{
			ProjectID:        1,
			FingerprintHash:  "deadbeef",
			ProposedCategory: "test_bug",
			Rationale:        "test says so",
		},
		editorInfo(),
		stores,
		zap.NewNop(),
		"https://app.example.com",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ProposalID != 99 {
		t.Errorf("want proposal_id=99, got %d", out.ProposalID)
	}
	if !strings.Contains(out.ReviewURL, "/admin/proposals/defect/99") {
		t.Errorf("unexpected review_url: %q", out.ReviewURL)
	}
	if insertedProposal == nil {
		t.Fatal("want proposal inserted, mock CreateFn not called")
	}
	if insertedProposal.FingerprintHash != "deadbeef" {
		t.Errorf("want hash=deadbeef, got %q", insertedProposal.FingerprintHash)
	}
	// Verify audit log.
	events := audit.EventsByAction(store.AuditActionMCPProposeDefectClassify)
	if len(events) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(events))
	}
	if events[0].TargetType != "proposal" {
		t.Errorf("want target_type=proposal, got %q", events[0].TargetType)
	}
	if events[0].TargetID != "99" {
		t.Errorf("want target_id=99, got %q", events[0].TargetID)
	}
}

// TestProposeClassifyDefect_AuditFailure_ReturnsError verifies that when the
// audit log fails the handler returns an error.
func TestProposeClassifyDefect_AuditFailure_ReturnsError(t *testing.T) {
	auditErr := errors.New("audit db down")
	audit := testutil.NewMockAuditLogger()
	audit.RecordErr = auditErr

	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		audit,
	)

	_, _, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(),
		tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "abc", ProposedCategory: "product_bug"},
		editorInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err == nil {
		t.Fatal("want error when audit fails, got nil")
	}
	if !strings.Contains(err.Error(), "audit") {
		t.Errorf("want error mentioning audit, got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// propose_known_issue tests
// ---------------------------------------------------------------------------

// TestProposeKnownIssue_InvalidRegex verifies that a malformed regex is rejected.
func TestProposeKnownIssue_InvalidRegex(t *testing.T) {
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, _, err := tools.ExecProposeKnownIssueForTest(
		context.Background(),
		tools.ProposeKnownIssueInput{
			ProjectID:    1,
			RegexPattern: "[invalid-regex",
		},
		editorInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err == nil {
		t.Fatal("want error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "regex") {
		t.Errorf("want error mentioning regex, got: %q", err.Error())
	}
}

// TestProposeKnownIssue_DryRunCount verifies that 3 of 10 messages match the
// regex and DryRunMatchCount=3 is returned.
func TestProposeKnownIssue_DryRunCount(t *testing.T) {
	messages := []string{
		"NullPointerException in foo",
		"timeout connecting to db",
		"NullPointerException in bar",
		"assertion failed: expected 1 got 2",
		"NullPointerException in baz",
		"connection refused",
		"index out of range",
		"unexpected EOF",
		"no such file or directory",
		"deadline exceeded",
	}

	trStore := &testutil.MockTestResultStore{
		ListRecentMessagesFn: func(_ context.Context, _ int64, _ int) ([]string, error) {
			return messages, nil
		},
	}
	kiStore := &testutil.MockKnownIssueProposalStore{}
	audit := testutil.NewMockAuditLogger()
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		kiStore,
		&testutil.MockFlakyProposalStore{},
		trStore,
		audit,
	)

	_, out, err := tools.ExecProposeKnownIssueForTest(
		context.Background(),
		tools.ProposeKnownIssueInput{
			ProjectID:    1,
			RegexPattern: "NullPointerException",
		},
		editorInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.DryRunMatchCount != 3 {
		t.Errorf("want DryRunMatchCount=3, got %d", out.DryRunMatchCount)
	}
}

// TestProposeKnownIssue_DryRunCap verifies that 5000 matching messages are
// capped at 1000 in DryRunMatchCount.
func TestProposeKnownIssue_DryRunCap(t *testing.T) {
	// Return 5000 matching messages.
	messages := make([]string, 5000)
	for i := range messages {
		messages[i] = "fatal error: out of memory"
	}

	trStore := &testutil.MockTestResultStore{
		ListRecentMessagesFn: func(_ context.Context, _ int64, _ int) ([]string, error) {
			return messages, nil
		},
	}
	audit := testutil.NewMockAuditLogger()
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		trStore,
		audit,
	)

	_, out, err := tools.ExecProposeKnownIssueForTest(
		context.Background(),
		tools.ProposeKnownIssueInput{
			ProjectID:    1,
			RegexPattern: "fatal error",
		},
		editorInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.DryRunMatchCount != 1000 {
		t.Errorf("want DryRunMatchCount=1000 (capped), got %d", out.DryRunMatchCount)
	}
}

// ---------------------------------------------------------------------------
// propose_mark_flaky tests
// ---------------------------------------------------------------------------

// TestProposeMarkFlaky_EmptyHistoryID verifies that an empty history_id returns
// an error mentioning history_id.
func TestProposeMarkFlaky_EmptyHistoryID(t *testing.T) {
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(),
		tools.ProposeMarkFlakyInput{
			ProjectID:    1,
			TestFullName: "pkg.TestFoo",
			HistoryID:    "",
		},
		editorInfo(),
		stores,
		zap.NewNop(),
		"",
	)
	if err == nil {
		t.Fatal("want error for empty history_id, got nil")
	}
	if !strings.Contains(err.Error(), "history_id") {
		t.Errorf("want error mentioning history_id, got: %q", err.Error())
	}
}
