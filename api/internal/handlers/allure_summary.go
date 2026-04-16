package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const topFailuresLimit = 10

type summaryBuildMeta struct {
	BuildID     int64  `json:"build_id"`
	ProjectID   int64  `json:"project_id"`
	BuildNumber int    `json:"build_number"`
	CreatedAt   string `json:"created_at"`
	IsLatest    bool   `json:"is_latest"`
	CIProvider  string `json:"ci_provider,omitempty"`
	CIBuildURL  string `json:"ci_build_url,omitempty"`
	CIBranch    string `json:"ci_branch,omitempty"`
	CICommitSHA string `json:"ci_commit_sha,omitempty"`
}

type summaryStatistics struct {
	Passed     int     `json:"passed"`
	Failed     int     `json:"failed"`
	Broken     int     `json:"broken"`
	Skipped    int     `json:"skipped"`
	Unknown    int     `json:"unknown"`
	Total      int     `json:"total"`
	PassedPct  float64 `json:"passed_pct"`
	FailedPct  float64 `json:"failed_pct"`
	BrokenPct  float64 `json:"broken_pct"`
	SkippedPct float64 `json:"skipped_pct"`
	UnknownPct float64 `json:"unknown_pct"`
}

type summaryTiming struct {
	DurationMs int64 `json:"duration_ms"`
}

type summaryQuality struct {
	FlakyCount     int `json:"flaky_count"`
	RetriedCount   int `json:"retried_count"`
	NewFailedCount int `json:"new_failed_count"`
	NewPassedCount int `json:"new_passed_count"`
}

type summaryFailure struct {
	TestName   string `json:"test_name"`
	FullName   string `json:"full_name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	NewFailed  bool   `json:"new_failed"`
	Flaky      bool   `json:"flaky"`
}

type summaryTrendDelta struct {
	PreviousBuildNumber int   `json:"previous_build_number"`
	PassedDelta         int   `json:"passed_delta"`
	FailedDelta         int   `json:"failed_delta"`
	BrokenDelta         int   `json:"broken_delta"`
	SkippedDelta        int   `json:"skipped_delta"`
	TotalDelta          int   `json:"total_delta"`
	DurationDeltaMs     int64 `json:"duration_delta_ms"`
}

// derefInt64 returns the value pointed to by p, or 0 if p is nil.
func derefInt64(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}

// derefStr returns the value pointed to by p, or "" if p is nil.
func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

// pct computes (count/total)*100, returning 0 when total is 0.
func pct(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

// GetReportSummary godoc
// @Summary      Get report summary with statistics, top failures, and trend
// @Description  Returns a JSON summary of a build including statistics, quality metrics,
// @Description  top failures, and trend deltas compared to the previous build.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Build order number or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/summary [get]
func (h *ReportHandler) GetReportSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate project_id.
	projectID, _, _, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	// Resolve build: "latest" or numeric build_number.
	reportID := r.PathValue("report_id")
	if reportID == "" {
		reportID = "latest"
	}
	if err := validateReportID(reportID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var err error
	var build store.Build

	if reportID == "latest" {
		build, err = h.buildStore.GetLatestBuild(ctx, projectID)
	} else {
		buildNumber, parseErr := strconv.Atoi(reportID)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "report_id must be a number or 'latest'")
			return
		}
		build, err = h.buildStore.GetBuildByNumber(ctx, projectID, buildNumber)
	}

	if err != nil {
		if errors.Is(err, store.ErrBuildNotFound) {
			writeError(w, http.StatusNotFound, "build not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build metadata.
	buildMeta := summaryBuildMeta{
		BuildID:     build.ID,
		ProjectID:   projectID,
		BuildNumber: build.BuildNumber,
		CreatedAt:   build.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		IsLatest:    build.IsLatest,
		CIProvider:  derefStr(build.CIProvider),
		CIBuildURL:  derefStr(build.CIBuildURL),
		CIBranch:    derefStr(build.CIBranch),
		CICommitSHA: derefStr(build.CICommitSHA),
	}

	// Statistics with percentages.
	passed := derefInt(build.StatPassed)
	failed := derefInt(build.StatFailed)
	broken := derefInt(build.StatBroken)
	skipped := derefInt(build.StatSkipped)
	unknown := derefInt(build.StatUnknown)
	total := derefInt(build.StatTotal)

	stats := summaryStatistics{
		Passed:     passed,
		Failed:     failed,
		Broken:     broken,
		Skipped:    skipped,
		Unknown:    unknown,
		Total:      total,
		PassedPct:  pct(passed, total),
		FailedPct:  pct(failed, total),
		BrokenPct:  pct(broken, total),
		SkippedPct: pct(skipped, total),
		UnknownPct: pct(unknown, total),
	}

	timing := summaryTiming{DurationMs: derefInt64(build.DurationMs)}

	quality := summaryQuality{
		FlakyCount:     derefInt(build.FlakyCount),
		RetriedCount:   derefInt(build.RetriedCount),
		NewFailedCount: derefInt(build.NewFailedCount),
		NewPassedCount: derefInt(build.NewPassedCount),
	}

	// Top failures from test_results.
	var topFailures []summaryFailure
	if h.testResultStore != nil {
		results, err := h.testResultStore.ListFailedByBuild(ctx, projectID, build.ID, topFailuresLimit)
		if err == nil {
			for i := range results {
				topFailures = append(topFailures, summaryFailure{
					TestName:   results[i].TestName,
					FullName:   results[i].FullName,
					Status:     results[i].Status,
					DurationMs: results[i].DurationMs,
					NewFailed:  results[i].NewFailed,
					Flaky:      results[i].Flaky,
				})
			}
		}
	}
	if topFailures == nil {
		topFailures = []summaryFailure{}
	}

	// Trend delta vs previous build.
	var trend *summaryTrendDelta
	prev, err := h.buildStore.GetPreviousBuild(ctx, projectID, build.BuildNumber)
	if err == nil {
		trend = &summaryTrendDelta{
			PreviousBuildNumber: prev.BuildNumber,
			PassedDelta:         passed - derefInt(prev.StatPassed),
			FailedDelta:         failed - derefInt(prev.StatFailed),
			BrokenDelta:         broken - derefInt(prev.StatBroken),
			SkippedDelta:        skipped - derefInt(prev.StatSkipped),
			TotalDelta:          total - derefInt(prev.StatTotal),
			DurationDeltaMs:     derefInt64(build.DurationMs) - derefInt64(prev.DurationMs),
		}
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"build":        buildMeta,
		"statistics":   stats,
		"timing":       timing,
		"quality":      quality,
		"top_failures": topFailures,
		"trend":        trend,
	}, "Report summary successfully obtained")
}
