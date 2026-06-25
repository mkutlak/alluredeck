package pg

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// TestDedupeResultsByHistoryID covers the pure collapse logic that prevents
// retried Allure tests (one *-result.json per attempt, same historyId) from
// duplicating their enrichment children in InsertBatchFull. The latest attempt
// (greatest StopMs) must win regardless of input order, while empty-historyId
// results are never collapsed.
func TestDedupeResultsByHistoryID(t *testing.T) {
	tests := []struct {
		name string
		in   []*parser.Result
		// want is the expected (historyId, status, stopMs) of the survivors, in order.
		want []struct {
			historyID string
			status    string
			stopMs    int64
		}
	}{
		{
			name: "no duplicates preserves order",
			in: []*parser.Result{
				{HistoryID: "a", Status: "passed", StopMs: 10},
				{HistoryID: "b", Status: "failed", StopMs: 20},
			},
			want: []struct {
				historyID string
				status    string
				stopMs    int64
			}{{"a", "passed", 10}, {"b", "failed", 20}},
		},
		{
			name: "latest attempt wins when earlier attempt is first",
			in: []*parser.Result{
				{HistoryID: "a", Status: "failed", StopMs: 100},
				{HistoryID: "a", Status: "passed", StopMs: 200},
			},
			want: []struct {
				historyID string
				status    string
				stopMs    int64
			}{{"a", "passed", 200}},
		},
		{
			name: "latest attempt wins when latest attempt is first",
			in: []*parser.Result{
				{HistoryID: "a", Status: "passed", StopMs: 200},
				{HistoryID: "a", Status: "failed", StopMs: 100},
			},
			want: []struct {
				historyID string
				status    string
				stopMs    int64
			}{{"a", "passed", 200}},
		},
		{
			name: "empty historyId entries are never collapsed",
			in: []*parser.Result{
				{HistoryID: "", Status: "passed", StopMs: 1},
				{HistoryID: "", Status: "failed", StopMs: 2},
			},
			want: []struct {
				historyID string
				status    string
				stopMs    int64
			}{{"", "passed", 1}, {"", "failed", 2}},
		},
		{
			name: "mixed empty and duplicate non-empty",
			in: []*parser.Result{
				{HistoryID: "", Status: "broken", StopMs: 5},
				{HistoryID: "a", Status: "failed", StopMs: 100},
				{HistoryID: "", Status: "skipped", StopMs: 6},
				{HistoryID: "a", Status: "passed", StopMs: 300},
			},
			want: []struct {
				historyID string
				status    string
				stopMs    int64
			}{{"", "broken", 5}, {"a", "passed", 300}, {"", "skipped", 6}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeResultsByHistoryID(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("length: got %d, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].HistoryID != w.historyID || got[i].Status != w.status || got[i].StopMs != w.stopMs {
					t.Errorf("result[%d]: got (%q,%q,%d), want (%q,%q,%d)",
						i, got[i].HistoryID, got[i].Status, got[i].StopMs,
						w.historyID, w.status, w.stopMs)
				}
			}
		})
	}
}
