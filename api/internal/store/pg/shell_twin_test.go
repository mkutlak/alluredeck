package pg_test

import (
	"context"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// seedShell inserts a message-less, childless stability row — the shape the
// historic double-ingestion bug left behind.
func seedShell(t *testing.T, ctx context.Context, trStore interface {
	InsertBatch(ctx context.Context, results []store.TestResult) error
}, projectID, buildID int64, fullName, historyID string) {
	t.Helper()
	err := trStore.InsertBatch(ctx, []store.TestResult{{
		BuildID: buildID, ProjectID: projectID,
		TestName: fullName, FullName: fullName,
		Status: "failed", HistoryID: historyID, DurationMs: 10,
	}})
	if err != nil {
		t.Fatalf("seed shell %q/%q: %v", fullName, historyID, err)
	}
}

// TestDeleteShellTwinBatch_Semantics seeds every documented twin shape into one
// build and asserts the batch delete removes exactly the shells whose richer
// sibling authorizes it — the same contract migration 0049 originally carried
// before the cleanup moved off the startup path.
func TestDeleteShellTwinBatch_Semantics(t *testing.T) {
	s, trStore, projectID, buildID := attemptsFixture(t, "shell-twin-sem")
	ctx := context.Background()

	// Case A — red twin: shell + enriched sibling with a status_message.
	seedShell(t, ctx, trStore, projectID, buildID, "suite.Red", "genA.h")
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "suite.Red", FullName: "suite.Red", HistoryID: "rawA:h",
		Status: "failed", StatusMessage: "boom", StopMs: 100,
	}}); err != nil {
		t.Fatalf("seed enriched red: %v", err)
	}

	// Case B — green twin: shell + enriched sibling with NO message but labels.
	seedShell(t, ctx, trStore, projectID, buildID, "suite.Green", "genB.h")
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "suite.Green", FullName: "suite.Green", HistoryID: "rawB:h",
		Status: "passed", StopMs: 100,
		Labels: []parser.Label{{Name: "suite", Value: "s"}},
	}}); err != nil {
		t.Fatalf("seed enriched green: %v", err)
	}

	// Case C — both twins bare: ambiguous, both must survive.
	seedShell(t, ctx, trStore, projectID, buildID, "suite.Ambiguous", "genC1.h")
	seedShell(t, ctx, trStore, projectID, buildID, "suite.Ambiguous", "genC2.h")

	// Case D — empty full_name: never grouped, never deleted, even though a
	// message-carrying empty-name row exists in the same build.
	seedShell(t, ctx, trStore, projectID, buildID, "", "genD1.h")
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "unnamed", FullName: "", HistoryID: "rawD:h",
		Status: "failed", StatusMessage: "unrelated", StopMs: 100,
	}}); err != nil {
		t.Fatalf("seed enriched empty-name: %v", err)
	}

	// Case E — parameterized variants: same full_name, each with its own
	// test_parameters child rows. Both must survive.
	for _, hid := range []string{"rawE1:h", "rawE2:h"} {
		if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
			Name: "suite.Param", FullName: "suite.Param", HistoryID: hid,
			Status: "passed", StopMs: 100,
			Parameters: []parser.Parameter{{Name: "variant", Value: hid}},
		}}); err != nil {
			t.Fatalf("seed parameterized %s: %v", hid, err)
		}
	}

	deleted, err := trStore.DeleteShellTwinBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteShellTwinBatch: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2 (the red and green shells)", deleted)
	}

	assertCount := func(fullName string, want int) {
		t.Helper()
		var n int
		if err := s.Pool().QueryRow(ctx,
			`SELECT count(*) FROM test_results WHERE build_id=$1 AND full_name=$2`,
			buildID, fullName).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", fullName, err)
		}
		if n != want {
			t.Errorf("%q: %d rows, want %d", fullName, n, want)
		}
	}
	assertCount("suite.Red", 1)
	assertCount("suite.Green", 1)
	assertCount("suite.Ambiguous", 2)
	assertCount("", 2)
	assertCount("suite.Param", 2)

	// Idempotence: a second pass finds nothing.
	deleted, err = trStore.DeleteShellTwinBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteShellTwinBatch (second pass): %v", err)
	}
	if deleted != 0 {
		t.Errorf("second pass deleted %d rows, want 0", deleted)
	}
}

// TestDeleteShellTwinBatch_RespectsLimit seeds three deletable twins and
// drains them with limit=1: every batch must delete exactly one row and the
// backlog must converge to zero.
func TestDeleteShellTwinBatch_RespectsLimit(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "shell-twin-limit")
	ctx := context.Background()

	for _, name := range []string{"suite.A", "suite.B", "suite.C"} {
		seedShell(t, ctx, trStore, projectID, buildID, name, "gen-"+name+".h")
		if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
			Name: name, FullName: name, HistoryID: "raw-" + name + ":h",
			Status: "failed", StatusMessage: "boom", StopMs: 100,
		}}); err != nil {
			t.Fatalf("seed enriched %s: %v", name, err)
		}
	}

	for i := range 3 {
		n, err := trStore.DeleteShellTwinBatch(ctx, 1)
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		if n != 1 {
			t.Fatalf("batch %d deleted %d rows, want 1", i, n)
		}
	}
	n, err := trStore.DeleteShellTwinBatch(ctx, 1)
	if err != nil {
		t.Fatalf("final batch: %v", err)
	}
	if n != 0 {
		t.Errorf("final batch deleted %d rows, want 0", n)
	}
}
