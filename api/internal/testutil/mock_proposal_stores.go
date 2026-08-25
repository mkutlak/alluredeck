package testutil

import (
	"context"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface checks.
var (
	_ store.DefectProposalStorer     = (*MockDefectProposalStore)(nil)
	_ store.KnownIssueProposalStorer = (*MockKnownIssueProposalStore)(nil)
	_ store.FlakyProposalStorer      = (*MockFlakyProposalStore)(nil)
)

// ---------------------------------------------------------------------------
// MockDefectProposalStore
// ---------------------------------------------------------------------------

// MockDefectProposalStore is a test double for store.DefectProposalStorer.
type MockDefectProposalStore struct {
	CreateFn               func(ctx context.Context, p *store.DefectProposal) (int64, error)
	GetFn                  func(ctx context.Context, id int64) (*store.DefectProposal, error)
	ListPendingFn          func(ctx context.Context, projectID int, limit int, cursor string) ([]*store.DefectProposal, string, error)
	ListFn                 func(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.DefectProposal, error)
	FindPendingDuplicateFn func(ctx context.Context, projectID int, fingerprintHash, proposedCategory string) (*store.DefectProposal, error)
	MarkReviewedFn         func(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error
}

func (m *MockDefectProposalStore) Create(ctx context.Context, p *store.DefectProposal) (int64, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, p)
	}
	return 1, nil
}

func (m *MockDefectProposalStore) Get(ctx context.Context, id int64) (*store.DefectProposal, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, nil
}

func (m *MockDefectProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.DefectProposal, string, error) {
	if m.ListPendingFn != nil {
		return m.ListPendingFn(ctx, projectID, limit, cursor)
	}
	return nil, "", nil
}

func (m *MockDefectProposalStore) List(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.DefectProposal, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID, status, limit)
	}
	return nil, nil
}

func (m *MockDefectProposalStore) FindPendingDuplicate(ctx context.Context, projectID int, fingerprintHash, proposedCategory string) (*store.DefectProposal, error) {
	if m.FindPendingDuplicateFn != nil {
		return m.FindPendingDuplicateFn(ctx, projectID, fingerprintHash, proposedCategory)
	}
	return nil, nil
}

func (m *MockDefectProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	if m.MarkReviewedFn != nil {
		return m.MarkReviewedFn(ctx, id, reviewedBy, status)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockKnownIssueProposalStore
// ---------------------------------------------------------------------------

// MockKnownIssueProposalStore is a test double for store.KnownIssueProposalStorer.
type MockKnownIssueProposalStore struct {
	CreateFn               func(ctx context.Context, p *store.KnownIssueProposal) (int64, error)
	GetFn                  func(ctx context.Context, id int64) (*store.KnownIssueProposal, error)
	ListPendingFn          func(ctx context.Context, projectID int, limit int, cursor string) ([]*store.KnownIssueProposal, string, error)
	ListFn                 func(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.KnownIssueProposal, error)
	FindPendingDuplicateFn func(ctx context.Context, projectID int, regexPattern string) (*store.KnownIssueProposal, error)
	MarkReviewedFn         func(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error
}

func (m *MockKnownIssueProposalStore) Create(ctx context.Context, p *store.KnownIssueProposal) (int64, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, p)
	}
	return 1, nil
}

func (m *MockKnownIssueProposalStore) Get(ctx context.Context, id int64) (*store.KnownIssueProposal, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, nil
}

func (m *MockKnownIssueProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.KnownIssueProposal, string, error) {
	if m.ListPendingFn != nil {
		return m.ListPendingFn(ctx, projectID, limit, cursor)
	}
	return nil, "", nil
}

func (m *MockKnownIssueProposalStore) List(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.KnownIssueProposal, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID, status, limit)
	}
	return nil, nil
}

func (m *MockKnownIssueProposalStore) FindPendingDuplicate(ctx context.Context, projectID int, regexPattern string) (*store.KnownIssueProposal, error) {
	if m.FindPendingDuplicateFn != nil {
		return m.FindPendingDuplicateFn(ctx, projectID, regexPattern)
	}
	return nil, nil
}

func (m *MockKnownIssueProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	if m.MarkReviewedFn != nil {
		return m.MarkReviewedFn(ctx, id, reviewedBy, status)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockFlakyProposalStore
// ---------------------------------------------------------------------------

// MockFlakyProposalStore is a test double for store.FlakyProposalStorer.
type MockFlakyProposalStore struct {
	CreateFn               func(ctx context.Context, p *store.FlakyProposal) (int64, error)
	GetFn                  func(ctx context.Context, id int64) (*store.FlakyProposal, error)
	ListPendingFn          func(ctx context.Context, projectID int, limit int, cursor string) ([]*store.FlakyProposal, string, error)
	ListFn                 func(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.FlakyProposal, error)
	FindPendingDuplicateFn func(ctx context.Context, projectID int, historyID string) (*store.FlakyProposal, error)
	MarkReviewedFn         func(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error
}

func (m *MockFlakyProposalStore) Create(ctx context.Context, p *store.FlakyProposal) (int64, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, p)
	}
	return 1, nil
}

func (m *MockFlakyProposalStore) Get(ctx context.Context, id int64) (*store.FlakyProposal, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, nil
}

func (m *MockFlakyProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.FlakyProposal, string, error) {
	if m.ListPendingFn != nil {
		return m.ListPendingFn(ctx, projectID, limit, cursor)
	}
	return nil, "", nil
}

func (m *MockFlakyProposalStore) List(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.FlakyProposal, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID, status, limit)
	}
	return nil, nil
}

func (m *MockFlakyProposalStore) FindPendingDuplicate(ctx context.Context, projectID int, historyID string) (*store.FlakyProposal, error) {
	if m.FindPendingDuplicateFn != nil {
		return m.FindPendingDuplicateFn(ctx, projectID, historyID)
	}
	return nil, nil
}

func (m *MockFlakyProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	if m.MarkReviewedFn != nil {
		return m.MarkReviewedFn(ctx, id, reviewedBy, status)
	}
	return nil
}
