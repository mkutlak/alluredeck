package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

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

// stabilityRawEntry is used to parse test-result JSON files.
type stabilityRawEntry struct {
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	Status        string `json:"status"`
	NewFailed     bool   `json:"newFailed"`
	NewPassed     bool   `json:"newPassed"`
	RetriesCount  int    `json:"retriesCount"`
	StatusDetails *struct {
		Flaky bool `json:"flaky"`
	} `json:"statusDetails"`
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

	projectID, _, storageKey, ok := h.lookupProjectSlug(w, r)
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

	if h.testResultStore != nil {
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
		return
	}

	relBase := "reports/" + reportID + "/data/test-results"
	entries, err := h.store.ReadDir(ctx, storageKey, relBase)

	var flakyTests []stabilityTestEntry
	var newFailed []stabilityTestEntry
	var newPassed []stabilityTestEntry
	summary := stabilitySummary{}

	if err == nil {
		for _, entry := range entries {
			if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
				continue
			}
			data, err := h.store.ReadFile(ctx, storageKey, relBase+"/"+entry.Name)
			if err != nil {
				continue
			}
			var re stabilityRawEntry
			if json.Unmarshal(data, &re) != nil {
				continue
			}
			summary.Total++

			isFlaky := re.StatusDetails != nil && re.StatusDetails.Flaky
			if isFlaky {
				summary.FlakyCount++
				flakyTests = append(flakyTests, stabilityTestEntry{
					Name:         re.Name,
					FullName:     re.FullName,
					Status:       re.Status,
					RetriesCount: re.RetriesCount,
				})
			}
			if re.RetriesCount > 0 {
				summary.RetriedCount++
			}
			if re.NewFailed {
				summary.NewFailedCount++
				newFailed = append(newFailed, stabilityTestEntry{
					Name:     re.Name,
					FullName: re.FullName,
					Status:   re.Status,
				})
			}
			if re.NewPassed {
				summary.NewPassedCount++
				newPassed = append(newPassed, stabilityTestEntry{
					Name:     re.Name,
					FullName: re.FullName,
					Status:   re.Status,
				})
			}
		}
	}

	// Ensure arrays are never null in JSON.
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
