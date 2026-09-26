package tools_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestPagination_NoGaps walks every page of a 7-item seed with limit=3 and
// asserts each item is seen exactly once. It is the regression guard for the
// off-by-one page math (page := offset/limit+1 paired with PerPage: limit+1)
// that started page 2 one row late, skipping a row at every boundary.
func TestPagination_NoGaps(t *testing.T) {
	ctx := context.Background()
	mocks := testutil.New()
	for i := 1; i <= 7; i++ {
		if _, err := mocks.Projects.CreateProject(ctx, fmt.Sprintf("p%d", i)); err != nil {
			t.Fatalf("seed project: %v", err)
		}
		if err := mocks.MemBuilds.InsertBuild(ctx, 1, i); err != nil {
			t.Fatalf("seed build: %v", err)
		}
	}
	defects := &pagedDefectStore{MemDefectStore: testutil.NewMemDefectStore()}
	for i := 1; i <= 7; i++ {
		defects.rows = append(defects.rows, store.DefectListRow{DefectFingerprint: store.DefectFingerprint{ID: fmt.Sprintf("d%d", i)}})
	}

	tests := []struct {
		tool      string
		stores    *bootstrap.Stores
		args      map[string]any
		key       string
		prefix    string
		wantTotal int
	}{
		{"list_projects", &bootstrap.Stores{Project: mocks.Projects}, map[string]any{"limit": 3}, "slug", "p", 7},
		{"list_recent_builds", &bootstrap.Stores{Build: mocks.MemBuilds}, map[string]any{"project_id": 1, "limit": 3}, "build_number", "", 0},
		{"list_defects", &bootstrap.Stores{Defect: defects}, map[string]any{"project_id": 1, "limit": 3}, "id", "d", 7},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			cs := setupTestServer(t, tc.stores)
			seen := map[string]int{}
			args := tc.args
			for page := 1; ; page++ {
				if page > 10 {
					t.Fatalf("pagination never terminated; seen %v", seen)
				}
				out, _ := call[struct {
					Items      []map[string]any `json:"items"`
					NextCursor string           `json:"next_cursor"`
					Total      int              `json:"total"`
				}](t, cs, tc.tool, args)
				if out.Total != tc.wantTotal {
					t.Errorf("page %d total = %d, want %d", page, out.Total, tc.wantTotal)
				}
				for _, item := range out.Items {
					seen[fmt.Sprint(item[tc.key])]++
				}
				if out.NextCursor == "" {
					break
				}
				args["cursor"] = out.NextCursor
			}
			for i := 1; i <= 7; i++ {
				if id := fmt.Sprintf("%s%d", tc.prefix, i); seen[id] != 1 {
					t.Errorf("%s %q seen %d times, want exactly once (gap or duplicate); all: %v", tc.key, id, seen[id], seen)
				}
			}
			if len(seen) != 7 {
				t.Errorf("saw %d distinct items, want 7: %v", len(seen), seen)
			}
		})
	}
}

func TestListRecentBuilds(t *testing.T) {
	built := store.Build{ID: 10, BuildNumber: 2, CIBranch: new("main"), CICommitSHA: new("abc123")}
	tests := []struct {
		name       string
		args       map[string]any
		branchErr  error
		want       []tools.RecentBuildItem
		wantBranch *int64 // branch id forwarded to ListBuildsPaginatedBranch
		wantErr    string
	}{
		{
			name: "maps builds and forwards the resolved branch",
			args: map[string]any{"project_id": 1, "branch": "main", "limit": 10},
			want: []tools.RecentBuildItem{{BuildID: 10, BuildNumber: 2, Branch: "main", CommitSHA: "abc123",
				CreatedAt: "0001-01-01T00:00:00Z"}},
			wantBranch: new(int64(9)),
		},
		{name: "unknown branch is empty, not an error", args: map[string]any{"project_id": 1, "branch": "nope"},
			branchErr: store.ErrBranchNotFound},
		// Treating every lookup error as "branch not found" masked DB faults as
		// "no builds on this branch".
		{name: "branch lookup failure is surfaced", args: map[string]any{"project_id": 1, "branch": "main"},
			branchErr: errors.New("db connection reset"), wantErr: "db connection reset"},
		{name: "non-positive project_id", args: map[string]any{"project_id": 0}, wantErr: "project_id must be positive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, name string) (*store.Branch, error) {
				return &store.Branch{ID: 9, Name: name}, tc.branchErr
			}
			var gotBranch *int64
			var gotPerPage int
			mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ int64, _, perPage int, branchID *int64) ([]store.Build, int, error) {
				gotBranch, gotPerPage = branchID, perPage
				return []store.Build{built}, 1, nil
			}
			cs := setupTestServer(t, &bootstrap.Stores{Build: mocks.Builds, Branch: mocks.Branches})

			if tc.wantErr != "" {
				if msg := callErr(t, cs, "list_recent_builds", tc.args); !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				return
			}
			out, _ := call[tools.ListRecentBuildsOutput](t, cs, "list_recent_builds", tc.args)
			if !reflect.DeepEqual(out.Items, tc.want) {
				t.Errorf("items = %+v, want %+v", out.Items, tc.want)
			}
			if tc.wantBranch != nil && (gotBranch == nil || *gotBranch != *tc.wantBranch || gotPerPage != 10) {
				t.Errorf("ListBuildsPaginatedBranch(perPage=%d, branch=%v), want (10, %d)", gotPerPage, gotBranch, *tc.wantBranch)
			}
		})
	}
}

func TestFindTestByName(t *testing.T) {
	mocks := testutil.New()
	var gotSubstring string
	mocks.TestResults.SearchByNameFn = func(_ context.Context, _ int64, substring string, _ int) ([]*store.TestResult, error) {
		gotSubstring = substring
		return []*store.TestResult{
			{BuildID: 42, HistoryID: "h1", FullName: "com.example.MyTest", Status: "failed"},
			{BuildID: 43, HistoryID: "h2", FullName: "com.example.MyOtherTest", Status: "passed"},
		}, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{TestResult: mocks.TestResults})

	out, _ := call[tools.FindTestByNameOutput](t, cs, "find_test_by_name", map[string]any{"project_id": 1, "name_substring": "MyTest"})
	want := []tools.TestNameItem{
		{HistoryID: "h1", FullName: "com.example.MyTest", LastSeenBuildID: 42, LastSeenStatus: "failed"},
		{HistoryID: "h2", FullName: "com.example.MyOtherTest", LastSeenBuildID: 43, LastSeenStatus: "passed"},
	}
	if !reflect.DeepEqual(out.Items, want) || gotSubstring != "MyTest" {
		t.Errorf("items = %+v (searched %q), want %+v (searched MyTest)", out.Items, gotSubstring, want)
	}
	for _, args := range []map[string]any{{"project_id": 1, "name_substring": ""}, {"project_id": 0, "name_substring": "Test"}} {
		callErr(t, cs, "find_test_by_name", args)
	}
}

func TestResolveURL(t *testing.T) {
	total, passed, failed, broken := 10, 8, 1, 1
	projects := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, id int64) (*store.Project, error) {
			return &store.Project{ID: id, Slug: "numeric", DisplayName: "Numeric"}, nil
		},
		GetProjectBySlugFn: func(_ context.Context, slug string) (*store.Project, error) {
			return &store.Project{ID: 7, Slug: slug, DisplayName: "By Slug"}, nil
		},
	}
	// The build id encodes the forwarded (project id, build number).
	builds := &testutil.MockBuildStore{GetBuildByNumberFn: func(_ context.Context, projectID int64, n int) (store.Build, error) {
		b := store.Build{ID: projectID*1000 + int64(n), BuildNumber: n}
		if n == 28 {
			b.CIBranch, b.CICommitSHA = new("main"), new("abc123")
			b.StatTotal, b.StatPassed, b.StatFailed, b.StatBroken = &total, &passed, &failed, &broken
		}
		return b, nil
	}}
	cs := setupTestServer(t, &bootstrap.Stores{Project: projects, Build: builds})

	tests := []struct {
		name        string
		args        map[string]any
		want        *tools.ResolveURLOutput // full output, when set
		wantBuildID int64
	}{
		{name: "numeric project URL", args: map[string]any{"url": "http://localhost:7474/projects/1/reports/28"},
			want: &tools.ResolveURLOutput{ProjectID: 1, ProjectSlug: "numeric", DisplayName: "Numeric", BuildID: 1028, BuildNumber: 28,
				Branch: "main", CommitSHA: "abc123", CreatedAt: "0001-01-01T00:00:00Z",
				Status: "total=10 passed=8 failed=1 broken=1", HasFailures: true, ReportURL: "/projects/1/reports/28"}},
		// The canonical report link uses the numeric project id, never the slug.
		{name: "slug project URL", args: map[string]any{"url": "http://localhost:7474/projects/my-slug/reports/5"},
			want: &tools.ResolveURLOutput{ProjectID: 7, ProjectSlug: "my-slug", DisplayName: "By Slug", BuildID: 7005, BuildNumber: 5,
				CreatedAt: "0001-01-01T00:00:00Z", ReportURL: "/projects/7/reports/5"}},
		{name: "project_ref and build_number", args: map[string]any{"project_ref": "3", "build_number": 10}, wantBuildID: 3010},
		{name: "trailing slash", args: map[string]any{"url": "http://host/projects/1/reports/28/"}, wantBuildID: 1028},
		{name: "suites subview", args: map[string]any{"url": "http://host/projects/1/reports/28/suites"}, wantBuildID: 1028},
		{name: "test subview", args: map[string]any{"url": "http://host/projects/1/reports/28/test/abc-123"}, wantBuildID: 1028},
		{name: "unrecognised path", args: map[string]any{"url": "http://localhost:7474/projects/1/something/else"}},
		{name: "neither url nor project_ref", args: map[string]any{"project_ref": "", "build_number": 5}},
		// The path regex is anchored, so a prefix before /projects is rejected.
		{name: "sub-path prefix", args: map[string]any{"url": "http://host/foo/projects/1/reports/28"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			switch {
			case tc.want != nil:
				if out, _ := call[tools.ResolveURLOutput](t, cs, "resolve_url", tc.args); out != *tc.want {
					t.Errorf("output = %+v, want %+v", out, *tc.want)
				}
			case tc.wantBuildID != 0:
				if out, _ := call[tools.ResolveURLOutput](t, cs, "resolve_url", tc.args); out.BuildID != tc.wantBuildID {
					t.Errorf("build_id = %d, want %d", out.BuildID, tc.wantBuildID)
				}
			default:
				callErr(t, cs, "resolve_url", tc.args)
			}
		})
	}
}
