package pg

import (
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestCollapseByFullName: entries encoding one test twice (same full_name and
// test_name, different history_id) collapse to one, in first-seen position.
// An entry with both status sides wins over a one-sided add/remove; between
// two one-sided entries the first is kept.
func TestCollapseByFullName(t *testing.T) {
	matched := store.DiffEntry{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1-dot", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed}
	added := store.DiffEntry{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1-colon", StatusB: "failed", Category: store.DiffAdded}
	addedTwin := store.DiffEntry{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1-twin", StatusB: "passed", Category: store.DiffAdded}
	fixed := store.DiffEntry{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed}
	newTest := store.DiffEntry{TestName: "New", FullName: "pkg.New", HistoryID: "h3", StatusB: "passed", Category: store.DiffAdded}

	tests := []struct {
		name string
		in   []store.DiffEntry
		want []store.DiffEntry
	}{
		{"empty", nil, nil},
		{"distinct keys all kept", []store.DiffEntry{matched, fixed}, []store.DiffEntry{matched, fixed}},
		{"both-sides entry wins, genuine new test survives", []store.DiffEntry{added, newTest, matched}, []store.DiffEntry{matched, newTest}},
		{"both one-sided keeps the first", []store.DiffEntry{added, addedTwin}, []store.DiffEntry{added}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collapseByFullName(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("collapseByFullName = %+v, want %+v", got, tt.want)
			}
		})
	}
}
