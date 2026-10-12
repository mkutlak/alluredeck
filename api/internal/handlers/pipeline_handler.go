package handlers

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// PipelineHandler serves cross-project pipeline run queries for parent projects.
type PipelineHandler struct {
	pipelineStore   store.PipelineStorer
	projectStore    store.ProjectHierarchyReader
	knownIssueStore store.KnownIssueStorer
	projectsDir     string
	logger          *zap.Logger
	now             func() time.Time
}

// NewPipelineHandler creates a PipelineHandler.
func NewPipelineHandler(ps store.PipelineStorer, projStore store.ProjectHierarchyReader, kis store.KnownIssueStorer, projectsDir string, logger *zap.Logger) *PipelineHandler {
	return &PipelineHandler{
		pipelineStore:   ps,
		projectStore:    projStore,
		knownIssueStore: kis,
		projectsDir:     projectsDir,
		logger:          logger,
		now:             time.Now,
	}
}

const (
	defaultPipelinePerPage  = 10
	defaultRunFailuresLimit = 500
	maxRunFailuresLimit     = 2000
)

// awaitingSuitesWindow is how long after its newest upload a run is still
// treated as possibly waiting for suites that have not uploaded yet; past it
// the verdict stands.
// ponytail: fixed window; make it configurable if CI pipelines routinely take longer
const awaitingSuitesWindow = 30 * time.Minute

// markAwaitingSuites sets Aggregate.AwaitingSuites on every run whose newest
// upload is under awaitingSuitesWindow old: the number of suites the store
// expects the run to hold (see store.PipelineStorer.ExpectedSuites) that have
// not uploaded. Older runs cost no store call. A store error is logged and
// leaves the run as it is: the feed must not fail over a hint. groupID is the
// group for runs whose rows carry none (the per-parent list).
func (h *PipelineHandler) markAwaitingSuites(ctx context.Context, runs []pipelineRunResp, groupID int64) {
	now := h.now()
	// ponytail: one sequential ExpectedSuites call per run inside the window (N per page, each proportional to the group's build history); upgrade to one batched query per request (unnest of the recent run keys plus LATERAL)
	for i := range runs {
		// An aborted request would fail every remaining call and log a warning each.
		if ctx.Err() != nil {
			return
		}
		run := &runs[i]
		newest, err := time.Parse(time.RFC3339, run.Timestamp)
		if err != nil || now.Sub(newest) >= awaitingSuitesWindow {
			continue
		}
		group := cmp.Or(run.GroupProjectID, groupID)
		runKey := cmp.Or(run.PipelineID, run.CommitSHA)
		expected, err := h.pipelineStore.ExpectedSuites(ctx, group, runKey)
		if err != nil {
			h.logger.Warn("expected suites", zap.Int64("group_project_id", group), zap.String("run_key", runKey), zap.Error(err))
			continue
		}
		uploaded := make(map[int64]bool, len(run.Suites))
		for j := range run.Suites {
			uploaded[run.Suites[j].ProjectID] = true
		}
		for _, id := range expected {
			if !uploaded[id] {
				run.Aggregate.AwaitingSuites++
			}
		}
	}
}

// markFailureOrigins sets Aggregate.Origins on every run with failing suites:
// how many of its failing tests are new and how many inherited, for the feed's
// collapsed row. A run with more failures than the panel lists (over
// defaultRunFailuresLimit) is left without, as is a run whose failures the
// store could not list: the feed must not fail over a hint. groupID is the group
// for runs whose rows carry none.
func (h *PipelineHandler) markFailureOrigins(ctx context.Context, runs []pipelineRunResp, groupID int64) {
	// ponytail: one sequential ListRunFailures call per failing run on the page (10 by default, up to maxPerPage), each also running the failed-steps query it does not need; upgrade to one batched counts query per request (unnest of the page's run keys plus LATERAL), the same path markAwaitingSuites notes for ExpectedSuites
	for i := range runs {
		// An aborted request would fail every remaining call and log a warning each.
		if ctx.Err() != nil {
			return
		}
		run := &runs[i]
		failed := 0
		for j := range run.Suites {
			failed += run.Suites[j].Failed
		}
		if failed == 0 {
			continue
		}
		group := cmp.Or(run.GroupProjectID, groupID)
		runKey := cmp.Or(run.PipelineID, run.CommitSHA)
		rows, err := h.pipelineStore.ListRunFailures(ctx, group, runKey, defaultRunFailuresLimit+1)
		if err != nil {
			h.logger.Warn("run failure origins", zap.Int64("group_project_id", group), zap.String("run_key", runKey), zap.Error(err))
			continue
		}
		if len(rows) > defaultRunFailuresLimit {
			continue
		}
		origins := countRunOrigins(rows)
		run.Aggregate.Origins = &origins
	}
}

// countRunOrigins counts a run's failing tests as new or inherited. It mirrors
// the UI's mergeRunFailures + countOrigins so the feed row and the expanded
// panel's header agree: copies of one test (same project and full name, or test
// name when that is empty) fold into one, whose origin is the strongest of the
// copies (inherited, then new, else unknown) and which is environment if any
// copy is. An environment failure counts as inherited and also in Environment;
// a test with no known origin is not reported.
func countRunOrigins(rows []store.RunFailureRow) pipelineOriginsResp {
	type testKey struct {
		projectID int64
		identity  string
	}
	type merged struct {
		origin      string
		environment bool
	}
	rank := func(origin string) int {
		switch origin {
		case store.FailureOriginInherited:
			return 2
		case store.FailureOriginNew:
			return 1
		}
		return 0
	}
	tests := map[testKey]merged{}
	for i := range rows {
		row := &rows[i]
		key := testKey{row.ProjectID, cmp.Or(row.FullName, row.TestName)}
		t, seen := tests[key]
		if !seen || rank(row.Origin) > rank(t.origin) {
			t.origin = row.Origin
		}
		t.environment = t.environment || row.Environment
		tests[key] = t
	}
	var counts pipelineOriginsResp
	for _, t := range tests {
		switch {
		case t.environment:
			counts.Inherited++
			counts.Environment++
		case t.origin == store.FailureOriginNew:
			counts.New++
		case t.origin == store.FailureOriginInherited:
			counts.Inherited++
		}
	}
	return counts
}

// GetPipelineRuns returns paginated pipeline runs for a parent project,
// grouped by commit SHA with per-suite and aggregate statistics.
//
//	@Summary      List pipeline runs
//	@Description  Returns child-suite builds grouped by commit SHA for a parent project. While a run's newest upload is under 30 minutes old, `aggregate.awaiting_suites` counts the suites of its previous run that have not uploaded yet.
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
	h.markAwaitingSuites(ctx, runs, projectID)

	writePagedSuccess(w, runs, "pipeline runs retrieved", newPaginationMeta(pp.Page, pp.PerPage, total))
}

// GetAllPipelineRuns returns paginated pipeline runs across every parent
// project's child suites, grouped by commit SHA within each parent group with
// per-suite and aggregate statistics. Unlike GetPipelineRuns, this endpoint is
// not scoped to a single parent project.
//
//	@Summary      List all pipeline runs
//	@Description  Returns child-suite builds across every parent project, grouped by commit SHA within each parent's group. While a run's newest upload is under 30 minutes old, `aggregate.awaiting_suites` counts the suites of its previous run that have not uploaded yet. For a run with failures, `aggregate.origins` counts its failing tests as new or inherited (`environment` is the inherited ones caused by the environment), omitted when its failures list would be truncated (more than 500 failing results), and all zero when no failing test has a known origin.
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
	h.markAwaitingSuites(ctx, runs, 0)
	h.markFailureOrigins(ctx, runs, 0)

	writePagedSuccess(w, runs, "pipeline runs retrieved", newPaginationMeta(pp.Page, pp.PerPage, total))
}

// runFailureResp is one failing test of a run, carrying the suite and build it
// came from so the client does not have to correlate it back.
//
// new_failed is Allure's own comparison with the suite's previous build, kept
// for compatibility. origin is the comparison to read: "inherited" when the
// test failed or broke in the suite's baseline, "new" when it passed there,
// "unknown" when the baseline cannot say. The baseline is the latest older build
// on the suite's own default branch outside this run, with all the shards of its
// pipeline when it had one (see store.RunFailureRow). environment is true when
// the test's defect fingerprint is categorised infrastructure. cluster_key is
// shared by the tests with one failure shape (see runFailureClusterKey).
// failed_step is the deepest failed step that key was built from (the step
// name), absent when the test has no failed step, no history id, or the step has no name.
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
	Origin       string `json:"origin"`
	Environment  bool   `json:"environment"`
	Known        bool   `json:"known"`
	ErrorMessage string `json:"error_message"`
	ClusterKey   string `json:"cluster_key"`
	FailedStep   string `json:"failed_step,omitempty"`
}

// runFailureStep is the deepest failed step of the row, the last element of its
// failed-step path, or "" when the test has no failed step.
func runFailureStep(row *store.RunFailureRow) string {
	if n := len(row.FailedStepPath); n > 0 {
		return row.FailedStepPath[n-1]
	}
	return ""
}

// runFailureClusterKey is the key diagnose_pipeline clusters the row on: the
// message is picked and cut by runner.ClusterMessage, the phase comes from the
// failed-step path, and the deepest step is the path's last. All of it goes
// through the functions diagnose_pipeline uses, so the browser and the IDE group
// the same tests together.
func runFailureClusterKey(row *store.RunFailureRow) string {
	return runner.ClusterKey(
		runner.ClusterMessage(row.FailedStepMessage, row.StatusMessage),
		triage.FailurePhase(row.FailedStepPath),
		runFailureStep(row))
}

// GetPipelineRun returns one pipeline run: the same object the runs feed lists,
// built from the same rows, for a link that lands on that run.
//
//	@Summary      Get a pipeline run
//	@Description  Returns one pipeline run of a parent (group) project with per-suite and aggregate statistics, as listed by the pipeline runs endpoints. The run key is a CI pipeline ID, or a commit SHA when the pipeline ID is absent. While the run's newest upload is under 30 minutes old, `aggregate.awaiting_suites` counts the suites of its previous run that have not uploaded yet.
//	@Tags         pipeline
//	@Produce      json
//	@Param        project_id  path   string  true   "Parent (group) project ID"
//	@Param        run_key     path   string  true   "CI pipeline ID, or commit SHA when the pipeline ID is absent"
//	@Success      200  {object}  map[string]any
//	@Failure      400  {object}  map[string]any
//	@Failure      404  {object}  map[string]any
//	@Failure      500  {object}  map[string]any
//	@Router       /projects/{project_id}/pipeline-runs/{run_key} [get]
func (h *PipelineHandler) GetPipelineRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	runKey := r.PathValue("run_key")
	if runKey == "" {
		writeError(w, http.StatusBadRequest, "invalid run_key")
		return
	}

	ctx := r.Context()
	rows, err := h.pipelineStore.GetPipelineRun(ctx, projectID, runKey)
	if err != nil {
		h.logger.Error("get pipeline run", zap.Int64("project_id", projectID), zap.String("run_key", runKey), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error getting pipeline run")
		return
	}

	runs := groupPipelineRuns(rows)
	if len(runs) == 0 {
		writeError(w, http.StatusNotFound, "pipeline run not found")
		return
	}
	h.markAwaitingSuites(ctx, runs[:1], projectID)

	writeSuccess(w, http.StatusOK, runs[0], "pipeline run retrieved")
}

// GetRunFailures returns every failing test in a single pipeline run.
//
// One run spans all the builds its child suites uploaded under the same
// pipeline ID, and a sharded suite contributes one build per shard, so this
// replaces what would otherwise be a per-build request fan-out from the client.
//
//	@Summary      List run failures
//	@Description  Returns the failed and broken tests of every build in one pipeline run, flagged as known, flaky, retried, or newly-failed. Each test also carries `origin` (`inherited` when it failed or broke in the suite's baseline, `new` when it passed there, `unknown` when the baseline cannot say: the baseline is the latest older build on the suite's own default branch outside this run, with all the shards of its pipeline when it had one) and `environment` (its defect fingerprint is categorised infrastructure). Each test also carries `cluster_key`: tests with the same key share a failure shape (the same normalised message, failure phase and deepest failed step), the key diagnose_pipeline clusters on. `failed_step` is the deepest failed step the key was built from (the step name, absent when the test has no failed step, no history id, or the step has no name).
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
			Origin:       row.Origin,
			Environment:  row.Environment,
			Known:        knownByProject[row.ProjectID][row.TestName],
			ErrorMessage: firstErrorLine(row.StatusMessage),
			ClusterKey:   runFailureClusterKey(row),
			FailedStep:   runFailureStep(row),
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

// pipelineSuiteResp is one suite of a run. Status is "passed" (every test that
// ran passed), "skipped" (stats exist but nothing ran), "degraded" (pass rate
// >= 70%) or "failed" (below that, or no stats at all). PassRate is floored to
// one decimal and is 0 when nothing ran.
type pipelineSuiteResp struct {
	ProjectID   int64   `json:"project_id"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name,omitempty"`
	BuildNumber int     `json:"build_number"`
	BuildID     int64   `json:"build_id"`
	PassRate    float64 `json:"pass_rate"`
	Total       int     `json:"total"`
	Passed      int     `json:"passed"`
	Failed      int     `json:"failed"`
	Skipped     int     `json:"skipped"`
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
	TestsSkipped    int     `json:"tests_skipped"`
	PassRate        float64 `json:"pass_rate"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	AwaitingSuites  int     `json:"awaiting_suites,omitempty"`
	// Origins counts the run's failing tests; see countRunOrigins. Only the
	// runs feed sets it.
	Origins *pipelineOriginsResp `json:"origins,omitempty"`
}

// pipelineOriginsResp counts a run's failing tests by where they come from.
// Environment is a subset of Inherited.
type pipelineOriginsResp struct {
	New         int `json:"new"`
	Inherited   int `json:"inherited"`
	Environment int `json:"environment"`
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
	hasStats    bool // some contributing build reported stats (stat_total non-NULL)
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

		sa.hasStats = sa.hasStats || r.StatTotal != nil
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
			// Floor with integer arithmetic so the rate never overstates:
			// 2499/2500 is 99.9, not 100.0.
			passRate := 0.0
			if denom > 0 {
				passRate = float64(sa.passed*1000/denom) / 10
			}

			// A merged suite only passes when every test that ran passed, so one
			// failing shard is enough to keep the whole suite out of the passed
			// bucket. A suite where nothing ran is "skipped"; one with no stats at
			// all (ReadBuildStats failed) is a broken report and stays "failed".
			status := "failed"
			switch {
			case !sa.hasStats:
			case denom <= 0:
				status = "skipped"
			case sa.passed >= denom:
				status = "passed"
			case passRate >= 70:
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
				Passed:      sa.passed,
				Failed:      sa.failed,
				Skipped:     sa.skipped,
				DurationMs:  sa.durationMs,
				Status:      status,
				Builds:      sa.builds,
			})

			effPassed += sa.passed
			effDenom += denom

			agg.TestsTotal += sa.total
			agg.TestsSkipped += sa.skipped
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
			agg.PassRate = float64(effPassed*1000/effDenom) / 10
		}
		acc.resp.Aggregate = agg
		result = append(result, acc.resp)
	}

	return result
}
