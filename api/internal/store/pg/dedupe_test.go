package pg

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// TestDedupeResultsByHistoryID covers the pure collapse that keeps retried
// Allure tests (one *-result.json per attempt, same historyId) from
// duplicating their enrichment children in InsertBatchFull: the latest attempt
// (greatest StopMs) wins regardless of input order, and empty historyIds are
// never collapsed. Rows are "historyId/status/stopMs".
func TestDedupeResultsByHistoryID(t *testing.T) {
	r := func(hid, status string, stop int64) *parser.Result {
		return &parser.Result{HistoryID: hid, Status: status, StopMs: stop}
	}
	tests := []struct {
		name string
		in   []*parser.Result
		want []string
	}{
		{"no duplicates preserves order", []*parser.Result{r("a", "passed", 10), r("b", "failed", 20)},
			[]string{"a/passed/10", "b/failed/20"}},
		{"latest attempt wins when earlier attempt is first", []*parser.Result{r("a", "failed", 100), r("a", "passed", 200)},
			[]string{"a/passed/200"}},
		{"latest attempt wins when latest attempt is first", []*parser.Result{r("a", "passed", 200), r("a", "failed", 100)},
			[]string{"a/passed/200"}},
		{"empty historyId entries are never collapsed", []*parser.Result{r("", "passed", 1), r("", "failed", 2)},
			[]string{"/passed/1", "/failed/2"}},
		// A reporter may omit the stop timestamp (StopMs 0). ">=" made the LAST
		// file ParseDir read win, so the survivor depended on filename order;
		// with no timestamp to order by, the first read wins.
		{"equal stop_ms keeps the first result read", []*parser.Result{r("a", "failed", 0), r("a", "passed", 0)},
			[]string{"a/failed/0"}},
		{"timestamped attempt outranks a missing stop_ms read first", []*parser.Result{r("a", "failed", 0), r("a", "passed", 100)},
			[]string{"a/passed/100"}},
		{"timestamped attempt outranks a missing stop_ms read last", []*parser.Result{r("a", "passed", 100), r("a", "failed", 0)},
			[]string{"a/passed/100"}},
		{"mixed empty and duplicate non-empty",
			[]*parser.Result{r("", "broken", 5), r("a", "failed", 100), r("", "skipped", 6), r("a", "passed", 300)},
			[]string{"/broken/5", "a/passed/300", "/skipped/6"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, res := range dedupeResultsByHistoryID(tt.in) {
				got = append(got, fmt.Sprintf("%s/%s/%d", res.HistoryID, res.Status, res.StopMs))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("survivors = %v, want %v", got, tt.want)
			}
		})
	}
}
