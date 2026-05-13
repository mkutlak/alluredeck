package tools

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
)

// ---------------------------------------------------------------------------
// list_attachments
// ---------------------------------------------------------------------------

// ListAttachmentsInput holds parameters for list_attachments.
type ListAttachmentsInput struct {
	ProjectID int    `json:"project_id"`
	BuildID   int64  `json:"build_id"`
	HistoryID string `json:"history_id"`
}

// AttachmentItem is one attachment in the list_attachments response.
type AttachmentItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Mime        string `json:"mime"`
	SizeBytes   int64  `json:"size_bytes"`
	ResourceURI string `json:"resource_uri"`
}

// ListAttachmentsOutput is the structured output for list_attachments.
type ListAttachmentsOutput struct {
	Items []AttachmentItem `json:"items"`
}

// RegisterAttachmentTools registers list_attachments on s.
func RegisterAttachmentTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_attachments",
		Description: "List attachments for a specific test in a build. Returns resource URIs only — use the alluredeck://attachment/{id} resource to retrieve content.",
	}, listAttachmentsHandler(stores, logger))
}

func listAttachmentsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListAttachmentsInput) (*mcpsdk.CallToolResult, ListAttachmentsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListAttachmentsInput) (*mcpsdk.CallToolResult, ListAttachmentsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.BuildID <= 0 {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("build_id must be positive")
		}
		if in.HistoryID == "" {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("history_id must not be empty")
		}

		attachments, _, err := stores.Attachment.ListByBuild(ctx, int64(in.ProjectID), in.BuildID, "", "", 200, 0)
		if err != nil {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("listing attachments: %w", err)
		}

		items := make([]AttachmentItem, 0, len(attachments))
		for _, a := range attachments {
			items = append(items, AttachmentItem{
				ID:          a.ID,
				Name:        a.Name,
				Mime:        a.MimeType,
				SizeBytes:   a.SizeBytes,
				ResourceURI: fmt.Sprintf("alluredeck://attachment/%d", a.ID),
			})
		}

		return nil, ListAttachmentsOutput{Items: items}, nil
	}
}
