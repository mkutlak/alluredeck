package pg

import (
	"testing"
)

// TestFailedStepPath exercises the pure step-tree walk used by
// GetFailedStepPath: given a flat list of test_steps rows it must reconstruct
// the ordered root→leaf failed-step path and surface the deepest failed step's
// status_message.
func TestFailedStepPath(t *testing.T) {
	tests := []struct {
		name      string
		steps     []stepRow
		wantPath  []string
		wantError string
	}{
		{
			name:      "no steps",
			steps:     nil,
			wantPath:  []string{},
			wantError: "",
		},
		{
			name: "all passed - no failed path",
			steps: []stepRow{
				{id: 1, parentID: nil, name: "root", status: "passed", stepOrder: 0},
				{id: 2, parentID: new(int64(1)), name: "child", status: "passed", stepOrder: 0},
			},
			wantPath:  []string{},
			wantError: "",
		},
		{
			name: "single-level failed step",
			steps: []stepRow{
				{id: 1, parentID: nil, name: "Before Hooks", status: "passed", stepOrder: 0},
				{id: 2, parentID: nil, name: "Login", status: "failed", statusMessage: "assertion failed", stepOrder: 1},
			},
			wantPath:  []string{"Login"},
			wantError: "assertion failed",
		},
		{
			name: "nested failed path descends to deepest failed step",
			steps: []stepRow{
				// root failed step
				{id: 1, parentID: nil, name: "Test Body", status: "failed", statusMessage: "outer", stepOrder: 0},
				// children of step 1: a passed one and a failed one
				{id: 2, parentID: new(int64(1)), name: "Setup", status: "passed", stepOrder: 0},
				{id: 3, parentID: new(int64(1)), name: "Call API", status: "broken", statusMessage: "status 500", stepOrder: 1},
				// child of step 3 (deepest failed)
				{id: 4, parentID: new(int64(3)), name: "HTTP GET /users", status: "broken", statusMessage: "status 500 from /users", stepOrder: 0},
			},
			wantPath:  []string{"Test Body", "Call API", "HTTP GET /users"},
			wantError: "status 500 from /users",
		},
		{
			name: "picks lowest step_order among failed siblings",
			steps: []stepRow{
				{id: 1, parentID: nil, name: "second-failed", status: "failed", statusMessage: "second", stepOrder: 2},
				{id: 2, parentID: nil, name: "first-failed", status: "failed", statusMessage: "first", stepOrder: 1},
				{id: 3, parentID: nil, name: "passed", status: "passed", stepOrder: 0},
			},
			// Query orders by step_order, so siblings arrive [passed, first, second];
			// firstFailedChild returns "first-failed".
			wantPath:  []string{"first-failed"},
			wantError: "first",
		},
		{
			name: "deepest failed step has empty status_message",
			steps: []stepRow{
				{id: 1, parentID: nil, name: "Test Body", status: "failed", statusMessage: "outer", stepOrder: 0},
				{id: 2, parentID: new(int64(1)), name: "inner", status: "failed", statusMessage: "", stepOrder: 0},
			},
			wantPath:  []string{"Test Body", "inner"},
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotErr := failedStepPath(tt.steps)

			if len(gotPath) != len(tt.wantPath) {
				t.Fatalf("path length: got %v, want %v", gotPath, tt.wantPath)
			}
			for i := range gotPath {
				if gotPath[i] != tt.wantPath[i] {
					t.Errorf("path[%d]: got %q, want %q", i, gotPath[i], tt.wantPath[i])
				}
			}
			if gotErr != tt.wantError {
				t.Errorf("error message: got %q, want %q", gotErr, tt.wantError)
			}
		})
	}
}
