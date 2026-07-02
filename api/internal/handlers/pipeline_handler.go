package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PipelineHandler serves cross-project pipeline run queries for parent projects.
type PipelineHandler struct {
	pipelineStore store.PipelineStorer
	projectStore  store.ProjectHierarchyReader
	projectsDir   string
	logger        *zap.Logger
}

// NewPipelineHandler creates a PipelineHandler.
func NewPipelineHandler(ps store.PipelineStorer, projStore store.ProjectHierarchyReader, projectsDir string, logger *zap.Logger) *PipelineHandler {
	return &PipelineHandler{
		pipelineStore: ps,
		projectStore:  projStore,
		projectsDir:   projectsDir,
		logger:        logger,
	}
}

const defaultPipelinePerPage = 10

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
	BuildNumber int     `json:"build_number"`
	BuildID     int64   `json:"build_id"`
	PassRate    float64 `json:"pass_rate"`
	Total       int     `json:"total"`
	Failed      int     `json:"failed"`
	DurationMs  int64   `json:"duration_ms"`
	Status      string  `json:"status"`
}

type pipelineAggResp struct {
	SuitesPassed    int     `json:"suites_passed"`
	SuitesTotal     int     `json:"suites_total"`
	TestsPassed     int     `json:"tests_passed"`
	TestsTotal      int     `json:"tests_total"`
	PassRate        float64 `json:"pass_rate"`
	TotalDurationMs int64   `json:"total_duration_ms"`
}

// groupPipelineRuns groups flat store rows by pipeline ID (if set) or commit SHA and computes aggregates.
func groupPipelineRuns(rows []store.PipelineRunRow) []pipelineRunResp {
	if len(rows) == 0 {
		return nil
	}

	type runAccum struct {
		resp      pipelineRunResp
		maxTS     time.Time
		effPassed int // sum of stat_passed across suites (for aggregate pass rate)
		effDenom  int // sum of (stat_total - stat_skipped) across suites (for aggregate pass rate)
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

		total := derefInt(r.StatTotal)
		failed := derefInt(r.StatFailed) + derefInt(r.StatBroken)
		passed := derefInt(r.StatPassed)
		skipped := derefInt(r.StatSkipped)
		dur := derefInt64(r.DurationMs)

		denom := total - skipped
		passRate := 0.0
		if denom > 0 {
			passRate = math.Round(float64(passed)/float64(denom)*1000) / 10
		}

		status := "failed"
		if passRate >= 100 {
			status = "passed"
		} else if passRate >= 70 {
			status = "degraded"
		}

		acc.effPassed += passed
		acc.effDenom += denom

		acc.resp.Suites = append(acc.resp.Suites, pipelineSuiteResp{
			ProjectID:   r.ProjectID,
			Slug:        r.Slug,
			BuildNumber: r.BuildNumber,
			BuildID:     r.BuildID,
			PassRate:    passRate,
			Total:       total,
			Failed:      failed,
			DurationMs:  dur,
			Status:      status,
		})
	}

	result := make([]pipelineRunResp, 0, len(order))
	for _, key := range order {
		acc := byKey[key]
		acc.resp.Timestamp = acc.maxTS.UTC().Format(time.RFC3339)

		var agg pipelineAggResp
		agg.SuitesTotal = len(acc.resp.Suites)
		for _, s := range acc.resp.Suites {
			agg.TestsPassed += s.Total - s.Failed
			agg.TestsTotal += s.Total
			agg.TotalDurationMs += s.DurationMs
			if s.Status == "passed" {
				agg.SuitesPassed++
			}
		}
		if acc.effDenom > 0 {
			agg.PassRate = math.Round(float64(acc.effPassed)/float64(acc.effDenom)*1000) / 10
		}
		acc.resp.Aggregate = agg
		result = append(result, acc.resp)
	}

	return result
}
