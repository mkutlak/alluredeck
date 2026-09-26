package pg_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// proposalAPI adapts the three proposal stores to one shape. key is each
// store's identity column: fingerprint hash (defects, with category
// product_bug), history id (flaky), regex pattern (known issues).
type proposalAPI struct {
	create  func(ctx context.Context, projectID int, userID int64, key string) (int64, error)
	list    func(ctx context.Context, projectID int, status store.ProposalStatus, limit int) ([]int64, error)
	findDup func(ctx context.Context, projectID int, key string) (int64, error) // 0 = no duplicate
	review  func(ctx context.Context, id, reviewedBy int64, status store.ProposalStatus) error
	// otherDup looks key up under a different defect category; defects only.
	otherDup func(ctx context.Context, projectID int, key string) (int64, error)
}

func proposalIDs[T any](ps []*T, err error, id func(*T) int64) ([]int64, error) {
	ids := make([]int64, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, id(p))
	}
	return ids, err
}

func proposalID[T any](p *T, err error, id func(*T) int64) (int64, error) {
	if p == nil {
		return 0, err
	}
	return id(p), err
}

// TestProposalStores covers List (status filter, empty = every status, most
// recent first, limit) and the FindPendingDuplicate idempotency check, which
// matches only a still-pending proposal with the same identity.
func TestProposalStores(t *testing.T) {
	pending := store.ProposalStatusPending
	cases := []struct {
		name     string
		reviewTo store.ProposalStatus // status the third proposal is reviewed to
		api      func(s *pg.PGStore) proposalAPI
	}{
		{"defect", store.ProposalStatusApproved, func(s *pg.PGStore) proposalAPI {
			ps := pg.NewDefectProposalStore(s)
			id := func(p *store.DefectProposal) int64 { return p.ID }
			return proposalAPI{
				create: func(ctx context.Context, projectID int, userID int64, key string) (int64, error) {
					return ps.Create(ctx, &store.DefectProposal{ProjectID: projectID, FingerprintHash: key,
						ProposedCategory: "product_bug", ProposerUserID: userID, Status: pending})
				},
				list: func(ctx context.Context, projectID int, st store.ProposalStatus, limit int) ([]int64, error) {
					got, err := ps.List(ctx, projectID, st, limit)
					return proposalIDs(got, err, id)
				},
				findDup: func(ctx context.Context, projectID int, key string) (int64, error) {
					got, err := ps.FindPendingDuplicate(ctx, projectID, key, "product_bug")
					return proposalID(got, err, id)
				},
				otherDup: func(ctx context.Context, projectID int, key string) (int64, error) {
					got, err := ps.FindPendingDuplicate(ctx, projectID, key, "test_bug")
					return proposalID(got, err, id)
				},
				review: ps.MarkReviewed,
			}
		}},
		{"flaky", store.ProposalStatusRejected, func(s *pg.PGStore) proposalAPI {
			ps := pg.NewFlakyProposalStore(s)
			id := func(p *store.FlakyProposal) int64 { return p.ID }
			return proposalAPI{
				create: func(ctx context.Context, projectID int, userID int64, key string) (int64, error) {
					return ps.Create(ctx, &store.FlakyProposal{ProjectID: projectID, TestFullName: "pkg." + key,
						HistoryID: key, ProposerUserID: userID, Status: pending})
				},
				list: func(ctx context.Context, projectID int, st store.ProposalStatus, limit int) ([]int64, error) {
					got, err := ps.List(ctx, projectID, st, limit)
					return proposalIDs(got, err, id)
				},
				findDup: func(ctx context.Context, projectID int, key string) (int64, error) {
					got, err := ps.FindPendingDuplicate(ctx, projectID, key)
					return proposalID(got, err, id)
				},
				review: ps.MarkReviewed,
			}
		}},
		{"known issue", store.ProposalStatusRejected, func(s *pg.PGStore) proposalAPI {
			ps := pg.NewKnownIssueProposalStore(s)
			id := func(p *store.KnownIssueProposal) int64 { return p.ID }
			return proposalAPI{
				create: func(ctx context.Context, projectID int, userID int64, key string) (int64, error) {
					return ps.Create(ctx, &store.KnownIssueProposal{ProjectID: projectID, RegexPattern: key,
						ProposedCategory: "infrastructure", ProposerUserID: userID, Status: pending})
				},
				list: func(ctx context.Context, projectID int, st store.ProposalStatus, limit int) ([]int64, error) {
					got, err := ps.List(ctx, projectID, st, limit)
					return proposalIDs(got, err, id)
				},
				findDup: func(ctx context.Context, projectID int, key string) (int64, error) {
					got, err := ps.FindPendingDuplicate(ctx, projectID, key)
					return proposalID(got, err, id)
				},
				review: ps.MarkReviewed,
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			api, projectID, userID := tc.api(f.s), int(f.id), f.newUser()
			create := func(key string) int64 {
				t.Helper()
				id, err := api.create(f.ctx, projectID, userID, key)
				if err != nil {
					t.Fatalf("create %s: %v", key, err)
				}
				return id
			}
			review := func(id int64, status store.ProposalStatus) {
				t.Helper()
				if err := api.review(f.ctx, id, userID, status); err != nil {
					t.Fatalf("MarkReviewed: %v", err)
				}
			}
			wantDup := func(label string, find func(context.Context, int, string) (int64, error), key string, want int64) {
				t.Helper()
				if got, err := find(f.ctx, projectID, key); err != nil || got != want {
					t.Errorf("%s: FindPendingDuplicate(%q) = %d, %v; want %d", label, key, got, err, want)
				}
			}

			first := create("key-a")
			time.Sleep(2 * time.Millisecond) // distinct created_at for ordering
			second := create("key-b")
			third := create("key-c")
			review(third, tc.reviewTo)
			for _, l := range []struct {
				status store.ProposalStatus
				limit  int
				want   []int64
			}{
				{pending, 10, []int64{second, first}},
				{"", 10, []int64{third, second, first}},
				{"", 1, []int64{third}},
			} {
				if got, err := api.list(f.ctx, projectID, l.status, l.limit); err != nil || !slices.Equal(got, l.want) {
					t.Errorf("List(status=%q, limit=%d) = %v, %v; want %v", l.status, l.limit, got, err, l.want)
				}
			}

			wantDup("no match", api.findDup, "no-such-key", 0)
			wantDup("pending match", api.findDup, "key-a", first)
			if api.otherDup != nil {
				wantDup("different category", api.otherDup, "key-a", 0)
			}
			review(first, store.ProposalStatusApproved)
			wantDup("approved", api.findDup, "key-a", 0)
		})
	}
}
