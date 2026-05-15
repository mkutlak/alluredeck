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

// buildStoresDiscovery assembles a *bootstrap.Stores for discovery tool tests.
func buildStoresDiscovery(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Project:    mocks.Projects,
		Build:      mocks.Builds,
		TestResult: mocks.TestResults,
		Branch:     mocks.Branches,
	}
}

func decodeResolveURL(t *testing.T, res *mcpsdk.CallToolResult) tools.ResolveURLOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ResolveURLOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ResolveURLOutput: %v", err)
	}
	return out
}

func decodeListProjects(t *testing.T, res *mcpsdk.CallToolResult) tools.ListProjectsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ListProjectsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListProjectsOutput: %v", err)
	}
	return out
}

func decodeListRecentBuilds(t *testing.T, res *mcpsdk.CallToolResult) tools.ListRecentBuildsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ListRecentBuildsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListRecentBuildsOutput: %v", err)
	}
	return out
}

func decodeFindTestByName(t *testing.T, res *mcpsdk.CallToolResult) tools.FindTestByNameOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.FindTestByNameOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal FindTestByNameOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// list_projects
// ---------------------------------------------------------------------------

func TestListProjects_HappyPath(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()
	if _, err := mocks.Projects.CreateProject(ctx, "alpha"); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if _, err := mocks.Projects.CreateProject(ctx, "beta"); err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListProjects(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
}

func TestListProjects_Pagination(t *testing.T) {
	mocks := testutil.New()
	ctx := context.Background()
	for _, slug := range []string{"p1", "p2", "p3"} {
		if _, err := mocks.Projects.CreateProject(ctx, slug); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))

	// Page 1 — limit=2 requests perPage=3 internally (limit+1), should return 2 items + cursor.
	res1, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"limit": 2},
	})
	if err != nil {
		t.Fatalf("page1 CallTool: %v", err)
	}
	if res1.IsError {
		t.Fatalf("page1 unexpected error: %v", res1.Content)
	}
	out1 := decodeListProjects(t, res1)
	if len(out1.Items) != 2 {
		t.Errorf("page1: want 2 items, got %d", len(out1.Items))
	}
	if out1.NextCursor == "" {
		t.Error("page1: want non-empty next_cursor")
	}
}

// ---------------------------------------------------------------------------
// list_recent_builds
// ---------------------------------------------------------------------------

func TestListRecentBuilds_HappyPath(t *testing.T) {
	commitSHA := "abc123"
	branch := "main"
	b1 := store.Build{ID: 10, ProjectID: 1, BuildNumber: 2, CIBranch: &branch, CICommitSHA: &commitSHA}
	b2 := store.Build{ID: 20, ProjectID: 1, BuildNumber: 1}

	mocks := testutil.New()
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ int64, _, _ int, _ *int64) ([]store.Build, int, error) {
		return []store.Build{b1, b2}, 2, nil
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_recent_builds",
		Arguments: map[string]any{"project_id": 1, "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListRecentBuilds(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].BuildID != 10 {
		t.Errorf("want build_id=10, got %d", out.Items[0].BuildID)
	}
	if out.Items[0].Branch != "main" {
		t.Errorf("want branch=main, got %q", out.Items[0].Branch)
	}
	if out.Items[0].CommitSHA != "abc123" {
		t.Errorf("want commit_sha=abc123, got %q", out.Items[0].CommitSHA)
	}
}

func TestListRecentBuilds_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_recent_builds",
		Arguments: map[string]any{"project_id": 0},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestListRecentBuilds_Pagination(t *testing.T) {
	all := []store.Build{
		{ID: 1, BuildNumber: 1}, {ID: 2, BuildNumber: 2},
		{ID: 3, BuildNumber: 3}, {ID: 4, BuildNumber: 4},
	}
	mocks := testutil.New()
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ int64, page, perPage int, _ *int64) ([]store.Build, int, error) {
		// perPage includes the +1 has-more sentinel.
		start := (page - 1) * (perPage - 1)
		if start >= len(all) {
			return nil, len(all), nil
		}
		end := min(start+perPage, len(all))
		return all[start:end], len(all), nil
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res1, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_recent_builds",
		Arguments: map[string]any{"project_id": 1, "limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res1.IsError {
		t.Fatalf("unexpected error: %v", res1.Content)
	}
	out1 := decodeListRecentBuilds(t, res1)
	if len(out1.Items) != 2 {
		t.Errorf("page1: want 2 items, got %d", len(out1.Items))
	}
	if out1.NextCursor == "" {
		t.Error("page1: want non-empty next_cursor")
	}
}

// ---------------------------------------------------------------------------
// find_test_by_name
// ---------------------------------------------------------------------------

func TestFindTestByName_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.SearchByNameFn = func(_ context.Context, _ int64, _ string, _ int) ([]*store.TestResult, error) {
		return []*store.TestResult{
			{BuildID: 42, HistoryID: "h1", FullName: "com.example.MyTest", Status: "failed"},
			{BuildID: 42, HistoryID: "h2", FullName: "com.example.MyOtherTest", Status: "passed"},
		}, nil
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "find_test_by_name",
		Arguments: map[string]any{"project_id": 1, "name_substring": "MyTest"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeFindTestByName(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].HistoryID != "h1" {
		t.Errorf("want history_id=h1, got %q", out.Items[0].HistoryID)
	}
	if out.Items[0].LastSeenStatus != "failed" {
		t.Errorf("want status=failed, got %q", out.Items[0].LastSeenStatus)
	}
}

func TestFindTestByName_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	// Missing name_substring.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "find_test_by_name",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty name_substring")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "find_test_by_name",
		Arguments: map[string]any{"name_substring": "Test"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestFindTestByName_EmptyResults(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.SearchByNameFn = func(_ context.Context, _ int64, _ string, _ int) ([]*store.TestResult, error) {
		return nil, nil
	}

	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "find_test_by_name",
		Arguments: map[string]any{"project_id": 1, "name_substring": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	out := decodeFindTestByName(t, res)
	if len(out.Items) != 0 {
		t.Errorf("want 0 items, got %d", len(out.Items))
	}
}

// buildStoresResolveURL assembles stores for resolve_url tests using the
// fine-grained MockProjectStore (not MemProjectStore) so Fn fields can be set.
func buildStoresResolveURL(projMock *testutil.MockProjectStore, buildMock *testutil.MockBuildStore) *bootstrap.Stores {
	return &bootstrap.Stores{
		Project: projMock,
		Build:   buildMock,
	}
}

// ---------------------------------------------------------------------------
// resolve_url
// ---------------------------------------------------------------------------

func TestResolveURL_ByURL_NumericProject(t *testing.T) {
	branch := "main"
	sha := "abc123"
	total := 10
	passed := 8
	failed := 1
	broken := 1

	projMock := &testutil.MockProjectStore{}
	projMock.GetProjectFn = func(_ context.Context, id int64) (*store.Project, error) {
		return &store.Project{ID: id, Slug: "my-project", DisplayName: "My Project"}, nil
	}
	buildMock := &testutil.MockBuildStore{}
	buildMock.GetBuildByNumberFn = func(_ context.Context, _ int64, buildNumber int) (store.Build, error) {
		return store.Build{
			ID:          164,
			ProjectID:   1,
			BuildNumber: buildNumber,
			CIBranch:    &branch,
			CICommitSHA: &sha,
			StatTotal:   &total,
			StatPassed:  &passed,
			StatFailed:  &failed,
			StatBroken:  &broken,
		}, nil
	}

	cs := setupTestServer(t, buildStoresResolveURL(projMock, buildMock))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"url": "http://localhost:7474/projects/1/reports/28"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeResolveURL(t, res)
	if out.ProjectID != 1 {
		t.Errorf("want project_id=1, got %d", out.ProjectID)
	}
	if out.BuildID != 164 {
		t.Errorf("want build_id=164, got %d", out.BuildID)
	}
	if out.BuildNumber != 28 {
		t.Errorf("want build_number=28, got %d", out.BuildNumber)
	}
	if out.Branch != "main" {
		t.Errorf("want branch=main, got %q", out.Branch)
	}
	if out.CommitSHA != "abc123" {
		t.Errorf("want commit_sha=abc123, got %q", out.CommitSHA)
	}
	if !out.HasFailures {
		t.Error("want has_failures=true")
	}
}

func TestResolveURL_ByURL_SlugProject(t *testing.T) {
	projMock := &testutil.MockProjectStore{}
	projMock.GetProjectBySlugFn = func(_ context.Context, slug string) (*store.Project, error) {
		return &store.Project{ID: 7, Slug: slug, DisplayName: "Slug Project"}, nil
	}
	buildMock := &testutil.MockBuildStore{}
	buildMock.GetBuildByNumberFn = func(_ context.Context, _ int64, buildNumber int) (store.Build, error) {
		return store.Build{ID: 99, ProjectID: 7, BuildNumber: buildNumber}, nil
	}

	cs := setupTestServer(t, buildStoresResolveURL(projMock, buildMock))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"url": "http://localhost:7474/projects/my-slug/reports/5"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeResolveURL(t, res)
	if out.ProjectID != 7 {
		t.Errorf("want project_id=7, got %d", out.ProjectID)
	}
	if out.BuildID != 99 {
		t.Errorf("want build_id=99, got %d", out.BuildID)
	}
}

func TestResolveURL_ByComponents(t *testing.T) {
	projMock := &testutil.MockProjectStore{}
	projMock.GetProjectFn = func(_ context.Context, id int64) (*store.Project, error) {
		return &store.Project{ID: id, Slug: "proj", DisplayName: "Proj"}, nil
	}
	buildMock := &testutil.MockBuildStore{}
	buildMock.GetBuildByNumberFn = func(_ context.Context, _ int64, buildNumber int) (store.Build, error) {
		return store.Build{ID: 55, ProjectID: 3, BuildNumber: buildNumber}, nil
	}

	cs := setupTestServer(t, buildStoresResolveURL(projMock, buildMock))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"project_ref": "3", "build_number": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeResolveURL(t, res)
	if out.BuildID != 55 {
		t.Errorf("want build_id=55, got %d", out.BuildID)
	}
}

func TestResolveURL_InvalidURL(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"url": "http://localhost:7474/projects/1/something/else"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for unrecognised URL path")
	}
}

func TestResolveURL_MissingInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	// Neither url nor project_ref.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"build_number": 5},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true when neither url nor project_ref provided")
	}
}

// TestResolveURL_SubPathRejected verifies that the anchored regex rejects a URL
// whose path has a prefix before /projects/…/reports/… (Fix 3).
func TestResolveURL_SubPathRejected(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDiscovery(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resolve_url",
		Arguments: map[string]any{"url": "http://host/foo/projects/1/reports/28"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for path /foo/projects/1/reports/28 (not anchored at root)")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------
