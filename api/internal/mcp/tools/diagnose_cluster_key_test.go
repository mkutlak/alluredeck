package tools

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/runner"
)

// TestClusterKey_IsTheSharedRunnerKey: the clusters of diagnose_pipeline (and
// diagnose_failure, with its own inputs) are keyed by the function that makes
// the REST failures list's cluster_key, so for the same inputs the browser and
// the IDE show the same groups. That holds only while this package delegates to
// runner.ClusterKey with the deepest failed step as the last element of the
// path.
//
// The REST half of the contract (handlers derives the key from the same
// runner.ClusterKey) is pinned in the handlers package; it cannot be asserted
// from here because handlers imports this package.
func TestClusterKey_IsTheSharedRunnerKey(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message string
		phase   string
		path    []string
		last    string
	}{
		{"deepest step is the last element", "boom 10.0.0.1", "before_hooks", []string{"Before Hooks", "fixture: db"}, "fixture: db"},
		{"single step", "boom", "test_body", []string{"click"}, "click"},
		{"no path", "boom", "test_body", nil, ""},
		{"no message", "", "test_body", []string{"click"}, "click"},
		{"whitespace message", " \n", "setup_fixture", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := clusterFixture("t", tt.message, tt.phase, tt.path...)
			want := runner.ClusterKey(tt.message, tt.phase, tt.last)
			if got := clusterKey(&d); got != want {
				t.Errorf("clusterKey = %q, want runner.ClusterKey = %q", got, want)
			}
		})
	}
}
