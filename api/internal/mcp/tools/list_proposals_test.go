package tools_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// decodeListProposals JSON-round-trips StructuredContent into ListProposalsOutput.
func decodeListProposals(t *testing.T, res *mcpsdk.CallToolResult) tools.ListProposalsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ListProposalsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListProposalsOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// list_proposals: validation
// ---------------------------------------------------------------------------

func TestListProposals_ValidationErrors(t *testing.T) {
	stores := &bootstrap.Stores{
		DefectProposals:     &testutil.MockDefectProposalStore{},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
	}
	cs := setupTestServer(t, stores)
	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing project_id", map[string]any{}},
		{"zero project_id", map[string]any{"project_id": 0}},
		{"negative project_id", map[string]any{"project_id": -1}},
		{"invalid kind", map[string]any{"project_id": 1, "kind": "not_a_kind"}},
		{"invalid status", map[string]any{"project_id": 1, "status": "not_a_status"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_proposals", Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !res.IsError {
				t.Fatalf("want IsError=true for %s", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// list_proposals: aggregation, sorting, and kind filtering
// ---------------------------------------------------------------------------

func TestListProposals_AggregatesAllKindsSortedMostRecentFirst(t *testing.T) {
	users := testutil.NewMemUserStore()
	ctx := context.Background()
	reviewer, err := users.CreateLocal(ctx, "reviewer@example.com", "Reviewer", "hash", "editor")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	middle := oldest.Add(time.Hour)
	newest := middle.Add(time.Hour)

	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(_ context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.DefectProposal, error) {
				if projectID != 7 {
					t.Errorf("DefectProposals.List projectID = %d, want 7", projectID)
				}
				return []*store.DefectProposal{{
					ID: 1, ProjectID: projectID, FingerprintHash: "fp-1", ProposedCategory: "product_bug",
					ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: oldest,
				}}, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{
			ListFn: func(_ context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.KnownIssueProposal, error) {
				return []*store.KnownIssueProposal{{
					ID: 2, ProjectID: projectID, RegexPattern: "pattern-2", ProposedCategory: "infrastructure",
					ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: newest,
				}}, nil
			},
		},
		FlakyProposals: &testutil.MockFlakyProposalStore{
			ListFn: func(_ context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.FlakyProposal, error) {
				return []*store.FlakyProposal{{
					ID: 3, ProjectID: projectID, TestFullName: "pkg.TestFoo", HistoryID: "h-3",
					ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: middle,
				}}, nil
			},
		},
		User: users,
	}
	cs := setupTestServer(t, stores)

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_proposals",
		Arguments: map[string]any{"project_id": 7},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListProposals(t, res)
	if len(out.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(out.Items))
	}

	// Most recent first, across all three kinds.
	wantOrder := []struct {
		id   int64
		kind string
	}{
		{2, "known_issue"},
		{3, "flaky"},
		{1, "defect_classify"},
	}
	for i, want := range wantOrder {
		if out.Items[i].ID != want.id || out.Items[i].Kind != want.kind {
			t.Errorf("item[%d] = (id=%d, kind=%s), want (id=%d, kind=%s)", i, out.Items[i].ID, out.Items[i].Kind, want.id, want.kind)
		}
	}

	for _, item := range out.Items {
		// The display name, never the email address: the REST proposal
		// endpoints expose only the numeric proposer id, and this tool must
		// not become the surface that hands out a staff directory.
		if item.Proposer != "Reviewer" {
			t.Errorf("item id=%d proposer = %q, want the display name %q", item.ID, item.Proposer, "Reviewer")
		}
		if strings.Contains(item.Proposer, "@") {
			t.Errorf("item id=%d proposer = %q, want no email address in the output", item.ID, item.Proposer)
		}
		if item.ReviewURL == "" {
			t.Errorf("item id=%d has empty review_url", item.ID)
		}
	}

	// known_issue summary fields.
	if out.Items[0].Pattern != "pattern-2" {
		t.Errorf("known_issue item pattern = %q, want pattern-2", out.Items[0].Pattern)
	}
	// flaky summary fields.
	if out.Items[1].TestFullName != "pkg.TestFoo" || out.Items[1].HistoryID != "h-3" {
		t.Errorf("flaky item = (test_full_name=%q, history_id=%q), want (pkg.TestFoo, h-3)", out.Items[1].TestFullName, out.Items[1].HistoryID)
	}
	// defect_classify summary fields.
	if out.Items[2].FingerprintHash != "fp-1" {
		t.Errorf("defect_classify item fingerprint_hash = %q, want fp-1", out.Items[2].FingerprintHash)
	}
}

func TestListProposals_KindFilter_OnlyQueriesThatStore(t *testing.T) {
	calledDefect, calledFlaky := false, false

	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(context.Context, int, store.ProposalStatus, int) ([]*store.DefectProposal, error) {
				calledDefect = true
				return nil, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{
			ListFn: func(_ context.Context, projectID int, status store.ProposalStatus, limit int) ([]*store.KnownIssueProposal, error) {
				return []*store.KnownIssueProposal{{ID: 5, ProjectID: projectID, RegexPattern: "p", Status: store.ProposalStatusPending}}, nil
			},
		},
		FlakyProposals: &testutil.MockFlakyProposalStore{
			ListFn: func(context.Context, int, store.ProposalStatus, int) ([]*store.FlakyProposal, error) {
				calledFlaky = true
				return nil, nil
			},
		},
	}
	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_proposals",
		Arguments: map[string]any{"project_id": 7, "kind": "known_issue"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if calledDefect || calledFlaky {
		t.Errorf("kind=known_issue queried other stores: defect=%v flaky=%v", calledDefect, calledFlaky)
	}

	out := decodeListProposals(t, res)
	if len(out.Items) != 1 || out.Items[0].Kind != "known_issue" {
		t.Fatalf("want 1 known_issue item, got %+v", out.Items)
	}
}

func TestListProposals_LimitDefaultAndClamp(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantLimit int
	}{
		{"default when omitted", map[string]any{"project_id": 7}, 50},
		{"clamped above max", map[string]any{"project_id": 7, "limit": 5000}, 200},
		{"within range passes through", map[string]any{"project_id": 7, "limit": 10}, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotLimit int
			stores := &bootstrap.Stores{
				DefectProposals: &testutil.MockDefectProposalStore{
					ListFn: func(_ context.Context, _ int, _ store.ProposalStatus, limit int) ([]*store.DefectProposal, error) {
						gotLimit = limit
						return nil, nil
					},
				},
				KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
				FlakyProposals:      &testutil.MockFlakyProposalStore{},
			}
			cs := setupTestServer(t, stores)
			ctx := context.Background()

			args := map[string]any{"project_id": tc.args["project_id"], "kind": "defect_classify"}
			if limit, ok := tc.args["limit"]; ok {
				args["limit"] = limit
			}
			res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      "list_proposals",
				Arguments: args,
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %v", res.Content)
			}
			if gotLimit != tc.wantLimit {
				t.Errorf("limit passed to store = %d, want %d", gotLimit, tc.wantLimit)
			}
		})
	}
}

func TestListProposals_ProposerFallsBackWhenUserMissing(t *testing.T) {
	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, _ int) ([]*store.DefectProposal, error) {
				return []*store.DefectProposal{{
					ID: 9, ProjectID: projectID, FingerprintHash: "fp", ProposedCategory: "product_bug",
					ProposerUserID: 404, Status: store.ProposalStatusPending, CreatedAt: time.Now(),
				}}, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
		User:                testutil.NewMemUserStore(), // empty: GetByID(404) finds nothing
	}
	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_proposals",
		Arguments: map[string]any{"project_id": 1, "kind": "defect_classify"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListProposals(t, res)
	if len(out.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(out.Items))
	}
	if out.Items[0].Proposer != "user:404" {
		t.Errorf("proposer = %q, want fallback %q", out.Items[0].Proposer, "user:404")
	}
}

// countingUserStore is a decorator over another store.UserStorer that counts
// GetByID calls. It exists because the memoization under test is invisible in
// the output — the labels are identical either way — so only the call count
// can distinguish one lookup per distinct user from one lookup per row.
type countingUserStore struct {
	store.UserStorer
	getByIDCalls map[int64]int
}

var _ store.UserStorer = (*countingUserStore)(nil)

func (c *countingUserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	c.getByIDCalls[id]++
	return c.UserStorer.GetByID(ctx, id)
}

// TestListProposals_ResolvesEachProposerOnce pins the memoization: proposals
// cluster on a handful of proposers (often a single automation account), so a
// full page must not issue one identical user lookup per row.
func TestListProposals_ResolvesEachProposerOnce(t *testing.T) {
	users := testutil.NewMemUserStore()
	ctx := context.Background()
	reviewer, err := users.CreateLocal(ctx, "reviewer@example.com", "Reviewer", "hash", "editor")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	counting := &countingUserStore{UserStorer: users, getByIDCalls: map[int64]int{}}

	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, _ int) ([]*store.DefectProposal, error) {
				rows := make([]*store.DefectProposal, 0, 5)
				for i := range 5 {
					rows = append(rows, &store.DefectProposal{
						ID: int64(i + 1), ProjectID: projectID, FingerprintHash: "fp",
						ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending,
						CreatedAt: created.Add(time.Duration(i) * time.Minute),
					})
				}
				return rows, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{
			ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, _ int) ([]*store.KnownIssueProposal, error) {
				return []*store.KnownIssueProposal{{
					ID: 20, ProjectID: projectID, RegexPattern: "p",
					ProposerUserID: reviewer.ID, Status: store.ProposalStatusPending, CreatedAt: created,
				}}, nil
			},
		},
		FlakyProposals: &testutil.MockFlakyProposalStore{},
		User:           counting,
	}
	cs := setupTestServer(t, stores)

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_proposals",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListProposals(t, res)
	if len(out.Items) != 6 {
		t.Fatalf("want 6 items, got %d", len(out.Items))
	}
	if got := counting.getByIDCalls[reviewer.ID]; got != 1 {
		t.Errorf("GetByID(%d) called %d time(s) for 6 proposals, want 1", reviewer.ID, got)
	}
	for _, item := range out.Items {
		if item.Proposer != "Reviewer" {
			t.Errorf("item id=%d proposer = %q, want %q", item.ID, item.Proposer, "Reviewer")
		}
	}
}

// TestListProposals_TiedTimestampsTrimDeterministically pins the stable sort.
// Proposals written by one batch share a created_at, so an unstable sort would
// let the `[:limit]` trim keep a different subset on each identical call.
func TestListProposals_TiedTimestampsTrimDeterministically(t *testing.T) {
	tied := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, _ int) ([]*store.DefectProposal, error) {
				rows := make([]*store.DefectProposal, 0, 8)
				for i := range 8 {
					rows = append(rows, &store.DefectProposal{
						ID: int64(i + 1), ProjectID: projectID, FingerprintHash: "fp",
						Status: store.ProposalStatusPending, CreatedAt: tied,
					})
				}
				return rows, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
	}
	cs := setupTestServer(t, stores)
	ctx := context.Background()

	var first []int64
	for call := range 5 {
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
			Name:      "list_proposals",
			Arguments: map[string]any{"project_id": 1, "limit": 3},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %v", res.Content)
		}
		out := decodeListProposals(t, res)
		got := make([]int64, 0, len(out.Items))
		for _, item := range out.Items {
			got = append(got, item.ID)
		}
		if call == 0 {
			first = got
			// Stable order over tied timestamps is the store's own order.
			if !reflect.DeepEqual(got, []int64{1, 2, 3}) {
				t.Fatalf("kept ids = %v, want the first three rows in store order [1 2 3]", got)
			}
			continue
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("call %d kept ids = %v, want %v on every identical call", call+1, got, first)
		}
	}
}

// TestListProposals_TextDigest verifies the unstructured Content carries a
// one-line count digest, matching the textResult convention used by every
// other read tool.
func TestListProposals_TextDigest(t *testing.T) {
	stores := &bootstrap.Stores{
		DefectProposals: &testutil.MockDefectProposalStore{
			ListFn: func(_ context.Context, projectID int, _ store.ProposalStatus, _ int) ([]*store.DefectProposal, error) {
				return []*store.DefectProposal{{ID: 1, ProjectID: projectID, Status: store.ProposalStatusPending, CreatedAt: time.Now()}}, nil
			},
		},
		KnownIssueProposals: &testutil.MockKnownIssueProposalStore{},
		FlakyProposals:      &testutil.MockFlakyProposalStore{},
	}
	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_proposals",
		Arguments: map[string]any{"project_id": 1, "kind": "defect_classify"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok && tc.Text == "1 proposal(s) for project 1" {
			found = true
		}
	}
	if !found {
		t.Errorf("want a one-line digest in Content, got: %v", res.Content)
	}
}
