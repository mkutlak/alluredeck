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
func signAttachmentURL(t *testing.T, signingKey []byte, id, exp int64) string {
	t.Helper()
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte("attachment:" + strconv.FormatInt(id, 10) + ":exp:" + strconv.FormatInt(exp, 10)))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig
}

// serveSignedAttachmentRequest dispatches a GET /attachments/{id} request
// against a freshly built handler and returns the recorded response.
func serveSignedAttachmentRequest(
	h *AttachmentDownloadHandler,
	id int64,
	query string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/attachments/"+strconv.FormatInt(id, 10)+"?"+query, nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	h.ServeSignedAttachment(rec, req)
	return rec
}

func TestServeSignedAttachment(t *testing.T) {
	const attachmentID = int64(55)
	const blobContent = "screenshot-bytes"
	signingKey := []byte("test-signing-key-32-bytes-padded!")

	loc := &store.AttachmentLocation{
		StorageKey:  "proj-storage-key",
		BuildNumber: 9,
		Source:      "shot.png",
		MimeType:    "image/png",
		SizeBytes:   int64(len(blobContent)),
	}

	newHandler := func(as store.AttachmentStorer, ds *mockDataStore) *AttachmentDownloadHandler {
		return NewAttachmentDownloadHandler(as, ds, signingKey, zap.NewNop())
	}

	validExp := time.Now().Add(5 * time.Minute).Unix()
	expiredExp := time.Now().Add(-time.Minute).Unix()

	t.Run("valid signature streams the blob", func(t *testing.T) {
		as := &mockAttachmentStore{location: loc}
		ds := &mockDataStore{content: blobContent, mimeType: "image/png"}
		h := newHandler(as, ds)

		rec := serveSignedAttachmentRequest(h, attachmentID, signAttachmentURL(t, signingKey, attachmentID, validExp))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: want 200, got %d (body %q)", rec.Code, rec.Body.String())
		}
		if rec.Body.String() != blobContent {
			t.Errorf("body: want %q, got %q", blobContent, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type: want image/png, got %q", ct)
		}
	})

	t.Run("expired URL is rejected with 403", func(t *testing.T) {
		as := &mockAttachmentStore{location: loc}
		ds := &mockDataStore{content: blobContent, mimeType: "image/png"}
		h := newHandler(as, ds)

		rec := serveSignedAttachmentRequest(h, attachmentID, signAttachmentURL(t, signingKey, attachmentID, expiredExp))

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status: want 403, got %d", rec.Code)
		}
	})

	t.Run("tampered signature is rejected with 403", func(t *testing.T) {
		as := &mockAttachmentStore{location: loc}
		ds := &mockDataStore{content: blobContent, mimeType: "image/png"}
		h := newHandler(as, ds)

		query := signAttachmentURL(t, signingKey, attachmentID, validExp)
		// Flip the last hex char to a guaranteed-different value. Replacing it
		// with a fixed constant is flaky: the signature itself sometimes ends
		// in that constant, leaving the "tampered" string identical.
		last := query[len(query)-1]
		flipped := byte('0')
		if last == '0' {
			flipped = '1'
		}
		tampered := query[:len(query)-1] + string(flipped)

		rec := serveSignedAttachmentRequest(h, attachmentID, tampered)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status: want 403, got %d", rec.Code)
		}
	})

	t.Run("signature minted for a different id is rejected", func(t *testing.T) {
		as := &mockAttachmentStore{location: loc}
		ds := &mockDataStore{content: blobContent, mimeType: "image/png"}
		h := newHandler(as, ds)

		// Signature is valid for attachmentID+1 but the path is attachmentID.
		query := signAttachmentURL(t, signingKey, attachmentID+1, validExp)

		rec := serveSignedAttachmentRequest(h, attachmentID, query)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status: want 403, got %d", rec.Code)
		}
	})

	t.Run("missing sig query param is a 400", func(t *testing.T) {
		h := newHandler(&mockAttachmentStore{location: loc}, &mockDataStore{})

		rec := serveSignedAttachmentRequest(h, attachmentID, "exp="+strconv.FormatInt(validExp, 10))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status: want 400, got %d", rec.Code)
		}
	})

	t.Run("unknown attachment is a 404", func(t *testing.T) {
		// location nil → mockAttachmentStore.GetLocation returns ErrAttachmentNotFound.
		h := newHandler(&mockAttachmentStore{}, &mockDataStore{})

		rec := serveSignedAttachmentRequest(h, attachmentID, signAttachmentURL(t, signingKey, attachmentID, validExp))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: want 404, got %d", rec.Code)
		}
	})

	t.Run("path-traversal source is rejected with 400 without touching storage", func(t *testing.T) {
		traversalSources := []string{
			"../../../etc/passwd",
			"sub/dir/shot.png",
			"..\\..\\windows\\system32",
			"shot.png\x00.txt",
		}
		for _, badSource := range traversalSources {
			t.Run(badSource, func(t *testing.T) {
				badLoc := &store.AttachmentLocation{
					StorageKey:  "proj-storage-key",
					BuildNumber: 9,
					Source:      badSource,
					MimeType:    "image/png",
				}
				as := &mockAttachmentStore{location: badLoc}
				ds := &mockDataStore{content: blobContent, mimeType: "image/png"}
				h := newHandler(as, ds)

				rec := serveSignedAttachmentRequest(h, attachmentID, signAttachmentURL(t, signingKey, attachmentID, validExp))

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status: want 400, got %d (body %q)", rec.Code, rec.Body.String())
				}
				if ds.openCalls != 0 {
					t.Errorf("storage was accessed %d time(s) for a bad source; want 0", ds.openCalls)
				}
			})
		}
	})
}
