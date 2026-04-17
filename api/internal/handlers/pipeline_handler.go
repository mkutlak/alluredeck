package handlers

import (
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
	projectStore  store.ProjectStorer
	projectsDir   string
	logger        *zap.Logger
}

// NewPipelineHandler creates a PipelineHandler.
func NewPipelineHandler(ps store.PipelineStorer, projStore store.ProjectStorer, projectsDir string, logger *zap.Logger) *PipelineHandler {
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

// Response types — private to this handler.

type pipelineRunResp struct {
	CommitSHA  string              `json:"commit_sha"`
	Branch     string              `json:"branch"`
	CIBuildURL string              `json:"ci_build_url,omitempty"`
	Timestamp  string              `json:"timestamp"`
	Suites     []pipelineSuiteResp `json:"suites"`
	Aggregate  pipelineAggResp     `json:"aggregate"`
}

type pipelineSuiteResp struct {
	ProjectID   string  `json:"project_id"`
	Slug        string  `json:"slug"`
	BuildNumber int     `json:"build_number"`
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

// groupPipelineRuns groups flat store rows by commit SHA and computes aggregates.
func groupPipelineRuns(rows []store.PipelineRunRow) []pipelineRunResp {
	if len(rows) == 0 {
		return nil
	}

	type runAccum struct {
		resp  pipelineRunResp
		maxTS time.Time
	}

	order := []string{}
	bysha := map[string]*runAccum{}

	for i := range rows {
		r := &rows[i]
		acc, exists := bysha[r.CommitSHA]
		if !exists {
			acc = &runAccum{
				resp: pipelineRunResp{
					CommitSHA:  r.CommitSHA,
					Branch:     r.Branch,
					CIBuildURL: r.CIBuildURL,
				},
			}
			bysha[r.CommitSHA] = acc
			order = append(order, r.CommitSHA)
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
		dur := derefInt64(r.DurationMs)

		passRate := 0.0
		if total > 0 {
			passRate = math.Round(float64(passed)/float64(total)*1000) / 10
		}

		status := "failed"
		if passRate >= 100 {
			status = "passed"
		} else if passRate >= 70 {
			status = "degraded"
		}

		acc.resp.Suites = append(acc.resp.Suites, pipelineSuiteResp{
			ProjectID:   strconv.FormatInt(r.ProjectID, 10),
			Slug:        r.Slug,
			BuildNumber: r.BuildNumber,
			PassRate:    passRate,
			Total:       total,
			Failed:      failed,
			DurationMs:  dur,
			Status:      status,
		})
	}

	result := make([]pipelineRunResp, 0, len(order))
	for _, sha := range order {
		acc := bysha[sha]
		acc.resp.Timestamp = acc.maxTS.UTC().Format(time.RFC3339)

		// Compute aggregate.
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
		if agg.TestsTotal > 0 {
			agg.PassRate = math.Round(float64(agg.TestsPassed)/float64(agg.TestsTotal)*1000) / 10
		}
		acc.resp.Aggregate = agg
		result = append(result, acc.resp)
	}

	return result
}
