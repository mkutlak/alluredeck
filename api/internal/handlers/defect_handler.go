package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// DefectHandler handles HTTP requests for defect fingerprint management.
type DefectHandler struct {
	defectStore store.DefectStorer
	projectsDir string
	logger      *zap.Logger
}

// NewDefectHandler creates a DefectHandler.
func NewDefectHandler(defectStore store.DefectStorer, projectsDir string, logger *zap.Logger) *DefectHandler {
	return &DefectHandler{defectStore: defectStore, projectsDir: projectsDir, logger: logger}
}

// parseDefectFilter extracts a DefectFilter from query parameters.
func (h *DefectHandler) parseDefectFilter(r *http.Request) store.DefectFilter {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return store.DefectFilter{
		Resolution: q.Get("resolution"),
		Category:   q.Get("category"),
		Search:     q.Get("search"),
		SortBy:     q.Get("sort_by"),
		Order:      q.Get("order"),
		Page:       page,
		PerPage:    perPage,
	}
}

// isValidDefectCategory returns true if s is a recognised defect category constant.
func isValidDefectCategory(s string) bool {
	switch s {
	case store.DefectCategoryProductBug,
		store.DefectCategoryTestBug,
		store.DefectCategoryInfrastructure,
		store.DefectCategoryToInvestigate:
		return true
	}
	return false
}

// isValidDefectResolution returns true if s is a recognised defect resolution constant.
func isValidDefectResolution(s string) bool {
	switch s {
	case store.DefectResolutionOpen,
		store.DefectResolutionFixed,
		store.DefectResolutionMuted,
		store.DefectResolutionWontFix:
		return true
	}
	return false
}

// ListProjectDefects handles GET /projects/{project_id}/defects
func (h *DefectHandler) ListProjectDefects(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	filter := h.parseDefectFilter(r)

	rows, total, err := h.defectStore.ListByProject(r.Context(), projectID, filter)
	if err != nil {
		h.logger.Error("list project defects", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing defects")
		return
	}
	if rows == nil {
		rows = []store.DefectListRow{}
	}

	writePagedSuccess(w, http.StatusOK, rows, "Defects successfully obtained", newPaginationMeta(filter.Page, filter.PerPage, total))
}

// ListBuildDefects handles GET /projects/{project_id}/builds/{build_id}/defects
func (h *DefectHandler) ListBuildDefects(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	buildIDStr := r.PathValue("build_id")
	buildID, err := strconv.ParseInt(buildIDStr, 10, 64)
	if err != nil || buildID < 1 {
		writeError(w, http.StatusBadRequest, "invalid build_id")
		return
	}

	filter := h.parseDefectFilter(r)

	rows, total, err := h.defectStore.ListByBuild(r.Context(), projectID, buildID, filter)
	if err != nil {
		h.logger.Error("list build defects", zap.String("project_id", projectID), zap.Int64("build_id", buildID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing defects")
		return
	}
	if rows == nil {
		rows = []store.DefectListRow{}
	}

	writePagedSuccess(w, http.StatusOK, rows, "Defects successfully obtained", newPaginationMeta(filter.Page, filter.PerPage, total))
}

// GetDefect handles GET /projects/{project_id}/defects/{defect_id}
func (h *DefectHandler) GetDefect(w http.ResponseWriter, r *http.Request) {
	defectID := r.PathValue("defect_id")
	if defectID == "" {
		writeError(w, http.StatusBadRequest, "defect_id is required")
		return
	}

	fp, err := h.defectStore.GetByID(r.Context(), defectID)
	if err != nil {
		if errors.Is(err, store.ErrDefectNotFound) {
			writeError(w, http.StatusNotFound, "defect not found")
			return
		}
		h.logger.Error("get defect", zap.String("defect_id", defectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching defect")
		return
	}

	writeSuccess(w, http.StatusOK, fp, "Defect successfully obtained")
}

// GetDefectTests handles GET /projects/{project_id}/defects/{defect_id}/tests
func (h *DefectHandler) GetDefectTests(w http.ResponseWriter, r *http.Request) {
	defectID := r.PathValue("defect_id")
	if defectID == "" {
		writeError(w, http.StatusBadRequest, "defect_id is required")
		return
	}

	q := r.URL.Query()

	var buildID *int64
	if bStr := q.Get("build_id"); bStr != "" {
		bid, err := strconv.ParseInt(bStr, 10, 64)
		if err != nil || bid < 1 {
			writeError(w, http.StatusBadRequest, "invalid build_id")
			return
		}
		buildID = &bid
	}

	pg := parsePagination(r)

	results, total, err := h.defectStore.GetTestResults(r.Context(), defectID, buildID, pg.Page, pg.PerPage)
	if err != nil {
		h.logger.Error("get defect tests", zap.String("defect_id", defectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching defect tests")
		return
	}
	if results == nil {
		results = []store.TestResult{}
	}

	writePagedSuccess(w, http.StatusOK, results, "Defect tests successfully obtained", newPaginationMeta(pg.Page, pg.PerPage, total))
}

// UpdateDefect handles PATCH /projects/{project_id}/defects/{defect_id}
func (h *DefectHandler) UpdateDefect(w http.ResponseWriter, r *http.Request) {
	defectID := r.PathValue("defect_id")
	if defectID == "" {
		writeError(w, http.StatusBadRequest, "defect_id is required")
		return
	}

	var req struct {
		Category     *string `json:"category"`
		Resolution   *string `json:"resolution"`
		KnownIssueID *int64  `json:"known_issue_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Category != nil && !isValidDefectCategory(*req.Category) {
		writeError(w, http.StatusBadRequest, "invalid category value")
		return
	}
	if req.Resolution != nil && !isValidDefectResolution(*req.Resolution) {
		writeError(w, http.StatusBadRequest, "invalid resolution value")
		return
	}

	if err := h.defectStore.UpdateDefect(r.Context(), defectID, req.Category, req.Resolution, req.KnownIssueID); err != nil {
		if errors.Is(err, store.ErrDefectNotFound) {
			writeError(w, http.StatusNotFound, "defect not found")
			return
		}
		h.logger.Error("update defect", zap.String("defect_id", defectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error updating defect")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"id": defectID}, "Defect updated")
}

// BulkUpdateDefects handles POST /projects/{project_id}/defects/bulk
func (h *DefectHandler) BulkUpdateDefects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DefectIDs  []string `json:"defect_ids"`
		Category   *string  `json:"category"`
		Resolution *string  `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.DefectIDs) == 0 {
		writeError(w, http.StatusBadRequest, "defect_ids must not be empty")
		return
	}

	if req.Category != nil && !isValidDefectCategory(*req.Category) {
		writeError(w, http.StatusBadRequest, "invalid category value")
		return
	}
	if req.Resolution != nil && !isValidDefectResolution(*req.Resolution) {
		writeError(w, http.StatusBadRequest, "invalid resolution value")
		return
	}

	if err := h.defectStore.BulkUpdate(r.Context(), req.DefectIDs, req.Category, req.Resolution); err != nil {
		h.logger.Error("bulk update defects", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error bulk updating defects")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"updated": len(req.DefectIDs)}, "Defects updated")
}

// GetProjectDefectSummary handles GET /projects/{project_id}/defects/summary
func (h *DefectHandler) GetProjectDefectSummary(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	summary, err := h.defectStore.GetProjectSummary(r.Context(), projectID)
	if err != nil {
		h.logger.Error("get project defect summary", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching defect summary")
		return
	}

	writeSuccess(w, http.StatusOK, summary, "Defect summary successfully obtained")
}

// GetBuildDefectSummary handles GET /projects/{project_id}/builds/{build_id}/defects/summary
func (h *DefectHandler) GetBuildDefectSummary(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	buildIDStr := r.PathValue("build_id")
	buildID, err := strconv.ParseInt(buildIDStr, 10, 64)
	if err != nil || buildID < 1 {
		writeError(w, http.StatusBadRequest, "invalid build_id")
		return
	}

	summary, err := h.defectStore.GetBuildSummary(r.Context(), projectID, buildID)
	if err != nil {
		h.logger.Error("get build defect summary", zap.String("project_id", projectID), zap.Int64("build_id", buildID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching build defect summary")
		return
	}

	writeSuccess(w, http.StatusOK, summary, "Build defect summary successfully obtained")
}
