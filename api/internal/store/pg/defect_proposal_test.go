package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestDefectProposalStore_List_FiltersByStatusAndLimit verifies that List
// respects the status filter (empty = all statuses), orders most-recent
// first, and clamps to the requested limit.
func TestDefectProposalStore_List_FiltersByStatusAndLimit(t *testing.T) {
	s := openProposalTestStore(t)
	proposals := pg.NewDefectProposalStore(s)
	ctx := context.Background()

	projectID, userID := seedProposalFixtures(t, s, "defect-list-"+uniqueSuffix())

	firstID, err := proposals.Create(ctx, &store.DefectProposal{
		ProjectID: projectID, FingerprintHash: "hash-a", ProposedCategory: "product_bug",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // guarantee a distinct created_at for ordering

	secondID, err := proposals.Create(ctx, &store.DefectProposal{
		ProjectID: projectID, FingerprintHash: "hash-b", ProposedCategory: "test_bug",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	approvedID, err := proposals.Create(ctx, &store.DefectProposal{
		ProjectID: projectID, FingerprintHash: "hash-c", ProposedCategory: "infrastructure",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}
	if err := proposals.MarkReviewed(ctx, approvedID, userID, store.ProposalStatusApproved); err != nil {
		t.Fatalf("MarkReviewed: %v", err)
	}

	t.Run("status=pending returns only pending, most recent first", func(t *testing.T) {
		got, err := proposals.List(ctx, projectID, store.ProposalStatusPending, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d proposals, want 2", len(got))
		}
		if got[0].ID != secondID || got[1].ID != firstID {
			t.Errorf("order = [%d %d], want [%d %d] (most recent first)", got[0].ID, got[1].ID, secondID, firstID)
		}
	})

	t.Run("status empty returns every status", func(t *testing.T) {
		got, err := proposals.List(ctx, projectID, "", 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d proposals, want 3", len(got))
		}
	})

	t.Run("limit clamps result count", func(t *testing.T) {
		got, err := proposals.List(ctx, projectID, "", 1)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d proposals, want 1", len(got))
		}
		if got[0].ID != approvedID {
			t.Errorf("got id=%d, want most recent id=%d", got[0].ID, approvedID)
		}
	})
}

// TestDefectProposalStore_FindPendingDuplicate verifies the idempotency check
// matches on (project_id, fingerprint_hash, proposed_category) and only when
// the existing proposal is still pending.
func TestDefectProposalStore_FindPendingDuplicate(t *testing.T) {
	s := openProposalTestStore(t)
	proposals := pg.NewDefectProposalStore(s)
	ctx := context.Background()

	projectID, userID := seedProposalFixtures(t, s, "defect-dup-"+uniqueSuffix())

	t.Run("no matching proposal returns nil", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "no-such-hash", "product_bug")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})

	id, err := proposals.Create(ctx, &store.DefectProposal{
		ProjectID: projectID, FingerprintHash: "dup-hash", ProposedCategory: "product_bug",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("matching pending proposal is found", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "dup-hash", "product_bug")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got == nil || got.ID != id {
			t.Fatalf("got %+v, want proposal id=%d", got, id)
		}
	})

	t.Run("different category is not a duplicate", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "dup-hash", "test_bug")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil for a different category", got)
		}
	})

	if err := proposals.MarkReviewed(ctx, id, userID, store.ProposalStatusApproved); err != nil {
		t.Fatalf("MarkReviewed: %v", err)
	}

	t.Run("approved proposal is no longer a pending duplicate", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "dup-hash", "product_bug")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil once the proposal is approved", got)
		}
	})
}
