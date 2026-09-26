package tools_test

import (
	"bytes"
	"context"
	"io"
	"maps"
	"reflect"
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

// TestListAttachments pins the scoping fix: attachments are listed for the one
// test named by (project_id, build_id, history_id) via ListByTestResult — the
// old ListByBuild call handed every test the whole build's attachments.
func TestListAttachments(t *testing.T) {
	mocks := testutil.New()
	var got [3]any
	mocks.Attachments.ListByTestResultFn = func(_ context.Context, projectID, buildID int64, historyID string, _ int) ([]store.TestAttachment, error) {
		got = [3]any{projectID, buildID, historyID}
		return []store.TestAttachment{
			{ID: 1, Name: "screenshot.png", MimeType: "image/png", SizeBytes: 8192},
			{ID: 2, Name: "log.txt", MimeType: "text/plain", SizeBytes: 1024},
		}, nil
	}
	cs := setupTestServer(t, &bootstrap.Stores{Attachment: mocks.Attachments})

	out, _ := call[tools.ListAttachmentsOutput](t, cs, "list_attachments", map[string]any{"project_id": 5, "build_id": 77, "history_id": "h-target"})
	want := []tools.AttachmentItem{
		{ID: 1, Name: "screenshot.png", Mime: "image/png", SizeBytes: 8192, ResourceURI: "alluredeck://attachment/1"},
		{ID: 2, Name: "log.txt", Mime: "text/plain", SizeBytes: 1024, ResourceURI: "alluredeck://attachment/2"},
	}
	if !reflect.DeepEqual(out.Items, want) {
		t.Errorf("items = %+v, want %+v", out.Items, want)
	}
	if got != [3]any{int64(5), int64(77), "h-target"} {
		t.Errorf("ListByTestResult(project, build, history) = %v, want [5 77 h-target]", got)
	}
	for _, args := range []map[string]any{
		{"project_id": 1, "build_id": 10, "history_id": ""},
		{"project_id": 0, "build_id": 10, "history_id": "h1"},
		{"project_id": 1, "build_id": 0, "history_id": "h1"},
	} {
		callErr(t, cs, "list_attachments", args)
	}
}

// TestGetAttachment covers get_attachment, the tool twin of the
// alluredeck://attachment resource for gateways that proxy only tools. Text is
// windowed by offset/max_bytes, raster images up to 2 MiB are inlined, and
// everything else (SVG, other types, oversized images, no storage backend)
// comes back as a one-line digest plus a signed URL. Every read is scoped to
// the caller's project and guarded against path traversal.
func TestGetAttachment(t *testing.T) {
	const threeMiB = 3 << 20
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0x00}
	loc := func(source, mime string, size int64) *store.AttachmentLocation {
		return &store.AttachmentLocation{ProjectID: 1, StorageKey: "k", BuildNumber: 1, Source: source, MimeType: mime, SizeBytes: size}
	}
	out := func(mime string, size int64, offset, returned int, truncated bool) tools.GetAttachmentOutput {
		return tools.GetAttachmentOutput{AttachmentID: 7, Name: "attachment-7", Mime: mime, SizeBytes: size,
			Offset: offset, ReturnedBytes: returned, Truncated: truncated}
	}
	digits := "0123456789"
	huge := strings.Repeat("x", threeMiB)

	tests := []struct {
		name       string
		loc        *store.AttachmentLocation // nil: unknown attachment
		body       string
		noStorage  bool
		args       map[string]any
		want       tools.GetAttachmentOutput
		wantSigned bool
		text       string // exact text block, when set
		image      []byte // exact image block, when set
		digestOnly bool   // no inlined bytes: one digest line, not a JSON echo
		wantRead   bool
		wantErr    string
	}{
		{name: "small text is inlined in full", loc: loc("log.txt", "text/plain", 10), body: digits,
			want: out("text/plain", 10, 0, 10, false), text: digits, wantRead: true},
		{name: "text window is truncated with a continuation marker and a signed URL", loc: loc("big.log", "text/plain", 10), body: digits,
			args: map[string]any{"max_bytes": 4}, want: out("text/plain", 10, 0, 4, true), wantSigned: true, wantRead: true,
			text: "0123\n…[truncated: bytes 0-4 of 10; call again with offset=4]"},
		{name: "offset window reaching the end is not truncated", loc: loc("big.log", "text/plain", 10), body: digits,
			args: map[string]any{"offset": 4, "max_bytes": 6}, want: out("text/plain", 10, 4, 6, false), text: "456789", wantRead: true},
		// returned_bytes counts bytes read, so dropping the split rune does not
		// shift the caller's paging arithmetic.
		{name: "window never splits a rune", loc: loc("log.txt", "text/plain", 5), body: "aé b",
			args: map[string]any{"max_bytes": 2}, want: out("text/plain", 5, 0, 2, true), wantSigned: true, wantRead: true,
			text: "a\n…[truncated: bytes 0-2 of 5; call again with offset=2]"},
		{name: "max_bytes defaults to 64KiB", loc: loc("huge.log", "text/plain", threeMiB), body: huge,
			want: out("text/plain", threeMiB, 0, 65536, true), wantSigned: true, wantRead: true},
		{name: "negative max_bytes clamps to 1", loc: loc("huge.log", "text/plain", threeMiB), body: huge,
			args: map[string]any{"max_bytes": -5}, want: out("text/plain", threeMiB, 0, 1, true), wantSigned: true, wantRead: true},
		{name: "max_bytes clamps to 2MiB", loc: loc("huge.log", "text/plain", threeMiB), body: huge,
			args: map[string]any{"max_bytes": 10 << 20}, want: out("text/plain", threeMiB, 0, 2<<20, true), wantSigned: true, wantRead: true},
		{name: "max_bytes within range passes through", loc: loc("huge.log", "text/plain", threeMiB), body: huge,
			args: map[string]any{"max_bytes": 100}, want: out("text/plain", threeMiB, 0, 100, true), wantSigned: true, wantRead: true},
		{name: "raster image is inlined as an image block", loc: loc("shot.png", "image/png", int64(len(png))), body: string(png),
			want: out("image/png", int64(len(png)), 0, len(png), false), image: png, wantRead: true},
		{name: "other MIME types fall back to a signed URL unread", loc: loc("archive.zip", "application/zip", 4096), body: "unread",
			want: out("application/zip", 4096, 0, 0, false), wantSigned: true, digestOnly: true},
		{name: "oversized image falls back to a signed URL unread", loc: loc("huge.png", "image/png", threeMiB), body: "unread",
			want: out("image/png", threeMiB, 0, 0, false), wantSigned: true, digestOnly: true},
		// SVG is a scriptable document whose bytes and MIME type both come
		// from an ingested report.
		{name: "SVG is never inlined", loc: loc("diagram.svg", "image/svg+xml", 64), body: `<svg onload="x()"/>`,
			want: out("image/svg+xml", 64, 0, 0, false), wantSigned: true, digestOnly: true},
		{name: "no storage backend always signs", loc: loc("log.txt", "text/plain", 12), noStorage: true,
			want: out("text/plain", 12, 0, 0, false), wantSigned: true, digestOnly: true},
		// A paging client one window too far gets an empty window, not a
		// full-object read and not a download link.
		{name: "offset at the end reads nothing", loc: loc("log.txt", "text/plain", 10), body: digits,
			args: map[string]any{"offset": 10}, want: out("text/plain", 10, 10, 0, false)},
		{name: "offset past the end reads nothing", loc: loc("log.txt", "text/plain", 10), body: digits,
			args: map[string]any{"offset": 60}, want: out("text/plain", 10, 60, 0, false)},
		{name: "unknown attachment", wantErr: "attachment 7 not found"},
		// test_attachments.id is a global sequence: without project scoping any
		// viewer could walk every attachment, and a distinct error would still
		// answer "does attachment N exist somewhere?".
		{name: "attachment of another project reads as unknown", body: "secret",
			loc:     &store.AttachmentLocation{ProjectID: 42, StorageKey: "k", BuildNumber: 1, Source: "secret.log", MimeType: "text/plain", SizeBytes: 6},
			wantErr: "attachment 7 not found"},
		{name: "project_id is required before any lookup", loc: loc("log.txt", "text/plain", 4), body: "data",
			args: map[string]any{"project_id": 0}, wantErr: "project_id must be positive"},
		{name: "non-positive attachment_id", args: map[string]any{"attachment_id": 0}, wantErr: "attachment_id must be positive"},
		{name: "negative offset", args: map[string]any{"offset": -1}, wantErr: "offset must be non-negative"},
		{name: "path-traversal source", loc: loc("../../etc/passwd", "text/plain", 16), body: "x", wantErr: "invalid attachment source"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			lookups := 0
			mocks.Attachments.GetLocationFn = func(_ context.Context, id int64) (*store.AttachmentLocation, error) {
				lookups++
				if tc.loc == nil || id != 7 {
					return nil, store.ErrAttachmentNotFound
				}
				return tc.loc, nil
			}
			mocks.Attachments.GetByIDFn = func(_ context.Context, id int64) (*store.TestAttachment, error) {
				return &store.TestAttachment{ID: id, Name: "attachment-7"}, nil
			}
			reads := 0
			var ds storage.Store
			if !tc.noStorage {
				ds = &storage.MockStore{OpenReportFileFn: func(context.Context, string, string, string) (io.ReadCloser, string, error) {
					reads++
					return io.NopCloser(strings.NewReader(tc.body)), "", nil
				}}
			}
			cs := connect(t, func(s *mcpsdk.Server) {
				tools.RegisterAttachmentContentTool(s, &bootstrap.Stores{Attachment: mocks.Attachments}, zap.NewNop(),
					[]byte("key"), "http://localhost:8080", ds)
			})
			args := map[string]any{"project_id": 1, "attachment_id": 7}
			maps.Copy(args, tc.args)

			if tc.wantErr != "" {
				if msg := callErr(t, cs, "get_attachment", args); !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				if reads != 0 {
					t.Errorf("storage was read %d time(s) for a refused call", reads)
				}
				if args["project_id"] == 0 && lookups != 0 {
					t.Error("the attachment was resolved before project_id was validated")
				}
				return
			}
			res := callTool(t, cs, "get_attachment", args)
			if res.IsError {
				t.Fatalf("unexpected tool error: %s", textOf(t, res))
			}
			got := decode[tools.GetAttachmentOutput](t, res)
			if signed := strings.HasPrefix(got.SignedURL, "http://localhost:8080/attachments/7?"); signed != tc.wantSigned {
				t.Errorf("signed_url = %q, want signed=%v", got.SignedURL, tc.wantSigned)
			}
			got.SignedURL = ""
			if got != tc.want {
				t.Errorf("output = %+v, want %+v", got, tc.want)
			}
			if (reads > 0) != tc.wantRead {
				t.Errorf("storage reads = %d, want read=%v", reads, tc.wantRead)
			}
			if len(res.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(res.Content))
			}
			if tc.image != nil {
				ic, ok := res.Content[0].(*mcpsdk.ImageContent)
				if !ok || ic.MIMEType != "image/png" || !bytes.Equal(ic.Data, tc.image) {
					t.Errorf("content = %#v, want an image/png block carrying the raw bytes", res.Content[0])
				}
				return
			}
			text := textOf(t, res)
			switch {
			case tc.digestOnly && strings.HasPrefix(text, "{"):
				// The SDK echoes the whole structured output as JSON text when a
				// handler returns a nil result; the client would pay twice.
				t.Errorf("want a one-line digest, got the payload echoed as text: %q", text)
			case tc.text != "" && text != tc.text:
				t.Errorf("text = %q, want %q", text, tc.text)
			case !utf8.ValidString(text):
				t.Errorf("text is not valid UTF-8: %q", text)
			}
		})
	}
}
