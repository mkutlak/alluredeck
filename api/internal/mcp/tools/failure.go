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
	ProjectID   int    `json:"project_id"`
	BuildID     int    `json:"build_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	SummaryOnly bool   `json:"summary_only,omitempty"`
}

// FailingTestItem is one result row returned by the list_failing_tests tool.
type FailingTestItem struct {
	TestResultID    int64  `json:"test_result_id"`
	BuildID         int    `json:"build_id"`
	HistoryID       string `json:"history_id"`
	FullName        string `json:"full_name"`
	Status          string `json:"status"`
	Retries         int    `json:"retries"`
	Flaky           bool   `json:"flaky"`
	FingerprintHash string `json:"fingerprint_hash,omitempty"`
	KnownIssueID    int64  `json:"known_issue_id,omitempty"`
}

// FailingSummary is returned when summary_only=true.
type FailingSummary struct {
	TotalFailed int            `json:"total_failed"`
	Statuses    map[string]int `json:"statuses"`
}

// BuildRef holds lightweight build metadata hoisted to the top-level output.
type BuildRef struct {
	BuildNumber int    `json:"build_number"`
	Branch      string `json:"branch,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
}

// ListFailingTestsOutput is the structured output for the list_failing_tests tool.
type ListFailingTestsOutput struct {
	Items      []FailingTestItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Build      *BuildRef         `json:"build,omitempty"`
	Summary    *FailingSummary   `json:"summary,omitempty"`
}

// RegisterFailureTools registers the failure-related MCP tools on s.
func RegisterFailureTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_failing_tests",
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

		// Decode pagination cursor to get the offset (used for NextCursor calculation).
		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return nil, ListFailingTestsOutput{}, fmt.Errorf("invalid cursor: %w", err)
		}

		// Resolve build ID.
		buildID := int64(in.BuildID)
		var resolvedBuild store.Build
		if buildID == 0 {
			latest, err := stores.Build.GetLatestBuild(ctx, int64(in.ProjectID))
			if err != nil {
				if errors.Is(err, store.ErrBuildNotFound) {
					return nil, ListFailingTestsOutput{Items: nil}, nil
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

		// Fetch limit+1 rows to detect whether a next page exists.
		rows, err := stores.TestResult.ListFailedByBuild(ctx, int64(in.ProjectID), buildID, in.Limit+1)
		if err != nil {
			return nil, ListFailingTestsOutput{}, fmt.Errorf("listing failing tests: %w", err)
		}

		// Determine if there are more results.
		hasMore := len(rows) > in.Limit
		if hasMore {
			rows = rows[:in.Limit]
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
			if br.BuildNumber > 0 {
				out.Build = br
			}
		}

		if in.SummaryOnly {
			statuses := make(map[string]int)
			for i := range rows {
				statuses[rows[i].Status]++
			}
			out.Summary = &FailingSummary{
				TotalFailed: len(rows),
				Statuses:    statuses,
			}
			out.Items = []FailingTestItem{}
			return nil, out, nil
		}

		// Map store rows to output items.
		items := make([]FailingTestItem, len(rows))
		for i := range rows {
			items[i] = FailingTestItem{
				BuildID:   int(rows[i].BuildID),
				HistoryID: rows[i].HistoryID,
				FullName:  rows[i].FullName,
				Status:    rows[i].Status,
				Retries:   rows[i].Retries,
				Flaky:     rows[i].Flaky,
			}
		}
		out.Items = items

		// Build next cursor if more results exist.
		if hasMore {
			out.NextCursor = encodeCursor(offset + in.Limit)
		}

		return nil, out, nil
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
