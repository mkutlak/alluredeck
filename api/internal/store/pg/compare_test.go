package pg

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestCollapseByFullName verifies that duplicate-encoding entries (same
// full_name/test_name, different history_id) are collapsed to the entry where
// both status sides are populated — i.e. a real diff wins over an unmatched
// add/remove. The genuine new test must survive.
func TestCollapseByFullName_DedupePrefersBothSides(t *testing.T) {
	entries := []store.DiffEntry{
		// Two encodings of the same test: one matched (both sides), one unmatched (added).
		{TestName: "MyTest", FullName: "pkg.MyTest", HistoryID: "h1-dot", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		{TestName: "MyTest", FullName: "pkg.MyTest", HistoryID: "h1-colon", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
		// Genuinely new test — must survive unchanged.
		{TestName: "NewTest", FullName: "pkg.NewTest", HistoryID: "h2", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
	}

	got := collapseByFullName(entries)

	if len(got) != 2 {
		t.Fatalf("want 2 entries after collapse, got %d: %+v", len(got), got)
	}

	// First entry: the matched one (both sides non-empty) must win.
	if got[0].HistoryID != "h1-dot" {
		t.Errorf("want HistoryID=h1-dot (both-sides entry), got %q", got[0].HistoryID)
	}
	if got[0].StatusA != "passed" || got[0].StatusB != "failed" {
		t.Errorf("want StatusA=passed, StatusB=failed; got A=%q B=%q", got[0].StatusA, got[0].StatusB)
	}

	// Second entry: the genuinely new test must survive.
	if got[1].HistoryID != "h2" {
		t.Errorf("want HistoryID=h2 for new test, got %q", got[1].HistoryID)
	}
}

// TestCollapseByFullName_Empty verifies that an empty input returns an empty output.
func TestCollapseByFullName_Empty(t *testing.T) {
	got := collapseByFullName(nil)
	if len(got) != 0 {
		t.Errorf("want empty, got %d entries", len(got))
	}
}

// TestCollapseByFullName_NoDuplicates verifies that entries with distinct keys
// are all preserved.
func TestCollapseByFullName_NoDuplicates(t *testing.T) {
	entries := []store.DiffEntry{
		{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
	}

	got := collapseByFullName(entries)

	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
}

// TestCollapseByFullName_BothUnmatched verifies that when both duplicate entries
// are one-sided (neither has both statuses), the first occurrence is kept.
func TestCollapseByFullName_BothUnmatched(t *testing.T) {
	entries := []store.DiffEntry{
		{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1-first", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
		{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1-second", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
	}

	got := collapseByFullName(entries)

	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].HistoryID != "h1-first" {
		t.Errorf("want first occurrence kept, got HistoryID=%q", got[0].HistoryID)
	}
}

// TestCollapseByFullName_TwinAdded feeds two DiffAdded entries with the same
// full_name/test_name but different history_id values. Both have StatusA=""
// and StatusB="passed" — neither "wins" the both-sides preference — so the
// first occurrence must be kept and the second dropped. Stable choice is the
// first occurrence (index 0).
func TestCollapseByFullName_TwinAdded(t *testing.T) {
	entries := []store.DiffEntry{
		{TestName: "NewTest", FullName: "pkg.NewTest", HistoryID: "h-alpha", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
		{TestName: "NewTest", FullName: "pkg.NewTest", HistoryID: "h-beta", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
	}

	got := collapseByFullName(entries)

	if len(got) != 1 {
		t.Fatalf("want 1 entry after twin-Added collapse, got %d: %+v", len(got), got)
	}
	// Stable choice: first occurrence (h-alpha) is kept because neither side
	// has both statuses populated, so no upgrade happens.
	if got[0].HistoryID != "h-alpha" {
		t.Errorf("want HistoryID=h-alpha (first occurrence), got %q", got[0].HistoryID)
	}
	if got[0].StatusA != "" || got[0].StatusB != "passed" {
		t.Errorf("want StatusA='', StatusB='passed'; got A=%q B=%q", got[0].StatusA, got[0].StatusB)
	}
}
