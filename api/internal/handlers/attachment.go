package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AttachmentHandler handles attachment listing and serving endpoints.
type AttachmentHandler struct {
	attachmentStore store.AttachmentStorer
	buildStore      store.BuildStorer
	dataStore       storage.Store
	logger          *zap.Logger
}

// NewAttachmentHandler creates an AttachmentHandler.
func NewAttachmentHandler(
	attachmentStore store.AttachmentStorer,
	buildStore store.BuildStorer,
	dataStore storage.Store,
	logger *zap.Logger,
) *AttachmentHandler {
	return &AttachmentHandler{
		attachmentStore: attachmentStore,
		buildStore:      buildStore,
		dataStore:       dataStore,
		logger:          logger,
	}
}

// resolveBuild resolves the build for the given project and reportID.
// It returns the build and true on success, or writes an error response and returns false.
func (h *AttachmentHandler) resolveBuild(w http.ResponseWriter, r *http.Request, projectID, reportID string) (store.Build, bool) {
	ctx := r.Context()
	var build store.Build
	var err error

	if reportID == "latest" {
		build, err = h.buildStore.GetLatestBuild(ctx, projectID)
	} else {
		buildOrder, parseErr := strconv.Atoi(reportID)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "report_id must be a number or 'latest'")
			return store.Build{}, false
		}
		build, err = h.buildStore.GetBuildByOrder(ctx, projectID, buildOrder)
	}

	if err != nil {
		if errors.Is(err, store.ErrBuildNotFound) {
			writeError(w, http.StatusNotFound, "build not found")
			return store.Build{}, false
		}
		h.logger.Error("failed to resolve build", zap.String("project_id", projectID), zap.String("report_id", reportID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.Build{}, false
	}

	return build, true
}

// attachmentItem is the JSON shape for a single attachment in the list response.
type attachmentItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	URL       string `json:"url"`
}

// ListAttachments godoc
// @Summary      List attachments for a build
// @Description  Returns paginated attachment metadata for all test attachments in a build.
// @Tags         attachments
// @Produce      json
// @Param        project_id  path   string  true  "Project ID"
// @Param        report_id   path   string  true  "Build order number or 'latest'"
// @Param        mime_type   query  string  false "MIME type prefix filter (e.g. 'image')"
// @Param        limit       query  int     false "Max results (default 100, max 500)"
// @Param        offset      query  int     false "Pagination offset (default 0)"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/attachments [get]
func (h *AttachmentHandler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	build, ok := h.resolveBuild(w, r, projectID, reportID)
	if !ok {
		return
	}

	// Parse query params.
	q := r.URL.Query()
	mimeType := q.Get("mime_type")

	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	atts, total, err := h.attachmentStore.ListByBuild(ctx, projectID, build.ID, mimeType, limit, offset)
	if err != nil {
		h.logger.Error("failed to list attachments", zap.String("project_id", projectID), zap.Int64("build_id", build.ID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	items := make([]attachmentItem, len(atts))
	for i, att := range atts {
		items[i] = attachmentItem{
			ID:        att.ID,
			Name:      att.Name,
			Source:    att.Source,
			MimeType:  att.MimeType,
			SizeBytes: att.SizeBytes,
			URL:       fmt.Sprintf("/api/v1/projects/%s/reports/%d/attachments/%s", projectID, build.BuildOrder, att.Source),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"attachments": items,
			"total":       total,
			"limit":       limit,
			"offset":      offset,
		},
		"metadata": map[string]string{"message": "Attachments successfully retrieved"},
	})
}

// ServeAttachment godoc
// @Summary      Serve an attachment file inline
// @Description  Streams the raw attachment file with the correct Content-Type for inline viewing.
// @Tags         attachments
// @Produce      application/octet-stream
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Build order number or 'latest'"
// @Param        source      path  string  true  "Attachment source filename"
// @Success      200
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/attachments/{source} [get]
func (h *AttachmentHandler) ServeAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	source := r.PathValue("source")

	// Path traversal defense: reject dangerous characters.
	if strings.Contains(source, "/") ||
		strings.Contains(source, "\\") ||
		strings.Contains(source, "..") ||
		strings.ContainsRune(source, 0) {
		writeError(w, http.StatusBadRequest, "invalid attachment source")
		return
	}

	build, ok := h.resolveBuild(w, r, projectID, reportID)
	if !ok {
		return
	}

	filePath := "data/attachments/" + source

	reader, mimeType, err := h.dataStore.OpenReportFile(ctx, projectID, strconv.Itoa(build.BuildOrder), filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment file not found")
		return
	}
	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", source))

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream attachment", zap.String("source", source), zap.Error(err))
	}
}
