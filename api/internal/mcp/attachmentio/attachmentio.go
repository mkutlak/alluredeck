// Package attachmentio holds the storage-read, MIME-classification, and
// URL-signing logic shared between the MCP attachment resource
// (internal/mcp/resources.go) and the get_attachment tool
// (internal/mcp/tools/attachment.go).
//
// It lives in its own package because internal/mcp imports
// internal/mcp/tools (to register tool handlers on server construction), so
// internal/mcp/tools cannot import internal/mcp without creating an import
// cycle. Both packages import this one instead, so the two attachment-content
// code paths (the resource read and the tool call) stay byte-for-byte
// consistent rather than drifting apart as separate copies.
package attachmentio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/mcp/signed"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ErrInvalidSource indicates an attachment source filename contains
// path-traversal characters and must not be used to build a storage path.
var ErrInvalidSource = errors.New("invalid attachment source")

// URLTTL is how long a signed attachment download URL stays valid.
const URLTTL = 10 * time.Minute

// ValidateSource rejects attachment source filenames that could be used for
// path traversal. Attachment sources originate from ingested Allure reports
// and are therefore attacker-influenced: they must be a bare filename with no
// path separators, no parent-directory references, and no NUL bytes before
// being joined onto a storage path.
//
// It is the single canonical guard shared by every code path that builds a
// "data/attachments/{source}" storage path (the REST ServeAttachment handler,
// the signed-download handler, the MCP attachment resource, and the
// get_attachment tool).
func ValidateSource(source string) error {
	if source == "" ||
		strings.Contains(source, "/") ||
		strings.Contains(source, "\\") ||
		strings.Contains(source, "..") ||
		strings.ContainsRune(source, 0) {
		return ErrInvalidSource
	}
	return nil
}

// IsTextMIME reports whether the given MIME type should be served as inline text.
func IsTextMIME(mime string) bool {
	lower := strings.ToLower(mime)
	return strings.HasPrefix(lower, "text/") ||
		lower == "application/json" ||
		lower == "application/xml" ||
		lower == "application/javascript"
}

// IsImageMIME reports whether the given MIME type may be returned as an inline
// image block.
//
// image/svg+xml is deliberately excluded. An SVG is a scriptable XML document,
// not a raster image, and both its bytes and its recorded MIME type come from
// an ingested Allure report — attacker-influenced on both counts. Callers fall
// back to a signed download URL for it, so the content is never inlined into an
// agent's context as a trusted image.
func IsImageMIME(mime string) bool {
	lower := strings.ToLower(mime)
	if !strings.HasPrefix(lower, "image/") {
		return false
	}
	// Compare against the bare type so a "; charset=..." parameter cannot
	// smuggle SVG past the check.
	base := strings.TrimSpace(lower)
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	return base != "image/svg+xml"
}

// ReadBlobWindow streams up to maxBytes of an attachment's content, starting
// at byte offset, from the file-storage backend. It resolves the same
// storage path the REST ServeAttachment handler uses:
// {storageKey}/reports/{buildNumber}/data/attachments/{source}.
//
// loc.Source is attacker-influenced data from ingested Allure reports, so it
// is validated with ValidateSource before being joined onto the storage path.
//
// offset must be >= 0 and maxBytes must be >= 0; callers are responsible for
// clamping maxBytes to whatever cap applies at their call site (e.g. the 2 MB
// inline cap). When offset is at or beyond the end of the blob, it returns an
// empty, non-nil slice rather than an error.
func ReadBlobWindow(ctx context.Context, dataStore storage.Store, loc *store.AttachmentLocation, offset, maxBytes int) ([]byte, error) {
	if err := ValidateSource(loc.Source); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be non-negative, got %d", offset)
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("maxBytes must be non-negative, got %d", maxBytes)
	}
	// A window that starts at or past the recorded end holds nothing, and the
	// only way to discover that by reading is to stream the entire object into
	// io.Discard first — for a paging client that walked one byte too far, a
	// full-object read that returns nothing. SizeBytes==0 means "not recorded"
	// rather than "empty", so it is not treated as an end marker.
	if loc.SizeBytes > 0 && int64(offset) >= loc.SizeBytes {
		return []byte{}, nil
	}

	filePath := "data/attachments/" + loc.Source
	reader, _, err := dataStore.OpenReportFile(ctx, loc.StorageKey, strconv.Itoa(loc.BuildNumber), filePath)
	if err != nil {
		return nil, fmt.Errorf("opening attachment blob: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if offset > 0 {
		if _, err := io.CopyN(io.Discard, reader, int64(offset)); err != nil {
			if errors.Is(err, io.EOF) {
				return []byte{}, nil
			}
			return nil, fmt.Errorf("discarding attachment blob prefix: %w", err)
		}
	}

	data, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)))
	if err != nil {
		return nil, fmt.Errorf("reading attachment blob: %w", err)
	}
	return data, nil
}

// SigPayload builds the canonical payload string that is signed for an
// attachment download URL: "attachment:{id}". The expiry is bound to the
// signature by signed.Sign rather than being part of this string.
func SigPayload(id int64) string {
	return fmt.Sprintf("attachment:%d", id)
}

// SignURL returns a signed URL for direct attachment download, valid for
// URLTTL from now. The URL embeds exp (Unix timestamp) and sig
// (HMAC-SHA256 hex), verified by the inverse check in the GET
// /attachments/{id} download route.
func SignURL(publicURL string, id int64, signingKey []byte) string {
	exp := time.Now().Add(URLTTL).Unix()
	sig := signed.Sign(signingKey, SigPayload(id), exp)
	base := strings.TrimRight(publicURL, "/")
	return fmt.Sprintf("%s/attachments/%d?exp=%d&sig=%s", base, id, exp, sig)
}
