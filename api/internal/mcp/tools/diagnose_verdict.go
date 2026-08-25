package tools

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// ---------------------------------------------------------------------------
// Build verdict for diagnose_failure.
//
// The per-test signals answer "what happened"; the verdict answers "what should
// I do about it". It is a deterministic function of the clustered failures —
// same input, same verdict — and it says out loud what it could not see, so a
// low-confidence answer is distinguishable from a confident one.
//
// Rules are evaluated per cluster in a fixed order, first match wins, and the
// build takes the verdict of its dominant (largest) cluster.
// ---------------------------------------------------------------------------

// Verdict values reported in BuildVerdict.Verdict.
const (
	VerdictProductRegression    = "product_regression"
	VerdictTestBug              = "test_bug"
	VerdictInfraEnv             = "infra_env"
	VerdictKnownIssue           = "known_issue"
	VerdictFlaky                = "flaky"
	VerdictInsufficientEvidence = "insufficient_evidence"
)

// Confidence values reported in BuildVerdict.Confidence.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Recommended actions reported in BuildVerdict.RecommendedAction.
const (
	ActionFileBug        = "file_bug"
	ActionFixTest        = "fix_test"
	ActionRerun          = "rerun"
	ActionLinkKnownIssue = "link_known_issue"
	ActionHumanNeeded    = "human_needed"
)

// Commit relations between a cluster's last-good build and the diagnosed build.
const (
	commitUnknown   = ""
	commitSame      = "same"
	commitDifferent = "different"
)

// verdictServerErrorFloor is the lowest status code that counts as a
// server-side failure for the infra_env rule.
const verdictServerErrorFloor = 500

// productRegressionMaxBuildsSincePass bounds how stale a failure may be and
// still read as a fresh regression. A test that has been failing for several
// builds is not evidence about the change under test.
const productRegressionMaxBuildsSincePass = 1

// verdictKnownIssueEvidenceCap bounds how many regex matches are quoted as
// evidence, so a project with many rules does not drown the verdict.
const verdictKnownIssueEvidenceCap = 3

// VerdictEvidence is one named observation the verdict rests on.
type VerdictEvidence struct {
	Signal string `json:"signal"`
	Value  string `json:"value"`
}

// BuildVerdict is the build-level judgement rolled up from the clusters.
type BuildVerdict struct {
	// Verdict is one of product_regression, test_bug, infra_env, known_issue,
	// flaky, insufficient_evidence.
	Verdict string `json:"verdict"`
	// Confidence is high, medium, or low. It is never high when the diagnosis
	// was truncated or when the clusters disagreed.
	Confidence string `json:"confidence"`
	// Evidence lists the observations that produced the verdict.
	Evidence []VerdictEvidence `json:"evidence"`
	// EvidenceGaps names what could not be checked, so a weak verdict is
	// distinguishable from a confident one.
	EvidenceGaps []string `json:"evidence_gaps,omitempty"`
	// RecommendedAction is the next step implied by the verdict.
	RecommendedAction string `json:"recommended_action"`
	// AffectedTestCount is the size of the dominant cluster — the number of
	// failing tests this verdict actually speaks for.
	AffectedTestCount int `json:"affected_test_count"`
}

// clusterVerdict is one cluster's judgement before roll-up.
type clusterVerdict struct {
	verdict    string
	confidence string
	evidence   []VerdictEvidence
	gaps       []string
}

// evaluateBuildVerdict judges every cluster and rolls the result up from the
// dominant one. clusters must be ordered largest-first (clusterFailingTests
// guarantees it) and members must be aligned with clusters.
func evaluateBuildVerdict(tests []DiagnoseTest, clusters []DiagnoseCluster, members [][]int, buildCommitSHA string, truncated bool) BuildVerdict {
	if len(clusters) == 0 {
		return BuildVerdict{
			Verdict:           VerdictInsufficientEvidence,
			Confidence:        ConfidenceLow,
			Evidence:          []VerdictEvidence{},
			EvidenceGaps:      []string{"no failing tests were examined"},
			RecommendedAction: ActionHumanNeeded,
		}
	}

	verdicts := make([]clusterVerdict, len(clusters))
	for i := range clusters {
		verdicts[i] = evaluateCluster(&clusters[i], memberTests(tests, members[i]), buildCommitSHA)
	}

	out := BuildVerdict{
		Verdict:           verdicts[0].verdict,
		Confidence:        verdicts[0].confidence,
		Evidence:          verdicts[0].evidence,
		EvidenceGaps:      verdicts[0].gaps,
		AffectedTestCount: clusters[0].MemberCount,
	}
	if out.Evidence == nil {
		out.Evidence = []VerdictEvidence{}
	}

	// Clusters that disagree mean the build has more than one story. Report the
	// dominant one, but say so and stop short of high confidence.
	if split := verdictSplit(verdicts); split != "" {
		out.Evidence = append(out.Evidence, VerdictEvidence{Signal: "cluster_verdict_split", Value: split})
		out.Confidence = capConfidence(out.Confidence)
	}

	// A truncated diagnosis has not seen the whole build, so it cannot be
	// confident about it whatever the examined tests say.
	if truncated {
		out.Confidence = capConfidence(out.Confidence)
		out.EvidenceGaps = append(out.EvidenceGaps, "diagnosis truncated: some failing tests were not examined")
	}

	out.RecommendedAction = recommendedAction(out.Verdict)
	return out
}

// memberTests projects a cluster's member indices back onto the diagnosed
// tests.
func memberTests(tests []DiagnoseTest, idx []int) []DiagnoseTest {
	out := make([]DiagnoseTest, 0, len(idx))
	for _, i := range idx {
		out = append(out, tests[i])
	}
	return out
}

// evaluateCluster runs the rule chain in priority order and returns the first
// match, falling back to insufficient_evidence.
func evaluateCluster(c *DiagnoseCluster, members []DiagnoseTest, buildCommitSHA string) clusterVerdict {
	rel, relSHA := lastGoodCommitRelation(members, buildCommitSHA)

	if v, ok := evalInfraEnv(c, members, rel, relSHA); ok {
		return v
	}
	if v, ok := evalKnownIssue(c, members); ok {
		return v
	}
	if v, ok := evalProductRegression(c, members, rel, relSHA); ok {
		return v
	}
	if v, ok := evalTestBug(c, rel, relSHA); ok {
		return v
	}
	if v, ok := evalFlaky(members); ok {
		return v
	}
	return evalInsufficientEvidence(c, rel)
}

// lastGoodCommitRelation reports whether the code changed between the cluster's
// last-good build and the diagnosed build, along with the last-good sha it
// compared. It returns commitUnknown when either side has no commit recorded —
// the caller must treat that as an evidence gap, not as "unchanged".
func lastGoodCommitRelation(members []DiagnoseTest, buildCommitSHA string) (relation, lastGoodSHA string) {
	if buildCommitSHA == "" {
		return commitUnknown, ""
	}
	for i := range members {
		lg := members[i].LastGood
		if lg == nil || lg.CommitSHA == "" {
			continue
		}
		if lg.CommitSHA == buildCommitSHA {
			return commitSame, lg.CommitSHA
		}
		return commitDifferent, lg.CommitSHA
	}
	return commitUnknown, ""
}

// evalInfraEnv: several tests died before reaching their body, all against the
// same endpoint answering 5xx, on code that has not changed since they last
// passed. That is the environment, not the build.
func evalInfraEnv(c *DiagnoseCluster, members []DiagnoseTest, rel, relSHA string) (clusterVerdict, bool) {
	if c.SharedStatusPattern == nil || c.SharedStatusPattern.StatusCode < verdictServerErrorFloor {
		return clusterVerdict{}, false
	}
	hooks := 0
	for i := range members {
		if members[i].Signals.FailurePhase == triage.PhaseBeforeHooks {
			hooks++
		}
	}
	if hooks < 2 {
		return clusterVerdict{}, false
	}
	// The code changed since these tests last passed, so the change itself is a
	// live suspect and this is not cleanly an environment failure.
	if rel == commitDifferent {
		return clusterVerdict{}, false
	}

	v := clusterVerdict{
		verdict: VerdictInfraEnv,
		evidence: []VerdictEvidence{
			{Signal: "failure_phase", Value: fmt.Sprintf("%s x%d", triage.PhaseBeforeHooks, hooks)},
			{Signal: "shared_status_pattern", Value: statusPatternValue(c.SharedStatusPattern)},
		},
	}
	if rel == commitSame {
		v.confidence = ConfidenceHigh
		v.evidence = append(v.evidence, VerdictEvidence{
			Signal: "last_good_commit", Value: "unchanged since the last pass (" + relSHA + ")"})
	} else {
		v.confidence = ConfidenceMedium
		v.gaps = append(v.gaps, "no last_good commit_sha to confirm the code is unchanged")
	}
	return v, true
}

// evalKnownIssue: somebody already knows about this. Either a human linked the
// defect fingerprint to a known issue, or an active known-issue rule matches
// the cluster's shared error.
func evalKnownIssue(c *DiagnoseCluster, members []DiagnoseTest) (clusterVerdict, bool) {
	var evidence []VerdictEvidence
	for i := range members {
		if ki := members[i].KnownIssue; ki != nil {
			evidence = append(evidence, VerdictEvidence{
				Signal: "known_issue_link",
				Value:  fmt.Sprintf("#%d %s confirmed on %s", ki.ID, ki.Name, members[i].FullName),
			})
			break
		}
	}
	for i, m := range c.KnownIssueRegexMatches {
		if i == verdictKnownIssueEvidenceCap {
			break
		}
		evidence = append(evidence, VerdictEvidence{
			Signal: "known_issue_regex",
			Value:  fmt.Sprintf("#%d %s matched %q", m.KnownIssueID, m.Name, m.MatchedSubstring),
		})
	}
	if len(evidence) == 0 {
		return clusterVerdict{}, false
	}
	return clusterVerdict{verdict: VerdictKnownIssue, confidence: ConfidenceHigh, evidence: evidence}, true
}

// evalProductRegression: the test reached its body, asserted, and the assertion
// failed — and it was passing a build or two ago on different code. The change
// is the suspect.
func evalProductRegression(c *DiagnoseCluster, members []DiagnoseTest, rel, relSHA string) (clusterVerdict, bool) {
	if c.FailurePhase != triage.PhaseTestBody || !isAssertionClassError(c.SharedError) {
		return clusterVerdict{}, false
	}
	worst := 0
	for i := range members {
		if n := members[i].Signals.BuildsSincePass; n > worst {
			worst = n
		}
	}
	if worst > productRegressionMaxBuildsSincePass {
		return clusterVerdict{}, false
	}
	// Same code, same assertion: whatever changed, it was not the commit, so
	// this is not a regression of the product.
	if rel == commitSame {
		return clusterVerdict{}, false
	}

	v := clusterVerdict{
		verdict: VerdictProductRegression,
		evidence: []VerdictEvidence{
			{Signal: "failure_phase", Value: triage.PhaseTestBody},
			{Signal: "error_class", Value: "assertion"},
			{Signal: "builds_since_pass", Value: strconv.Itoa(worst)},
		},
	}
	if rel == commitDifferent {
		v.confidence = ConfidenceHigh
		v.evidence = append(v.evidence, VerdictEvidence{
			Signal: "last_good_commit", Value: "changed since the last pass (" + relSHA + ")"})
	} else {
		v.confidence = ConfidenceMedium
		v.gaps = append(v.gaps, "no last_good commit_sha to confirm the code changed since the last pass")
	}
	return v, true
}

// evalTestBug: the test reached its body and then could not find or drive the
// thing it wanted, on code that has not changed since it last passed. The test
// is looking at the wrong thing.
func evalTestBug(c *DiagnoseCluster, rel, relSHA string) (clusterVerdict, bool) {
	if c.FailurePhase != triage.PhaseTestBody || !isLocatorClassError(c.SharedError) {
		return clusterVerdict{}, false
	}
	// The code moved under the test, so a broken selector may be the product's
	// doing rather than the test's.
	if rel == commitDifferent {
		return clusterVerdict{}, false
	}

	v := clusterVerdict{
		verdict:    VerdictTestBug,
		confidence: ConfidenceMedium,
		evidence: []VerdictEvidence{
			{Signal: "failure_phase", Value: triage.PhaseTestBody},
			{Signal: "error_class", Value: "locator"},
		},
	}
	if rel == commitSame {
		v.evidence = append(v.evidence, VerdictEvidence{
			Signal: "last_good_commit", Value: "unchanged since the last pass (" + relSHA + ")"})
	} else {
		v.gaps = append(v.gaps, "no last_good commit_sha to confirm the code is unchanged")
	}
	return v, true
}

// evalFlaky: either the store already flagged the test, or every member of the
// cluster passed in the immediately preceding build. The second test is
// deliberately strict — one member reverting is a regression, all of them
// reverting at once is a blip.
func evalFlaky(members []DiagnoseTest) (clusterVerdict, bool) {
	for i := range members {
		if members[i].Flaky {
			return clusterVerdict{
				verdict:    VerdictFlaky,
				confidence: ConfidenceMedium,
				evidence: []VerdictEvidence{
					{Signal: "flaky_flag", Value: "set on " + members[i].FullName},
				},
			}, true
		}
	}
	if len(members) == 0 {
		return clusterVerdict{}, false
	}
	for i := range members {
		if members[i].Signals.LastStatus != triage.StatusPassed || members[i].Signals.BuildsSincePass != 0 {
			return clusterVerdict{}, false
		}
	}
	return clusterVerdict{
		verdict:    VerdictFlaky,
		confidence: ConfidenceMedium,
		evidence: []VerdictEvidence{
			{Signal: "history_alternation",
				Value: fmt.Sprintf("all %d member(s) passed in the immediately preceding build", len(members))},
		},
	}, true
}

// evalInsufficientEvidence is the honest fallback: no rule matched, so say what
// was missing rather than guessing.
func evalInsufficientEvidence(c *DiagnoseCluster, rel string) clusterVerdict {
	v := clusterVerdict{
		verdict:    VerdictInsufficientEvidence,
		confidence: ConfidenceLow,
		evidence: []VerdictEvidence{
			{Signal: "failure_phase", Value: c.FailurePhase},
			{Signal: "member_count", Value: strconv.Itoa(c.MemberCount)},
		},
	}
	switch {
	case strings.TrimSpace(c.SharedError) == "":
		v.gaps = append(v.gaps, "no error message recorded for this cluster")
	case !isAssertionClassError(c.SharedError) && !isLocatorClassError(c.SharedError):
		v.gaps = append(v.gaps, "the shared error matches no assertion or locator class")
	}
	if c.SharedStatusPattern == nil {
		v.gaps = append(v.gaps, "no HTTP status pattern in the shared error")
	}
	if rel == commitUnknown {
		v.gaps = append(v.gaps, "no last_good commit_sha to compare against the build commit")
	}
	if len(v.gaps) == 0 {
		v.gaps = append(v.gaps, "no rule matched this cluster's signal combination")
	}
	return v
}

// verdictSplit summarizes disagreeing cluster verdicts as "infra_env x1,
// test_bug x2", in first-appearance order. It returns "" when the clusters
// agree or when there is only one.
func verdictSplit(verdicts []clusterVerdict) string {
	if len(verdicts) < 2 {
		return ""
	}
	order := make([]string, 0, len(verdicts))
	counts := make(map[string]int, len(verdicts))
	for _, v := range verdicts {
		if _, seen := counts[v.verdict]; !seen {
			order = append(order, v.verdict)
		}
		counts[v.verdict]++
	}
	if len(order) < 2 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, fmt.Sprintf("%s x%d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}

// capConfidence lowers high to medium and leaves everything else alone.
func capConfidence(c string) string {
	if c == ConfidenceHigh {
		return ConfidenceMedium
	}
	return c
}

// recommendedAction maps a verdict to the next step it implies.
func recommendedAction(verdict string) string {
	switch verdict {
	case VerdictInfraEnv:
		return ActionRerun
	case VerdictKnownIssue:
		return ActionLinkKnownIssue
	case VerdictProductRegression:
		return ActionFileBug
	case VerdictTestBug:
		return ActionFixTest
	default:
		return ActionHumanNeeded
	}
}

// statusPatternValue renders a shared status pattern for the evidence list.
func statusPatternValue(p *ClusterStatusPattern) string {
	if p.Endpoint == "" {
		return strconv.Itoa(p.StatusCode)
	}
	return fmt.Sprintf("%d at %s", p.StatusCode, p.Endpoint)
}

// assertionClassMarkers name an error where the test got far enough to compare
// something and the comparison failed.
var assertionClassMarkers = []string{
	"expect(",
	"expected",
	"assert",
	"tobe",
	"tocontain",
	"toequal",
	"tohave",
	"tomatch",
}

// locatorClassMarkers name an error where the test could not find or drive an
// element at all.
var locatorClassMarkers = []string{
	"locator",
	"selector",
	"getby",
	"queryselector",
	"element not found",
	"element is not visible",
	"element not interactable",
	"no element",
	"stale element",
	"nosuchelement",
	"waiting for element",
	"wait_for_selector",
}

// isAssertionClassError reports whether msg reads as a failed assertion.
func isAssertionClassError(msg string) bool {
	return matchesAnyMarker(msg, assertionClassMarkers)
}

// isLocatorClassError reports whether msg reads as an element that could not be
// found or driven. A Playwright auto-retrying assertion is deliberately both
// classes; the rule order decides which reading wins.
func isLocatorClassError(msg string) bool {
	return matchesAnyMarker(msg, locatorClassMarkers)
}

// matchesAnyMarker tests the normalized, lowercased message against a marker
// list. Normalization is shared with the cluster key so the classes are decided
// on exactly the text the clustering grouped on.
func matchesAnyMarker(msg string, markers []string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	norm := strings.ToLower(runner.NormalizeMessage(msg))
	for _, m := range markers {
		if strings.Contains(norm, m) {
			return true
		}
	}
	return false
}
