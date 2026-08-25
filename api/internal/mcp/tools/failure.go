package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ListFailingTestsInput holds the parameters for the list_failing_tests tool.
type ListFailingTestsInput struct {
	ProjectID int `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	BuildID   int `json:"build_id,omitempty" jsonschema:"Internal build id from resolve_url or list_recent_builds. Omit to use the project's latest build. The build_number in a UI URL is NOT the build_id."`
	// Branch is accepted but currently has no effect: the underlying query has
	// no branch filter. It stays in the input schema, marked as such, so a
	// caller does not silently assume it narrowed the result set.
	Branch      string `json:"branch,omitempty" jsonschema:"Currently a no-op: this field is accepted but not applied as a filter; results are not narrowed by branch."`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of failing tests to return. Defaults to 50, clamped to 200."`
	Cursor      string `json:"cursor,omitempty" jsonschema:"Deprecated: accepted but ignored. This tool has no cursor pagination; raise limit instead."`
	SummaryOnly bool   `json:"summary_only,omitempty" jsonschema:"When true, omit the items list and return only summary counts (total_failed, distinct_failed, status breakdown)."`
}

// FailingTestItem is one result row returned by the list_failing_tests tool.
type FailingTestItem struct {
	TestResultID int64  `json:"test_result_id"`
	BuildID      int    `json:"build_id"`
	HistoryID    string `json:"history_id"`
	FullName     string `json:"full_name"`
	Status       string `json:"status"`
	Retries      int    `json:"retries"`
	Flaky        bool   `json:"flaky"`
}

// FailingSummary is returned when summary_only=true.
type FailingSummary struct {
	// TotalFailed is the build's raw failed+broken count (builds.stat_failed +
	// builds.stat_broken) when those stats are available, falling back to
	// CountFailedByBuild's distinct count otherwise. It counts rows the way
	// the Allure report itself does, which can exceed DistinctFailed when a
	// test is ingested under two history_id schemes (see DistinctFailed).
	TotalFailed int `json:"total_failed"`
	// DistinctFailed is COUNT(DISTINCT full_name) among failed+broken rows —
	// the dedup-aware number a human reading the report would call "tests
	// failing", immune to the legacy double-ingestion of a test under two
	// history_id schemes.
	DistinctFailed int            `json:"distinct_failed"`
	Statuses       map[string]int `json:"statuses"`
}

// BuildRef holds lightweight build metadata hoisted to the top-level output.
type BuildRef struct {
	BuildNumber int    `json:"build_number"`
	Branch      string `json:"branch,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	// CIPipelineID identifies the CI pipeline run this build belongs to.
	// Playwright CI shards upload one build each under a shared pipeline ID —
	// a non-empty value here means sibling shard builds may exist; pass it to
	// diagnose_pipeline to check all of them at once.
	CIPipelineID string `json:"ci_pipeline_id,omitempty"`
	// CIPipelineURL is the CI system's link to the pipeline run.
	CIPipelineURL string `json:"ci_pipeline_url,omitempty"`
}

// ListFailingTestsOutput is the structured output for the list_failing_tests tool.
//
// There is no next_cursor: ListFailedByBuild has no offset pagination, so a
// cursor this tool emitted could never advance past the first page — every
// "page 2" call would silently re-return page 1. A caller detects that
// further failures may exist beyond this page when len(items) == limit.
type ListFailingTestsOutput struct {
	Items   []FailingTestItem `json:"items"`
	Build   *BuildRef         `json:"build,omitempty"`
	Summary *FailingSummary   `json:"summary,omitempty"`
}

// RegisterFailureTools registers the failure-related MCP tools on s.
func RegisterFailureTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_failing_tests",
		Title:       "List AllureDeck failing tests",
		Annotations: readOnlyAnnotations(),
		Description: "List tests that failed in a build. Use this first when debugging a CI failure; combine with get_test_failure for details on a specific test. URL build_number is NOT build_id — call resolve_url first or use list_recent_builds.",
	}, listFailingTestsHandler(stores, logger))
}

// listFailingTestsHandler returns the handler function for the list_failing_tests tool.
func listFailingTestsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListFailingTestsInput) (*mcpsdk.CallToolResult, ListFailingTestsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListFailingTestsInput) (*mcpsdk.CallToolResult, ListFailingTestsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListFailingTestsOutput{}, fmt.Errorf("project_id must be positive")
		}

		// Clamp limit.
		if in.Limit <= 0 {
			in.Limit = 50
		}
		if in.Limit > 200 {
			in.Limit = 200
		}

		// in.Cursor is deliberately ignored: ListFailedByBuild has no offset
		// pagination, so a cursor this tool emitted could never move the
		// window — every "next page" call would silently re-return page one.
		// It stays in the input schema, marked deprecated, so existing
		// callers do not fail on an unknown argument.

		// Resolve build ID.
		buildID := int64(in.BuildID)
		var resolvedBuild store.Build
		if buildID == 0 {
			latest, err := stores.Build.GetLatestBuild(ctx, int64(in.ProjectID))
			if err != nil {
				if errors.Is(err, store.ErrBuildNotFound) {
					return textResult(fmt.Sprintf("no builds found for project %d", in.ProjectID)),
						ListFailingTestsOutput{Items: nil}, nil
				}
				return nil, ListFailingTestsOutput{}, fmt.Errorf("resolving latest build: %w", err)
			}
			buildID = latest.ID
			resolvedBuild = latest
		} else {
			// Fetch the build row — doubles as existence check and source of
			// CI metadata for the top-level `build` hoist (consistent with
			// compare_builds, which always returns it for a resolved target).
			b, err := stores.Build.GetBuildByID(ctx, int64(in.ProjectID), buildID)
			if err != nil {
				if errors.Is(err, store.ErrBuildNotFound) {
					return nil, ListFailingTestsOutput{}, fmt.Errorf(
						"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map)",
						buildID, in.ProjectID,
					)
				}
				return nil, ListFailingTestsOutput{}, fmt.Errorf("fetching build: %w", err)
			}
			resolvedBuild = b
		}

		// Fetch exactly limit rows: without a cursor there is no next page to
		// probe for, and len(items) == limit already tells the caller further
		// failures may exist beyond this page.
		rows, err := stores.TestResult.ListFailedByBuild(ctx, int64(in.ProjectID), buildID, in.Limit)
		if err != nil {
			return nil, ListFailingTestsOutput{}, fmt.Errorf("listing failing tests: %w", err)
		}

		out := ListFailingTestsOutput{}

		// Hoist build metadata when results are non-empty.
		if len(rows) > 0 || resolvedBuild.ID != 0 {
			br := &BuildRef{BuildNumber: resolvedBuild.BuildNumber}
			if resolvedBuild.CIBranch != nil {
				br.Branch = *resolvedBuild.CIBranch
			}
			if resolvedBuild.CICommitSHA != nil {
				br.CommitSHA = *resolvedBuild.CICommitSHA
			}
			if resolvedBuild.CIPipelineID != nil {
				br.CIPipelineID = *resolvedBuild.CIPipelineID
			}
			if resolvedBuild.CIPipelineURL != nil {
				br.CIPipelineURL = *resolvedBuild.CIPipelineURL
			}
			if br.BuildNumber > 0 {
				out.Build = br
			}
		}

		// distinctFailed is the dedup-aware failure count; totalFailed prefers
		// the build's own raw stats (already fetched, no extra query) and
		// falls back to the same distinct count when the build predates stat
		// tracking (StatFailed/StatBroken nil).
		distinctFailed, err := stores.TestResult.CountFailedByBuild(ctx, int64(in.ProjectID), buildID)
		if err != nil {
			return nil, ListFailingTestsOutput{}, fmt.Errorf("counting failed tests: %w", err)
		}
		totalFailed := distinctFailed
		if resolvedBuild.StatFailed != nil && resolvedBuild.StatBroken != nil {
			totalFailed = *resolvedBuild.StatFailed + *resolvedBuild.StatBroken
		}

		if in.SummaryOnly {
			statuses := make(map[string]int)
			for i := range rows {
				statuses[rows[i].Status]++
			}
			out.Summary = &FailingSummary{
				TotalFailed:    totalFailed,
				DistinctFailed: distinctFailed,
				Statuses:       statuses,
			}
			out.Items = []FailingTestItem{}
			digest := fmt.Sprintf("build #%d: %d failing (%d distinct)", resolvedBuild.BuildNumber, totalFailed, distinctFailed)
			return textResult(digest), out, nil
		}

		// Map store rows to output items.
		items := make([]FailingTestItem, len(rows))
		for i := range rows {
			items[i] = FailingTestItem{
				TestResultID: rows[i].ID,
				BuildID:      int(rows[i].BuildID),
				HistoryID:    rows[i].HistoryID,
				FullName:     rows[i].FullName,
				Status:       rows[i].Status,
				Retries:      rows[i].Retries,
				Flaky:        rows[i].Flaky,
			}
		}
		out.Items = items

		digest := fmt.Sprintf("build #%d: %d failing (%d distinct), showing %d",
			resolvedBuild.BuildNumber, totalFailed, distinctFailed, len(items))
		return textResult(digest), out, nil
	}
}

// encodeCursor encodes an integer offset into an opaque base64url cursor string.
// Duplicated from internal/mcp/pagination.go to avoid a circular import.
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// decodeCursor decodes a cursor produced by encodeCursor back to an integer offset.
// Duplicated from internal/mcp/pagination.go to avoid a circular import.
func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor: %w", err)
	}
	offset, err := strconv.Atoi(string(b))
	if err != nil {
		return 0, fmt.Errorf("invalid cursor value: %w", err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("cursor offset must be non-negative")
	}
	return offset, nil
}
