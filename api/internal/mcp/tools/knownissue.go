package tools

import (
	"context"
	"fmt"
	"regexp"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
)

// ---------------------------------------------------------------------------
// match_known_issues
// ---------------------------------------------------------------------------

// MatchKnownIssuesInput holds parameters for match_known_issues.
type MatchKnownIssuesInput struct {
	ProjectID    int    `json:"project_id"`
	ErrorMessage string `json:"error_message"`
}

// KnownIssueMatch is one match returned by match_known_issues.
type KnownIssueMatch struct {
	KnownIssueID    int64  `json:"known_issue_id"`
	Name            string `json:"name"`
	RegexPattern    string `json:"regex_pattern"`
	MatchedSubstring string `json:"matched_substring"`
}

// MatchKnownIssuesOutput is the structured output for match_known_issues.
type MatchKnownIssuesOutput struct {
	Items []KnownIssueMatch `json:"items"`
}

// RegisterKnownIssueTools registers match_known_issues on s.
func RegisterKnownIssueTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "match_known_issues",
		Description: "Match an error message against all active known-issue regex patterns for a project. Returns every pattern that matches.",
	}, matchKnownIssuesHandler(stores, logger))
}

func matchKnownIssuesHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in MatchKnownIssuesInput) (*mcpsdk.CallToolResult, MatchKnownIssuesOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in MatchKnownIssuesInput) (*mcpsdk.CallToolResult, MatchKnownIssuesOutput, error) {
		if in.ProjectID <= 0 {
			return nil, MatchKnownIssuesOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.ErrorMessage == "" {
			return nil, MatchKnownIssuesOutput{}, fmt.Errorf("error_message must not be empty")
		}

		issues, err := stores.KnownIssue.List(ctx, int64(in.ProjectID), true)
		if err != nil {
			return nil, MatchKnownIssuesOutput{}, fmt.Errorf("listing known issues: %w", err)
		}

		matches := make([]KnownIssueMatch, 0)
		for _, ki := range issues {
			re, err := regexp.Compile(ki.Pattern)
			if err != nil {
				// Fail-soft: skip patterns that fail to compile.
				continue
			}
			loc := re.FindStringIndex(in.ErrorMessage)
			if loc == nil {
				continue
			}
			matches = append(matches, KnownIssueMatch{
				KnownIssueID:    ki.ID,
				Name:            ki.TestName,
				RegexPattern:    ki.Pattern,
				MatchedSubstring: in.ErrorMessage[loc[0]:loc[1]],
			})
		}

		return nil, MatchKnownIssuesOutput{Items: matches}, nil
	}
}
