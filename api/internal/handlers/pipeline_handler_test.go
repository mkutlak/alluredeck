package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func newPipelineHandler(t *testing.T, ps *testutil.MockPipelineStore, projStore *testutil.MemProjectStore) *PipelineHandler {
	t.Helper()
	return NewPipelineHandler(ps, projStore, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
}

func pipelineRequest(t *testing.T, h *PipelineHandler, projectID, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/projects/" + projectID + "/pipeline-runs"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("project_id", projectID)
	rr := httptest.NewRecorder()
	h.GetPipelineRuns(rr, req)
	return rr
}

func TestPipelineHandler_GetPipelineRuns_Success(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	childA, _ := projStore.CreateProjectWithParent(context.Background(), "child-a", parentProj.ID)
	childB, _ := projStore.CreateProjectWithParent(context.Background(), "child-b", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	now := time.Now().UTC()
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, parentID int64, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
			return []store.PipelineRunRow{
				{CommitSHA: "abc1234", Branch: "main", CIBuildURL: "https://ci/1", CreatedAt: now, ProjectID: childA.ID, Slug: "child-a", BuildNumber: 5, BuildID: 501, StatPassed: new(40), StatFailed: new(2), StatBroken: new(0), StatTotal: new(42), DurationMs: new(int64(15000))},
				{CommitSHA: "abc1234", Branch: "main", CIBuildURL: "", CreatedAt: now.Add(-time.Second), ProjectID: childB.ID, Slug: "child-b", BuildNumber: 3, BuildID: 301, StatPassed: new(100), StatFailed: new(0), StatBroken: new(0), StatTotal: new(100), DurationMs: new(int64(30000))},
				{CommitSHA: "def5678", Branch: "main", CIBuildURL: "https://ci/2", CreatedAt: now.Add(-time.Hour), ProjectID: childA.ID, Slug: "child-a", BuildNumber: 4, BuildID: 401, StatPassed: new(42), StatFailed: new(0), StatBroken: new(0), StatTotal: new(42), DurationMs: new(int64(14000))},
			}, 2, nil
		},
	}

	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, parentIDStr, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data       []pipelineRunResp `json:"data"`
		Pagination PaginationMeta    `json:"pagination"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(resp.Data))
	}

	run0 := resp.Data[0]
	if run0.CommitSHA != "abc1234" {
		t.Errorf("run[0] commit_sha = %q, want abc1234", run0.CommitSHA)
	}
	if run0.CIBuildURL != "https://ci/1" {
		t.Errorf("run[0] ci_build_url = %q, want https://ci/1", run0.CIBuildURL)
	}
	if len(run0.Suites) != 2 {
		t.Errorf("run[0] suites count = %d, want 2", len(run0.Suites))
	}
	if run0.Aggregate.SuitesTotal != 2 {
		t.Errorf("run[0] aggregate.suites_total = %d, want 2", run0.Aggregate.SuitesTotal)
	}
	if run0.Aggregate.TestsTotal != 142 {
		t.Errorf("run[0] aggregate.tests_total = %d, want 142", run0.Aggregate.TestsTotal)
	}
	if run0.Suites[0].ProjectID != childA.ID {
		t.Errorf("run[0] suites[0].project_id = %d, want %d", run0.Suites[0].ProjectID, childA.ID)
	}
	if run0.Suites[0].BuildID != 501 {
		t.Errorf("run[0] suites[0].build_id = %d, want 501", run0.Suites[0].BuildID)
	}

	run1 := resp.Data[1]
	if run1.CommitSHA != "def5678" {
		t.Errorf("run[1] commit_sha = %q, want def5678", run1.CommitSHA)
	}
	if len(run1.Suites) != 1 {
		t.Errorf("run[1] suites count = %d, want 1", len(run1.Suites))
	}
	if run1.Suites[0].Status != "passed" {
		t.Errorf("run[1] suite status = %q, want passed", run1.Suites[0].Status)
	}

	if resp.Pagination.Total != 2 {
		t.Errorf("pagination.total = %d, want 2", resp.Pagination.Total)
	}
}

// CI shards a suite across parallel jobs and every shard uploads its own build
// under the same pipeline ID (see the Playwright --shard invocation in the
// ui-tests template). Those builds are one logical suite and must be merged,
// otherwise the feed reports shard-builds as suites and renders the same suite
// several times with contradictory statuses.
func TestPipelineHandler_MergesShardBuildsIntoOneSuite(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	child, _ := projStore.CreateProjectWithParent(context.Background(), "ui-users", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	now := time.Now().UTC()
	shard := func(buildNumber int, buildID int64, passed, failed, skipped, total int, dur int64, created time.Time) store.PipelineRunRow {
		return store.PipelineRunRow{
			PipelineID: "196765", CommitSHA: "6fb9dec", Branch: "master", CreatedAt: created,
			ProjectID: child.ID, Slug: "ui-users", DisplayName: "UI Users",
			BuildNumber: buildNumber, BuildID: buildID,
			StatPassed: &passed, StatFailed: &failed, StatBroken: new(0),
			StatSkipped: &skipped, StatTotal: &total, DurationMs: &dur,
		}
	}

	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _ int64, _ string, _, _ int) ([]store.PipelineRunRow, int, error) {
			return []store.PipelineRunRow{
				shard(656, 17465, 22, 2, 0, 24, 3000, now),
				shard(655, 17464, 22, 2, 0, 24, 2000, now.Add(-time.Second)),
				shard(654, 17463, 21, 0, 3, 24, 1000, now.Add(-2*time.Minute)),
			}, 1, nil
		},
	}

	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, parentIDStr, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []pipelineRunResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Data))
	}

	run := resp.Data[0]
	if len(run.Suites) != 1 {
		t.Fatalf("expected 3 shard builds to merge into 1 suite, got %d suites", len(run.Suites))
	}

	s := run.Suites[0]
	if s.Total != 72 || s.Failed != 4 {
		t.Errorf("suite total/failed = %d/%d, want 72/4", s.Total, s.Failed)
	}
	if s.DurationMs != 6000 {
		t.Errorf("suite duration_ms = %d, want 6000 (sum of shards)", s.DurationMs)
	}
	if s.DisplayName != "UI Users" {
		t.Errorf("suite display_name = %q, want %q", s.DisplayName, "UI Users")
	}
	// 65 passed of (72 total - 3 skipped) = 94.2%.
	if s.PassRate != 94.2 {
		t.Errorf("suite pass_rate = %v, want 94.2", s.PassRate)
	}
	if s.Status != "degraded" {
		t.Errorf("suite status = %q, want degraded", s.Status)
	}
	// The single-report link must point at the newest shard.
	if s.BuildNumber != 656 || s.BuildID != 17465 {
		t.Errorf("suite build = #%d/%d, want #656/17465", s.BuildNumber, s.BuildID)
	}
	// Every contributing shard stays addressable, ordered oldest-first.
	if len(s.Builds) != 3 {
		t.Fatalf("suite builds = %d, want 3", len(s.Builds))
	}
	for i, want := range []int{654, 655, 656} {
		if s.Builds[i].BuildNumber != want {
			t.Errorf("builds[%d].build_number = %d, want %d", i, s.Builds[i].BuildNumber, want)
		}
	}

	agg := run.Aggregate
	if agg.SuitesTotal != 1 {
		t.Errorf("aggregate.suites_total = %d, want 1 (distinct suites, not shard builds)", agg.SuitesTotal)
	}
	if agg.SuitesPassed != 0 {
		t.Errorf("aggregate.suites_passed = %d, want 0 (a shard failed)", agg.SuitesPassed)
	}
	if agg.TestsTotal != 72 {
		t.Errorf("aggregate.tests_total = %d, want 72", agg.TestsTotal)
	}
	// Skipped tests must not be counted as passed, or the tests fraction
	// disagrees with the pass-rate percentage shown beside it.
	if agg.TestsPassed != 65 {
		t.Errorf("aggregate.tests_passed = %d, want 65 (excludes the 3 skipped)", agg.TestsPassed)
	}
}

func TestPipelineHandler_MergedSuitePassesOnlyWhenEveryShardPasses(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	clean, _ := projStore.CreateProjectWithParent(context.Background(), "clean", parentProj.ID)
	mixed, _ := projStore.CreateProjectWithParent(context.Background(), "mixed", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	now := time.Now().UTC()
	row := func(projectID int64, slug string, buildNumber int, buildID int64, passed, failed, total int) store.PipelineRunRow {
		return store.PipelineRunRow{
			PipelineID: "p1", CommitSHA: "sha", Branch: "master", CreatedAt: now,
			ProjectID: projectID, Slug: slug, BuildNumber: buildNumber, BuildID: buildID,
			StatPassed: &passed, StatFailed: &failed, StatBroken: new(0),
			StatSkipped: new(0), StatTotal: &total, DurationMs: new(int64(100)),
		}
	}

	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _ int64, _ string, _, _ int) ([]store.PipelineRunRow, int, error) {
			return []store.PipelineRunRow{
				row(clean.ID, "clean", 1, 11, 10, 0, 10),
				row(clean.ID, "clean", 2, 12, 10, 0, 10),
				row(mixed.ID, "mixed", 1, 21, 10, 0, 10),
				row(mixed.ID, "mixed", 2, 22, 9, 1, 10),
			}, 1, nil
		},
	}

	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, parentIDStr, "")

	var resp struct {
		Data []pipelineRunResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Data))
	}

	agg := resp.Data[0].Aggregate
	if agg.SuitesTotal != 2 {
		t.Errorf("aggregate.suites_total = %d, want 2", agg.SuitesTotal)
	}
	if agg.SuitesPassed != 1 {
		t.Errorf("aggregate.suites_passed = %d, want 1 — a suite passes only when every shard passed", agg.SuitesPassed)
	}

	bySlug := map[string]pipelineSuiteResp{}
	for _, s := range resp.Data[0].Suites {
		bySlug[s.Slug] = s
	}
	if bySlug["clean"].Status != "passed" {
		t.Errorf("clean suite status = %q, want passed", bySlug["clean"].Status)
	}
	if bySlug["mixed"].Status == "passed" {
		t.Error("mixed suite must not report passed when one shard failed")
	}
}

func TestPipelineHandler_GetPipelineRuns_Empty(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, parentIDStr, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []pipelineRunResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 runs, got %d", len(resp.Data))
	}
}

func TestPipelineHandler_GetPipelineRuns_BranchFilter(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	var capturedBranch string
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _ int64, branch string, _, _ int) ([]store.PipelineRunRow, int, error) {
			capturedBranch = branch
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	pipelineRequest(t, h, parentIDStr, "branch=develop")

	if capturedBranch != "develop" {
		t.Errorf("branch = %q, want develop", capturedBranch)
	}
}

func TestPipelineHandler_GetPipelineRuns_NotParent(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	standaloneProj, _ := projStore.CreateProject(context.Background(), "standalone")
	standaloneIDStr := fmt.Sprintf("%d", standaloneProj.ID)

	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, standaloneIDStr, "")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPipelineHandler_GetPipelineRuns_DefaultPerPage(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	var capturedPerPage int
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _ int64, _ string, _, perPage int) ([]store.PipelineRunRow, int, error) {
			capturedPerPage = perPage
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	pipelineRequest(t, h, parentIDStr, "")

	if capturedPerPage != 10 {
		t.Errorf("per_page = %d, want 10", capturedPerPage)
	}
}

func TestPipelineHandler_GetPipelineRuns_Pagination(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _ int64, _ string, _, _ int) ([]store.PipelineRunRow, int, error) {
			return nil, 25, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, parentIDStr, "page=2&per_page=10")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Pagination PaginationMeta `json:"pagination"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Pagination.Page != 2 {
		t.Errorf("pagination.page = %d, want 2", resp.Pagination.Page)
	}
	if resp.Pagination.PerPage != 10 {
		t.Errorf("pagination.per_page = %d, want 10", resp.Pagination.PerPage)
	}
	if resp.Pagination.Total != 25 {
		t.Errorf("pagination.total = %d, want 25", resp.Pagination.Total)
	}
	if resp.Pagination.TotalPages != 3 {
		t.Errorf("pagination.total_pages = %d, want 3", resp.Pagination.TotalPages)
	}
}

func allPipelineRunsRequest(h *PipelineHandler, query string) *httptest.ResponseRecorder {
	path := "/api/v1/pipeline-runs"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h.GetAllPipelineRuns(rr, req)
	return rr
}

func TestPipelineHandler_GetAllPipelineRuns_CrossGroupSameSHA(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	now := time.Now().UTC()

	ps := &testutil.MockPipelineStore{
		ListAllPipelineRunsFn: func(_ context.Context, _ string, _ []int64, _, _ int) ([]store.PipelineRunRow, int, error) {
			return []store.PipelineRunRow{
				{
					CommitSHA: "sameSHA", Branch: "main", CreatedAt: now,
					ProjectID: 10, Slug: "child-a", BuildNumber: 5, BuildID: 501,
					GroupProjectID: 100, GroupSlug: "group-a",
					StatPassed: new(10), StatFailed: new(0), StatBroken: new(0), StatTotal: new(10), DurationMs: new(int64(1000)),
				},
				{
					CommitSHA: "sameSHA", Branch: "main", CreatedAt: now.Add(-time.Minute),
					ProjectID: 20, Slug: "child-b", BuildNumber: 3, BuildID: 301,
					GroupProjectID: 200, GroupSlug: "group-b",
					StatPassed: new(20), StatFailed: new(0), StatBroken: new(0), StatTotal: new(20), DurationMs: new(int64(2000)),
				},
			}, 2, nil
		},
	}

	h := newPipelineHandler(t, ps, projStore)
	rr := allPipelineRunsRequest(h, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []pipelineRunResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 separate runs for same SHA under different groups, got %d", len(resp.Data))
	}

	byGroup := map[int64]pipelineRunResp{}
	for _, run := range resp.Data {
		byGroup[run.GroupProjectID] = run
	}

	runA, ok := byGroup[100]
	if !ok {
		t.Fatalf("expected a run with group_project_id=100, got %+v", resp.Data)
	}
	if runA.GroupSlug != "group-a" {
		t.Errorf("run(group=100).group_slug = %q, want group-a", runA.GroupSlug)
	}
	if len(runA.Suites) != 1 || runA.Suites[0].BuildID != 501 {
		t.Errorf("run(group=100) suites = %+v, want single suite with build_id=501", runA.Suites)
	}

	runB, ok := byGroup[200]
	if !ok {
		t.Fatalf("expected a run with group_project_id=200, got %+v", resp.Data)
	}
	if runB.GroupSlug != "group-b" {
		t.Errorf("run(group=200).group_slug = %q, want group-b", runB.GroupSlug)
	}
	if len(runB.Suites) != 1 || runB.Suites[0].BuildID != 301 {
		t.Errorf("run(group=200) suites = %+v, want single suite with build_id=301", runB.Suites)
	}
}

func TestPipelineHandler_GetAllPipelineRuns_ArgsCaptured(t *testing.T) {
	projStore := testutil.NewMemProjectStore()

	var (
		capturedBranch   string
		capturedGroupIDs []int64
	)
	ps := &testutil.MockPipelineStore{
		ListAllPipelineRunsFn: func(_ context.Context, branch string, groupIDs []int64, _, _ int) ([]store.PipelineRunRow, int, error) {
			capturedBranch = branch
			capturedGroupIDs = groupIDs
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	allPipelineRunsRequest(h, "branch=develop&group_id=1&group_id=2")

	if capturedBranch != "develop" {
		t.Errorf("branch = %q, want develop", capturedBranch)
	}
	if len(capturedGroupIDs) != 2 || capturedGroupIDs[0] != 1 || capturedGroupIDs[1] != 2 {
		t.Errorf("group_ids = %v, want [1 2]", capturedGroupIDs)
	}
}

func TestPipelineHandler_GetAllPipelineRuns_InvalidGroupID(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := allPipelineRunsRequest(h, "group_id=abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPipelineHandler_GetAllPipelineRuns_Empty(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := allPipelineRunsRequest(h, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data       []pipelineRunResp `json:"data"`
		Pagination PaginationMeta    `json:"pagination"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 runs, got %d", len(resp.Data))
	}
	if resp.Pagination.Total != 0 {
		t.Errorf("pagination.total = %d, want 0", resp.Pagination.Total)
	}
}

func runFailuresRequest(h *PipelineHandler, projectID, runKey, query string) *httptest.ResponseRecorder {
	path := "/api/v1/projects/" + projectID + "/pipeline-runs/" + runKey + "/failures"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("run_key", runKey)
	rr := httptest.NewRecorder()
	h.GetRunFailures(rr, req)
	return rr
}

func TestPipelineHandler_GetRunFailures_TagsRowsWithSuiteAndFlagsKnown(t *testing.T) {
	ctx := context.Background()
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(ctx, "parent")
	child, _ := projStore.CreateProjectWithParent(ctx, "ui-users", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	knownStore := testutil.NewMemKnownIssueStore()
	if _, err := knownStore.Create(ctx, child.ID, "flaky login", "", "", ""); err != nil {
		t.Fatalf("seed known issue: %v", err)
	}

	var (
		capturedGroupID int64
		capturedRunKey  string
		capturedLimit   int
	)
	ps := &testutil.MockPipelineStore{
		ListRunFailuresFn: func(_ context.Context, groupProjectID int64, runKey string, limit int) ([]store.RunFailureRow, error) {
			capturedGroupID, capturedRunKey, capturedLimit = groupProjectID, runKey, limit
			return []store.RunFailureRow{
				{
					ProjectID: child.ID, Slug: "ui-users", DisplayName: "UI Users",
					BuildID: 17465, BuildNumber: 656,
					TestName: "flaky login", FullName: "spec.js:1:1", Status: "failed",
					HistoryID: "h1", Retries: 3,
					StatusMessage: "TimeoutError: locator.click\n  at foo.ts:12",
				},
				{
					ProjectID: child.ID, Slug: "ui-users", DisplayName: "UI Users",
					BuildID: 17464, BuildNumber: 655,
					TestName: "add member", FullName: "spec.js:2:1", Status: "broken",
					HistoryID: "h2", NewFailed: true,
					StatusMessage: "",
				},
			}, nil
		},
	}

	h := NewPipelineHandler(ps, projStore, knownStore, t.TempDir(), zap.NewNop())
	rr := runFailuresRequest(h, parentIDStr, "196765", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if capturedGroupID != parentProj.ID {
		t.Errorf("store got group_project_id = %d, want %d", capturedGroupID, parentProj.ID)
	}
	if capturedRunKey != "196765" {
		t.Errorf("store got run_key = %q, want 196765", capturedRunKey)
	}
	// One extra row is requested so a full page can be reported as truncated.
	if capturedLimit != defaultRunFailuresLimit+1 {
		t.Errorf("store got limit = %d, want %d", capturedLimit, defaultRunFailuresLimit+1)
	}

	var resp struct {
		Data     []runFailureResp `json:"data"`
		Metadata struct {
			Truncated bool `json:"truncated"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resp.Data))
	}
	if resp.Metadata.Truncated {
		t.Error("metadata.truncated = true, want false for a short result")
	}

	first := resp.Data[0]
	if first.Slug != "ui-users" || first.BuildNumber != 656 {
		t.Errorf("row[0] suite = %q #%d, want ui-users #656", first.Slug, first.BuildNumber)
	}
	if !first.Known {
		t.Error("row[0] known = false, want true — it matches an active known issue")
	}
	// Only the first line of the error is sent; the client renders a preview.
	if first.ErrorMessage != "TimeoutError: locator.click" {
		t.Errorf("row[0] error_message = %q, want the first line only", first.ErrorMessage)
	}
	if resp.Data[1].Known {
		t.Error("row[1] known = true, want false")
	}
	if !resp.Data[1].NewFailed {
		t.Error("row[1] new_failed = false, want true")
	}
}

func TestPipelineHandler_GetRunFailures_TruncatesAtLimit(t *testing.T) {
	ctx := context.Background()
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(ctx, "parent")
	child, _ := projStore.CreateProjectWithParent(ctx, "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	ps := &testutil.MockPipelineStore{
		ListRunFailuresFn: func(_ context.Context, _ int64, _ string, limit int) ([]store.RunFailureRow, error) {
			rows := make([]store.RunFailureRow, limit)
			for i := range rows {
				rows[i] = store.RunFailureRow{ProjectID: child.ID, Slug: "child", TestName: fmt.Sprintf("t%d", i)}
			}
			return rows, nil
		},
	}

	h := NewPipelineHandler(ps, projStore, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
	rr := runFailuresRequest(h, parentIDStr, "196765", "limit=3")

	var resp struct {
		Data     []runFailureResp `json:"data"`
		Metadata struct {
			Truncated bool `json:"truncated"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Errorf("data length = %d, want 3 (the overflow row is trimmed)", len(resp.Data))
	}
	if !resp.Metadata.Truncated {
		t.Error("metadata.truncated = false, want true when the limit was reached")
	}
}

func TestPipelineHandler_GetRunFailures_InvalidLimit(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	h := NewPipelineHandler(&testutil.MockPipelineStore{}, projStore, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
	rr := runFailuresRequest(h, parentIDStr, "196765", "limit=-1")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPipelineHandler_GetRunFailures_EmptyRunKey(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	parentProj, _ := projStore.CreateProject(context.Background(), "parent")
	_, _ = projStore.CreateProjectWithParent(context.Background(), "child", parentProj.ID)
	parentIDStr := fmt.Sprintf("%d", parentProj.ID)

	h := NewPipelineHandler(&testutil.MockPipelineStore{}, projStore, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
	rr := runFailuresRequest(h, parentIDStr, "", "")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPipelineHandler_GetAllPipelineRuns_DefaultPerPage(t *testing.T) {
	projStore := testutil.NewMemProjectStore()

	var capturedPerPage int
	ps := &testutil.MockPipelineStore{
		ListAllPipelineRunsFn: func(_ context.Context, _ string, _ []int64, _, perPage int) ([]store.PipelineRunRow, int, error) {
			capturedPerPage = perPage
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	allPipelineRunsRequest(h, "")

	if capturedPerPage != 10 {
		t.Errorf("per_page = %d, want 10", capturedPerPage)
	}
}
