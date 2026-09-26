package pg_test

import (
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestFailureSummaryStore: a miss is (nil, nil); Upsert round-trips every
// field with created_at from the DB default; a second Upsert for the same
// (build, history) replaces the row; nil evidence reads back empty, never NULL.
func TestFailureSummaryStore(t *testing.T) {
	f := newFixture(t)
	fs := pg.NewFailureSummaryStore(f.s)
	b := f.build(1)
	get := func(historyID string) *store.FailureSummary {
		t.Helper()
		got, err := fs.Get(f.ctx, b, historyID)
		if err != nil {
			t.Fatalf("Get(%s): %v", historyID, err)
		}
		return got
	}
	if got := get("no-such-history"); got != nil {
		t.Errorf("Get on a miss = %+v, want nil", got)
	}

	for _, in := range []store.FailureSummary{
		{BuildID: b, HistoryID: "h1", ProjectID: f.id, InputHash: "hash-v1", Hypothesis: "The product returned 500.",
			Category: "product_bug", Confidence: "medium", Evidence: []string{"status 500 from /users", "last passed 3 builds ago"},
			Model: "llama3.1", PromptVersion: 1},
		{BuildID: b, HistoryID: "h1", ProjectID: f.id, InputHash: "hash-v2", Hypothesis: "revised guess",
			Category: "flake", Confidence: "high", Evidence: []string{"b"}, Model: "m2", PromptVersion: 2},
		{BuildID: b, HistoryID: "h2", ProjectID: f.id, InputHash: "h", Hypothesis: "no evidence",
			Category: "test_bug", Model: "m", PromptVersion: 1, Evidence: nil},
	} {
		if err := fs.Upsert(f.ctx, in); err != nil {
			t.Fatalf("Upsert %s/%s: %v", in.HistoryID, in.InputHash, err)
		}
		got := get(in.HistoryID)
		if got == nil || got.CreatedAt.IsZero() {
			t.Fatalf("Get(%s) = %+v, want the row with created_at set", in.HistoryID, got)
		}
		got.CreatedAt = in.CreatedAt
		if in.Evidence == nil { // stored as a JSON array, so it reads back empty, not null
			if got.Evidence == nil || len(got.Evidence) != 0 {
				t.Errorf("Get(%s).Evidence = %#v, want an empty non-nil slice", in.HistoryID, got.Evidence)
			}
			got.Evidence = nil
		}
		if !reflect.DeepEqual(*got, in) {
			t.Errorf("Get(%s) = %+v, want %+v", in.HistoryID, *got, in)
		}
	}
}
