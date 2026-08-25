package tools

import (
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/failure"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

const verdictBuildSHA = "buildsha1"

// withLastGood attaches a last-good pointer carrying the given commit sha.
func withLastGood(d DiagnoseTest, sha string) DiagnoseTest {
	d.LastGood = &failure.LastGood{BuildID: 80, BuildNumber: 25, CommitSHA: sha}
	return d
}

// withHistory sets the two history-derived signals the verdict rules read.
func withHistory(d DiagnoseTest, lastStatus string, buildsSincePass int) DiagnoseTest {
	d.Signals.LastStatus = lastStatus
	d.Signals.BuildsSincePass = buildsSincePass
	return d
}

// withFlaky sets the store-side flaky flag.
func withFlaky(d DiagnoseTest) DiagnoseTest {
	d.Flaky = true
	return d
}

// withKnownIssue attaches the human-confirmed known-issue FK.
func withKnownIssue(d DiagnoseTest, id int64, name string) DiagnoseTest {
	d.KnownIssue = &KnownIssueRef{ID: id, Name: name}
	return d
}

// evalFixture clusters the given tests and returns the rolled-up verdict, the
// same way the handler does.
func evalFixture(t *testing.T, tests []DiagnoseTest, buildSHA string, truncated bool) BuildVerdict {
	t.Helper()
	clusters, members := clusterFailingTests(tests)
	return evaluateBuildVerdict(tests, clusters, members, buildSHA, truncated)
}

const (
	assertMsg  = "expect(received).toBe(200) — received 500"
	locatorMsg = "Timed out 5000ms waiting for locator('#save') to become visible"
	infraMsg   = "API call failed with status 500. URL: /api/TokenAuth/Authenticate"
	genericMsg = "Navigation timeout of 30000 ms exceeded"
)

// TestEvaluateBuildVerdict_Rules is the rule matrix: one row per verdict rule,
// plus the near-miss rows that must NOT trigger it. Each row is a whole build,
// clustered exactly as the handler clusters it.
func TestEvaluateBuildVerdict_Rules(t *testing.T) {
	tests := []struct {
		name           string
		in             []DiagnoseTest
		buildSHA       string
		truncated      bool
		wantVerdict    string
		wantConfidence string
		wantAction     string
		wantAffected   int
		wantGap        bool
	}{
		// ---- infra_env -------------------------------------------------
		{
			name: "infra_env high: two before-hooks failures, shared 5xx, unchanged commit",
			in: []DiagnoseTest{
				withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
				withLastGood(withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInfraEnv,
			wantConfidence: ConfidenceHigh,
			wantAction:     ActionRerun,
			wantAffected:   2,
		},
		{
			name: "infra_env medium: no last-good commit to confirm the code is unchanged",
			in: []DiagnoseTest{
				withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"),
				withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInfraEnv,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionRerun,
			wantAffected:   2,
			wantGap:        true,
		},
		{
			name: "not infra_env: the commit changed since the last pass",
			in: []DiagnoseTest{
				withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), "oldsha"),
				withLastGood(withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), "oldsha"),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   2,
			wantGap:        true,
		},
		{
			name: "not infra_env: a single before-hooks failure is not a pattern",
			in: []DiagnoseTest{
				withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
			wantGap:        true,
		},
		{
			name: "not infra_env: a 4xx is not a server-side failure",
			in: []DiagnoseTest{
				withLastGood(withStatus(clusterFixture("a", "API call failed with status 403", triage.PhaseBeforeHooks, "Before Hooks", "login"), 403, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
				withLastGood(withStatus(clusterFixture("b", "API call failed with status 403", triage.PhaseBeforeHooks, "Before Hooks", "login"), 403, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   2,
			wantGap:        true,
		},

		// ---- known_issue -----------------------------------------------
		{
			name: "known_issue high: a member carries the confirmed FK",
			in: []DiagnoseTest{
				withKnownIssue(clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body"), 12, "flaky-auth"),
				clusterFixture("b", genericMsg, triage.PhaseTestBody, "Test Body"),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictKnownIssue,
			wantConfidence: ConfidenceHigh,
			wantAction:     ActionLinkKnownIssue,
			wantAffected:   2,
		},

		// ---- product_regression ----------------------------------------
		{
			name: "product_regression high: fresh assertion failure after a code change",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", assertMsg, triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusPassed, 0),
				withHistory(withLastGood(clusterFixture("b", assertMsg, triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusPassed, 1),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictProductRegression,
			wantConfidence: ConfidenceHigh,
			wantAction:     ActionFileBug,
			wantAffected:   2,
		},
		{
			name: "product_regression medium: assertion failure with no commit evidence",
			in: []DiagnoseTest{
				withHistory(clusterFixture("a", assertMsg, triage.PhaseTestBody, "Test Body", "check total"), triage.StatusPassed, 0),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictProductRegression,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionFileBug,
			wantAffected:   1,
			wantGap:        true,
		},
		{
			name: "not product_regression: the assertion has been failing for many builds",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", assertMsg, triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusFailed, 6),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
			wantGap:        true,
		},
		{
			name: "not product_regression: the code did not change since the last pass",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", assertMsg, triage.PhaseTestBody, "Test Body", "check total"), verdictBuildSHA), triage.StatusPassed, 0),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictFlaky,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
		},
		{
			name: "not product_regression: a before-hooks assertion is not a test-body regression",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", assertMsg, triage.PhaseBeforeHooks, "Before Hooks", "seed"), "oldsha"), triage.StatusFailed, 1),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
			wantGap:        true,
		},

		// ---- test_bug ---------------------------------------------------
		{
			name: "test_bug medium: locator timeout against unchanged code",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", locatorMsg, triage.PhaseTestBody, "Test Body", "click save"), verdictBuildSHA), triage.StatusFailed, 3),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictTestBug,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionFixTest,
			wantAffected:   1,
		},
		{
			name: "test_bug medium: locator timeout with no commit evidence",
			in: []DiagnoseTest{
				withHistory(clusterFixture("a", locatorMsg, triage.PhaseTestBody, "Test Body", "click save"), triage.StatusFailed, 3),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictTestBug,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionFixTest,
			wantAffected:   1,
			wantGap:        true,
		},
		{
			name: "not test_bug: the code changed, so the selector break may be the product",
			in: []DiagnoseTest{
				withHistory(withLastGood(clusterFixture("a", locatorMsg, triage.PhaseTestBody, "Test Body", "click save"), "oldsha"), triage.StatusFailed, 3),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
			wantGap:        true,
		},

		// ---- flaky -------------------------------------------------------
		{
			name: "flaky medium: the store already flagged the test",
			in: []DiagnoseTest{
				withHistory(withFlaky(clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body")), triage.StatusFailed, 4),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictFlaky,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
		},
		{
			name: "flaky medium: every member passed in the immediately preceding build",
			in: []DiagnoseTest{
				withHistory(clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body"), triage.StatusPassed, 0),
				withHistory(clusterFixture("b", genericMsg, triage.PhaseTestBody, "Test Body"), triage.StatusPassed, 0),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictFlaky,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionHumanNeeded,
			wantAffected:   2,
		},
		{
			name: "not flaky: only one of two members passed last build",
			in: []DiagnoseTest{
				withHistory(clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body"), triage.StatusPassed, 0),
				withHistory(clusterFixture("b", genericMsg, triage.PhaseTestBody, "Test Body"), triage.StatusFailed, 3),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   2,
			wantGap:        true,
		},

		// ---- insufficient evidence ---------------------------------------
		{
			name: "insufficient_evidence low: nothing to go on",
			in: []DiagnoseTest{
				withHistory(clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body"), triage.StatusFailed, 2),
			},
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   1,
			wantGap:        true,
		},
		{
			name:           "insufficient_evidence low: no failing tests at all",
			in:             nil,
			buildSHA:       verdictBuildSHA,
			wantVerdict:    VerdictInsufficientEvidence,
			wantConfidence: ConfidenceLow,
			wantAction:     ActionHumanNeeded,
			wantAffected:   0,
			wantGap:        true,
		},

		// ---- confidence ceilings -----------------------------------------
		{
			name: "truncation forbids a high-confidence verdict",
			in: []DiagnoseTest{
				withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
				withLastGood(withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
			},
			buildSHA:       verdictBuildSHA,
			truncated:      true,
			wantVerdict:    VerdictInfraEnv,
			wantConfidence: ConfidenceMedium,
			wantAction:     ActionRerun,
			wantAffected:   2,
			wantGap:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evalFixture(t, tc.in, tc.buildSHA, tc.truncated)
			if got.Verdict != tc.wantVerdict {
				t.Errorf("verdict: got %q, want %q (evidence %+v, gaps %v)", got.Verdict, tc.wantVerdict, got.Evidence, got.EvidenceGaps)
			}
			if got.Confidence != tc.wantConfidence {
				t.Errorf("confidence: got %q, want %q (evidence %+v, gaps %v)", got.Confidence, tc.wantConfidence, got.Evidence, got.EvidenceGaps)
			}
			if got.RecommendedAction != tc.wantAction {
				t.Errorf("recommended_action: got %q, want %q", got.RecommendedAction, tc.wantAction)
			}
			if got.AffectedTestCount != tc.wantAffected {
				t.Errorf("affected_test_count: got %d, want %d", got.AffectedTestCount, tc.wantAffected)
			}
			if tc.wantGap && len(got.EvidenceGaps) == 0 {
				t.Error("evidence_gaps: got none, want at least one")
			}
			if !tc.wantGap && len(got.EvidenceGaps) != 0 {
				t.Errorf("evidence_gaps: got %v, want none", got.EvidenceGaps)
			}
			if got.Confidence == ConfidenceHigh && tc.truncated {
				t.Error("confidence must never be high when the diagnosis is truncated")
			}
		})
	}
}

// TestEvaluateBuildVerdict_KnownIssueRegexMatch verifies a cluster-level regex
// match is enough for a known_issue verdict, with no per-test FK involved.
func TestEvaluateBuildVerdict_KnownIssueRegexMatch(t *testing.T) {
	tests := []DiagnoseTest{
		clusterFixture("a", genericMsg, triage.PhaseTestBody, "Test Body"),
		clusterFixture("b", genericMsg, triage.PhaseTestBody, "Test Body"),
	}
	clusters, members := clusterFailingTests(tests)
	clusters[0].KnownIssueRegexMatches = []KnownIssueRegexMatch{
		{KnownIssueID: 9, Name: "nav-timeout", MatchedSubstring: "Navigation timeout"},
	}

	got := evaluateBuildVerdict(tests, clusters, members, verdictBuildSHA, false)
	if got.Verdict != VerdictKnownIssue {
		t.Errorf("verdict: got %q, want %q", got.Verdict, VerdictKnownIssue)
	}
	if got.Confidence != ConfidenceHigh {
		t.Errorf("confidence: got %q, want high", got.Confidence)
	}
	if got.RecommendedAction != ActionLinkKnownIssue {
		t.Errorf("recommended_action: got %q, want %q", got.RecommendedAction, ActionLinkKnownIssue)
	}
	if !evidenceMentions(got, "known_issue") {
		t.Errorf("evidence must name the known issue, got %+v", got.Evidence)
	}
}

// TestEvaluateBuildVerdict_MixedClustersCapConfidence verifies that when the
// clusters disagree the build takes the dominant cluster's verdict, its
// confidence is capped at medium, and the evidence records the split rather
// than quietly presenting one cluster's story as the whole build's.
func TestEvaluateBuildVerdict_MixedClustersCapConfidence(t *testing.T) {
	tests := []DiagnoseTest{
		withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
		withLastGood(withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
		withHistory(withLastGood(clusterFixture("c", locatorMsg, triage.PhaseTestBody, "Test Body", "click save"), verdictBuildSHA), triage.StatusFailed, 3),
	}

	got := evalFixture(t, tests, verdictBuildSHA, false)
	if got.Verdict != VerdictInfraEnv {
		t.Errorf("verdict: got %q, want the dominant cluster's %q", got.Verdict, VerdictInfraEnv)
	}
	if got.Confidence != ConfidenceMedium {
		t.Errorf("confidence: got %q, want medium (capped by the cluster split)", got.Confidence)
	}
	if got.AffectedTestCount != 2 {
		t.Errorf("affected_test_count: got %d, want 2 (the dominant cluster only)", got.AffectedTestCount)
	}
	if !evidenceMentions(got, "cluster_verdict_split") {
		t.Errorf("evidence must record the split, got %+v", got.Evidence)
	}
}

// TestEvaluateBuildVerdict_AgreeingClustersKeepHighConfidence verifies the cap
// applies to a genuine disagreement, not to two clusters that reached the same
// verdict.
func TestEvaluateBuildVerdict_AgreeingClustersKeepHighConfidence(t *testing.T) {
	tests := []DiagnoseTest{
		withLastGood(withStatus(clusterFixture("a", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
		withLastGood(withStatus(clusterFixture("b", infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate"), verdictBuildSHA),
		withLastGood(withStatus(clusterFixture("c", "API call failed with status 502. URL: /api/Session/Start", triage.PhaseBeforeHooks, "Before Hooks", "session"), 502, "/api/Session/Start"), verdictBuildSHA),
		withLastGood(withStatus(clusterFixture("d", "API call failed with status 502. URL: /api/Session/Start", triage.PhaseBeforeHooks, "Before Hooks", "session"), 502, "/api/Session/Start"), verdictBuildSHA),
	}

	got := evalFixture(t, tests, verdictBuildSHA, false)
	if got.Verdict != VerdictInfraEnv {
		t.Fatalf("verdict: got %q, want %q", got.Verdict, VerdictInfraEnv)
	}
	if got.Confidence != ConfidenceHigh {
		t.Errorf("confidence: got %q, want high (both clusters agree)", got.Confidence)
	}
	if evidenceMentions(got, "cluster_verdict_split") {
		t.Errorf("agreeing clusters must not report a split, got %+v", got.Evidence)
	}
}

// evidenceMentions reports whether any evidence entry names the given signal.
func evidenceMentions(v BuildVerdict, signal string) bool {
	for _, e := range v.Evidence {
		if strings.Contains(e.Signal, signal) {
			return true
		}
	}
	return false
}

// TestErrorClassifiers pins the two message classes the verdict rules split on.
// The Playwright auto-retrying assertion is deliberately both: it is an
// assertion expressed through a locator, and the rule order decides which
// reading wins.
func TestErrorClassifiers(t *testing.T) {
	tests := []struct {
		name          string
		msg           string
		wantAssertion bool
		wantLocator   bool
	}{
		{name: "expect toBe", msg: "expect(received).toBe(200)", wantAssertion: true},
		{name: "assertion error", msg: "AssertionError: values differ", wantAssertion: true},
		{name: "toContain", msg: "expected list toContain 'x'", wantAssertion: true},
		{name: "plain locator timeout", msg: "waiting for locator('#save')", wantLocator: true},
		{name: "selector", msg: "no element matches selector .btn", wantLocator: true},
		{name: "stale element", msg: "stale element reference", wantLocator: true},
		{
			name:          "auto-retrying assertion is both",
			msg:           "Timed out 5000ms waiting for expect(locator).toContainText('Saved')",
			wantAssertion: true,
			wantLocator:   true,
		},
		{name: "neither", msg: "Navigation timeout of 30000 ms exceeded"},
		{name: "empty", msg: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAssertionClassError(tc.msg); got != tc.wantAssertion {
				t.Errorf("isAssertionClassError(%q) = %v, want %v", tc.msg, got, tc.wantAssertion)
			}
			if got := isLocatorClassError(tc.msg); got != tc.wantLocator {
				t.Errorf("isLocatorClassError(%q) = %v, want %v", tc.msg, got, tc.wantLocator)
			}
		})
	}
}
