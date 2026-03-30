package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const (
	// multiTimelineMaxLimit is the maximum number of builds the multi-build
	// timeline endpoint will return in a single request.
	multiTimelineMaxLimit = 10

	// multiTimelineDefaultLimit is used when no limit parameter is provided.
	multiTimelineDefaultLimit = 1
)

// multiTimelineBuild holds the per-build data in the multi-build timeline response.
type multiTimelineBuild struct {
	BuildOrder int                `json:"build_order"`
	CreatedAt  string             `json:"created_at"`
	TestCases  []timelineTestCase `json:"test_cases"`
	Summary    timelineSummary    `json:"summary"`
}

// ProjectTimelineHandler handles HTTP requests for multi-build project timelines.
type ProjectTimelineHandler struct {
	buildStore      store.BuildStorer
	testResultStore store.TestResultStorer
	branchStore     store.BranchStorer
	projectsDir     string
}

// NewProjectTimelineHandler creates and returns a new ProjectTimelineHandler.
func NewProjectTimelineHandler(bs store.BuildStorer, trs store.TestResultStorer, brs store.BranchStorer, projectsDir string) *ProjectTimelineHandler {
	return &ProjectTimelineHandler{
		buildStore:      bs,
		testResultStore: trs,
		branchStore:     brs,
		projectsDir:     projectsDir,
	}
}

// GetProjectTimeline godoc
// @Summary      Get multi-build test execution timeline
// @Description  Returns test execution timeline data across one or more builds, with optional branch and date range filters.
// @Tags         timeline
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        branch      query  string  false  "Branch name filter"
// @Param        from        query  string  false  "Start date (YYYY-MM-DD, inclusive)"
// @Param        to          query  string  false  "End date (YYYY-MM-DD, inclusive end-of-day)"
// @Param        limit       query  int     false  "Max builds (1-10, default 1)"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/timeline [get]
func (h *ProjectTimelineHandler) GetProjectTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	q := r.URL.Query()

	// Parse and clamp limit.
	limit := multiTimelineDefaultLimit
	if ls := q.Get("limit"); ls != "" {
		if v, err := strconv.Atoi(ls); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > multiTimelineMaxLimit {
		limit = multiTimelineMaxLimit
	}

	// Resolve optional branch filter.
	var branchID *int64
	if branchName := q.Get("branch"); branchName != "" && h.branchStore != nil {
		if br, err := h.branchStore.GetByName(ctx, projectID, branchName); err == nil {
			branchID = &br.ID
		}
	}

	// Parse optional date range.
	fromStr := q.Get("from")
	toStr := q.Get("to")
	hasDateRange := fromStr != "" || toStr != ""

	var fromTime, toTime time.Time
	if hasDateRange {
		if fromStr != "" {
			parsed, err := time.Parse("2006-01-02", fromStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'from' date format, expected YYYY-MM-DD")
				return
			}
			fromTime = parsed.UTC()
		}
		if toStr != "" {
			parsed, err := time.Parse("2006-01-02", toStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'to' date format, expected YYYY-MM-DD")
				return
			}
			// End-of-day inclusive: advance to start of next day.
			toTime = parsed.AddDate(0, 0, 1).UTC()
		} else {
			// No "to" — use current time.
			toTime = time.Now().UTC()
		}
		if fromStr == "" {
			// No "from" — use epoch.
			fromTime = time.Time{}
		}
	}

	// Fetch builds.
	var totalBuilds int
	var builds []struct {
		ID         int64
		BuildOrder int
		CreatedAt  time.Time
	}

	if hasDateRange && h.buildStore != nil {
		dbBuilds, total, err := h.buildStore.ListBuildsInRange(ctx, projectID, branchID, fromTime, toTime, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list builds in range")
			return
		}
		totalBuilds = total
		for i := range dbBuilds {
			builds = append(builds, struct {
				ID         int64
				BuildOrder int
				CreatedAt  time.Time
			}{ID: dbBuilds[i].ID, BuildOrder: dbBuilds[i].BuildOrder, CreatedAt: dbBuilds[i].CreatedAt})
		}
	} else if h.buildStore != nil {
		dbBuilds, total, err := h.buildStore.ListBuildsPaginatedBranch(ctx, projectID, 1, limit, branchID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list builds")
			return
		}
		totalBuilds = total
		for i := range dbBuilds {
			builds = append(builds, struct {
				ID         int64
				BuildOrder int
				CreatedAt  time.Time
			}{ID: dbBuilds[i].ID, BuildOrder: dbBuilds[i].BuildOrder, CreatedAt: dbBuilds[i].CreatedAt})
		}
	}

	// For each build, resolve the internal build ID if needed (ID may be 0 from stores
	// that don't populate it). Collect all build IDs for the multi-timeline query.
	var buildIDs []int64
	buildIDToOrder := make(map[int64]int)
	buildIDToCreatedAt := make(map[int64]time.Time)

	for i := range builds {
		bid := builds[i].ID
		// If the build ID is 0, resolve it via GetBuildID.
		if bid == 0 && h.testResultStore != nil {
			if resolved, err := h.testResultStore.GetBuildID(ctx, projectID, builds[i].BuildOrder); err == nil {
				bid = resolved
				builds[i].ID = bid
			}
		}
		if bid > 0 {
			buildIDs = append(buildIDs, bid)
			buildIDToOrder[bid] = builds[i].BuildOrder
			buildIDToCreatedAt[bid] = builds[i].CreatedAt
		}
	}

	// Query timeline rows for all builds in one call.
	var responseBuilds []multiTimelineBuild
	var globalMinStart, globalMaxStop int64

	if len(buildIDs) > 0 && h.testResultStore != nil {
		rows, err := h.testResultStore.ListTimelineMulti(ctx, projectID, buildIDs, timelineMaxItems)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to query timeline data")
			return
		}

		// Group rows by build ID.
		type buildBucket struct {
			order     int
			createdAt time.Time
			cases     []timelineTestCase
		}
		buckets := make(map[int64]*buildBucket)
		for _, bid := range buildIDs {
			buckets[bid] = &buildBucket{
				order:     buildIDToOrder[bid],
				createdAt: buildIDToCreatedAt[bid],
			}
		}

		for _, row := range rows {
			dur := row.StopMs - row.StartMs
			tc := timelineTestCase{
				Name:     row.TestName,
				FullName: row.FullName,
				Status:   row.Status,
				Start:    row.StartMs,
				Stop:     row.StopMs,
				Duration: dur,
				Thread:   row.Thread,
				Host:     row.Host,
			}
			if b, ok := buckets[row.BuildID]; ok {
				b.cases = append(b.cases, tc)
			}

			if globalMinStart == 0 || row.StartMs < globalMinStart {
				globalMinStart = row.StartMs
			}
			if row.StopMs > globalMaxStop {
				globalMaxStop = row.StopMs
			}
		}

		// Build response in build-order (use buildIDs order which is descending from DB,
		// but we want ascending for the response).
		for i := len(buildIDs) - 1; i >= 0; i-- {
			bid := buildIDs[i]
			bucket := buckets[bid]
			cases := bucket.cases
			if cases == nil {
				cases = []timelineTestCase{}
			}

			total := len(cases)
			truncated := false
			if total > timelineMaxItems {
				cases = cases[:timelineMaxItems]
				truncated = true
			}

			var minStart, maxStop, totalDuration int64
			for j, tc := range cases {
				if j == 0 || tc.Start < minStart {
					minStart = tc.Start
				}
				if tc.Stop > maxStop {
					maxStop = tc.Stop
				}
				totalDuration += tc.Duration
			}

			responseBuilds = append(responseBuilds, multiTimelineBuild{
				BuildOrder: bucket.order,
				CreatedAt:  bucket.createdAt.UTC().Format(time.RFC3339),
				TestCases:  cases,
				Summary: timelineSummary{
					Total:         total,
					MinStart:      minStart,
					MaxStop:       maxStop,
					TotalDuration: totalDuration,
					Truncated:     truncated,
				},
			})
		}
	}

	if responseBuilds == nil {
		responseBuilds = []multiTimelineBuild{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"builds":                responseBuilds,
			"total_builds_in_range": totalBuilds,
			"builds_returned":       len(responseBuilds),
			"global_min_start":      globalMinStart,
			"global_max_stop":       globalMaxStop,
		},
		"metadata": map[string]string{"message": "Timeline successfully obtained"},
	})
}
