package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// clusterKeysOf serves GetRunFailures over rows and returns each failure's
// cluster_key in row order, failing the test when any item lacks one.
func clusterKeysOf(t *testing.T, rows []store.RunFailureRow) []string {
	t.Helper()
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	parent, _ := projects.CreateProject(ctx, "parent")
	child, _ := projects.CreateProjectWithParent(ctx, "ui-users", parent.ID)
	for i := range rows {
		rows[i].ProjectID, rows[i].Slug, rows[i].Status = child.ID, "ui-users", "failed"
	}
	ps := &testutil.MockPipelineStore{
		ListRunFailuresFn: func(context.Context, int64, string, int) ([]store.RunFailureRow, error) { return rows, nil },
	}
	h := NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())

	code, body := serveJSON(t, h.GetRunFailures, http.MethodGet, "/api/v1/pipeline-runs/p/failures", "",
		"project_id", "1", "run_key", "p")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}
	keys := make([]string, len(rows))
	for i := range rows {
		k, _ := jsonAt(body, "data."+strconv.Itoa(i)+".cluster_key").(string)
		if k == "" {
			t.Fatalf("data.%d.cluster_key missing or empty in %v", i, body)
		}
		keys[i] = k
	}
	return keys
}

// TestGetRunFailures_ClusterKey: tests that share a failure shape (normalised
// message, failure phase, deepest failed step) share a cluster_key, the same
// key diagnose_pipeline clusters on; any difference in that shape splits them.
func TestGetRunFailures_ClusterKey(t *testing.T) {
	hooks := []string{"Before Hooks", "fixture: db"}
	body := []string{"click", "expect"}

	for _, tt := range []struct {
		name string
		a, b store.RunFailureRow
		same bool
	}{
		{"same message and path",
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: hooks},
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: hooks}, true},
		{"dynamic values in the message are normalised away",
			store.RunFailureRow{StatusMessage: "connect ECONNREFUSED 10.0.0.1:5432", FailedStepPath: body},
			store.RunFailureRow{StatusMessage: "connect ECONNREFUSED 10.9.8.7:5432", FailedStepPath: body}, true},
		{"outer steps differ but the deepest failed step is the same",
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"outer a", "expect"}},
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"outer b", "expect"}}, true},
		{"a Before Hooks path differs from no path",
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"Before Hooks"}},
			store.RunFailureRow{StatusMessage: "boom"}, false},
		{"different failure phase, same deepest step",
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"Before Hooks", "expect"}},
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"expect"}}, false},
		{"different deepest step",
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"click", "expect"}},
			store.RunFailureRow{StatusMessage: "boom", FailedStepPath: []string{"click", "navigate"}}, false},
		{"the deepest failed step's message beats the test's message",
			store.RunFailureRow{StatusMessage: "test message one", FailedStepMessage: "step message", FailedStepPath: body},
			store.RunFailureRow{StatusMessage: "test message two", FailedStepMessage: "step message", FailedStepPath: body}, true},
		{"the test's message does not mask a different step message",
			store.RunFailureRow{StatusMessage: "same test message", FailedStepMessage: "step message one", FailedStepPath: body},
			store.RunFailureRow{StatusMessage: "same test message", FailedStepMessage: "step message two", FailedStepPath: body}, false},
		{"the test's message is used when the step has none",
			store.RunFailureRow{StatusMessage: "same test message", FailedStepPath: body},
			store.RunFailureRow{StatusMessage: "same test message", FailedStepPath: body}, true},
		{"a step message differs from the same text absent",
			store.RunFailureRow{StatusMessage: "same test message", FailedStepMessage: "step message", FailedStepPath: body},
			store.RunFailureRow{StatusMessage: "same test message", FailedStepPath: body}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keys := clusterKeysOf(t, []store.RunFailureRow{tt.a, tt.b})
			if got := keys[0] == keys[1]; got != tt.same {
				t.Errorf("keys %q and %q: equal = %v, want %v", keys[0], keys[1], got, tt.same)
			}
		})
	}
}

// TestGetRunFailures_ClusterKeyIsTheSharedKey: cluster_key is exactly
// runner.ClusterKey over the winning message, triage's failure phase of the
// path and the path's last step, the inputs diagnose_pipeline feeds it, so the
// browser and the IDE agree on the groups. It is emitted on every item, even a
// test with no message and no failed step, and is 16 lowercase hex chars.
func TestGetRunFailures_ClusterKeyIsTheSharedKey(t *testing.T) {
	rows := []store.RunFailureRow{
		{TestName: "hook", StatusMessage: "test msg", FailedStepMessage: "step msg", FailedStepPath: []string{"Before Hooks", "fixture: db"}},
		{TestName: "body", StatusMessage: "test msg\nsecond line", FailedStepPath: []string{"click"}},
		{TestName: "bare"},
	}
	want := []string{
		runner.ClusterKey("step msg", triage.PhaseBeforeHooks, "fixture: db"),
		runner.ClusterKey("test msg\nsecond line", triage.PhaseTestBody, "click"),
		runner.ClusterKey("", triage.PhaseTestBody, ""),
	}
	shape := regexp.MustCompile(`^[0-9a-f]{16}$`)

	got := clusterKeysOf(t, rows)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d (%s): cluster_key = %q, want %q", i, rows[i].TestName, got[i], want[i])
		}
		if !shape.MatchString(got[i]) {
			t.Errorf("row %d: cluster_key %q is not 16 lowercase hex chars", i, got[i])
		}
	}
}

// TestGetRunFailures_ClusterKeyCutsTheMessageInBytes: diagnose_pipeline cuts the
// message it clusters on at 500 bytes, so non-ASCII messages that agree up to
// that byte are one cluster there. The REST key must say the same, whichever
// script the message is in and wherever, even mid-rune, the cut falls.
func TestGetRunFailures_ClusterKeyCutsTheMessageInBytes(t *testing.T) {
	cyrillic := strings.Repeat("я", 250)        // 2 bytes each: exactly 500 bytes
	oddPrefix := strings.Repeat("я", 249) + "x" // 499 bytes: byte 500 is the first half of the next rune

	for _, tt := range []struct {
		name string
		a, b store.RunFailureRow
	}{
		{"test messages agree on their first 500 bytes",
			store.RunFailureRow{StatusMessage: cyrillic + " tail one"},
			store.RunFailureRow{StatusMessage: cyrillic + " tail two"}},
		{"step messages agree on their first 500 bytes",
			store.RunFailureRow{StatusMessage: "test one", FailedStepMessage: cyrillic + " tail one"},
			store.RunFailureRow{StatusMessage: "test two", FailedStepMessage: cyrillic + " tail two"}},
		{"the cut lands inside a rune",
			store.RunFailureRow{StatusMessage: oddPrefix + "я tail one"},
			store.RunFailureRow{StatusMessage: oddPrefix + "я tail two"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keys := clusterKeysOf(t, []store.RunFailureRow{tt.a, tt.b})
			if keys[0] != keys[1] {
				t.Errorf("keys %q and %q differ, want one cluster", keys[0], keys[1])
			}
		})
	}

	// Control: a difference inside the first 500 bytes still splits them.
	keys := clusterKeysOf(t, []store.RunFailureRow{
		{StatusMessage: "один " + cyrillic},
		{StatusMessage: "два " + cyrillic},
	})
	if keys[0] == keys[1] {
		t.Errorf("messages that differ before byte 500 share the key %q", keys[0])
	}
}
