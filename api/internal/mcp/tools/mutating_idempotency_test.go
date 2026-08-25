package tools_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// ---------------------------------------------------------------------------
// Idempotency: a pending duplicate short-circuits the write and echoes the
// existing proposal via duplicate_of, rather than inserting a second one.
// ---------------------------------------------------------------------------

func TestProposeClassifyDefect_DuplicatePending_NoWrite(t *testing.T) {
	dupCreatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	createCalled := false

	defectStore := &testutil.MockDefectProposalStore{
		FindPendingDuplicateFn: func(_ context.Context, projectID int, fingerprintHash, proposedCategory string) (*store.DefectProposal, error) {
			if projectID != 1 || fingerprintHash != "deadbeef" || proposedCategory != "test_bug" {
				t.Fatalf("FindPendingDuplicate called with unexpected args: project=%d hash=%q category=%q", projectID, fingerprintHash, proposedCategory)
			}
			return &store.DefectProposal{ID: 77, CreatedAt: dupCreatedAt}, nil
		},
		CreateFn: func(context.Context, *store.DefectProposal) (int64, error) {
			createCalled = true
			return 999, nil
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
		context.Background(), nil,
		tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "deadbeef", ProposedCategory: "test_bug"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled {
		t.Error("Create was called despite a pending duplicate existing")
	}
	if len(audit.Events()) != 0 {
		t.Errorf("audit recorded %d events, want 0 for a duplicate that wrote nothing", len(audit.Events()))
	}
	if out.ProposalID != 77 {
		t.Errorf("ProposalID = %d, want 77 (the existing proposal)", out.ProposalID)
	}
	if out.DuplicateOf == nil {
		t.Fatal("want DuplicateOf set, got nil")
	}
	if out.DuplicateOf.ProposalID != 77 {
		t.Errorf("DuplicateOf.ProposalID = %d, want 77", out.DuplicateOf.ProposalID)
	}
	if !out.DuplicateOf.CreatedAt.Equal(dupCreatedAt) {
		t.Errorf("DuplicateOf.CreatedAt = %v, want %v", out.DuplicateOf.CreatedAt, dupCreatedAt)
	}
	if out.DuplicateOf.ReviewURL != out.ReviewURL {
		t.Errorf("DuplicateOf.ReviewURL = %q, want it to match top-level ReviewURL %q", out.DuplicateOf.ReviewURL, out.ReviewURL)
	}
}

func TestProposeClassifyDefect_NoPendingDuplicate_WritesNormally(t *testing.T) {
	var created *store.DefectProposal
	defectStore := &testutil.MockDefectProposalStore{
		FindPendingDuplicateFn: func(context.Context, int, string, string) (*store.DefectProposal, error) {
			return nil, nil
		},
		CreateFn: func(_ context.Context, p *store.DefectProposal) (int64, error) {
			cp := *p
			created = &cp
			return 5, nil
		},
	}
	stores := buildMutatingStores(
		defectStore,
		&testutil.MockKnownIssueProposalStore{},
		&testutil.MockFlakyProposalStore{},
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, out, err := tools.ExecProposeClassifyDefectForTest(
		context.Background(), nil,
		tools.ProposeClassifyDefectInput{ProjectID: 1, FingerprintHash: "deadbeef", ProposedCategory: "test_bug"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil {
		t.Fatal("want Create called when no pending duplicate exists")
	}
	if out.ProposalID != 5 {
		t.Errorf("ProposalID = %d, want 5", out.ProposalID)
	}
	if out.DuplicateOf != nil {
		t.Errorf("DuplicateOf = %+v, want nil on a genuine insert", out.DuplicateOf)
	}
}

func TestProposeKnownIssue_DuplicatePending_NoWrite(t *testing.T) {
	dupCreatedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	createCalled, dryRunCalled := false, false

	kiStore := &testutil.MockKnownIssueProposalStore{
		FindPendingDuplicateFn: func(_ context.Context, projectID int, regexPattern string) (*store.KnownIssueProposal, error) {
			if projectID != 1 || regexPattern != "NullPointerException" {
				t.Fatalf("FindPendingDuplicate called with unexpected args: project=%d pattern=%q", projectID, regexPattern)
			}
			return &store.KnownIssueProposal{ID: 42, CreatedAt: dupCreatedAt, DryRunMatchCount: 3}, nil
		},
		CreateFn: func(context.Context, *store.KnownIssueProposal) (int64, error) {
			createCalled = true
			return 999, nil
		},
	}
	trStore := &testutil.MockTestResultStore{
		ListRecentMessagesFn: func(context.Context, int64, int) ([]string, error) {
			dryRunCalled = true
			return nil, nil
		},
	}
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		kiStore,
		&testutil.MockFlakyProposalStore{},
		trStore,
		testutil.NewMockAuditLogger(),
	)

	_, out, err := tools.ExecProposeKnownIssueForTest(
		context.Background(), nil,
		tools.ProposeKnownIssueInput{ProjectID: 1, RegexPattern: "NullPointerException"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled {
		t.Error("Create was called despite a pending duplicate existing")
	}
	if dryRunCalled {
		t.Error("dry-run message scan ran despite a pending duplicate existing")
	}
	if out.ProposalID != 42 {
		t.Errorf("ProposalID = %d, want 42", out.ProposalID)
	}
	if out.DuplicateOf == nil || out.DuplicateOf.ProposalID != 42 {
		t.Fatalf("want DuplicateOf.ProposalID=42, got %+v", out.DuplicateOf)
	}
	if !out.DuplicateOf.CreatedAt.Equal(dupCreatedAt) {
		t.Errorf("DuplicateOf.CreatedAt = %v, want %v", out.DuplicateOf.CreatedAt, dupCreatedAt)
	}
}

func TestProposeMarkFlaky_DuplicatePending_NoWrite(t *testing.T) {
	dupCreatedAt := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	createCalled := false

	flakyStore := &testutil.MockFlakyProposalStore{
		FindPendingDuplicateFn: func(_ context.Context, projectID int, historyID string) (*store.FlakyProposal, error) {
			if projectID != 7 || historyID != "h-login" {
				t.Fatalf("FindPendingDuplicate called with unexpected args: project=%d history_id=%q", projectID, historyID)
			}
			return &store.FlakyProposal{ID: 88, CreatedAt: dupCreatedAt}, nil
		},
		CreateFn: func(context.Context, *store.FlakyProposal) (int64, error) {
			createCalled = true
			return 999, nil
		},
	}
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		flakyStore,
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, out, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), nil,
		tools.ProposeMarkFlakyInput{ProjectID: 7, TestFullName: "pkg.LoginTest", HistoryID: "h-login"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled {
		t.Error("Create was called despite a pending duplicate existing")
	}
	if out.ProposalID != 88 {
		t.Errorf("ProposalID = %d, want 88", out.ProposalID)
	}
	if out.DuplicateOf == nil || out.DuplicateOf.ProposalID != 88 {
		t.Fatalf("want DuplicateOf.ProposalID=88, got %+v", out.DuplicateOf)
	}
	if !out.DuplicateOf.CreatedAt.Equal(dupCreatedAt) {
		t.Errorf("DuplicateOf.CreatedAt = %v, want %v", out.DuplicateOf.CreatedAt, dupCreatedAt)
	}
}

// TestProposeDuplicateCheck_ErrorPropagates verifies that a store error during
// the duplicate check is surfaced rather than silently treated as "no duplicate".
func TestProposeDuplicateCheck_ErrorPropagates(t *testing.T) {
	flakyStore := &testutil.MockFlakyProposalStore{
		FindPendingDuplicateFn: func(context.Context, int, string) (*store.FlakyProposal, error) {
			return nil, context.DeadlineExceeded
		},
	}
	stores := buildMutatingStores(
		&testutil.MockDefectProposalStore{},
		&testutil.MockKnownIssueProposalStore{},
		flakyStore,
		&testutil.MockTestResultStore{},
		testutil.NewMockAuditLogger(),
	)

	_, _, err := tools.ExecProposeMarkFlakyForTest(
		context.Background(), nil,
		tools.ProposeMarkFlakyInput{ProjectID: 7, TestFullName: "pkg.LoginTest", HistoryID: "h-login"},
		editorInfo(), stores, zap.NewNop(), "https://app.example.com", nil,
	)
	if err == nil {
		t.Fatal("want error when the duplicate check itself fails")
	}
}
