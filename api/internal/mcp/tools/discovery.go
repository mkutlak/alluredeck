package tools

import (
	"context"
	"fmt"

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
	HistoryID      string `json:"history_id"`
	FullName       string `json:"full_name"`
	LastSeenBuildID int64 `json:"last_seen_build_id"`
	LastSeenStatus string `json:"last_seen_status"`
}

// FindTestByNameOutput is the structured output for find_test_by_name.
type FindTestByNameOutput struct {
	Items []TestNameItem `json:"items"`
}

// ---------------------------------------------------------------------------
// RegisterDiscoveryTools
// ---------------------------------------------------------------------------

// RegisterDiscoveryTools registers list_projects, list_recent_builds, and
// find_test_by_name on s.
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
		for i, b := range builds {
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
			items[i] = item
		}

		var nextCursor string
		if hasMore {
			nextCursor = encodeCursor(offset + in.Limit)
		}

		return nil, ListRecentBuildsOutput{Items: items, NextCursor: nextCursor}, nil
	}
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
