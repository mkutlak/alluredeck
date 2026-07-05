package tools

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// ---------------------------------------------------------------------------
// diagnose_failure
//
// diagnose_failure collapses the multi-call CI-failure investigation workflow
// (resolve_url → list_failing_tests → get_test_failure → get_test_history → …)
// into a single server-side call. Given a build it resolves the build, lists
// every failing test, and for each one fetches failure detail, build history,
// and the failed-step path, then runs triage.Analyze to attach objective
// triage signals. The result is one structure an AI agent can read top to
// bottom to diagnose the build.
// ---------------------------------------------------------------------------

// diagnoseDefaultMaxTests is the default cap on the number of failing tests
// examined in detail. Tests beyond the cap are truncated and reported via
// DiagnoseFailureOutput.Truncated.
const diagnoseDefaultMaxTests = 20

// diagnoseAbsoluteMaxTests is the hard upper bound on max_tests; a caller
// cannot ask for more detailed analysis than this in a single call.
const diagnoseAbsoluteMaxTests = 100

// diagnoseHistoryDepth is how many recent builds of a test's history are
// fetched to feed triage (builds-since-pass, fast-fail baseline, last status).
const diagnoseHistoryDepth = 20

// DiagnoseFailureInput holds parameters for the diagnose_failure tool.
//
// The build is identified by exactly one of: a UI URL, a (project_ref,
// build_number) pair, or a (project_id, build_id) pair. Resolution mirrors
// resolve_url so the same inputs that work there work here.
type DiagnoseFailureInput struct {
	// URL is a UI report URL, e.g. "http://host/projects/1/reports/28".
	URL string `json:"url,omitempty"`
	// ProjectRef is a numeric project_id or a slug; used with BuildNumber when
	// URL is absent.
	ProjectRef string `json:"project_ref,omitempty"`
	// BuildNumber is the human-facing build number from the UI; used with
	// ProjectRef when URL is absent.
	BuildNumber int `json:"build_number,omitempty"`
	// ProjectID + BuildID identify the build directly when the caller already
	// holds resolved IDs (e.g. from a prior tool call).
	ProjectID int   `json:"project_id,omitempty"`
	BuildID   int64 `json:"build_id,omitempty"`
	// SummaryOnly omits per-test heavy fields (failed_step_path, attachments),
	// keeping error_message and signals for a compact overview.
	SummaryOnly bool `json:"summary_only,omitempty"`
	// MaxTests caps the number of failing tests examined in detail. Defaults to
	// 20, clamped to 100. Tests beyond the cap are reported via `truncated`.
	MaxTests int `json:"max_tests,omitempty"`
	// IncludeLastGoodDiff additionally computes the full last-good→current diff
	// per test (one extra whole-build comparison query each). Off by default to
	// keep the common path cheap.
	IncludeLastGoodDiff bool `json:"include_last_good_diff,omitempty"`
}

// DiagnoseBuildSummary is the build-level header of a diagnose_failure result.
type DiagnoseBuildSummary struct {
	ProjectID   int64             `json:"project_id"`
	ProjectSlug string            `json:"project_slug"`
	DisplayName string            `json:"display_name"`
	BuildID     int64             `json:"build_id"`
	BuildNumber int               `json:"build_number"`
	Branch      string            `json:"branch,omitempty"`
	CommitSHA   string            `json:"commit_sha,omitempty"`
	CreatedAt   string            `json:"created_at"`
	TotalTests  int               `json:"total_tests"`
	PassedTests int               `json:"passed_tests"`
	FailedTests int               `json:"failed_tests"`
	BrokenTests int               `json:"broken_tests"`
	ReportURL   string            `json:"report_url"`
	Environment map[string]string `json:"environment,omitempty"`
}

// DiagnoseTest is one failing test with its diagnosis attached.
type DiagnoseTest struct {
	FullName     string `json:"full_name"`
	HistoryID    string `json:"history_id"`
	Status       string `json:"status"`
	DurationMs   int64  `json:"duration_ms"`
	ErrorMessage string `json:"error_message,omitempty"`
	// FailedStepPath is the ordered list of step names from the root step to
	// the deepest failed step. Omitted when SummaryOnly is set.
	FailedStepPath []string `json:"failed_step_path,omitempty"`
	// Signals carries the objective triage signals computed by triage.Analyze.
	Signals triage.Signals `json:"signals"`
	// Fingerprint is the defect fingerprint linked to this test, when one exists.
	Fingerprint *FingerprintInfo `json:"fingerprint,omitempty"`
	// KnownIssue is the known issue matched via the defect fingerprint, if any.
	KnownIssue *KnownIssueRef `json:"known_issue,omitempty"`
	// Attachments are the build attachments. Omitted when SummaryOnly is set.
	Attachments []AttachmentRef `json:"attachments,omitempty"`
	// LastGood points at the build where this test last passed (branch-scoped
	// when available). Nil when the test has no prior passing build.
	LastGood *LastGood `json:"last_good,omitempty"`
	// LastGoodDiff summarizes the last-good→current whole-build comparison.
	// Populated only when include_last_good_diff is set and a last-good build
	// exists.
	LastGoodDiff *LastGoodDiff `json:"last_good_diff,omitempty"`
}

// DiagnoseFailureOutput is the structured output for diagnose_failure: a
// build-level summary followed by a per-failing-test diagnosis array.
type DiagnoseFailureOutput struct {
	Build DiagnoseBuildSummary `json:"build"`
	// FailingTests holds one diagnosed entry per failing test, up to MaxTests.
	FailingTests []DiagnoseTest `json:"failing_tests"`
	// ExaminedTests is the number of failing tests diagnosed in detail.
	ExaminedTests int `json:"examined_tests"`
	// Truncated is true when more failing tests exist than were examined.
	Truncated bool `json:"truncated"`
	// TruncatedCount is the number of failing tests not examined in detail.
	TruncatedCount int `json:"truncated_count,omitempty"`
}

// RegisterDiagnoseTools registers the diagnose_failure tool on s.
func RegisterDiagnoseTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "diagnose_failure",
		Description: "Diagnose a failing CI build in ONE call. Use this FIRST when given a failing build or a report URL — it resolves the build, lists every failing test, and for each one returns the error message, failed-step path, defect fingerprint, known issue, attachments, and objective triage signals (fast-fail, failure phase, retry consistency, builds-since-pass, category hint). Triage signals (builds-since-pass, last-status, fast-fail baseline) are scoped to the build's branch when available, so comparisons reflect only the same line of development. Each failing test also carries a `last_good` pointer to the build where it last passed (build_number, commit_sha, builds_since); it is omitted when the test never passed before. Also returns the test environment metadata (Allure environment.properties: base URLs, versions, and any debug links the CI recorded). Accepts a UI URL, (project_ref, build_number), or (project_id, build_id). Set summary_only=true for a compact overview; max_tests caps detailed analysis (default 20). Set include_last_good_diff=true to additionally return, per test, the last-good→current whole-build diff (`last_good_diff`: this test's passed→failed transition plus co-regressions in that span) — one extra comparison query per test, off by default.",
	}, diagnoseFailureHandler(stores, logger))
}

func diagnoseFailureHandler(stores *bootstrap.Stores, logger *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in DiagnoseFailureInput) (*mcpsdk.CallToolResult, DiagnoseFailureOutput, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in DiagnoseFailureInput) (*mcpsdk.CallToolResult, DiagnoseFailureOutput, error) {
		// Clamp max_tests.
		maxTests := in.MaxTests
		if maxTests <= 0 {
			maxTests = diagnoseDefaultMaxTests
		}
		if maxTests > diagnoseAbsoluteMaxTests {
			maxTests = diagnoseAbsoluteMaxTests
		}

		// 1. Resolve the build to (project, build).
		proj, build, err := resolveDiagnoseTarget(ctx, stores, in)
		if err != nil {
			return nil, DiagnoseFailureOutput{}, err
		}

		out := DiagnoseFailureOutput{
			Build:        diagnoseBuildSummary(proj, build),
			FailingTests: []DiagnoseTest{},
		}

		// 2. List failing tests for the build. Fetch maxTests+1 to detect
		//    truncation without a second query.
		failing, err := stores.TestResult.ListFailedByBuild(ctx, proj.ID, build.ID, maxTests+1)
		if err != nil {
			return nil, DiagnoseFailureOutput{}, fmt.Errorf("listing failing tests: %w", err)
		}
		if len(failing) > maxTests {
			out.Truncated = true
			out.TruncatedCount = len(failing) - maxTests
			failing = failing[:maxTests]
		}

		// 3. Diagnose each failing test. Attachments are resolved per test
		//    result inside diagnoseTest (scoped via test_result_id) so a test
		//    only carries its own attachments, never the whole build's.
		//    lgCache memoizes the gated last-good→current whole-build diff:
		//    every test shares the same current build and many share the same
		//    last-good build, so without it CompareBuildsByHistoryID would be
		//    re-run identically once per failing test.
		lgCache := newLastGoodDiffCache()
		for i := range failing {
			out.FailingTests = append(out.FailingTests,
				diagnoseTest(ctx, stores, logger, proj.ID, build, &failing[i], in.SummaryOnly, in.IncludeLastGoodDiff, lgCache))
		}
		out.ExaminedTests = len(out.FailingTests)

		return nil, out, nil
	}
}

// resolveDiagnoseTarget resolves the diagnose_failure input to a project and
// build. It accepts a UI URL, (project_ref, build_number), or (project_id,
// build_id), mirroring resolve_url so callers have one consistent contract.
func resolveDiagnoseTarget(ctx context.Context, stores *bootstrap.Stores, in DiagnoseFailureInput) (*store.Project, *store.Build, error) {
	// Direct ID path: project_id + build_id.
	if in.ProjectID > 0 && in.BuildID > 0 {
		proj, err := stores.Project.GetProject(ctx, int64(in.ProjectID))
		if err != nil {
			return nil, nil, fmt.Errorf("project not found (id=%d): %w", in.ProjectID, err)
		}
		if proj == nil {
			return nil, nil, fmt.Errorf("project %d not found", in.ProjectID)
		}
		b, err := stores.Build.GetBuildByID(ctx, proj.ID, in.BuildID)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use a URL or project_ref+build_number instead): %w",
				in.BuildID, in.ProjectID, err)
		}
		return proj, &b, nil
	}

	// URL or (project_ref, build_number) path.
	projectRef, buildNumber, err := diagnoseRefAndNumber(in)
	if err != nil {
		return nil, nil, err
	}

	var proj *store.Project
	if numericRe.MatchString(projectRef) {
		id, _ := strconv.ParseInt(projectRef, 10, 64)
		proj, err = stores.Project.GetProject(ctx, id)
		if err != nil {
			return nil, nil, fmt.Errorf("project not found (id=%s): %w", projectRef, err)
		}
	} else {
		proj, err = stores.Project.GetProjectBySlug(ctx, projectRef)
		if err != nil {
			return nil, nil, fmt.Errorf("project not found (slug=%q): %w", projectRef, err)
		}
	}
	if proj == nil {
		return nil, nil, fmt.Errorf("project %q not found", projectRef)
	}

	b, err := stores.Build.GetBuildByNumber(ctx, proj.ID, buildNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("build #%d not found in project %q: %w", buildNumber, proj.Slug, err)
	}
	return proj, &b, nil
}

// diagnoseRefAndNumber extracts (projectRef, buildNumber) from either the URL
// or the explicit project_ref + build_number fields.
func diagnoseRefAndNumber(in DiagnoseFailureInput) (string, int, error) {
	if in.URL != "" {
		parsed, err := url.Parse(in.URL)
		if err != nil {
			return "", 0, fmt.Errorf("invalid url %q: %w", in.URL, err)
		}
		m := reURLPath.FindStringSubmatch(parsed.Path)
		if m == nil {
			return "", 0, fmt.Errorf("url path %q does not match /projects/<proj>/reports/<num>", parsed.Path)
		}
		num, err := strconv.Atoi(m[reURLPath.SubexpIndex("num")])
		if err != nil {
			return "", 0, fmt.Errorf("build_number in url is not an integer: %w", err)
		}
		return m[reURLPath.SubexpIndex("proj")], num, nil
	}

	if in.ProjectRef == "" {
		return "", 0, fmt.Errorf("provide one of: url, (project_ref + build_number), or (project_id + build_id)")
	}
	if in.BuildNumber <= 0 {
		return "", 0, fmt.Errorf("build_number must be positive when url is absent")
	}
	return in.ProjectRef, in.BuildNumber, nil
}

// diagnoseBuildSummary builds the build-level header of the output.
func diagnoseBuildSummary(proj *store.Project, build *store.Build) DiagnoseBuildSummary {
	s := DiagnoseBuildSummary{
		ProjectID:   proj.ID,
		ProjectSlug: proj.Slug,
		DisplayName: proj.DisplayName,
		BuildID:     build.ID,
		BuildNumber: build.BuildNumber,
		CreatedAt:   build.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		ReportURL:   fmt.Sprintf("/projects/%d/reports/%d", proj.ID, build.BuildNumber),
	}
	if build.CIBranch != nil {
		s.Branch = *build.CIBranch
	}
	if build.CICommitSHA != nil {
		s.CommitSHA = *build.CICommitSHA
	}
	if build.StatTotal != nil {
		s.TotalTests = *build.StatTotal
	}
	if build.StatPassed != nil {
		s.PassedTests = *build.StatPassed
	}
	if build.StatFailed != nil {
		s.FailedTests = *build.StatFailed
	}
	if build.StatBroken != nil {
		s.BrokenTests = *build.StatBroken
	}
	s.Environment = build.Environment
	return s
}

// diagnoseAttachmentsLimit caps how many per-test attachment refs are returned
// for a single test; ample for any realistic test while bounding output size.
const diagnoseAttachmentsLimit = 50

// diagnoseAttachments fetches the attachments belonging to a single test
// result, scoped via test_result_id so a test carries only its own
// attachments. It returns nil on error or when no test matches (attachments
// are best-effort context, never fatal to a diagnosis); the caller logs a
// warning so the failure is observable.
func diagnoseAttachments(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID, buildID int64, historyID string) []AttachmentRef {
	rows, err := stores.Attachment.ListByTestResult(ctx, projectID, buildID, historyID, diagnoseAttachmentsLimit)
	if err != nil {
		logger.Warn("diagnose_failure: per-test attachment fetch failed",
			zap.Int64("project_id", projectID),
			zap.Int64("build_id", buildID),
			zap.String("history_id", historyID),
			zap.Error(err))
		return nil
	}
	refs := make([]AttachmentRef, 0, len(rows))
	for _, a := range rows {
		refs = append(refs, attachmentToRef(a))
	}
	return refs
}

// diagnoseTest assembles the diagnosis for a single failing test: failure
// detail, build history, failed-step path, per-test attachments, and triage
// signals.
func diagnoseTest(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID int64, build *store.Build, tr *store.TestResult, summaryOnly, includeLastGoodDiff bool, lgCache *lastGoodDiffCache) DiagnoseTest {
	d := DiagnoseTest{
		FullName:   tr.FullName,
		HistoryID:  tr.HistoryID,
		Status:     tr.Status,
		DurationMs: tr.DurationMs,
	}

	// Failed-step path: walk test_steps for the deepest failed step. The path
	// drives triage's failure-phase classification; the deepest failed step's
	// status_message is the most specific error text available for the test.
	stepPath, errorMessage, err := stores.TestResult.GetFailedStepPath(ctx, projectID, build.ID, tr.HistoryID)
	if err != nil {
		// Missing steps are not fatal: triage degrades gracefully to test_body.
		stepPath = nil
		errorMessage = ""
	}
	d.ErrorMessage = errorMessage
	if !summaryOnly && len(stepPath) > 0 {
		d.FailedStepPath = stepPath
	}

	// Defect fingerprint + known issue via the defect_fingerprint_id FK.
	var category string
	if fpID, err := stores.TestResult.GetDefectFingerprintID(ctx, projectID, build.ID, tr.HistoryID); err == nil && fpID != nil {
		if defect, err := stores.Defect.GetByID(ctx, *fpID); err == nil && defect != nil {
			category = defect.Category
			d.Fingerprint = &FingerprintInfo{
				Hash:     defect.FingerprintHash,
				Category: defect.Category,
			}
			if defect.KnownIssueID != nil {
				if ki, err := stores.KnownIssue.Get(ctx, *defect.KnownIssueID); err == nil && ki != nil {
					d.KnownIssue = &KnownIssueRef{ID: ki.ID, Name: ki.TestName}
				}
			}
		}
	}

	// Build history for this test → feeds triage (builds-since-pass,
	// last-status, fast-fail baseline). Scoped to the build's branch when
	// available; nil falls back to cross-branch behavior.
	history := diagnoseTestHistory(ctx, stores, logger, projectID, tr.HistoryID, build.ID, build.BranchID)

	// Last-good pointer: the most recent build before this one where the test
	// passed (branch-scoped when available). Best-effort — a store error yields
	// no last-good context rather than failing the diagnosis. Always computed
	// (one indexed LIMIT 1 row); the heavier whole-build diff below is gated.
	// beforeBuildOrder is build.BuildNumber (build_order), not build.ID: builds
	// are not guaranteed to be ingested in build_order sequence, so build.ID
	// ordering does not reliably reflect chronological order.
	if lg, err := stores.TestResult.GetLastPassingBuild(ctx, projectID, tr.HistoryID, build.BranchID, build.BuildNumber); err != nil {
		logger.Warn("diagnose_failure: last-good build fetch failed",
			zap.Int64("project_id", projectID),
			zap.String("history_id", tr.HistoryID),
			zap.Error(err))
	} else if lg != nil {
		d.LastGood = &LastGood{
			BuildID:     lg.BuildID,
			BuildNumber: lg.BuildNumber,
			CreatedAt:   lg.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			BuildsSince: buildsSinceLastGood(history.priorBuildOrders, lg.BuildNumber, build.BuildNumber),
		}
		if lg.CICommitSHA != nil {
			d.LastGood.CommitSHA = *lg.CICommitSHA
		}

		// Whole-build last-good→current diff, gated behind the opt-in flag.
		// CompareBuildsByHistoryID(lg, current) orders A=last-good, B=current so
		// StatusFrom/StatusTo read correctly. lgCache memoizes this per
		// lastGoodBuildID across the failing-tests loop. Best-effort like the
		// pointer above.
		if includeLastGoodDiff {
			if diffs, err := lgCache.get(ctx, stores, projectID, lg.BuildID, build.ID); err != nil {
				logger.Warn("diagnose_failure: last-good diff comparison failed",
					zap.Int64("project_id", projectID),
					zap.String("history_id", tr.HistoryID),
					zap.Error(err))
			} else {
				lgd := buildLastGoodDiff(diffs, tr.HistoryID, lg.BuildID, build.ID)
				d.LastGoodDiff = &lgd
			}
		}
	}

	// Run triage to attach objective signals.
	d.Signals = triage.Analyze(triage.Input{
		DurationMs:          tr.DurationMs,
		ErrorMessage:        errorMessage,
		FailedStepPath:      stepPath,
		BuildHistory:        history.entries,
		PreviousBuildStatus: history.previousStatus,
		Category:            category,
	})

	// Attachments scoped to this test result only — never the whole build.
	if !summaryOnly {
		d.Attachments = diagnoseAttachments(ctx, stores, logger, projectID, build.ID, tr.HistoryID)
	}
	return d
}

// diagnoseHistory is the build-history view triage needs for one test.
type diagnoseHistory struct {
	// entries is the test's recent build history, most-recent-first.
	entries []triage.BuildHistoryEntry
	// previousStatus is the status of the build immediately preceding the
	// current (failing) one. Empty when there is no prior build.
	previousStatus string
	// priorBuildOrders holds the build_order (TestHistoryEntry.BuildNumber) of
	// each prior-history entry (the current build excluded), aligned with
	// `entries`. triage.BuildHistoryEntry does not carry a build identifier, so
	// this is the source used to count builds since the last-good pass.
	// GetTestHistory's window is the most-recent diagnoseHistoryDepth builds
	// project/branch-wide, NOT capped at the currently diagnosed build — when
	// diagnosing a non-latest build this window can hold builds newer than the
	// one being diagnosed, so buildsSinceLastGood must bound its count above by
	// the current build's order too, not just below by the last-good order.
	priorBuildOrders []int
}

// buildsSinceLastGood counts prior-history builds whose build_order falls
// strictly between the last-good build and the current (diagnosed) build:
// builds that ran after the last-good pass but before the current build. Both
// bounds are required — the depth-capped history window is the most recent N
// builds overall, so when a non-latest build is diagnosed it can contain
// builds newer than currentOrder, which must not be counted. The result is
// bounded by diagnoseHistoryDepth: a gap wider than the window reports at most
// diagnoseHistoryDepth-1 even though LastGood itself is found via an uncapped
// query.
func buildsSinceLastGood(priorBuildOrders []int, lastGoodOrder, currentOrder int) int {
	n := 0
	for _, order := range priorBuildOrders {
		if order > lastGoodOrder && order < currentOrder {
			n++
		}
	}
	return n
}

// lastGoodDiffCache memoizes CompareBuildsByHistoryID(lastGoodBuildID,
// currentBuildID) results within a single diagnose_failure call. The current
// build is constant across every failing test in the call, and many failing
// tests share the same last-good build, so without memoization the same
// whole-build diff query would be re-run identically once per failing test
// under include_last_good_diff. Not safe for concurrent use; diagnoseTest is
// invoked sequentially by diagnoseFailureHandler's loop.
type lastGoodDiffCache struct {
	diffs map[int64][]store.DiffEntry
	errs  map[int64]error
}

// newLastGoodDiffCache returns an empty cache ready for use.
func newLastGoodDiffCache() *lastGoodDiffCache {
	return &lastGoodDiffCache{
		diffs: make(map[int64][]store.DiffEntry),
		errs:  make(map[int64]error),
	}
}

// get returns the cached diff for lastGoodBuildID, fetching and memoizing it
// via CompareBuildsByHistoryID on first use (keyed only by lastGoodBuildID
// since currentBuildID is constant for the lifetime of the cache). A cached
// error is replayed without re-querying the store.
func (c *lastGoodDiffCache) get(ctx context.Context, stores *bootstrap.Stores, projectID, lastGoodBuildID, currentBuildID int64) ([]store.DiffEntry, error) {
	if diffs, ok := c.diffs[lastGoodBuildID]; ok {
		return diffs, nil
	}
	if err, ok := c.errs[lastGoodBuildID]; ok {
		return nil, err
	}
	diffs, err := stores.TestResult.CompareBuildsByHistoryID(ctx, projectID, lastGoodBuildID, currentBuildID)
	if err != nil {
		c.errs[lastGoodBuildID] = err
		return nil, err
	}
	c.diffs[lastGoodBuildID] = diffs
	return diffs, nil
}

// diagnoseTestHistory fetches a test's recent build history and converts it to
// the triage view. The current (failing) build is excluded from `entries` so
// builds-since-pass counts only prior builds; its predecessor's status is
// surfaced as previousStatus. When branchID is non-nil, history is scoped to
// that branch; nil falls back to cross-branch behavior.
func diagnoseTestHistory(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID int64, historyID string, currentBuildID int64, branchID *int64) diagnoseHistory {
	rows, err := stores.TestResult.GetTestHistory(ctx, projectID, historyID, branchID, diagnoseHistoryDepth)
	if err != nil {
		logger.Warn("diagnose_failure: test history fetch failed",
			zap.Int64("project_id", projectID),
			zap.String("history_id", historyID),
			zap.Error(err))
		return diagnoseHistory{}
	}

	var h diagnoseHistory
	for _, r := range rows {
		// Skip the current build: triage treats `entries` as prior history.
		if r.BuildID == currentBuildID {
			continue
		}
		h.entries = append(h.entries, triage.BuildHistoryEntry{
			Status:     r.Status,
			DurationMs: r.DurationMs,
		})
		h.priorBuildOrders = append(h.priorBuildOrders, r.BuildNumber)
	}
	// GetTestHistory returns rows most-recent-first; the first prior entry is
	// the immediately preceding build.
	if len(h.entries) > 0 {
		h.previousStatus = h.entries[0].Status
	}
	return h
}
