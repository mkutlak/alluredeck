package tools_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestDiagnosePipeline seeds two shard builds under one ci_pipeline_id: one
// failure shared by both shards plus one local to the first. It pins what the
// tool exists for: both shards are found from a single (project_id,
// pipeline_id) call, and the shared failure is ONE cross-shard cluster, not
// two unrelated per-build failures.
func TestDiagnosePipeline(t *testing.T) {
	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), "shard-demo")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	stats := func(total, passed, failed int) (*int, *int, *int, *int) { return &total, &passed, &failed, new(0) }
	shard := func(id int64, number, total, passed, failed int) store.Build {
		b := store.Build{ID: id, ProjectID: proj.ID, BuildNumber: number, CIBranch: new("main"),
			CIPipelineID: new("pipe-456"), CIPipelineURL: new("https://ci.example.com/runs/456")}
		b.StatTotal, b.StatPassed, b.StatFailed, b.StatBroken = stats(total, passed, failed)
		return b
	}
	mocks.Pipeline.ListBuildsByPipelineIDFn = func(_ context.Context, projectID int64, pipelineID string, _ int) ([]store.Build, error) {
		if projectID != proj.ID || pipelineID != "pipe-456" {
			return nil, nil
		}
		return []store.Build{shard(10, 1, 5, 3, 2), shard(11, 2, 4, 3, 1)}, nil
	}
	const sharedErr = "connect: connection refused to https://staging.internal/health"
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, buildID int64, _ int) ([]store.TestResult, error) {
		switch buildID {
		case 10:
			return []store.TestResult{
				{BuildID: 10, HistoryID: "h1:1", FullName: "suite.LoginTest", Status: "failed", StatusMessage: sharedErr},
				{BuildID: 10, HistoryID: "h2:1", FullName: "suite.CheckoutTest", Status: "failed", StatusMessage: "assertion failed: price mismatch"},
			}, nil
		case 11:
			return []store.TestResult{{BuildID: 11, HistoryID: "h3:1", FullName: "suite.SignupTest", Status: "failed", StatusMessage: sharedErr}}, nil
		}
		return nil, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{Project: mocks.Projects, Build: mocks.Builds, TestResult: mocks.TestResults, Pipeline: mocks.Pipeline})

	out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline", map[string]any{"project_id": proj.ID, "pipeline_id": "pipe-456"})
	if out.PipelineID != "pipe-456" || out.PipelineURL != "https://ci.example.com/runs/456" {
		t.Errorf("pipeline = (%q, %q), want (pipe-456, https://ci.example.com/runs/456)", out.PipelineID, out.PipelineURL)
	}
	if want := (tools.PipelineTotals{Builds: 2, FailedBuilds: 2, TotalTests: 9, FailedTests: 3}); out.Totals != want {
		t.Errorf("totals = %+v, want %+v", out.Totals, want)
	}
	if len(out.Builds) != 2 {
		t.Fatalf("builds = %d, want 2", len(out.Builds))
	}
	if b := out.Builds[0]; b.BuildID != 10 || b.BuildNumber != 1 || b.Stats != (tools.PipelineBuildStats{Total: 5, Passed: 3, Failed: 2}) {
		t.Errorf("first shard = (id %d, #%d, %+v), want (10, #1, {5 3 2 0})", b.BuildID, b.BuildNumber, b.Stats)
	}

	if len(out.Clusters) != 2 || out.Clusters[0].MemberCount != 2 {
		t.Fatalf("clusters = %+v, want a 2-member shared cluster plus a local one", out.Clusters)
	}
	dominant := out.Clusters[0]
	if names := slices.Sorted(slices.Values(dominant.MemberFullNames)); !slices.Equal(names, []string{"suite.LoginTest", "suite.SignupTest"}) {
		t.Errorf("shared cluster members = %v, want LoginTest and SignupTest", names)
	}
	// Both shards' copies of the shared failure carry the dominant cluster's id.
	for _, b := range out.Builds {
		for _, ft := range b.FailingTests {
			if (ft.FullName == "suite.LoginTest" || ft.FullName == "suite.SignupTest") && ft.ClusterID != dominant.ClusterID {
				t.Errorf("%s cluster_id = %q, want the shared %q", ft.FullName, ft.ClusterID, dominant.ClusterID)
			}
		}
	}

	if msg := callErr(t, cs, "diagnose_pipeline", map[string]any{"project_id": proj.ID, "pipeline_id": "does-not-exist"}); !strings.Contains(msg, "list_recent_builds") {
		t.Errorf("unknown pipeline_id error %q lacks the list_recent_builds hint", msg)
	}
	for _, args := range []map[string]any{{"project_id": 0, "pipeline_id": "p1"}, {"project_id": 1, "pipeline_id": ""}} {
		callErr(t, cs, "diagnose_pipeline", args)
	}
}
