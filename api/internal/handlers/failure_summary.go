package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/failure"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// aiDisclaimer is attached to every generated summary. The output is a
// hypothesis to guide investigation, never an authoritative verdict.
const aiDisclaimer = "AI hypothesis — verify before acting."

// FailureSummaryHandler serves the opt-in, in-product LLM failure summary for a
// single failing test.
type FailureSummaryHandler struct {
	svc          *failure.Service
	projectStore store.ProjectStorer
	logger       *zap.Logger
}

// NewFailureSummaryHandler creates a FailureSummaryHandler.
func NewFailureSummaryHandler(svc *failure.Service, projectStore store.ProjectStorer, logger *zap.Logger) *FailureSummaryHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FailureSummaryHandler{svc: svc, projectStore: projectStore, logger: logger}
}

// failureSummaryBody is the JSON shape of a generated summary.
type failureSummaryBody struct {
	Hypothesis string   `json:"hypothesis"`
	Category   string   `json:"category"`
	Confidence string   `json:"confidence,omitempty"`
	Evidence   []string `json:"evidence"`
}

// failureSummaryLastGood is the JSON shape of the last-good pointer in the
// response (a REST-specific projection of failure.LastGood).
type failureSummaryLastGood struct {
	BuildNumber int    `json:"build_number"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	BuildsSince int    `json:"builds_since"`
}

// failureSummaryData is the enabled-path "data" object.
type failureSummaryData struct {
	Enabled     bool                    `json:"enabled"`
	Cached      bool                    `json:"cached"`
	BuildID     int64                   `json:"build_id"`
	HistoryID   string                  `json:"history_id"`
	Summary     *failureSummaryBody     `json:"summary"`
	LastGood    *failureSummaryLastGood `json:"last_good,omitempty"`
	Model       string                  `json:"model"`
	GeneratedAt string                  `json:"generated_at,omitempty"`
	Disclaimer  string                  `json:"disclaimer"`
	Error       string                  `json:"error,omitempty"`
}

// failureSummaryEnvelope is the standard {data, metadata} response envelope.
type failureSummaryEnvelope struct {
	Data     any               `json:"data"`
	MetaData map[string]string `json:"metadata"`
}

// GetFailureSummary godoc
// @Summary      Get the AI failure summary for a failing test
// @Description  Returns a cached or freshly generated LLM hypothesis about why
// @Description  a test failed. Off by default; when disabled the body reports
// @Description  enabled:false. The summary is an AI hypothesis, not a verdict.
// @Tags         failures
// @Produce      json
// @Param        project_id  path   string  true  "Project ID or slug"
// @Param        build_id    path   int     true  "Build ID (surrogate PK)"
// @Param        history_id  path   string  true  "Test history ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/builds/{build_id}/tests/{history_id}/failure-summary [get]
func (h *FailureSummaryHandler) GetFailureSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	buildID, err := strconv.ParseInt(r.PathValue("build_id"), 10, 64)
	if err != nil || buildID <= 0 {
		writeError(w, http.StatusBadRequest, "build_id must be a positive integer")
		return
	}

	historyID := r.PathValue("history_id")
	if historyID == "" {
		writeError(w, http.StatusBadRequest, "history_id is required")
		return
	}

	res, err := h.svc.SummaryFor(ctx, projectID, buildID, historyID)
	if err != nil {
		if errors.Is(err, store.ErrBuildNotFound) {
			writeError(w, http.StatusNotFound, "build not found")
			return
		}
		h.logger.Error("failure summary failed",
			zap.Int64("project_id", projectID), zap.Int64("build_id", buildID),
			zap.String("history_id", historyID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !res.Enabled {
		writeJSON(w, http.StatusOK, failureSummaryEnvelope{
			Data:     map[string]any{"enabled": false},
			MetaData: map[string]string{"message": "LLM summaries are disabled"},
		})
		return
	}

	data := failureSummaryData{
		Enabled:    true,
		Cached:     res.Cached,
		BuildID:    buildID,
		HistoryID:  historyID,
		Model:      res.Model,
		Disclaimer: aiDisclaimer,
	}
	if res.LastGood != nil {
		data.LastGood = &failureSummaryLastGood{
			BuildNumber: res.LastGood.BuildNumber,
			CommitSHA:   res.LastGood.CommitSHA,
			BuildsSince: res.LastGood.BuildsSince,
		}
	}

	message := "Failure summary generated"
	switch {
	case res.Err != nil:
		// Soft generation failure: enabled:true, summary null, error set. Never 5xx.
		data.Error = "generation failed"
		message = "Failure summary generation failed"
	case res.Summary != nil:
		data.Summary = &failureSummaryBody{
			Hypothesis: res.Summary.Hypothesis,
			Category:   res.Summary.Category,
			Confidence: res.Summary.Confidence,
			Evidence:   res.Summary.Evidence,
		}
		data.GeneratedAt = res.Summary.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	writeJSON(w, http.StatusOK, failureSummaryEnvelope{
		Data:     data,
		MetaData: map[string]string{"message": message},
	})
}
