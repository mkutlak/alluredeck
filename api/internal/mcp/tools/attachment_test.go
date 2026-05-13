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

func buildStoresAttachment(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Attachment: mocks.Attachments,
	}
}

func decodeListAttachments(t *testing.T, res *mcpsdk.CallToolResult) tools.ListAttachmentsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ListAttachmentsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListAttachmentsOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// list_attachments
// ---------------------------------------------------------------------------

func TestListAttachments_HappyPath(t *testing.T) {
	mocks := testutil.New()
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return []store.TestAttachment{
			{ID: 1, Name: "screenshot.png", MimeType: "image/png", SizeBytes: 8192},
			{ID: 2, Name: "log.txt", MimeType: "text/plain", SizeBytes: 1024},
		}, 2, nil
	}

	cs := setupTestServer(t, buildStoresAttachment(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListAttachments(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].ResourceURI != "alluredeck://attachment/1" {
		t.Errorf("want resource_uri=alluredeck://attachment/1, got %q", out.Items[0].ResourceURI)
	}
	if out.Items[1].ResourceURI != "alluredeck://attachment/2" {
		t.Errorf("want resource_uri=alluredeck://attachment/2, got %q", out.Items[1].ResourceURI)
	}
	// Verify no inline content — only URI is returned.
	if out.Items[0].Name != "screenshot.png" {
		t.Errorf("want name=screenshot.png, got %q", out.Items[0].Name)
	}
	if out.Items[0].Mime != "image/png" {
		t.Errorf("want mime=image/png, got %q", out.Items[0].Mime)
	}
	if out.Items[0].SizeBytes != 8192 {
		t.Errorf("want size_bytes=8192, got %d", out.Items[0].SizeBytes)
	}
}

func TestListAttachments_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresAttachment(mocks))
	ctx := context.Background()

	// Missing history_id.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"project_id": 1, "build_id": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty history_id")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}

	// Missing build_id.
	res3, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"project_id": 1, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res3.IsError {
		t.Fatal("want IsError=true for build_id=0")
	}
}

func TestListAttachments_Empty(t *testing.T) {
	mocks := testutil.New()
	mocks.Attachments.ListByBuildFn = func(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
		return nil, 0, nil
	}

	cs := setupTestServer(t, buildStoresAttachment(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"project_id": 1, "build_id": 10, "history_id": "h1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListAttachments(t, res)
	if len(out.Items) != 0 {
		t.Errorf("want 0 items, got %d", len(out.Items))
	}
}
