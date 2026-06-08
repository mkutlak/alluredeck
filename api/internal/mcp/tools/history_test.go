package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func buildStoresHistory(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
		Attachment: mocks.Attachments,
		Defect:     mocks.Defects,
		KnownIssue: mocks.KnownIssues,
		Branch:     mocks.Branches,
	}
}

func decodeGetTestFailure(t *testing.T, res *mcpsdk.CallToolResult) tools.GetTestFailureOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetTestFailureOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetTestFailureOutput: %v", err)
	}
	return out
}

func decodeGetTestHistory(t *testing.T, res *mcpsdk.CallToolResult) tools.GetTestHistoryOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetTestHistoryOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetTestHistoryOutput: %v", err)
	}
	return out
}

func decodeCompareBuilds(t *testing.T, res *mcpsdk.CallToolResult) tools.CompareBuildsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.CompareBuildsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal CompareBuildsOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// get_test_failure
// ---------------------------------------------------------------------------

func TestGetTestFailure_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 1234},
		}, nil
	}
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{
			{ID: 99, Name: "screenshot.png", MimeType: "image/png", SizeBytes: 4096},
		}, 1, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.Status != "failed" {
		t.Errorf("want status=failed, got %q", out.Status)
	}
	if out.DurationMs != 1234 {
		t.Errorf("want duration_ms=1234, got %d", out.DurationMs)
	}
	if len(out.Attachments) == 0 {
		t.Error("want at least one attachment")
	}
	if out.Attachments[0].ResourceURI != "alluredeck://attachment/99" {
		t.Errorf("want resource_uri=alluredeck://attachment/99, got %q", out.Attachments[0].ResourceURI)
	}
}

func TestGetTestFailure_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing history_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty history_id")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestGetTestFailure_NotFound(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, HistoryID: "other-id", Status: "failed"},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for missing history_id")
	}
}

// TestGetTestFailure_BuildIDNotInProject verifies that a build_id not belonging
// to the project returns IsError=true with the hint message.
// GetBuildByID is used (not BuildExists) so we inject via GetBuildByIDFn.
func TestGetTestFailure_BuildIDNotInProject(t *testing.T) {
	mocks := testutil.New()
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, _ int64) (store.Build, error) {
		return store.Build{}, store.ErrBuildNotFound
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 28, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for build_id not in project")
	}
	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			if contains(tc.Text, "resolve_url") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("want hint about resolve_url in error message")
	}
}

// TestGetTestFailure_CIMetadataForNonLatestBuild verifies that CI metadata is
// populated for any build (not just the latest). Previously the code called
// GetLatestBuild and gated CI on build.ID == in.BuildID, silently dropping CI
// for non-latest builds.
func TestGetTestFailure_CIMetadataForNonLatestBuild(t *testing.T) {
	branch := "main"
	sha := "abc123"
	pipeline := "https://ci.example.com/jobs/42"

	mocks := testutil.New()
	// Return a non-latest build with CI fields set.
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		return store.Build{
			ID:            buildID,
			ProjectID:     1,
			BuildNumber:   5,
			CIBranch:      &branch,
			CICommitSHA:   &sha,
			CIPipelineURL: &pipeline,
		}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 50, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 100},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 50, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.CI == nil {
		t.Fatal("want non-nil CI metadata for non-latest build")
	}
	if out.CI.Branch != branch {
		t.Errorf("want branch=%q, got %q", branch, out.CI.Branch)
	}
	if out.CI.CommitSHA != sha {
		t.Errorf("want commit_sha=%q, got %q", sha, out.CI.CommitSHA)
	}
	if out.CI.PipelineURL != pipeline {
		t.Errorf("want pipeline_url=%q, got %q", pipeline, out.CI.PipelineURL)
	}
}

// TestGetTestFailure_Fingerprint verifies that the defect fingerprint is
// resolved through test_results.defect_fingerprint_id (the FK), not by passing
// history_id to GetByHash. The fingerprint and its linked known issue must be
// populated in the output.
func TestGetTestFailure_Fingerprint(t *testing.T) {
	const fpUUID = "11111111-1111-1111-1111-111111111111"

	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 50},
		}, nil
	}
	// The test row at (project=1, build=10, history=h1) links to fpUUID.
	mocks.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, projectID int64, buildID int64, historyID string) (*string, error) {
		if projectID == 1 && buildID == 10 && historyID == "h1" {
			id := fpUUID
			return &id, nil
		}
		return nil, store.ErrTestResultNotFound
	}
	// Seed a known issue so the defect's KnownIssueID resolves.
	ki, err := mocks.KnownIssues.Create(context.Background(), 1, "flaky-login", ".*", "", "")
	if err != nil {
		t.Fatalf("seed known issue: %v", err)
	}
	// Seed the defect so GetByID(fpUUID) resolves.
	mocks.Defects.Seed(store.DefectFingerprint{
		ID:              fpUUID,
		ProjectID:       1,
		FingerprintHash: "deadbeefhash",
		Category:        store.DefectCategoryProductBug,
		KnownIssueID:    &ki.ID,
	})

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.Fingerprint == nil {
		t.Fatal("want non-nil Fingerprint resolved via defect_fingerprint_id")
	}
	if out.Fingerprint.Hash != "deadbeefhash" {
		t.Errorf("want fingerprint hash=deadbeefhash, got %q", out.Fingerprint.Hash)
	}
	if out.Fingerprint.Category != store.DefectCategoryProductBug {
		t.Errorf("want category=%q, got %q", store.DefectCategoryProductBug, out.Fingerprint.Category)
	}
	if out.KnownIssue == nil {
		t.Fatal("want non-nil KnownIssue linked from the defect")
	}
	if out.KnownIssue.ID != ki.ID {
		t.Errorf("want known_issue id=%d, got %d", ki.ID, out.KnownIssue.ID)
	}
	if out.KnownIssue.Name != "flaky-login" {
		t.Errorf("want known_issue name=flaky-login, got %q", out.KnownIssue.Name)
	}
}

// TestGetTestFailure_NoFingerprint verifies that a test row with a NULL
// defect_fingerprint_id leaves out.Fingerprint nil without erroring.
func TestGetTestFailure_NoFingerprint(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 50},
		}, nil
	}
	// Row exists but has no linked fingerprint (nil pointer).
	mocks.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, _ int64, _ int64, _ string) (*string, error) {
		return nil, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.Fingerprint != nil {
		t.Errorf("want nil Fingerprint for unlinked test, got %+v", out.Fingerprint)
	}
}

// ---------------------------------------------------------------------------
// get_test_history
// ---------------------------------------------------------------------------

func TestGetTestHistory_HappyPath(t *testing.T) {
	sha := "deadbeef"
	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildNumber: 5, BuildID: 50, Status: "passed", DurationMs: 500, CreatedAt: time.Now(), CICommitSHA: &sha},
			{BuildNumber: 4, BuildID: 40, Status: "failed", DurationMs: 800, CreatedAt: time.Now()},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].Status != "passed" {
		t.Errorf("want status=passed, got %q", out.Items[0].Status)
	}
	if out.Items[0].CommitSHA != "deadbeef" {
		t.Errorf("want commit_sha=deadbeef, got %q", out.Items[0].CommitSHA)
	}
}

func TestGetTestHistory_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing history_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty history_id")
	}
}

func TestGetTestHistory_Pagination(t *testing.T) {
	entries := make([]store.TestHistoryEntry, 5)
	for i := range entries {
		entries[i] = store.TestHistoryEntry{BuildNumber: i + 1, BuildID: int64(i + 1), Status: "passed", CreatedAt: time.Now()}
	}

	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, limit int) ([]store.TestHistoryEntry, error) {
		if limit > len(entries) {
			return entries, nil
		}
		return entries[:limit], nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// limit=2 → handler requests 3, gets 3, returns 2 with cursor.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.NextCursor == "" {
		t.Error("want non-empty next_cursor")
	}
}

// ---------------------------------------------------------------------------
// compare_builds
// ---------------------------------------------------------------------------

func TestCompareBuilds_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
			{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
			{TestName: "T3", FullName: "pkg.T3", HistoryID: "h3", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 1, "target_build_id": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if len(out.Regressed) != 1 {
		t.Errorf("want 1 regressed, got %d", len(out.Regressed))
	}
	if len(out.Fixed) != 1 {
		t.Errorf("want 1 fixed, got %d", len(out.Fixed))
	}
	if len(out.NewFailed) != 1 {
		t.Errorf("want 1 new_failed, got %d", len(out.NewFailed))
	}
	if out.Regressed[0].HistoryID != "h1" {
		t.Errorf("want regressed history_id=h1, got %q", out.Regressed[0].HistoryID)
	}
}

func TestCompareBuilds_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	// Missing base_build_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "target_build_id": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for missing base_build_id")
	}
}

// TestCompareBuilds_BaseNotInProject verifies that base_build_id not in project
// returns IsError=true with hint. Base path now uses GetBuildByID (not BuildExists).
func TestCompareBuilds_BaseNotInProject(t *testing.T) {
	mocks := testutil.New()
	// base build (28) returns not-found; target never reached.
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		if buildID == 28 {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{ID: buildID}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 28, "target_build_id": 164},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for base_build_id not in project")
	}
	found := false
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			if contains(tc.Text, "resolve_url") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("want hint about resolve_url in error message")
	}
}

// TestCompareBuilds_FormatSummary verifies that format=summary returns counts only.
func TestCompareBuilds_FormatSummary(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
			{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
			{TestName: "T3", FullName: "pkg.T3", HistoryID: "h3", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
			{TestName: "T4", FullName: "pkg.T4", HistoryID: "h4", StatusA: "", StatusB: "passed", Category: store.DiffAdded},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 1, "target_build_id": 2, "format": "summary"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if out.Summary == nil {
		t.Fatal("want non-nil summary")
	}
	if out.Summary.Regressed != 1 {
		t.Errorf("want regressed=1, got %d", out.Summary.Regressed)
	}
	if out.Summary.Fixed != 1 {
		t.Errorf("want fixed=1, got %d", out.Summary.Fixed)
	}
	if out.Summary.NewPassed != 2 {
		t.Errorf("want new_passed=2, got %d", out.Summary.NewPassed)
	}
	// Per-test lists must be absent in summary mode.
	if len(out.Regressed) != 0 {
		t.Errorf("want empty regressed list in summary mode, got %d", len(out.Regressed))
	}
}

// TestCompareBuilds_BuildRefPopulated verifies that the Build field in the output
// is non-nil and carries the target build's build_number for all format modes.
func TestCompareBuilds_BuildRefPopulated(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}
	// base_build_id=1 → zero build (no branch); target_build_id=200 → build_number=42.
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		if buildID == 200 {
			return store.Build{ID: 200, BuildNumber: 42}, nil
		}
		// base build (id=1) returns a zero build — exists, no branch set.
		return store.Build{ID: buildID}, nil
	}

	for _, format := range []string{"full", "compact", "summary"} {
		t.Run("format="+format, func(t *testing.T) {
			cs := setupTestServer(t, buildStoresHistory(mocks))
			ctx := context.Background()

			res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "compare_builds",
				Arguments: map[string]any{
					"project_id":      1,
					"base_build_id":   1,
					"target_build_id": 200,
					"format":          format,
				},
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected error: %v", res.Content)
			}

			out := decodeCompareBuilds(t, res)
			if out.Build == nil {
				t.Fatal("want non-nil Build in output")
			}
			if out.Build.BuildNumber != 42 {
				t.Errorf("want build_number=42, got %d", out.Build.BuildNumber)
			}
		})
	}
}

// TestCompareBuilds_FormatCompact verifies that format=compact omits history_id and test_name.
func TestCompareBuilds_FormatCompact(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 1, "target_build_id": 2, "format": "compact"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if len(out.Regressed) != 1 {
		t.Fatalf("want 1 regressed, got %d", len(out.Regressed))
	}
	item := out.Regressed[0]
	if item.FullName != "pkg.T1" {
		t.Errorf("want full_name=pkg.T1, got %q", item.FullName)
	}
	// compact omits test_name and history_id.
	if item.TestName != "" {
		t.Errorf("want empty test_name in compact mode, got %q", item.TestName)
	}
	if item.HistoryID != "" {
		t.Errorf("want empty history_id in compact mode, got %q", item.HistoryID)
	}
}

// TestGetTestFailure_EnvironmentPropagated verifies that when the build row has
// an Environment map, it is propagated verbatim to get_test_failure output.
func TestGetTestFailure_EnvironmentPropagated(t *testing.T) {
	mocks := testutil.New()
	env := map[string]string{
		"Grafana.Drilldown.URL": "https://example/x",
		"Loki.Query":            `{k8s_namespace_name="ns-x"}`,
	}
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, _ int64) (store.Build, error) {
		return store.Build{
			ID:          10,
			ProjectID:   1,
			BuildNumber: 3,
			Environment: env,
		}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 100},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if len(out.Environment) != 2 {
		t.Fatalf("want 2 environment entries, got %d: %v", len(out.Environment), out.Environment)
	}
	if out.Environment["Grafana.Drilldown.URL"] != "https://example/x" {
		t.Errorf("Grafana.Drilldown.URL: got %q", out.Environment["Grafana.Drilldown.URL"])
	}
	if out.Environment["Loki.Query"] != `{k8s_namespace_name="ns-x"}` {
		t.Errorf("Loki.Query: got %q", out.Environment["Loki.Query"])
	}
}

// ---------------------------------------------------------------------------
// compare_builds — branch mismatch warning (Change 3)
// ---------------------------------------------------------------------------

// TestCompareBuilds_SameBranch verifies that no branch_mismatch is set when
// both builds share the same branch_id.
func TestCompareBuilds_SameBranch(t *testing.T) {
	branchID := int64(7)
	mocks := testutil.New()
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		return store.Build{ID: buildID, BranchID: &branchID}, nil
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if out.BranchMismatch != nil {
		t.Errorf("want nil BranchMismatch for same-branch builds, got %+v", out.BranchMismatch)
	}
	// Diff is still present.
	if len(out.Regressed) != 1 {
		t.Errorf("want 1 regressed, got %d", len(out.Regressed))
	}
}

// TestCompareBuilds_DifferentBranch verifies that branch_mismatch is populated
// when the two builds have different branch_ids, and that the diff is still returned.
func TestCompareBuilds_DifferentBranch(t *testing.T) {
	branchA := int64(1)
	branchB := int64(2)
	branchNameA := "main"
	branchNameB := "feature/x"

	mocks := testutil.New()
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		switch buildID {
		case 10:
			return store.Build{ID: 10, BranchID: &branchA, CIBranch: &branchNameA}, nil
		case 20:
			return store.Build{ID: 20, BranchID: &branchB, CIBranch: &branchNameB}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if out.BranchMismatch == nil {
		t.Fatal("want non-nil BranchMismatch for different-branch builds")
	}
	if out.BranchMismatch.BaseBranch != branchNameA {
		t.Errorf("want base_branch=%q, got %q", branchNameA, out.BranchMismatch.BaseBranch)
	}
	if out.BranchMismatch.TargetBranch != branchNameB {
		t.Errorf("want target_branch=%q, got %q", branchNameB, out.BranchMismatch.TargetBranch)
	}
	if out.BranchMismatch.Message == "" {
		t.Error("want non-empty Message in BranchMismatch")
	}
	// Diff is still present despite the mismatch.
	if len(out.Regressed) != 1 {
		t.Errorf("want 1 regressed even with branch mismatch, got %d", len(out.Regressed))
	}
}

// TestCompareBuilds_DifferentBranch_Summary verifies that branch_mismatch is also
// present in format=summary output.
func TestCompareBuilds_DifferentBranch_Summary(t *testing.T) {
	branchA := int64(1)
	branchB := int64(2)
	branchNameA := "main"
	branchNameB := "feature/x"

	mocks := testutil.New()
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		switch buildID {
		case 10:
			return store.Build{ID: 10, BranchID: &branchA, CIBranch: &branchNameA}, nil
		case 20:
			return store.Build{ID: 20, BranchID: &branchB, CIBranch: &branchNameB}, nil
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20, "format": "summary"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if out.BranchMismatch == nil {
		t.Fatal("want non-nil BranchMismatch in summary format for different-branch builds")
	}
	if out.BranchMismatch.BaseBranch != branchNameA {
		t.Errorf("want base_branch=%q, got %q", branchNameA, out.BranchMismatch.BaseBranch)
	}
	if out.Summary == nil {
		t.Fatal("want non-nil Summary in summary format")
	}
	if out.Summary.Regressed != 1 {
		t.Errorf("want regressed=1, got %d", out.Summary.Regressed)
	}
}

// ---------------------------------------------------------------------------
// get_test_history — optional branch param (Change 4)
// ---------------------------------------------------------------------------

// TestGetTestHistory_BranchResolved verifies that when branch is set, the resolved
// branch ID is passed to GetTestHistory (non-nil branchID).
func TestGetTestHistory_BranchResolved(t *testing.T) {
	const wantBranchID = int64(42)
	var receivedBranchID *int64

	mocks := testutil.New()
	mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, name string) (*store.Branch, error) {
		if name == "main" {
			return &store.Branch{ID: wantBranchID, Name: "main"}, nil
		}
		return nil, store.ErrBranchNotFound
	}
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
		receivedBranchID = branchID
		return []store.TestHistoryEntry{
			{BuildNumber: 5, BuildID: 50, Status: "passed", DurationMs: 100, CreatedAt: time.Now()},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "branch": "main"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if receivedBranchID == nil {
		t.Fatal("want non-nil branchID passed to GetTestHistory when branch is set")
	}
	if *receivedBranchID != wantBranchID {
		t.Errorf("want branchID=%d, got %d", wantBranchID, *receivedBranchID)
	}

	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 1 {
		t.Errorf("want 1 item, got %d", len(out.Items))
	}
}

// TestGetTestHistory_UnknownBranch verifies that an unknown branch name returns
// empty items and no error (branch not found is not a user mistake).
func TestGetTestHistory_UnknownBranch(t *testing.T) {
	mocks := testutil.New()
	mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, _ string) (*store.Branch, error) {
		return nil, store.ErrBranchNotFound
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "branch": "no-such-branch"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("want no error for unknown branch, got: %v", res.Content)
	}

	out := decodeGetTestHistory(t, res)
	if len(out.Items) != 0 {
		t.Errorf("want 0 items for unknown branch, got %d", len(out.Items))
	}
}

// TestGetTestHistory_NoBranch verifies that omitting branch passes nil branchID
// to GetTestHistory (unchanged cross-branch behavior).
func TestGetTestHistory_NoBranch(t *testing.T) {
	var receivedBranchID *int64

	mocks := testutil.New()
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
		receivedBranchID = branchID
		return []store.TestHistoryEntry{
			{BuildNumber: 3, BuildID: 30, Status: "failed", DurationMs: 200, CreatedAt: time.Now()},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if receivedBranchID != nil {
		t.Errorf("want nil branchID when branch is not set, got %d", *receivedBranchID)
	}
}

// TestGetTestHistory_BranchLookupError verifies that a non-sentinel error from
// branch resolution (e.g. a DB failure) surfaces as a tool error rather than
// being masked as an empty "no history on this branch" result.
func TestGetTestHistory_BranchLookupError(t *testing.T) {
	var historyCalled bool

	mocks := testutil.New()
	mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, _ string) (*store.Branch, error) {
		return nil, fmt.Errorf("get branch by name: %w", context.DeadlineExceeded)
	}
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		historyCalled = true
		return nil, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_history",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1", "branch": "main"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want tool error when branch resolution fails with a non-sentinel error")
	}
	if historyCalled {
		t.Error("GetTestHistory must not be called when branch resolution errors")
	}
}

// TestCompareBuilds_OneBranchUnknown verifies the conservative mismatch rule:
// when one build has a known branch and the other has none (NULL branch_id),
// no branch_mismatch is emitted and the diff is still returned.
func TestCompareBuilds_OneBranchUnknown(t *testing.T) {
	branchA := int64(1)
	branchNameA := "main"

	mocks := testutil.New()
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, buildID int64) (store.Build, error) {
		switch buildID {
		case 10:
			return store.Build{ID: 10, BranchID: &branchA, CIBranch: &branchNameA}, nil
		case 20:
			return store.Build{ID: 20}, nil // no branch_id (legacy build)
		}
		return store.Build{}, store.ErrBuildNotFound
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "compare_builds",
		Arguments: map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeCompareBuilds(t, res)
	if out.BranchMismatch != nil {
		t.Errorf("want nil BranchMismatch when one branch is unknown, got %+v", out.BranchMismatch)
	}
	if len(out.Regressed) != 1 {
		t.Errorf("want 1 regressed, got %d", len(out.Regressed))
	}
}

// TestGetTestFailure_EnvironmentAbsent verifies that when the build has no
// environment, the environment field is absent (nil) in the output.
func TestGetTestFailure_EnvironmentAbsent(t *testing.T) {
	mocks := testutil.New()
	// Default GetBuildByIDFn returns zero Build (no Environment).
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 10, ProjectID: 1, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", DurationMs: 100},
		}, nil
	}

	cs := setupTestServer(t, buildStoresHistory(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_test_failure",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetTestFailure(t, res)
	if out.Environment != nil {
		t.Errorf("want nil environment when absent, got %v", out.Environment)
	}
}
