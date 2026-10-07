package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// restFailure is the part of a run-failures row (GET
// /projects/{id}/pipeline-runs/{run_key}/failures) the agreement test compares.
type restFailure struct {
	ProjectID   int64  `json:"project_id"`
	FullName    string `json:"full_name"`
	Origin      string `json:"origin"`
	Environment bool   `json:"environment"`
}

// TestDiagnosePipeline_OriginsAgreeWithRESTFailures: what the UI reads from the
// run failures endpoint and what an agent reads from diagnose_pipeline are the
// same facts. One group run is served both ways from the same rows: the REST
// handler over the whole group (a sibling suite's failures included), the tool
// over the one suite it is asked about. The summary must equal the REST rows of
// that suite counted by the runs page's bucket rule (an environment failure
// counts as inherited, and as environment), and every failing test the tool
// lists must carry the tags its REST row has. Build 10 holds three failures
// with max_tests_per_build=2, so the summary also has to cover tests the tool
// does not list.
func TestDiagnosePipeline_OriginsAgreeWithRESTFailures(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()
	group, _ := mocks.Projects.CreateProject(ctx, "group")
	web, _ := mocks.Projects.CreateProjectWithParent(ctx, "web", group.ID)
	api, _ := mocks.Projects.CreateProjectWithParent(ctx, "api", group.ID)

	const runKey = "pipe-789"
	row := func(p *store.Project, buildID int64, number int, id int64, name, origin string, env bool) store.RunFailureRow {
		return store.RunFailureRow{
			ProjectID: p.ID, Slug: p.Slug, BuildID: buildID, BuildNumber: number, TestResultID: id,
			TestName: name, FullName: p.Slug + "." + name, HistoryID: "h-" + name, Status: "failed",
			DurationMs: 1000 - id, Origin: origin, Environment: env, StatusMessage: "expected " + name,
		}
	}
	webRows := []store.RunFailureRow{
		row(web, 10, 1, 1, "login", store.FailureOriginNew, false),
		row(web, 10, 1, 2, "checkout", store.FailureOriginInherited, false),
		row(web, 10, 1, 3, "search", store.FailureOriginUnknown, true),
		row(web, 11, 2, 4, "signup", store.FailureOriginNew, true),
		row(web, 11, 2, 5, "profile", store.FailureOriginInherited, false),
	}
	groupRows := append(append([]store.RunFailureRow{}, webRows...), row(api, 20, 1, 6, "health", store.FailureOriginNew, false))

	mocks.Pipeline.ListRunFailuresFn = func(_ context.Context, groupID int64, key string, _ int) ([]store.RunFailureRow, error) {
		if groupID != group.ID || key != runKey {
			return nil, nil
		}
		return groupRows, nil
	}
	mocks.Pipeline.ListFailuresByPipelineIDFn = func(_ context.Context, projectID int64, key string, _ int) ([]store.RunFailureRow, error) {
		if projectID != web.ID || key != runKey {
			return nil, nil
		}
		return webRows, nil
	}
	mocks.Pipeline.ListBuildsByPipelineIDFn = func(_ context.Context, projectID int64, key string, _ int) ([]store.Build, error) {
		if projectID != web.ID || key != runKey {
			return nil, nil
		}
		shard := func(id int64, number int) store.Build {
			return store.Build{ID: id, ProjectID: web.ID, BuildNumber: number, CIPipelineID: new(runKey), CIBranch: new("feature")}
		}
		return []store.Build{shard(10, 1), shard(11, 2)}, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, buildID int64, _ int) ([]store.TestResult, error) {
		var out []store.TestResult
		for i := range webRows {
			if r := &webRows[i]; r.BuildID == buildID {
				out = append(out, store.TestResult{ID: r.TestResultID, BuildID: r.BuildID, ProjectID: r.ProjectID, FullName: r.FullName,
					HistoryID: r.HistoryID, Status: r.Status, DurationMs: r.DurationMs, StatusMessage: r.StatusMessage})
			}
		}
		return out, nil
	}

	// REST: the handler over the whole group.
	ph := handlers.NewPipelineHandler(mocks.Pipeline, mocks.Projects, mocks.KnownIssues, t.TempDir(), zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+strconv.FormatInt(group.ID, 10)+"/pipeline-runs/"+runKey+"/failures", nil)
	req.SetPathValue("project_id", strconv.FormatInt(group.ID, 10))
	req.SetPathValue("run_key", runKey)
	rec := httptest.NewRecorder()
	ph.GetRunFailures(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetRunFailures status = %d: %s", rec.Code, rec.Body)
	}
	var rest struct {
		Data []restFailure `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rest); err != nil {
		t.Fatalf("decode REST body: %v", err)
	}
	restByName := map[string]restFailure{}
	var want tools.FailureOrigins
	for _, r := range rest.Data {
		restByName[r.FullName] = r
		if r.ProjectID != web.ID {
			continue
		}
		want.Total++
		// The runs page's bucket rule: an environment failure is never new, it
		// counts as inherited and as environment, a subset of inherited.
		switch {
		case r.Environment:
			want.Inherited++
			want.Environment++
		case r.Origin == "new":
			want.New++
		case r.Origin == "inherited":
			want.Inherited++
		case r.Origin == "unknown":
			want.Unknown++
		}
	}
	if want != (tools.FailureOrigins{Total: 5, New: 1, Inherited: 4, Unknown: 0, Environment: 2}) {
		t.Fatalf("REST counts for the suite = %+v, want 5 failures: 1 new, 4 inherited (2 of them environment), 0 unknown", want)
	}

	// MCP: diagnose_pipeline over the one suite.
	cs := setupTestServer(t, &bootstrap.Stores{
		Project: mocks.Projects, Build: mocks.Builds, TestResult: mocks.TestResults, Pipeline: mocks.Pipeline,
	})
	out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline",
		map[string]any{"project_id": web.ID, "pipeline_id": runKey, "max_tests_per_build": 2})

	if out.FailureOrigins == nil || *out.FailureOrigins != want {
		t.Errorf("failure_origins = %+v, want the REST counts %+v", out.FailureOrigins, want)
	}
	listed := 0
	for _, b := range out.Builds {
		for _, ft := range b.FailingTests {
			listed++
			r, ok := restByName[ft.FullName]
			if !ok {
				t.Errorf("%s: listed by diagnose_pipeline but missing from the REST failures", ft.FullName)
				continue
			}
			if ft.Origin != r.Origin || ft.Environment != r.Environment {
				t.Errorf("%s: origin/environment = %q/%v, REST says %q/%v", ft.FullName, ft.Origin, ft.Environment, r.Origin, r.Environment)
			}
		}
	}
	if listed != 4 { // build 10 is capped at 2 of its 3, build 11 lists both
		t.Errorf("listed %d failing tests, want 4", listed)
	}
}

// TestDiagnosePipeline_OriginsUnavailable: the tags are enrichment. When the
// store cannot produce them the diagnosis still comes back, without tags or a
// summary that would read as "nothing failed", and says so in warnings.
func TestDiagnosePipeline_OriginsUnavailable(t *testing.T) {
	mocks := testutil.New()
	proj, _ := mocks.Projects.CreateProject(context.Background(), "solo")
	mocks.Pipeline.ListBuildsByPipelineIDFn = func(context.Context, int64, string, int) ([]store.Build, error) {
		return []store.Build{{ID: 1, ProjectID: proj.ID, BuildNumber: 1, CIPipelineID: new("p")}}, nil
	}
	mocks.Pipeline.ListFailuresByPipelineIDFn = func(context.Context, int64, string, int) ([]store.RunFailureRow, error) {
		return nil, context.DeadlineExceeded
	}
	mocks.TestResults.ListFailedByBuildFn = func(context.Context, int64, int64, int) ([]store.TestResult, error) {
		return []store.TestResult{{ID: 1, BuildID: 1, FullName: "t", Status: "failed", StatusMessage: "boom"}}, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{Project: mocks.Projects, Build: mocks.Builds, TestResult: mocks.TestResults, Pipeline: mocks.Pipeline})

	out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline", map[string]any{"project_id": proj.ID, "pipeline_id": "p"})
	if out.FailureOrigins != nil {
		t.Errorf("failure_origins = %+v, want none when the lookup failed", out.FailureOrigins)
	}
	if len(out.Builds) != 1 || len(out.Builds[0].FailingTests) != 1 || out.Builds[0].FailingTests[0].Origin != "" {
		t.Errorf("builds = %+v, want the failing test listed without an origin", out.Builds)
	}
	if len(out.Warnings) != 1 {
		t.Errorf("warnings = %v, want one saying origins are unavailable", out.Warnings)
	}
}

// TestDiagnosePipeline_OriginsTruncated: past the cap on tagged failures the
// counts are lower bounds and failing tests beyond the cap carry no origin.
// The output says both: failure_origins.truncated and a warning that names the
// per-test tags as well as the counts.
func TestDiagnosePipeline_OriginsTruncated(t *testing.T) {
	mocks := testutil.New()
	proj, _ := mocks.Projects.CreateProject(context.Background(), "solo")
	var capRows int // the tool asks for cap+1 rows to detect truncation
	mocks.Pipeline.ListBuildsByPipelineIDFn = func(context.Context, int64, string, int) ([]store.Build, error) {
		return []store.Build{{ID: 1, ProjectID: proj.ID, BuildNumber: 1, CIPipelineID: new("p")}}, nil
	}
	mocks.Pipeline.ListFailuresByPipelineIDFn = func(_ context.Context, _ int64, _ string, limit int) ([]store.RunFailureRow, error) {
		capRows = limit - 1
		rows := make([]store.RunFailureRow, limit) // one more than the cap: the store has more
		for i := range rows {
			rows[i] = store.RunFailureRow{TestResultID: int64(i + 1), Origin: store.FailureOriginNew}
		}
		return rows, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(context.Context, int64, int64, int) ([]store.TestResult, error) {
		return []store.TestResult{
			{ID: 1, BuildID: 1, FullName: "inside the cap", Status: "failed", StatusMessage: "a"},
			{ID: int64(capRows) + 1, BuildID: 1, FullName: "past the cap", Status: "failed", StatusMessage: "b"},
		}, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{Project: mocks.Projects, Build: mocks.Builds, TestResult: mocks.TestResults, Pipeline: mocks.Pipeline})

	out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline", map[string]any{"project_id": proj.ID, "pipeline_id": "p"})
	if out.FailureOrigins == nil || !out.FailureOrigins.Truncated || out.FailureOrigins.Total != capRows || out.FailureOrigins.New != capRows {
		t.Errorf("failure_origins = %+v, want %d rows counted and truncated", out.FailureOrigins, capRows)
	}
	var origins = map[string]string{}
	for _, ft := range out.Builds[0].FailingTests {
		origins[ft.FullName] = ft.Origin
	}
	if origins["inside the cap"] != store.FailureOriginNew || origins["past the cap"] != "" {
		t.Errorf("per-test origins = %v, want new inside the cap and none past it", origins)
	}
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "failing_tests") || !strings.Contains(out.Warnings[0], "origin") {
		t.Errorf("warnings = %v, want one that names the per-test tags as well as the counts", out.Warnings)
	}
}

// TestDiagnosePipeline_EnvironmentFailuresCountAsInherited: an environment
// failure is never "new" for the person reading the summary. It counts under
// inherited whatever its stored origin, and is also counted as environment, a
// subset of inherited, so new + inherited + unknown = total. This is the rule
// the runs page applies to the same rows. The per-test origin and environment
// stay as stored, the way the run failures endpoint serves them.
func TestDiagnosePipeline_EnvironmentFailuresCountAsInherited(t *testing.T) {
	mocks := testutil.New()
	proj, _ := mocks.Projects.CreateProject(context.Background(), "solo")
	rows := []store.RunFailureRow{
		{TestResultID: 1, FullName: "new", Origin: store.FailureOriginNew},
		{TestResultID: 2, FullName: "new+env", Origin: store.FailureOriginNew, Environment: true},
		{TestResultID: 3, FullName: "unknown", Origin: store.FailureOriginUnknown},
		{TestResultID: 4, FullName: "unknown+env", Origin: store.FailureOriginUnknown, Environment: true},
		{TestResultID: 5, FullName: "inherited", Origin: store.FailureOriginInherited},
		{TestResultID: 6, FullName: "inherited+env", Origin: store.FailureOriginInherited, Environment: true},
	}
	mocks.Pipeline.ListBuildsByPipelineIDFn = func(context.Context, int64, string, int) ([]store.Build, error) {
		return []store.Build{{ID: 1, ProjectID: proj.ID, BuildNumber: 1, CIPipelineID: new("p")}}, nil
	}
	mocks.Pipeline.ListFailuresByPipelineIDFn = func(context.Context, int64, string, int) ([]store.RunFailureRow, error) {
		return rows, nil
	}
	mocks.TestResults.ListFailedByBuildFn = func(context.Context, int64, int64, int) ([]store.TestResult, error) {
		out := make([]store.TestResult, len(rows))
		for i := range rows {
			out[i] = store.TestResult{ID: rows[i].TestResultID, BuildID: 1, FullName: rows[i].FullName, Status: "failed", StatusMessage: "x"}
		}
		return out, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{Project: mocks.Projects, Build: mocks.Builds, TestResult: mocks.TestResults, Pipeline: mocks.Pipeline})

	out, _ := call[tools.DiagnosePipelineOutput](t, cs, "diagnose_pipeline", map[string]any{"project_id": proj.ID, "pipeline_id": "p"})
	want := tools.FailureOrigins{Total: 6, New: 1, Inherited: 4, Unknown: 1, Environment: 3}
	if out.FailureOrigins == nil || *out.FailureOrigins != want {
		t.Errorf("failure_origins = %+v, want %+v (environment rows count as inherited, new + inherited + unknown = total)", out.FailureOrigins, want)
	}
	for _, ft := range out.Builds[0].FailingTests {
		var raw *store.RunFailureRow
		for i := range rows {
			if rows[i].FullName == ft.FullName {
				raw = &rows[i]
			}
		}
		if raw == nil || ft.Origin != raw.Origin || ft.Environment != raw.Environment {
			t.Errorf("%s: origin/environment = %q/%v, want the stored %v", ft.FullName, ft.Origin, ft.Environment, raw)
		}
	}
}
