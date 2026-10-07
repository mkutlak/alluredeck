package handlers

import (
	"context"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// runFailureItems serves GetRunFailures over rows and returns the decoded
// items in row order, so a test can look at which keys an item carries.
func runFailureItems(t *testing.T, rows []store.RunFailureRow) []map[string]any {
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
	data, _ := jsonAt(body, "data").([]any)
	if len(data) != len(rows) {
		t.Fatalf("got %d items, want %d: %v", len(data), len(rows), body)
	}
	items := make([]map[string]any, len(data))
	for i := range data {
		items[i], _ = data[i].(map[string]any)
		if items[i] == nil {
			t.Fatalf("data.%d is not an object: %v", i, data[i])
		}
	}
	return items
}

// TestGetRunFailures_FailedStep: failed_step is the deepest failed step, the
// last element of the failed-step path, and the key is left out of the JSON
// (not sent as "") when the test has no failed step.
func TestGetRunFailures_FailedStep(t *testing.T) {
	for _, tt := range []struct {
		name string
		path []string
		want string // "" means the key must be absent
	}{
		{"nested path", []string{"Before Hooks", "fixture: db", "connect"}, "connect"},
		{"single step", []string{"click"}, "click"},
		{"empty path", []string{}, ""},
		{"nil path", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := runFailureItems(t, []store.RunFailureRow{{StatusMessage: "boom", FailedStepPath: tt.path}})[0]
			got, present := item["failed_step"]
			if tt.want == "" {
				if present {
					t.Errorf("failed_step = %#v, want the key absent", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("failed_step = %#v, want %q", got, tt.want)
			}
		})
	}
}

// TestGetRunFailures_FailedStepTellsClustersApart: two tests with the same
// message and phase but different deepest steps get different cluster keys, and
// failed_step is what tells the two clusters apart.
func TestGetRunFailures_FailedStepTellsClustersApart(t *testing.T) {
	items := runFailureItems(t, []store.RunFailureRow{
		{StatusMessage: "timeout", FailedStepPath: []string{"click", "wait for button"}},
		{StatusMessage: "timeout", FailedStepPath: []string{"click", "wait for dialog"}},
	})
	steps := [2]string{}
	keys := [2]string{}
	for i := range items {
		steps[i], _ = items[i]["failed_step"].(string)
		keys[i], _ = items[i]["cluster_key"].(string)
	}
	if steps[0] != "wait for button" || steps[1] != "wait for dialog" {
		t.Errorf("failed_step = %q, %q; want %q, %q", steps[0], steps[1], "wait for button", "wait for dialog")
	}
	if keys[0] == "" || keys[0] == keys[1] {
		t.Errorf("cluster_keys %q and %q must be set and differ", keys[0], keys[1])
	}
}

// TestGetRunFailures_FailedStepIsWhatTheKeyIsBuiltFrom: rebuilding the key from
// the item's own failed_step gives the item's cluster_key, so the step shown is
// the step the key stands for. A row with no failed step rebuilds from "".
func TestGetRunFailures_FailedStepIsWhatTheKeyIsBuiltFrom(t *testing.T) {
	rows := []store.RunFailureRow{
		{StatusMessage: "test msg", FailedStepMessage: "step msg", FailedStepPath: []string{"Before Hooks", "fixture: db"}},
		{StatusMessage: "test msg\nsecond line", FailedStepPath: []string{"click", "expect"}},
		{StatusMessage: "bare"},
	}
	paths := [][]string{rows[0].FailedStepPath, rows[1].FailedStepPath, nil}
	items := runFailureItems(t, rows)
	for i, item := range items {
		step, _ := item["failed_step"].(string)
		got, _ := item["cluster_key"].(string)
		want := runner.ClusterKey(
			runner.ClusterMessage(rows[i].FailedStepMessage, rows[i].StatusMessage),
			triage.FailurePhase(paths[i]),
			step)
		if got != want {
			t.Errorf("row %d: cluster_key = %q, want %q (rebuilt from failed_step %q)", i, got, want, step)
		}
	}
}
