package tools_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetTestFailure(t *testing.T) {
	const fpUUID = "11111111-1111-1111-1111-111111111111"
	env := map[string]string{"Grafana.Drilldown.URL": "https://example/x", "Loki.Query": `{k8s_namespace_name="ns-x"}`}
	ciBuild := &store.Build{ID: 10, ProjectID: 1, BuildNumber: 5, CIBranch: new("main"), CICommitSHA: new("abc123"),
		CIPipelineURL: new("https://ci.example.com/jobs/42"), Environment: env}

	tests := []struct {
		name        string
		args        map[string]any
		build       *store.Build // returned by GetBuildByID; nil: ErrBuildNotFound
		result      *store.TestResult
		fingerprint bool
		want        tools.GetTestFailureOutput
		wantErr     string
	}{
		{
			// CI metadata comes from the requested build row, not the latest
			// build; the fingerprint is resolved through the
			// defect_fingerprint_id FK, never by passing history_id to GetByHash.
			name:        "failed test with CI, environment, fingerprint and its own attachments",
			args:        map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
			build:       ciBuild,
			result:      &store.TestResult{HistoryID: "h1", FullName: "pkg.Test1", Status: "failed", StatusMessage: "boom", DurationMs: 1234},
			fingerprint: true,
			want: tools.GetTestFailureOutput{Status: "failed", StatusMessage: "boom", DurationMs: 1234,
				Attachments: []tools.AttachmentRef{{ID: 99, Name: "h1.png", Mime: "image/png", SizeBytes: 4096, ResourceURI: "alluredeck://attachment/99"}},
				CI:          &tools.CIInfo{CommitSHA: "abc123", Branch: "main", PipelineURL: "https://ci.example.com/jobs/42"},
				Fingerprint: &tools.FingerprintInfo{Hash: "deadbeefhash", Category: store.DefectCategoryProductBug},
				KnownIssue:  &tools.KnownIssueRef{Name: "flaky-login"},
				Environment: env},
		},
		{
			// The row is looked up by key, so a passing test reports its status
			// instead of a misleading not-found.
			name:   "passing test is reported, not treated as missing",
			args:   map[string]any{"project_id": 1, "build_id": 10, "history_id": "h-green"},
			build:  &store.Build{ID: 10, BuildNumber: 5},
			result: &store.TestResult{HistoryID: "h-green", FullName: "pkg.GreenTest", Status: "passed", DurationMs: 77},
			want:   tools.GetTestFailureOutput{Status: "passed", DurationMs: 77, Attachments: []tools.AttachmentRef{}},
		},
		{name: "unknown history_id", args: map[string]any{"project_id": 1, "build_id": 10, "history_id": "nope"},
			build: &store.Build{ID: 10}, wantErr: `history_id "nope" not found`},
		{name: "build outside the project hints at resolve_url", args: map[string]any{"project_id": 1, "build_id": 28, "history_id": "h1"},
			wantErr: "resolve_url"},
		{name: "empty history_id", args: map[string]any{"project_id": 1, "build_id": 10, "history_id": ""}, wantErr: "history_id must not be empty"},
		{name: "non-positive project_id", args: map[string]any{"project_id": 0, "build_id": 10, "history_id": "h1"}, wantErr: "project_id must be positive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
				if tc.build == nil || tc.build.ID != id {
					return store.Build{}, store.ErrBuildNotFound
				}
				return *tc.build, nil
			}
			mocks.TestResults.GetByHistoryIDFn = func(_ context.Context, projectID, buildID int64, historyID string) (*store.TestResult, error) {
				if tc.result == nil || projectID != 1 || buildID != 10 || historyID != tc.result.HistoryID {
					return nil, nil
				}
				return tc.result, nil
			}
			mocks.TestResults.ListFailedByBuildFn = func(context.Context, int64, int64, int) ([]store.TestResult, error) {
				t.Error("get_test_failure must look the row up by key, not scan the failing list")
				return nil, nil
			}
			mocks.Attachments.ListByTestResultFn = func(_ context.Context, _, _ int64, historyID string, _ int) ([]store.TestAttachment, error) {
				if historyID != "h1" {
					return nil, nil
				}
				return []store.TestAttachment{{ID: 99, Name: "h1.png", MimeType: "image/png", SizeBytes: 4096}}, nil
			}
			mocks.Attachments.ListByBuildFn = func(context.Context, int64, int64, string, string, int, int) ([]store.TestAttachment, int, error) {
				t.Error("attachments must be scoped to the test result, not the whole build")
				return nil, 0, nil
			}
			if tc.fingerprint {
				ki, err := mocks.KnownIssues.Create(context.Background(), 1, "flaky-login", ".*", "", "")
				if err != nil {
					t.Fatalf("seed known issue: %v", err)
				}
				tc.want.KnownIssue.ID = ki.ID
				mocks.Defects.Seed(store.DefectFingerprint{ID: fpUUID, ProjectID: 1, FingerprintHash: "deadbeefhash",
					Category: store.DefectCategoryProductBug, KnownIssueID: &ki.ID})
				mocks.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, projectID, buildID int64, historyID string) (*string, error) {
					if projectID != 1 || buildID != 10 || historyID != "h1" {
						return nil, store.ErrTestResultNotFound
					}
					return new(fpUUID), nil
				}
			}
			cs := setupTestServer(t, &bootstrap.Stores{Build: mocks.Builds, TestResult: mocks.TestResults, Attachment: mocks.Attachments,
				Defect: mocks.Defects, KnownIssue: mocks.KnownIssues})

			if tc.wantErr != "" {
				if msg := callErr(t, cs, "get_test_failure", tc.args); !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				return
			}
			if out, _ := call[tools.GetTestFailureOutput](t, cs, "get_test_failure", tc.args); !reflect.DeepEqual(out, tc.want) {
				t.Errorf("output = %+v, want %+v", out, tc.want)
			}
		})
	}
}

func TestGetTestHistory(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entries := []store.TestHistoryEntry{
		{BuildNumber: 5, BuildID: 50, Status: "passed", DurationMs: 500, CreatedAt: created, CICommitSHA: new("deadbeef")},
		{BuildNumber: 4, BuildID: 40, Status: "failed", DurationMs: 800, CreatedAt: created, BranchName: "feature/x"},
		{BuildNumber: 3, BuildID: 30, Status: "failed", DurationMs: 900, CreatedAt: created},
	}
	tests := []struct {
		name       string
		args       map[string]any
		branchErr  error
		want       []tools.TestHistoryItem
		wantBranch *int64 // branch id forwarded to GetTestHistory
		wantLimit  int
		wantErr    string
	}{
		{
			// The store has no offset pagination, so exactly `limit` rows are
			// fetched and no next_cursor is emitted (the old one re-returned
			// page one forever); a deprecated cursor is accepted and ignored.
			// Each run names its own branch so a cross-branch history does not
			// read as one timeline.
			name: "cross-branch runs, exact limit, cursor ignored",
			args: map[string]any{"project_id": 1, "history_id": "h1", "limit": 2, "cursor": "not-a-real-cursor"},
			want: []tools.TestHistoryItem{
				{BuildID: 50, BuildNumber: 5, Status: "passed", DurationMs: 500, CommitSHA: "deadbeef", CreatedAt: "2026-01-02T03:04:05Z"},
				{BuildID: 40, BuildNumber: 4, Status: "failed", DurationMs: 800, CreatedAt: "2026-01-02T03:04:05Z", Branch: "feature/x"},
			},
			wantLimit: 2,
		},
		{name: "branch is resolved to its id", args: map[string]any{"project_id": 1, "history_id": "h1", "branch": "main", "limit": 1},
			want: []tools.TestHistoryItem{{BuildID: 50, BuildNumber: 5, Status: "passed", DurationMs: 500, CommitSHA: "deadbeef",
				CreatedAt: "2026-01-02T03:04:05Z"}}, wantBranch: new(int64(42)), wantLimit: 1},
		{name: "unknown branch is empty, not an error", args: map[string]any{"project_id": 1, "history_id": "h1", "branch": "nope"},
			branchErr: store.ErrBranchNotFound},
		// A DB fault must not masquerade as "no history on this branch".
		{name: "branch lookup failure is surfaced", args: map[string]any{"project_id": 1, "history_id": "h1", "branch": "main"},
			branchErr: context.DeadlineExceeded, wantErr: "resolving branch"},
		{name: "empty history_id", args: map[string]any{"project_id": 1, "history_id": ""}, wantErr: "history_id must not be empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, name string) (*store.Branch, error) {
				return &store.Branch{ID: 42, Name: name}, tc.branchErr
			}
			var gotBranch *int64
			gotLimit := 0
			mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, limit int) ([]store.TestHistoryEntry, error) {
				gotBranch, gotLimit = branchID, limit
				return entries[:min(limit, len(entries))], nil
			}
			cs := setupTestServer(t, &bootstrap.Stores{TestResult: mocks.TestResults, Branch: mocks.Branches})

			if tc.wantErr != "" {
				if msg := callErr(t, cs, "get_test_history", tc.args); !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				if gotLimit != 0 {
					t.Error("GetTestHistory ran although the call failed")
				}
				return
			}
			out, res := call[tools.GetTestHistoryOutput](t, cs, "get_test_history", tc.args)
			if !reflect.DeepEqual(out.Items, tc.want) {
				t.Errorf("items = %+v, want %+v", out.Items, tc.want)
			}
			if gotLimit != tc.wantLimit || !reflect.DeepEqual(gotBranch, tc.wantBranch) {
				t.Errorf("GetTestHistory(branch=%v, limit=%d), want (%v, %d)", gotBranch, gotLimit, tc.wantBranch, tc.wantLimit)
			}
			if raw := decode[map[string]any](t, res); raw["next_cursor"] != nil {
				t.Errorf("next_cursor must not be emitted: %v", raw)
			}
		})
	}
}

func TestCompareBuilds(t *testing.T) {
	diffs := []store.DiffEntry{
		{TestName: "T1", FullName: "pkg.T1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		{TestName: "T2", FullName: "pkg.T2", HistoryID: "h2", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
		{TestName: "T3", FullName: "pkg.T3", HistoryID: "h3", StatusB: "failed", Category: store.DiffAdded},
		{TestName: "T4", FullName: "pkg.T4", HistoryID: "h4", StatusB: "passed", Category: store.DiffAdded},
		{TestName: "T5", FullName: "pkg.T5", HistoryID: "h5", StatusA: "passed", Category: store.DiffRemoved},
	}
	item := func(testName, fullName, historyID, a, b string) []tools.DiffItem {
		return []tools.DiffItem{{TestName: testName, FullName: fullName, HistoryID: historyID, StatusA: a, StatusB: b}}
	}
	full := tools.CompareBuildsOutput{
		Regressed: item("T1", "pkg.T1", "h1", "passed", "failed"),
		Fixed:     item("T2", "pkg.T2", "h2", "failed", "passed"),
		NewFailed: item("T3", "pkg.T3", "h3", "", "failed"),
		NewPassed: item("T4", "pkg.T4", "h4", "", "passed"),
		Removed:   item("T5", "pkg.T5", "h5", "passed", ""),
	}
	// compact drops test_name and history_id; full_name already carries both.
	compact := tools.CompareBuildsOutput{
		Regressed: item("", "pkg.T1", "", "passed", "failed"),
		Fixed:     item("", "pkg.T2", "", "failed", "passed"),
		NewFailed: item("", "pkg.T3", "", "", "failed"),
		NewPassed: item("", "pkg.T4", "", "", "passed"),
		Removed:   item("", "pkg.T5", "", "passed", ""),
	}
	summary := tools.CompareBuildsOutput{Summary: &tools.CompareSummary{Regressed: 1, Fixed: 1, NewPassed: 1, NewFailed: 1, Removed: 1}}
	mismatch := &tools.BranchMismatchWarning{BaseBranch: "main", TargetBranch: "feature/x"}
	// The target build is hoisted as `build` in every format; the warning
	// fires only when both branches are known and differ.
	with := func(o tools.CompareBuildsOutput, branch string, mm *tools.BranchMismatchWarning) tools.CompareBuildsOutput {
		o.Build, o.BranchMismatch = &tools.BuildRef{BuildNumber: 42, Branch: branch}, mm
		return o
	}
	mainID, featID := int64(1), int64(2)

	tests := []struct {
		name         string
		format       string
		targetBranch *int64 // the base build is always on branch 1 "main"
		targetName   *string
		want         tools.CompareBuildsOutput
	}{
		{"default format, same branch", "", &mainID, new("main"), with(full, "main", nil)},
		{"compact, different branches", "compact", &featID, new("feature/x"), with(compact, "feature/x", mismatch)},
		{"summary, different branches", "summary", &featID, new("feature/x"), with(summary, "feature/x", mismatch)},
		{"full, target branch unknown (legacy build)", "full", nil, nil, with(full, "", nil)},
	}
	newSession := func(t *testing.T, targetBranch *int64, targetName *string) (*mcpsdk.ClientSession, *[3]int64) {
		mocks := testutil.New()
		mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
			switch id {
			case 10:
				return store.Build{ID: 10, BuildNumber: 41, BranchID: &mainID, CIBranch: new("main")}, nil
			case 20:
				return store.Build{ID: 20, BuildNumber: 42, BranchID: targetBranch, CIBranch: targetName}, nil
			}
			return store.Build{}, store.ErrBuildNotFound
		}
		var got [3]int64
		mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, projectID, a, b int64) ([]store.DiffEntry, error) {
			got = [3]int64{projectID, a, b}
			return diffs, nil
		}
		return setupTestServer(t, &bootstrap.Stores{Build: mocks.Builds, TestResult: mocks.TestResults}), &got
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs, gotArgs := newSession(t, tc.targetBranch, tc.targetName)
			out, _ := call[tools.CompareBuildsOutput](t, cs, "compare_builds",
				map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20, "format": tc.format})
			if out.BranchMismatch != nil {
				if out.BranchMismatch.Message == "" {
					t.Error("branch_mismatch carries no message")
				}
				out.BranchMismatch.Message = ""
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("output = %+v, want %+v", out, tc.want)
			}
			if *gotArgs != [3]int64{1, 10, 20} {
				t.Errorf("CompareBuildsByHistoryID(project, base, target) = %v, want [1 10 20]", *gotArgs)
			}
		})
	}

	cs, _ := newSession(t, &mainID, new("main"))
	// An unknown format is an error, not a silent fall-through to the full
	// payload a caller never asked to pay for.
	for _, format := range []string{"brief", "FULL", "json"} {
		callErr(t, cs, "compare_builds", map[string]any{"project_id": 1, "base_build_id": 10, "target_build_id": 20, "format": format})
	}
	callErr(t, cs, "compare_builds", map[string]any{"project_id": 1, "base_build_id": 0, "target_build_id": 20})
	if msg := callErr(t, cs, "compare_builds", map[string]any{"project_id": 1, "base_build_id": 28, "target_build_id": 20}); !strings.Contains(msg, "resolve_url") {
		t.Errorf("base outside the project: error %q lacks the resolve_url hint", msg)
	}
}
