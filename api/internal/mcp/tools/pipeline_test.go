package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// buildPipelineStores wires the stores diagnose_pipeline needs.
func buildPipelineStores(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Project:    mocks.Projects,
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
		Pipeline:   mocks.Pipeline,
	}
}

// decodePipelineOutput JSON-round-trips StructuredContent into DiagnosePipelineOutput.
func decodePipelineOutput(t *testing.T, res *mcpsdk.CallToolResult) tools.DiagnosePipelineOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	var out tools.DiagnosePipelineOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal DiagnosePipelineOutput: %v", err)
	}
	return out
}

// seedPipelineProject creates a project via the in-memory project store and
// returns its ID.
func seedPipelineProject(t *testing.T, mocks *testutil.MockStores) int64 {
	t.Helper()
	proj, err := mocks.Projects.CreateProject(context.Background(), "shard-demo")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return proj.ID
}

// TestDiagnosePipeline_MultiShardClustering seeds two shard builds under one
// ci_pipeline_id: one shared failure (same error text) appears in both shards
// plus a build-local failure unique to the first shard. It pins the two
// behaviors diagnose_pipeline exists for: (1) both shard builds are found and
// diagnosed from a single project_id + pipeline_id call, and (2) the shared
// failure clusters into ONE cross-shard cluster spanning both builds rather
// than reading as two unrelated failures.
func TestDiagnosePipeline_MultiShardClustering(t *testing.T) {
	mocks := testutil.New()
	projectID := seedPipelineProject(t, mocks)

	branch := "main"
	pipelineURL := "https://ci.example.com/runs/456"
	pipelineID := "pipe-456"
	total1, passed1, failed1, broken1 := 5, 3, 2, 0
	total2, passed2, failed2, broken2 := 4, 3, 1, 0

	build1 := store.Build{
		ID: 10, ProjectID: projectID, BuildNumber: 1,
		CIBranch: &branch, CIPipelineID: &pipelineID, CIPipelineURL: &pipelineURL,
		StatTotal: &total1, StatPassed: &passed1, StatFailed: &failed1, StatBroken: &broken1,
	}
	build2 := store.Build{
		ID: 11, ProjectID: projectID, BuildNumber: 2,
		CIBranch: &branch, CIPipelineID: &pipelineID, CIPipelineURL: &pipelineURL,
		StatTotal: &total2, StatPassed: &passed2, StatFailed: &failed2, StatBroken: &broken2,
	}

	mocks.Pipeline.ListBuildsByPipelineIDFn = func(_ context.Context, gotProjectID int64, gotPipelineID string, _ int) ([]store.Build, error) {
		if gotProjectID != projectID || gotPipelineID != pipelineID {
			return nil, nil
		}
		return []store.Build{build1, build2}, nil
	}

	const sharedErr = "connect: connection refused to https://staging.internal/health"

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, buildID int64, _ int) ([]store.TestResult, error) {
		switch buildID {
		case 10:
			return []store.TestResult{
				{BuildID: 10, ProjectID: projectID, HistoryID: "h1:1", FullName: "suite.LoginTest", Status: "failed", StatusMessage: sharedErr},
				{BuildID: 10, ProjectID: projectID, HistoryID: "h2:1", FullName: "suite.CheckoutTest", Status: "failed", StatusMessage: "assertion failed: price mismatch"},
			}, nil
		case 11:
			return []store.TestResult{
				{BuildID: 11, ProjectID: projectID, HistoryID: "h3:1", FullName: "suite.SignupTest", Status: "failed", StatusMessage: sharedErr},
			}, nil
		}
		return nil, nil
	}

	cs := setupTestServer(t, buildPipelineStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_pipeline",
		Arguments: map[string]any{"project_id": projectID, "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodePipelineOutput(t, res)

	if out.PipelineID != pipelineID {
		t.Errorf("want pipeline_id=%q, got %q", pipelineID, out.PipelineID)
	}
	if out.PipelineURL != pipelineURL {
		t.Errorf("want pipeline_url=%q, got %q", pipelineURL, out.PipelineURL)
	}
	if len(out.Builds) != 2 {
		t.Fatalf("want 2 builds, got %d", len(out.Builds))
	}
	if out.Totals.Builds != 2 {
		t.Errorf("want totals.builds=2, got %d", out.Totals.Builds)
	}
	if out.Totals.FailedBuilds != 2 {
		t.Errorf("want totals.failed_builds=2, got %d", out.Totals.FailedBuilds)
	}
	if out.Totals.TotalTests != 9 {
		t.Errorf("want totals.total_tests=9, got %d", out.Totals.TotalTests)
	}
	if out.Totals.FailedTests != 3 {
		t.Errorf("want totals.failed_tests=3, got %d", out.Totals.FailedTests)
	}

	// The shared error must collapse into ONE cluster spanning both shards,
	// not two separate per-build clusters.
	if len(out.Clusters) != 2 {
		t.Fatalf("want 2 clusters (1 shared + 1 unique), got %d: %+v", len(out.Clusters), out.Clusters)
	}
	dominant := out.Clusters[0]
	if dominant.MemberCount != 2 {
		t.Fatalf("want dominant cluster member_count=2, got %d", dominant.MemberCount)
	}
	wantMembers := map[string]bool{"suite.LoginTest": true, "suite.SignupTest": true}
	for _, name := range dominant.MemberFullNames {
		if !wantMembers[name] {
			t.Errorf("unexpected member %q in dominant cluster", name)
		}
		delete(wantMembers, name)
	}
	if len(wantMembers) != 0 {
		t.Errorf("dominant cluster missing members: %v", wantMembers)
	}

	// Both shard builds' failing tests must carry the SAME cluster_id for the
	// shared failure — that is what lets a caller see "one outage, two shards"
	// instead of two unrelated failures.
	var build1ClusterID, build2ClusterID string
	for _, b := range out.Builds {
		for _, ft := range b.FailingTests {
			switch ft.FullName {
			case "suite.LoginTest":
				build1ClusterID = ft.ClusterID
			case "suite.SignupTest":
				build2ClusterID = ft.ClusterID
			}
		}
	}
	if build1ClusterID == "" || build1ClusterID != build2ClusterID {
		t.Errorf("want LoginTest and SignupTest to share a cluster_id, got %q vs %q", build1ClusterID, build2ClusterID)
	}
	if build1ClusterID != dominant.ClusterID {
		t.Errorf("want shared cluster_id=%q (dominant), got %q", dominant.ClusterID, build1ClusterID)
	}
}

// TestDiagnosePipeline_SingleBuild covers the degenerate case of a pipeline_id
// that only has one shard build — diagnose_pipeline should still work and
// report one build with one (single-member) cluster.
func TestDiagnosePipeline_SingleBuild(t *testing.T) {
	mocks := testutil.New()
	projectID := seedPipelineProject(t, mocks)

	branch := "main"
	pipelineID := "pipe-solo"
	total, passed, failed, broken := 3, 2, 1, 0

	build := store.Build{
		ID: 20, ProjectID: projectID, BuildNumber: 5,
		CIBranch: &branch, CIPipelineID: &pipelineID,
		StatTotal: &total, StatPassed: &passed, StatFailed: &failed, StatBroken: &broken,
	}

	mocks.Pipeline.ListBuildsByPipelineIDFn = func(_ context.Context, _ int64, _ string, _ int) ([]store.Build, error) {
		return []store.Build{build}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, buildID int64, _ int) ([]store.TestResult, error) {
		if buildID != 20 {
			return nil, nil
		}
		return []store.TestResult{
			{BuildID: 20, ProjectID: projectID, HistoryID: "h1:1", FullName: "suite.OnlyTest", Status: "failed", StatusMessage: "timeout waiting for selector"},
		}, nil
	}

	cs := setupTestServer(t, buildPipelineStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_pipeline",
		Arguments: map[string]any{"project_id": projectID, "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodePipelineOutput(t, res)
	if len(out.Builds) != 1 {
		t.Fatalf("want 1 build, got %d", len(out.Builds))
	}
	if out.Builds[0].BuildID != 20 || out.Builds[0].BuildNumber != 5 {
		t.Errorf("want build_id=20 build_number=5, got build_id=%d build_number=%d", out.Builds[0].BuildID, out.Builds[0].BuildNumber)
	}
	if out.Builds[0].Stats.Total != 3 || out.Builds[0].Stats.Passed != 2 || out.Builds[0].Stats.Failed != 1 {
		t.Errorf("unexpected stats: %+v", out.Builds[0].Stats)
	}
	if len(out.Builds[0].FailingTests) != 1 || out.Builds[0].FailingTests[0].FullName != "suite.OnlyTest" {
		t.Fatalf("want 1 failing test suite.OnlyTest, got %+v", out.Builds[0].FailingTests)
	}
	if out.Totals.Builds != 1 || out.Totals.FailedBuilds != 1 || out.Totals.FailedTests != 1 {
		t.Errorf("unexpected totals: %+v", out.Totals)
	}
	if len(out.Clusters) != 1 || out.Clusters[0].MemberCount != 1 {
		t.Fatalf("want 1 cluster with 1 member, got %+v", out.Clusters)
	}
}

// TestDiagnosePipeline_UnknownPipelineID verifies that a pipeline_id with no
// matching builds returns a clear error carrying a hint to use
// list_recent_builds (which now surfaces ci_pipeline_id) to find a valid value.
func TestDiagnosePipeline_UnknownPipelineID(t *testing.T) {
	mocks := testutil.New()
	projectID := seedPipelineProject(t, mocks)

	mocks.Pipeline.ListBuildsByPipelineIDFn = func(_ context.Context, _ int64, _ string, _ int) ([]store.Build, error) {
		return nil, nil
	}

	cs := setupTestServer(t, buildPipelineStores(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_pipeline",
		Arguments: map[string]any{"project_id": projectID, "pipeline_id": "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for unknown pipeline_id")
	}

	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok && contains(tc.Text, "list_recent_builds") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("want hint about list_recent_builds in error message, got: %v", res.Content)
	}
}

// TestDiagnosePipeline_InvalidInput verifies that a non-positive project_id or
// an empty pipeline_id is rejected before any store call.
func TestDiagnosePipeline_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "zero project_id", args: map[string]any{"project_id": 0, "pipeline_id": "p1"}},
		{name: "empty pipeline_id", args: map[string]any{"project_id": 1, "pipeline_id": ""}},
	}

	mocks := testutil.New()
	cs := setupTestServer(t, buildPipelineStores(mocks))
	ctx := context.Background()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      "diagnose_pipeline",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("want IsError=true for args %v", tc.args)
			}
		})
	}
}
