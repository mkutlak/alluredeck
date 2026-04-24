package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
)

const timelineMaxItems = 5000

// timelineTestCase is the JSON representation of a single test case in the timeline.
type timelineTestCase struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Status   string `json:"status"`
	Start    int64  `json:"start"`
	Stop     int64  `json:"stop"`
	Duration int64  `json:"duration"`
	Thread   string `json:"thread"`
	Host     string `json:"host"`
}

// timelineSummary aggregates metrics across the full result set.
type timelineSummary struct {
	Total         int   `json:"total"`
	MinStart      int64 `json:"min_start"`
	MaxStop       int64 `json:"max_stop"`
	TotalDuration int64 `json:"total_duration"`
	Truncated     bool  `json:"truncated"`
}

// GetReportTimeline godoc
// @Summary      Get test execution timeline for a report
// @Description  Returns all test cases with start/stop timestamps for Gantt-chart rendering.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/timeline [get]
func (h *ReportHandler) GetReportTimeline(w http.ResponseWriter, r *http.Request) {
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

	// Resolve "latest" to a numeric build order via the database when possible.
	if reportID == "latest" && h.buildStore != nil && h.testResultStore != nil {
		if latestBuild, err := h.buildStore.GetLatestBuild(ctx, projectID); err == nil {
			reportID = strconv.Itoa(latestBuild.BuildNumber)
		}
	}

	// Database fast path: for numeric report_id, serve from database instead of N+1 S3 reads.
	if buildNumber, err := strconv.Atoi(reportID); err == nil && h.testResultStore != nil {
		if buildID, err := h.testResultStore.GetBuildID(ctx, projectID, buildNumber); err == nil {
			rows, err := h.testResultStore.ListTimeline(ctx, projectID, buildID, timelineMaxItems+1)
			if err == nil {
				total := len(rows)
				truncated := false
				if total > timelineMaxItems {
					rows = rows[:timelineMaxItems]
					truncated = true
				}

				testCases := make([]timelineTestCase, len(rows))
				var minStart, maxStop, totalDuration int64
				for i := range rows {
					dur := rows[i].StopMs - rows[i].StartMs
					testCases[i] = timelineTestCase{
						Name:     rows[i].TestName,
						FullName: rows[i].FullName,
						Status:   rows[i].Status,
						Start:    rows[i].StartMs,
						Stop:     rows[i].StopMs,
						Duration: dur,
						Thread:   rows[i].Thread,
						Host:     rows[i].Host,
					}
					if i == 0 || rows[i].StartMs < minStart {
						minStart = rows[i].StartMs
					}
					if rows[i].StopMs > maxStop {
						maxStop = rows[i].StopMs
					}
					totalDuration += dur
				}

				writeJSON(w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"test_cases": testCases,
						"summary": timelineSummary{
							Total:         total,
							MinStart:      minStart,
							MaxStop:       maxStop,
							TotalDuration: totalDuration,
							Truncated:     truncated,
						},
					},
					"metadata": map[string]string{"message": "Timeline successfully obtained"},
				})
				return
			}
		}
	}

	relBase := "reports/" + reportID + "/data/test-results"
	entries, err := h.store.ReadDir(ctx, storageKey, relBase)

	// rawEntry is used to parse both nested and top-level time formats.
	type rawTime struct {
		Start    int64 `json:"start"`
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	}
	type rawLabel struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	type rawEntry struct {
		Name     string     `json:"name"`
		FullName string     `json:"fullName"`
		Status   string     `json:"status"`
		Start    int64      `json:"start"` // top-level format (raw results)
		Stop     int64      `json:"stop"`  // top-level format (raw results)
		Time     *rawTime   `json:"time"`  // nested format (generated report)
		Labels   []rawLabel `json:"labels"`
	}

	var testCases []timelineTestCase
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir {
				continue
			}
			data, err := h.store.ReadFile(ctx, storageKey, relBase+"/"+entry.Name)
			if err != nil {
				continue
			}
			var re rawEntry
			if json.Unmarshal(data, &re) != nil {
				continue
			}

			var start, stop, duration int64
			if re.Time != nil {
				start = re.Time.Start
				stop = re.Time.Stop
				duration = re.Time.Duration
			} else {
				start = re.Start
				stop = re.Stop
				duration = stop - start
			}

			var thread, host string
			for _, lbl := range re.Labels {
				switch lbl.Name {
				case "thread":
					thread = lbl.Value
				case "host":
					host = lbl.Value
				}
			}

			testCases = append(testCases, timelineTestCase{
				Name:     re.Name,
				FullName: re.FullName,
				Status:   re.Status,
				Start:    start,
				Stop:     stop,
				Duration: duration,
				Thread:   thread,
				Host:     host,
			})
		}
	}

	// Sort ascending by start time.
	sort.Slice(testCases, func(i, j int) bool {
		return testCases[i].Start < testCases[j].Start
	})

	total := len(testCases)
	truncated := false
	if total > timelineMaxItems {
		testCases = testCases[:timelineMaxItems]
		truncated = true
	}

	// Compute summary metrics over the full (pre-truncation) set.
	var minStart, maxStop, totalDuration int64
	// Re-read from the truncated slice for display metrics; report total over all.
	for i, tc := range testCases {
		if i == 0 || tc.Start < minStart {
			minStart = tc.Start
		}
		if tc.Stop > maxStop {
			maxStop = tc.Stop
		}
		totalDuration += tc.Duration
	}

	// Ensure test_cases is never null in JSON.
	if testCases == nil {
		testCases = []timelineTestCase{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"test_cases": testCases,
			"summary": timelineSummary{
				Total:         total,
				MinStart:      minStart,
				MaxStop:       maxStop,
				TotalDuration: totalDuration,
				Truncated:     truncated,
			},
		},
		"metadata": map[string]string{"message": "Timeline successfully obtained"},
	})
}
