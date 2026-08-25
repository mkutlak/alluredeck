package attachmentio_test

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/mcp/attachmentio"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestValidateSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{"plain filename", "screenshot.png", false},
		{"filename with dashes", "test-result-42.log", false},
		{"filename with single dot", "report.v2.json", false},
		{"empty", "", true},
		{"forward slash", "sub/shot.png", true},
		{"absolute path", "/etc/passwd", true},
		{"backslash", "sub\\shot.png", true},
		{"parent traversal", "../secret", true},
		{"embedded parent traversal", "a/../../secret", true},
		{"dotdot only", "..", true},
		{"NUL byte", "shot.png\x00.txt", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := attachmentio.ValidateSource(tc.source)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateSource(%q): want error, got nil", tc.source)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateSource(%q): want nil error, got %v", tc.source, err)
			}
		})
	}
}

func TestIsTextMIME(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"text/plain", true},
		{"TEXT/PLAIN", true},
		{"application/json", true},
		{"application/xml", true},
		{"application/javascript", true},
		{"image/png", false},
		{"application/zip", false},
		{"video/mp4", false},
	}
	for _, tc := range tests {
		if got := attachmentio.IsTextMIME(tc.mime); got != tc.want {
			t.Errorf("IsTextMIME(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestIsImageMIME(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"IMAGE/JPEG", true},
		{"text/plain", false},
		{"application/zip", false},
		// SVG is a scriptable XML document whose MIME type an ingested report
		// controls, so it must never be handed back as an inline image block.
		{"image/svg+xml", false},
		{"IMAGE/SVG+XML", false},
		{"image/svg+xml; charset=utf-8", false},
	}
	for _, tc := range tests {
		if got := attachmentio.IsImageMIME(tc.mime); got != tc.want {
			t.Errorf("IsImageMIME(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

// mockStore is a minimal storage.Store double for exercising ReadBlobWindow.
func mockStoreServing(content string) *storage.MockStore {
	return &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader(content)), "text/plain", nil
		},
	}
}

func TestReadBlobWindow_FullRead(t *testing.T) {
	const content = "0123456789"
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}

	got, err := attachmentio.ReadBlobWindow(context.Background(), mockStoreServing(content), loc, 0, 100)
	if err != nil {
		t.Fatalf("ReadBlobWindow: %v", err)
	}
	if string(got) != content {
		t.Errorf("want %q, got %q", content, got)
	}
}

func TestReadBlobWindow_OffsetWindow(t *testing.T) {
	const content = "0123456789"
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}

	got, err := attachmentio.ReadBlobWindow(context.Background(), mockStoreServing(content), loc, 3, 4)
	if err != nil {
		t.Fatalf("ReadBlobWindow: %v", err)
	}
	if string(got) != "3456" {
		t.Errorf("want %q, got %q", "3456", got)
	}
}

func TestReadBlobWindow_MaxBytesClampsShortOfEnd(t *testing.T) {
	const content = "hello world"
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}

	got, err := attachmentio.ReadBlobWindow(context.Background(), mockStoreServing(content), loc, 0, 5)
	if err != nil {
		t.Fatalf("ReadBlobWindow: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("want %q, got %q", "hello", got)
	}
}

func TestReadBlobWindow_OffsetBeyondEnd(t *testing.T) {
	const content = "short"
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: int64(len(content))}

	got, err := attachmentio.ReadBlobWindow(context.Background(), mockStoreServing(content), loc, 100, 10)
	if err != nil {
		t.Fatalf("ReadBlobWindow: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got %q", got)
	}
}

func TestReadBlobWindow_OffsetAtOrBeyondSizeSkipsStorage(t *testing.T) {
	tests := []struct {
		name   string
		offset int
	}{
		{"offset exactly at end", 5},
		{"offset past end", 4096},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var openCalled bool
			ds := &storage.MockStore{
				OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
					openCalled = true
					return io.NopCloser(strings.NewReader("short")), "text/plain", nil
				},
			}
			loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: 5}

			got, err := attachmentio.ReadBlobWindow(context.Background(), ds, loc, tc.offset, 10)
			if err != nil {
				t.Fatalf("ReadBlobWindow: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("want empty slice, got %q", got)
			}
			if openCalled {
				t.Error("storage was read for an offset at or beyond size_bytes; want short-circuit")
			}
		})
	}
}

func TestReadBlobWindow_UnknownSizeStillReads(t *testing.T) {
	// SizeBytes==0 means "not recorded", not "empty": the read must still run.
	const content = "0123456789"
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: 0}

	got, err := attachmentio.ReadBlobWindow(context.Background(), mockStoreServing(content), loc, 2, 4)
	if err != nil {
		t.Fatalf("ReadBlobWindow: %v", err)
	}
	if string(got) != "2345" {
		t.Errorf("want %q, got %q", "2345", got)
	}
}

func TestReadBlobWindow_NegativeOffsetOrMaxBytesRejected(t *testing.T) {
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "f.txt", MimeType: "text/plain", SizeBytes: 5}
	ds := mockStoreServing("hello")

	if _, err := attachmentio.ReadBlobWindow(context.Background(), ds, loc, -1, 10); err == nil {
		t.Error("want error for negative offset, got nil")
	}
	if _, err := attachmentio.ReadBlobWindow(context.Background(), ds, loc, 0, -1); err == nil {
		t.Error("want error for negative maxBytes, got nil")
	}
}

func TestReadBlobWindow_InvalidSourceRejectedBeforeStorageAccess(t *testing.T) {
	var openCalled bool
	ds := &storage.MockStore{
		OpenReportFileFn: func(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
			openCalled = true
			return io.NopCloser(bytes.NewReader(nil)), "", nil
		},
	}
	loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 1, Source: "../evil", MimeType: "text/plain", SizeBytes: 5}

	if _, err := attachmentio.ReadBlobWindow(context.Background(), ds, loc, 0, 10); err == nil {
		t.Fatal("want error for path-traversal source, got nil")
	}
	if openCalled {
		t.Error("storage was accessed for a path-traversal source; want no access")
	}
}

func TestSignURL(t *testing.T) {
	signingKey := []byte("test-signing-key-32-bytes-padded!")
	url := attachmentio.SignURL("http://localhost:8080", 42, signingKey)

	want := "http://localhost:8080/attachments/42?exp="
	if !strings.HasPrefix(url, want) {
		t.Errorf("want prefix %q, got %q", want, url)
	}
	if !strings.Contains(url, "&sig=") {
		t.Errorf("want &sig= in URL, got %q", url)
	}
}

func TestSigPayload(t *testing.T) {
	if got, want := attachmentio.SigPayload(7), "attachment:"+strconv.Itoa(7); got != want {
		t.Errorf("SigPayload(7) = %q, want %q", got, want)
	}
}
