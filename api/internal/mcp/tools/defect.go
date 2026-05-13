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
// get_defect_cluster
// ---------------------------------------------------------------------------

// GetDefectClusterInput holds parameters for get_defect_cluster.
type GetDefectClusterInput struct {
	ProjectID       int    `json:"project_id"`
	FingerprintHash string `json:"fingerprint_hash"`
}

// GetDefectClusterOutput is the structured output for get_defect_cluster.
type GetDefectClusterOutput struct {
	ID                     string `json:"id"`
	FingerprintHash        string `json:"fingerprint_hash"`
	NormalizedMessage      string `json:"normalized_message"`
	Category               string `json:"category"`
	Resolution             string `json:"resolution"`
	OccurrenceCount        int    `json:"occurrence_count"`
	FirstSeenBuildID       int64  `json:"first_seen_build_id"`
	LastSeenBuildID        int64  `json:"last_seen_build_id"`
	ConsecutiveCleanBuilds int    `json:"consecutive_clean_builds"`
	KnownIssueID           *int64 `json:"known_issue_id,omitempty"`
}

// ---------------------------------------------------------------------------
// list_defects
// ---------------------------------------------------------------------------

// ListDefectsInput holds parameters for list_defects.
type ListDefectsInput struct {
	ProjectID  int    `json:"project_id"`
	Category   string `json:"category,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
}

// DefectItem is one defect in the list_defects response.
type DefectItem struct {
	ID              string `json:"id"`
	FingerprintHash string `json:"fingerprint_hash"`
	NormalizedMessage string `json:"normalized_message"`
	Category        string `json:"category"`
	Resolution      string `json:"resolution"`
	OccurrenceCount int    `json:"occurrence_count"`
	LastSeenBuildID int64  `json:"last_seen_build_id"`
}

// ListDefectsOutput is the structured output for list_defects.
type ListDefectsOutput struct {
	Items      []DefectItem `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

// ---------------------------------------------------------------------------
// RegisterDefectTools
// ---------------------------------------------------------------------------

// RegisterDefectTools registers get_defect_cluster and list_defects on s.
func RegisterDefectTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_defect_cluster",
		Description: "Get details of a defect cluster by fingerprint hash. Use after get_test_failure to understand the deduplicated defect group.",
	}, getDefectClusterHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_defects",
		Description: "List defect fingerprints for a project with optional filters for category and resolution.",
	}, listDefectsHandler(stores, logger))
}

func getDefectClusterHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetDefectClusterInput) (*mcpsdk.CallToolResult, GetDefectClusterOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in GetDefectClusterInput) (*mcpsdk.CallToolResult, GetDefectClusterOutput, error) {
		if in.ProjectID <= 0 {
			return nil, GetDefectClusterOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.FingerprintHash == "" {
			return nil, GetDefectClusterOutput{}, fmt.Errorf("fingerprint_hash must not be empty")
		}

		fp, err := stores.Defect.GetByHash(ctx, int64(in.ProjectID), in.FingerprintHash)
		if err != nil {
			return nil, GetDefectClusterOutput{}, fmt.Errorf("fetching defect cluster: %w", err)
		}

		return nil, defectToClusterOutput(fp), nil
	}
}

func defectToClusterOutput(fp *store.DefectFingerprint) GetDefectClusterOutput {
	return GetDefectClusterOutput{
		ID:                     fp.ID,
		FingerprintHash:        fp.FingerprintHash,
		NormalizedMessage:      fp.NormalizedMessage,
		Category:               fp.Category,
		Resolution:             fp.Resolution,
		OccurrenceCount:        fp.OccurrenceCount,
		FirstSeenBuildID:       fp.FirstSeenBuildID,
		LastSeenBuildID:        fp.LastSeenBuildID,
		ConsecutiveCleanBuilds: fp.ConsecutiveCleanBuilds,
		KnownIssueID:           fp.KnownIssueID,
	}
}

func listDefectsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListDefectsInput) (*mcpsdk.CallToolResult, ListDefectsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListDefectsInput) (*mcpsdk.CallToolResult, ListDefectsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListDefectsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.Limit <= 0 {
			in.Limit = 50
		}
		if in.Limit > 200 {
			in.Limit = 200
		}

		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return nil, ListDefectsOutput{}, fmt.Errorf("invalid cursor: %w", err)
		}

		page := offset/in.Limit + 1
		filter := store.DefectFilter{
			Category:   in.Category,
			Resolution: in.Resolution,
			Page:       page,
			PerPage:    in.Limit + 1,
		}

		rows, _, err := stores.Defect.ListByProject(ctx, int64(in.ProjectID), filter)
		if err != nil {
			return nil, ListDefectsOutput{}, fmt.Errorf("listing defects: %w", err)
		}

		hasMore := len(rows) > in.Limit
		if hasMore {
			rows = rows[:in.Limit]
		}

		items := make([]DefectItem, len(rows))
		for i, r := range rows {
			items[i] = DefectItem{
				ID:                r.ID,
				FingerprintHash:   r.FingerprintHash,
				NormalizedMessage: r.NormalizedMessage,
				Category:          r.Category,
				Resolution:        r.Resolution,
				OccurrenceCount:   r.OccurrenceCount,
				LastSeenBuildID:   r.LastSeenBuildID,
			}
		}

		var nextCursor string
		if hasMore {
			nextCursor = encodeCursor(offset + in.Limit)
		}

		return nil, ListDefectsOutput{Items: items, NextCursor: nextCursor}, nil
	}
}
