package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AnalyticsHandler handles the expanded analytics endpoints backed by the PG schema.
type AnalyticsHandler struct {
	analyticsStore store.AnalyticsStorer
	branchStore    store.BranchStorer
	logger         *zap.Logger
}

// NewAnalyticsHandler creates an AnalyticsHandler. analyticsStore may be nil
// when analytics are unavailable — all endpoints return empty data in that case.
func NewAnalyticsHandler(analyticsStore store.AnalyticsStorer, branchStore store.BranchStorer, logger *zap.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsStore: analyticsStore, branchStore: branchStore, logger: logger}
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

// resolveBranchID resolves a branch name to its ID using the branch store.
// Returns nil if the branch name is empty, the store is nil, or the branch is not found.
func (h *AnalyticsHandler) resolveBranchID(r *http.Request, projectID, branchName string) *int64 {
	if branchName == "" || h.branchStore == nil {
		return nil
	}
	br, err := h.branchStore.GetByName(r.Context(), projectID, branchName)
	if err != nil || br == nil {
		return nil
	}
	return &br.ID
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

	branchID := h.resolveBranchID(r, projectID, q.Get("branch"))

	data, err := h.analyticsStore.ListTopErrors(r.Context(), projectID, builds, limit, branchID)
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

	q := r.URL.Query()
	builds := parseClampedInt(q.Get("builds"))

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []store.SuitePassRate{}, "project_id": projectID})
		return
	}

	branchID := h.resolveBranchID(r, projectID, q.Get("branch"))

	data, err := h.analyticsStore.ListSuitePassRates(r.Context(), projectID, builds, branchID)
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

	branchID := h.resolveBranchID(r, projectID, q.Get("branch"))

	data, err := h.analyticsStore.ListLabelBreakdown(r.Context(), projectID, labelName, builds, branchID)
	if err != nil {
		h.logger.Error("list label breakdown", zap.String("project_id", projectID), zap.Error(err))
		data = []store.LabelCount{}
	}
	if data == nil {
		data = []store.LabelCount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "project_id": projectID})
}

// GetTrends returns per-build statistics for trend charts.
func (h *AnalyticsHandler) GetTrends(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	q := r.URL.Query()
	builds := parseClampedInt(q.Get("builds"))

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":       emptyTrendsResponse(),
			"project_id": projectID,
		})
		return
	}

	branchID := h.resolveBranchID(r, projectID, q.Get("branch"))

	points, err := h.analyticsStore.ListTrendPoints(r.Context(), projectID, builds, branchID)
	if err != nil {
		h.logger.Error("list trend points", zap.String("project_id", projectID), zap.Error(err))
		points = []store.TrendPoint{}
	}
	if points == nil {
		points = []store.TrendPoint{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":       buildTrendsResponse(points),
		"project_id": projectID,
	})
}

type trendsResponse struct {
	Status   []statusPoint   `json:"status"`
	PassRate []passRatePoint `json:"pass_rate"`
	Duration []durationPoint `json:"duration"`
	Kpi      *kpiResponse    `json:"kpi"`
}

type statusPoint struct {
	Name    string `json:"name"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Broken  int    `json:"broken"`
	Skipped int    `json:"skipped"`
}

type passRatePoint struct {
	Name     string  `json:"name"`
	PassRate float64 `json:"pass_rate"`
}

type durationPoint struct {
	Name        string `json:"name"`
	DurationSec int    `json:"duration_sec"`
}

type kpiResponse struct {
	PassRate        float64   `json:"pass_rate"`
	PassRateTrend   []float64 `json:"pass_rate_trend"`
	TotalTests      int       `json:"total_tests"`
	TotalTestsTrend []int     `json:"total_tests_trend"`
	AvgDuration     int64     `json:"avg_duration"`
	DurationTrend   []int64   `json:"duration_trend"`
	FailedCount     int       `json:"failed_count"`
	FailedTrend     []int     `json:"failed_trend"`
}

func emptyTrendsResponse() trendsResponse {
	return trendsResponse{
		Status:   []statusPoint{},
		PassRate: []passRatePoint{},
		Duration: []durationPoint{},
		Kpi:      nil,
	}
}

func buildTrendsResponse(points []store.TrendPoint) trendsResponse {
	status := make([]statusPoint, 0, len(points))
	passRate := make([]passRatePoint, 0, len(points))
	duration := make([]durationPoint, 0, len(points))

	for _, p := range points {
		name := fmt.Sprintf("#%d", p.BuildOrder)
		status = append(status, statusPoint{
			Name:    name,
			Passed:  p.Passed,
			Failed:  p.Failed,
			Broken:  p.Broken,
			Skipped: p.Skipped,
		})
		passRate = append(passRate, passRatePoint{
			Name:     name,
			PassRate: p.PassRate,
		})
		duration = append(duration, durationPoint{
			Name:        name,
			DurationSec: int(math.Round(float64(p.DurationMs) / 1000)),
		})
	}

	var kpi *kpiResponse
	if len(points) > 0 {
		sparklineN := min(len(points), 10)
		sparkline := points[len(points)-sparklineN:]

		passRateTrend := make([]float64, 0, sparklineN)
		totalTrend := make([]int, 0, sparklineN)
		durationTrend := make([]int64, 0, sparklineN)
		failedTrend := make([]int, 0, sparklineN)
		for _, p := range sparkline {
			passRateTrend = append(passRateTrend, p.PassRate)
			totalTrend = append(totalTrend, p.Total)
			durationTrend = append(durationTrend, p.DurationMs)
			failedTrend = append(failedTrend, p.Failed+p.Broken)
		}

		latest := points[len(points)-1]
		kpi = &kpiResponse{
			PassRate:        latest.PassRate,
			PassRateTrend:   passRateTrend,
			TotalTests:      latest.Total,
			TotalTestsTrend: totalTrend,
			AvgDuration:     latest.DurationMs,
			DurationTrend:   durationTrend,
			FailedCount:     latest.Failed + latest.Broken,
			FailedTrend:     failedTrend,
		}
	}

	return trendsResponse{
		Status:   status,
		PassRate: passRate,
		Duration: duration,
		Kpi:      kpi,
	}
}
