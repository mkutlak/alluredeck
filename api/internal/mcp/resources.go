package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
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
	// alluredeck://attachment/{id}
	s.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		URITemplate: "alluredeck://attachment/{id}",
		Name:        "attachment",
		Description: "Retrieve attachment content. Text and small images are inlined; large files return a signed download link as the resource URI.",
	}, attachmentResourceHandler(stores, logger, signingKey, publicURL, dataStore))

	// alluredeck://project/{project_id}/build/{build_id}/test/{history_id}
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

		att, err := stores.Attachment.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrAttachmentNotFound) {
				return nil, fmt.Errorf("attachment %d not found", id)
			}
			return nil, fmt.Errorf("fetching attachment %d: %w", id, err)
		}

		// Attempt text inline when storage is available.
		if isTextMIME(att.MimeType) && dataStore != nil {
			data, err := dataStore.ReadFile(ctx, "", att.Source)
			if err == nil {
				return &mcpsdk.ReadResourceResult{
					Contents: []*mcpsdk.ResourceContents{
						{
							URI:      uri,
							MIMEType: att.MimeType,
							Text:     string(data),
						},
					},
				}, nil
			}
			logger.Warn("could not read text attachment, falling back to signed URL",
				zap.Int64("attachment_id", id), zap.Error(err))
		}

		// Attempt blob inline for small images when storage is available.
		if isImageMIME(att.MimeType) && att.SizeBytes <= attachmentInlineMaxBytes && dataStore != nil {
			data, err := dataStore.ReadFile(ctx, "", att.Source)
			if err == nil {
				return &mcpsdk.ReadResourceResult{
					Contents: []*mcpsdk.ResourceContents{
						{
							URI:      uri,
							MIMEType: att.MimeType,
							Blob:     []byte(base64.StdEncoding.EncodeToString(data)),
						},
					},
				}, nil
			}
			logger.Warn("could not read image attachment, falling back to signed URL",
				zap.Int64("attachment_id", id), zap.Error(err))
		}

		// Fallback: return a resource entry whose URI is the HMAC-signed download URL.
		// Clients can follow this URL to download the attachment directly.
		signedURL := buildSignedURL(publicURL, id, signingKey)
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{
				{
					URI:      signedURL,
					MIMEType: att.MimeType,
					Text:     fmt.Sprintf("Download at: %s", signedURL),
				},
			},
		}, nil
	}
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

// buildSignedURL returns a signed URL for direct attachment download.
// The URL embeds exp (Unix timestamp) and sig (HMAC-SHA256 hex).
func buildSignedURL(publicURL string, id int64, signingKey []byte) string {
	exp := time.Now().Add(10 * time.Minute).Unix()
	payload := fmt.Sprintf("attachment:%d:exp:%d", id, exp)
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	base := strings.TrimRight(publicURL, "/")
	return fmt.Sprintf("%s/attachments/%d?exp=%d&sig=%s", base, id, exp, sig)
}
