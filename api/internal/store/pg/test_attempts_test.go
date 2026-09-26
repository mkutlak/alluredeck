package pg_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// attempted is a retried Playwright-style result carrying its own attempts.
func attempted(historyID string, attempts ...parser.Attempt) *parser.Result {
	return &parser.Result{
		Name: historyID, FullName: "spec > " + historyID, HistoryID: historyID,
		Status: "failed", StartMs: 10, StopMs: 200, Attempts: attempts,
	}
}

func att(i int, status, msg string) parser.Attempt {
	return parser.Attempt{Index: i, Status: status, StatusMessage: msg}
}

func row(i int, status, msg string) store.TestAttemptRow {
	return store.TestAttemptRow{AttemptIndex: i, Status: status, StatusMessage: msg}
}

// TestInsertBatchFull_Attempts drives InsertBatchFull uploads and reads the
// attempt rows back through GetAttempts.
func TestInsertBatchFull_Attempts(t *testing.T) {
	long := strings.Repeat("x", 5000)
	tests := []struct {
		name    string
		uploads [][]*parser.Result
		want    map[string][]store.TestAttemptRow // historyID -> attempts; nil = none
	}{
		{
			// Handed over out of index order: reads must ORDER BY attempt_index.
			name: "one row per attempt, read back in attempt_index order",
			uploads: [][]*parser.Result{{attempted("h",
				att(2, "failed", "third failure"), att(0, "failed", "first failure"), att(1, "timedOut", "Test timeout of 30000ms exceeded"))}},
			want: map[string][]store.TestAttemptRow{"h": {
				row(0, "failed", "first failure"), row(1, "timedOut", "Test timeout of 30000ms exceeded"), row(2, "failed", "third failure")}},
		},
		{
			// The parent upsert lands on the same test_result_id; a plain insert
			// would violate UNIQUE (test_result_id, attempt_index).
			name: "re-ingestion replaces the sequence rather than merging it",
			uploads: [][]*parser.Result{
				{attempted("h", att(0, "failed", "old A"), att(1, "failed", "old B"), att(2, "failed", "old C"))},
				{attempted("h", att(0, "failed", "new A"), att(1, "passed", ""))},
			},
			want: map[string][]store.TestAttemptRow{"h": {row(0, "failed", "new A"), row(1, "passed", "")}},
		},
		{
			// A lone attempt already sits on the test_results row.
			name:    "a single attempt is not persisted",
			uploads: [][]*parser.Result{{attempted("h", att(0, "failed", "boom"))}},
			want:    map[string][]store.TestAttemptRow{"h": nil},
		},
		{
			// Dropping below the persist threshold writes nothing, so clearing
			// cannot be a side effect of the insert.
			name: "re-ingestion with a single attempt withdraws the old sequence",
			uploads: [][]*parser.Result{
				{attempted("h", att(0, "failed", "old A"), att(1, "failed", "old B"))},
				{attempted("h", att(0, "passed", ""))},
			},
			want: map[string][]store.TestAttemptRow{"h": nil},
		},
		{
			name: "every result a batch touches is rewritten",
			uploads: [][]*parser.Result{
				{
					attempted("keeps", att(0, "failed", "old A"), att(1, "failed", "old B")),
					attempted("drops", att(0, "failed", "old X"), att(1, "failed", "old Y"), att(2, "failed", "old Z")),
				},
				{attempted("keeps", att(0, "failed", "new A"), att(1, "passed", "")), attempted("drops", att(0, "passed", ""))},
			},
			want: map[string][]store.TestAttemptRow{"keeps": {row(0, "failed", "new A"), row(1, "passed", "")}, "drops": nil},
		},
		{
			// The final attempt's full text lives on test_results.
			name:    "status_message is capped at 2000 characters",
			uploads: [][]*parser.Result{{attempted("h", att(0, "failed", long), att(1, "failed", long))}},
			want:    map[string][]store.TestAttemptRow{"h": {row(0, "failed", long[:2000]), row(1, "failed", long[:2000])}},
		},
		{
			// Rows exist under the EMPTY history_id, so an unguarded query would
			// return them.
			name:    "empty and unknown historyIds read as none",
			uploads: [][]*parser.Result{{attempted("", att(0, "failed", "a"), att(1, "failed", "b"))}},
			want:    map[string][]store.TestAttemptRow{"": nil, "no-such-history-id": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			b := f.build(1)
			for _, upload := range tt.uploads {
				f.insertFull(b, upload...)
			}
			for historyID, want := range tt.want {
				got, err := f.results.GetAttempts(f.ctx, f.id, b, historyID)
				if err != nil {
					t.Fatalf("GetAttempts(%q): %v", historyID, err)
				}
				if !slices.Equal(got, want) {
					t.Errorf("attempts(%q) = %+v, want %+v", historyID, got, want)
				}
			}
		})
	}
}

// TestDeleteByBuild_CascadesAttempts: retention pruning a build takes its
// attempt rows with it (ON DELETE CASCADE) rather than orphaning them.
func TestDeleteByBuild_CascadesAttempts(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	f.insertFull(b, attempted("hist-doomed", att(0, "failed", "a"), att(1, "failed", "b")))
	tr, err := f.results.GetByHistoryID(f.ctx, f.id, b, "hist-doomed")
	if err != nil || tr == nil {
		t.Fatalf("GetByHistoryID = %v, %v", tr, err)
	}
	const q = "SELECT COUNT(*) FROM test_attempts WHERE test_result_id=$1"
	if n := f.count(q, tr.ID); n != 2 {
		t.Fatalf("attempts before delete = %d, want 2", n)
	}
	if err := f.results.DeleteByBuild(f.ctx, b); err != nil {
		t.Fatalf("DeleteByBuild: %v", err)
	}
	if n := f.count(q, tr.ID); n != 0 {
		t.Errorf("attempts after DeleteByBuild = %d, want 0 (ON DELETE CASCADE)", n)
	}
}
