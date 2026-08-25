package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestKnownIssueProposalStore_List_FiltersByStatusAndLimit mirrors
// TestDefectProposalStore_List_FiltersByStatusAndLimit for known_issue_proposals.
func TestKnownIssueProposalStore_List_FiltersByStatusAndLimit(t *testing.T) {
	s := openProposalTestStore(t)
	proposals := pg.NewKnownIssueProposalStore(s)
	ctx := context.Background()

	projectID, userID := seedProposalFixtures(t, s, "ki-list-"+uniqueSuffix())

	firstID, err := proposals.Create(ctx, &store.KnownIssueProposal{
		ProjectID: projectID, RegexPattern: "pattern-a", ProposedCategory: "infrastructure",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	secondID, err := proposals.Create(ctx, &store.KnownIssueProposal{
		ProjectID: projectID, RegexPattern: "pattern-b", ProposedCategory: "infrastructure",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	rejectedID, err := proposals.Create(ctx, &store.KnownIssueProposal{
		ProjectID: projectID, RegexPattern: "pattern-c", ProposedCategory: "infrastructure",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}
	if err := proposals.MarkReviewed(ctx, rejectedID, userID, store.ProposalStatusRejected); err != nil {
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
		if got[0].ID != rejectedID {
			t.Errorf("got id=%d, want most recent id=%d", got[0].ID, rejectedID)
		}
	})
}

// TestKnownIssueProposalStore_FindPendingDuplicate verifies the idempotency
// check matches on (project_id, regex_pattern) and only when the existing
// proposal is still pending.
//
// known_issue_proposals has no test_name column (regex rules match by
// message, not by test identity), so the identity is (project_id, pattern)
// only -- unlike flaky_proposals, which also has test_full_name.
func TestKnownIssueProposalStore_FindPendingDuplicate(t *testing.T) {
	s := openProposalTestStore(t)
	proposals := pg.NewKnownIssueProposalStore(s)
	ctx := context.Background()

	projectID, userID := seedProposalFixtures(t, s, "ki-dup-"+uniqueSuffix())

	t.Run("no matching proposal returns nil", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "no-such-pattern")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})

	id, err := proposals.Create(ctx, &store.KnownIssueProposal{
		ProjectID: projectID, RegexPattern: "dup-pattern", ProposedCategory: "infrastructure",
		ProposerUserID: userID, Status: store.ProposalStatusPending,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("matching pending proposal is found", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "dup-pattern")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got == nil || got.ID != id {
			t.Fatalf("got %+v, want proposal id=%d", got, id)
		}
	})

	if err := proposals.MarkReviewed(ctx, id, userID, store.ProposalStatusApproved); err != nil {
		t.Fatalf("MarkReviewed: %v", err)
	}

	t.Run("approved proposal is no longer a pending duplicate", func(t *testing.T) {
		got, err := proposals.FindPendingDuplicate(ctx, projectID, "dup-pattern")
		if err != nil {
			t.Fatalf("FindPendingDuplicate: %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil once the proposal is approved", got)
		}
	})
}
