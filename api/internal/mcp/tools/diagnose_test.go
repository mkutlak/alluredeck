package tools_test

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/failure"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// diagFixture is project "demo" with build 100 (#28 on main, commit abc123,
// stats 10/7/2/1) whose failing tests are rows.
type diagFixture struct {
	*testutil.MockStores
	build store.Build
	rows  []store.TestResult
}

func newDiagFixture(t *testing.T, rows ...store.TestResult) *diagFixture {
	t.Helper()
	f := &diagFixture{MockStores: testutil.New(), rows: rows}
	proj, err := f.Projects.CreateProject(context.Background(), "demo")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	total, passed, failed, broken := 10, 7, 2, 1
	f.build = store.Build{ID: 100, ProjectID: proj.ID, BuildNumber: 28, CIBranch: new("main"), CICommitSHA: new("abc123"),
		StatTotal: &total, StatPassed: &passed, StatFailed: &failed, StatBroken: &broken}
	f.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != f.build.ID {
			return store.Build{}, store.ErrBuildNotFound
		}
		return f.build, nil
	}
	f.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
		return f.rows[:min(limit, len(f.rows))], nil
	}
	return f
}

// failed is a failing test row of build 100 named pkg.<historyID>.
func failed(historyID string) store.TestResult {
	return store.TestResult{BuildID: 100, HistoryID: historyID, FullName: "pkg." + historyID, Status: "failed", DurationMs: 10}
}

// run calls diagnose_failure for build 100 with extra arguments merged in.
func (f *diagFixture) run(t *testing.T, extra map[string]any) (tools.DiagnoseFailureOutput, *mcpsdk.CallToolResult) {
	t.Helper()
	cs := setupTestServer(t, &bootstrap.Stores{Project: f.Projects, Build: f.Builds, TestResult: f.TestResults,
		Attachment: f.Attachments, Defect: f.Defects, KnownIssue: f.KnownIssues})
	args := map[string]any{"project_id": f.build.ProjectID, "build_id": f.build.ID}
	maps.Copy(args, extra)
	return call[tools.DiagnoseFailureOutput](t, cs, "diagnose_failure", args)
}

// TestDiagnoseFailure_FullDiagnosis is the one-call diagnosis: build summary,
// per-test failure detail and step path, history-fed triage signals,
// fingerprint and known issue, and attachments scoped to each test result —
// never the build-wide set.
func TestDiagnoseFailure_FullDiagnosis(t *testing.T) {
	const fpUUID = "22222222-2222-2222-2222-222222222222"
	login := failed("h1")
	login.Retries, login.Flaky, login.DurationMs = 2, true, 1200
	f := newDiagFixture(t, login, failed("h2"))
	env := map[string]string{"Grafana.Drilldown.URL": "https://example/x", "Loki.Query": `{k8s_namespace_name="ns-x"}`}
	f.build.Environment = env

	f.TestResults.GetFailedStepPathFn = func(_ context.Context, _, _ int64, historyID string) ([]string, string, error) {
		if historyID == "h1" {
			return []string{"Test Body", "Call API"}, "status 500 from /users", nil
		}
		return nil, "", nil
	}
	var historyBranch = new(int64(-1))
	f.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
		historyBranch = branchID
		return []store.TestHistoryEntry{
			{BuildID: 100, BuildNumber: 28, Status: "failed", DurationMs: 1200},
			{BuildID: 90, BuildNumber: 27, Status: "failed", DurationMs: 1100},
			{BuildID: 80, BuildNumber: 26, Status: "passed", DurationMs: 5000},
		}, nil
	}
	var attachErr error
	f.Attachments.ListByTestResultFn = func(_ context.Context, _, _ int64, historyID string, _ int) ([]store.TestAttachment, error) {
		if historyID == "h1" {
			return []store.TestAttachment{{ID: 7, Name: "trace.zip", MimeType: "application/zip", SizeBytes: 2048}}, attachErr
		}
		return []store.TestAttachment{{ID: 2, Name: "h2-a.png"}, {ID: 3, Name: "h2-b.png"}}, attachErr
	}
	f.Attachments.ListByBuildFn = func(context.Context, int64, int64, string, string, int, int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{{ID: 90}, {ID: 91}, {ID: 92}}, 3, nil
	}
	f.TestResults.GetDefectFingerprintIDFn = func(_ context.Context, _, _ int64, historyID string) (*string, error) {
		if historyID != "h1" {
			return nil, nil
		}
		return new(fpUUID), nil
	}
	ki, err := f.KnownIssues.Create(context.Background(), f.build.ProjectID, "flaky-api", ".*", "", "")
	if err != nil {
		t.Fatalf("seed known issue: %v", err)
	}
	f.Defects.Seed(store.DefectFingerprint{ID: fpUUID, ProjectID: f.build.ProjectID, FingerprintHash: "cafebabehash",
		Category: store.DefectCategoryInfrastructure, Resolution: "confirmed", OccurrenceCount: 17, FirstSeenBuildID: 42, KnownIssueID: &ki.ID})

	out, _ := f.run(t, nil)
	b := out.Build
	if b.BuildNumber != 28 || b.Branch != "main" || b.FailedTests != 2 || b.BrokenTests != 1 || !reflect.DeepEqual(b.Environment, env) {
		t.Errorf("build summary = %+v", b)
	}
	if out.ExaminedTests != 2 || len(out.FailingTests) != 2 {
		t.Fatalf("examined %d (%d items), want 2", out.ExaminedTests, len(out.FailingTests))
	}
	d, other := out.FailingTests[0], out.FailingTests[1]
	if d.FullName != "pkg.h1" || d.ErrorMessage != "status 500 from /users" || !reflect.DeepEqual(d.FailedStepPath, []string{"Test Body", "Call API"}) ||
		d.Retries != 2 || !d.Flaky {
		t.Errorf("h1 detail = %+v", d)
	}
	wantFP := tools.FingerprintInfo{Hash: "cafebabehash", Category: store.DefectCategoryInfrastructure, OccurrenceCount: 17, Resolution: "confirmed", FirstSeenBuildID: 42}
	if d.Fingerprint == nil || *d.Fingerprint != wantFP || d.KnownIssue == nil || *d.KnownIssue != (tools.KnownIssueRef{ID: ki.ID, Name: "flaky-api"}) {
		t.Errorf("h1 fingerprint = %+v, known issue = %+v", d.Fingerprint, d.KnownIssue)
	}
	if other.Fingerprint != nil || other.KnownIssue != nil {
		t.Errorf("h2 carries h1's defect: %+v %+v", other.Fingerprint, other.KnownIssue)
	}
	wantAtt := []tools.AttachmentRef{{ID: 7, Name: "trace.zip", Mime: "application/zip", SizeBytes: 2048, ResourceURI: "alluredeck://attachment/7"}}
	if !reflect.DeepEqual(d.Attachments, wantAtt) || len(other.Attachments) != 2 || other.Attachments[0].ID != 2 || other.Attachments[1].ID != 3 {
		t.Errorf("attachments = %+v / %+v, want each test's own set only", d.Attachments, other.Attachments)
	}
	// Prior history is [failed #27, passed #26] once the current build is
	// dropped; no branch on the build means a cross-branch (nil) lookup.
	s := d.Signals
	if s.LastStatus != triage.StatusFailed || s.BuildsSincePass != 1 || s.FailurePhase != triage.PhaseTestBody ||
		s.RepeatedStatusPattern == nil || s.RepeatedStatusPattern.StatusCode != 500 || s.CategoryHint.Value != store.DefectCategoryInfrastructure {
		t.Errorf("h1 signals = %+v", s)
	}
	if historyBranch != nil {
		t.Errorf("history branch = %d, want nil for a build without a branch", *historyBranch)
	}

	// summary_only drops the heavy per-test fields, keeping the message and signals.
	out, _ = f.run(t, map[string]any{"summary_only": true})
	if d := out.FailingTests[0]; d.FailedStepPath != nil || d.Attachments != nil || d.ErrorMessage != "status 500 from /users" ||
		d.Signals.CategoryHint.Value != store.DefectCategoryInfrastructure {
		t.Errorf("summary_only entry = %+v", d)
	}

	// Attachments are best-effort: a fetch failure costs them, not the diagnosis.
	attachErr = context.DeadlineExceeded
	out, _ = f.run(t, nil)
	if len(out.FailingTests) != 2 || out.FailingTests[0].Attachments != nil || out.FailingTests[0].ErrorMessage == "" {
		t.Errorf("attachment fetch error: failing tests = %+v", out.FailingTests)
	}
}

// TestDiagnoseFailure_Dedupe guards the live bug where 8 real failures came
// back as examined_tests=16: Playwright records each test twice, as an
// enriched row ("md5:md5") and an empty shell ("md5.md5"). The fetch budget,
// the collapse, and truncation must all count tests, not rows.
func TestDiagnoseFailure_Dedupe(t *testing.T) {
	twin := func(fullName, shell, enriched, msg string) []store.TestResult {
		return []store.TestResult{
			{BuildID: 100, HistoryID: shell, FullName: fullName, Status: "failed"},
			{BuildID: 100, HistoryID: enriched, FullName: fullName, Status: "failed", StatusMessage: msg},
		}
	}
	tests := []struct {
		name          string
		rows          []store.TestResult
		distinct      int
		maxTests      int
		wantLimit     int
		wantExamined  int
		wantTruncated int
		wantMerged    map[string][]string // survivor history_id -> merged ids
		wantWarnings  []string
	}{
		{name: "truncation is measured against the distinct failing count",
			rows: []store.TestResult{failed("ha"), failed("hb"), failed("hc"), failed("hd")}, distinct: 4, maxTests: 3,
			wantLimit: 8, wantExamined: 3, wantTruncated: 1},
		// Asking for max_tests+1 rows would leave only half as many tests
		// after the collapse; the handler asks for 2*max_tests+2.
		{name: "fetch budget survives twin rows",
			rows:     slices.Concat(twin("pkg.A", "A.dup", "A:real", "boom"), twin("pkg.B", "B.dup", "B:real", "boom"), twin("pkg.C", "C.dup", "C:real", "boom")),
			distinct: 3, maxTests: 3, wantLimit: 8, wantExamined: 3,
			wantMerged:   map[string][]string{"A:real": {"A.dup"}, "B:real": {"B.dup"}, "C:real": {"C.dup"}},
			wantWarnings: []string{"3 duplicate history_id rows merged by full_name"}},
		// The shell comes first, so ordering cannot be what picks the survivor.
		{name: "twins collapse onto the enriched row",
			rows:     slices.Concat(twin("spec/login.ts > login", "abc.abc", "abc:abc", "expected 200, got 500"), twin("spec/cart.ts > cart", "def.def", "def:def", "timeout")),
			distinct: 2, wantLimit: 42, wantExamined: 2,
			wantMerged:   map[string][]string{"abc:abc": {"abc.abc"}, "def:def": {"def.def"}},
			wantWarnings: []string{"2 duplicate history_id rows merged by full_name"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newDiagFixture(t, tc.rows...)
			gotLimit := 0
			f.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
				gotLimit = limit
				return tc.rows[:min(limit, len(tc.rows))], nil
			}
			f.TestResults.CountFailedByBuildFn = func(context.Context, int64, int64) (int, error) { return tc.distinct, nil }
			extra := map[string]any{}
			if tc.maxTests > 0 {
				extra["max_tests"] = tc.maxTests
			}
			out, _ := f.run(t, extra)

			if gotLimit != tc.wantLimit || out.ExaminedTests != tc.wantExamined || out.TruncatedCount != tc.wantTruncated || out.Truncated != (tc.wantTruncated > 0) {
				t.Errorf("limit %d, examined %d, truncated %v/%d; want %d, %d, %d", gotLimit, out.ExaminedTests,
					out.Truncated, out.TruncatedCount, tc.wantLimit, tc.wantExamined, tc.wantTruncated)
			}
			merged := map[string][]string{}
			for _, d := range out.FailingTests {
				if d.MergedHistoryIDs != nil {
					merged[d.HistoryID] = d.MergedHistoryIDs
				}
			}
			if tc.wantMerged == nil {
				tc.wantMerged = map[string][]string{}
			}
			if !reflect.DeepEqual(merged, tc.wantMerged) || !reflect.DeepEqual(out.Warnings, tc.wantWarnings) {
				t.Errorf("merged = %v, warnings = %q; want %v, %q", merged, out.Warnings, tc.wantMerged, tc.wantWarnings)
			}
		})
	}
}

// TestDiagnoseFailure_LastGood covers the last-good pointer and its
// whole-build diff. The pointer is branch-scoped and bounded by the build's
// build_order (never its id: ids follow ingestion order). The comparison is
// memoized per distinct last-good build, so its counts are always emitted
// while the full lists stay behind include_last_good_diff. Every store
// failure degrades the context rather than failing the diagnosis.
func TestDiagnoseFailure_LastGood(t *testing.T) {
	good := &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed", CICommitSHA: new("goodsha")}
	diffs := []store.DiffEntry{
		{TestName: "pkg.h1", FullName: "pkg.h1", HistoryID: "h1", StatusA: "passed", StatusB: "failed", DurationA: 900, DurationB: 1500, Category: store.DiffRegressed},
		{FullName: "pkg.hCo", HistoryID: "hCo", StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
		{FullName: "pkg.hFix", HistoryID: "hFix", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
		{FullName: "pkg.hNew", HistoryID: "hNew", StatusB: "failed", Category: store.DiffAdded},
	}
	history := func(orders ...int) []store.TestHistoryEntry {
		var h []store.TestHistoryEntry
		for _, o := range orders {
			id := int64(1000 + o)
			if o == 28 {
				id = 100 // the diagnosed build itself
			}
			h = append(h, store.TestHistoryEntry{BuildID: id, BuildNumber: o, Status: "failed"})
		}
		return h
	}
	type calls struct {
		lastGoodBranches []*int64
		before           int
		historyBranch    *int64
		compares         [][2]int64
	}
	branch3, branch7 := int64(3), int64(7)

	tests := []struct {
		name        string
		branchID    *int64
		buildNumber int
		failing     []string
		history     []store.TestHistoryEntry
		lastGood    func(branch *int64) (*store.TestHistoryEntry, error)
		compareErr  error
		include     bool
		check       func(t *testing.T, out tools.DiagnoseFailureOutput, c *calls)
	}{
		{
			name: "branch-scoped pointer with build_order bound, builds_since and counts", branchID: &branch3, buildNumber: 28,
			failing: []string{"h1"}, history: history(28, 27, 26, 25),
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return good, nil },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, c *calls) {
				d := out.FailingTests[0]
				if len(c.lastGoodBranches) != 1 || c.lastGoodBranches[0] == nil || *c.lastGoodBranches[0] != 3 || c.before != 28 {
					t.Errorf("GetLastPassingBuild(branches %v, before %d), want (3, build_order 28)", c.lastGoodBranches, c.before)
				}
				if c.historyBranch == nil || *c.historyBranch != 3 {
					t.Errorf("history branch = %v, want the build's branch 3", c.historyBranch)
				}
				// Builds strictly between last-good #25 and current #28: #27, #26.
				if want := (failure.LastGood{BuildID: 80, BuildNumber: 25, CommitSHA: "goodsha", CreatedAt: "0001-01-01T00:00:00Z", BuildsSince: 2}); d.LastGood == nil || *d.LastGood != want {
					t.Errorf("last_good = %+v, want %+v", d.LastGood, want)
				}
				if d.LastGoodDiffCounts == nil || *d.LastGoodDiffCounts != (tools.LastGoodDiffCounts{Regressed: 2, Fixed: 1, Added: 1}) || d.LastGoodDiff != nil {
					t.Errorf("counts = %+v, diff = %+v; want {2 1 1} and no lists without the flag", d.LastGoodDiffCounts, d.LastGoodDiff)
				}
				if !reflect.DeepEqual(c.compares, [][2]int64{{80, 100}}) {
					t.Errorf("comparisons = %v, want one (last-good 80, current 100)", c.compares)
				}
			},
		},
		{
			name: "flag expands the counts into the full diff", buildNumber: 28, failing: []string{"h1"}, include: true,
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return good, nil },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, _ *calls) {
				diff := out.FailingTests[0].LastGoodDiff
				if diff == nil {
					t.Fatal("last_good_diff is nil with include_last_good_diff set")
				}
				if diff.FromBuildID != 80 || diff.ToBuildID != 100 || diff.RegressedCount != 2 || diff.FixedCount != 1 {
					t.Errorf("diff = %+v, want 80->100 with 2 regressed, 1 fixed", diff)
				}
				if th := diff.ThisTest; th.HistoryID != "h1" || th.StatusFrom != "passed" || th.StatusTo != "failed" || th.DurationDelta != 600 {
					t.Errorf("this_test = %+v, want h1 passed->failed, +600ms", th)
				}
				// The sample excludes the diagnosed test itself.
				if len(diff.SampleRegressed) != 1 || diff.SampleRegressed[0].HistoryID != "hCo" {
					t.Errorf("sample_regressed = %+v, want [hCo]", diff.SampleRegressed)
				}
			},
		},
		{
			name: "tests sharing a last-good build share one comparison", buildNumber: 28, failing: []string{"h1", "hCo"}, include: true,
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return good, nil },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, c *calls) {
				if len(c.compares) != 1 {
					t.Errorf("comparisons = %v, want 1 for two tests sharing last-good 80", c.compares)
				}
				for _, d := range out.FailingTests {
					if d.LastGoodDiff == nil || d.LastGoodDiff.ThisTest.HistoryID != d.HistoryID {
						t.Errorf("%s: this_test resolved from the shared diff = %+v", d.HistoryID, d.LastGoodDiff)
					}
				}
			},
		},
		// The regression: GetTestHistory's window is the latest builds overall,
		// so diagnosing a non-latest build sees NEWER runs (#20, #18, #16) that
		// must not count. Only #12 lies between last-good #10 and current #15.
		{
			name: "builds_since is bounded by the diagnosed build", buildNumber: 15, failing: []string{"h1"},
			history: []store.TestHistoryEntry{
				{BuildID: 1020, BuildNumber: 20, Status: "passed"}, {BuildID: 1018, BuildNumber: 18, Status: "failed"},
				{BuildID: 1016, BuildNumber: 16, Status: "failed"}, {BuildID: 100, BuildNumber: 15, Status: "failed"},
				{BuildID: 1012, BuildNumber: 12, Status: "failed"}, {BuildID: 1010, BuildNumber: 10, Status: "passed"},
			},
			lastGood: func(*int64) (*store.TestHistoryEntry, error) {
				return &store.TestHistoryEntry{BuildID: 1010, BuildNumber: 10, Status: "passed"}, nil
			},
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, _ *calls) {
				if lg := out.FailingTests[0].LastGood; lg == nil || lg.BuildsSince != 1 {
					t.Errorf("last_good = %+v, want builds_since 1", lg)
				}
			},
		},
		{
			name: "never passed, no branch: no pointer, no counts, no comparison", buildNumber: 28, failing: []string{"h1"},
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return nil, nil },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, c *calls) {
				d := out.FailingTests[0]
				if d.LastGood != nil || d.LastGoodDiffCounts != nil || d.LastGoodAbsentReason != "" || d.CrossBranchLastGood != nil || len(c.compares) != 0 {
					t.Errorf("entry = %+v after %d comparisons, want no last-good context", d, len(c.compares))
				}
			},
		},
		// An absent last_good would read as "not investigated"; it is
		// explained, and ONE cross-branch lookup supplies a weaker anchor.
		{
			name: "never passed on its branch: reason plus one cross-branch lookup", branchID: &branch7, buildNumber: 28, failing: []string{"h1"},
			lastGood: func(branch *int64) (*store.TestHistoryEntry, error) {
				if branch != nil {
					return nil, nil
				}
				return &store.TestHistoryEntry{BuildID: 55, BuildNumber: 20, Status: "passed", BranchName: "main",
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), CICommitSHA: new("deadbee")}, nil
			},
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, c *calls) {
				d := out.FailingTests[0]
				if d.LastGood != nil || d.LastGoodAbsentReason != "no passing run on branch feature/x in recorded history" {
					t.Errorf("last_good = %+v, reason = %q", d.LastGood, d.LastGoodAbsentReason)
				}
				if len(c.lastGoodBranches) != 2 || c.lastGoodBranches[1] != nil {
					t.Errorf("GetLastPassingBuild branches = %v, want [7, nil]", c.lastGoodBranches)
				}
				want := tools.CrossBranchLastGood{Branch: "main", BuildID: 55, BuildNumber: 20, CommitSHA: "deadbee", CreatedAt: "2026-01-02T03:04:05Z"}
				if d.CrossBranchLastGood == nil || *d.CrossBranchLastGood != want {
					t.Errorf("cross_branch_last_good = %+v, want %+v", d.CrossBranchLastGood, want)
				}
			},
		},
		{
			name: "pointer lookup failure degrades to nil", buildNumber: 28, failing: []string{"h1"},
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return nil, context.DeadlineExceeded },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, _ *calls) {
				if lg := out.FailingTests[0].LastGood; lg != nil {
					t.Errorf("last_good = %+v, want nil", lg)
				}
			},
		},
		{
			name: "comparison failure keeps the pointer", buildNumber: 28, failing: []string{"h1"}, include: true, compareErr: context.DeadlineExceeded,
			lastGood: func(*int64) (*store.TestHistoryEntry, error) { return good, nil },
			check: func(t *testing.T, out tools.DiagnoseFailureOutput, _ *calls) {
				if d := out.FailingTests[0]; d.LastGood == nil || d.LastGoodDiff != nil || d.LastGoodDiffCounts != nil {
					t.Errorf("entry = %+v, want the pointer without diff or counts", d)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rows []store.TestResult
			for _, id := range tc.failing {
				rows = append(rows, failed(id))
			}
			f := newDiagFixture(t, rows...)
			f.build.BuildNumber, f.build.BranchID = tc.buildNumber, tc.branchID
			if tc.branchID != nil {
				f.build.CIBranch = new("feature/x")
			}
			c := &calls{}
			f.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, _ string, branchID *int64, _ int) ([]store.TestHistoryEntry, error) {
				c.historyBranch = branchID
				return tc.history, nil
			}
			f.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, branchID *int64, before int) (*store.TestHistoryEntry, error) {
				c.lastGoodBranches = append(c.lastGoodBranches, branchID)
				c.before = before
				return tc.lastGood(branchID)
			}
			f.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, a, b int64) ([]store.DiffEntry, error) {
				c.compares = append(c.compares, [2]int64{a, b})
				return diffs, tc.compareErr
			}
			out, _ := f.run(t, map[string]any{"include_last_good_diff": tc.include})
			tc.check(t, out, c)
		})
	}
}

// Two real error messages from the report that motivated clustering: four
// failures share a TokenAuth 500 raised in beforeEach, three share one toast
// assertion — previously a flat list of seven equal-looking entries.
const (
	tokenAuth500   = "Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate"
	toastAssertion = "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')"
)

// TestDiagnoseFailure_TwoRootCauses is the end-to-end guard for the live
// complaint: seven failures with two root causes come back as two clusters
// carrying the duplicated text once, a build verdict rolled up from the
// dominant cluster, and a headline digest instead of a second payload.
func TestDiagnoseFailure_TwoRootCauses(t *testing.T) {
	var rows []store.TestResult
	for _, h := range []string{"h1", "h2", "h3", "h4"} {
		rows = append(rows, store.TestResult{BuildID: 100, HistoryID: h, FullName: "spec/auth-" + h + ".ts > login", Status: "failed", DurationMs: 120, StatusMessage: tokenAuth500})
	}
	for _, h := range []string{"t1", "t2", "t3"} {
		rows = append(rows, store.TestResult{BuildID: 100, HistoryID: h, FullName: "spec/save-" + h + ".ts > save", Status: "failed", DurationMs: 5200, StatusMessage: toastAssertion})
	}
	f := newDiagFixture(t, rows...)
	f.TestResults.GetFailedStepPathFn = func(_ context.Context, _, _ int64, historyID string) ([]string, string, error) {
		if strings.HasPrefix(historyID, "h") {
			return []string{"Before Hooks", "login via API"}, tokenAuth500, nil
		}
		return []string{"Test Body", "check toast"}, toastAssertion, nil
	}
	// Every test last passed on the diagnosed build's own commit.
	f.TestResults.GetLastPassingBuildFn = func(context.Context, int64, string, *int64, int) (*store.TestHistoryEntry, error) {
		return &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed", CICommitSHA: new("abc123")}, nil
	}
	count := 7
	f.TestResults.CountFailedByBuildFn = func(context.Context, int64, int64) (int, error) { return count, nil }

	out, res := f.run(t, nil)
	if len(out.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2 (%+v)", len(out.Clusters), out.Clusters)
	}
	auth := out.Clusters[0]
	if auth.ClusterID != "c1" || auth.MemberCount != 4 || auth.FailurePhase != triage.PhaseBeforeHooks || auth.SharedError != tokenAuth500 ||
		auth.SharedStatusPattern == nil || auth.SharedStatusPattern.StatusCode != 500 || out.Clusters[1].ClusterID != "c2" || out.Clusters[1].MemberCount != 3 {
		t.Errorf("clusters = %+v", out.Clusters)
	}
	withText := 0
	for _, d := range out.FailingTests {
		if d.ClusterID == "" || d.LastGood == nil || d.Signals.FailurePhase == "" {
			t.Errorf("test %q lost its cluster_id, last_good or signals: %+v", d.FullName, d)
		}
		if d.ErrorMessage != "" {
			withText++
		}
	}
	if withText != 2 {
		t.Errorf("tests keeping error_message = %d, want 2 (one representative per cluster)", withText)
	}
	v := out.BuildVerdict
	if v.Verdict != tools.VerdictInfraEnv || v.Confidence != tools.ConfidenceMedium || v.RecommendedAction != tools.ActionRerun ||
		v.AffectedTestCount != 4 || len(v.Evidence) == 0 || len(v.EvidenceGaps) != 0 {
		t.Errorf("verdict = %+v, want infra_env/medium (clusters disagree)/rerun over the 4-test cluster", v)
	}
	digest := textOf(t, res)
	for _, want := range []string{"build #28 (main)", "7 failing in 2 clusters", "verdict: infra_env (medium)", "4x status 500", "in before_hooks", "/projects/"} {
		if !strings.Contains(digest, want) {
			t.Errorf("digest %q is missing %q", digest, want)
		}
	}
	if len(res.Content) != 1 || len(digest) >= 200 {
		t.Errorf("digest = %d block(s), %d bytes; want one headline under 200 bytes, not a payload copy", len(res.Content), len(digest))
	}

	// A diagnosis that did not see the whole build cannot be confident about it.
	count = 40
	out, _ = f.run(t, nil)
	if !out.Truncated || out.BuildVerdict.Confidence == tools.ConfidenceHigh || len(out.BuildVerdict.EvidenceGaps) == 0 {
		t.Errorf("truncated diagnosis: truncated=%v verdict=%+v, want a recorded gap and no high confidence", out.Truncated, out.BuildVerdict)
	}
}

// TestDiagnoseFailure_KnownIssueRegexMatchesOnCluster: the project's active
// rules are matched against each cluster's shared error and reported on the
// CLUSTER, distinct from the per-test human-confirmed FK. A broken pattern is
// skipped rather than failing the call.
func TestDiagnoseFailure_KnownIssueRegexMatchesOnCluster(t *testing.T) {
	f := newDiagFixture(t, failed("h1"))
	for name, pattern := range map[string]string{"auth-service-flap": `TokenAuth/Authenticate`, "broken-rule": `([unclosed`, "never-matches": `will-not-match`} {
		if _, err := f.KnownIssues.Create(context.Background(), f.build.ProjectID, name, pattern, "", ""); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	f.TestResults.GetFailedStepPathFn = func(context.Context, int64, int64, string) ([]string, string, error) {
		return []string{"Test Body", "call api"}, tokenAuth500, nil
	}

	out, _ := f.run(t, nil)
	if len(out.Clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(out.Clusters))
	}
	if m := out.Clusters[0].KnownIssueRegexMatches; len(m) != 1 || m[0].Name != "auth-service-flap" || m[0].MatchedSubstring != "TokenAuth/Authenticate" {
		t.Errorf("known_issue_regex_matches = %+v, want only auth-service-flap", m)
	}
	if out.FailingTests[0].KnownIssue != nil {
		t.Errorf("a regex-only match must not set the per-test known_issue: %+v", out.FailingTests[0].KnownIssue)
	}
	if out.BuildVerdict.Verdict != tools.VerdictKnownIssue || out.BuildVerdict.RecommendedAction != tools.ActionLinkKnownIssue {
		t.Errorf("verdict = %+v, want known_issue / link_known_issue", out.BuildVerdict)
	}
}

func TestDiagnoseFailure_InvalidTarget(t *testing.T) {
	f := newDiagFixture(t)
	cs := setupTestServer(t, &bootstrap.Stores{Project: f.Projects, Build: f.Builds, TestResult: f.TestResults})
	callErr(t, cs, "diagnose_failure", map[string]any{})
	callErr(t, cs, "diagnose_failure", map[string]any{"project_id": f.build.ProjectID, "build_id": 999})
}
