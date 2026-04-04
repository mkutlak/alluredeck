package handlers

import (
	"context"
	"encoding/json"
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
	return NewPipelineHandler(ps, projStore, t.TempDir(), zap.NewNop())
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
	_ = projStore.CreateProjectWithParent(context.Background(), "child-a", "parent")
	_ = projStore.CreateProjectWithParent(context.Background(), "child-b", "parent")

	now := time.Now().UTC()
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, parentID, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
			return []store.PipelineRunRow{
				{CommitSHA: "abc1234", Branch: "main", CIBuildURL: "https://ci/1", CreatedAt: now, ProjectID: "child-a", BuildOrder: 5, StatPassed: intPtr(40), StatFailed: intPtr(2), StatBroken: intPtr(0), StatTotal: intPtr(42), DurationMs: int64Ptr(15000)},
				{CommitSHA: "abc1234", Branch: "main", CIBuildURL: "", CreatedAt: now.Add(-time.Second), ProjectID: "child-b", BuildOrder: 3, StatPassed: intPtr(100), StatFailed: intPtr(0), StatBroken: intPtr(0), StatTotal: intPtr(100), DurationMs: int64Ptr(30000)},
				{CommitSHA: "def5678", Branch: "main", CIBuildURL: "https://ci/2", CreatedAt: now.Add(-time.Hour), ProjectID: "child-a", BuildOrder: 4, StatPassed: intPtr(42), StatFailed: intPtr(0), StatBroken: intPtr(0), StatTotal: intPtr(42), DurationMs: int64Ptr(14000)},
			}, 2, nil
		},
	}

	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, "parent", "")

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

func TestPipelineHandler_GetPipelineRuns_Empty(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	_ = projStore.CreateProjectWithParent(context.Background(), "child", "parent")

	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, "parent", "")

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
	_ = projStore.CreateProjectWithParent(context.Background(), "child", "parent")

	var capturedBranch string
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _, branch string, _, _ int) ([]store.PipelineRunRow, int, error) {
			capturedBranch = branch
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	pipelineRequest(t, h, "parent", "branch=develop")

	if capturedBranch != "develop" {
		t.Errorf("branch = %q, want develop", capturedBranch)
	}
}

func TestPipelineHandler_GetPipelineRuns_NotParent(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	_ = projStore.CreateProject(context.Background(), "standalone")

	ps := &testutil.MockPipelineStore{}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, "standalone", "")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPipelineHandler_GetPipelineRuns_DefaultPerPage(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	_ = projStore.CreateProjectWithParent(context.Background(), "child", "parent")

	var capturedPerPage int
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _, _ string, _, perPage int) ([]store.PipelineRunRow, int, error) {
			capturedPerPage = perPage
			return nil, 0, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	pipelineRequest(t, h, "parent", "")

	if capturedPerPage != 10 {
		t.Errorf("per_page = %d, want 10", capturedPerPage)
	}
}

func TestPipelineHandler_GetPipelineRuns_Pagination(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	_ = projStore.CreateProjectWithParent(context.Background(), "child", "parent")

	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(_ context.Context, _, _ string, _, _ int) ([]store.PipelineRunRow, int, error) {
			return nil, 25, nil
		},
	}
	h := newPipelineHandler(t, ps, projStore)
	rr := pipelineRequest(t, h, "parent", "page=2&per_page=10")

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
