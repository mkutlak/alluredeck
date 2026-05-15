package tools

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ---------------------------------------------------------------------------
// list_projects
// ---------------------------------------------------------------------------

// ListProjectsInput holds parameters for list_projects.
type ListProjectsInput struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// ProjectItem is one project in the list_projects response.
type ProjectItem struct {
	ProjectID       int64  `json:"project_id"`
	Slug            string `json:"slug"`
	DisplayName     string `json:"display_name"`
	ParentProjectID *int64 `json:"parent_project_id,omitempty"`
}

// ListProjectsOutput is the structured output for list_projects.
type ListProjectsOutput struct {
	Items      []ProjectItem `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// ---------------------------------------------------------------------------
// list_recent_builds
// ---------------------------------------------------------------------------

// ListRecentBuildsInput holds parameters for list_recent_builds.
type ListRecentBuildsInput struct {
	ProjectID int    `json:"project_id"`
	Branch    string `json:"branch,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

// RecentBuildItem is one build in the list_recent_builds response.
type RecentBuildItem struct {
	BuildID       int64  `json:"build_id"`
	BuildNumber   int    `json:"build_number"`
	Branch        string `json:"branch,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CreatedAt     string `json:"created_at"`
	StatusSummary string `json:"status_summary,omitempty"`
}

// ListRecentBuildsOutput is the structured output for list_recent_builds.
type ListRecentBuildsOutput struct {
	Items      []RecentBuildItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// ---------------------------------------------------------------------------
// find_test_by_name
// ---------------------------------------------------------------------------

// FindTestByNameInput holds parameters for find_test_by_name.
type FindTestByNameInput struct {
	ProjectID     int    `json:"project_id"`
	NameSubstring string `json:"name_substring"`
}

// TestNameItem is one test in the find_test_by_name response.
type TestNameItem struct {
	HistoryID       string `json:"history_id"`
	FullName        string `json:"full_name"`
	LastSeenBuildID int64  `json:"last_seen_build_id"`
	LastSeenStatus  string `json:"last_seen_status"`
}

// FindTestByNameOutput is the structured output for find_test_by_name.
type FindTestByNameOutput struct {
	Items []TestNameItem `json:"items"`
}

// ---------------------------------------------------------------------------
// resolve_url
// ---------------------------------------------------------------------------

// reURLPath matches the /projects/<proj>/reports/<num> path pattern in the
// alluredeck UI. <proj> may be a numeric project_id or a slug string.
// Anchored at ^ so that sub-paths like /foo/projects/x/reports/1 are rejected.
var reURLPath = regexp.MustCompile(`^/projects/(?P<proj>[^/]+)/reports/(?P<num>\d+)/?$`)

// ResolveURLInput holds parameters for the resolve_url tool.
// Either url OR (project_ref + build_number) must be provided.
type ResolveURLInput struct {
	URL         string `json:"url,omitempty"`          // e.g. "http://host/projects/1/reports/28"
	ProjectRef  string `json:"project_ref,omitempty"`  // numeric id OR slug; used when URL is absent
	BuildNumber int    `json:"build_number,omitempty"` // used when URL is absent
}

// ResolveURLOutput is the structured output for the resolve_url tool.
type ResolveURLOutput struct {
	ProjectID   int64  `json:"project_id"`
	ProjectSlug string `json:"project_slug"`
	DisplayName string `json:"display_name"`
	BuildID     int64  `json:"build_id"` // for downstream tool calls
	BuildNumber int    `json:"build_number"`
	Branch      string `json:"branch,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status_summary,omitempty"`
	HasFailures bool   `json:"has_failures"`
	ReportURL   string `json:"report_url"` // canonical UI link
}

// numericRe matches a string that is entirely decimal digits.
var numericRe = regexp.MustCompile(`^\d+$`)

// ---------------------------------------------------------------------------
// RegisterDiscoveryTools
// ---------------------------------------------------------------------------

// RegisterDiscoveryTools registers list_projects, list_recent_builds,
// find_test_by_name, and resolve_url on s.
func RegisterDiscoveryTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_projects",
		Description: "List alluredeck projects with pagination. Use to discover available project IDs before calling other tools.",
	}, listProjectsHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_recent_builds",
		Description: "List recent builds for a project, optionally filtered by branch. Returns build IDs needed for other tools.",
	}, listRecentBuildsHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "find_test_by_name",
		Description: "Search tests by name substring (case-insensitive). Returns up to 100 matches with their history_id for use in get_test_failure and get_test_history.",
	}, findTestByNameHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "resolve_url",
		Description: "Resolve a UI URL or (project_ref, build_number) pair to the build_id and project context needed by other tools. Call this first when given a URL — the build_number in the URL is NOT the build_id that other tools require.",
	}, resolveURLHandler(stores, logger))
}

func listProjectsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListProjectsInput) (*mcpsdk.CallToolResult, ListProjectsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListProjectsInput) (*mcpsdk.CallToolResult, ListProjectsOutput, error) {
		if in.Limit <= 0 {
			in.Limit = 50
		}
		if in.Limit > 200 {
			in.Limit = 200
		}

		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return nil, ListProjectsOutput{}, fmt.Errorf("invalid cursor: %w", err)
		}

		// page is 1-based; perPage is limit+1 for has-more detection.
		page := offset/in.Limit + 1
		rows, total, err := stores.Project.ListProjectsPaginated(ctx, page, in.Limit+1)
		if err != nil {
			return nil, ListProjectsOutput{}, fmt.Errorf("listing projects: %w", err)
		}

		hasMore := len(rows) > in.Limit
		if hasMore {
			rows = rows[:in.Limit]
		}

		items := make([]ProjectItem, len(rows))
		for i, p := range rows {
			items[i] = projectToItem(p)
		}

		var nextCursor string
		if hasMore {
			nextCursor = encodeCursor(offset + in.Limit)
		}

		_ = total // available for callers if needed
		return nil, ListProjectsOutput{Items: items, NextCursor: nextCursor}, nil
	}
}

func projectToItem(p store.Project) ProjectItem {
	item := ProjectItem{
		ProjectID:   p.ID,
		Slug:        p.Slug,
		DisplayName: p.DisplayName,
	}
	if p.ParentID != nil {
		item.ParentProjectID = p.ParentID
	}
	return item
}

func listRecentBuildsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListRecentBuildsInput) (*mcpsdk.CallToolResult, ListRecentBuildsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListRecentBuildsInput) (*mcpsdk.CallToolResult, ListRecentBuildsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListRecentBuildsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.Limit <= 0 {
			in.Limit = 20
		}
		if in.Limit > 100 {
			in.Limit = 100
		}

		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return nil, ListRecentBuildsOutput{}, fmt.Errorf("invalid cursor: %w", err)
		}

		var branchID *int64
		if in.Branch != "" {
			br, err := stores.Branch.GetByName(ctx, int64(in.ProjectID), in.Branch)
			if err != nil {
				// Branch not found — return empty, not an error.
				// This is intentional: an unknown branch name is not a user mistake
				// the same way an unknown build_id is. The caller may be probing
				// whether a branch exists, and returning an error here would break
				// scripts that legitimately check for a branch before building.
				return nil, ListRecentBuildsOutput{Items: nil}, nil
			}
			branchID = &br.ID
		}

		page := offset/in.Limit + 1
		builds, _, err := stores.Build.ListBuildsPaginatedBranch(ctx, int64(in.ProjectID), page, in.Limit+1, branchID)
		if err != nil {
			return nil, ListRecentBuildsOutput{}, fmt.Errorf("listing builds: %w", err)
		}

		hasMore := len(builds) > in.Limit
		if hasMore {
			builds = builds[:in.Limit]
		}

		items := make([]RecentBuildItem, len(builds))
		for i := range builds {
			items[i] = recentBuildItem(builds[i])
		}

		var nextCursor string
		if hasMore {
			nextCursor = encodeCursor(offset + in.Limit)
		}

		return nil, ListRecentBuildsOutput{Items: items, NextCursor: nextCursor}, nil
	}
}

// recentBuildItem converts a store.Build to a RecentBuildItem.
func recentBuildItem(b store.Build) RecentBuildItem {
	item := RecentBuildItem{
		BuildID:     b.ID,
		BuildNumber: b.BuildNumber,
		CreatedAt:   b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if b.CIBranch != nil {
		item.Branch = *b.CIBranch
	}
	if b.CICommitSHA != nil {
		item.CommitSHA = *b.CICommitSHA
	}
	if b.StatTotal != nil {
		passed := 0
		if b.StatPassed != nil {
			passed = *b.StatPassed
		}
		failed := 0
		if b.StatFailed != nil {
			failed = *b.StatFailed
		}
		broken := 0
		if b.StatBroken != nil {
			broken = *b.StatBroken
		}
		item.StatusSummary = fmt.Sprintf("total=%d passed=%d failed=%d broken=%d",
			*b.StatTotal, passed, failed, broken)
	}
	return item
}

func findTestByNameHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in FindTestByNameInput) (*mcpsdk.CallToolResult, FindTestByNameOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in FindTestByNameInput) (*mcpsdk.CallToolResult, FindTestByNameOutput, error) {
		if in.ProjectID <= 0 {
			return nil, FindTestByNameOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.NameSubstring == "" {
			return nil, FindTestByNameOutput{}, fmt.Errorf("name_substring must not be empty")
		}

		results, err := stores.TestResult.SearchByName(ctx, int64(in.ProjectID), in.NameSubstring, 100)
		if err != nil {
			return nil, FindTestByNameOutput{}, fmt.Errorf("searching tests: %w", err)
		}

		items := make([]TestNameItem, 0, len(results))
		for _, r := range results {
			items = append(items, TestNameItem{
				HistoryID:       r.HistoryID,
				FullName:        r.FullName,
				LastSeenBuildID: r.BuildID,
				LastSeenStatus:  r.Status,
			})
		}

		return nil, FindTestByNameOutput{Items: items}, nil
	}
}

func resolveURLHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ResolveURLInput) (*mcpsdk.CallToolResult, ResolveURLOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ResolveURLInput) (*mcpsdk.CallToolResult, ResolveURLOutput, error) {
		var projectRef string
		var buildNumber int

		if in.URL != "" {
			// Parse the UI URL: /projects/<proj>/reports/<num>
			parsed, err := url.Parse(in.URL)
			if err != nil {
				return nil, ResolveURLOutput{}, fmt.Errorf("invalid url %q: %w", in.URL, err)
			}
			m := reURLPath.FindStringSubmatch(parsed.Path)
			if m == nil {
				return nil, ResolveURLOutput{}, fmt.Errorf("url path %q does not match /projects/<proj>/reports/<num>", parsed.Path)
			}
			projectRef = m[reURLPath.SubexpIndex("proj")]
			num, err := strconv.Atoi(m[reURLPath.SubexpIndex("num")])
			if err != nil {
				return nil, ResolveURLOutput{}, fmt.Errorf("build_number in url is not an integer: %w", err)
			}
			buildNumber = num
		} else {
			// Use explicit project_ref + build_number.
			if in.ProjectRef == "" {
				return nil, ResolveURLOutput{}, fmt.Errorf("either url or project_ref must be provided")
			}
			if in.BuildNumber <= 0 {
				return nil, ResolveURLOutput{}, fmt.Errorf("build_number must be positive when url is absent")
			}
			projectRef = in.ProjectRef
			buildNumber = in.BuildNumber
		}

		// Resolve project: numeric id → GetProject, slug → GetProjectBySlug.
		var proj *store.Project
		var err error
		if numericRe.MatchString(projectRef) {
			id, _ := strconv.ParseInt(projectRef, 10, 64)
			proj, err = stores.Project.GetProject(ctx, id)
			if err != nil {
				return nil, ResolveURLOutput{}, fmt.Errorf("project not found (id=%s): %w", projectRef, err)
			}
		} else {
			proj, err = stores.Project.GetProjectBySlug(ctx, projectRef)
			if err != nil {
				return nil, ResolveURLOutput{}, fmt.Errorf("project not found (slug=%q): %w", projectRef, err)
			}
		}
		if proj == nil {
			return nil, ResolveURLOutput{}, fmt.Errorf("project %q not found", projectRef)
		}

		// Resolve build_number → build row.
		b, err := stores.Build.GetBuildByNumber(ctx, proj.ID, buildNumber)
		if err != nil {
			return nil, ResolveURLOutput{}, fmt.Errorf("build #%d not found in project %q: %w", buildNumber, proj.Slug, err)
		}

		out := ResolveURLOutput{
			ProjectID:   proj.ID,
			ProjectSlug: proj.Slug,
			DisplayName: proj.DisplayName,
			BuildID:     b.ID,
			BuildNumber: b.BuildNumber,
			CreatedAt:   b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			ReportURL:   fmt.Sprintf("/projects/%d/reports/%d", proj.ID, b.BuildNumber),
		}
		if b.CIBranch != nil {
			out.Branch = *b.CIBranch
		}
		if b.CICommitSHA != nil {
			out.CommitSHA = *b.CICommitSHA
		}
		if b.StatTotal != nil && b.StatFailed != nil && b.StatBroken != nil {
			failed := *b.StatFailed + *b.StatBroken
			out.HasFailures = failed > 0
			out.Status = fmt.Sprintf("total=%d passed=%d failed=%d broken=%d",
				*b.StatTotal,
				func() int {
					if b.StatPassed != nil {
						return *b.StatPassed
					}
					return 0
				}(),
				*b.StatFailed,
				*b.StatBroken,
			)
		}

		return nil, out, nil
	}
}
