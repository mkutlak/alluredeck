package triage_test

import (
	"encoding/json"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// deref makes optional signal fields comparable: nil stays nil.
func deref[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestComputeFastFail(t *testing.T) {
	passed := func(ms int64) triage.BuildHistoryEntry {
		return triage.BuildHistoryEntry{Status: triage.StatusPassed, DurationMs: ms}
	}
	failed := func(ms int64) triage.BuildHistoryEntry {
		return triage.BuildHistoryEntry{Status: triage.StatusFailed, DurationMs: ms}
	}
	tests := []struct {
		name      string
		duration  int64
		history   []triage.BuildHistoryEntry // most recent first
		wantFast  bool
		wantLast  *int64
		wantRatio *float64
	}{
		{"fast fail with prior pass", 200, []triage.BuildHistoryEntry{failed(200), passed(4000)}, true, new(int64(4000)), new(0.05)},
		{"no prior passing build", 200, []triage.BuildHistoryEntry{failed(200), {Status: triage.StatusBroken, DurationMs: 300}}, false, nil, nil},
		{"empty build history", 500, nil, false, nil, nil},
		{"low ratio but over 5s absolute is not fast", 9000, []triage.BuildHistoryEntry{failed(9000), passed(60000)}, false, new(int64(60000)), new(0.15)},
		{"small absolute but ratio above threshold is not fast", 3000, []triage.BuildHistoryEntry{failed(3000), passed(4000)}, false, new(int64(4000)), new(0.75)},
		{"zero passing duration yields no ratio", 100, []triage.BuildHistoryEntry{passed(0)}, false, new(int64(0)), nil},
		{"uses most recent passing build", 100, []triage.BuildHistoryEntry{failed(100), passed(2000), passed(9999)}, true, new(int64(2000)), new(0.05)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{DurationMs: tt.duration, BuildHistory: tt.history}).FastFail
			if got.FastFail != tt.wantFast || got.DurationMs != tt.duration ||
				deref(got.LastPassingDurationMs) != deref(tt.wantLast) || deref(got.DurationRatio) != deref(tt.wantRatio) {
				t.Errorf("FastFail = {%v, %d, last %v, ratio %v}, want {%v, %d, last %v, ratio %v}",
					got.FastFail, got.DurationMs, deref(got.LastPassingDurationMs), deref(got.DurationRatio),
					tt.wantFast, tt.duration, deref(tt.wantLast), deref(tt.wantRatio))
			}
		})
	}
}

func TestComputeFailurePhase(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want string
	}{
		{"empty path defaults to test body", nil, triage.PhaseTestBody},
		{"before hooks", []string{"Before Hooks", "login user"}, triage.PhaseBeforeHooks},
		{"after hooks", []string{"After Hooks", "teardown db"}, triage.PhaseAfterHooks},
		{"fixture prefix is setup", []string{"fixture: browser context", "navigate"}, triage.PhaseSetupFixture},
		{"setup keyword is setup", []string{"setup environment", "do thing"}, triage.PhaseSetupFixture},
		{"plain test body", []string{"open page", "click button", "assert title"}, triage.PhaseTestBody},
		{"outermost marker wins over inner fixture", []string{"Before Hooks", "fixture: db", "seed data"}, triage.PhaseBeforeHooks},
		{"case insensitive matching", []string{"BEFORE HOOKS"}, triage.PhaseBeforeHooks},
	}
	for _, tt := range tests {
		if got := triage.Analyze(triage.Input{FailedStepPath: tt.path}).FailurePhase; got != tt.want {
			t.Errorf("%s: FailurePhase = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestComputeRetryConsistency(t *testing.T) {
	keys := func(ks ...string) []triage.RetryAttempt {
		out := make([]triage.RetryAttempt, 0, len(ks))
		for _, k := range ks {
			out = append(out, triage.RetryAttempt{ErrorKey: k})
		}
		return out
	}
	tests := []struct {
		name     string
		attempts []triage.RetryAttempt
		want     string
	}{
		{"no attempts is single", nil, triage.RetrySingle},
		{"one attempt is single", keys("abc"), triage.RetrySingle},
		{"all same key is consistent", keys("timeout", "timeout", "timeout"), triage.RetryConsistent},
		{"differing keys are varying", keys("timeout", "assertion"), triage.RetryVarying},
		{"last attempt differs is varying", keys("timeout", "timeout", "network"), triage.RetryVarying},
	}
	for _, tt := range tests {
		if got := triage.Analyze(triage.Input{RetryAttempts: tt.attempts}).RetryConsistency; got != tt.want {
			t.Errorf("%s: RetryConsistency = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestComputeStatusPattern covers extraction of the HTTP status (and endpoint)
// from the error message, whether retries repeat it, and the anchoring and
// selection rules. First-match-wins used to hand back the expectation rather
// than the failure ("Expected status 200 but got status 500" -> 200) and an
// unbounded three-digit match used to slice a code out of a longer number
// ("status 5000ms elapsed" -> 500, "status: 8080 listener died" -> 808).
func TestComputeStatusPattern(t *testing.T) {
	msgs := func(ms ...string) []triage.RetryAttempt {
		out := make([]triage.RetryAttempt, 0, len(ms))
		for _, m := range ms {
			out = append(out, triage.RetryAttempt{ErrorMessage: m})
		}
		return out
	}
	tests := []struct {
		name         string
		errMsg       string
		attempts     []triage.RetryAttempt
		wantCode     int // 0 = no pattern
		wantEndpoint string
		wantSame     bool
	}{
		{name: "no status pattern returns nil", errMsg: "element not found: timed out waiting for selector"},
		{name: "simple status match", errMsg: "request failed with status 503", wantCode: 503},
		{name: "status with colon and code word", errMsg: "HTTP status code: 404 received", wantCode: 404},
		{name: "status with url endpoint", errMsg: "GET https://api.example.com/v1/users returned status 500", wantCode: 500, wantEndpoint: "https://api.example.com/v1/users"},
		{name: "status with absolute path endpoint", errMsg: "call to /api/orders failed: status 502", wantCode: 502, wantEndpoint: "/api/orders"},
		{name: "same status across retries", errMsg: "status 500", attempts: msgs("status 500", "got status 500 again"), wantCode: 500, wantSame: true},
		{name: "varying status across retries", errMsg: "status 500", attempts: msgs("status 500", "status 503"), wantCode: 500},
		{name: "retry without status breaks sameness", errMsg: "status 500", attempts: msgs("status 500", "connection reset"), wantCode: 500},
		{name: "single retry leaves same flag false", errMsg: "status 500", attempts: msgs("status 500"), wantCode: 500},
		{name: "prefers the served 5xx over the expected 2xx", errMsg: "Expected status 200 but got status 500", wantCode: 500},
		{name: "prefers a 5xx regardless of position", errMsg: "returned 404 on retry, then status 503", wantCode: 503},
		{name: "prefers a 4xx over other classes", errMsg: "status 301 redirect, then status 404 missing", wantCode: 404},
		{name: "falls back to the first code in range", errMsg: "status 301 redirect", wantCode: 301},
		{name: "duration is not sliced into a status", errMsg: "status 5000ms elapsed"},
		{name: "port number is not sliced into a status", errMsg: "status: 8080 listener died"},
		{name: "a code followed by a time unit is a duration", errMsg: "status polling: 503 ms since the last probe"},
		{name: "out-of-range codes are rejected", errMsg: "status 099 is not a status"},
		{name: "exit codes are not HTTP statuses", errMsg: "process died with exit code 137"},
		{name: "http anchor without the word status", errMsg: "HTTP 503 Service Unavailable", wantCode: 503},
		{name: "responded anchor", errMsg: "gateway responded with 502", wantCode: 502},
		{name: "status prose with a distant code", errMsg: "status: Internal Server Error, transaction 502 aborted", wantCode: 502},
		{name: "camelCase statusCode still anchors", errMsg: "API call failed with statusCode: 500", wantCode: 500},
		{name: "snake_case status_code still anchors", errMsg: "request rejected, status_code=503", wantCode: 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{ErrorMessage: tt.errMsg, RetryAttempts: tt.attempts}).RepeatedStatusPattern
			if tt.wantCode == 0 {
				if got != nil {
					t.Fatalf("RepeatedStatusPattern = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("RepeatedStatusPattern = nil, want non-nil")
			}
			if got.StatusCode != tt.wantCode || got.Endpoint != tt.wantEndpoint || got.SameStatusAcrossRetries != tt.wantSame {
				t.Errorf("RepeatedStatusPattern = %+v, want {StatusCode: %d, Endpoint: %q, SameStatusAcrossRetries: %v}",
					got, tt.wantCode, tt.wantEndpoint, tt.wantSame)
			}
		})
	}
}

// TestStatusCodeAgreesWithFingerprintCategory pairs this package's status
// extraction with runner.CategorizeError, which stamps the persisted defect
// category on the same text. The two must never contradict each other: a
// message the fingerprinter calls infrastructure must yield a 5xx here, and a
// message it does not must not yield one, otherwise the stored category and the
// live hint disagree about the same error.
//
// triage deliberately imports nothing from runner, so the expected categories
// are copied. Keep them in sync with the 5xx anchoring rows of
// TestCategorizeError in internal/runner/fingerprint_test.go.
func TestStatusCodeAgreesWithFingerprintCategory(t *testing.T) {
	const categoryInfrastructure = "infrastructure"

	tests := []struct {
		msg string
		// runnerCategory is what runner.CategorizeError returns for msg.
		runnerCategory string
	}{
		{"test failed after 500 ms", "to_investigate"},
		{"response time was 503 ms, over the 200ms budget", "to_investigate"},
		{"failed to decode 512 bytes", "to_investigate"},
		{"encoded 550 rows", "to_investigate"},
		{"status: Internal Server Error, transaction 502 aborted", categoryInfrastructure},
		{"Expected status 200, received 503", "product_bug"},
		{"Timed out 5000ms waiting for locator", "to_investigate"},
		{"HTTP 503 Service Unavailable", categoryInfrastructure},
	}
	for _, tt := range tests {
		got := triage.Analyze(triage.Input{ErrorMessage: tt.msg}).RepeatedStatusPattern
		if is5xx := got != nil && got.StatusCode >= 500; is5xx != (tt.runnerCategory == categoryInfrastructure) {
			t.Errorf("%q: RepeatedStatusPattern = %+v, but the fingerprinter calls it %q", tt.msg, got, tt.runnerCategory)
		}
	}
}

func TestComputeBuildsSincePass(t *testing.T) {
	statuses := func(ss ...string) []triage.BuildHistoryEntry {
		out := make([]triage.BuildHistoryEntry, 0, len(ss))
		for _, s := range ss {
			out = append(out, triage.BuildHistoryEntry{Status: s})
		}
		return out
	}
	tests := []struct {
		name    string
		history []triage.BuildHistoryEntry // most recent first
		want    int
	}{
		{"empty history is zero", nil, 0},
		{"most recent build passed", statuses(triage.StatusPassed, triage.StatusFailed), 0},
		{"three failures since last pass", statuses(triage.StatusFailed, triage.StatusBroken, triage.StatusFailed, triage.StatusPassed), 3},
		{"never passed counts all builds", statuses(triage.StatusFailed, triage.StatusFailed), 2},
	}
	for _, tt := range tests {
		if got := triage.Analyze(triage.Input{BuildHistory: tt.history}).BuildsSincePass; got != tt.want {
			t.Errorf("%s: BuildsSincePass = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// TestCategoryHint covers the stored-category passthrough (trimmed, defaulting
// to to_investigate, always low confidence) and the one case where triage
// fills in a category of its own: a fast abort in the before-hooks phase whose
// error carries a 5xx is an environment failure. That upgrade applies only
// when there is no real category — a value other than the default got there
// because a human approved a classify proposal, and the signals must not
// replace that verdict, least of all at "medium" while the human value is only
// ever surfaced at "low".
func TestCategoryHint(t *testing.T) {
	beforeHooks := []string{"Before Hooks", "login via API"}
	fastHistory := []triage.BuildHistoryEntry{{Status: triage.StatusPassed, DurationMs: 5000}}
	signals := func(msg, category string) triage.Input {
		return triage.Input{DurationMs: 120, ErrorMessage: msg, FailedStepPath: beforeHooks, BuildHistory: fastHistory, Category: category}
	}
	const status503 = "API call failed with status 503. URL: /api/TokenAuth/Authenticate"
	low := func(v string) triage.CategoryHint {
		return triage.CategoryHint{Value: v, Confidence: "low", Source: "heuristic"}
	}
	upgraded := triage.CategoryHint{Value: triage.CategoryInfrastructure, Confidence: "medium", Source: "signals"}

	slow := signals("status 500 from /api/TokenAuth/Authenticate", "")
	slow.DurationMs = 4800
	testBody := signals("status 500 from /api/orders", "")
	testBody.FailedStepPath = []string{"Test Body", "click save"}
	noPriorPass := signals("status 500 from /api/TokenAuth/Authenticate", "")
	noPriorPass.BuildHistory = nil
	status599 := signals("status 599 from gateway", "")
	status599.DurationMs = 50

	tests := []struct {
		name string
		in   triage.Input
		want triage.CategoryHint
	}{
		{"empty defaults to to_investigate", triage.Input{}, low(triage.CategoryToInvestigate)},
		{"whitespace defaults to to_investigate", triage.Input{Category: "   "}, low(triage.CategoryToInvestigate)},
		{"trims and passes through", triage.Input{Category: "  test_bug  "}, low("test_bug")},

		{"before_hooks + fast_fail + 500 upgrades", signals("API call failed with status 500. URL: /api/TokenAuth/Authenticate", "to_investigate"), upgraded},
		{"empty category leaves room for the upgrade", signals(status503, ""), upgraded},
		{"padded to_investigate is still the default", signals(status503, "  to_investigate  "), upgraded},
		{"599 is still 5xx", status599, upgraded},

		{"human product_bug outranks the signals", signals(status503, "product_bug"), low("product_bug")},
		{"human test_bug outranks the signals", signals(status503, "test_bug"), low("test_bug")},
		{"human infrastructure stays a passthrough", signals(status503, "infrastructure"), low("infrastructure")},

		{"4xx does not upgrade", signals("status 404 from /api/users", ""), low(triage.CategoryToInvestigate)},
		{"no status pattern does not upgrade", signals("socket hang up", ""), low(triage.CategoryToInvestigate)},
		{"slow before-hooks failure does not upgrade", slow, low(triage.CategoryToInvestigate)},
		{"test_body phase does not upgrade", testBody, low(triage.CategoryToInvestigate)},
		{"no prior pass means no fast_fail and no upgrade", noPriorPass, low(triage.CategoryToInvestigate)},
	}
	for _, tt := range tests {
		if got := triage.Analyze(tt.in).CategoryHint; got != tt.want {
			t.Errorf("%s: CategoryHint = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// TestSignalsJSONTags verifies the output serializes with the snake_case keys
// the MCP layer exposes, carrying the previous build status through.
func TestSignalsJSONTags(t *testing.T) {
	in := triage.Input{
		DurationMs:          100,
		ErrorMessage:        "request failed with status 503",
		BuildHistory:        []triage.BuildHistoryEntry{{Status: triage.StatusPassed, DurationMs: 4000}},
		PreviousBuildStatus: triage.StatusFailed,
		Category:            "infrastructure",
	}
	data, err := json.Marshal(triage.Analyze(in))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{
		"fast_fail", "failure_phase", "retry_consistency",
		"repeated_status_pattern", "last_status", "builds_since_pass", "category_hint",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q in %s", key, data)
		}
	}
	if got := string(m["last_status"]); got != `"`+triage.StatusFailed+`"` {
		t.Errorf("last_status = %s, want the previous build status %q", got, triage.StatusFailed)
	}
}
