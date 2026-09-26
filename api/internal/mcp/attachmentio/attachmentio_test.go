package attachmentio_test

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/mcp/attachmentio"
	"github.com/mkutlak/alluredeck/api/internal/mcp/signed"
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
			if err := attachmentio.ValidateSource(tc.source); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSource(%q) err = %v, wantErr %v", tc.source, err, tc.wantErr)
			}
		})
	}
}

func TestMIMEClassification(t *testing.T) {
	tests := []struct {
		mime        string
		text, image bool
	}{
		{"text/plain", true, false},
		{"TEXT/PLAIN", true, false},
		{"application/json", true, false},
		{"application/xml", true, false},
		{"application/javascript", true, false},
		{"image/png", false, true},
		{"IMAGE/JPEG", false, true},
		{"application/zip", false, false},
		{"video/mp4", false, false},
		// SVG is a scriptable XML document whose MIME type an ingested report
		// controls, so it must never be handed back as an inline image block.
		{"image/svg+xml", false, false},
		{"IMAGE/SVG+XML", false, false},
		{"image/svg+xml; charset=utf-8", false, false},
	}
	for _, tc := range tests {
		if got := attachmentio.IsTextMIME(tc.mime); got != tc.text {
			t.Errorf("IsTextMIME(%q) = %v, want %v", tc.mime, got, tc.text)
		}
		if got := attachmentio.IsImageMIME(tc.mime); got != tc.image {
			t.Errorf("IsImageMIME(%q) = %v, want %v", tc.mime, got, tc.image)
		}
	}
}

func TestReadBlobWindow(t *testing.T) {
	const content = "0123456789"
	tests := []struct {
		name          string
		source        string
		size          int64
		offset, max   int
		want          string
		wantErr       bool
		wantNoStorage bool
	}{
		{name: "full read", source: "f.txt", size: 10, offset: 0, max: 100, want: content},
		{name: "offset window capped by max bytes", source: "f.txt", size: 10, offset: 3, max: 4, want: "3456"},
		// A window at or past the recorded end holds nothing; reading to find
		// that out would stream the whole object first.
		{name: "offset exactly at end skips storage", source: "f.txt", size: 10, offset: 10, max: 10, wantNoStorage: true},
		{name: "offset past end skips storage", source: "f.txt", size: 10, offset: 4096, max: 10, wantNoStorage: true},
		// SizeBytes==0 means "not recorded", not "empty".
		{name: "unknown size still reads", source: "f.txt", size: 0, offset: 2, max: 4, want: "2345"},
		{name: "negative offset", source: "f.txt", size: 10, offset: -1, max: 10, wantErr: true, wantNoStorage: true},
		{name: "negative max bytes", source: "f.txt", size: 10, offset: 0, max: -1, wantErr: true, wantNoStorage: true},
		{name: "path-traversal source", source: "../evil", size: 10, offset: 0, max: 10, wantErr: true, wantNoStorage: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opened string
			ds := &storage.MockStore{OpenReportFileFn: func(_ context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
				opened = projectID + "|" + reportID + "|" + filePath
				return io.NopCloser(strings.NewReader(content)), "text/plain", nil
			}}
			loc := &store.AttachmentLocation{StorageKey: "k", BuildNumber: 3, Source: tc.source, MimeType: "text/plain", SizeBytes: tc.size}

			got, err := attachmentio.ReadBlobWindow(context.Background(), ds, loc, tc.offset, tc.max)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if string(got) != tc.want {
				t.Errorf("window = %q, want %q", got, tc.want)
			}
			if tc.wantNoStorage && opened != "" {
				t.Errorf("storage was read (%s), want no access", opened)
			}
			if !tc.wantNoStorage && opened != "k|3|data/attachments/f.txt" {
				t.Errorf("storage read %q, want k|3|data/attachments/f.txt", opened)
			}
		})
	}
}

// TestSignURL checks the download link against the canonical payload
// "attachment:{id}" so a change in the signed format breaks here, not in the
// field.
func TestSignURL(t *testing.T) {
	key := []byte("test-signing-key-32-bytes-padded!")
	raw := attachmentio.SignURL("http://localhost:8080/", 42, key)

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	if u.Scheme+"://"+u.Host+u.Path != "http://localhost:8080/attachments/42" {
		t.Errorf("URL = %q, want http://localhost:8080/attachments/42?...", raw)
	}
	exp, _ := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if expT := time.Unix(exp, 0); expT.Before(time.Now()) || expT.After(time.Now().Add(attachmentio.URLTTL+time.Second)) {
		t.Errorf("exp = %v, want within URLTTL from now", expT)
	}
	if err := signed.Verify(key, "attachment:42", exp, u.Query().Get("sig"), time.Now()); err != nil {
		t.Errorf("signature does not verify for attachment:42: %v", err)
	}
}
