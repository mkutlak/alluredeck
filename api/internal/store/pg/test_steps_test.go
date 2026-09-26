package pg

import (
	"slices"
	"testing"
)

// TestFailedStepPath exercises the pure step-tree walk behind
// GetFailedStepPath: from flat test_steps rows it rebuilds the root→leaf
// failed-step path and surfaces the deepest failed step's status_message.
func TestFailedStepPath(t *testing.T) {
	tests := []struct {
		name      string
		steps     []stepRow
		wantPath  []string
		wantError string
	}{
		{name: "no steps", wantPath: []string{}},
		{name: "all passed - no failed path", steps: []stepRow{
			{id: 1, name: "root", status: "passed"},
			{id: 2, parentID: new(int64(1)), name: "child", status: "passed"},
		}, wantPath: []string{}},
		{name: "single-level failed step", steps: []stepRow{
			{id: 1, name: "Before Hooks", status: "passed"},
			{id: 2, name: "Login", status: "failed", statusMessage: "assertion failed", stepOrder: 1},
		}, wantPath: []string{"Login"}, wantError: "assertion failed"},
		{name: "nested failed path descends to deepest failed step", steps: []stepRow{
			{id: 1, name: "Test Body", status: "failed", statusMessage: "outer"},
			{id: 2, parentID: new(int64(1)), name: "Setup", status: "passed"},
			{id: 3, parentID: new(int64(1)), name: "Call API", status: "broken", statusMessage: "status 500", stepOrder: 1},
			{id: 4, parentID: new(int64(3)), name: "HTTP GET /users", status: "broken", statusMessage: "status 500 from /users"},
		}, wantPath: []string{"Test Body", "Call API", "HTTP GET /users"}, wantError: "status 500 from /users"},
		{name: "picks lowest step_order among failed siblings", steps: []stepRow{
			{id: 1, name: "second-failed", status: "failed", statusMessage: "second", stepOrder: 2},
			{id: 2, name: "first-failed", status: "failed", statusMessage: "first", stepOrder: 1},
			{id: 3, name: "passed", status: "passed"},
		}, wantPath: []string{"first-failed"}, wantError: "first"},
		{name: "deepest failed step has empty status_message", steps: []stepRow{
			{id: 1, name: "Test Body", status: "failed", statusMessage: "outer"},
			{id: 2, parentID: new(int64(1)), name: "inner", status: "failed"},
		}, wantPath: []string{"Test Body", "inner"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotErr := failedStepPath(tt.steps)
			if !slices.Equal(gotPath, tt.wantPath) || gotErr != tt.wantError {
				t.Errorf("failedStepPath = %q, %q; want %q, %q", gotPath, gotErr, tt.wantPath, tt.wantError)
			}
		})
	}
}
