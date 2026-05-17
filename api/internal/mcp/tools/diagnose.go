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
}

// DiagnoseBuildSummary is the build-level header of a diagnose_failure result.
type DiagnoseBuildSummary struct {
	ProjectID   int64  `json:"project_id"`
	ProjectSlug string `json:"project_slug"`
	DisplayName string `json:"display_name"`
	BuildID     int64  `json:"build_id"`
	BuildNumber int    `json:"build_number"`
	Branch      string `json:"branch,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
	TotalTests  int    `json:"total_tests"`
	PassedTests int    `json:"passed_tests"`
	FailedTests int    `json:"failed_tests"`
	BrokenTests int    `json:"broken_tests"`
	ReportURL   string `json:"report_url"`
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
		Description: "Diagnose a failing CI build in ONE call. Use this FIRST when given a failing build or a report URL — it resolves the build, lists every failing test, and for each one returns the error message, failed-step path, defect fingerprint, known issue, attachments, and objective triage signals (fast-fail, failure phase, retry consistency, builds-since-pass, category hint). Accepts a UI URL, (project_ref, build_number), or (project_id, build_id). Set summary_only=true for a compact overview; max_tests caps detailed analysis (default 20).",
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
		for i := range failing {
			out.FailingTests = append(out.FailingTests,
				diagnoseTest(ctx, stores, logger, proj.ID, build, &failing[i], in.SummaryOnly))
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
func diagnoseTest(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID int64, build *store.Build, tr *store.TestResult, summaryOnly bool) DiagnoseTest {
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
	// last-status, fast-fail baseline).
	history := diagnoseTestHistory(ctx, stores, logger, projectID, tr.HistoryID, build.ID)

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
}

// diagnoseTestHistory fetches a test's recent build history and converts it to
// the triage view. The current (failing) build is excluded from `entries` so
// builds-since-pass counts only prior builds; its predecessor's status is
// surfaced as previousStatus.
func diagnoseTestHistory(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID int64, historyID string, currentBuildID int64) diagnoseHistory {
	rows, err := stores.TestResult.GetTestHistory(ctx, projectID, historyID, nil, diagnoseHistoryDepth)
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
	}
	// GetTestHistory returns rows most-recent-first; the first prior entry is
	// the immediately preceding build.
	if len(h.entries) > 0 {
		h.previousStatus = h.entries[0].Status
	}
	return h
}
