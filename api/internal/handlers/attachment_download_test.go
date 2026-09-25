package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// signAttachmentURL builds the exp+sig query string for a signed attachment
// download, using the same HMAC scheme the MCP server signs with.
func signAttachmentURL(signingKey []byte, id, exp int64) string {
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte("attachment:" + strconv.FormatInt(id, 10) + ":exp:" + strconv.FormatInt(exp, 10)))
	return "exp=" + strconv.FormatInt(exp, 10) + "&sig=" + hex.EncodeToString(mac.Sum(nil))
}

// TestServeSignedAttachment covers GET /attachments/{id}, which is mounted
// without auth middleware: the HMAC in the URL is its only authentication.
func TestServeSignedAttachment(t *testing.T) {
	const id = int64(55)
	const blob = "screenshot-bytes"
	signingKey := []byte("test-signing-key-32-bytes-padded!")
	valid := signAttachmentURL(signingKey, id, time.Now().Add(5*time.Minute).Unix())
	// Flip the last hex char to a guaranteed-different value; a fixed
	// replacement is flaky when the signature already ends in it.
	flip := "0"
	if valid[len(valid)-1] == '0' {
		flip = "1"
	}
	tampered := valid[:len(valid)-1] + flip

	tests := []struct {
		name   string
		query  string
		source string // "" means the attachment is unknown
		want   int
	}{
		{name: "valid signature streams the blob", query: valid, source: "shot.png", want: http.StatusOK},
		{name: "expired URL", query: signAttachmentURL(signingKey, id, time.Now().Add(-time.Minute).Unix()), source: "shot.png", want: http.StatusForbidden},
		{name: "tampered signature", query: tampered, source: "shot.png", want: http.StatusForbidden},
		{name: "signature minted for another id", query: signAttachmentURL(signingKey, id+1, time.Now().Add(5*time.Minute).Unix()), source: "shot.png", want: http.StatusForbidden},
		{name: "missing sig", query: "exp=" + strconv.FormatInt(time.Now().Add(5*time.Minute).Unix(), 10), source: "shot.png", want: http.StatusBadRequest},
		{name: "unknown attachment", query: valid, want: http.StatusNotFound},
		// A stored source that is not a bare file name is rejected before
		// storage is touched.
		{name: "traversal source", query: valid, source: "../../../etc/passwd", want: http.StatusBadRequest},
		{name: "nested source", query: valid, source: "sub/dir/shot.png", want: http.StatusBadRequest},
		{name: "windows traversal source", query: valid, source: "..\\..\\windows\\system32", want: http.StatusBadRequest},
		{name: "nul byte source", query: valid, source: "shot.png\x00.txt", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := &mockAttachmentStore{}
			if tc.source != "" {
				as.location = &store.AttachmentLocation{StorageKey: "proj-storage-key", BuildNumber: 9, Source: tc.source, MimeType: "image/png", SizeBytes: int64(len(blob))}
			}
			ds := &mockDataStore{content: blob, mimeType: "image/png"}
			h := NewAttachmentDownloadHandler(as, ds, signingKey, zap.NewNop())
			req := httptest.NewRequest(http.MethodGet, "/attachments/55?"+tc.query, nil)
			req.SetPathValue("id", "55")
			rec := httptest.NewRecorder()
			h.ServeSignedAttachment(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want != http.StatusOK {
				if tc.source != "" && ds.openCalls != 0 {
					t.Errorf("storage opened %d time(s) for a rejected request, want 0", ds.openCalls)
				}
				return
			}
			if rec.Body.String() != blob || rec.Header().Get("Content-Type") != "image/png" {
				t.Errorf("body, Content-Type = %q, %q; want %q, image/png", rec.Body.String(), rec.Header().Get("Content-Type"), blob)
			}
		})
	}
}
