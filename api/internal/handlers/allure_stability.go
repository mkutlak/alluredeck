package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// stabilityTestEntry represents one test in the stability response.
type stabilityTestEntry struct {
	Name                string `json:"name"`
	FullName            string `json:"full_name"`
	Status              string `json:"status"`
	RetriesCount        int    `json:"retries_count"`
	RetriesStatusChange bool   `json:"retries_status_change"`
}

// stabilitySummary aggregates stability metrics for the response.
type stabilitySummary struct {
	FlakyCount     int `json:"flaky_count"`
	RetriedCount   int `json:"retried_count"`
	NewFailedCount int `json:"new_failed_count"`
	NewPassedCount int `json:"new_passed_count"`
	Total          int `json:"total"`
}

// GetReportStability godoc
// @Summary      Get stability data for a report
// @Description  Returns flaky tests, regressions, and fixes for a given report.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/stability [get]
func (h *ReportHandler) GetReportStability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, _, _, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	reportID := r.PathValue("report_id")
	if reportID == "" {
		reportID = "latest"
	}
	if err := validateReportID(reportID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.testResultStore == nil {
		// When testResultStore is not available, return empty stability data.
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"flaky_tests": []stabilityTestEntry{},
				"new_failed":  []stabilityTestEntry{},
				"new_passed":  []stabilityTestEntry{},
				"summary":     stabilitySummary{},
			},
			"metadata": map[string]string{"message": "Stability data successfully obtained"},
		})
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

	results, err := h.testResultStore.ListStabilityByBuild(ctx, projectID, build.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	summary := stabilitySummary{
		FlakyCount:     derefInt(build.FlakyCount),
		RetriedCount:   derefInt(build.RetriedCount),
		NewFailedCount: derefInt(build.NewFailedCount),
		NewPassedCount: derefInt(build.NewPassedCount),
		Total:          derefInt(build.StatTotal),
	}

	var flakyTests []stabilityTestEntry
	var newFailed []stabilityTestEntry
	var newPassed []stabilityTestEntry

	for i := range results {
		if results[i].Flaky {
			flakyTests = append(flakyTests, stabilityTestEntry{
				Name:         results[i].TestName,
				FullName:     results[i].FullName,
				Status:       results[i].Status,
				RetriesCount: results[i].Retries,
			})
		}
		if results[i].NewFailed {
			newFailed = append(newFailed, stabilityTestEntry{
				Name:     results[i].TestName,
				FullName: results[i].FullName,
				Status:   results[i].Status,
			})
		}
		if results[i].NewPassed {
			newPassed = append(newPassed, stabilityTestEntry{
				Name:     results[i].TestName,
				FullName: results[i].FullName,
				Status:   results[i].Status,
			})
		}
	}

	if flakyTests == nil {
		flakyTests = []stabilityTestEntry{}
	}
	if newFailed == nil {
		newFailed = []stabilityTestEntry{}
	}
	if newPassed == nil {
		newPassed = []stabilityTestEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"flaky_tests": flakyTests,
			"new_failed":  newFailed,
			"new_passed":  newPassed,
			"summary":     summary,
		},
		"metadata": map[string]string{"message": "Stability data successfully obtained"},
	})
}
