package tools_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// Run links point at the UI page /runs/<group project id>/<run key>. The group
// is the parent of the build's suite project and the key is the pipeline id,
// else the commit SHA: the same key the runs feed and the single-run endpoint
// use, so a link from an agent opens the page the feed row would.
const (
	runURLBase       = "https://deck.example.com"
	runURLGroupID    = 3
	runURLSuiteID    = 5
	runURLStandalone = 9
)

// runURLStores is a group (3) with one suite (5) and a standalone project (9).
// Build numbers select the CI metadata: 1 has a pipeline id, 2 only a commit
// SHA, 3 a pipeline id that needs escaping, 4 no CI metadata at all.
func runURLStores() *bootstrap.Stores {
	projects := &testutil.MockProjectStore{GetProjectFn: func(_ context.Context, id int64) (*store.Project, error) {
		p := &store.Project{ID: id, Slug: "suite"}
		if id == runURLSuiteID {
			p.ParentID = new(int64(runURLGroupID))
		}
		return p, nil
	}}
	builds := &testutil.MockBuildStore{GetBuildByNumberFn: func(_ context.Context, projectID int64, n int) (store.Build, error) {
		b := store.Build{ID: projectID*1000 + int64(n), ProjectID: projectID, BuildNumber: n}
		switch n {
		case 1:
			b.CIPipelineID, b.CICommitSHA = new("196765"), new("abc123")
		case 2:
			b.CICommitSHA = new("abc123")
		case 3:
			b.CIPipelineID = new("feat/x 1")
		}
		return b, nil
	}}
	return &bootstrap.Stores{Project: projects, Build: builds}
}

// connectWithURL serves every tool with publicURL as the external base URL.
func connectWithURL(t *testing.T, stores *bootstrap.Stores, publicURL string) *mcpsdk.ClientSession {
	t.Helper()
	return connect(t, func(s *mcpsdk.Server) { tools.RegisterAll(s, stores, zap.NewNop(), publicURL, nil) })
}

// TestResolveURL_RunURL: resolve_url hands back the run page next to the
// report page, absolute when the server knows its external URL.
func TestResolveURL_RunURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		project   int
		build     int
		want      string
	}{
		{name: "pipeline id keys the run", publicURL: runURLBase, project: runURLSuiteID, build: 1, want: runURLBase + "/runs/3/196765"},
		{name: "commit sha keys a run without a pipeline id", publicURL: runURLBase, project: runURLSuiteID, build: 2, want: runURLBase + "/runs/3/abc123"},
		{name: "key is path-escaped", publicURL: runURLBase, project: runURLSuiteID, build: 3, want: runURLBase + "/runs/3/feat%2Fx%201"},
		{name: "trailing slash on the external URL", publicURL: runURLBase + "/", project: runURLSuiteID, build: 1, want: runURLBase + "/runs/3/196765"},
		// Without an external URL the link stays relative, as the report link does.
		{name: "relative without an external URL", project: runURLSuiteID, build: 1, want: "/runs/3/196765"},
		{name: "build without CI metadata has no run", publicURL: runURLBase, project: runURLSuiteID, build: 4, want: ""},
		// Runs are per group: a standalone project's build belongs to none.
		{name: "standalone project has no run", publicURL: runURLBase, project: runURLStandalone, build: 1, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := connectWithURL(t, runURLStores(), tc.publicURL)
			out, _ := call[tools.ResolveURLOutput](t, cs, "resolve_url", map[string]any{"project_ref": strconv.Itoa(tc.project), "build_number": tc.build})
			if out.RunURL != tc.want {
				t.Errorf("run_url = %q, want %q", out.RunURL, tc.want)
			}
			if want := fmt.Sprintf("/projects/%d/reports/%d", tc.project, tc.build); out.ReportURL != want {
				t.Errorf("report_url = %q, want %q (unchanged)", out.ReportURL, want)
			}
		})
	}
}

// TestDiagnosePipeline_RunURL: the tool is keyed by pipeline id, so the run
// page is that id under the suite's group.
func TestDiagnosePipeline_RunURL(t *testing.T) {
	stores := runURLStores()
	pipeline := &testutil.MockPipelineStore{ListBuildsByPipelineIDFn: func(_ context.Context, projectID int64, _ string, _ int) ([]store.Build, error) {
		return []store.Build{{ID: 10, ProjectID: projectID, BuildNumber: 1, CIPipelineID: new("196765")}}, nil
	}}
	stores.Pipeline, stores.TestResult = pipeline, testutil.New().TestResults

	for _, tc := range []struct {
		name    string
		project int
		want    string
	}{
		{"suite of a group", runURLSuiteID, runURLBase + "/runs/3/196765"},
		{"standalone project", runURLStandalone, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := connectWithURL(t, stores, runURLBase)
			out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline", map[string]any{"project_id": tc.project, "pipeline_id": "196765"})
			if out.RunURL != tc.want {
				t.Errorf("run_url = %q, want %q", out.RunURL, tc.want)
			}
		})
	}
}

// TestDiagnoseFailure_RunURL: the build header links the run the build is part
// of, falling back to the commit SHA like resolve_url.
func TestDiagnoseFailure_RunURL(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pipelineID *string
		wantKey    string
	}{
		{"pipeline id", new("196765"), "196765"},
		{"commit sha", nil, "abc123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDiagFixture(t, failed("h1"))
			group, err := f.Projects.CreateProject(context.Background(), "group")
			if err != nil {
				t.Fatal(err)
			}
			suite, err := f.Projects.CreateProjectWithParent(context.Background(), "suite", group.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.build.ProjectID, f.build.CIPipelineID = suite.ID, tc.pipelineID
			stores := &bootstrap.Stores{Project: f.Projects, Build: f.Builds, TestResult: f.TestResults,
				Attachment: f.Attachments, Defect: f.Defects, KnownIssue: f.KnownIssues}
			cs := connectWithURL(t, stores, runURLBase)
			out, _ := call[tools.DiagnoseFailureOutput](t, cs, "diagnose_failure", map[string]any{"project_id": suite.ID, "build_id": f.build.ID})
			want := fmt.Sprintf("%s/runs/%d/%s", runURLBase, group.ID, tc.wantKey)
			if out.Build.RunURL != want {
				t.Errorf("run_url = %q, want %q", out.Build.RunURL, want)
			}
		})
	}
}

// projectLookups counts GetProject calls, so a test can pin that a tool pays
// for at most one project lookup and none when there is nothing to link.
func projectLookups(stores *bootstrap.Stores) *int {
	n := new(int)
	get := stores.Project.(*testutil.MockProjectStore).GetProjectFn
	stores.Project.(*testutil.MockProjectStore).GetProjectFn = func(ctx context.Context, id int64) (*store.Project, error) {
		*n++
		return get(ctx, id)
	}
	return n
}

// TestListRecentBuilds_RunURL: every build of a page links its own run, from a
// single project lookup for the whole page, and a page of builds that name no
// run costs no lookup at all.
func TestListRecentBuilds_RunURL(t *testing.T) {
	page := []store.Build{
		{ID: 1, BuildNumber: 3, CIPipelineID: new("196765"), CICommitSHA: new("abc123")},
		{ID: 2, BuildNumber: 2, CICommitSHA: new("def456")},
		{ID: 3, BuildNumber: 1},
	}
	for _, tc := range []struct {
		name        string
		project     int
		builds      []store.Build
		want        []string
		wantLookups int
	}{
		{"suite of a group", runURLSuiteID, page, []string{runURLBase + "/runs/3/196765", runURLBase + "/runs/3/def456", ""}, 1},
		{"standalone project", runURLStandalone, page, []string{"", "", ""}, 1},
		{"builds without CI metadata", runURLSuiteID, page[2:], []string{""}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stores := runURLStores()
			stores.Build = &testutil.MockBuildStore{ListBuildsPaginatedBranchFn: func(context.Context, int64, int, int, *int64) ([]store.Build, int, error) {
				return tc.builds, len(tc.builds), nil
			}}
			lookups := projectLookups(stores)
			cs := connectWithURL(t, stores, runURLBase)
			out, _ := call[tools.ListRecentBuildsOutput](t, cs, "list_recent_builds", map[string]any{"project_id": tc.project})
			if len(out.Items) != len(tc.want) {
				t.Fatalf("items = %d, want %d", len(out.Items), len(tc.want))
			}
			for i, item := range out.Items {
				if item.RunURL != tc.want[i] {
					t.Errorf("item %d run_url = %q, want %q", i, item.RunURL, tc.want[i])
				}
			}
			if *lookups != tc.wantLookups {
				t.Errorf("project lookups = %d, want %d", *lookups, tc.wantLookups)
			}
		})
	}
}

// TestListFailingTests_RunURL: the build header carries the run link.
func TestListFailingTests_RunURL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		project     int
		build       store.Build
		want        string
		wantLookups int
	}{
		{"pipeline id", runURLSuiteID, store.Build{ID: 7, BuildNumber: 4, CIPipelineID: new("196765")}, runURLBase + "/runs/3/196765", 1},
		{"commit sha", runURLSuiteID, store.Build{ID: 7, BuildNumber: 4, CICommitSHA: new("abc123")}, runURLBase + "/runs/3/abc123", 1},
		{"standalone project", runURLStandalone, store.Build{ID: 7, BuildNumber: 4, CIPipelineID: new("196765")}, "", 1},
		{"no run key, no lookup", runURLSuiteID, store.Build{ID: 7, BuildNumber: 4}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stores := runURLStores()
			stores.Build = &testutil.MockBuildStore{GetBuildByIDFn: func(context.Context, int64, int64) (store.Build, error) { return tc.build, nil }}
			stores.TestResult = testutil.New().TestResults
			lookups := projectLookups(stores)
			cs := connectWithURL(t, stores, runURLBase)
			out, _ := call[tools.ListFailingTestsOutput](t, cs, "list_failing_tests", map[string]any{"project_id": tc.project, "build_id": 7})
			if out.Build == nil || out.Build.RunURL != tc.want {
				t.Errorf("build = %+v, want run_url %q", out.Build, tc.want)
			}
			if *lookups != tc.wantLookups {
				t.Errorf("project lookups = %d, want %d", *lookups, tc.wantLookups)
			}
		})
	}
}

// TestCompareBuilds_RunURL: the target build's header carries the run link.
func TestCompareBuilds_RunURL(t *testing.T) {
	stores := runURLStores()
	stores.Build = &testutil.MockBuildStore{GetBuildByIDFn: func(_ context.Context, _, id int64) (store.Build, error) {
		return store.Build{ID: id, BuildNumber: int(id), CIPipelineID: new("196765")}, nil
	}}
	stores.TestResult = testutil.New().TestResults
	for _, tc := range []struct {
		name    string
		project int
		want    string
	}{
		{"suite of a group", runURLSuiteID, runURLBase + "/runs/3/196765"},
		{"standalone project", runURLStandalone, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := connectWithURL(t, stores, runURLBase)
			out, _ := call[tools.CompareBuildsOutput](t, cs, "compare_builds", map[string]any{"project_id": tc.project, "base_build_id": 10, "target_build_id": 20})
			if out.Build == nil || out.Build.RunURL != tc.want {
				t.Errorf("build = %+v, want run_url %q", out.Build, tc.want)
			}
		})
	}
}

// TestRunURL_ProjectLookupFailureKeepsTheResult: run_url is a convenience
// link, so a project store that fails costs the link, never the tool result.
// The failure is logged so the missing link is explainable.
func TestRunURL_ProjectLookupFailureKeepsTheResult(t *testing.T) {
	build := store.Build{ID: 7, BuildNumber: 4, CIPipelineID: new("196765")}
	stores := &bootstrap.Stores{
		Project: &testutil.MockProjectStore{GetProjectFn: func(context.Context, int64) (*store.Project, error) {
			return nil, errors.New("db connection reset")
		}},
		Build: &testutil.MockBuildStore{
			ListBuildsPaginatedBranchFn: func(context.Context, int64, int, int, *int64) ([]store.Build, int, error) {
				return []store.Build{build}, 1, nil
			},
			GetBuildByIDFn: func(context.Context, int64, int64) (store.Build, error) { return build, nil },
		},
		TestResult: testutil.New().TestResults,
	}
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, cs *mcpsdk.ClientSession) (buildNumber int, runURL string)
	}{
		{"list_recent_builds", func(t *testing.T, cs *mcpsdk.ClientSession) (int, string) {
			out, _ := call[tools.ListRecentBuildsOutput](t, cs, "list_recent_builds", map[string]any{"project_id": runURLSuiteID})
			if len(out.Items) != 1 {
				t.Fatalf("items = %+v, want the one build", out.Items)
			}
			return out.Items[0].BuildNumber, out.Items[0].RunURL
		}},
		{"list_failing_tests", func(t *testing.T, cs *mcpsdk.ClientSession) (int, string) {
			out, _ := call[tools.ListFailingTestsOutput](t, cs, "list_failing_tests", map[string]any{"project_id": runURLSuiteID, "build_id": 7})
			if out.Build == nil {
				t.Fatal("no build header")
			}
			return out.Build.BuildNumber, out.Build.RunURL
		}},
		{"compare_builds", func(t *testing.T, cs *mcpsdk.ClientSession) (int, string) {
			out, _ := call[tools.CompareBuildsOutput](t, cs, "compare_builds", map[string]any{"project_id": runURLSuiteID, "base_build_id": 6, "target_build_id": 7})
			if out.Build == nil {
				t.Fatal("no build header")
			}
			return out.Build.BuildNumber, out.Build.RunURL
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.WarnLevel)
			cs := connect(t, func(s *mcpsdk.Server) {
				tools.RegisterAll(s, stores, zap.New(core), runURLBase, nil)
			})
			number, runURL := tc.run(t, cs)
			if number != build.BuildNumber || runURL != "" {
				t.Errorf("build #%d run_url %q, want the result for #%d with no run_url", number, runURL, build.BuildNumber)
			}
			if logs.FilterField(zap.Error(errors.New("db connection reset"))).Len() != 1 {
				t.Errorf("logged %v, want one warning carrying the project lookup error", logs.All())
			}
		})
	}
}
