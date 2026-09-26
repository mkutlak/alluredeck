package tools_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// proposalStores answers List on each proposal store with the given rows and
// records the project id and limit the defect store was asked for.
type proposalStores struct {
	defects  []*store.DefectProposal
	issues   []*store.KnownIssueProposal
	flaky    []*store.FlakyProposal
	users    store.UserStorer
	project  int
	limit    int
	listings int
}

func (p *proposalStores) stores() *bootstrap.Stores {
	return &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, limit int) ([]*store.DefectProposal, error) {
			p.project, p.limit = projectID, limit
			p.listings++
			return p.defects, nil
		}},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{ListFn: func(context.Context, int, store.ProposalStatus, int) ([]*store.KnownIssueProposal, error) {
			p.listings++
			return p.issues, nil
		}},
		FlakyProposals: &testutil.MockFlakyProposalStore{ListFn: func(context.Context, int, store.ProposalStatus, int) ([]*store.FlakyProposal, error) {
			p.listings++
			return p.flaky, nil
		}},
		User: p.users,
	}
}

func TestListProposals_Validation(t *testing.T) {
	cs := setupTestServer(t, (&proposalStores{}).stores())
	for _, args := range []map[string]any{
		{"project_id": 0},
		{"project_id": 1, "kind": "not_a_kind"},
		{"project_id": 1, "status": "not_a_status"},
	} {
		callErr(t, cs, "list_proposals", args)
	}
}

// TestListProposals_AggregatesAllKinds merges the three proposal tables most
// recent first. The proposer is the user's display name, never the email
// address: the REST endpoints expose only the numeric id, and this tool must
// not become the surface that hands out a staff directory.
func TestListProposals_AggregatesAllKinds(t *testing.T) {
	ctx := context.Background()
	users := testutil.NewMemUserStore()
	reviewer, err := users.CreateLocal(ctx, "reviewer@example.com", "Reviewer", "hash", "editor")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &proposalStores{
		users: users,
		defects: []*store.DefectProposal{{ID: 1, ProjectID: 7, FingerprintHash: "fp-1", ProposedCategory: "product_bug",
			ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: oldest}},
		issues: []*store.KnownIssueProposal{{ID: 2, ProjectID: 7, RegexPattern: "pattern-2", ProposedCategory: "infrastructure",
			ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: oldest.Add(2 * time.Hour)}},
		flaky: []*store.FlakyProposal{{ID: 3, ProjectID: 7, TestFullName: "pkg.TestFoo", HistoryID: "h-3",
			ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: oldest.Add(time.Hour)}},
	}
	cs := setupTestServer(t, p.stores())

	out, _ := call[tools.ListProposalsOutput](t, cs, "list_proposals", map[string]any{"project_id": 7})
	want := []tools.ProposalSummary{
		{ID: 2, Kind: "known_issue", Pattern: "pattern-2", ProposedCategory: "infrastructure"},
		{ID: 3, Kind: "flaky", TestFullName: "pkg.TestFoo", HistoryID: "h-3"},
		{ID: 1, Kind: "defect_classify", FingerprintHash: "fp-1", ProposedCategory: "product_bug"},
	}
	for i := range out.Items {
		if out.Items[i].Proposer != "Reviewer" || out.Items[i].ReviewURL == "" {
			t.Errorf("item %d proposer = %q, review_url = %q; want the display name and a link", out.Items[i].ID, out.Items[i].Proposer, out.Items[i].ReviewURL)
		}
		out.Items[i].Status, out.Items[i].CreatedAt, out.Items[i].Proposer, out.Items[i].ReviewURL = "", time.Time{}, "", ""
	}
	if !reflect.DeepEqual(out.Items, want) {
		t.Errorf("items = %+v, want %+v (most recent first)", out.Items, want)
	}
	if p.project != 7 {
		t.Errorf("listed project %d, want 7", p.project)
	}
}

func TestListProposals_KindFilter(t *testing.T) {
	p := &proposalStores{
		defects: []*store.DefectProposal{{ID: 1, Status: store.ProposalStatusPending}},
		issues:  []*store.KnownIssueProposal{{ID: 5, Status: store.ProposalStatusPending}},
		flaky:   []*store.FlakyProposal{{ID: 3, Status: store.ProposalStatusPending}},
	}
	cs := setupTestServer(t, p.stores())

	out, _ := call[tools.ListProposalsOutput](t, cs, "list_proposals", map[string]any{"project_id": 7, "kind": "known_issue"})
	if len(out.Items) != 1 || out.Items[0].ID != 5 || out.Items[0].Kind != "known_issue" || p.listings != 1 {
		t.Errorf("items = %+v after %d store listings, want only known_issue 5 from one listing", out.Items, p.listings)
	}
}

func TestListProposals_LimitDefaultAndClamp(t *testing.T) {
	tests := []struct {
		name      string
		limit     any
		wantLimit int
	}{
		{"default when omitted", nil, 50},
		{"clamped above max", 5000, 200},
		{"within range passes through", 10, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &proposalStores{}
			args := map[string]any{"project_id": 7, "kind": "defect_classify"}
			if tc.limit != nil {
				args["limit"] = tc.limit
			}
			call[tools.ListProposalsOutput](t, setupTestServer(t, p.stores()), "list_proposals", args)
			if p.limit != tc.wantLimit {
				t.Errorf("limit passed to store = %d, want %d", p.limit, tc.wantLimit)
			}
		})
	}
}

// countingUserStore counts GetByID calls: the memoization under test is
// invisible in the output, so only the call count can show one lookup per
// distinct proposer rather than one per row.
type countingUserStore struct {
	store.UserStorer
	calls map[int64]int
}

var _ store.UserStorer = (*countingUserStore)(nil)

func (c *countingUserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	c.calls[id]++
	return c.UserStorer.GetByID(ctx, id)
}

// TestListProposals_ResolvesEachProposerOnce: proposals cluster on a handful of
// proposers, so a page must not issue one identical user lookup per row. An
// unknown proposer falls back to "user:<id>" instead of failing the call.
func TestListProposals_ResolvesEachProposerOnce(t *testing.T) {
	ctx := context.Background()
	users := testutil.NewMemUserStore()
	reviewer, err := users.CreateLocal(ctx, "reviewer@example.com", "Reviewer", "hash", "editor")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	counting := &countingUserStore{UserStorer: users, calls: map[int64]int{}}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &proposalStores{users: counting}
	for i := range 5 {
		p.defects = append(p.defects, &store.DefectProposal{ID: int64(i + 1), ProposerUserID: reviewer.ID, CreatedAt: created.Add(time.Duration(i) * time.Minute)})
	}
	p.issues = []*store.KnownIssueProposal{{ID: 20, ProposerUserID: reviewer.ID, CreatedAt: created}}
	p.flaky = []*store.FlakyProposal{{ID: 30, ProposerUserID: 404, CreatedAt: created}}
	cs := setupTestServer(t, p.stores())

	out, _ := call[tools.ListProposalsOutput](t, cs, "list_proposals", map[string]any{"project_id": 1})
	if len(out.Items) != 7 {
		t.Fatalf("items = %d, want 7", len(out.Items))
	}
	if got := counting.calls[reviewer.ID]; got != 1 {
		t.Errorf("GetByID(%d) called %d time(s) for 6 proposals, want 1", reviewer.ID, got)
	}
	for _, item := range out.Items {
		want := "Reviewer"
		if item.ID == 30 {
			want = "user:404"
		}
		if item.Proposer != want || strings.Contains(item.Proposer, "@") {
			t.Errorf("item %d proposer = %q, want %q", item.ID, item.Proposer, want)
		}
	}
}

// TestListProposals_TiedTimestampsTrimDeterministically pins the stable sort:
// proposals written by one batch share a created_at, so an unstable sort would
// let the [:limit] trim keep a different subset on each identical call.
func TestListProposals_TiedTimestampsTrimDeterministically(t *testing.T) {
	// Two batches interleaved: ids 2, 4, 6... share the newer created_at.
	p := &proposalStores{}
	for i := range 40 {
		created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC).Add(time.Duration(i%2) * time.Hour)
		p.defects = append(p.defects, &store.DefectProposal{ID: int64(i + 1), CreatedAt: created})
	}
	cs := setupTestServer(t, p.stores())

	for n := range 5 {
		out, _ := call[tools.ListProposalsOutput](t, cs, "list_proposals", map[string]any{"project_id": 1, "limit": 3})
		got := make([]int64, 0, len(out.Items))
		for _, item := range out.Items {
			got = append(got, item.ID)
		}
		if !reflect.DeepEqual(got, []int64{2, 4, 6}) {
			t.Fatalf("call %d kept ids %v, want the newest batch's first three in store order [2 4 6]", n+1, got)
		}
	}
}
