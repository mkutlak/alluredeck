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

const (
	assertMsg  = "expect(received).toBe(200) — received 500"
	locatorMsg = "Timed out 5000ms waiting for locator('#save') to become visible"
	infraMsg   = "API call failed with status 500. URL: /api/TokenAuth/Authenticate"
	genericMsg = "Navigation timeout of 30000 ms exceeded"
)

// evidenceMentions reports whether any evidence entry names the given signal.
func evidenceMentions(v BuildVerdict, signal string) bool {
	for _, e := range v.Evidence {
		if strings.Contains(e.Signal, signal) {
			return true
		}
	}
	return false
}

// TestEvaluateBuildVerdict_Rules is the rule matrix: one row per verdict rule,
// plus the near-miss rows that must NOT trigger it. Each row is a whole build,
// clustered exactly as the handler clusters it, against build commit
// verdictBuildSHA.
func TestEvaluateBuildVerdict_Rules(t *testing.T) {
	// A before-hooks failure against a 5xx endpoint.
	infra := func(name string) DiagnoseTest {
		return withStatus(clusterFixture(name, infraMsg, triage.PhaseBeforeHooks, "Before Hooks", "login"), 500, "/api/TokenAuth/Authenticate")
	}
	status403 := func(name string) DiagnoseTest {
		return withLastGood(withStatus(clusterFixture(name, "API call failed with status 403", triage.PhaseBeforeHooks, "Before Hooks", "login"),
			403, "/api/TokenAuth/Authenticate"), verdictBuildSHA)
	}
	session502 := func(name string) DiagnoseTest {
		return withLastGood(withStatus(clusterFixture(name, "API call failed with status 502. URL: /api/Session/Start", triage.PhaseBeforeHooks,
			"Before Hooks", "session"), 502, "/api/Session/Start"), verdictBuildSHA)
	}
	assertion := func(name, phase string, steps ...string) DiagnoseTest {
		return clusterFixture(name, assertMsg, phase, steps...)
	}
	locator := func(name string) DiagnoseTest {
		return clusterFixture(name, locatorMsg, triage.PhaseTestBody, "Test Body", "click save")
	}
	generic := func(name string) DiagnoseTest {
		return clusterFixture(name, genericMsg, triage.PhaseTestBody, "Test Body")
	}
	flaky := generic("a")
	flaky.Flaky = true
	knownFK := generic("a")
	knownFK.KnownIssue = &KnownIssueRef{ID: 12, Name: "flaky-auth"}

	tests := []struct {
		name                        string
		in                          []DiagnoseTest
		truncated                   bool
		verdict, confidence, action string
		affected                    int
		gap, split                  bool
	}{
		// ---- infra_env
		{"infra_env high: two before-hooks failures, shared 5xx, unchanged commit",
			[]DiagnoseTest{withLastGood(infra("a"), verdictBuildSHA), withLastGood(infra("b"), verdictBuildSHA)}, false,
			VerdictInfraEnv, ConfidenceHigh, ActionRerun, 2, false, false},
		{"infra_env medium: no last-good commit to confirm the code is unchanged",
			[]DiagnoseTest{infra("a"), infra("b")}, false,
			VerdictInfraEnv, ConfidenceMedium, ActionRerun, 2, true, false},
		{"not infra_env: the commit changed since the last pass",
			[]DiagnoseTest{withLastGood(infra("a"), "oldsha"), withLastGood(infra("b"), "oldsha")}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 2, true, false},
		{"not infra_env: a single before-hooks failure is not a pattern",
			[]DiagnoseTest{withLastGood(infra("a"), verdictBuildSHA)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 1, true, false},
		{"not infra_env: a 4xx is not a server-side failure",
			[]DiagnoseTest{status403("a"), status403("b")}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 2, true, false},
		// ---- known_issue
		{"known_issue high: a member carries the confirmed FK", []DiagnoseTest{knownFK, generic("b")}, false,
			VerdictKnownIssue, ConfidenceHigh, ActionLinkKnownIssue, 2, false, false},
		// ---- product_regression
		{"product_regression high: fresh assertion failure after a code change",
			[]DiagnoseTest{
				withHistory(withLastGood(assertion("a", triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusPassed, 0),
				withHistory(withLastGood(assertion("b", triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusPassed, 1),
			}, false, VerdictProductRegression, ConfidenceHigh, ActionFileBug, 2, false, false},
		{"product_regression medium: assertion failure with no commit evidence",
			[]DiagnoseTest{withHistory(assertion("a", triage.PhaseTestBody, "Test Body", "check total"), triage.StatusPassed, 0)}, false,
			VerdictProductRegression, ConfidenceMedium, ActionFileBug, 1, true, false},
		{"not product_regression: the assertion has been failing for many builds",
			[]DiagnoseTest{withHistory(withLastGood(assertion("a", triage.PhaseTestBody, "Test Body", "check total"), "oldsha"), triage.StatusFailed, 6)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 1, true, false},
		{"not product_regression: the code did not change since the last pass",
			[]DiagnoseTest{withHistory(withLastGood(assertion("a", triage.PhaseTestBody, "Test Body", "check total"), verdictBuildSHA), triage.StatusPassed, 0)}, false,
			VerdictFlaky, ConfidenceMedium, ActionHumanNeeded, 1, false, false},
		{"not product_regression: a before-hooks assertion is not a test-body regression",
			[]DiagnoseTest{withHistory(withLastGood(assertion("a", triage.PhaseBeforeHooks, "Before Hooks", "seed"), "oldsha"), triage.StatusFailed, 1)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 1, true, false},
		// ---- test_bug
		{"test_bug medium: locator timeout against unchanged code",
			[]DiagnoseTest{withHistory(withLastGood(locator("a"), verdictBuildSHA), triage.StatusFailed, 3)}, false,
			VerdictTestBug, ConfidenceMedium, ActionFixTest, 1, false, false},
		{"test_bug medium: locator timeout with no commit evidence",
			[]DiagnoseTest{withHistory(locator("a"), triage.StatusFailed, 3)}, false,
			VerdictTestBug, ConfidenceMedium, ActionFixTest, 1, true, false},
		{"not test_bug: the code changed, so the selector break may be the product",
			[]DiagnoseTest{withHistory(withLastGood(locator("a"), "oldsha"), triage.StatusFailed, 3)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 1, true, false},
		// ---- flaky
		{"flaky medium: the store already flagged the test", []DiagnoseTest{withHistory(flaky, triage.StatusFailed, 4)}, false,
			VerdictFlaky, ConfidenceMedium, ActionHumanNeeded, 1, false, false},
		{"flaky medium: every member passed in the immediately preceding build",
			[]DiagnoseTest{withHistory(generic("a"), triage.StatusPassed, 0), withHistory(generic("b"), triage.StatusPassed, 0)}, false,
			VerdictFlaky, ConfidenceMedium, ActionHumanNeeded, 2, false, false},
		{"not flaky: only one of two members passed last build",
			[]DiagnoseTest{withHistory(generic("a"), triage.StatusPassed, 0), withHistory(generic("b"), triage.StatusFailed, 3)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 2, true, false},
		// ---- insufficient evidence
		{"insufficient_evidence low: nothing to go on", []DiagnoseTest{withHistory(generic("a"), triage.StatusFailed, 2)}, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 1, true, false},
		{"insufficient_evidence low: no failing tests at all", nil, false,
			VerdictInsufficientEvidence, ConfidenceLow, ActionHumanNeeded, 0, true, false},
		// ---- confidence ceilings
		{"truncation forbids a high-confidence verdict",
			[]DiagnoseTest{withLastGood(infra("a"), verdictBuildSHA), withLastGood(infra("b"), verdictBuildSHA)}, true,
			VerdictInfraEnv, ConfidenceMedium, ActionRerun, 2, true, false},
		// Disagreeing clusters: the dominant one decides, confidence is capped
		// and the split is recorded rather than one cluster's story presented
		// as the whole build's.
		{"disagreeing clusters cap confidence",
			[]DiagnoseTest{withLastGood(infra("a"), verdictBuildSHA), withLastGood(infra("b"), verdictBuildSHA),
				withHistory(withLastGood(locator("c"), verdictBuildSHA), triage.StatusFailed, 3)}, false,
			VerdictInfraEnv, ConfidenceMedium, ActionRerun, 2, false, true},
		{"agreeing clusters keep high confidence",
			[]DiagnoseTest{withLastGood(infra("a"), verdictBuildSHA), withLastGood(infra("b"), verdictBuildSHA), session502("c"), session502("d")}, false,
			VerdictInfraEnv, ConfidenceHigh, ActionRerun, 2, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusters, members := clusterFailingTests(tc.in)
			got := evaluateBuildVerdict(tc.in, clusters, members, verdictBuildSHA, tc.truncated)
			if got.Verdict != tc.verdict || got.Confidence != tc.confidence || got.RecommendedAction != tc.action || got.AffectedTestCount != tc.affected {
				t.Errorf("verdict = %s/%s/%s affected %d, want %s/%s/%s affected %d (evidence %+v, gaps %v)",
					got.Verdict, got.Confidence, got.RecommendedAction, got.AffectedTestCount,
					tc.verdict, tc.confidence, tc.action, tc.affected, got.Evidence, got.EvidenceGaps)
			}
			if (len(got.EvidenceGaps) > 0) != tc.gap {
				t.Errorf("evidence_gaps = %v, want gaps=%v", got.EvidenceGaps, tc.gap)
			}
			if evidenceMentions(got, "cluster_verdict_split") != tc.split {
				t.Errorf("evidence = %+v, want cluster_verdict_split=%v", got.Evidence, tc.split)
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
	clusters[0].KnownIssueRegexMatches = []KnownIssueRegexMatch{{KnownIssueID: 9, Name: "nav-timeout", MatchedSubstring: "Navigation timeout"}}

	got := evaluateBuildVerdict(tests, clusters, members, verdictBuildSHA, false)
	if got.Verdict != VerdictKnownIssue || got.Confidence != ConfidenceHigh || got.RecommendedAction != ActionLinkKnownIssue {
		t.Errorf("verdict = %s/%s/%s, want known_issue/high/link_known_issue", got.Verdict, got.Confidence, got.RecommendedAction)
	}
	if !evidenceMentions(got, "known_issue") {
		t.Errorf("evidence must name the known issue, got %+v", got.Evidence)
	}
}

// TestErrorClassifiers pins the two message classes the verdict rules split on.
// The Playwright auto-retrying assertion is deliberately both: it is an
// assertion expressed through a locator, and the rule order decides which
// reading wins.
func TestErrorClassifiers(t *testing.T) {
	tests := []struct {
		msg                  string
		assertion, locatorOK bool
	}{
		{"expect(received).toBe(200)", true, false},
		{"AssertionError: values differ", true, false},
		{"expected list toContain 'x'", true, false},
		{"waiting for locator('#save')", false, true},
		{"no element matches selector .btn", false, true},
		{"stale element reference", false, true},
		{"Timed out 5000ms waiting for expect(locator).toContainText('Saved')", true, true},
		{"Navigation timeout of 30000 ms exceeded", false, false},
		{"", false, false},
	}
	for _, tc := range tests {
		if got := isAssertionClassError(tc.msg); got != tc.assertion {
			t.Errorf("isAssertionClassError(%q) = %v, want %v", tc.msg, got, tc.assertion)
		}
		if got := isLocatorClassError(tc.msg); got != tc.locatorOK {
			t.Errorf("isLocatorClassError(%q) = %v, want %v", tc.msg, got, tc.locatorOK)
		}
	}
}
