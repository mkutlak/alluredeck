package pg_test

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// TestDeleteShellTwinBatch seeds every documented twin shape into one build and
// drains the batch delete: it removes exactly the message-less, childless
// shells whose richer sibling authorizes it — the contract migration 0049
// carried before the cleanup moved off the startup path — never more than
// limit rows per call, and finds nothing once drained.
func TestDeleteShellTwinBatch(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	// shell is what the historic double-ingestion bug left behind.
	shell := func(fullName, historyID string) { f.insert(f.result(b, fullName, "failed", historyID)) }
	enriched := func(r parser.Result) {
		r.Name, r.StopMs = "enriched "+r.FullName, 100
		f.insertFull(b, &r)
	}

	// Red twins: shell + enriched sibling with a status_message.
	for _, name := range []string{"suite.A", "suite.B", "suite.C"} {
		shell(name, "gen-"+name+".h")
		enriched(parser.Result{FullName: name, HistoryID: "raw-" + name + ":h", Status: "failed", StatusMessage: "boom"})
	}
	// Green twin: shell + enriched sibling with NO message but labels.
	shell("suite.Green", "genB.h")
	enriched(parser.Result{FullName: "suite.Green", HistoryID: "rawB:h", Status: "passed", Labels: []parser.Label{{Name: "suite", Value: "s"}}})
	// Both twins bare: ambiguous, both survive.
	shell("suite.Ambiguous", "genC1.h")
	shell("suite.Ambiguous", "genC2.h")
	// Empty full_name: never grouped, even beside a message-carrying empty-name row.
	shell("", "genD1.h")
	enriched(parser.Result{FullName: "", HistoryID: "rawD:h", Status: "failed", StatusMessage: "unrelated"})
	// Parameterized variants: same full_name, each with its own parameters.
	for _, hid := range []string{"rawE1:h", "rawE2:h"} {
		enriched(parser.Result{FullName: "suite.Param", HistoryID: hid, Status: "passed", Parameters: []parser.Parameter{{Name: "variant", Value: hid}}})
	}

	for i, step := range []struct {
		limit int
		want  int64
	}{{1, 1}, {1, 1}, {100, 2}, {100, 0}} {
		n, err := f.results.DeleteShellTwinBatch(f.ctx, step.limit)
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		if n != step.want {
			t.Fatalf("batch %d (limit %d) deleted %d rows, want %d", i, step.limit, n, step.want)
		}
	}
	for name, want := range map[string]int{
		"suite.A": 1, "suite.B": 1, "suite.C": 1, "suite.Green": 1, "suite.Ambiguous": 2, "": 2, "suite.Param": 2,
	} {
		if n := f.count("SELECT count(*) FROM test_results WHERE build_id=$1 AND full_name=$2", b, name); n != want {
			t.Errorf("%q: %d rows, want %d", name, n, want)
		}
	}
}
