package tools

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/runner"
)

// ---------------------------------------------------------------------------
// Failure clustering for diagnose_failure.
//
// A build with eight failures often has two root causes: four tests that never
// reached their body because one API answered 500 in beforeEach, and three that
// tripped over the same assertion. Reported as a flat list those eight entries
// read as eight problems and cost eight times the tokens to say the same thing
// twice. Clustering groups them by the shape of the failure, keeps the full
// text on one representative per group, and lets the build verdict reason over
// groups rather than rows.
// ---------------------------------------------------------------------------

// clusterNoMessageKey stands in for an absent error message in the cluster key,
// so tests with no message group together instead of each forming a cluster of
// one keyed on the empty string. It is a key only — it never reaches the
// output.
const clusterNoMessageKey = "(no error message)"

// clusterKnownIssueMatchCap bounds how many regex matches are reported per
// cluster; a project with a large rule set should not turn one cluster into a
// wall of matches.
const clusterKnownIssueMatchCap = 5

// ClusterStatusPattern is the HTTP status pattern every member of a cluster
// agrees on. It deliberately omits triage's same_status_across_retries: that
// is a per-test property of one test's retry attempts (surfaced per test as
// signals.retry_consistency / retry_attempts), not something a cluster of
// several tests can agree on, so promoting it to the cluster would imply
// agreement the data never established.
type ClusterStatusPattern struct {
	StatusCode int    `json:"status_code"`
	Endpoint   string `json:"endpoint,omitempty"`
}

// KnownIssueRegexMatch is a known-issue rule whose pattern matched a cluster's
// shared error text. It is NOT the same thing as DiagnoseTest.KnownIssue: that
// one is the human-confirmed link recorded on the defect fingerprint, this one
// is a regex that happens to match right now and has been confirmed by nobody.
type KnownIssueRegexMatch struct {
	KnownIssueID     int64  `json:"known_issue_id"`
	Name             string `json:"name"`
	MatchedSubstring string `json:"matched_substring"`
}

// DiagnoseCluster is a group of failing tests that share a failure shape, and
// therefore most likely one root cause.
type DiagnoseCluster struct {
	// ClusterID is a per-call label ("c1", "c2", …) assigned after sorting, so
	// c1 is always the largest cluster. It is not stable across calls.
	ClusterID string `json:"cluster_id"`
	// MemberCount is how many failing tests fell into this cluster.
	MemberCount int `json:"member_count"`
	// MemberFullNames lists every member, in the order they were diagnosed.
	MemberFullNames []string `json:"member_full_names"`
	// RepresentativeFullName is the member that kept its full error text and
	// failed-step path; the others were stripped of both.
	RepresentativeFullName string `json:"representative_full_name"`
	// SharedError is the representative's error message. Members share a
	// normalized form of it — the raw text is kept because that is what a
	// reader (and a known-issue regex) actually needs.
	SharedError string `json:"shared_error,omitempty"`
	// FailurePhase is the lifecycle phase shared by every member.
	FailurePhase string `json:"failure_phase,omitempty"`
	// SharedStatusPattern is reported only when every member's error yields the
	// same status code and endpoint.
	SharedStatusPattern *ClusterStatusPattern `json:"shared_status_pattern,omitempty"`
	// KnownIssueRegexMatches lists the project's active known-issue rules whose
	// pattern matches SharedError.
	KnownIssueRegexMatches []KnownIssueRegexMatch `json:"known_issue_regex_matches,omitempty"`
}

// clusterFailingTests groups tests by failure shape and returns the clusters
// plus, aligned with them, the indices of each cluster's members in tests.
//
// It mutates tests: every entry is tagged with its ClusterID, and every
// NON-representative member has its error_message and failed_step_path cleared.
// That is the token cut the clustering exists for — the text is identical to
// the representative's by construction, so repeating it once per member buys
// nothing. Everything else on a member (signals, fingerprint, attachments,
// last-good) survives untouched.
//
// Clusters are sorted by member count descending, ties keeping first-seen
// order, and the ids are assigned after the sort so c1 is the dominant cluster.
func clusterFailingTests(tests []DiagnoseTest) ([]DiagnoseCluster, [][]int) {
	if len(tests) == 0 {
		return nil, nil
	}

	slotOf := make(map[string]int, len(tests))
	groups := make([][]int, 0, len(tests))
	for i := range tests {
		key := clusterKey(&tests[i])
		slot, seen := slotOf[key]
		if !seen {
			slotOf[key] = len(groups)
			groups = append(groups, []int{i})
			continue
		}
		groups[slot] = append(groups[slot], i)
	}

	slices.SortStableFunc(groups, func(a, b []int) int { return len(b) - len(a) })

	clusters := make([]DiagnoseCluster, 0, len(groups))
	for gi, group := range groups {
		id := fmt.Sprintf("c%d", gi+1)
		rep := &tests[group[0]]
		c := DiagnoseCluster{
			ClusterID:              id,
			MemberCount:            len(group),
			MemberFullNames:        make([]string, 0, len(group)),
			RepresentativeFullName: rep.FullName,
			SharedError:            rep.ErrorMessage,
			FailurePhase:           rep.Signals.FailurePhase,
			SharedStatusPattern:    sharedStatusPattern(tests, group),
		}
		for n, idx := range group {
			tests[idx].ClusterID = id
			c.MemberFullNames = append(c.MemberFullNames, tests[idx].FullName)
			if n == 0 {
				continue // the representative keeps its text
			}
			tests[idx].ErrorMessage = ""
			tests[idx].FailedStepPath = nil
		}
		clusters = append(clusters, c)
	}
	return clusters, groups
}

// clusterKey is the failure shape two tests must share to be one root cause:
// the same error message once dynamic values are normalized away, the same
// lifecycle phase, and the same deepest failed step. The deepest step alone is
// used rather than the whole path because the outer steps differ per test while
// the step that actually blew up does not.
func clusterKey(d *DiagnoseTest) string {
	msg := runner.NormalizeMessage(strings.TrimSpace(d.ErrorMessage))
	if strings.TrimSpace(msg) == "" {
		msg = clusterNoMessageKey
	}
	var lastStep string
	if n := len(d.FailedStepPath); n > 0 {
		lastStep = d.FailedStepPath[n-1]
	}
	return msg + "\x00" + d.Signals.FailurePhase + "\x00" + lastStep
}

// sharedStatusPattern returns the status pattern common to every member of the
// group, or nil when any member lacks one or disagrees. A pattern only some of
// the members show is not shared, and reporting it as such would invent
// evidence the verdict rules then act on.
func sharedStatusPattern(tests []DiagnoseTest, group []int) *ClusterStatusPattern {
	first := tests[group[0]].Signals.RepeatedStatusPattern
	if first == nil {
		return nil
	}
	for _, idx := range group[1:] {
		p := tests[idx].Signals.RepeatedStatusPattern
		if p == nil || p.StatusCode != first.StatusCode || p.Endpoint != first.Endpoint {
			return nil
		}
	}
	return &ClusterStatusPattern{StatusCode: first.StatusCode, Endpoint: first.Endpoint}
}

// knownIssuePattern is one compiled known-issue rule.
type knownIssuePattern struct {
	id   int64
	name string
	re   *regexp.Regexp
}

// knownIssueMatcher holds a project's active known-issue rules, compiled once
// per diagnose_failure call and reused across every cluster. Compiling them per
// cluster (or worse, per test) would repeat the same work for no benefit.
type knownIssueMatcher struct {
	patterns []knownIssuePattern
}

// newKnownIssueMatcher loads and compiles the project's active known-issue
// rules. Every failure mode is soft: an unavailable store or an uncompilable
// pattern costs the matches it would have produced, never the diagnosis.
func newKnownIssueMatcher(ctx context.Context, stores *bootstrap.Stores, logger *zap.Logger, projectID int64) *knownIssueMatcher {
	m := &knownIssueMatcher{}
	if stores == nil || stores.KnownIssue == nil {
		return m
	}
	issues, err := stores.KnownIssue.List(ctx, projectID, true)
	if err != nil {
		logger.Warn("diagnose_failure: known-issue list failed",
			zap.Int64("project_id", projectID),
			zap.Error(err))
		return m
	}
	for i := range issues {
		re, err := regexp.Compile(issues[i].Pattern)
		if err != nil {
			// Fail-soft: a rule with a broken pattern is skipped, exactly as
			// match_known_issues skips it.
			logger.Warn("diagnose_failure: skipping known issue with an uncompilable pattern",
				zap.Int64("known_issue_id", issues[i].ID),
				zap.Error(err))
			continue
		}
		m.patterns = append(m.patterns, knownIssuePattern{id: issues[i].ID, name: issues[i].TestName, re: re})
	}
	return m
}

// match returns the known-issue rules whose pattern matches msg, capped at
// clusterKnownIssueMatchCap.
func (m *knownIssueMatcher) match(msg string) []KnownIssueRegexMatch {
	if m == nil || len(m.patterns) == 0 || msg == "" {
		return nil
	}
	var out []KnownIssueRegexMatch
	for _, p := range m.patterns {
		loc := p.re.FindStringIndex(msg)
		if loc == nil {
			continue
		}
		out = append(out, KnownIssueRegexMatch{
			KnownIssueID:     p.id,
			Name:             p.name,
			MatchedSubstring: msg[loc[0]:loc[1]],
		})
		if len(out) == clusterKnownIssueMatchCap {
			break
		}
	}
	return out
}
