package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func buildStoresAttachment(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Attachment: mocks.Attachments,
	}
}

// setupAttachmentContentServer wires a real MCP server with only get_attachment
// registered (via RegisterAttachmentContentTool, which needs the
// signing/storage dependencies that setupTestServer's RegisterAll call
// doesn't take) and returns a connected ClientSession.
func setupAttachmentContentServer(
	t *testing.T,
	stores *bootstrap.Stores,
	dataStore storage.Store,
	signingKey []byte,
	publicURL string,
) *mcpsdk.ClientSession {
	t.Helper()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-attachment-content", Version: "v0"}, nil)
	tools.RegisterAttachmentContentTool(srv, stores, zap.NewNop(), signingKey, publicURL, dataStore)

	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func decodeGetAttachment(t *testing.T, res *mcpsdk.CallToolResult) tools.GetAttachmentOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetAttachmentOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetAttachmentOutput: %v", err)
	}
	return out
}

// textServingStore returns a storage.MockStore whose OpenReportFile always
// serves content, regardless of the requested path.
func textServingStore(content string) *storage.MockStore {
	return &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader(content)), "text/plain", nil
		},
	}
}

// assertNoInlinedAttachmentContent verifies that a get_attachment result
// carries no ImageContent block and no TextContent block holding actual
// attachment bytes. The fallback branches (non-text/non-image MIME, oversized
// image, no storage backend) return a single one-line digest instead, so the
// client is not billed for the structured payload twice.
func assertNoInlinedAttachmentContent(t *testing.T, res *mcpsdk.CallToolResult) {
	t.Helper()
	for _, c := range res.Content {
		if _, ok := c.(*mcpsdk.ImageContent); ok {
			t.Fatalf("want no inlined ImageContent block, got one")
		}
	}
	if len(res.Content) != 1 {
		t.Fatalf("want exactly one digest content block, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("want *mcpsdk.TextContent digest, got %T", res.Content[0])
	}
	// A digest is a headline. The SDK's auto-fill (which happens only when a
	// handler returns a nil result) would instead echo the whole structured
	// output as JSON — recognisable by its leading brace.
	if strings.HasPrefix(strings.TrimSpace(tc.Text), "{") {
		t.Fatalf("want a compact digest, got the structured payload echoed as text: %q", tc.Text)
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
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, _ int64, _ int64, _ string, _ int) ([]store.TestAttachment, error) {
		return []store.TestAttachment{
			{ID: 1, Name: "screenshot.png", MimeType: "image/png", SizeBytes: 8192},
			{ID: 2, Name: "log.txt", MimeType: "text/plain", SizeBytes: 1024},
		}, nil
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
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, _ int64, _ int64, _ string, _ int) ([]store.TestAttachment, error) {
		return nil, nil
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

// TestListAttachments_ScopedToHistoryID pins the scoping fix: list_attachments
// must call ListByTestResult with (project_id, build_id, history_id) so it
// returns only the requested test's own attachments, not every attachment in
// the build. Passing a wrong history_id must not silently substitute a
// build-wide listing.
func TestListAttachments_ScopedToHistoryID(t *testing.T) {
	mocks := testutil.New()

	var gotProjectID, gotBuildID int64
	var gotHistoryID string
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, projectID int64, buildID int64, historyID string, _ int) ([]store.TestAttachment, error) {
		gotProjectID, gotBuildID, gotHistoryID = projectID, buildID, historyID
		if historyID != "h-target" {
			// A build-wide listing (the old ListByBuild bug) would return
			// attachments for every test regardless of historyID.
			return []store.TestAttachment{{ID: 99, Name: "other-test-attachment.png"}}, nil
		}
		return []store.TestAttachment{{ID: 1, Name: "scoped.log", MimeType: "text/plain", SizeBytes: 10}}, nil
	}

	cs := setupTestServer(t, buildStoresAttachment(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_attachments",
		Arguments: map[string]any{"project_id": 5, "build_id": 77, "history_id": "h-target"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	if gotProjectID != 5 || gotBuildID != 77 || gotHistoryID != "h-target" {
		t.Fatalf("ListByTestResult called with (%d, %d, %q), want (5, 77, %q)", gotProjectID, gotBuildID, gotHistoryID, "h-target")
	}

	out := decodeListAttachments(t, res)
	if len(out.Items) != 1 || out.Items[0].Name != "scoped.log" {
		t.Fatalf("want scoped result [scoped.log], got %+v", out.Items)
	}
}

// ---------------------------------------------------------------------------
// get_attachment
// ---------------------------------------------------------------------------

func TestGetAttachment_TextFullyInlined(t *testing.T) {
	const content = "hello from attachment text file"
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "log.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
	}
	mocks.Attachments.GetByIDFn = func(_ context.Context, id int64) (*store.TestAttachment, error) {
		return &store.TestAttachment{ID: id, Name: "log.txt", MimeType: "text/plain"}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore(content), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content block, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("want *mcpsdk.TextContent, got %T", res.Content[0])
	}
	if tc.Text != content {
		t.Errorf("want text=%q, got %q", content, tc.Text)
	}

	out := decodeGetAttachment(t, res)
	if out.Name != "log.txt" {
		t.Errorf("want name=log.txt, got %q", out.Name)
	}
	if out.SizeBytes != int64(len(content)) {
		t.Errorf("want size_bytes=%d, got %d", len(content), out.SizeBytes)
	}
	if out.ReturnedBytes != len(content) {
		t.Errorf("want returned_bytes=%d, got %d", len(content), out.ReturnedBytes)
	}
	if out.Truncated {
		t.Error("want truncated=false for content fully within max_bytes")
	}
	if out.SignedURL != "" {
		t.Errorf("want no signed_url for fully-inlined content, got %q", out.SignedURL)
	}
}

func TestGetAttachment_TextTruncated(t *testing.T) {
	const content = "0123456789"
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "big.log", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore(content), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1, "max_bytes": 4},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("want *mcpsdk.TextContent, got %T", res.Content[0])
	}
	wantMarker := "\n…[truncated: bytes 0-4 of 10; call again with offset=4]"
	if !strings.HasSuffix(tc.Text, wantMarker) {
		t.Errorf("want text to end with truncation marker %q, got %q", wantMarker, tc.Text)
	}
	if !strings.HasPrefix(tc.Text, "0123") {
		t.Errorf("want text to start with window content %q, got %q", "0123", tc.Text)
	}

	out := decodeGetAttachment(t, res)
	if !out.Truncated {
		t.Error("want truncated=true")
	}
	if out.ReturnedBytes != 4 {
		t.Errorf("want returned_bytes=4, got %d", out.ReturnedBytes)
	}
	if out.SignedURL == "" {
		t.Error("want signed_url set for truncated content")
	}
	if !strings.HasPrefix(out.SignedURL, "http://localhost:8080/attachments/1?") {
		t.Errorf("want signed_url to point at attachment 1, got %q", out.SignedURL)
	}
}

func TestGetAttachment_OffsetWindow(t *testing.T) {
	const content = "0123456789"
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "big.log", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore(content), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1, "offset": 4, "max_bytes": 6},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	tc := res.Content[0].(*mcpsdk.TextContent) //nolint:errcheck
	// offset=4, max_bytes=6 covers bytes [4,10) — the remainder of the
	// 10-byte file — so this is NOT truncated and carries no marker.
	if tc.Text != "456789" {
		t.Errorf("want text=%q, got %q", "456789", tc.Text)
	}

	out := decodeGetAttachment(t, res)
	if out.Offset != 4 {
		t.Errorf("want offset=4, got %d", out.Offset)
	}
	if out.ReturnedBytes != 6 {
		t.Errorf("want returned_bytes=6, got %d", out.ReturnedBytes)
	}
	if out.Truncated {
		t.Error("want truncated=false: window reaches end of file")
	}
}

func TestGetAttachment_MaxBytesClamping(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{"zero uses default", 0, 65536},
		{"negative clamps to 1", -5, 1},
		{"over cap clamps to 2MiB", 10 * 1024 * 1024, 2 * 1024 * 1024},
		{"within range passes through", 100, 100},
	}

	content := strings.Repeat("x", 3*1024*1024) // 3 MiB source, larger than any clamp ceiling

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
				return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "huge.log", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
			}

			cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore(content), []byte("key"), "http://localhost:8080")
			ctx := context.Background()

			args := map[string]any{"project_id": 1, "attachment_id": 1}
			if tc.requested != 0 {
				args["max_bytes"] = tc.requested
			}
			res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "get_attachment", Arguments: args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %v", res.Content)
			}

			out := decodeGetAttachment(t, res)
			if out.ReturnedBytes != tc.want {
				t.Errorf("want returned_bytes=%d, got %d", tc.want, out.ReturnedBytes)
			}
		})
	}
}

func TestGetAttachment_ImageInlined(t *testing.T) {
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0x00}
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "shot.png", MimeType: "image/png", SizeBytes: int64(len(imageBytes))}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader(string(imageBytes))), "image/png", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 8},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content block, got %d", len(res.Content))
	}
	ic, ok := res.Content[0].(*mcpsdk.ImageContent)
	if !ok {
		t.Fatalf("want *mcpsdk.ImageContent, got %T", res.Content[0])
	}
	if ic.MIMEType != "image/png" {
		t.Errorf("want mimeType=image/png, got %q", ic.MIMEType)
	}
	if !bytes.Equal(ic.Data, imageBytes) {
		t.Errorf("image data mismatch: want %v, got %v", imageBytes, ic.Data)
	}

	out := decodeGetAttachment(t, res)
	if out.SignedURL != "" {
		t.Errorf("want no signed_url for inlined image, got %q", out.SignedURL)
	}
	if out.ReturnedBytes != len(imageBytes) {
		t.Errorf("want returned_bytes=%d, got %d", len(imageBytes), out.ReturnedBytes)
	}
}

func TestGetAttachment_OtherMIMEFallsBackToSignedURL(t *testing.T) {
	var openCalled bool
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "archive.zip", MimeType: "application/zip", SizeBytes: 4096}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader("should not be read")), "application/zip", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 3},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	// The handler sets no Content itself for this branch, so the SDK falls
	// back to a single TextContent block containing the JSON-serialized
	// structured output (same as list_attachments) — not an inlined
	// ImageContent block or the raw attachment bytes.
	assertNoInlinedAttachmentContent(t, res)
	if openCalled {
		t.Error("storage was read for a non-inlinable MIME type; want no read")
	}

	out := decodeGetAttachment(t, res)
	if out.SignedURL == "" {
		t.Error("want signed_url set as fallback")
	}
}

func TestGetAttachment_OversizedImageFallsBackToSignedURL(t *testing.T) {
	var openCalled bool
	const overCap = 3 * 1024 * 1024 // > 2 MiB inline cap
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "huge.png", MimeType: "image/png", SizeBytes: overCap}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader("should not be read")), "image/png", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 9},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	assertNoInlinedAttachmentContent(t, res)
	if openCalled {
		t.Error("storage was read for an oversized image; want no read")
	}

	out := decodeGetAttachment(t, res)
	if out.SignedURL == "" {
		t.Error("want signed_url set as fallback for an oversized image")
	}
}

func TestGetAttachment_NilDataStoreAlwaysFallsBackToSignedURL(t *testing.T) {
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "log.txt", MimeType: "text/plain", SizeBytes: 12}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), nil, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	assertNoInlinedAttachmentContent(t, res)

	out := decodeGetAttachment(t, res)
	if out.SignedURL == "" {
		t.Error("want signed_url set when dataStore is nil")
	}
}

func TestGetAttachment_NotFound(t *testing.T) {
	mocks := testutil.New()
	// GetLocationFn left nil → returns store.ErrAttachmentNotFound (mock default).

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore("x"), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 999},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for missing attachment")
	}
}

func TestGetAttachment_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore("x"), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	// attachment_id <= 0.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 0},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for attachment_id=0")
	}

	// Negative offset.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1, "offset": -1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for negative offset")
	}
}

// TestGetAttachment_OtherProjectReportsNotFound pins the project scoping:
// test_attachments.id is a global sequence, so without it any viewer could
// walk every attachment in the deployment by incrementing the id. The error
// must be indistinguishable from an unknown id, otherwise the tool answers
// "does attachment N exist somewhere?" for every N.
func TestGetAttachment_OtherProjectReportsNotFound(t *testing.T) {
	var openCalled bool
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 42, StorageKey: "k", BuildNumber: 1, Source: "secret.log", MimeType: "text/plain", SizeBytes: 10}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader("secret")), "text/plain", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 7},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for an attachment owned by another project")
	}
	if openCalled {
		t.Error("storage was read for an out-of-project attachment; want no read")
	}

	// Identical wording to the unknown-id error: no existence oracle.
	unknown := testutil.New()
	unknownCS := setupAttachmentContentServer(t, buildStoresAttachment(unknown), mockStore, []byte("key"), "http://localhost:8080")
	unknownRes, err := unknownCS.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 7},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got, want := toolErrorText(t, res), toolErrorText(t, unknownRes); got != want {
		t.Errorf("cross-project error = %q, want it identical to the unknown-id error %q", got, want)
	}
}

func TestGetAttachment_MissingProjectIDRejected(t *testing.T) {
	var locCalled bool
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, _ int64) (*store.AttachmentLocation, error) {
		locCalled = true
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "log.txt", MimeType: "text/plain", SizeBytes: 4}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore("data"), []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"attachment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true when project_id is absent")
	}
	if locCalled {
		t.Error("attachment was resolved before project_id was validated")
	}
}

// TestGetAttachment_OffsetAtOrPastEnd covers the paging client that walked one
// window too far: it gets an empty window, not a full-object read, and no
// signed URL — there is nothing left to download.
func TestGetAttachment_OffsetAtOrPastEnd(t *testing.T) {
	const content = "0123456789"
	for _, offset := range []int{len(content), len(content) + 50} {
		var openCalled bool
		mocks := testutil.New()
		mocks.Attachments.GetLocationFn = func(_ context.Context, _ int64) (*store.AttachmentLocation, error) {
			return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "log.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
		}
		mockStore := &storage.MockStore{
			OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
				openCalled = true
				return io.NopCloser(strings.NewReader(content)), "text/plain", nil
			},
		}

		cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "get_attachment",
			Arguments: map[string]any{"project_id": 1, "attachment_id": 1, "offset": offset},
		})
		if err != nil {
			t.Fatalf("CallTool(offset=%d): %v", offset, err)
		}
		if res.IsError {
			t.Fatalf("offset=%d: unexpected tool error: %v", offset, res.Content)
		}
		if openCalled {
			t.Errorf("offset=%d: storage was read past the end of the object; want short-circuit", offset)
		}

		out := decodeGetAttachment(t, res)
		if out.ReturnedBytes != 0 {
			t.Errorf("offset=%d: want returned_bytes=0, got %d", offset, out.ReturnedBytes)
		}
		if out.Truncated {
			t.Errorf("offset=%d: want truncated=false, nothing remains", offset)
		}
		if out.SignedURL != "" {
			t.Errorf("offset=%d: want no signed_url when no bytes were withheld, got %q", offset, out.SignedURL)
		}
	}
}

// TestGetAttachment_SVGNotInlined guards the one image MIME type that is a
// scriptable document. Its bytes and its recorded MIME type both come from an
// ingested report, so it must reach the caller as a download link, never as an
// inline image block.
func TestGetAttachment_SVGNotInlined(t *testing.T) {
	var openCalled bool
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, _ int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "diagram.svg", MimeType: "image/svg+xml", SizeBytes: 64}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader(`<svg onload="x()"/>`)), "image/svg+xml", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	assertNoInlinedAttachmentContent(t, res)
	if openCalled {
		t.Error("storage was read for an SVG; want the signed-URL fallback")
	}

	out := decodeGetAttachment(t, res)
	if out.SignedURL == "" {
		t.Error("want signed_url set for image/svg+xml")
	}
}

// TestGetAttachment_WindowNeverSplitsARune pins the byte-offset windowing
// against multi-byte text: a window boundary inside a rune must drop the
// fragment rather than emit invalid UTF-8.
func TestGetAttachment_WindowNeverSplitsARune(t *testing.T) {
	const content = "aé b" // 'é' occupies bytes 1-2
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, _ int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "log.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}, nil
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), textServingStore(content), []byte("key"), "http://localhost:8080")
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "get_attachment",
		// max_bytes=2 cuts 'é' in half.
		Arguments: map[string]any{"project_id": 1, "attachment_id": 1, "max_bytes": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("want *mcpsdk.TextContent, got %T", res.Content[0])
	}
	if !utf8.ValidString(tc.Text) {
		t.Fatalf("window text is not valid UTF-8: %q", tc.Text)
	}
	if !strings.HasPrefix(tc.Text, "a") {
		t.Errorf("want window to keep the complete leading rune, got %q", tc.Text)
	}

	out := decodeGetAttachment(t, res)
	// returned_bytes counts attachment bytes read, not the runes emitted, so
	// the dropped fragment does not change the caller's paging arithmetic.
	if out.ReturnedBytes != 2 {
		t.Errorf("want returned_bytes=2 (bytes read), got %d", out.ReturnedBytes)
	}
}

// toolErrorText returns the text of an error CallToolResult.
func toolErrorText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("error result carries no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("want *mcpsdk.TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestGetAttachment_PathTraversalRejected(t *testing.T) {
	var openCalled bool
	mocks := testutil.New()
	mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: "../../etc/passwd", MimeType: "text/plain", SizeBytes: 16}, nil
	}
	mockStore := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(strings.NewReader("x")), "text/plain", nil
		},
	}

	cs := setupAttachmentContentServer(t, buildStoresAttachment(mocks), mockStore, []byte("key"), "http://localhost:8080")
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_attachment",
		Arguments: map[string]any{"project_id": 1, "attachment_id": 13},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for path-traversal source")
	}
	if openCalled {
		t.Error("storage was accessed for a path-traversal source; want no access")
	}
}
