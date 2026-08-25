package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/attachmentio"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ---------------------------------------------------------------------------
// list_attachments
// ---------------------------------------------------------------------------

// ListAttachmentsInput holds parameters for list_attachments.
type ListAttachmentsInput struct {
	ProjectID int    `json:"project_id"`
	BuildID   int64  `json:"build_id"`
	HistoryID string `json:"history_id"`
}

// AttachmentItem is one attachment in the list_attachments response.
type AttachmentItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Mime        string `json:"mime"`
	SizeBytes   int64  `json:"size_bytes"`
	ResourceURI string `json:"resource_uri"`
}

// ListAttachmentsOutput is the structured output for list_attachments.
type ListAttachmentsOutput struct {
	Items []AttachmentItem `json:"items"`
}

// RegisterAttachmentTools registers list_attachments on s.
func RegisterAttachmentTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_attachments",
		Title:       "List AllureDeck attachments",
		Annotations: readOnlyAnnotations(),
		Description: "List attachments for a specific test in a build. Returns metadata and resource URIs only — call get_attachment with the returned id AND the same project_id to retrieve content (text, image, or a download link); get_attachment requires project_id because an attachment id alone is not scoped to a project.",
	}, listAttachmentsHandler(stores, logger))
}

func listAttachmentsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListAttachmentsInput) (*mcpsdk.CallToolResult, ListAttachmentsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListAttachmentsInput) (*mcpsdk.CallToolResult, ListAttachmentsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.BuildID <= 0 {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("build_id must be positive")
		}
		if in.HistoryID == "" {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("history_id must not be empty")
		}

		// Scoped to the single test identified by history_id via
		// test_result_id, mirroring diagnoseAttachments in diagnose.go. A
		// prior version called ListByBuild here, which ignored HistoryID
		// entirely and returned every attachment in the build regardless of
		// which test the caller asked about.
		attachments, err := stores.Attachment.ListByTestResult(ctx, int64(in.ProjectID), in.BuildID, in.HistoryID, 200)
		if err != nil {
			return nil, ListAttachmentsOutput{}, fmt.Errorf("listing attachments: %w", err)
		}

		items := make([]AttachmentItem, 0, len(attachments))
		var totalBytes int64
		for _, a := range attachments {
			totalBytes += a.SizeBytes
			items = append(items, AttachmentItem{
				ID:          a.ID,
				Name:        a.Name,
				Mime:        a.MimeType,
				SizeBytes:   a.SizeBytes,
				ResourceURI: fmt.Sprintf("alluredeck://attachment/%d", a.ID),
			})
		}

		digest := fmt.Sprintf("%d attachment(s), %d byte(s) total for test %s in build %d",
			len(items), totalBytes, in.HistoryID, in.BuildID)
		return textResult(digest), ListAttachmentsOutput{Items: items}, nil
	}
}

// ---------------------------------------------------------------------------
// get_attachment
//
// AI agents reach this MCP server through a gateway that proxies tools but
// not resources, so the alluredeck://attachment/{id} resource
// (internal/mcp/resources.go) is unreachable in that deployment even though
// it does the same job. get_attachment exists so the highest-signal
// debugging evidence — Playwright _error-context-N markdown page snapshots,
// stdout, stderr, screenshots — stays reachable as a plain tool call.
// ---------------------------------------------------------------------------

// attachmentContentMaxBytes is the hard ceiling both on max_bytes (the text
// windowing cap) and on the image inline size. It is the same 2 MB cap the
// alluredeck://attachment/{id} resource applies (attachmentInlineMaxBytes in
// resources.go) so behavior is consistent across the resource and tool
// surfaces.
const attachmentContentMaxBytes = 2 * 1024 * 1024 // 2 MB

// getAttachmentDefaultMaxBytes is the max_bytes used when the caller omits
// the field (or passes zero, which is indistinguishable from omitted on the
// wire).
const getAttachmentDefaultMaxBytes = 65536

// GetAttachmentInput holds parameters for get_attachment.
type GetAttachmentInput struct {
	// ProjectID is the project the attachment must belong to. It is required:
	// test_attachments.id is a global sequence, so a bare-id lookup would let
	// any caller read every attachment in the deployment by enumerating ids.
	// The handler re-resolves the attachment's owning project and refuses the
	// read when it differs.
	ProjectID int `json:"project_id" jsonschema:"Internal numeric project id that owns the attachment — required. Use the same project_id you passed to list_attachments or diagnose_failure; an attachment id from another project is reported as not found."`
	// AttachmentID identifies the attachment, as returned by list_attachments
	// or diagnose_failure.
	AttachmentID int64 `json:"attachment_id"`
	// Offset is the byte offset into the attachment's text content to start
	// reading from. Ignored for image content, which is always returned in
	// full (images are capped at 2 MB — see attachmentContentMaxBytes).
	Offset int `json:"offset,omitempty"`
	// MaxBytes bounds how many bytes of text content are returned in one
	// call. Zero (the default) means 65536; any value is clamped to
	// [1, 2097152].
	MaxBytes int `json:"max_bytes,omitempty"`
}

// GetAttachmentOutput is the structured output for get_attachment.
type GetAttachmentOutput struct {
	AttachmentID int64  `json:"attachment_id"`
	Name         string `json:"name"`
	Mime         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	// Offset echoes the request's (possibly defaulted) offset.
	Offset int `json:"offset"`
	// ReturnedBytes is how many attachment bytes were actually read for this
	// call. For text content it is the window size before the truncation
	// marker is appended; for images it is the full image size.
	ReturnedBytes int `json:"returned_bytes"`
	// Truncated is true when more text content remains beyond this window
	// (offset + returned_bytes < size_bytes). Always false for image content
	// and for the signed-URL-only branches.
	Truncated bool `json:"truncated"`
	// SignedURL is a time-limited direct-download link. Set whenever content
	// was not fully inlined: truncated text, non-text/non-image MIME types,
	// an oversized image, or no storage backend configured.
	SignedURL string `json:"signed_url,omitempty"`
}

// RegisterAttachmentContentTool registers get_attachment on s.
//
// It is registered separately from RegisterAttachmentTools (called via
// RegisterAll in tools.go) because it needs the signing/storage dependencies
// that RegisterAll's other tools don't: signingKey and publicURL to issue
// signed download links, and dataStore to read blob content. Wire this up
// from internal/mcp/server.go, alongside the RegisterResources call that
// takes the same dependencies for the equivalent resource.
//
// dataStore may be nil; in that case every call falls back to a signed
// download URL rather than inlining content, matching the resource handler's
// behavior in resources.go.
func RegisterAttachmentContentTool(
	s *mcpsdk.Server,
	stores *bootstrap.Stores,
	logger *zap.Logger,
	signingKey []byte,
	publicURL string,
	dataStore storage.Store,
) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_attachment",
		Title:       "Get AllureDeck attachment content",
		Annotations: readOnlyAnnotations(),
		Description: "Fetch the content of a single attachment by id (from list_attachments or diagnose_failure) — Playwright _error-context-N markdown page snapshots, stdout, stderr logs, and screenshots. Use this after diagnose_failure to read the actual evidence, not just the reference. project_id is REQUIRED and must be the project that owns the attachment: an attachment id belonging to another project is reported as not found, so pass the same project_id you used to find the id. Text content is windowed: pass offset/max_bytes to page through large files, and re-call with the offset given in the truncation marker to continue; an offset at or past the end returns empty content with returned_bytes=0. Raster images up to 2 MB come back as an image content block. Anything else — image/svg+xml (a scriptable document, never inlined), other non-text types, or content too large to inline — is returned as structured metadata with a time-limited signed_url instead.",
	}, getAttachmentHandler(stores, logger, signingKey, publicURL, dataStore))
}

// resolveMaxBytes applies the get_attachment max_bytes default/clamp rule: 0
// (unset) becomes getAttachmentDefaultMaxBytes; any other value is clamped to
// [1, attachmentContentMaxBytes].
func resolveMaxBytes(requested int) int {
	if requested == 0 {
		return getAttachmentDefaultMaxBytes
	}
	if requested < 1 {
		return 1
	}
	if requested > attachmentContentMaxBytes {
		return attachmentContentMaxBytes
	}
	return requested
}

// attachmentName resolves the human-readable attachment filename via
// GetByID. GetLocation (used to resolve the storage path) does not carry
// Name, so this is a second, best-effort lookup: on error it falls back to
// the storage source filename so the tool call still succeeds with the best
// available label rather than failing outright.
func attachmentName(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, id int64, source string) string {
	a, err := stores.Attachment.GetByID(ctx, id)
	if err != nil {
		logger.Warn("get_attachment: could not resolve attachment name, falling back to source filename",
			zap.Int64("attachment_id", id), zap.Error(err))
		return source
	}
	return a.Name
}

func getAttachmentHandler(
	stores *bootstrap.Stores,
	logger *zap.Logger,
	signingKey []byte,
	publicURL string,
	dataStore storage.Store,
) func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetAttachmentInput) (*mcpsdk.CallToolResult, GetAttachmentOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in GetAttachmentInput) (*mcpsdk.CallToolResult, GetAttachmentOutput, error) {
		if in.ProjectID <= 0 {
			return nil, GetAttachmentOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.AttachmentID <= 0 {
			return nil, GetAttachmentOutput{}, fmt.Errorf("attachment_id must be positive")
		}
		if in.Offset < 0 {
			return nil, GetAttachmentOutput{}, fmt.Errorf("offset must be non-negative")
		}
		maxBytes := resolveMaxBytes(in.MaxBytes)

		loc, err := stores.Attachment.GetLocation(ctx, in.AttachmentID)
		if err != nil {
			if errors.Is(err, store.ErrAttachmentNotFound) {
				return nil, GetAttachmentOutput{}, fmt.Errorf("attachment %d not found", in.AttachmentID)
			}
			return nil, GetAttachmentOutput{}, fmt.Errorf("fetching attachment %d: %w", in.AttachmentID, err)
		}

		// Scope the read to the project the caller asked about. The error is
		// byte-for-byte the not-found error above on purpose: distinguishing
		// "wrong project" from "no such id" would turn this tool into an
		// existence oracle over every project in the deployment.
		if loc.ProjectID != int64(in.ProjectID) {
			logger.Warn("get_attachment: attachment belongs to another project",
				zap.Int64("attachment_id", in.AttachmentID),
				zap.Int("requested_project_id", in.ProjectID),
				zap.Int64("owning_project_id", loc.ProjectID))
			return nil, GetAttachmentOutput{}, fmt.Errorf("attachment %d not found", in.AttachmentID)
		}

		// loc.Source comes from ingested Allure reports (attacker-influenced).
		// Reject path-traversal sources before any storage path is built, so a
		// malicious source can neither be inlined nor handed out as a URL.
		if err := attachmentio.ValidateSource(loc.Source); err != nil {
			return nil, GetAttachmentOutput{}, fmt.Errorf("attachment %d: %w", in.AttachmentID, err)
		}

		out := GetAttachmentOutput{
			AttachmentID: in.AttachmentID,
			Name:         attachmentName(ctx, stores, logger, in.AttachmentID, loc.Source),
			Mime:         loc.MimeType,
			SizeBytes:    loc.SizeBytes,
			Offset:       in.Offset,
		}

		// No storage backend configured: every attachment falls back to a
		// signed download URL, mirroring the MCP resource's dataStore==nil
		// behavior in resources.go.
		if dataStore == nil {
			out.SignedURL = attachmentio.SignURL(publicURL, in.AttachmentID, signingKey)
			res := textResult(attachmentDigest(&out, "no storage backend; download via signed_url"))
			return res, out, nil
		}

		switch {
		case attachmentio.IsTextMIME(loc.MimeType):
			data, readErr := attachmentio.ReadBlobWindow(ctx, dataStore, loc, in.Offset, maxBytes)
			if readErr != nil {
				logger.Warn("get_attachment: could not read text content, falling back to signed URL",
					zap.Int64("attachment_id", in.AttachmentID), zap.Error(readErr))
				out.SignedURL = attachmentio.SignURL(publicURL, in.AttachmentID, signingKey)
				res := textResult(attachmentDigest(&out, "text read failed; download via signed_url"))
				return res, out, nil
			}
			out.ReturnedBytes = len(data)
			end := int64(in.Offset) + int64(len(data))
			out.Truncated = end < loc.SizeBytes

			// The window is cut on byte offsets, so both edges can land inside
			// a multi-byte rune. Drop those partial fragments rather than emit
			// invalid UTF-8 that a JSON encoder would mangle into U+FFFD.
			text := strings.ToValidUTF8(string(data), "")
			// A signed URL is issued only when bytes were left behind. Fully
			// inlined content needs no download link, and handing one out
			// would extend a credential for no reason.
			if out.Truncated {
				text += fmt.Sprintf("\n…[truncated: bytes %d-%d of %d; call again with offset=%d]",
					in.Offset, end, loc.SizeBytes, end)
				out.SignedURL = attachmentio.SignURL(publicURL, in.AttachmentID, signingKey)
			}
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
			}, out, nil

		case attachmentio.IsImageMIME(loc.MimeType) && loc.SizeBytes <= attachmentContentMaxBytes:
			data, readErr := attachmentio.ReadBlobWindow(ctx, dataStore, loc, 0, attachmentContentMaxBytes)
			if readErr != nil {
				logger.Warn("get_attachment: could not read image content, falling back to signed URL",
					zap.Int64("attachment_id", in.AttachmentID), zap.Error(readErr))
				out.SignedURL = attachmentio.SignURL(publicURL, in.AttachmentID, signingKey)
				res := textResult(attachmentDigest(&out, "image read failed; download via signed_url"))
				return res, out, nil
			}
			out.ReturnedBytes = len(data)
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.ImageContent{Data: data, MIMEType: loc.MimeType}},
			}, out, nil

		default:
			// Non-text, non-image MIME types (zip, video, image/svg+xml, ...)
			// and images over the inline cap are never inlined; the caller
			// downloads via the signed URL instead.
			out.SignedURL = attachmentio.SignURL(publicURL, in.AttachmentID, signingKey)
			res := textResult(attachmentDigest(&out, "not inlined; download via signed_url"))
			return res, out, nil
		}
	}
}

// attachmentDigest renders the one-line headline for a get_attachment result
// that carries no content block.
//
// Those branches previously returned a nil CallToolResult, which makes the SDK
// fill the text channel with the JSON serialization of the whole structured
// output — the client then pays for the same metadata twice. The branches that
// DO return content (inlined text, inlined image) keep returning it: the SDK
// leaves a non-nil Content untouched, so the payload is not duplicated there.
func attachmentDigest(out *GetAttachmentOutput, reason string) string {
	return fmt.Sprintf("attachment %d %q (%s, %d bytes): %s",
		out.AttachmentID, out.Name, out.Mime, out.SizeBytes, reason)
}
