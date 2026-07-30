package handlers

import (
	"cmp"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PipelineHandler serves cross-project pipeline run queries for parent projects.
type PipelineHandler struct {
	pipelineStore   store.PipelineStorer
	projectStore    store.ProjectHierarchyReader
	knownIssueStore store.KnownIssueStorer
	projectsDir     string
	logger          *zap.Logger
}

// NewPipelineHandler creates a PipelineHandler.
func NewPipelineHandler(ps store.PipelineStorer, projStore store.ProjectHierarchyReader, kis store.KnownIssueStorer, projectsDir string, logger *zap.Logger) *PipelineHandler {
	return &PipelineHandler{
		pipelineStore:   ps,
		projectStore:    projStore,
		knownIssueStore: kis,
		projectsDir:     projectsDir,
		logger:          logger,
	}
}

const (
	defaultPipelinePerPage  = 10
	defaultRunFailuresLimit = 500
	maxRunFailuresLimit     = 2000
)

// GetPipelineRuns returns paginated pipeline runs for a parent project,
// grouped by commit SHA with per-suite and aggregate statistics.
//
//	@Summary      List pipeline runs
//	@Description  Returns child-suite builds grouped by commit SHA for a parent project.
//	@Tags         pipeline
//	@Produce      json
//	@Param        project_id  path   string  true   "Parent project ID"
//	@Param        page        query  int     false  "Page number"           default(1)
//	@Param        per_page    query  int     false  "Results per page"      default(10)
//	@Param        branch      query  string  false  "Filter by branch name"
//	@Success      200  {object}  map[string]any
//	@Failure      400  {object}  map[string]any
//	@Failure      500  {object}  map[string]any
//	@Router       /projects/{project_id}/pipeline-runs [get]
func (h *PipelineHandler) GetPipelineRuns(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	ctx := r.Context()

	hasChildren, err := h.projectStore.HasChildren(ctx, projectID)
	if err != nil {
		h.logger.Error("check has children", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error checking project")
		return
	}
	if !hasChildren {
		writeError(w, http.StatusBadRequest, "project is not a parent project")
		return
	}

	pp := parsePagination(r)
	if r.URL.Query().Get("per_page") == "" {
		pp.PerPage = defaultPipelinePerPage
	}
	branch := r.URL.Query().Get("branch")

	rows, total, err := h.pipelineStore.ListPipelineRuns(ctx, projectID, branch, pp.Page, pp.PerPage)
	if err != nil {
		h.logger.Error("list pipeline runs", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing pipeline runs")
		return
	}

	runs := groupPipelineRuns(rows)
	if runs == nil {
		runs = []pipelineRunResp{}
	}

	writePagedSuccess(w, runs, "pipeline runs retrieved", newPaginationMeta(pp.Page, pp.PerPage, total))
}

// GetAllPipelineRuns returns paginated pipeline runs across every parent
// project's child suites, grouped by commit SHA within each parent group with
// per-suite and aggregate statistics. Unlike GetPipelineRuns, this endpoint is
// not scoped to a single parent project.
//
//	@Summary      List all pipeline runs
//	@Description  Returns child-suite builds across every parent project, grouped by commit SHA within each parent's group.
//	@Tags         pipeline
//	@Produce      json
//	@Param        page        query  int     false  "Page number"                            default(1)
//	@Param        per_page    query  int     false  "Results per page"                       default(10)
//	@Param        branch      query  string  false  "Filter by branch name"
//	@Param        group_id    query  []int   false  "Filter by parent (group) project ID"    collectionFormat(multi)
//	@Success      200  {object}  map[string]any
//	@Failure      400  {object}  map[string]any
//	@Failure      500  {object}  map[string]any
//	@Router       /pipeline-runs [get]
func (h *PipelineHandler) GetAllPipelineRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pp := parsePagination(r)
	if r.URL.Query().Get("per_page") == "" {
		pp.PerPage = defaultPipelinePerPage
	}
	branch := r.URL.Query().Get("branch")

	var groupIDs []int64
	for _, raw := range r.URL.Query()["group_id"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid group_id")
			return
		}
		groupIDs = append(groupIDs, id)
	}
	if groupIDs == nil {
		groupIDs = []int64{}
	}

	rows, total, err := h.pipelineStore.ListAllPipelineRuns(ctx, branch, groupIDs, pp.Page, pp.PerPage)
	if err != nil {
		h.logger.Error("list all pipeline runs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing pipeline runs")
		return
	}

	runs := groupPipelineRuns(rows)
	if runs == nil {
		runs = []pipelineRunResp{}
	}

	writePagedSuccess(w, runs, "pipeline runs retrieved", newPaginationMeta(pp.Page, pp.PerPage, total))
}

// runFailureResp is one failing test of a run, carrying the suite and build it
// came from so the client does not have to correlate it back.
type runFailureResp struct {
	ProjectID    int64  `json:"project_id"`
	Slug         string `json:"slug"`
	DisplayName  string `json:"display_name,omitempty"`
	BuildID      int64  `json:"build_id"`
	BuildNumber  int    `json:"build_number"`
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

// GetRunFailures returns every failing test in a single pipeline run.
//
// One run spans all the builds its child suites uploaded under the same
// pipeline ID, and a sharded suite contributes one build per shard, so this
// replaces what would otherwise be a per-build request fan-out from the client.
//
//	@Summary      List run failures
//	@Description  Returns the failed and broken tests of every build in one pipeline run, flagged as known, flaky, retried, or newly-failed.
//	@Tags         pipeline
//	@Produce      json
//	@Param        project_id  path   string  true   "Parent (group) project ID"
//	@Param        run_key     path   string  true   "CI pipeline ID, or commit SHA when the pipeline ID is absent"
//	@Param        limit       query  int     false  "Max results"  default(500)
//	@Success      200  {object}  map[string]any
//	@Failure      400  {object}  map[string]any
//	@Failure      500  {object}  map[string]any
//	@Router       /projects/{project_id}/pipeline-runs/{run_key}/failures [get]
func (h *PipelineHandler) GetRunFailures(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	runKey := r.PathValue("run_key")
	if runKey == "" {
		writeError(w, http.StatusBadRequest, "invalid run_key")
		return
	}

	limit := defaultRunFailuresLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxRunFailuresLimit {
		limit = maxRunFailuresLimit
	}

	ctx := r.Context()

	// Fetch one extra row so a full page can be reported as truncated rather
	// than silently looking complete.
	rows, err := h.pipelineStore.ListRunFailures(ctx, projectID, runKey, limit+1)
	if err != nil {
		h.logger.Error("list run failures", zap.Int64("project_id", projectID), zap.String("run_key", runKey), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing run failures")
		return
	}

	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}

	// Known issues are per child project, so consult the store once per
	// distinct suite present in the results rather than once per row.
	knownByProject := map[int64]map[string]bool{}
	for i := range rows {
		pid := rows[i].ProjectID
		if _, done := knownByProject[pid]; done {
			continue
		}
		knownIssues, err := h.knownIssueStore.List(ctx, pid, true)
		if err != nil {
			h.logger.Error("list known issues", zap.Int64("project_id", pid), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing known issues")
			return
		}
		names := make(map[string]bool, len(knownIssues))
		for j := range knownIssues {
			names[knownIssues[j].TestName] = true
		}
		knownByProject[pid] = names
	}

	data := make([]runFailureResp, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		data = append(data, runFailureResp{
			ProjectID:    row.ProjectID,
			Slug:         row.Slug,
			DisplayName:  row.DisplayName,
			BuildID:      row.BuildID,
			BuildNumber:  row.BuildNumber,
			TestName:     row.TestName,
			FullName:     row.FullName,
			Status:       row.Status,
			DurationMs:   row.DurationMs,
			HistoryID:    row.HistoryID,
			Flaky:        row.Flaky,
			Retries:      row.Retries,
			NewFailed:    row.NewFailed,
			Known:        knownByProject[row.ProjectID][row.TestName],
			ErrorMessage: firstErrorLine(row.StatusMessage),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"metadata": map[string]any{
			"message":   "run failures retrieved",
			"truncated": truncated,
		},
	})
}

// Response types — private to this handler.

type pipelineRunResp struct {
	PipelineID     string              `json:"pipeline_id,omitempty"`
	PipelineURL    string              `json:"pipeline_url,omitempty"`
	CommitSHA      string              `json:"commit_sha"`
	Branch         string              `json:"branch"`
	CIBuildURL     string              `json:"ci_build_url,omitempty"`
	Timestamp      string              `json:"timestamp"`
	GroupProjectID int64               `json:"group_project_id,omitempty"`
	GroupSlug      string              `json:"group_slug,omitempty"`
	Suites         []pipelineSuiteResp `json:"suites"`
	Aggregate      pipelineAggResp     `json:"aggregate"`
}

type pipelineSuiteResp struct {
	ProjectID   int64   `json:"project_id"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name,omitempty"`
	BuildNumber int     `json:"build_number"`
	BuildID     int64   `json:"build_id"`
	PassRate    float64 `json:"pass_rate"`
	Total       int     `json:"total"`
	Failed      int     `json:"failed"`
	DurationMs  int64   `json:"duration_ms"`
	Status      string  `json:"status"`
	// Builds lists every build that contributed to this suite, oldest first.
	// More than one means the suite was sharded across parallel CI jobs that
	// each uploaded their own results. BuildID/BuildNumber above point at the
	// newest of these, so single-build suites keep their existing link.
	Builds []pipelineSuiteBuildResp `json:"builds"`
}

// pipelineSuiteBuildResp identifies one contributing build (one shard) of a suite.
type pipelineSuiteBuildResp struct {
	BuildID     int64 `json:"build_id"`
	BuildNumber int   `json:"build_number"`
}

type pipelineAggResp struct {
	SuitesPassed    int     `json:"suites_passed"`
	SuitesTotal     int     `json:"suites_total"`
	TestsPassed     int     `json:"tests_passed"`
	TestsTotal      int     `json:"tests_total"`
	PassRate        float64 `json:"pass_rate"`
	TotalDurationMs int64   `json:"total_duration_ms"`
}

// suiteAccum merges every build a single child project contributed to one run.
// CI shards a suite across parallel jobs and each shard uploads its own build,
// so one project can appear several times within the same pipeline; they are
// one logical suite and their counters sum.
type suiteAccum struct {
	projectID   int64
	slug        string
	displayName string
	total       int
	failed      int
	passed      int
	skipped     int
	durationMs  int64
	buildID     int64 // newest contributing build
	buildNumber int
	hasBuild    bool
	builds      []pipelineSuiteBuildResp
}

// groupPipelineRuns groups flat store rows by pipeline ID (if set) or commit SHA and computes aggregates.
func groupPipelineRuns(rows []store.PipelineRunRow) []pipelineRunResp {
	if len(rows) == 0 {
		return nil
	}

	type runAccum struct {
		resp       pipelineRunResp
		maxTS      time.Time
		suites     map[int64]*suiteAccum
		suiteOrder []int64 // project IDs in first-seen order
	}

	order := []string{}
	byKey := map[string]*runAccum{}

	for i := range rows {
		r := &rows[i]

		keyOf := r.CommitSHA
		if r.PipelineID != "" {
			keyOf = r.PipelineID
		}
		// Prefix with GroupProjectID so the same pipeline ID/commit SHA under two
		// different parent groups (ListAllPipelineRuns) remains two separate runs.
		// Per-parent rows (ListPipelineRuns) leave GroupProjectID at its zero
		// value, so the prefix is constant within a single request and grouping
		// behavior is unchanged.
		groupKey := fmt.Sprintf("%d:%s", r.GroupProjectID, keyOf)

		acc, exists := byKey[groupKey]
		if !exists {
			acc = &runAccum{
				resp: pipelineRunResp{
					PipelineID:     r.PipelineID,
					PipelineURL:    r.PipelineURL,
					CommitSHA:      r.CommitSHA,
					Branch:         r.Branch,
					CIBuildURL:     r.CIBuildURL,
					GroupProjectID: r.GroupProjectID,
					GroupSlug:      r.GroupSlug,
				},
				suites: map[int64]*suiteAccum{},
			}
			byKey[groupKey] = acc
			order = append(order, groupKey)
		}

		if r.CreatedAt.After(acc.maxTS) {
			acc.maxTS = r.CreatedAt
		}
		if r.CIBuildURL != "" && acc.resp.CIBuildURL == "" {
			acc.resp.CIBuildURL = r.CIBuildURL
		}

		sa, seen := acc.suites[r.ProjectID]
		if !seen {
			sa = &suiteAccum{projectID: r.ProjectID, slug: r.Slug, displayName: r.DisplayName}
			acc.suites[r.ProjectID] = sa
			acc.suiteOrder = append(acc.suiteOrder, r.ProjectID)
		}

		sa.total += derefInt(r.StatTotal)
		sa.failed += derefInt(r.StatFailed) + derefInt(r.StatBroken)
		sa.passed += derefInt(r.StatPassed)
		sa.skipped += derefInt(r.StatSkipped)
		sa.durationMs += derefInt64(r.DurationMs)
		sa.builds = append(sa.builds, pipelineSuiteBuildResp{BuildID: r.BuildID, BuildNumber: r.BuildNumber})
		if !sa.hasBuild || r.BuildNumber > sa.buildNumber {
			sa.buildNumber = r.BuildNumber
			sa.buildID = r.BuildID
			sa.hasBuild = true
		}
	}

	result := make([]pipelineRunResp, 0, len(order))
	for _, key := range order {
		acc := byKey[key]
		acc.resp.Timestamp = acc.maxTS.UTC().Format(time.RFC3339)

		var (
			agg       pipelineAggResp
			effPassed int // sum of stat_passed across suites (for aggregate pass rate)
			effDenom  int // sum of (stat_total - stat_skipped) across suites
		)

		for _, projectID := range acc.suiteOrder {
			sa := acc.suites[projectID]

			// Shards arrive newest-first; present them oldest-first so the
			// build links read in the order the shards were numbered.
			slices.SortFunc(sa.builds, func(a, b pipelineSuiteBuildResp) int {
				return cmp.Compare(a.BuildNumber, b.BuildNumber)
			})

			denom := sa.total - sa.skipped
			passRate := 0.0
			if denom > 0 {
				passRate = math.Round(float64(sa.passed)/float64(denom)*1000) / 10
			}

			// A merged suite only reaches 100% when every shard was clean, so
			// one failing shard is enough to keep the whole suite out of the
			// passed bucket.
			status := "failed"
			if passRate >= 100 {
				status = "passed"
			} else if passRate >= 70 {
				status = "degraded"
			}

			acc.resp.Suites = append(acc.resp.Suites, pipelineSuiteResp{
				ProjectID:   sa.projectID,
				Slug:        sa.slug,
				DisplayName: sa.displayName,
				BuildNumber: sa.buildNumber,
				BuildID:     sa.buildID,
				PassRate:    passRate,
				Total:       sa.total,
				Failed:      sa.failed,
				DurationMs:  sa.durationMs,
				Status:      status,
				Builds:      sa.builds,
			})

			effPassed += sa.passed
			effDenom += denom

			agg.TestsTotal += sa.total
			agg.TotalDurationMs += sa.durationMs
			if status == "passed" {
				agg.SuitesPassed++
			}
		}

		agg.SuitesTotal = len(acc.resp.Suites)
		// Count actually-passed tests rather than total-minus-failed, so the
		// tests fraction agrees with the pass rate rendered beside it instead
		// of quietly counting skipped tests as passes.
		agg.TestsPassed = effPassed
		if effDenom > 0 {
			agg.PassRate = math.Round(float64(effPassed)/float64(effDenom)*1000) / 10
		}
		acc.resp.Aggregate = agg
		result = append(result, acc.resp)
	}

	return result
}
