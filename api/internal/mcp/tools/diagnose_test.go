package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
// are truncated and reported. truncated_count is measured against the DISTINCT
// failing-test count from the store, not the number of rows fetched, so it
// stays truthful when duplicate rows are collapsed.
func TestDiagnoseFailure_Truncation(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	// Four distinct failing tests exist; the caller asks for three.
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, limit int) ([]store.TestResult, error) {
		rows := make([]store.TestResult, 0, 4)
		for i := range 4 {
			if len(rows) == limit {
				break
			}
			rows = append(rows, store.TestResult{
				BuildID: 100, ProjectID: projectID, HistoryID: "h" + string(rune('a'+i)),
				FullName: "pkg.Test" + string(rune('a'+i)), Status: "failed", DurationMs: 10,
			})
		}
		return rows, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _ int64, _ int64) (int, error) {
		return 4, nil
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

// TestDiagnoseFailure_FetchesEnoughRowsToSurviveDedup pins the fetch budget.
// Playwright writes two rows per test, so asking for max_tests+1 rows would
// yield only (max_tests+1)/2 distinct tests after the collapse and silently
// under-report a build. The handler must ask for 2*max_tests+2.
func TestDiagnoseFailure_FetchesEnoughRowsToSurviveDedup(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	var gotLimit int
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, limit int) ([]store.TestResult, error) {
		gotLimit = limit
		// Three tests, each recorded twice under the two history_id schemes.
		rows := make([]store.TestResult, 0, 6)
		for _, name := range []string{"pkg.A", "pkg.B", "pkg.C"} {
			rows = append(rows,
				store.TestResult{BuildID: 100, ProjectID: projectID, HistoryID: name + ".dup",
					FullName: name, Status: "failed", DurationMs: 10},
				store.TestResult{BuildID: 100, ProjectID: projectID, HistoryID: name + ":real",
					FullName: name, Status: "failed", DurationMs: 10, StatusMessage: "boom"},
			)
		}
		if len(rows) > limit {
			rows = rows[:limit]
		}
		return rows, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _ int64, _ int64) (int, error) {
		return 3, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100, "max_tests": 3},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	if gotLimit != 8 {
		t.Errorf("store limit: got %d, want 8 (2*max_tests+2)", gotLimit)
	}

	out := decodeDiagnoseFailure(t, res)
	if out.ExaminedTests != 3 {
		t.Fatalf("examined_tests: got %d, want 3 (all twins collapsed, none lost)", out.ExaminedTests)
	}
	if out.Truncated {
		t.Errorf("truncated: got true, want false — every distinct test was examined")
	}
}

// TestDiagnoseFailure_MergesDuplicateRows is the regression guard for the live
// bug: 8 real failures were reported as examined_tests=16 because Playwright
// records each test twice — an enriched row (history_id "md5:md5") and an empty
// shell ("md5.md5"). The enriched row must survive, the shell's identifier must
// be reported under merged_history_ids, and a top-level warning must say how
// many rows were merged.
func TestDiagnoseFailure_MergesDuplicateRows(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			// Empty shell first, so the ordering cannot be what saves us.
			{BuildID: 100, ProjectID: projectID, HistoryID: "abc.abc",
				FullName: "spec/login.ts > login", Status: "failed", DurationMs: 10},
			{BuildID: 100, ProjectID: projectID, HistoryID: "abc:abc",
				FullName: "spec/login.ts > login", Status: "failed", DurationMs: 10,
				StatusMessage: "expected 200, got 500"},
			{BuildID: 100, ProjectID: projectID, HistoryID: "def.def",
				FullName: "spec/cart.ts > cart", Status: "broken", DurationMs: 20},
			{BuildID: 100, ProjectID: projectID, HistoryID: "def:def",
				FullName: "spec/cart.ts > cart", Status: "broken", DurationMs: 20,
				StatusMessage: "timeout"},
		}, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _ int64, _ int64) (int, error) {
		return 2, nil
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
	if out.ExaminedTests != 2 {
		t.Fatalf("examined_tests: got %d, want 2 (4 rows are 2 tests)", out.ExaminedTests)
	}
	if out.Truncated {
		t.Errorf("truncated: got true, want false")
	}
	byName := make(map[string]tools.DiagnoseTest, len(out.FailingTests))
	for _, d := range out.FailingTests {
		byName[d.FullName] = d
	}
	login := byName["spec/login.ts > login"]
	if login.HistoryID != "abc:abc" {
		t.Errorf("surviving history_id: got %q, want abc:abc (the enriched row)", login.HistoryID)
	}
	if len(login.MergedHistoryIDs) != 1 || login.MergedHistoryIDs[0] != "abc.abc" {
		t.Errorf("merged_history_ids: got %v, want [abc.abc]", login.MergedHistoryIDs)
	}
	if len(out.Warnings) != 1 {
		t.Fatalf("warnings: got %v, want exactly one merge warning", out.Warnings)
	}
	if !contains(out.Warnings[0], "2 duplicate history_id rows merged by full_name") {
		t.Errorf("warning text: got %q", out.Warnings[0])
	}
}

// TestDiagnoseFailure_EmitsRetriesFlakyAndFingerprintDetail verifies the signals
// the store already returns are surfaced rather than dropped on the floor.
func TestDiagnoseFailure_EmitsRetriesFlakyAndFingerprintDetail(t *testing.T) {
	const fpUUID = "33333333-3333-3333-3333-333333333333"

	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Retried",
				Status: "failed", DurationMs: 10, Retries: 2, Flaky: true},
		}, nil
	}
	mocks.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, _ int64, _ int64, _ string) (*string, error) {
		id := fpUUID
		return &id, nil
	}
	mocks.Defects.Seed(store.DefectFingerprint{
		ID:               fpUUID,
		ProjectID:        projectID,
		FingerprintHash:  "hash-abc",
		Category:         store.DefectCategoryProductBug,
		Resolution:       "confirmed",
		OccurrenceCount:  17,
		FirstSeenBuildID: 42,
	})

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
	if len(out.FailingTests) != 1 {
		t.Fatalf("failing_tests: got %d, want 1", len(out.FailingTests))
	}
	d := out.FailingTests[0]
	if d.Retries != 2 {
		t.Errorf("retries: got %d, want 2", d.Retries)
	}
	if !d.Flaky {
		t.Error("flaky: got false, want true")
	}
	if d.Fingerprint == nil {
		t.Fatal("fingerprint: got nil")
	}
	if d.Fingerprint.OccurrenceCount != 17 {
		t.Errorf("fingerprint.occurrence_count: got %d, want 17", d.Fingerprint.OccurrenceCount)
	}
	if d.Fingerprint.Resolution != "confirmed" {
		t.Errorf("fingerprint.resolution: got %q, want confirmed", d.Fingerprint.Resolution)
	}
	if d.Fingerprint.FirstSeenBuildID != 42 {
		t.Errorf("fingerprint.first_seen_build_id: got %d, want 42", d.Fingerprint.FirstSeenBuildID)
	}
}

// TestDiagnoseFailure_ExplainsAbsentLastGood verifies that when a test has
// never passed on the build's own branch the output says so and falls back to
// ONE cross-branch lookup, instead of leaving last_good silently absent.
func TestDiagnoseFailure_ExplainsAbsentLastGood(t *testing.T) {
	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), "demo")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	branch := "feature/x"
	branchID := int64(7)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		return store.Build{ID: id, ProjectID: proj.ID, BuildNumber: 28, CIBranch: &branch, BranchID: &branchID}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: proj.ID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 10},
		}, nil
	}

	crossBranchCalls := 0
	sha := "deadbee"
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, bID *int64, _ int) (*store.TestHistoryEntry, error) {
		if bID != nil {
			return nil, nil // never passed on this branch
		}
		crossBranchCalls++
		return &store.TestHistoryEntry{
			BuildID: 55, BuildNumber: 20, Status: "passed",
			CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			BranchName: "main", CICommitSHA: &sha,
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": proj.ID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("failing_tests: got %d, want 1", len(out.FailingTests))
	}
	d := out.FailingTests[0]
	if d.LastGood != nil {
		t.Errorf("last_good: got %+v, want nil", d.LastGood)
	}
	want := "no passing run on branch feature/x in recorded history"
	if d.LastGoodAbsentReason != want {
		t.Errorf("last_good_absent_reason: got %q, want %q", d.LastGoodAbsentReason, want)
	}
	if crossBranchCalls != 1 {
		t.Errorf("cross-branch lookups: got %d, want exactly 1", crossBranchCalls)
	}
	if d.CrossBranchLastGood == nil {
		t.Fatal("cross_branch_last_good: got nil")
	}
	if d.CrossBranchLastGood.Branch != "main" {
		t.Errorf("cross_branch_last_good.branch: got %q, want main", d.CrossBranchLastGood.Branch)
	}
	if d.CrossBranchLastGood.BuildNumber != 20 || d.CrossBranchLastGood.BuildID != 55 {
		t.Errorf("cross_branch_last_good build: got %d/%d, want 55/20",
			d.CrossBranchLastGood.BuildID, d.CrossBranchLastGood.BuildNumber)
	}
	if d.CrossBranchLastGood.CommitSHA != sha {
		t.Errorf("cross_branch_last_good.commit_sha: got %q, want %q", d.CrossBranchLastGood.CommitSHA, sha)
	}
}

// TestDiagnoseFailure_DigestIsShortAndNotAPayloadCopy verifies the tool returns
// a one-line text digest rather than letting the SDK duplicate the whole
// structured payload as JSON text.
func TestDiagnoseFailure_DigestIsShortAndNotAPayloadCopy(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 10},
		}, nil
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
	if len(res.Content) != 1 {
		t.Fatalf("content blocks: got %d, want 1 digest block", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content block type: got %T, want *TextContent", res.Content[0])
	}
	if len(tc.Text) >= 200 {
		t.Errorf("digest length: got %d bytes, want under 200", len(tc.Text))
	}
	if !contains(tc.Text, "build #28") || !contains(tc.Text, "/projects/") {
		t.Errorf("digest should name the build and the report URL, got %q", tc.Text)
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

// TestDiagnoseFailure_LastGoodPopulated verifies that when a test has a prior
// passing build, the last_good pointer is populated with the correct
// build_number, commit_sha, and builds_since, and that GetLastPassingBuild is
// called with the build's branch scope and its build_order (build_number) —
// NOT its build_id — as the exclusive upper bound.
func TestDiagnoseFailure_LastGoodPopulated(t *testing.T) {
	const buildID int64 = 100
	const lastGoodBuildID int64 = 80
	const branchID int64 = 3

	mocks := testutil.New()
	proj, err := mocks.Projects.CreateProject(context.Background(), "lastgood-proj")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	branchStr := "main"
	sha := "cursha"
	total, passed, failed, broken := 10, 7, 2, 1
	branchIDVal := branchID
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != buildID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{
			ID: buildID, ProjectID: proj.ID, BuildNumber: 28,
			CIBranch: &branchStr, CICommitSHA: &sha,
			StatTotal: &total, StatPassed: &passed, StatFailed: &failed, StatBroken: &broken,
			BranchID: &branchIDVal,
		}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: buildID, ProjectID: proj.ID, HistoryID: "h1", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 1200},
		}, nil
	}
	// Prior history (most-recent-first, includes current build order 28). After
	// the current build is dropped (matched by BuildID), priorBuildOrders =
	// [27, 26, 25]; entries strictly between the last-good order (25) and the
	// current order (28) are {27, 26} → builds_since = 2.
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildID: 100, BuildNumber: 28, Status: "failed", DurationMs: 1200},
			{BuildID: 95, BuildNumber: 27, Status: "failed", DurationMs: 1100},
			{BuildID: 90, BuildNumber: 26, Status: "failed", DurationMs: 1150},
			{BuildID: 80, BuildNumber: 25, Status: "passed", DurationMs: 5000},
		}, nil
	}
	// Capture the branch scope + exclusive bound GetLastPassingBuild is called with.
	var capturedBranchID *int64
	var capturedBefore int
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, br *int64, before int) (*store.TestHistoryEntry, error) {
		capturedBranchID = br
		capturedBefore = before
		return &store.TestHistoryEntry{
			BuildID: lastGoodBuildID, BuildNumber: 25, Status: "passed",
			DurationMs: 5000, CICommitSHA: new("goodsha"),
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": proj.ID, "build_id": buildID},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if capturedBranchID == nil || *capturedBranchID != branchID {
		t.Errorf("GetLastPassingBuild branchID: got %v, want %d", capturedBranchID, branchID)
	}
	const wantBeforeOrder = 28
	if capturedBefore != wantBeforeOrder {
		t.Errorf("GetLastPassingBuild beforeBuildOrder: got %d, want %d (build_order, not build_id)", capturedBefore, wantBeforeOrder)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	lg := out.FailingTests[0].LastGood
	if lg == nil {
		t.Fatal("want last_good populated when a prior passing build exists")
	}
	if lg.BuildID != lastGoodBuildID {
		t.Errorf("last_good.build_id: got %d, want %d", lg.BuildID, lastGoodBuildID)
	}
	if lg.BuildNumber != 25 {
		t.Errorf("last_good.build_number: got %d, want 25", lg.BuildNumber)
	}
	if lg.CommitSHA != "goodsha" {
		t.Errorf("last_good.commit_sha: got %q, want goodsha", lg.CommitSHA)
	}
	if lg.BuildsSince != 2 {
		t.Errorf("last_good.builds_since: got %d, want 2", lg.BuildsSince)
	}
	// last_good_diff must be absent when include_last_good_diff is unset.
	if out.FailingTests[0].LastGoodDiff != nil {
		t.Errorf("last_good_diff must be nil without include_last_good_diff, got %+v", out.FailingTests[0].LastGoodDiff)
	}
}

// TestDiagnoseFailure_LastGoodNilWhenNeverPassed verifies that when the test has
// no prior passing build, last_good is omitted (nil).
func TestDiagnoseFailure_LastGoodNilWhenNeverPassed(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.NeverPassed", Status: "failed", DurationMs: 10},
		}, nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return nil, nil // never passed before
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
	if len(out.FailingTests) != 1 {
		t.Fatalf("want 1 failing test, got %d", len(out.FailingTests))
	}
	if out.FailingTests[0].LastGood != nil {
		t.Errorf("want last_good nil when the test never passed, got %+v", out.FailingTests[0].LastGood)
	}
}

// TestDiagnoseFailure_LastGoodDiffGatedByFlag verifies what the flag actually
// gates. The comparison itself is memoized per distinct last-good build, so it
// is cheap enough to run by default and always yields last_good_diff_counts;
// what include_last_good_diff buys is the expensive part of the payload — the
// this_test transition and the co-regression sample in last_good_diff.
func TestDiagnoseFailure_LastGoodDiffGatedByFlag(t *testing.T) {
	const buildID int64 = 100
	const lastGoodBuildID int64 = 80

	newMocks := func() (*testutil.MockStores, int64) {
		mocks := testutil.New()
		projectID := seedDiagnoseProjectBuild(t, mocks, buildID, 28)
		mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{
				{BuildID: buildID, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 1500},
			}, nil
		}
		mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
			return &store.TestHistoryEntry{BuildID: lastGoodBuildID, BuildNumber: 25, Status: "passed", DurationMs: 900}, nil
		}
		return mocks, projectID
	}

	// 1. Default path: counts are emitted, the heavy lists are not.
	t.Run("flag off emits counts without the lists", func(t *testing.T) {
		mocks, projectID := newMocks()
		compareCalls := 0
		mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
			compareCalls++
			return []store.DiffEntry{
				{FullName: "pkg.LoginTest", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
				{FullName: "pkg.CoTest", HistoryID: "hCo", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
				{FullName: "pkg.FixedTest", HistoryID: "hFix", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
				{FullName: "pkg.NewTest", HistoryID: "hNew", StatusB: "failed", Category: store.DiffAdded},
			}, nil
		}

		cs := setupTestServer(t, buildStoresDiagnose(mocks))
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "diagnose_failure",
			Arguments: map[string]any{"project_id": projectID, "build_id": buildID},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %v", res.Content)
		}
		if compareCalls != 1 {
			t.Errorf("CompareBuildsByHistoryID calls: got %d, want 1 (memoized per last-good build)", compareCalls)
		}
		out := decodeDiagnoseFailure(t, res)
		counts := out.FailingTests[0].LastGoodDiffCounts
		if counts == nil {
			t.Fatal("last_good_diff_counts must be emitted whenever last_good exists")
		}
		if counts.Regressed != 2 || counts.Fixed != 1 || counts.Added != 1 {
			t.Errorf("last_good_diff_counts: got %+v, want {regressed:2 fixed:1 added:1}", counts)
		}
		if out.FailingTests[0].LastGoodDiff != nil {
			t.Errorf("last_good_diff must be nil without the flag, got %+v", out.FailingTests[0].LastGoodDiff)
		}
	})

	// 1b. No last-good build means no counts and no comparison at all.
	t.Run("no last good means no counts", func(t *testing.T) {
		mocks, projectID := newMocks()
		mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
			return nil, nil
		}
		compareCalled := false
		mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
			compareCalled = true
			return nil, nil
		}

		cs := setupTestServer(t, buildStoresDiagnose(mocks))
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "diagnose_failure",
			Arguments: map[string]any{"project_id": projectID, "build_id": buildID},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %v", res.Content)
		}
		if compareCalled {
			t.Error("CompareBuildsByHistoryID must not run when the test has no last-good build")
		}
		out := decodeDiagnoseFailure(t, res)
		if out.FailingTests[0].LastGoodDiffCounts != nil {
			t.Errorf("last_good_diff_counts must be nil without a last-good build, got %+v", out.FailingTests[0].LastGoodDiffCounts)
		}
	})

	// 2. Flag on: comparison runs with (lastGood, current) and populates the diff.
	t.Run("flag on populates diff", func(t *testing.T) {
		mocks, projectID := newMocks()
		var gotA, gotB int64
		mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, a, b int64) ([]store.DiffEntry, error) {
			gotA, gotB = a, b
			return []store.DiffEntry{
				{TestName: "pkg.LoginTest", FullName: "pkg.LoginTest", HistoryID: "h1", StatusA: "passed", StatusB: "failed", DurationA: 900, DurationB: 1500, Category: store.DiffRegressed},
				{TestName: "pkg.CoTest", FullName: "pkg.CoTest", HistoryID: "hCo", StatusA: "passed", StatusB: "failed", DurationA: 100, DurationB: 200, Category: store.DiffRegressed},
				{TestName: "pkg.FixedTest", FullName: "pkg.FixedTest", HistoryID: "hFix", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
			}, nil
		}

		cs := setupTestServer(t, buildStoresDiagnose(mocks))
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "diagnose_failure",
			Arguments: map[string]any{"project_id": projectID, "build_id": buildID, "include_last_good_diff": true},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %v", res.Content)
		}
		if gotA != lastGoodBuildID || gotB != buildID {
			t.Errorf("CompareBuildsByHistoryID args: got (A=%d,B=%d), want (A=%d,B=%d)", gotA, gotB, lastGoodBuildID, buildID)
		}

		out := decodeDiagnoseFailure(t, res)
		diff := out.FailingTests[0].LastGoodDiff
		if diff == nil {
			t.Fatal("want last_good_diff populated when include_last_good_diff is set")
		}
		if diff.FromBuildID != lastGoodBuildID || diff.ToBuildID != buildID {
			t.Errorf("diff from/to: got %d/%d, want %d/%d", diff.FromBuildID, diff.ToBuildID, lastGoodBuildID, buildID)
		}
		if diff.ThisTest.HistoryID != "h1" || diff.ThisTest.StatusFrom != "passed" || diff.ThisTest.StatusTo != "failed" {
			t.Errorf("this_test transition: got %+v", diff.ThisTest)
		}
		if diff.ThisTest.DurationDelta != 600 {
			t.Errorf("this_test.duration_delta_ms: got %d, want 600", diff.ThisTest.DurationDelta)
		}
		if diff.RegressedCount != 2 || diff.FixedCount != 1 {
			t.Errorf("counts: regressed=%d fixed=%d, want 2/1", diff.RegressedCount, diff.FixedCount)
		}
		// sample_regressed excludes the diagnosed test (h1); only the co-regression remains.
		if len(diff.SampleRegressed) != 1 || diff.SampleRegressed[0].HistoryID != "hCo" {
			t.Errorf("sample_regressed: got %+v, want [hCo]", diff.SampleRegressed)
		}
	})
}

// TestDiagnoseFailure_LastGoodStoreErrorDegrades verifies best-effort semantics:
// a GetLastPassingBuild error leaves last_good nil without failing the tool, and
// a CompareBuildsByHistoryID error (flag on) leaves last_good_diff nil while the
// last_good pointer itself is still populated.
func TestDiagnoseFailure_LastGoodStoreErrorDegrades(t *testing.T) {
	t.Run("pointer fetch error degrades to nil", func(t *testing.T) {
		mocks := testutil.New()
		projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)
		mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{
				{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 10},
			}, nil
		}
		mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
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
			t.Fatalf("last-good fetch error must not fail the tool: %v", res.Content)
		}
		out := decodeDiagnoseFailure(t, res)
		if out.FailingTests[0].LastGood != nil {
			t.Errorf("want last_good nil on fetch error, got %+v", out.FailingTests[0].LastGood)
		}
	})

	t.Run("diff comparison error keeps pointer, nil diff", func(t *testing.T) {
		mocks := testutil.New()
		projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)
		mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{
				{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Test", Status: "failed", DurationMs: 10},
			}, nil
		}
		mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
			return &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed"}, nil
		}
		mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
			return nil, context.DeadlineExceeded
		}

		cs := setupTestServer(t, buildStoresDiagnose(mocks))
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "diagnose_failure",
			Arguments: map[string]any{"project_id": projectID, "build_id": 100, "include_last_good_diff": true},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("last-good diff error must not fail the tool: %v", res.Content)
		}
		out := decodeDiagnoseFailure(t, res)
		d := out.FailingTests[0]
		if d.LastGood == nil {
			t.Error("last_good pointer must remain populated despite a diff error")
		}
		if d.LastGoodDiff != nil {
			t.Errorf("last_good_diff must be nil on comparison error, got %+v", d.LastGoodDiff)
		}
	})
}

// TestDiagnoseFailure_LastGoodBuildsSince_NonLatestBuild is a regression test
// for buildsSinceLastGood over-counting when the diagnosed build is NOT the
// latest build to run this test. GetTestHistory's window is the most recent
// diagnoseHistoryDepth builds of this test project/branch-wide — not capped at
// the build being diagnosed — so when a non-latest build is diagnosed, that
// window holds builds NEWER than the current one too. Counting "builds newer
// than last-good" without an upper bound at the current build's order collapses
// to "count the whole window." This test diagnoses build_order=15 while the
// test's history also includes later runs at orders 16, 18, and 20; the
// last-good pass is at order 10, and there is exactly one intervening build
// (order 12) between last-good and current. builds_since must be 1, not the
// window size.
func TestDiagnoseFailure_LastGoodBuildsSince_NonLatestBuild(t *testing.T) {
	const buildID int64 = 1015
	const buildNumber = 15
	const lastGoodBuildID int64 = 1010

	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, buildID, buildNumber)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: buildID, ProjectID: projectID, HistoryID: "hNL", FullName: "pkg.NonLatestTest", Status: "failed", DurationMs: 100},
		}, nil
	}
	// History window (most-recent-first, as GetTestHistory returns): includes
	// three runs NEWER than the diagnosed build (orders 20, 18, 16), the current
	// build itself (order 15, excluded by BuildID match), one intervening build
	// (order 12), and the last-good pass (order 10).
	mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
		return []store.TestHistoryEntry{
			{BuildID: 1020, BuildNumber: 20, Status: "passed", DurationMs: 100},
			{BuildID: 1018, BuildNumber: 18, Status: "failed", DurationMs: 100},
			{BuildID: 1016, BuildNumber: 16, Status: "failed", DurationMs: 100},
			{BuildID: buildID, BuildNumber: buildNumber, Status: "failed", DurationMs: 100}, // current, excluded
			{BuildID: 1012, BuildNumber: 12, Status: "failed", DurationMs: 100},
			{BuildID: lastGoodBuildID, BuildNumber: 10, Status: "passed", DurationMs: 100},
		}, nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return &store.TestHistoryEntry{BuildID: lastGoodBuildID, BuildNumber: 10, Status: "passed", DurationMs: 100}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": buildID},
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
	lg := out.FailingTests[0].LastGood
	if lg == nil {
		t.Fatal("want last_good populated")
	}
	const wantBuildsSince = 1 // only order 12 falls strictly between 10 and 15
	if lg.BuildsSince != wantBuildsSince {
		t.Errorf("last_good.builds_since: got %d, want %d (must not count builds newer than the diagnosed build)",
			lg.BuildsSince, wantBuildsSince)
	}
}

// TestDiagnoseFailure_LastGoodDiff_DedupesSharedLastGoodBuild verifies that
// CompareBuildsByHistoryID is invoked at most once per distinct last-good
// build_id across the failing-tests loop, even when multiple failing tests
// share the same last-good build (the common case: most tests last passed at
// the same recent green build). Without memoization the same whole-build diff
// query would be re-run once per failing test.
func TestDiagnoseFailure_LastGoodDiff_DedupesSharedLastGoodBuild(t *testing.T) {
	const buildID int64 = 100
	const lastGoodBuildID int64 = 80

	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, buildID, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: buildID, ProjectID: projectID, HistoryID: "hA", FullName: "pkg.TestA", Status: "failed", DurationMs: 100},
			{BuildID: buildID, ProjectID: projectID, HistoryID: "hB", FullName: "pkg.TestB", Status: "failed", DurationMs: 100},
			{BuildID: buildID, ProjectID: projectID, HistoryID: "hC", FullName: "pkg.TestC", Status: "failed", DurationMs: 100},
		}, nil
	}
	// All three failing tests share the same last-good build.
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return &store.TestHistoryEntry{BuildID: lastGoodBuildID, BuildNumber: 25, Status: "passed"}, nil
	}
	compareCalls := 0
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, a, b int64) ([]store.DiffEntry, error) {
		compareCalls++
		return []store.DiffEntry{
			{TestName: "pkg.TestA", FullName: "pkg.TestA", HistoryID: "hA", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
			{TestName: "pkg.TestB", FullName: "pkg.TestB", HistoryID: "hB", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
			{TestName: "pkg.TestC", FullName: "pkg.TestC", HistoryID: "hC", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": buildID, "include_last_good_diff": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if compareCalls != 1 {
		t.Errorf("CompareBuildsByHistoryID call count: got %d, want 1 (memoized per last-good build_id across 3 failing tests sharing it)", compareCalls)
	}

	// Each test's own this_test transition must still be correctly resolved
	// from the shared/cached diff set.
	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 3 {
		t.Fatalf("want 3 failing tests, got %d", len(out.FailingTests))
	}
	for _, d := range out.FailingTests {
		if d.LastGoodDiff == nil {
			t.Fatalf("test %q: want last_good_diff populated", d.HistoryID)
		}
		if d.LastGoodDiff.ThisTest.HistoryID != d.HistoryID {
			t.Errorf("test %q: this_test.history_id got %q, want %q", d.HistoryID, d.LastGoodDiff.ThisTest.HistoryID, d.HistoryID)
		}
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

// tokenAuth500 and toastAssertion are the two real error messages from the
// report that motivated clustering: eight failures, four sharing a TokenAuth
// 500 raised in beforeEach and three sharing one toast assertion, all of it
// previously rendered as a flat list of eight equal-looking entries.
const (
	tokenAuth500   = "Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate"
	toastAssertion = "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')"
)

// seedTwoRootCauses wires a build whose failures have exactly two root causes:
// four before-hooks failures against one 500 endpoint and three test-body
// failures on one assertion. The last-good build carries the same commit sha as
// the diagnosed build, so the code has not changed since these tests passed.
func seedTwoRootCauses(t *testing.T) (*testutil.MockStores, int64) {
	t.Helper()
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	hooks := []string{"h1", "h2", "h3", "h4"}
	toast := []string{"t1", "t2", "t3"}

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		rows := make([]store.TestResult, 0, len(hooks)+len(toast))
		for _, h := range hooks {
			rows = append(rows, store.TestResult{
				BuildID: 100, ProjectID: projectID, HistoryID: h,
				FullName: "spec/auth-" + h + ".ts > login", Status: "failed", DurationMs: 120,
				StatusMessage: tokenAuth500,
			})
		}
		for _, h := range toast {
			rows = append(rows, store.TestResult{
				BuildID: 100, ProjectID: projectID, HistoryID: h,
				FullName: "spec/save-" + h + ".ts > save", Status: "failed", DurationMs: 5200,
				StatusMessage: toastAssertion,
			})
		}
		return rows, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, historyID string) ([]string, string, error) {
		if strings.HasPrefix(historyID, "h") {
			return []string{"Before Hooks", "login via API"}, tokenAuth500, nil
		}
		return []string{"Test Body", "check toast"}, toastAssertion, nil
	}
	// Every test last passed at the same build, on the same commit as the one
	// being diagnosed ("abc123" from seedDiagnoseProjectBuild).
	sameSHA := "abc123"
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed", CICommitSHA: &sameSHA}, nil
	}
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _ int64, _ int64) (int, error) {
		return len(hooks) + len(toast), nil
	}
	return mocks, projectID
}

// TestDiagnoseFailure_ClustersSharedRootCauses is the end-to-end guard for the
// live complaint: seven failures with two root causes must come back as two
// clusters, with the duplicated error text carried once per cluster rather than
// once per test.
func TestDiagnoseFailure_ClustersSharedRootCauses(t *testing.T) {
	mocks, projectID := seedTwoRootCauses(t)

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
	if len(out.Clusters) != 2 {
		t.Fatalf("clusters: got %d, want 2 (%+v)", len(out.Clusters), out.Clusters)
	}

	auth := out.Clusters[0]
	if auth.ClusterID != "c1" || auth.MemberCount != 4 {
		t.Errorf("dominant cluster: got id=%q count=%d, want c1/4", auth.ClusterID, auth.MemberCount)
	}
	if auth.FailurePhase != triage.PhaseBeforeHooks {
		t.Errorf("dominant cluster failure_phase: got %q, want before_hooks", auth.FailurePhase)
	}
	if auth.SharedError != tokenAuth500 {
		t.Errorf("dominant cluster shared_error: got %q", auth.SharedError)
	}
	if auth.SharedStatusPattern == nil || auth.SharedStatusPattern.StatusCode != 500 {
		t.Errorf("dominant cluster shared_status_pattern: got %+v, want 500", auth.SharedStatusPattern)
	}
	if len(auth.MemberFullNames) != 4 {
		t.Errorf("dominant cluster member_full_names: got %v", auth.MemberFullNames)
	}
	if out.Clusters[1].ClusterID != "c2" || out.Clusters[1].MemberCount != 3 {
		t.Errorf("second cluster: got id=%q count=%d, want c2/3", out.Clusters[1].ClusterID, out.Clusters[1].MemberCount)
	}

	// Every test is tagged; only the two representatives keep the heavy text.
	withText := 0
	byName := make(map[string]tools.DiagnoseTest, len(out.FailingTests))
	for _, d := range out.FailingTests {
		byName[d.FullName] = d
		if d.ClusterID == "" {
			t.Errorf("test %q carries no cluster_id", d.FullName)
		}
		if d.ErrorMessage != "" {
			withText++
		}
	}
	if withText != 2 {
		t.Errorf("tests keeping error_message: got %d, want 2 (one representative per cluster)", withText)
	}
	rep := byName[auth.RepresentativeFullName]
	if rep.ErrorMessage != tokenAuth500 || len(rep.FailedStepPath) == 0 {
		t.Errorf("representative must keep its text and step path, got %+v", rep)
	}
	// A stripped member keeps everything except the duplicated text.
	for _, name := range auth.MemberFullNames[1:] {
		m := byName[name]
		if m.ErrorMessage != "" || m.FailedStepPath != nil {
			t.Errorf("member %q must be stripped, got msg=%q path=%v", name, m.ErrorMessage, m.FailedStepPath)
		}
		if m.Signals.FailurePhase != triage.PhaseBeforeHooks {
			t.Errorf("member %q lost its signals: %+v", name, m.Signals)
		}
		if m.LastGood == nil {
			t.Errorf("member %q lost its last_good pointer", name)
		}
	}
}

// TestDiagnoseFailure_BuildVerdict verifies the build-level judgement: the
// dominant cluster decides the verdict, the disagreement with the second
// cluster caps the confidence, and the action follows the verdict.
func TestDiagnoseFailure_BuildVerdict(t *testing.T) {
	mocks, projectID := seedTwoRootCauses(t)

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
	v := out.BuildVerdict
	if v.Verdict != tools.VerdictInfraEnv {
		t.Errorf("verdict: got %q, want %q (evidence %+v, gaps %v)", v.Verdict, tools.VerdictInfraEnv, v.Evidence, v.EvidenceGaps)
	}
	if v.Confidence != tools.ConfidenceMedium {
		t.Errorf("confidence: got %q, want medium (the two clusters disagree)", v.Confidence)
	}
	if v.RecommendedAction != tools.ActionRerun {
		t.Errorf("recommended_action: got %q, want %q", v.RecommendedAction, tools.ActionRerun)
	}
	if v.AffectedTestCount != 4 {
		t.Errorf("affected_test_count: got %d, want 4 (the dominant cluster)", v.AffectedTestCount)
	}
	if len(v.Evidence) == 0 {
		t.Error("verdict must carry its evidence")
	}
}

// TestDiagnoseFailure_VerdictNeverHighWhenTruncated pins the ceiling: a
// diagnosis that did not see the whole build cannot be confident about it.
func TestDiagnoseFailure_VerdictNeverHighWhenTruncated(t *testing.T) {
	mocks, projectID := seedTwoRootCauses(t)
	// More failures exist than were examined.
	mocks.TestResults.CountFailedByBuildFn = func(_ context.Context, _ int64, _ int64) (int, error) {
		return 40, nil
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
	if !out.Truncated {
		t.Fatal("want truncated=true for this fixture")
	}
	if out.BuildVerdict.Confidence == tools.ConfidenceHigh {
		t.Error("confidence must never be high when the diagnosis is truncated")
	}
	if len(out.BuildVerdict.EvidenceGaps) == 0 {
		t.Error("a truncated diagnosis must record the gap")
	}
}

// TestDiagnoseFailure_KnownIssueRegexMatchesOnCluster verifies the inline
// known-issue matching: the project's active rules are matched against each
// cluster's shared error, reported on the CLUSTER, and kept distinct from the
// per-test human-confirmed FK. A broken pattern is skipped rather than failing
// the call.
func TestDiagnoseFailure_KnownIssueRegexMatchesOnCluster(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)
	ctx := context.Background()

	if _, err := mocks.KnownIssues.Create(ctx, projectID, "auth-service-flap", `TokenAuth/Authenticate`, "", ""); err != nil {
		t.Fatalf("seed known issue: %v", err)
	}
	if _, err := mocks.KnownIssues.Create(ctx, projectID, "broken-rule", `([unclosed`, "", ""); err != nil {
		t.Fatalf("seed broken known issue: %v", err)
	}
	if _, err := mocks.KnownIssues.Create(ctx, projectID, "never-matches", `will-not-match-anything`, "", ""); err != nil {
		t.Fatalf("seed non-matching known issue: %v", err)
	}

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "spec/a.ts > a", Status: "failed", DurationMs: 100},
		}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return []string{"Test Body", "call api"}, tokenAuth500, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("a broken known-issue pattern must not fail the call: %v", res.Content)
	}

	out := decodeDiagnoseFailure(t, res)
	if len(out.Clusters) != 1 {
		t.Fatalf("clusters: got %d, want 1", len(out.Clusters))
	}
	matches := out.Clusters[0].KnownIssueRegexMatches
	if len(matches) != 1 {
		t.Fatalf("known_issue_regex_matches: got %+v, want exactly the one matching rule", matches)
	}
	if matches[0].Name != "auth-service-flap" {
		t.Errorf("match name: got %q, want auth-service-flap", matches[0].Name)
	}
	if matches[0].MatchedSubstring != "TokenAuth/Authenticate" {
		t.Errorf("matched_substring: got %q", matches[0].MatchedSubstring)
	}
	// A regex hit is not a confirmed link: the per-test FK stays empty.
	if out.FailingTests[0].KnownIssue != nil {
		t.Errorf("per-test known_issue must stay nil for a regex-only match, got %+v", out.FailingTests[0].KnownIssue)
	}
	// A regex hit is still enough to reach the known_issue verdict.
	if out.BuildVerdict.Verdict != tools.VerdictKnownIssue {
		t.Errorf("verdict: got %q, want %q", out.BuildVerdict.Verdict, tools.VerdictKnownIssue)
	}
	if out.BuildVerdict.RecommendedAction != tools.ActionLinkKnownIssue {
		t.Errorf("recommended_action: got %q, want %q", out.BuildVerdict.RecommendedAction, tools.ActionLinkKnownIssue)
	}
}

// TestDiagnoseFailure_DigestLeadsWithVerdict verifies the headline: cluster
// count and verdict first, so a reader can decide whether to open the payload
// without opening the payload.
func TestDiagnoseFailure_DigestLeadsWithVerdict(t *testing.T) {
	mocks, projectID := seedTwoRootCauses(t)

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
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content block type: got %T, want *TextContent", res.Content[0])
	}

	for _, want := range []string{
		"build #28 (main)",
		"7 failing in 2 clusters",
		"verdict: infra_env (medium)",
		"4x status 500",
		"in before_hooks",
		"/projects/",
	} {
		if !contains(tc.Text, want) {
			t.Errorf("digest missing %q, got %q", want, tc.Text)
		}
	}
	// The digest must stay a headline, not become a second payload.
	if len(tc.Text) >= 250 {
		t.Errorf("digest length: got %d bytes, want under 250 (%q)", len(tc.Text), tc.Text)
	}
}
