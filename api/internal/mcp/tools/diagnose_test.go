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
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// buildStoresDiagnose wires the stores diagnose_failure needs.
func buildStoresDiagnose(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Project:    mocks.Projects,
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
		Attachment: mocks.Attachments,
		Defect:     mocks.Defects,
		KnownIssue: mocks.KnownIssues,
	}
}

func decodeDiagnoseFailure(t *testing.T, res *mcpsdk.CallToolResult) tools.DiagnoseFailureOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.DiagnoseFailureOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal DiagnoseFailureOutput: %v", err)
	}
	return out
}

// seedDiagnoseProjectBuild seeds a project and points GetBuildByID at a build
// with the given build_id, so the (project_id, build_id) resolution path works.
func seedDiagnoseProjectBuild(t *testing.T, mocks *testutil.MockStores, buildID int64, buildNumber int) (projectID int64) {
	t.Helper()
	proj, err := mocks.Projects.CreateProject(context.Background(), "demo")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	branch := "main"
	sha := "abc123"
	total, passed, failed, broken := 10, 7, 2, 1
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != buildID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{
			ID:          buildID,
			ProjectID:   proj.ID,
			BuildNumber: buildNumber,
			CIBranch:    &branch,
			CICommitSHA: &sha,
			StatTotal:   &total,
			StatPassed:  &passed,
			StatFailed:  &failed,
			StatBroken:  &broken,
		}, nil
	}
	return proj.ID
}

// TestDiagnoseFailure_HappyPath exercises the full one-call diagnosis: build
// resolution, per-test failure detail, failed-step path, build history feeding
// triage, and fingerprint/known-issue attachment.
func TestDiagnoseFailure_HappyPath(t *testing.T) {
	const fpUUID = "22222222-2222-2222-2222-222222222222"

	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 1200},
		}, nil
	}
	// Failed-step path: root→leaf, with the deepest step's error message.
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, historyID string) ([]string, string, error) {
		if historyID == "h1" {
			return []string{"Test Body", "Call API"}, "status 500 from /users", nil
		}
		return nil, "", nil
	}
	// Build history: most-recent-first, current build first then prior builds.
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildID: 100, BuildNumber: 28, Status: "failed", DurationMs: 1200},
			{BuildID: 90, BuildNumber: 27, Status: "failed", DurationMs: 1100},
			{BuildID: 80, BuildNumber: 26, Status: "passed", DurationMs: 5000},
		}, nil
	}
	// Attachments are scoped per test result via ListByTestResult.
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, _ int64, _ int64, historyID string, _ int) ([]store.TestAttachment, error) {
		if historyID == "h1" {
			return []store.TestAttachment{
				{ID: 7, Name: "trace.zip", MimeType: "application/zip", SizeBytes: 2048},
			}, nil
		}
		return nil, nil
	}
	// Fingerprint + known issue.
	mocks.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, _ int64, _ int64, _ string) (*string, error) {
		id := fpUUID
		return &id, nil
	}
	ki, err := mocks.KnownIssues.Create(context.Background(), projectID, "flaky-api", ".*", "", "")
	if err != nil {
		t.Fatalf("seed known issue: %v", err)
	}
	mocks.Defects.Seed(store.DefectFingerprint{
		ID:              fpUUID,
		ProjectID:       projectID,
		FingerprintHash: "cafebabehash",
		Category:        store.DefectCategoryInfrastructure,
		KnownIssueID:    &ki.ID,
	})

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)

	// Build-level summary.
	if out.Build.BuildNumber != 28 {
		t.Errorf("build_number: got %d, want 28", out.Build.BuildNumber)
	}
	if out.Build.Branch != "main" {
		t.Errorf("branch: got %q, want main", out.Build.Branch)
	}
	if out.Build.FailedTests != 2 || out.Build.BrokenTests != 1 {
		t.Errorf("failed/broken counts: got %d/%d, want 2/1", out.Build.FailedTests, out.Build.BrokenTests)
	}

	// Per-test diagnosis.
	if out.ExaminedTests != 1 || len(out.FailingTests) != 1 {
		t.Fatalf("examined tests: got %d (%d items), want 1", out.ExaminedTests, len(out.FailingTests))
	}
	d := out.FailingTests[0]
	if d.FullName != "pkg.LoginTest" {
		t.Errorf("full_name: got %q", d.FullName)
	}
	if d.ErrorMessage != "status 500 from /users" {
		t.Errorf("error_message: got %q", d.ErrorMessage)
	}
	if len(d.FailedStepPath) != 2 || d.FailedStepPath[1] != "Call API" {
		t.Errorf("failed_step_path: got %v", d.FailedStepPath)
	}
	if len(d.Attachments) != 1 || d.Attachments[0].ResourceURI != "alluredeck://attachment/7" {
		t.Errorf("attachments: got %+v", d.Attachments)
	}
	if d.Fingerprint == nil || d.Fingerprint.Hash != "cafebabehash" {
		t.Errorf("fingerprint: got %+v", d.Fingerprint)
	}
	if d.KnownIssue == nil || d.KnownIssue.Name != "flaky-api" {
		t.Errorf("known_issue: got %+v", d.KnownIssue)
	}

	// Triage signals: prior history is [failed(27), passed(26)] after the
	// current build is dropped, so last_status=failed and builds_since_pass=1.
	if d.Signals.LastStatus != triage.StatusFailed {
		t.Errorf("signals.last_status: got %q, want failed", d.Signals.LastStatus)
	}
	if d.Signals.BuildsSincePass != 1 {
		t.Errorf("signals.builds_since_pass: got %d, want 1", d.Signals.BuildsSincePass)
	}
	// Error message "status 500 ..." yields a status pattern.
	if d.Signals.RepeatedStatusPattern == nil || d.Signals.RepeatedStatusPattern.StatusCode != 500 {
		t.Errorf("signals.repeated_status_pattern: got %+v", d.Signals.RepeatedStatusPattern)
	}
	// "Call API" path has no hook/fixture marker → test_body.
	if d.Signals.FailurePhase != triage.PhaseTestBody {
		t.Errorf("signals.failure_phase: got %q, want test_body", d.Signals.FailurePhase)
	}
	// Category propagated from the defect record.
	if d.Signals.CategoryHint.Value != store.DefectCategoryInfrastructure {
		t.Errorf("signals.category_hint: got %q", d.Signals.CategoryHint.Value)
	}
}

// TestDiagnoseFailure_PerTestAttachments verifies that each failing test
// carries ONLY its own attachments — resolved via ListByTestResult scoped by
// history_id — never the build-wide attachment set.
func TestDiagnoseFailure_PerTestAttachments(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 10},
			{BuildID: 100, ProjectID: projectID, HistoryID: "h2", FullName: "pkg.LogoutTest", Status: "failed", DurationMs: 20},
		}, nil
	}
	// Per-test attachments: h1 owns one attachment, h2 owns two.
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, _ int64, _ int64, historyID string, _ int) ([]store.TestAttachment, error) {
		switch historyID {
		case "h1":
			return []store.TestAttachment{{ID: 1, Name: "h1.png"}}, nil
		case "h2":
			return []store.TestAttachment{{ID: 2, Name: "h2-a.png"}, {ID: 3, Name: "h2-b.png"}}, nil
		default:
			return nil, nil
		}
	}
	// ListByBuild must NOT be used for per-test scoping; if it is, the test
	// would see this build-wide noise leak into every test entry.
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{
			{ID: 90, Name: "build-wide-1"}, {ID: 91, Name: "build-wide-2"}, {ID: 92, Name: "build-wide-3"},
		}, 3, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 2 {
		t.Fatalf("want 2 failing tests, got %d", len(out.FailingTests))
	}

	for _, d := range out.FailingTests {
		switch d.HistoryID {
		case "h1":
			if len(d.Attachments) != 1 || d.Attachments[0].ID != 1 {
				t.Errorf("h1 attachments: got %+v, want exactly [id=1]", d.Attachments)
			}
		case "h2":
			if len(d.Attachments) != 2 || d.Attachments[0].ID != 2 || d.Attachments[1].ID != 3 {
				t.Errorf("h2 attachments: got %+v, want exactly [id=2,id=3]", d.Attachments)
			}
		default:
			t.Errorf("unexpected history_id %q", d.HistoryID)
		}
		// No build-wide attachment id (90/91/92) may appear on any test.
		for _, a := range d.Attachments {
			if a.ID == 90 || a.ID == 91 || a.ID == 92 {
				t.Errorf("build-wide attachment %d leaked into test %q", a.ID, d.HistoryID)
			}
		}
	}
}

// TestDiagnoseFailure_AttachmentFetchError verifies a per-test attachment
// fetch failure is non-fatal: the test is still diagnosed, just with no
// attachments.
func TestDiagnoseFailure_AttachmentFetchError(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 10},
		}, nil
	}
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, _ int64, _ int64, _ string, _ int) ([]store.TestAttachment, error) {
		return nil, context.DeadlineExceeded
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("attachment fetch error must not fail the tool: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	if len(out.FailingTests[0].Attachments) != 0 {
		t.Errorf("want no attachments on fetch error, got %+v", out.FailingTests[0].Attachments)
	}
}

// TestDiagnoseFailure_Truncation verifies that more failing tests than max_tests
// are truncated and reported.
func TestDiagnoseFailure_Truncation(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	// Return more rows than the cap; the handler asks for max_tests+1.
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, limit int) ([]store.TestResult, error) {
		rows := make([]store.TestResult, 0, limit)
		for i := range limit {
			rows = append(rows, store.TestResult{
				BuildID: 100, ProjectID: projectID, HistoryID: "h" + string(rune('a'+i)),
				FullName: "pkg.Test", Status: "failed", DurationMs: 10,
			})
		}
		return rows, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100, "max_tests": 3},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if out.ExaminedTests != 3 {
		t.Errorf("examined_tests: got %d, want 3", out.ExaminedTests)
	}
	if !out.Truncated {
		t.Error("want truncated=true")
	}
	if out.TruncatedCount != 1 {
		t.Errorf("truncated_count: got %d, want 1", out.TruncatedCount)
	}
}

// TestDiagnoseFailure_SummaryOnly verifies that summary_only omits heavy
// per-test fields (failed_step_path, attachments) but keeps error_message and
// signals.
func TestDiagnoseFailure_SummaryOnly(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 100},
		}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return []string{"Test Body"}, "boom", nil
	}
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{{ID: 1, Name: "x.png"}}, 1, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100, "summary_only": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	d := out.FailingTests[0]
	if len(d.FailedStepPath) != 0 {
		t.Errorf("summary_only must omit failed_step_path, got %v", d.FailedStepPath)
	}
	if len(d.Attachments) != 0 {
		t.Errorf("summary_only must omit attachments, got %+v", d.Attachments)
	}
	// error_message and signals are retained.
	if d.ErrorMessage != "boom" {
		t.Errorf("error_message must be retained, got %q", d.ErrorMessage)
	}
	if d.Signals.CategoryHint.Value == "" {
		t.Error("signals must be retained in summary_only mode")
	}
}

// TestDiagnoseFailure_InvalidInput verifies missing identifiers error cleanly.
func TestDiagnoseFailure_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true when no build identifier is provided")
	}
}

// TestDiagnoseFailure_BuildNotFound verifies a build_id not in the project
// errors with a resolution hint.
func TestDiagnoseFailure_BuildNotFound(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 999},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for unknown build_id")
	}
}

// TestDiagnoseFailure_EnvironmentPropagated verifies that when the build has an
// Environment map, it is propagated verbatim to the build summary output.
func TestDiagnoseFailure_EnvironmentPropagated(t *testing.T) {
	mocks := testutil.New()

	branch := "main"
	sha := "abc123"
	total, passed, failed, broken := 10, 7, 2, 1
	env := map[string]string{
		"Grafana.Drilldown.URL": "https://example/x",
		"Loki.Query":            `{k8s_namespace_name="ns-x"}`,
	}
	const buildID int64 = 200
	mocks.Projects.CreateProject(context.Background(), "env-proj") //nolint:errcheck
	proj, _ := mocks.Projects.GetProjectBySlug(context.Background(), "env-proj")
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != buildID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{
			ID:          buildID,
			ProjectID:   proj.ID,
			BuildNumber: 5,
			CIBranch:    &branch,
			CICommitSHA: &sha,
			StatTotal:   &total,
			StatPassed:  &passed,
			StatFailed:  &failed,
			StatBroken:  &broken,
			Environment: env,
		}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return nil, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": proj.ID, "build_id": buildID},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)

	if len(out.Build.Environment) != 2 {
		t.Fatalf("want 2 environment entries, got %d: %v", len(out.Build.Environment), out.Build.Environment)
	}
	if out.Build.Environment["Grafana.Drilldown.URL"] != "https://example/x" {
		t.Errorf("Grafana.Drilldown.URL: got %q", out.Build.Environment["Grafana.Drilldown.URL"])
	}
	if out.Build.Environment["Loki.Query"] != `{k8s_namespace_name="ns-x"}` {
		t.Errorf("Loki.Query: got %q", out.Build.Environment["Loki.Query"])
	}
}

// TestDiagnoseFailure_EnvironmentAbsent verifies that when the build has no
// environment, the environment field is omitted from the JSON output.
func TestDiagnoseFailure_EnvironmentAbsent(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 300, 1)
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return nil, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 300},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if out.Build.Environment != nil {
		t.Errorf("want nil environment when absent, got %v", out.Build.Environment)
	}
}

// TestDiagnoseFailure_BranchScopedTriage verifies that when the diagnosed build
// has a BranchID set, the triage signals (builds_since_pass, last_status) reflect
// only branch-scoped history. The test seeds history spanning two branches: the
// test passes on branch A but fails repeatedly on branch B. When the diagnosed
// build is on branch B, signals must reflect only branch B history.
func TestDiagnoseFailure_BranchScopedTriage(t *testing.T) {
	const buildID int64 = 500
	const branchBID int64 = 2

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), "branch-scope-proj")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	branchStr := "feature-b"
	sha := "def456"
	total, passed, failed, broken := 5, 3, 1, 1
	branchBIDVal := branchBID
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != buildID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{
			ID:          buildID,
			ProjectID:   proj.ID,
			BuildNumber: 10,
			CIBranch:    &branchStr,
			CICommitSHA: &sha,
			StatTotal:   &total,
			StatPassed:  &passed,
			StatFailed:  &failed,
			StatBroken:  &broken,
			BranchID:    &branchBIDVal,
		}, nil
	}

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: buildID, ProjectID: proj.ID, HistoryID: "hX", FullName: "pkg.BranchTest", Status: "failed", DurationMs: 500},
		}, nil
	}

	// GetTestHistory captures the branchID argument and returns branch-B-only
	// history: two consecutive failures (no pass), so builds_since_pass > 1.
	var capturedBranchID *int64
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
		capturedBranchID = branchID
		// Branch B history: current build (500) + two prior failures on branch B.
		return []store.TestHistoryEntry{
			{BuildID: 500, BuildNumber: 10, Status: "failed", DurationMs: 500},
			{BuildID: 490, BuildNumber: 9, Status: "failed", DurationMs: 480},
			{BuildID: 480, BuildNumber: 8, Status: "failed", DurationMs: 460},
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": proj.ID, "build_id": buildID},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	// Verify GetTestHistory was called with the build's branch ID.
	if capturedBranchID == nil {
		t.Fatal("GetTestHistory must be called with non-nil branchID when build.BranchID is set")
	}
	if *capturedBranchID != branchBID {
		t.Errorf("GetTestHistory branchID: got %d, want %d", *capturedBranchID, branchBID)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	sig := out.FailingTests[0].Signals

	// Prior branch-B history after dropping current build: [failed(9), failed(8)].
	// No pass in history → builds_since_pass = 2.
	if sig.BuildsSincePass != 2 {
		t.Errorf("builds_since_pass: got %d, want 2", sig.BuildsSincePass)
	}
	// last_status is the immediately preceding build: failed.
	if sig.LastStatus != triage.StatusFailed {
		t.Errorf("last_status: got %q, want %q", sig.LastStatus, triage.StatusFailed)
	}
}

// TestDiagnoseFailure_NilBranchCrossBranchFallback verifies that when the
// diagnosed build has no BranchID (nil), GetTestHistory is called with nil and
// cross-branch behavior is unchanged.
func TestDiagnoseFailure_NilBranchCrossBranchFallback(t *testing.T) {
	const buildID int64 = 600

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), "nil-branch-proj")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	sha := "ghi789"
	total, passed, failed, broken := 5, 3, 1, 1
	// BranchID deliberately left nil — simulates an older build with no branch_id.
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != buildID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{
			ID:          buildID,
			ProjectID:   proj.ID,
			BuildNumber: 20,
			CICommitSHA: &sha,
			StatTotal:   &total,
			StatPassed:  &passed,
			StatFailed:  &failed,
			StatBroken:  &broken,
			BranchID:    nil, // no branch
		}, nil
	}

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: buildID, ProjectID: proj.ID, HistoryID: "hY", FullName: "pkg.CrossTest", Status: "failed", DurationMs: 300},
		}, nil
	}

	var capturedBranchID *int64
	called := false
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
		called = true
		capturedBranchID = branchID
		return []store.TestHistoryEntry{
			{BuildID: 600, BuildNumber: 20, Status: "failed", DurationMs: 300},
			{BuildID: 590, BuildNumber: 19, Status: "passed", DurationMs: 250},
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": proj.ID, "build_id": buildID},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	// GetTestHistory must be called with nil branchID for cross-branch fallback.
	if !called {
		t.Fatal("GetTestHistory was not called")
	}
	if capturedBranchID != nil {
		t.Errorf("GetTestHistory branchID: got %d, want nil (cross-branch fallback)", *capturedBranchID)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	sig := out.FailingTests[0].Signals

	// Cross-branch prior history: [passed(19)] after dropping current build (600).
	// builds_since_pass = 0 (immediately preceded by a pass).
	if sig.BuildsSincePass != 0 {
		t.Errorf("builds_since_pass: got %d, want 0", sig.BuildsSincePass)
	}
	if sig.LastStatus != triage.StatusPassed {
		t.Errorf("last_status: got %q, want %q", sig.LastStatus, triage.StatusPassed)
	}
}
