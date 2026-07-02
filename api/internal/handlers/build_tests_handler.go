package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const (
	defaultBuildTestsLimit = 50
	maxBuildTestsLimit     = 200
)

// BuildTestsHandler serves per-build test result listings (currently
// failed/broken tests) enriched with known-issue and stability flags.
type BuildTestsHandler struct {
	testResultStore store.TestResultReader
	knownIssueStore store.KnownIssueStorer
	projectStore    store.ProjectReader
	logger          *zap.Logger
}

// NewBuildTestsHandler creates a BuildTestsHandler.
func NewBuildTestsHandler(trs store.TestResultReader, kis store.KnownIssueStorer, ps store.ProjectReader, logger *zap.Logger) *BuildTestsHandler {
	return &BuildTestsHandler{
		testResultStore: trs,
		knownIssueStore: kis,
		projectStore:    ps,
		logger:          logger,
	}
}

// buildTestResp is the JSON representation of a single test result returned
// by ListBuildTests.
type buildTestResp struct {
	TestName     string `json:"test_name"`
	FullName     string `json:"full_name"`
	Status       string `json:"status"`
	DurationMs   int64  `json:"duration_ms"`
	HistoryID    string `json:"history_id"`
	Flaky        bool   `json:"flaky"`
	Retries      int    `json:"retries"`
	NewFailed    bool   `json:"new_failed"`
	Known        bool   `json:"known"`
	ErrorMessage string `json:"error_message"`
}

// ListBuildTests returns the failed (and broken) tests for a single build,
// each flagged with whether it matches an active known issue.
//
//	@Summary      List build tests
//	@Description  Returns failed/broken tests for a build, flagged as known, flaky, retried, or newly-failed.
//	@Tags         builds
//	@Produce      json
//	@Param        project_id  path   string  true   "Project ID"
//	@Param        build_id    path   string  true   "Build ID (primary key)"
//	@Param        status      query  string  false  "Test status filter"  default(failed)
//	@Param        limit       query  int     false  "Max results"          default(50)
//	@Success      200  {object}  map[string]any
//	@Failure      400  {object}  map[string]any
//	@Failure      500  {object}  map[string]any
//	@Router       /projects/{project_id}/builds/{build_id}/tests [get]
func (h *BuildTestsHandler) ListBuildTests(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	buildIDStr := r.PathValue("build_id")
	buildID, err := strconv.ParseInt(buildIDStr, 10, 64)
	if err != nil || buildID < 1 {
		writeError(w, http.StatusBadRequest, "invalid build_id")
		return
	}

	q := r.URL.Query()

	status := q.Get("status")
	if status == "" {
		status = "failed"
	}
	if status != "failed" {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	limit := defaultBuildTestsLimit
	if limitStr := q.Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	if limit > maxBuildTestsLimit {
		limit = maxBuildTestsLimit
	}

	ctx := r.Context()

	results, err := h.testResultStore.ListFailedByBuild(ctx, projectID, buildID, limit)
	if err != nil {
		h.logger.Error("list failed by build", zap.Int64("project_id", projectID), zap.Int64("build_id", buildID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing build tests")
		return
	}

	// Only consult the known-issue store when there is something to check
	// against — this is a hot path hit on every build-detail page load.
	var knownMap map[string]bool
	if len(results) > 0 {
		knownIssues, err := h.knownIssueStore.List(ctx, projectID, true)
		if err != nil {
			h.logger.Error("list known issues", zap.Int64("project_id", projectID), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing known issues")
			return
		}
		knownMap = make(map[string]bool, len(knownIssues))
		for i := range knownIssues {
			knownMap[knownIssues[i].TestName] = true
		}
	}

	data := make([]buildTestResp, 0, len(results))
	for i := range results {
		tr := &results[i]
		data = append(data, buildTestResp{
			TestName:     tr.TestName,
			FullName:     tr.FullName,
			Status:       tr.Status,
			DurationMs:   tr.DurationMs,
			HistoryID:    tr.HistoryID,
			Flaky:        tr.Flaky,
			Retries:      tr.Retries,
			NewFailed:    tr.NewFailed,
			Known:        knownMap[tr.TestName],
			ErrorMessage: firstErrorLine(tr.StatusMessage),
		})
	}

	writeSuccess(w, http.StatusOK, data, "build tests retrieved")
}

// firstErrorLine returns the first line of msg (splitting on '\n' and
// trimming a trailing '\r' from CRLF input), capped at 300 runes.
func firstErrorLine(msg string) string {
	line := msg
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSuffix(line, "\r")

	const maxRunes = 300
	runes := []rune(line)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}
