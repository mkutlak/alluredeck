package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

const attachmentInlineMaxBytes = 2 * 1024 * 1024 // 2 MB

// attachmentURLTTL is how long a signed attachment download URL stays valid.
const attachmentURLTTL = 10 * time.Minute

// ErrInvalidAttachmentSource indicates an attachment source filename contains
// path-traversal characters and must not be used to build a storage path.
var ErrInvalidAttachmentSource = errors.New("invalid attachment source")

// ValidateAttachmentSource rejects attachment source filenames that could be
// used for path traversal. Attachment sources originate from ingested Allure
// reports and are therefore attacker-influenced: they must be a bare filename
// with no path separators, no parent-directory references, and no NUL bytes
// before being joined onto a storage path.
//
// It is the single canonical guard shared by every code path that builds a
// "data/attachments/{source}" storage path (the REST ServeAttachment handler,
// the signed-download handler, and the MCP attachment resource).
func ValidateAttachmentSource(source string) error {
	if source == "" ||
		strings.Contains(source, "/") ||
		strings.Contains(source, "\\") ||
		strings.Contains(source, "..") ||
		strings.ContainsRune(source, 0) {
		return ErrInvalidAttachmentSource
	}
	return nil
}

// isTextMIME reports whether the given MIME type should be served as inline text.
func isTextMIME(mime string) bool {
	lower := strings.ToLower(mime)
	return strings.HasPrefix(lower, "text/") ||
		lower == "application/json" ||
		lower == "application/xml" ||
		lower == "application/javascript"
}

// isImageMIME reports whether the given MIME type is an image.
func isImageMIME(mime string) bool {
	return strings.HasPrefix(strings.ToLower(mime), "image/")
}

// RegisterResources registers all alluredeck MCP resource templates on s.
// dataStore may be nil; in that case attachment content is always returned as
// a signed resource link URL rather than inlined.
func RegisterResources(
	s *mcpsdk.Server,
	stores *bootstrap.Stores,
	logger *zap.Logger,
	signingKey []byte,
	publicURL string,
	dataStore storage.Store,
) {
	s.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		URITemplate: "alluredeck://attachment/{id}",
		Name:        "attachment",
		Description: "Retrieve attachment content. Text and small images are inlined; large files return a signed download link as the resource URI.",
	}, attachmentResourceHandler(stores, logger, signingKey, publicURL, dataStore))

	s.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		URITemplate: "alluredeck://project/{project_id}/build/{build_id}/test/{history_id}",
		Name:        "test_failure",
		Description: "Retrieve the same payload as get_test_failure for a specific test in a build.",
	}, testResourceHandler(stores, logger))
}

func attachmentResourceHandler(
	stores *bootstrap.Stores,
	logger *zap.Logger,
	signingKey []byte,
	publicURL string,
	dataStore storage.Store,
) func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		// Extract attachment ID from the request URI.
		// URI format: alluredeck://attachment/{id}
		uri := req.Params.URI
		rawID := strings.TrimPrefix(uri, "alluredeck://attachment/")
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid attachment URI %q", uri)
		}

		// Resolve the attachment's storage location (project storage key, build
		// number, source filename, MIME type). This single join lets us both
		// inline content and stream it via the signed download route, without
		// guessing the storage path.
		loc, err := stores.Attachment.GetLocation(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrAttachmentNotFound) {
				return nil, fmt.Errorf("attachment %d not found", id)
			}
			return nil, fmt.Errorf("fetching attachment %d: %w", id, err)
		}

		// loc.Source comes from ingested Allure reports (attacker-influenced).
		// Reject path-traversal sources before any storage path is built, so a
		// malicious source can neither be inlined nor handed out as a URL.
		if err := ValidateAttachmentSource(loc.Source); err != nil {
			return nil, fmt.Errorf("attachment %d: %w", id, err)
		}

		// Attempt to inline small text/image content directly when storage is
		// available, so the MCP client gets the content without a second round
		// trip. readAttachmentBlob mirrors the storage path used by the REST
		// ServeAttachment handler.
		//
		// Both text and image inlining are bounded by attachmentInlineMaxBytes
		// so a huge attachment cannot be read fully into memory and blow the
		// MCP client's context. Oversized attachments fall through to the
		// signed-download URL below.
		if dataStore != nil {
			inlineText := isTextMIME(loc.MimeType) && loc.SizeBytes <= attachmentInlineMaxBytes
			inlineImage := isImageMIME(loc.MimeType) && loc.SizeBytes <= attachmentInlineMaxBytes
			if inlineText || inlineImage {
				data, err := readAttachmentBlob(ctx, dataStore, loc)
				switch {
				case err != nil:
					logger.Warn("could not read attachment content, falling back to signed URL",
						zap.Int64("attachment_id", id), zap.Error(err))
				case inlineText:
					return &mcpsdk.ReadResourceResult{
						Contents: []*mcpsdk.ResourceContents{
							{
								URI:      uri,
								MIMEType: loc.MimeType,
								Text:     string(data),
							},
						},
					}, nil
				default: // inlineImage
					// Blob is []byte; the JSON encoder base64-encodes it
					// automatically, so the raw bytes are passed here.
					return &mcpsdk.ReadResourceResult{
						Contents: []*mcpsdk.ResourceContents{
							{
								URI:      uri,
								MIMEType: loc.MimeType,
								Blob:     data,
							},
						},
					}, nil
				}
			}
		}

		// Fallback: return a resource entry whose URI is the HMAC-signed download
		// URL. Clients can follow this URL to download the attachment directly
		// via the GET /attachments/{id} route.
		signedURL := buildSignedURL(publicURL, id, signingKey)
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{
				{
					URI:      signedURL,
					MIMEType: loc.MimeType,
					Text:     fmt.Sprintf("Download at: %s", signedURL),
				},
			},
		}, nil
	}
}

// readAttachmentBlob streams an attachment's bytes from the file-storage
// backend. It resolves the same storage path the REST ServeAttachment handler
// uses: {storageKey}/reports/{buildNumber}/data/attachments/{source}.
//
// loc.Source is attacker-influenced data from ingested Allure reports, so it is
// validated with ValidateAttachmentSource before being joined onto the storage
// path. The read is bounded at attachmentInlineMaxBytes as a defensive cap even
// when the caller has already checked loc.SizeBytes.
func readAttachmentBlob(ctx context.Context, dataStore storage.Store, loc *store.AttachmentLocation) ([]byte, error) {
	if err := ValidateAttachmentSource(loc.Source); err != nil {
		return nil, err
	}
	filePath := "data/attachments/" + loc.Source
	reader, _, err := dataStore.OpenReportFile(ctx, loc.StorageKey, strconv.Itoa(loc.BuildNumber), filePath)
	if err != nil {
		return nil, fmt.Errorf("opening attachment blob: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(io.LimitReader(reader, attachmentInlineMaxBytes))
	if err != nil {
		return nil, fmt.Errorf("reading attachment blob: %w", err)
	}
	return data, nil
}

func testResourceHandler(
	stores *bootstrap.Stores,
	_ *zap.Logger,
) func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		// Extract path parameters from URI.
		// URI format: alluredeck://project/{project_id}/build/{build_id}/test/{history_id}
		uri := req.Params.URI
		parts, err := parseTestResourceURI(uri)
		if err != nil {
			return nil, err
		}
		projectID, buildID, historyID := parts[0], parts[1], parts[2]

		projID, err := strconv.ParseInt(projectID, 10, 64)
		if err != nil || projID <= 0 {
			return nil, fmt.Errorf("invalid project_id %q in URI", projectID)
		}
		bldID, err := strconv.ParseInt(buildID, 10, 64)
		if err != nil || bldID <= 0 {
			return nil, fmt.Errorf("invalid build_id %q in URI", buildID)
		}
		if historyID == "" {
			return nil, fmt.Errorf("history_id must not be empty")
		}

		rows, err := stores.TestResult.ListFailedByBuild(ctx, projID, bldID, 1000)
		if err != nil {
			return nil, fmt.Errorf("fetching test results: %w", err)
		}

		var matched *store.TestResult
		for i := range rows {
			if rows[i].HistoryID == historyID {
				matched = &rows[i]
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("test with history_id %q not found in build %d", historyID, bldID)
		}

		// Fetch attachments best-effort.
		var attachmentCount int
		if attachments, _, err := stores.Attachment.ListByBuild(ctx, projID, bldID, "", "", 200, 0); err == nil {
			attachmentCount = len(attachments)
		}

		text := fmt.Sprintf("status: %s\nduration_ms: %d\nattachments: %d",
			matched.Status, matched.DurationMs, attachmentCount)

		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{
				{
					URI:      uri,
					MIMEType: "text/plain",
					Text:     text,
				},
			},
		}, nil
	}
}

// parseTestResourceURI extracts [project_id, build_id, history_id] from
// alluredeck://project/{project_id}/build/{build_id}/test/{history_id}.
func parseTestResourceURI(uri string) ([3]string, error) {
	// Strip scheme.
	path := strings.TrimPrefix(uri, "alluredeck://")
	// Expected segments: project/<id>/build/<id>/test/<hid>
	parts := strings.SplitN(path, "/", 6)
	if len(parts) < 6 || parts[0] != "project" || parts[2] != "build" || parts[4] != "test" {
		return [3]string{}, fmt.Errorf("invalid test resource URI: %q", uri)
	}
	return [3]string{parts[1], parts[3], parts[5]}, nil
}

// attachmentSigPayload builds the canonical payload string that is signed for
// an attachment download URL: "attachment:{id}:exp:{exp_unix}".
func attachmentSigPayload(id, exp int64) string {
	return fmt.Sprintf("attachment:%d:exp:%d", id, exp)
}

// signAttachment computes the HMAC-SHA256 signature (hex-encoded) over the
// canonical payload for the given attachment id and expiry.
func signAttachment(signingKey []byte, id, exp int64) string {
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(attachmentSigPayload(id, exp)))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAttachmentSig validates a signed attachment download request. It checks
// that exp has not passed and that sig is a constant-time match for the
// expected HMAC. It returns nil on success, or an error describing the failure.
//
// It is the canonical verifier for the GET /attachments/{id} download route and
// is the inverse of the signing performed by buildSignedURL.
func VerifyAttachmentSig(signingKey []byte, id, exp int64, sig string, now time.Time) error {
	if exp <= 0 {
		return errors.New("missing or invalid exp")
	}
	if now.Unix() > exp {
		return errors.New("signed URL has expired")
	}
	want := signAttachment(signingKey, id, exp)
	// Compare hex strings in constant time to avoid timing side channels.
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return errors.New("signature mismatch")
	}
	return nil
}

// buildSignedURL returns a signed URL for direct attachment download.
// The URL embeds exp (Unix timestamp) and sig (HMAC-SHA256 hex).
func buildSignedURL(publicURL string, id int64, signingKey []byte) string {
	exp := time.Now().Add(attachmentURLTTL).Unix()
	sig := signAttachment(signingKey, id, exp)
	base := strings.TrimRight(publicURL, "/")
	return fmt.Sprintf("%s/attachments/%d?exp=%d&sig=%s", base, id, exp, sig)
}
