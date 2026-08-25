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
	tests := []struct {
		name      string
		in        []store.TestResult
		wantKept  []string // history_ids of the kept rows, in order
		wantMerge map[string][]string
		wantWarn  []string // substrings expected in the warnings, in order
	}{
		{
			name:      "empty input",
			in:        nil,
			wantKept:  nil,
			wantMerge: map[string][]string{},
		},
		{
			name: "no duplicates passes through unchanged",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a:a", StatusMessage: "boom"},
				{FullName: "b", HistoryID: "b:b"},
			},
			wantKept:  []string{"a:a", "b:b"},
			wantMerge: map[string][]string{},
		},
		{
			name: "enriched row wins over empty shell",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a.a"},
				{FullName: "a", HistoryID: "a:a", StatusMessage: "boom"},
			},
			wantKept:  []string{"a:a"},
			wantMerge: map[string][]string{"a:a": {"a.a"}},
		},
		{
			name: "enriched row wins even when it arrives first",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a:a", StatusMessage: "boom"},
				{FullName: "a", HistoryID: "a.a"},
			},
			wantKept:  []string{"a:a"},
			wantMerge: map[string][]string{"a:a": {"a.a"}},
		},
		{
			name: "tie on empty message falls back to the colon scheme",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a.a"},
				{FullName: "a", HistoryID: "a:a"},
			},
			wantKept:  []string{"a:a"},
			wantMerge: map[string][]string{"a:a": {"a.a"}},
		},
		{
			// Two rows that BOTH carry a message are two results, not a twin
			// pair — even when the messages happen to read the same. Dropping
			// one would report a failure that was never merged away.
			name: "two messages under one full_name are kept and warned about",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a:1", StatusMessage: "boom"},
				{FullName: "a", HistoryID: "a:2", StatusMessage: "boom"},
			},
			wantKept:  []string{"a:1", "a:2"},
			wantMerge: map[string][]string{},
			wantWarn:  []string{`2 tests share full_name "a" with distinct results`},
		},
		{
			// The parameterized case the merge must survive: same full_name,
			// genuinely different failures.
			name: "parameterized pair with distinct messages both survive",
			in: []store.TestResult{
				{FullName: "p", HistoryID: "p:1", StatusMessage: "expected 1, got 2"},
				{FullName: "p", HistoryID: "p:2", StatusMessage: "expected 7, got 9"},
			},
			wantKept:  []string{"p:1", "p:2"},
			wantMerge: map[string][]string{},
			wantWarn:  []string{`2 tests share full_name "p" with distinct results`},
		},
		{
			// Shells alongside two real results are not attributable to either
			// one, so the whole group is left intact rather than half-merged.
			name: "distinct results plus shells are all kept",
			in: []store.TestResult{
				{FullName: "p", HistoryID: "p.1"},
				{FullName: "p", HistoryID: "p:1", StatusMessage: "first"},
				{FullName: "p", HistoryID: "p:2", StatusMessage: "second"},
			},
			wantKept:  []string{"p.1", "p:1", "p:2"},
			wantMerge: map[string][]string{},
			wantWarn:  []string{`2 tests share full_name "p" with distinct results`},
		},
		{
			name: "a non-empty message beats the colon scheme",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a:a"},
				{FullName: "a", HistoryID: "a.a", StatusMessage: "boom"},
			},
			wantKept:  []string{"a.a"},
			wantMerge: map[string][]string{"a.a": {"a:a"}},
		},
		{
			name: "kept rows preserve first-seen order across groups",
			in: []store.TestResult{
				{FullName: "b", HistoryID: "b.b"},
				{FullName: "a", HistoryID: "a.a"},
				{FullName: "b", HistoryID: "b:b", StatusMessage: "boom"},
				{FullName: "a", HistoryID: "a:a", StatusMessage: "bang"},
			},
			wantKept:  []string{"b:b", "a:a"},
			wantMerge: map[string][]string{"b:b": {"b.b"}, "a:a": {"a.a"}},
		},
		{
			name: "three twins collapse to one with both dropped ids recorded",
			in: []store.TestResult{
				{FullName: "a", HistoryID: "a.1"},
				{FullName: "a", HistoryID: "a:2", StatusMessage: "boom"},
				{FullName: "a", HistoryID: "a.3"},
			},
			wantKept:  []string{"a:2"},
			wantMerge: map[string][]string{"a:2": {"a.1", "a.3"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kept, merged, warnings := dedupeByFullName(tc.in)

			gotKept := make([]string, 0, len(kept))
			for _, r := range kept {
				gotKept = append(gotKept, r.HistoryID)
			}
			if len(gotKept) == 0 {
				gotKept = nil
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

// TestDedupeByFullName_EmptyFullNameNotCollapsed guards against collapsing
// unrelated tests: rows with an empty full_name are not the Playwright twin
// case and must each survive, exactly as an empty history_id is never a valid
// lookup key elsewhere in the codebase.
func TestDedupeByFullName_EmptyFullNameNotCollapsed(t *testing.T) {
	kept, merged, warnings := dedupeByFullName([]store.TestResult{
		{FullName: "", HistoryID: "x"},
		{FullName: "", HistoryID: "y"},
	})
	if len(kept) != 2 {
		t.Errorf("kept: got %d rows, want 2 (empty full_name must not group)", len(kept))
	}
	if len(merged) != 0 {
		t.Errorf("mergedIDs: got %v, want empty", merged)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: got %v, want none (empty full_name is not a shared identity)", warnings)
	}
}
