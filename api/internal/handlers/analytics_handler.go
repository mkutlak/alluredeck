package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AnalyticsHandler handles the expanded analytics endpoints backed by the PG schema.
type AnalyticsHandler struct {
	analyticsStore store.AnalyticsStorer
	logger         *zap.Logger
}

// NewAnalyticsHandler creates an AnalyticsHandler. analyticsStore may be nil
// when analytics are unavailable — all endpoints return empty data in that case.
func NewAnalyticsHandler(analyticsStore store.AnalyticsStorer, logger *zap.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsStore: analyticsStore, logger: logger}
}

// parseClampedInt parses s as an int in [1,100], defaulting to 20.
func parseClampedInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return 20
	}
	if v > 100 {
		return 100
	}
	return v
}

// GetTopErrors returns the most common failure messages across recent builds.
func (h *AnalyticsHandler) GetTopErrors(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")

	q := r.URL.Query()
	builds := parseClampedInt(q.Get("builds"))
	limit := parseClampedInt(q.Get("limit"))

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []store.ErrorCluster{}, "project_id": projectID})
		return
	}

	data, err := h.analyticsStore.ListTopErrors(r.Context(), projectID, builds, limit)
	if err != nil {
		h.logger.Error("list top errors", zap.String("project_id", projectID), zap.Error(err))
		data = []store.ErrorCluster{}
	}
	if data == nil {
		data = []store.ErrorCluster{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "project_id": projectID})
}

// GetSuitePassRates returns per-suite pass rates across recent builds.
func (h *AnalyticsHandler) GetSuitePassRates(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")

	builds := parseClampedInt(r.URL.Query().Get("builds"))

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []store.SuitePassRate{}, "project_id": projectID})
		return
	}

	data, err := h.analyticsStore.ListSuitePassRates(r.Context(), projectID, builds)
	if err != nil {
		h.logger.Error("list suite pass rates", zap.String("project_id", projectID), zap.Error(err))
		data = []store.SuitePassRate{}
	}
	if data == nil {
		data = []store.SuitePassRate{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "project_id": projectID})
}

// GetLabelBreakdown returns counts grouped by label value for a given label name.
func (h *AnalyticsHandler) GetLabelBreakdown(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")

	q := r.URL.Query()
	labelName := q.Get("name")
	if labelName == "" {
		labelName = "severity"
	}
	builds := parseClampedInt(q.Get("builds"))

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []store.LabelCount{}, "project_id": projectID})
		return
	}

	data, err := h.analyticsStore.ListLabelBreakdown(r.Context(), projectID, labelName, builds)
	if err != nil {
		h.logger.Error("list label breakdown", zap.String("project_id", projectID), zap.Error(err))
		data = []store.LabelCount{}
	}
	if data == nil {
		data = []store.LabelCount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "project_id": projectID})
}
