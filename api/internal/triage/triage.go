// Package triage computes objective "triage signals" about a failed CI test.
//
// It is pure logic: callers populate plain input structs and receive a
// [Signals] struct in return. The package deliberately imports nothing from
// the runner, parser, store, or mcp packages so it stays trivially testable
// and free of side effects.
package triage

import (
	"regexp"
	"strconv"
	"strings"
)

// Build status constants. Callers populate [BuildHistoryEntry.Status] and
// [Input.PreviousBuildStatus] with these values (or any other string; only
// StatusPassed is treated specially when counting builds since the last pass).
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusBroken  = "broken"
	StatusSkipped = "skipped"
	StatusUnknown = "unknown"
)

// Failure phase values reported in [Signals.FailurePhase].
const (
	PhaseBeforeHooks  = "before_hooks"
	PhaseSetupFixture = "setup_fixture"
	PhaseTestBody     = "test_body"
	PhaseAfterHooks   = "after_hooks"
)

// Retry consistency values reported in [Signals.RetryConsistency].
const (
	RetrySingle     = "single"
	RetryConsistent = "consistent"
	RetryVarying    = "varying"
)

// Category hint constants.
const (
	CategoryToInvestigate = "to_investigate"
	// CategoryInfrastructure is the only category triage will assert on its
	// own, and only from the signal combination in [computeCategoryHint].
	CategoryInfrastructure = "infrastructure"
	confidenceLow          = "low"
	confidenceMedium       = "medium"
	sourceHeuristic        = "heuristic"
	// sourceSignals marks a hint derived from the signals computed in this same
	// call rather than passed through from the caller's defect record.
	sourceSignals = "signals"
)

// Status-code bounds. infraStatusFloor is the lowest code that counts as a
// server-side failure, both when ranking extracted codes and when deciding the
// category-hint upgrade; clientErrorFloor is its 4xx equivalent. Codes outside
// [statusCodeFloor, statusCodeCeiling] are not HTTP statuses at all.
const (
	statusCodeFloor   = 100
	clientErrorFloor  = 400
	infraStatusFloor  = 500
	statusCodeCeiling = 599
)

// fastFailRatioThreshold is the maximum duration ratio (failing / last passing)
// below which a failure is considered a fast-fail anomaly.
const fastFailRatioThreshold = 0.2

// fastFailAbsoluteMaxMs is the maximum absolute failing duration (in
// milliseconds) for a failure to qualify as a fast-fail. A failure that took a
// small fraction of a long passing run but still ran for, say, 30s is not a
// "fast" fail in any meaningful sense.
const fastFailAbsoluteMaxMs int64 = 5000

// RetryAttempt describes a single retry attempt of a failed test.
//
// ErrorKey is an opaque fingerprint identifying the failure (for example a
// hash of the normalized error message). When the caller has no precomputed
// fingerprint it may pass the raw error message instead; the triage package
// only compares attempts for equality, it never interprets the value.
type RetryAttempt struct {
	// ErrorKey is the fingerprint / error key for this attempt.
	ErrorKey string
	// ErrorMessage is the raw error message for this attempt, used for the
	// repeated-status-pattern detection across retries.
	ErrorMessage string
}

// BuildHistoryEntry describes one entry in a test's recent build history.
type BuildHistoryEntry struct {
	// Status is the test result status for this build (see the Status* constants).
	Status string
	// DurationMs is the test's execution time in this build, in milliseconds.
	DurationMs int64
}

// Input carries everything the triage package needs to compute [Signals].
// The caller (the diagnose_failure MCP tool) populates it from the defect
// record, the failed test, and its build history.
type Input struct {
	// DurationMs is the failed test's execution time, in milliseconds.
	DurationMs int64

	// ErrorMessage is the failed test's error message.
	ErrorMessage string

	// FailedStepPath is the ordered list of step names from the root step to
	// the deepest failed step. May be empty when no step information exists.
	FailedStepPath []string

	// RetryAttempts holds every retry attempt of the test, in attempt order.
	// May be empty when the test was not retried.
	RetryAttempts []RetryAttempt

	// BuildHistory holds the test's recent build history, ordered from most
	// recent to oldest. The first entry, when present, is the build that
	// produced this failure or the one immediately preceding it depending on
	// the caller's convention; PreviousBuildStatus disambiguates "last status".
	BuildHistory []BuildHistoryEntry

	// PreviousBuildStatus is the status of the build immediately preceding the
	// current (failing) one. Empty when there is no prior build.
	PreviousBuildStatus string

	// Category is the category string from the existing defect record
	// (for example "infrastructure", "test_bug", "product_bug",
	// "to_investigate"). Empty when the defect has no category.
	Category string
}

// FastFailSignal reports whether a failure aborted in a tiny fraction of the
// most recent passing run's duration.
type FastFailSignal struct {
	// FastFail is true when the failure both ran for a small absolute time and
	// took less than fastFailRatioThreshold of the last passing run.
	FastFail bool `json:"fast_fail"`
	// DurationMs is the failing test's duration, in milliseconds.
	DurationMs int64 `json:"duration_ms"`
	// LastPassingDurationMs is the duration of the most recent passing build,
	// in milliseconds. Nil when there is no prior passing build.
	LastPassingDurationMs *int64 `json:"last_passing_duration_ms"`
	// DurationRatio is DurationMs / LastPassingDurationMs. Nil when there is no
	// prior passing build to compare against.
	DurationRatio *float64 `json:"duration_ratio"`
}

// StatusPattern reports an HTTP-status-like pattern extracted from the error
// message.
type StatusPattern struct {
	// StatusCode is the HTTP status code parsed from the error message.
	StatusCode int `json:"status_code"`
	// Endpoint is the URL or path associated with the status, when one was
	// found in the error message. Empty when none was detected.
	Endpoint string `json:"endpoint,omitempty"`
	// SameStatusAcrossRetries is true when every retry attempt's error message
	// yields the same status code. False when retries diverge or there are
	// fewer than two attempts to compare.
	SameStatusAcrossRetries bool `json:"same_status_across_retries"`
}

// CategoryHint carries a defect category. Normally it is a passthrough of the
// caller-supplied category annotated with low confidence; when the caller has
// no category beyond the "to_investigate" default and the signals computed in
// this same call supply one, triage fills in its own value instead (see
// [computeCategoryHint]).
type CategoryHint struct {
	// Value is the category string (defaults to "to_investigate" when the
	// caller supplies an empty category).
	Value string `json:"value"`
	// Confidence is "low" for a passthrough and "medium" for a signal-derived
	// value. It is never "high": this is a hint, not a verdict.
	Confidence string `json:"confidence"`
	// Source is "heuristic" for a passthrough of the caller's category, or
	// "signals" when the value was derived from this call's own signals.
	Source string `json:"source"`
}

// Signals is the objective triage output computed from an [Input].
type Signals struct {
	// FastFail describes the fast-fail duration anomaly.
	FastFail FastFailSignal `json:"fast_fail"`

	// FailurePhase is the lifecycle phase the failure occurred in: one of
	// before_hooks, setup_fixture, test_body, after_hooks.
	FailurePhase string `json:"failure_phase"`

	// RetryConsistency is "single", "consistent", or "varying".
	RetryConsistency string `json:"retry_consistency"`

	// RepeatedStatusPattern holds the HTTP status pattern extracted from the
	// error message. Nil when the error message contains no status pattern.
	RepeatedStatusPattern *StatusPattern `json:"repeated_status_pattern"`

	// LastStatus is the status of the previous build for this test. Empty when
	// there is no previous build.
	LastStatus string `json:"last_status"`

	// BuildsSincePass is the number of consecutive recent builds since the last
	// passing build, counting from the most recent entry of BuildHistory. When
	// no passing build appears in the history it equals len(BuildHistory).
	BuildsSincePass int `json:"builds_since_pass"`

	// CategoryHint wraps the caller-supplied defect category.
	CategoryHint CategoryHint `json:"category_hint"`
}

// statusRe matches an HTTP-status mention such as "status 404", "status: 503"
// or "HTTP 500" (case-insensitive). Group 1 is the three-digit code, group 2 a
// trailing duration unit when the "code" is really a timing.
//
// The anchor set, the digit-free gap, the mandatory non-alphanumeric separator
// and the duration unit deliberately mirror runner.re5xx. The two are applied
// to the same error text — runner stamps the persisted defect category, this
// package feeds the live category hint — so a divergence between them shows up
// as the stored category and the hint disagreeing about one error. triage
// imports nothing from runner, so the pairing is held by
// TestStatusCodeAgreesWithFingerprintCategory rather than by shared code. The
// one intentional difference: runner also anchors on a standalone "code", which
// only ever yields a 5xx there, while here it would read "exit code 137" as an
// HTTP status.
//
// The anchors carry a leading word boundary only, so "statusCode: 500" and
// "status_code=503" still match while "decode"/"encoded" do not. The code needs
// a trailing boundary of its own: without one, "status 5000ms elapsed" reported
// 500 and "status: 8080 listener died" reported 808.
var statusRe = regexp.MustCompile(
	`(?i)\b(?:status|http|response|responded|returned)` + // anchor word
		`[^0-9\n]{0,40}[^0-9A-Za-z\n]` + // digit-free gap, then a separator
		`(\d{3})\b` + // the status code itself
		`(?:\s?(ms|msec|msecs|millis|milliseconds|s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b)?`)

// endpointRe matches a URL or an absolute path so the offending endpoint can be
// surfaced alongside the status code.
var endpointRe = regexp.MustCompile(`https?://[^\s"'` + "`" + `)]+|(?:^|[\s"'` + "`" + `(])(/[A-Za-z0-9._~%/\-]+)`)

// Analyze computes [Signals] from the given [Input]. It never returns an error
// and is safe to call with zero-valued or partially populated input.
func Analyze(in Input) Signals {
	s := Signals{
		FastFail:              computeFastFail(in),
		FailurePhase:          computeFailurePhase(in.FailedStepPath),
		RetryConsistency:      computeRetryConsistency(in.RetryAttempts),
		RepeatedStatusPattern: computeStatusPattern(in.ErrorMessage, in.RetryAttempts),
		LastStatus:            in.PreviousBuildStatus,
		BuildsSincePass:       computeBuildsSincePass(in.BuildHistory),
	}
	// The category hint reads the signals computed above, so it is filled in
	// last rather than inline in the literal.
	s.CategoryHint = computeCategoryHint(in.Category, s)
	return s
}

// computeFastFail detects the anomaly where a failure aborts in a tiny fraction
// of the most recent passing run's duration. With no prior passing build the
// ratio and last-passing fields are nil and FastFail is false.
func computeFastFail(in Input) FastFailSignal {
	sig := FastFailSignal{DurationMs: in.DurationMs}

	lastPass, ok := lastPassingDuration(in.BuildHistory)
	if !ok {
		return sig
	}
	passing := lastPass
	sig.LastPassingDurationMs = &passing

	// A non-positive passing duration cannot yield a meaningful ratio.
	if passing <= 0 {
		return sig
	}

	ratio := float64(in.DurationMs) / float64(passing)
	sig.DurationRatio = &ratio

	sig.FastFail = ratio < fastFailRatioThreshold && in.DurationMs <= fastFailAbsoluteMaxMs
	return sig
}

// lastPassingDuration returns the duration of the most recent passing build in
// the history (which is ordered most-recent-first). The bool is false when no
// passing build exists.
func lastPassingDuration(history []BuildHistoryEntry) (int64, bool) {
	for _, e := range history {
		if e.Status == StatusPassed {
			return e.DurationMs, true
		}
	}
	return 0, false
}

// computeFailurePhase derives the lifecycle phase from the failed-step path.
// It picks the outermost (closest to the root) meaningful phase marker so that,
// for example, a fixture failure deep inside "Before Hooks" still reports the
// hook phase rather than test_body.
func computeFailurePhase(path []string) string {
	for _, step := range path {
		lower := strings.ToLower(strings.TrimSpace(step))
		switch {
		case strings.Contains(lower, "after hooks"):
			return PhaseAfterHooks
		case strings.Contains(lower, "before hooks"):
			return PhaseBeforeHooks
		case strings.Contains(lower, "fixture:") || strings.Contains(lower, "setup"):
			return PhaseSetupFixture
		}
	}
	return PhaseTestBody
}

// computeRetryConsistency classifies the retry attempts by their error keys.
// Zero or one attempt is "single"; all-equal keys are "consistent"; any
// divergence is "varying".
func computeRetryConsistency(attempts []RetryAttempt) string {
	if len(attempts) < 2 {
		return RetrySingle
	}
	first := attempts[0].ErrorKey
	for _, a := range attempts[1:] {
		if a.ErrorKey != first {
			return RetryVarying
		}
	}
	return RetryConsistent
}

// computeStatusPattern extracts an HTTP status code (and optional endpoint)
// from the primary error message and reports whether every retry attempt
// shares the same status. It returns nil when the primary message contains no
// status pattern.
func computeStatusPattern(errMsg string, attempts []RetryAttempt) *StatusPattern {
	code, ok := extractStatusCode(errMsg)
	if !ok {
		return nil
	}

	pattern := &StatusPattern{
		StatusCode: code,
		Endpoint:   extractEndpoint(errMsg),
	}

	// Determine whether all retry attempts yield the same status code.
	if len(attempts) >= 2 {
		same := true
		for _, a := range attempts {
			ac, aok := extractStatusCode(a.ErrorMessage)
			if !aok || ac != code {
				same = false
				break
			}
		}
		pattern.SameStatusAcrossRetries = same
	}

	return pattern
}

// extractStatusCode picks the most diagnostic HTTP status mentioned in msg.
//
// Every mention is scanned rather than just the first, because the first is
// routinely the expectation rather than the failure: "Expected status 200 but
// got status 500" must report the 500 that actually came back. A server error
// therefore wins outright, then a client error, and only failing both does the
// first in-range code stand. Anything outside 100-599, and any code that is
// really a duration, is discarded.
func extractStatusCode(msg string) (int, bool) {
	var firstInRange, firstClientError int

	for _, m := range statusRe.FindAllStringSubmatch(msg, -1) {
		if m[2] != "" {
			continue // "503 ms" is a timing, not a status
		}
		code, err := strconv.Atoi(m[1])
		if err != nil || code < statusCodeFloor || code > statusCodeCeiling {
			continue
		}
		if code >= infraStatusFloor {
			return code, true
		}
		if firstClientError == 0 && code >= clientErrorFloor {
			firstClientError = code
		}
		if firstInRange == 0 {
			firstInRange = code
		}
	}

	if firstClientError != 0 {
		return firstClientError, true
	}
	if firstInRange != 0 {
		return firstInRange, true
	}
	return 0, false
}

// extractEndpoint pulls a URL or absolute path out of msg, if one is present.
func extractEndpoint(msg string) string {
	m := endpointRe.FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	// Group 1 holds the captured absolute path; when it is empty the match was
	// a full URL, which is m[0].
	if m[1] != "" {
		return m[1]
	}
	return strings.TrimSpace(m[0])
}

// computeBuildsSincePass counts consecutive builds since the last pass,
// scanning the most-recent-first history. When no pass is found it returns the
// full history length.
func computeBuildsSincePass(history []BuildHistoryEntry) int {
	for i, e := range history {
		if e.Status == StatusPassed {
			return i
		}
	}
	return len(history)
}

// computeCategoryHint wraps the caller-supplied category, defaulting an empty
// value to "to_investigate" — except when the caller has no real category to
// offer and the signals computed in this same call supply one.
//
// The one such combination is a fast abort in the before-hooks phase whose
// error message carries a 5xx status: the test never reached its own body, gave
// up almost immediately, and the thing it was talking to answered with a server
// error. Absent any other information that reads as an environment failure, so
// the value is filled in and marked source="signals" at "medium" confidence —
// never "high": this is a hint, and the build-level verdict is where a real
// judgement is made.
//
// A stored category other than "to_investigate" always wins, however loud the
// signals are. Input.Category comes from the defect row, where a non-default
// value means a human approved a classify proposal; overruling that with an
// inference — and at a higher confidence than the human value is ever surfaced
// with — would silently bury the human's answer. Such a passthrough keeps
// source="heuristic", the value this field has always carried for a
// caller-supplied category, so existing consumers are unaffected.
func computeCategoryHint(category string, s Signals) CategoryHint {
	value := strings.TrimSpace(category)
	if value == "" {
		value = CategoryToInvestigate
	}

	if value == CategoryToInvestigate && signalsSayInfrastructure(s) {
		return CategoryHint{
			Value:      CategoryInfrastructure,
			Confidence: confidenceMedium,
			Source:     sourceSignals,
		}
	}

	return CategoryHint{
		Value:      value,
		Confidence: confidenceLow,
		Source:     sourceHeuristic,
	}
}

// signalsSayInfrastructure reports whether the signals alone are strong enough
// to call a failure an environment problem.
func signalsSayInfrastructure(s Signals) bool {
	return s.FailurePhase == PhaseBeforeHooks && s.FastFail.FastFail &&
		s.RepeatedStatusPattern != nil && s.RepeatedStatusPattern.StatusCode >= infraStatusFloor
}
