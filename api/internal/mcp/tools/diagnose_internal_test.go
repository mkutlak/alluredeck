package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestDedupeByFullName pins the twin-row collapse rule. Playwright ingestion
// writes two test_results rows per test — an enriched one (history_id scheme
// "md5:md5", carrying status_message/steps/attachments) and an empty shell
// (scheme "md5.md5") — so diagnose_failure would otherwise report every real
// failure twice. Collapse is keyed on full_name; the survivor is the row with
// a non-empty StatusMessage, and on a tie the ":"-scheme history_id wins
// because that is the row the enrichment tables are attached to.
//
// The collapse only fires on that twin signature: a group whose rows each
// carry their own message holds distinct results, not duplicates, and is left
// alone with a warning.
func TestDedupeByFullName(t *testing.T) {
	row := func(fullName, historyID, msg string) store.TestResult {
		return store.TestResult{FullName: fullName, HistoryID: historyID, StatusMessage: msg}
	}
	distinct := func(name string) string { return `2 tests share full_name "` + name + `" with distinct results` }

	tests := []struct {
		name      string
		in        []store.TestResult
		wantKept  []string // history_ids of the kept rows, in order
		wantMerge map[string][]string
		wantWarn  []string // substrings expected in the warnings, in order
	}{
		{"no duplicates passes through unchanged", []store.TestResult{row("a", "a:a", "boom"), row("b", "b:b", "")},
			[]string{"a:a", "b:b"}, map[string][]string{}, nil},
		{"enriched row wins over empty shell", []store.TestResult{row("a", "a.a", ""), row("a", "a:a", "boom")},
			[]string{"a:a"}, map[string][]string{"a:a": {"a.a"}}, nil},
		{"enriched row wins even when it arrives first", []store.TestResult{row("a", "a:a", "boom"), row("a", "a.a", "")},
			[]string{"a:a"}, map[string][]string{"a:a": {"a.a"}}, nil},
		{"tie on empty message falls back to the colon scheme", []store.TestResult{row("a", "a.a", ""), row("a", "a:a", "")},
			[]string{"a:a"}, map[string][]string{"a:a": {"a.a"}}, nil},
		{"a non-empty message beats the colon scheme", []store.TestResult{row("a", "a:a", ""), row("a", "a.a", "boom")},
			[]string{"a.a"}, map[string][]string{"a.a": {"a:a"}}, nil},
		// Two rows that BOTH carry a message are two results — a parameterized
		// test whose parameters never reached full_name — even when the
		// messages read the same. Dropping one would hide a real failure.
		{"two messages under one full_name are kept and warned about", []store.TestResult{row("a", "a:1", "boom"), row("a", "a:2", "boom")},
			[]string{"a:1", "a:2"}, map[string][]string{}, []string{distinct("a")}},
		// Shells alongside two real results are not attributable to either
		// one, so the whole group is left intact rather than half-merged.
		{"distinct results plus shells are all kept", []store.TestResult{row("p", "p.1", ""), row("p", "p:1", "first"), row("p", "p:2", "second")},
			[]string{"p.1", "p:1", "p:2"}, map[string][]string{}, []string{distinct("p")}},
		{"kept rows preserve first-seen order across groups",
			[]store.TestResult{row("b", "b.b", ""), row("a", "a.a", ""), row("b", "b:b", "boom"), row("a", "a:a", "bang")},
			[]string{"b:b", "a:a"}, map[string][]string{"b:b": {"b.b"}, "a:a": {"a.a"}}, nil},
		{"three twins collapse to one with both dropped ids recorded", []store.TestResult{row("a", "a.1", ""), row("a", "a:2", "boom"), row("a", "a.3", "")},
			[]string{"a:2"}, map[string][]string{"a:2": {"a.1", "a.3"}}, nil},
		// An empty name is not an identity; grouping on it would merge
		// unrelated tests.
		{"empty full_name never groups", []store.TestResult{row("", "x", ""), row("", "y", "")},
			[]string{"x", "y"}, map[string][]string{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kept, merged, warnings := dedupeByFullName(tc.in)
			var gotKept []string
			for _, r := range kept {
				gotKept = append(gotKept, r.HistoryID)
			}
			if !reflect.DeepEqual(gotKept, tc.wantKept) {
				t.Errorf("kept history_ids: got %v, want %v", gotKept, tc.wantKept)
			}
			if !reflect.DeepEqual(merged, tc.wantMerge) {
				t.Errorf("mergedIDs: got %v, want %v", merged, tc.wantMerge)
			}
			if len(warnings) != len(tc.wantWarn) {
				t.Fatalf("warnings: got %v, want %d entries containing %v", warnings, len(tc.wantWarn), tc.wantWarn)
			}
			for i, want := range tc.wantWarn {
				if !strings.Contains(warnings[i], want) {
					t.Errorf("warnings[%d] = %q, want substring %q", i, warnings[i], want)
				}
			}
		})
	}
}
