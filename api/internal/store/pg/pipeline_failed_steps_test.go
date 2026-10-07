package pg_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestListRunFailures_FailedStepPath: every failure of a run carries the deepest
// failed step trail of its own test result and that step's message, the inputs
// of the shared cluster key. The walk is the one GetFailedStepPath does (root
// first, lowest step_order failed or broken child at each level, passed steps
// never picked), a test without a failed step gets neither, and the message is
// capped like status_message. A failed step whose parent did not fail is never
// reached, and a test without a history_id is not looked up (as in
// diagnose_pipeline). Failures of different builds of the run (shards) each get
// their own trail.
func TestListRunFailures_FailedStepPath(t *testing.T) {
	f := newFixture(t) // the parent
	child := f.newProject(f.id)
	const runKey = "pipeline-failed-steps"

	failed := func(name, historyID string, steps ...parser.Step) *parser.Result {
		return &parser.Result{
			Name: name, FullName: name + ".spec.js", HistoryID: historyID, Status: "failed",
			StatusMessage: "test message of " + name, StartMs: 1000, StopMs: 2000, Steps: steps,
		}
	}
	long := strings.Repeat("x", 600)

	shardOne := f.ciBuild(child.ID, 1, runKey)
	shardTwo := f.ciBuild(child.ID, 2, runKey)
	for buildID, rs := range map[int64][]*parser.Result{
		shardOne: {
			failed("fixture failure", "h-fixture",
				parser.Step{Name: "Before Hooks", Status: "failed", StatusMessage: "hooks failed", Steps: []parser.Step{
					{Name: "launch browser", Status: "passed"},
					{Name: "fixture: db", Status: "failed", StatusMessage: "connection refused"},
					{Name: "fixture: cache", Status: "failed", StatusMessage: "ignored: a later failed sibling"},
				}},
				parser.Step{Name: "Test Body", Status: "passed"},
			),
			failed("body failure", "h-body",
				parser.Step{Name: "open page", Status: "passed"},
				parser.Step{Name: "click submit", Status: "broken", StatusMessage: "locator timed out"},
			),
			failed("no steps", "h-none"),
			failed("only passed steps", "h-passed", parser.Step{Name: "open page", Status: "passed"}),
			failed("failed step under a passed parent", "h-orphan",
				parser.Step{Name: "Test Body", Status: "passed", Steps: []parser.Step{
					{Name: "inner", Status: "failed", StatusMessage: "never reached by the walk"},
				}},
			),
			// diagnose_pipeline looks no steps up for a test without a history_id,
			// so neither does the run failures list.
			failed("no history id", "", parser.Step{Name: "click submit", Status: "broken", StatusMessage: "locator timed out"}),
		},
		shardTwo: {
			failed("long step message", "h-long", parser.Step{Name: "assert", Status: "failed", StatusMessage: long}),
		},
	} {
		if err := f.results.InsertBatchFull(f.ctx, buildID, child.ID, rs); err != nil {
			t.Fatalf("InsertBatchFull: %v", err)
		}
	}

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, runKey, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	if len(rows) != 7 {
		t.Fatalf("got %d rows, want 7: %+v", len(rows), rows)
	}

	byTest := map[string]store.RunFailureRow{}
	for _, r := range rows {
		byTest[r.TestName] = r
	}
	for _, tt := range []struct {
		test    string
		path    []string
		message string
	}{
		{"fixture failure", []string{"Before Hooks", "fixture: db"}, "connection refused"},
		{"body failure", []string{"click submit"}, "locator timed out"},
		{"no steps", nil, ""},
		{"only passed steps", nil, ""},
		{"failed step under a passed parent", nil, ""},
		{"no history id", nil, ""},
		{"long step message", []string{"assert"}, long[:500]},
	} {
		got, ok := byTest[tt.test]
		if !ok {
			t.Errorf("%s: no row returned", tt.test)
			continue
		}
		if !slices.Equal(got.FailedStepPath, tt.path) || got.FailedStepMessage != tt.message {
			t.Errorf("%s: FailedStepPath, FailedStepMessage = %q, %q; want %q, %q",
				tt.test, got.FailedStepPath, got.FailedStepMessage, tt.path, tt.message)
		}
	}
}
