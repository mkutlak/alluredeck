package tools

import (
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// clusterFixture builds a DiagnoseTest the way diagnoseTest would: the error
// message, the failed-step path, and the failure phase that triage derived from
// that path.
func clusterFixture(fullName, errMsg, phase string, steps ...string) DiagnoseTest {
	return DiagnoseTest{
		FullName:       fullName,
		HistoryID:      fullName + ":h",
		Status:         "failed",
		ErrorMessage:   errMsg,
		FailedStepPath: steps,
		Signals:        triage.Signals{FailurePhase: phase},
	}
}

// withStatus attaches a repeated-status pattern to a fixture.
func withStatus(d DiagnoseTest, code int, endpoint string) DiagnoseTest {
	d.Signals.RepeatedStatusPattern = &triage.StatusPattern{StatusCode: code, Endpoint: endpoint}
	return d
}

// TestClusterFailingTests_GroupsSharedRootCause is the core case behind the
// feature: a report where several tests fail for one reason must read as one
// cluster, not as a flat list of identical-looking entries.
func TestClusterFailingTests_GroupsSharedRootCause(t *testing.T) {
	tests := []DiagnoseTest{
		clusterFixture("spec/a.ts > a", "API call failed with status 500", triage.PhaseBeforeHooks, "Before Hooks", "login"),
		clusterFixture("spec/b.ts > b", "API call failed with status 500", triage.PhaseBeforeHooks, "Before Hooks", "login"),
		clusterFixture("spec/c.ts > c", "API call failed with status 500", triage.PhaseBeforeHooks, "Before Hooks", "login"),
	}

	clusters, members := clusterFailingTests(tests)

	if len(clusters) != 1 {
		t.Fatalf("clusters: got %d, want 1", len(clusters))
	}
	c := clusters[0]
	if c.ClusterID != "c1" {
		t.Errorf("cluster_id: got %q, want c1", c.ClusterID)
	}
	if c.MemberCount != 3 {
		t.Errorf("member_count: got %d, want 3", c.MemberCount)
	}
	wantNames := []string{"spec/a.ts > a", "spec/b.ts > b", "spec/c.ts > c"}
	if !reflect.DeepEqual(c.MemberFullNames, wantNames) {
		t.Errorf("member_full_names: got %v, want %v", c.MemberFullNames, wantNames)
	}
	if c.RepresentativeFullName != "spec/a.ts > a" {
		t.Errorf("representative_full_name: got %q, want spec/a.ts > a", c.RepresentativeFullName)
	}
	if c.SharedError != "API call failed with status 500" {
		t.Errorf("shared_error: got %q", c.SharedError)
	}
	if c.FailurePhase != triage.PhaseBeforeHooks {
		t.Errorf("failure_phase: got %q, want before_hooks", c.FailurePhase)
	}
	if !reflect.DeepEqual(members, [][]int{{0, 1, 2}}) {
		t.Errorf("member indices: got %v, want [[0 1 2]]", members)
	}

	// Every member is tagged, and only the representative keeps the heavy text.
	for i := range tests {
		if tests[i].ClusterID != "c1" {
			t.Errorf("tests[%d].cluster_id: got %q, want c1", i, tests[i].ClusterID)
		}
	}
	if tests[0].ErrorMessage == "" || len(tests[0].FailedStepPath) == 0 {
		t.Errorf("representative must keep error_message and failed_step_path, got %+v", tests[0])
	}
	for _, i := range []int{1, 2} {
		if tests[i].ErrorMessage != "" {
			t.Errorf("tests[%d].error_message: got %q, want cleared for a non-representative", i, tests[i].ErrorMessage)
		}
		if tests[i].FailedStepPath != nil {
			t.Errorf("tests[%d].failed_step_path: got %v, want nil for a non-representative", i, tests[i].FailedStepPath)
		}
		// Signals and the rest of the entry survive the token cut.
		if tests[i].Signals.FailurePhase != triage.PhaseBeforeHooks {
			t.Errorf("tests[%d] signals must survive the token cut, got %+v", i, tests[i].Signals)
		}
	}
}

// TestClusterFailingTests_NormalizesDynamicValues verifies the cluster key runs
// through NormalizeMessage, so two failures that differ only in a request id or
// a timestamp still land in the same cluster.
func TestClusterFailingTests_NormalizesDynamicValues(t *testing.T) {
	tests := []DiagnoseTest{
		clusterFixture("a", "request 11111111-2222-3333-4444-555555555555 failed at 2026-01-02T03:04:05Z", triage.PhaseTestBody, "Test Body"),
		clusterFixture("b", "request 99999999-8888-7777-6666-555555555555 failed at 2026-05-06T07:08:09Z", triage.PhaseTestBody, "Test Body"),
	}

	clusters, _ := clusterFailingTests(tests)
	if len(clusters) != 1 {
		t.Fatalf("clusters: got %d, want 1 (dynamic values must normalize away)", len(clusters))
	}
	if clusters[0].MemberCount != 2 {
		t.Errorf("member_count: got %d, want 2", clusters[0].MemberCount)
	}
	// shared_error is the representative's raw message, not the normalized key.
	if clusters[0].SharedError != tests[0].ErrorMessage {
		t.Errorf("shared_error: got %q, want the representative's raw message", clusters[0].SharedError)
	}
}

// TestClusterFailingTests_KeyComponents pins each part of the cluster key: the
// normalized message, the failure phase, and the deepest failed step.
func TestClusterFailingTests_KeyComponents(t *testing.T) {
	tests := []struct {
		name         string
		in           []DiagnoseTest
		wantClusters int
	}{
		{
			name: "different message splits",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseTestBody, "Test Body", "click"),
				clusterFixture("b", "bang", triage.PhaseTestBody, "Test Body", "click"),
			},
			wantClusters: 2,
		},
		{
			name: "different phase splits",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseTestBody, "Test Body", "click"),
				clusterFixture("b", "boom", triage.PhaseBeforeHooks, "Before Hooks", "click"),
			},
			wantClusters: 2,
		},
		{
			name: "different deepest step splits",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseTestBody, "Test Body", "click save"),
				clusterFixture("b", "boom", triage.PhaseTestBody, "Test Body", "click cancel"),
			},
			wantClusters: 2,
		},
		{
			name: "only the deepest step matters",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseTestBody, "Test Body", "open form", "click save"),
				clusterFixture("b", "boom", triage.PhaseTestBody, "Test Body", "click save"),
			},
			wantClusters: 1,
		},
		{
			name: "empty step path is empty-safe and groups",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseTestBody),
				clusterFixture("b", "boom", triage.PhaseTestBody),
			},
			wantClusters: 1,
		},
		{
			name: "empty messages group under the no-message key",
			in: []DiagnoseTest{
				clusterFixture("a", "", triage.PhaseTestBody),
				clusterFixture("b", "   ", triage.PhaseTestBody),
			},
			wantClusters: 1,
		},
		{
			name: "an empty message does not join a real one",
			in: []DiagnoseTest{
				clusterFixture("a", "", triage.PhaseTestBody),
				clusterFixture("b", "boom", triage.PhaseTestBody),
			},
			wantClusters: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusters, _ := clusterFailingTests(tc.in)
			if len(clusters) != tc.wantClusters {
				t.Errorf("clusters: got %d, want %d (%+v)", len(clusters), tc.wantClusters, clusters)
			}
		})
	}
}

// TestClusterFailingTests_EmptyMessageSharedError verifies the placeholder used
// for the key never leaks into shared_error.
func TestClusterFailingTests_EmptyMessageSharedError(t *testing.T) {
	clusters, _ := clusterFailingTests([]DiagnoseTest{clusterFixture("a", "", triage.PhaseTestBody)})
	if len(clusters) != 1 {
		t.Fatalf("clusters: got %d, want 1", len(clusters))
	}
	if clusters[0].SharedError != "" {
		t.Errorf("shared_error: got %q, want empty when the test had no error message", clusters[0].SharedError)
	}
}

// TestClusterFailingTests_SortedByMemberCount verifies clusters are ordered
// largest-first and that the ids are assigned after the sort, so c1 is always
// the dominant cluster. Ties keep first-appearance order.
func TestClusterFailingTests_SortedByMemberCount(t *testing.T) {
	tests := []DiagnoseTest{
		clusterFixture("lonely", "singleton", triage.PhaseTestBody),
		clusterFixture("pair-a", "pair", triage.PhaseTestBody),
		clusterFixture("trio-a", "trio", triage.PhaseTestBody),
		clusterFixture("pair-b", "pair", triage.PhaseTestBody),
		clusterFixture("trio-b", "trio", triage.PhaseTestBody),
		clusterFixture("trio-c", "trio", triage.PhaseTestBody),
	}

	clusters, members := clusterFailingTests(tests)
	if len(clusters) != 3 {
		t.Fatalf("clusters: got %d, want 3", len(clusters))
	}
	wantIDs := []string{"c1", "c2", "c3"}
	wantReps := []string{"trio-a", "pair-a", "lonely"}
	wantCounts := []int{3, 2, 1}
	for i := range clusters {
		if clusters[i].ClusterID != wantIDs[i] {
			t.Errorf("clusters[%d].cluster_id: got %q, want %q", i, clusters[i].ClusterID, wantIDs[i])
		}
		if clusters[i].RepresentativeFullName != wantReps[i] {
			t.Errorf("clusters[%d].representative: got %q, want %q", i, clusters[i].RepresentativeFullName, wantReps[i])
		}
		if clusters[i].MemberCount != wantCounts[i] {
			t.Errorf("clusters[%d].member_count: got %d, want %d", i, clusters[i].MemberCount, wantCounts[i])
		}
	}
	// members is aligned with the sorted clusters and indexes back into tests.
	if !reflect.DeepEqual(members, [][]int{{2, 4, 5}, {1, 3}, {0}}) {
		t.Errorf("member indices: got %v, want [[2 4 5] [1 3] [0]]", members)
	}
	if tests[0].ClusterID != "c3" {
		t.Errorf("singleton cluster_id: got %q, want c3", tests[0].ClusterID)
	}
}

// TestClusterFailingTests_SharedStatusPattern verifies the shared status
// pattern is reported only when every member agrees on both the code and the
// endpoint — an inconsistent pattern is not "shared".
func TestClusterFailingTests_SharedStatusPattern(t *testing.T) {
	tests := []struct {
		name string
		in   []DiagnoseTest
		want *ClusterStatusPattern
	}{
		{
			name: "consistent across members",
			in: []DiagnoseTest{
				withStatus(clusterFixture("a", "boom", triage.PhaseBeforeHooks), 500, "/api/TokenAuth/Authenticate"),
				withStatus(clusterFixture("b", "boom", triage.PhaseBeforeHooks), 500, "/api/TokenAuth/Authenticate"),
			},
			want: &ClusterStatusPattern{StatusCode: 500, Endpoint: "/api/TokenAuth/Authenticate"},
		},
		{
			name: "divergent endpoint is not shared",
			in: []DiagnoseTest{
				withStatus(clusterFixture("a", "boom", triage.PhaseBeforeHooks), 500, "/api/one"),
				withStatus(clusterFixture("b", "boom", triage.PhaseBeforeHooks), 500, "/api/two"),
			},
			want: nil,
		},
		{
			name: "divergent code is not shared",
			in: []DiagnoseTest{
				withStatus(clusterFixture("a", "boom", triage.PhaseBeforeHooks), 500, "/api/one"),
				withStatus(clusterFixture("b", "boom", triage.PhaseBeforeHooks), 503, "/api/one"),
			},
			want: nil,
		},
		{
			name: "a member without a pattern breaks the share",
			in: []DiagnoseTest{
				withStatus(clusterFixture("a", "boom", triage.PhaseBeforeHooks), 500, "/api/one"),
				clusterFixture("b", "boom", triage.PhaseBeforeHooks),
			},
			want: nil,
		},
		{
			name: "no member has a pattern",
			in: []DiagnoseTest{
				clusterFixture("a", "boom", triage.PhaseBeforeHooks),
				clusterFixture("b", "boom", triage.PhaseBeforeHooks),
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusters, _ := clusterFailingTests(tc.in)
			if len(clusters) != 1 {
				t.Fatalf("clusters: got %d, want 1", len(clusters))
			}
			got := clusters[0].SharedStatusPattern
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("shared_status_pattern: got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestClusterFailingTests_Empty verifies the no-failures path yields no
// clusters rather than a nil-deref or a phantom entry.
func TestClusterFailingTests_Empty(t *testing.T) {
	clusters, members := clusterFailingTests(nil)
	if len(clusters) != 0 {
		t.Errorf("clusters: got %v, want none", clusters)
	}
	if len(members) != 0 {
		t.Errorf("members: got %v, want none", members)
	}
}
