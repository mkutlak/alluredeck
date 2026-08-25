package tools

import (
	"context"
	"errors"
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
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of projects to return per page. Defaults to 50, clamped to 200."`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque pagination cursor from a previous call's next_cursor. Omit to start from the first page."`
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
	// Total is the total number of projects across all pages.
	Total int `json:"total"`
}

// ---------------------------------------------------------------------------
// list_recent_builds
// ---------------------------------------------------------------------------

// ListRecentBuildsInput holds parameters for list_recent_builds.
type ListRecentBuildsInput struct {
	ProjectID int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	Branch    string `json:"branch,omitempty" jsonschema:"Restrict results to a single branch by name. An unknown branch returns an empty list rather than an error."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of builds to return per page. Defaults to 20, clamped to 100."`
	Cursor    string `json:"cursor,omitempty" jsonschema:"Opaque pagination cursor from a previous call's next_cursor. Omit to start from the first page."`
}

// RecentBuildItem is one build in the list_recent_builds response.
type RecentBuildItem struct {
	BuildID       int64  `json:"build_id"`
	BuildNumber   int    `json:"build_number"`
	Branch        string `json:"branch,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CreatedAt     string `json:"created_at"`
	StatusSummary string `json:"status_summary,omitempty"`
	// CIPipelineID identifies the CI pipeline run this build belongs to.
	// Playwright CI shards upload one build each under a shared pipeline ID —
	// a non-empty value here means sibling shard builds may exist; pass it to
	// diagnose_pipeline to check all of them at once.
	CIPipelineID string `json:"ci_pipeline_id,omitempty"`
	// CIPipelineURL is the CI system's link to the pipeline run.
	CIPipelineURL string `json:"ci_pipeline_url,omitempty"`
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
	ProjectID     int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	NameSubstring string `json:"name_substring" jsonschema:"Case-insensitive substring to match against test full names. Required, non-empty."`
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
// alluredeck UI. <proj> may be a numeric project_id or a slug string. Anchored
// at ^ so that sub-paths like /foo/projects/x/reports/1 are rejected. The
// trailing (?:/.*)? tolerates deeper UI paths under a report — a trailing
// slash, /suites, /test/<id>, etc. — since diagnose_failure shares this regex
// and UI links commonly point at a sub-view rather than the report root; the
// tail is ignored either way.
var reURLPath = regexp.MustCompile(`^/projects/(?P<proj>[^/]+)/reports/(?P<num>\d+)(?:/.*)?$`)

// ResolveURLInput holds parameters for the resolve_url tool.
// Either url OR (project_ref + build_number) must be provided.
type ResolveURLInput struct {
	// URL is the alluredeck UI URL, e.g. "http://host/projects/1/reports/28".
	// A trailing sub-path (/suites, /test/<id>, a trailing slash) is accepted
	// and ignored.
	URL         string `json:"url,omitempty" jsonschema:"AllureDeck UI URL, e.g. http://host/projects/<proj>/reports/<num>[/...]. Either url or (project_ref + build_number) must be provided."`
	ProjectRef  string `json:"project_ref,omitempty" jsonschema:"Numeric project id or slug; used only when url is absent."`
	BuildNumber int    `json:"build_number,omitempty" jsonschema:"UI build number (NOT build_id); used only when url is absent."`
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
	// CIPipelineID identifies the CI pipeline run this build belongs to.
	// Playwright CI shards upload one build each under a shared pipeline ID —
	// a non-empty value here means sibling shard builds may exist; pass it to
	// diagnose_pipeline to check all of them at once.
	CIPipelineID string `json:"ci_pipeline_id,omitempty"`
	// CIPipelineURL is the CI system's link to the pipeline run.
	CIPipelineURL string `json:"ci_pipeline_url,omitempty"`
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
		Title:       "List AllureDeck projects",
		Annotations: readOnlyAnnotations(),
		Description: "List alluredeck projects with pagination. Use to discover available project IDs before calling other tools.",
	}, listProjectsHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_recent_builds",
		Title:       "List AllureDeck recent builds",
		Annotations: readOnlyAnnotations(),
		Description: "List recent builds for a project, optionally filtered by branch. Returns build IDs needed for other tools.",
	}, listRecentBuildsHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "find_test_by_name",
		Title:       "Find AllureDeck test by name",
		Annotations: readOnlyAnnotations(),
		Description: "Search tests by name substring (case-insensitive). Returns up to 100 matches with their history_id for use in get_test_failure and get_test_history.",
	}, findTestByNameHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "resolve_url",
		Title:       "Resolve AllureDeck URL",
		Annotations: readOnlyAnnotations(),
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

		// page is 1-based, sized exactly to in.Limit (not in.Limit+1): offset is
		// always a multiple of in.Limit because nextCursor below only ever
		// advances by len(rows), which equals in.Limit on every page that has
		// more results. A page size that matched the cursor's own stride is
		// what a "limit+1 peek" trick got wrong here before — that trick
		// requires perPage to equal the cursor stride too, and it used limit+1
		// for one and limit for the other, so page 2 silently started one row
		// past where page 1 said it would.
		page := offset/in.Limit + 1
		rows, total, err := stores.Project.ListProjectsPaginated(ctx, page, in.Limit)
		if err != nil {
			return nil, ListProjectsOutput{}, fmt.Errorf("listing projects: %w", err)
		}

		items := make([]ProjectItem, len(rows))
		for i, p := range rows {
			items[i] = projectToItem(p)
		}

		var nextCursor string
		if offset+len(rows) < total {
			nextCursor = encodeCursor(offset + len(rows))
		}

		digest := fmt.Sprintf("%d project(s), showing %d", total, len(items))
		return textResult(digest), ListProjectsOutput{Items: items, NextCursor: nextCursor, Total: total}, nil
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
				// A genuinely absent branch is not a user mistake (the caller may
				// be probing) — return empty, not an error. Any other error is an
				// infrastructure failure and must surface, otherwise a transient DB
				// fault masquerades as "no builds on this branch". Mirrors
				// get_test_history's branch handling.
				if errors.Is(err, store.ErrBranchNotFound) {
					return textResult(fmt.Sprintf("branch %q not found in project %d; no builds", in.Branch, in.ProjectID)),
						ListRecentBuildsOutput{Items: nil}, nil
				}
				return nil, ListRecentBuildsOutput{}, fmt.Errorf("resolving branch %q: %w", in.Branch, err)
			}
			branchID = &br.ID
		}

		// page is sized exactly to in.Limit; see the matching comment in
		// listProjectsHandler for why perPage must equal the cursor's stride.
		page := offset/in.Limit + 1
		builds, total, err := stores.Build.ListBuildsPaginatedBranch(ctx, int64(in.ProjectID), page, in.Limit, branchID)
		if err != nil {
			return nil, ListRecentBuildsOutput{}, fmt.Errorf("listing builds: %w", err)
		}

		items := make([]RecentBuildItem, len(builds))
		for i := range builds {
			items[i] = recentBuildItem(builds[i])
		}

		var nextCursor string
		if offset+len(builds) < total {
			nextCursor = encodeCursor(offset + len(builds))
		}

		digest := fmt.Sprintf("%d build(s) for project %d, showing %d", total, in.ProjectID, len(items))
		if in.Branch != "" {
			digest += " on branch " + in.Branch
		}
		return textResult(digest), ListRecentBuildsOutput{Items: items, NextCursor: nextCursor}, nil
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
	if b.CIPipelineID != nil {
		item.CIPipelineID = *b.CIPipelineID
	}
	if b.CIPipelineURL != nil {
		item.CIPipelineURL = *b.CIPipelineURL
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

		digest := fmt.Sprintf("%d test(s) matching %q", len(items), in.NameSubstring)
		return textResult(digest), FindTestByNameOutput{Items: items}, nil
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
		if b.CIPipelineID != nil {
			out.CIPipelineID = *b.CIPipelineID
		}
		if b.CIPipelineURL != nil {
			out.CIPipelineURL = *b.CIPipelineURL
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

		digest := fmt.Sprintf("build #%d in project %s -> build_id %d", out.BuildNumber, out.ProjectSlug, out.BuildID)
		return textResult(digest), out, nil
	}
}
