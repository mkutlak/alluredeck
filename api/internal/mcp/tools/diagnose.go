package tools

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/failure"
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
	URL string `json:"url,omitempty" jsonschema:"AllureDeck UI report URL, e.g. http://host/projects/1/reports/28. Supply this OR (project_ref + build_number) OR (project_id + build_id)."`
	// ProjectRef is a numeric project_id or a slug; used with BuildNumber when
	// URL is absent.
	ProjectRef string `json:"project_ref,omitempty" jsonschema:"Numeric project_id or project slug. Use together with build_number when no url is given."`
	// BuildNumber is the human-facing build number from the UI; used with
	// ProjectRef when URL is absent.
	BuildNumber int `json:"build_number,omitempty" jsonschema:"Human-facing build number shown in the UI (builds.build_order), NOT the internal build_id. Use together with project_ref."`
	// ProjectID + BuildID identify the build directly when the caller already
	// holds resolved IDs (e.g. from a prior tool call).
	ProjectID int   `json:"project_id,omitempty" jsonschema:"Internal numeric project id. Use together with build_id when both were already resolved by an earlier tool call."`
	BuildID   int64 `json:"build_id,omitempty" jsonschema:"Internal build id from resolve_url or list_recent_builds. This is NOT the build_number in a UI URL. Use together with project_id."`
	// SummaryOnly omits per-test heavy fields (failed_step_path, attachments),
	// keeping error_message and signals for a compact overview.
	SummaryOnly bool `json:"summary_only,omitempty" jsonschema:"When true, omit the heavy per-test fields (failed_step_path, attachments) and keep error_message plus triage signals for a compact overview."`
	// MaxTests caps the number of failing tests examined in detail. Defaults to
	// 20, clamped to 100. Tests beyond the cap are reported via `truncated`.
	MaxTests int `json:"max_tests,omitempty" jsonschema:"Maximum number of failing tests to diagnose in detail. Defaults to 20, clamped to 100; the rest are reported via truncated/truncated_count."`
	// IncludeLastGoodDiff expands last_good_diff_counts into the full
	// last-good→current diff per test. Off by default because the lists are
	// large, not because they cost an extra query: the comparison already runs
	// for the counts and is memoized per distinct last-good build.
	IncludeLastGoodDiff bool `json:"include_last_good_diff,omitempty" jsonschema:"When true, expand last_good_diff_counts into the full per-test whole-build last-good to current diff (last_good_diff: this test's transition plus a sample of co-regressions). Off by default because the lists are large; the counts are always returned."`
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
	// CIPipelineID identifies the CI pipeline run this build belongs to.
	// Playwright CI shards upload one build each under a shared pipeline ID, so
	// a non-empty value here is a signal that sibling shard builds may exist —
	// see diagnose_pipeline to check all of them at once.
	CIPipelineID string `json:"ci_pipeline_id,omitempty"`
	// CIPipelineURL is the CI system's link to the pipeline run.
	CIPipelineURL string `json:"ci_pipeline_url,omitempty"`
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
	LastGood *failure.LastGood `json:"last_good,omitempty"`
	// LastGoodDiffCounts is the size of the last-good→current whole-build
	// change set. Emitted whenever a last-good build exists: the comparison is
	// memoized per distinct last-good build, so the counts cost at most one
	// query per distinct last-good build in the call rather than one per test.
	LastGoodDiffCounts *LastGoodDiffCounts `json:"last_good_diff_counts,omitempty"`
	// LastGoodDiff is the full last-good→current comparison — this test's own
	// transition plus a sample of co-regressions. The lists are the expensive
	// part of the payload, not the query, so they stay behind
	// include_last_good_diff while the counts above are always emitted.
	LastGoodDiff *failure.LastGoodDiff `json:"last_good_diff,omitempty"`
	// Retries is how many times the runner re-ran this test within the build.
	Retries int `json:"retries,omitempty"`
	// RetryAttempts is the per-attempt outcome sequence behind Retries, oldest
	// first. It is what `signals.retry_consistency` is computed from, so it is
	// the evidence for that classification: read it to see whether the retries
	// failed the same way or diverged. Empty when the test was not retried or
	// when the report carried no per-attempt detail.
	RetryAttempts []RetryAttemptRef `json:"retry_attempts,omitempty"`
	// Flaky reports whether the runner (or a human triage decision) marked this
	// test flaky.
	Flaky bool `json:"flaky,omitempty"`
	// MergedHistoryIDs lists the history_ids of duplicate rows that were
	// collapsed into this one by full_name. Playwright ingestion records the
	// same test twice — an enriched row and an empty shell under a second
	// history_id scheme — so the twins are merged and the dropped identifiers
	// surfaced here rather than silently discarded.
	MergedHistoryIDs []string `json:"merged_history_ids,omitempty"`
	// LastGoodAbsentReason explains an absent last_good instead of leaving the
	// caller to guess whether the lookup ran at all.
	LastGoodAbsentReason string `json:"last_good_absent_reason,omitempty"`
	// CrossBranchLastGood is the most recent pass found on ANY branch, returned
	// only when the branch-scoped last-good lookup came up empty. It is a
	// weaker bisect anchor than LastGood — the passing run is from a different
	// line of development — so it is reported under its own name.
	CrossBranchLastGood *CrossBranchLastGood `json:"cross_branch_last_good,omitempty"`
	// ClusterID names the cluster in DiagnoseFailureOutput.Clusters this test
	// belongs to. Non-representative members of a cluster carry no
	// error_message or failed_step_path; read the cluster's shared_error
	// instead.
	ClusterID string `json:"cluster_id,omitempty"`
}

// RetryAttemptRef is one execution attempt of a retried test.
//
// Status is verbatim from the source report rather than normalized to the
// Allure vocabulary: a Playwright attempt reports "timedOut" or "interrupted",
// and those say more about the failure than folding them into "broken" would.
type RetryAttemptRef struct {
	// AttemptIndex is the zero-based attempt number: 0 is the first run.
	AttemptIndex int `json:"attempt_index"`
	// Status is the attempt's outcome as the source report spelled it.
	Status string `json:"status"`
	// ErrorMessage is the attempt's error text, capped at
	// diagnoseAttemptMessageMax characters. Empty for a passing attempt.
	ErrorMessage string `json:"error_message,omitempty"`
}

// LastGoodDiffCounts is the size of the whole-build change set between a test's
// last-good build and the current one.
type LastGoodDiffCounts struct {
	Regressed int `json:"regressed"`
	Fixed     int `json:"fixed"`
	Added     int `json:"added"`
}

// CrossBranchLastGood points at the most recent build on any branch where a
// test passed. It is the fallback for a test with no passing run on its own
// branch; Branch names the branch that pass came from.
type CrossBranchLastGood struct {
	Branch      string `json:"branch,omitempty"`
	BuildID     int64  `json:"build_id"`
	BuildNumber int    `json:"build_number"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// DiagnoseFailureOutput is the structured output for diagnose_failure: a
// build-level summary followed by a per-failing-test diagnosis array.
type DiagnoseFailureOutput struct {
	Build DiagnoseBuildSummary `json:"build"`
	// BuildVerdict is the deterministic build-level judgement rolled up from
	// the dominant cluster. Always present; read its confidence and
	// evidence_gaps before acting on it.
	BuildVerdict BuildVerdict `json:"build_verdict"`
	// Clusters groups the failing tests by root cause, largest cluster first.
	Clusters []DiagnoseCluster `json:"clusters"`
	// FailingTests holds one diagnosed entry per failing test, up to MaxTests.
	FailingTests []DiagnoseTest `json:"failing_tests"`
	// ExaminedTests is the number of failing tests diagnosed in detail.
	ExaminedTests int `json:"examined_tests"`
	// Truncated is true when more failing tests exist than were examined. It is
	// derived from the DISTINCT full_name count of failing tests in the build,
	// not from the number of rows fetched.
	Truncated bool `json:"truncated"`
	// TruncatedCount is the number of failing tests not examined in detail.
	TruncatedCount int `json:"truncated_count,omitempty"`
	// Warnings carries caveats about the diagnosis itself — anything the caller
	// should know before trusting the counts, e.g. how many duplicate rows were
	// merged away.
	Warnings []string `json:"warnings,omitempty"`
}

// RegisterDiagnoseTools registers the diagnose_failure tool on s.
func RegisterDiagnoseTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "diagnose_failure",
		Title:       "Diagnose AllureDeck failure",
		Annotations: readOnlyAnnotations(),
		Description: "Diagnose a failing CI build in ONE call. Use this FIRST when given a failing build or a report URL — it resolves the build, groups every failing test by root cause, and judges the build. Read it top-down: `build_verdict` first, then `clusters`, then the per-test detail. `build_verdict` is deterministic: {verdict: product_regression|test_bug|infra_env|known_issue|flaky|insufficient_evidence, confidence, evidence[], evidence_gaps[], recommended_action: file_bug|fix_test|rerun|link_known_issue|human_needed, affected_test_count}. It is rolled up from the largest cluster; confidence is never high when the diagnosis was truncated or when the clusters disagreed, and `evidence_gaps` names what could not be checked — trust it accordingly. `clusters` groups failing tests that share a normalized error message, failure phase, and deepest failed step, largest cluster first: each carries member_count, member_full_names, a representative, shared_error, failure_phase, shared_status_pattern (only when every member agrees), and `known_issue_regex_matches` (active known-issue rules whose pattern matches the shared error — a regex hit that NO human has confirmed, unlike the per-test `known_issue` which is the human-linked defect FK). TOKEN NOTE: only a cluster's representative keeps `error_message` and `failed_step_path`; other members carry `cluster_id` and are otherwise complete — read the cluster's `shared_error` for their text. Per failing test you also get the defect fingerprint, known issue, attachments, and objective triage signals (fast-fail, failure phase, retry consistency, builds-since-pass, last-status, repeated status pattern, category hint). `retry_consistency` classifies a retried test as consistent (every attempt failed the same way — deterministic, so a rerun will not help) or varying (the attempts diverged — flaky or environmental); it is \"single\" for a test that ran once or whose report carried no per-attempt detail, so read it together with `retries` rather than treating \"single\" as evidence of a clean run. The attempts behind that verdict are returned as `retry_attempts` [{attempt_index, status, error_message}], oldest first, with status verbatim from the runner (a Playwright attempt reports \"timedOut\" or \"interrupted\") and error_message truncated; they are omitted for a test that was not retried. Triage signals are scoped to the build's branch when available, so comparisons reflect only the same line of development. `category_hint` is normally a passthrough of the ingestion-time defect category at low confidence; when the signals in this same call are strong enough (a fast abort in before_hooks against a 5xx endpoint) it is replaced with source=\"signals\" at medium confidence. Each failing test also carries a `last_good` pointer to the build where it last passed (build_number, commit_sha, builds_since) and, whenever last_good exists, `last_good_diff_counts` {regressed, fixed, added} for that span; both are omitted when the test never passed before. Also returns the test environment metadata (Allure environment.properties: base URLs, versions, and any debug links the CI recorded). Accepts a UI URL, (project_ref, build_number), or (project_id, build_id). Set summary_only=true for a compact overview; max_tests caps detailed analysis (default 20). Set include_last_good_diff=true to expand the counts into the full per-test diff (`last_good_diff`: this test's passed→failed transition plus co-regressions in that span); the lists are off by default because they are large, not because they are expensive. Duplicate rows for the same test (Playwright records each test twice, under two history_id schemes) are collapsed by full_name: the dropped identifiers appear on the surviving test as `merged_history_ids`, and a top-level `warnings` entry reports how many rows were merged, so `examined_tests` counts real failures rather than rows. The collapse is conservative — rows sharing a full_name are merged ONLY when at most one of them carries an error message (the enriched-row/empty-shell twin signature). When several rows under one full_name each carry their OWN error message they are genuinely different results (a parameterized test whose parameters never reached full_name), so they are all kept and a `warnings` entry names the full_name; a repeated full_name in `failing_tests` therefore means real distinct failures, not a dedup miss. When a test has no passing run on the build's own branch, `last_good_absent_reason` says so and `cross_branch_last_good` points at the most recent pass on any branch.",
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

		// 2. List failing tests for the build. Playwright ingestion writes two
		//    rows per test — an enriched one and an empty shell under a second
		//    history_id scheme — so ask for 2*maxTests+2 rows: enough that a
		//    build made entirely of twins still yields maxTests distinct tests
		//    after the collapse below, with a spare pair in hand.
		failing, err := stores.TestResult.ListFailedByBuild(ctx, proj.ID, build.ID, 2*maxTests+2)
		if err != nil {
			return nil, DiagnoseFailureOutput{}, fmt.Errorf("listing failing tests: %w", err)
		}

		// Collapse the twins BEFORE diagnosing anything: every per-test step
		// below costs several queries, and diagnosing both halves of a
		// duplicate pair doubles that cost to report the same failure twice.
		failing, mergedIDs, dedupeWarnings := dedupeByFullName(failing)
		if len(failing) > maxTests {
			failing = failing[:maxTests]
		}
		mergedRows := 0
		for i := range failing {
			mergedRows += len(mergedIDs[failing[i].HistoryID])
		}
		if mergedRows > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"%d duplicate history_id rows merged by full_name", mergedRows))
		}
		// Groups that shared a full_name but carried distinct results were left
		// unmerged; say so rather than let the caller read the repeated name as
		// a dedup failure.
		out.Warnings = append(out.Warnings, dedupeWarnings...)

		// 3. Diagnose each failing test. Attachments are resolved per test
		//    result inside diagnoseTest (scoped via test_result_id) so a test
		//    only carries its own attachments, never the whole build's.
		//    lgCache memoizes the gated last-good→current whole-build diff:
		//    every test shares the same current build and many share the same
		//    last-good build, so without it CompareBuildsByHistoryID would be
		//    re-run identically once per failing test.
		lgCache := newLastGoodDiffCache()
		for i := range failing {
			d := diagnoseTest(ctx, stores, logger, proj.ID, build, &failing[i], in.SummaryOnly, in.IncludeLastGoodDiff, lgCache)
			d.MergedHistoryIDs = mergedIDs[failing[i].HistoryID]
			out.FailingTests = append(out.FailingTests, d)
		}
		out.ExaminedTests = len(out.FailingTests)

		// 4. Truncation is measured against the DISTINCT failing-test count,
		//    not the number of rows fetched: with duplicate rows in play a
		//    row-count comparison reports phantom truncation on a build whose
		//    every failure was already examined. A count failure is
		//    non-fatal — it costs the truncation hint, not the diagnosis.
		if total, err := stores.TestResult.CountFailedByBuild(ctx, proj.ID, build.ID); err != nil {
			logger.Warn("diagnose_failure: failing-test count failed",
				zap.Int64("project_id", proj.ID),
				zap.Int64("build_id", build.ID),
				zap.Error(err))
		} else if total > out.ExaminedTests {
			out.Truncated = true
			out.TruncatedCount = total - out.ExaminedTests
		}

		// 5. Group the failures by root cause, then judge the build from the
		//    groups. Clustering also strips the duplicated error text off every
		//    non-representative member, so it must run after the per-test loop
		//    and before anything reads FailingTests. The verdict needs
		//    out.Truncated, so it runs after the count above.
		clusters, members := clusterFailingTests(out.FailingTests)
		matcher := newKnownIssueMatcher(ctx, stores, logger, proj.ID)
		for i := range clusters {
			clusters[i].KnownIssueRegexMatches = matcher.match(clusters[i].SharedError)
		}
		out.Clusters = clusters
		out.BuildVerdict = evaluateBuildVerdict(out.FailingTests, clusters, members, out.Build.CommitSHA, out.Truncated)

		digest := textResult(diagnoseDigest(&out))
		return digest, out, nil
	}
}

// diagnoseDigestSnippetMax bounds the shared-error excerpt in the digest, which
// has to stay a headline rather than become a second payload.
const diagnoseDigestSnippetMax = 60

// diagnoseDigest renders the one-line unstructured summary of a diagnosis. The
// structured payload carries the detail; this is the headline a client shows
// before deciding whether to read it, so it leads with the verdict and the
// dominant cluster — the two things that decide whether reading on is worth it.
func diagnoseDigest(out *DiagnoseFailureOutput) string {
	branch := out.Build.Branch
	if branch == "" {
		branch = "no branch"
	}
	d := fmt.Sprintf("build #%d (%s): %d failing in %d clusters",
		out.Build.BuildNumber, branch, out.ExaminedTests, len(out.Clusters))
	d += " — verdict: " + out.BuildVerdict.Verdict + " (" + out.BuildVerdict.Confidence + ")"
	if len(out.Clusters) > 0 {
		if detail := clusterDigestSnippet(&out.Clusters[0]); detail != "" {
			d += ": " + detail
		}
	}
	if out.Truncated {
		d += fmt.Sprintf("; %d not examined", out.TruncatedCount)
	}
	if len(out.Warnings) > 0 {
		d += fmt.Sprintf("; %d warning(s)", len(out.Warnings))
	}
	return d + "; report " + out.Build.ReportURL
}

// clusterDigestSnippet renders the dominant cluster as "4x status 500 at
// /api/TokenAuth/Authenticate in before_hooks". The status pattern is preferred
// over the raw message because it is already short and already the point.
func clusterDigestSnippet(c *DiagnoseCluster) string {
	detail := ""
	switch {
	case c.SharedStatusPattern != nil:
		detail = "status " + statusPatternValue(c.SharedStatusPattern)
	case strings.TrimSpace(c.SharedError) != "":
		detail = strings.Join(strings.Fields(c.SharedError), " ")
	default:
		return ""
	}
	if r := []rune(detail); len(r) > diagnoseDigestSnippetMax {
		detail = string(r[:diagnoseDigestSnippetMax]) + "…"
	}
	s := fmt.Sprintf("%dx %s", c.MemberCount, detail)
	if c.FailurePhase != "" {
		s += " in " + c.FailurePhase
	}
	return s
}

// dedupeByFullName collapses the duplicate test_results rows that Playwright
// ingestion produces: the same test is written twice per build, once as an
// enriched row (history_id scheme "md5:md5", carrying status_message, steps and
// attachments) and once as an empty shell (scheme "md5.md5"). Left alone they
// make diagnose_failure report every real failure twice.
//
// Rows are grouped by full_name — the identity that is stable across both
// schemes — but a group is only collapsed when it carries the TWIN SIGNATURE:
// at most one of its rows has a status_message, the rest being empty shells.
// That is the only shape in which the rows provably describe one failure.
//
// A group with two or more rows that each carry their own status_message is
// NOT the twin case: it is one full_name covering several genuinely different
// results, which is what a parameterized test looks like when the parameters do
// not reach full_name. Picking a winner there and listing the others as
// merged_history_ids would state something false — that the dropped rows were
// duplicates of the survivor — and would hide real failures. Those groups are
// left intact and reported through warnings instead.
//
// The survivor of a collapsed group is the row with a non-empty StatusMessage;
// on a tie the ":"-scheme history_id wins, because that is the row the
// enrichment tables hang off. kept preserves the first-seen order of the
// groups. mergedIDs maps a survivor's history_id to the history_ids dropped
// from its group, so nothing disappears silently.
//
// Rows with an empty full_name are never grouped: an empty name is not an
// identity, and collapsing on it would merge unrelated tests.
func dedupeByFullName(rows []store.TestResult) (kept []store.TestResult, mergedIDs map[string][]string, warnings []string) {
	mergedIDs = make(map[string][]string)
	if len(rows) == 0 {
		return nil, mergedIDs, nil
	}

	// Group row indices by full_name, preserving the first-seen order of the
	// groups. A row with an empty full_name gets a group of its own so it keeps
	// its position without ever merging.
	groups := make([][]int, 0, len(rows))
	slotOf := make(map[string]int, len(rows)) // full_name → index into groups
	for i := range rows {
		if rows[i].FullName == "" {
			groups = append(groups, []int{i})
			continue
		}
		slot, seen := slotOf[rows[i].FullName]
		if !seen {
			slotOf[rows[i].FullName] = len(groups)
			groups = append(groups, []int{i})
			continue
		}
		groups[slot] = append(groups[slot], i)
	}

	kept = make([]store.TestResult, 0, len(rows))
	for _, group := range groups {
		if len(group) == 1 {
			kept = append(kept, rows[group[0]])
			continue
		}

		enriched := 0
		for _, idx := range group {
			if rows[idx].StatusMessage != "" {
				enriched++
			}
		}
		if enriched > 1 {
			for _, idx := range group {
				kept = append(kept, rows[idx])
			}
			warnings = append(warnings, fmt.Sprintf(
				"%d tests share full_name %q with distinct results (parameterized?) — not merged",
				enriched, rows[group[0]].FullName))
			continue
		}

		// Twin signature: one enriched row at most, everything else an empty
		// shell of it.
		survivor := group[0]
		for _, idx := range group[1:] {
			if preferTestResult(&rows[idx], &rows[survivor]) {
				survivor = idx
			}
		}
		dropped := make([]string, 0, len(group)-1)
		for _, idx := range group {
			if idx != survivor {
				dropped = append(dropped, rows[idx].HistoryID)
			}
		}
		mergedIDs[rows[survivor].HistoryID] = dropped
		kept = append(kept, rows[survivor])
	}

	if len(kept) == 0 {
		return nil, mergedIDs, warnings
	}
	return kept, mergedIDs, warnings
}

// preferTestResult reports whether candidate should displace incumbent as the
// surviving row for a full_name. An enriched row (non-empty status_message)
// always wins; failing that, the ":"-scheme history_id wins; failing that the
// incumbent stays, so the input order breaks the final tie.
func preferTestResult(candidate, incumbent *store.TestResult) bool {
	if (candidate.StatusMessage != "") != (incumbent.StatusMessage != "") {
		return candidate.StatusMessage != ""
	}
	candColon := strings.Contains(candidate.HistoryID, ":")
	incColon := strings.Contains(incumbent.HistoryID, ":")
	if candColon != incColon {
		return candColon
	}
	return false
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
	if build.CIPipelineID != nil {
		s.CIPipelineID = *build.CIPipelineID
	}
	if build.CIPipelineURL != nil {
		s.CIPipelineURL = *build.CIPipelineURL
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

// diagnoseAttemptMessageMax bounds an attempt's error text in the output. The
// stored value is already capped at 2000 characters, but a diagnosis carries
// several attempts for each of up to max_tests tests, so the payload caps again
// and harder: enough text to tell two failures apart, not enough to repeat the
// full error once per attempt.
const diagnoseAttemptMessageMax = 500

// diagnoseRetryAttempts fetches the per-attempt outcomes of a retried test.
// It returns nil without querying when the test was not retried: attempts only
// exist to be compared with each other, so a single run has nothing to fetch,
// and skipping keeps the common case at zero extra queries. A store error is
// non-fatal — attempts are supporting evidence, never the diagnosis itself — so
// it yields no attempts and a warning the operator can see.
func diagnoseRetryAttempts(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID, buildID int64, tr *store.TestResult) []RetryAttemptRef {
	if tr.Retries <= 0 {
		return nil
	}
	rows, err := stores.TestResult.GetAttempts(ctx, projectID, buildID, tr.HistoryID)
	if err != nil {
		logger.Warn("diagnose_failure: retry attempt fetch failed",
			zap.Int64("project_id", projectID),
			zap.Int64("build_id", buildID),
			zap.String("history_id", tr.HistoryID),
			zap.Error(err))
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	refs := make([]RetryAttemptRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, RetryAttemptRef{
			AttemptIndex: r.AttemptIndex,
			Status:       r.Status,
			ErrorMessage: truncateRunes(r.StatusMessage, diagnoseAttemptMessageMax),
		})
	}
	return refs
}

// triageRetryAttempts converts the emitted attempts into triage's input shape.
//
// ErrorKey combines status and message because it is the identity triage
// compares attempts on, and neither half is sufficient alone: two attempts that
// both failed with no message are the same failure, while a failure and a pass
// that both carry no message are not. triage treats the key as opaque and only
// ever tests it for equality, so any stable encoding will do.
func triageRetryAttempts(refs []RetryAttemptRef) []triage.RetryAttempt {
	if len(refs) == 0 {
		return nil
	}
	out := make([]triage.RetryAttempt, 0, len(refs))
	for _, r := range refs {
		out = append(out, triage.RetryAttempt{
			ErrorKey:     r.Status + "|" + r.ErrorMessage,
			ErrorMessage: r.ErrorMessage,
		})
	}
	return out
}

// truncateRunes caps s at max runes. It counts runes rather than bytes so a
// multi-byte character is never split into an invalid UTF-8 fragment.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
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
		Retries:    tr.Retries,
		Flaky:      tr.Flaky,
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
				Hash:             defect.FingerprintHash,
				Category:         defect.Category,
				OccurrenceCount:  defect.OccurrenceCount,
				Resolution:       defect.Resolution,
				FirstSeenBuildID: defect.FirstSeenBuildID,
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
		d.LastGood = &failure.LastGood{
			BuildID:     lg.BuildID,
			BuildNumber: lg.BuildNumber,
			CreatedAt:   lg.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			BuildsSince: buildsSinceLastGood(history.priorBuildOrders, lg.BuildNumber, build.BuildNumber),
		}
		if lg.CICommitSHA != nil {
			d.LastGood.CommitSHA = *lg.CICommitSHA
		}

		// Whole-build last-good→current diff.
		// CompareBuildsByHistoryID(lg, current) orders A=last-good, B=current so
		// StatusFrom/StatusTo read correctly. lgCache memoizes this per
		// lastGoodBuildID across the failing-tests loop, so the whole loop costs
		// one query per DISTINCT last-good build — cheap enough that the counts
		// are always emitted. Only the lists (this_test, sample_regressed) stay
		// behind the flag, since they are what actually costs tokens.
		// Best-effort like the pointer above.
		if diffs, err := lgCache.get(ctx, stores, projectID, lg.BuildID, build.ID); err != nil {
			logger.Warn("diagnose_failure: last-good diff comparison failed",
				zap.Int64("project_id", projectID),
				zap.String("history_id", tr.HistoryID),
				zap.Error(err))
		} else {
			lgd := failure.BuildLastGoodDiff(diffs, tr.HistoryID, lg.BuildID, build.ID)
			d.LastGoodDiffCounts = &LastGoodDiffCounts{
				Regressed: lgd.RegressedCount,
				Fixed:     lgd.FixedCount,
				Added:     lgd.AddedCount,
			}
			if includeLastGoodDiff {
				d.LastGoodDiff = &lgd
			}
		}
	} else if build.BranchID != nil {
		// The branch-scoped lookup found nothing. Say why: an absent last_good
		// otherwise reads as "not investigated" when it actually means "this
		// test has never passed on this branch". Then make ONE cross-branch
		// attempt so the caller still gets a bisect anchor — a pass on another
		// branch is weaker evidence, so it is reported under its own field
		// rather than smuggled into last_good.
		branchName := "this branch"
		if build.CIBranch != nil && *build.CIBranch != "" {
			branchName = *build.CIBranch
		}
		d.LastGoodAbsentReason = fmt.Sprintf("no passing run on branch %s in recorded history", branchName)

		if cb, err := stores.TestResult.GetLastPassingBuild(ctx, projectID, tr.HistoryID, nil, build.BuildNumber); err != nil {
			logger.Warn("diagnose_failure: cross-branch last-good fetch failed",
				zap.Int64("project_id", projectID),
				zap.String("history_id", tr.HistoryID),
				zap.Error(err))
		} else if cb != nil {
			d.CrossBranchLastGood = &CrossBranchLastGood{
				Branch:      cb.BranchName,
				BuildID:     cb.BuildID,
				BuildNumber: cb.BuildNumber,
				CreatedAt:   cb.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			}
			if cb.CICommitSHA != nil {
				d.CrossBranchLastGood.CommitSHA = *cb.CICommitSHA
			}
		}
	}

	// Per-attempt outcomes, which drive the retry-consistency signal. Gated on
	// Retries so the common path — a test that ran once — costs no extra query;
	// a test that was never retried has nothing to compare anyway. Best-effort:
	// losing the attempts costs the retry signal, not the diagnosis.
	retryAttempts := diagnoseRetryAttempts(ctx, stores, logger, projectID, build.ID, tr)
	d.RetryAttempts = retryAttempts

	// Run triage to attach objective signals.
	d.Signals = triage.Analyze(triage.Input{
		DurationMs:          tr.DurationMs,
		ErrorMessage:        errorMessage,
		FailedStepPath:      stepPath,
		RetryAttempts:       triageRetryAttempts(retryAttempts),
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
