package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/mcp"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AttachmentDownloadHandler serves attachments via HMAC-signed, time-limited
// URLs. It backs the GET /attachments/{id} route that MCP resource links point
// at: the MCP server hands clients a signed URL, and this handler verifies the
// signature and streams the blob from file storage.
type AttachmentDownloadHandler struct {
	attachmentStore store.AttachmentStorer
	dataStore       storage.Store
	signingKey      []byte
	logger          *zap.Logger
}

// NewAttachmentDownloadHandler creates an AttachmentDownloadHandler.
func NewAttachmentDownloadHandler(
	attachmentStore store.AttachmentStorer,
	dataStore storage.Store,
	signingKey []byte,
	logger *zap.Logger,
) *AttachmentDownloadHandler {
	return &AttachmentDownloadHandler{
		attachmentStore: attachmentStore,
		dataStore:       dataStore,
		signingKey:      signingKey,
		logger:          logger,
	}
}

// ServeSignedAttachment godoc
// @Summary      Download an attachment via a signed URL
// @Description  Streams an attachment blob after verifying an HMAC-signed,
// @Description  time-limited URL. The exp and sig query params are produced by
// @Description  the MCP server when it hands out attachment resource links.
// @Tags         attachments
// @Produce      application/octet-stream
// @Param        id   path   int     true  "Attachment ID"
// @Param        exp  query  int     true  "Expiry Unix timestamp"
// @Param        sig  query  string  true  "HMAC-SHA256 signature (hex)"
// @Success      200
// @Failure      400  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /attachments/{id} [get]
func (h *AttachmentDownloadHandler) ServeSignedAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "attachment id must be a positive integer")
		return
	}

	q := r.URL.Query()
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "exp must be a Unix timestamp")
		return
	}
	sig := q.Get("sig")
	if sig == "" {
		writeError(w, http.StatusBadRequest, "sig is required")
		return
	}

	// Verify the HMAC signature and expiry using the canonical MCP verifier.
	if err := mcp.VerifyAttachmentSig(h.signingKey, id, exp, sig, time.Now()); err != nil {
		// Do not leak whether the failure was expiry vs. tampering in the body;
		// both are 403. The detail is logged for operators.
		h.logger.Warn("rejected signed attachment download",
			zap.Int64("attachment_id", id), zap.Error(err))
		writeError(w, http.StatusForbidden, "invalid or expired download link")
		return
	}

	// Resolve the attachment's storage location.
	loc, err := h.attachmentStore.GetLocation(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrAttachmentNotFound) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		h.logger.Error("failed to resolve attachment location",
			zap.Int64("attachment_id", id), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// loc.Source comes from ingested Allure reports (attacker-influenced).
	// Reject path-traversal sources before building any storage path, using
	// the same canonical guard the REST ServeAttachment handler applies.
	if err := mcp.ValidateAttachmentSource(loc.Source); err != nil {
		h.logger.Warn("rejected signed attachment download: invalid source",
			zap.Int64("attachment_id", id), zap.String("source", loc.Source))
		writeError(w, http.StatusBadRequest, "invalid attachment source")
		return
	}

	// Stream the blob. The storage path mirrors the REST ServeAttachment
	// handler: {storageKey}/reports/{buildNumber}/data/attachments/{source}.
	filePath := "data/attachments/" + loc.Source
	reader, mimeType, err := h.dataStore.OpenReportFile(ctx, loc.StorageKey, strconv.Itoa(loc.BuildNumber), filePath)
	if err != nil {
		h.logger.Warn("attachment blob not found in storage",
			zap.Int64("attachment_id", id), zap.String("source", loc.Source), zap.Error(err))
		writeError(w, http.StatusNotFound, "attachment file not found")
		return
	}
	defer func() { _ = reader.Close() }()

	// Prefer the MIME type recorded at parse time; fall back to the storage
	// backend's detected type.
	if loc.MimeType != "" {
		mimeType = loc.MimeType
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", loc.Source))

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream signed attachment",
			zap.Int64("attachment_id", id), zap.Error(err))
	}
}
