package triage_test

import (
	"encoding/json"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/triage"
)

func TestComputeFastFail(t *testing.T) {
	tests := []struct {
		name           string
		in             triage.Input
		wantFastFail   bool
		wantDuration   int64
		wantLastPass   *int64
		wantRatio      *float64
		ratioTolerance float64
	}{
		{
			name: "fast fail with prior pass",
			in: triage.Input{
				DurationMs: 200,
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusFailed, DurationMs: 200},
					{Status: triage.StatusPassed, DurationMs: 4000},
				},
			},
			wantFastFail: true,
			wantDuration: 200,
			wantLastPass: new(int64(4000)),
			wantRatio:    new(0.05),
		},
		{
			name: "no prior passing build",
			in: triage.Input{
				DurationMs: 200,
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusFailed, DurationMs: 200},
					{Status: triage.StatusBroken, DurationMs: 300},
				},
			},
			wantFastFail: false,
			wantDuration: 200,
			wantLastPass: nil,
			wantRatio:    nil,
		},
		{
			name: "empty build history",
			in: triage.Input{
				DurationMs:   500,
				BuildHistory: nil,
			},
			wantFastFail: false,
			wantDuration: 500,
			wantLastPass: nil,
			wantRatio:    nil,
		},
		{
			name: "low ratio but large absolute duration is not fast fail",
			in: triage.Input{
				DurationMs: 9000, // < 0.2 ratio but > 5s absolute
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusFailed, DurationMs: 9000},
					{Status: triage.StatusPassed, DurationMs: 60000},
				},
			},
			wantFastFail: false,
			wantDuration: 9000,
			wantLastPass: new(int64(60000)),
			wantRatio:    new(0.15),
		},
		{
			name: "small absolute but ratio above threshold is not fast fail",
			in: triage.Input{
				DurationMs: 3000,
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusFailed, DurationMs: 3000},
					{Status: triage.StatusPassed, DurationMs: 4000},
				},
			},
			wantFastFail: false,
			wantDuration: 3000,
			wantLastPass: new(int64(4000)),
			wantRatio:    new(0.75),
		},
		{
			name: "zero passing duration yields no ratio",
			in: triage.Input{
				DurationMs: 100,
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusPassed, DurationMs: 0},
				},
			},
			wantFastFail: false,
			wantDuration: 100,
			wantLastPass: new(int64(0)),
			wantRatio:    nil,
		},
		{
			name: "uses most recent passing build",
			in: triage.Input{
				DurationMs: 100,
				BuildHistory: []triage.BuildHistoryEntry{
					{Status: triage.StatusFailed, DurationMs: 100},
					{Status: triage.StatusPassed, DurationMs: 2000},
					{Status: triage.StatusPassed, DurationMs: 9999},
				},
			},
			wantFastFail: true,
			wantDuration: 100,
			wantLastPass: new(int64(2000)),
			wantRatio:    new(0.05),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(tt.in).FastFail

			if got.FastFail != tt.wantFastFail {
				t.Errorf("FastFail = %v, want %v", got.FastFail, tt.wantFastFail)
			}
			if got.DurationMs != tt.wantDuration {
				t.Errorf("DurationMs = %d, want %d", got.DurationMs, tt.wantDuration)
			}
			assertI64Ptr(t, "LastPassingDurationMs", got.LastPassingDurationMs, tt.wantLastPass)
			assertF64Ptr(t, "DurationRatio", got.DurationRatio, tt.wantRatio)
		})
	}
}

func TestComputeFailurePhase(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want string
	}{
		{
			name: "empty path defaults to test body",
			path: nil,
			want: triage.PhaseTestBody,
		},
		{
			name: "before hooks",
			path: []string{"Before Hooks", "login user"},
			want: triage.PhaseBeforeHooks,
		},
		{
			name: "after hooks",
			path: []string{"After Hooks", "teardown db"},
			want: triage.PhaseAfterHooks,
		},
		{
			name: "fixture prefix is setup",
			path: []string{"fixture: browser context", "navigate"},
			want: triage.PhaseSetupFixture,
		},
		{
			name: "setup keyword is setup",
			path: []string{"setup environment", "do thing"},
			want: triage.PhaseSetupFixture,
		},
		{
			name: "plain test body",
			path: []string{"open page", "click button", "assert title"},
			want: triage.PhaseTestBody,
		},
		{
			name: "outermost marker wins over inner fixture",
			path: []string{"Before Hooks", "fixture: db", "seed data"},
			want: triage.PhaseBeforeHooks,
		},
		{
			name: "case insensitive matching",
			path: []string{"BEFORE HOOKS"},
			want: triage.PhaseBeforeHooks,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{FailedStepPath: tt.path}).FailurePhase
			if got != tt.want {
				t.Errorf("FailurePhase = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeRetryConsistency(t *testing.T) {
	tests := []struct {
		name     string
		attempts []triage.RetryAttempt
		want     string
	}{
		{
			name:     "no attempts is single",
			attempts: nil,
			want:     triage.RetrySingle,
		},
		{
			name:     "one attempt is single",
			attempts: []triage.RetryAttempt{{ErrorKey: "abc"}},
			want:     triage.RetrySingle,
		},
		{
			name: "all same key is consistent",
			attempts: []triage.RetryAttempt{
				{ErrorKey: "timeout"},
				{ErrorKey: "timeout"},
				{ErrorKey: "timeout"},
			},
			want: triage.RetryConsistent,
		},
		{
			name: "differing keys are varying",
			attempts: []triage.RetryAttempt{
				{ErrorKey: "timeout"},
				{ErrorKey: "assertion"},
			},
			want: triage.RetryVarying,
		},
		{
			name: "last attempt differs is varying",
			attempts: []triage.RetryAttempt{
				{ErrorKey: "timeout"},
				{ErrorKey: "timeout"},
				{ErrorKey: "network"},
			},
			want: triage.RetryVarying,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{RetryAttempts: tt.attempts}).RetryConsistency
			if got != tt.want {
				t.Errorf("RetryConsistency = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeStatusPattern(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		attempts     []triage.RetryAttempt
		wantNil      bool
		wantCode     int
		wantEndpoint string
		wantSame     bool
	}{
		{
			name:    "no status pattern returns nil",
			errMsg:  "element not found: timed out waiting for selector",
			wantNil: true,
		},
		{
			name:     "simple status match",
			errMsg:   "request failed with status 503",
			wantNil:  false,
			wantCode: 503,
		},
		{
			name:     "status with colon and code word",
			errMsg:   "HTTP status code: 404 received",
			wantNil:  false,
			wantCode: 404,
		},
		{
			name:         "status with url endpoint",
			errMsg:       "GET https://api.example.com/v1/users returned status 500",
			wantNil:      false,
			wantCode:     500,
			wantEndpoint: "https://api.example.com/v1/users",
		},
		{
			name:         "status with absolute path endpoint",
			errMsg:       "call to /api/orders failed: status 502",
			wantNil:      false,
			wantCode:     502,
			wantEndpoint: "/api/orders",
		},
		{
			name:   "same status across retries",
			errMsg: "status 500",
			attempts: []triage.RetryAttempt{
				{ErrorMessage: "status 500"},
				{ErrorMessage: "got status 500 again"},
			},
			wantNil:  false,
			wantCode: 500,
			wantSame: true,
		},
		{
			name:   "varying status across retries",
			errMsg: "status 500",
			attempts: []triage.RetryAttempt{
				{ErrorMessage: "status 500"},
				{ErrorMessage: "status 503"},
			},
			wantNil:  false,
			wantCode: 500,
			wantSame: false,
		},
		{
			name:   "retry without status breaks sameness",
			errMsg: "status 500",
			attempts: []triage.RetryAttempt{
				{ErrorMessage: "status 500"},
				{ErrorMessage: "connection reset"},
			},
			wantNil:  false,
			wantCode: 500,
			wantSame: false,
		},
		{
			name:     "single retry leaves same flag false",
			errMsg:   "status 500",
			attempts: []triage.RetryAttempt{{ErrorMessage: "status 500"}},
			wantNil:  false,
			wantCode: 500,
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{
				ErrorMessage:  tt.errMsg,
				RetryAttempts: tt.attempts,
			}).RepeatedStatusPattern

			if tt.wantNil {
				if got != nil {
					t.Fatalf("RepeatedStatusPattern = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("RepeatedStatusPattern = nil, want non-nil")
			}
			if got.StatusCode != tt.wantCode {
				t.Errorf("StatusCode = %d, want %d", got.StatusCode, tt.wantCode)
			}
			if got.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %q, want %q", got.Endpoint, tt.wantEndpoint)
			}
			if got.SameStatusAcrossRetries != tt.wantSame {
				t.Errorf("SameStatusAcrossRetries = %v, want %v", got.SameStatusAcrossRetries, tt.wantSame)
			}
		})
	}
}

func TestComputeBuildsSincePass(t *testing.T) {
	tests := []struct {
		name    string
		history []triage.BuildHistoryEntry
		want    int
	}{
		{
			name:    "empty history is zero",
			history: nil,
			want:    0,
		},
		{
			name: "most recent build passed",
			history: []triage.BuildHistoryEntry{
				{Status: triage.StatusPassed},
				{Status: triage.StatusFailed},
			},
			want: 0,
		},
		{
			name: "three failures since last pass",
			history: []triage.BuildHistoryEntry{
				{Status: triage.StatusFailed},
				{Status: triage.StatusBroken},
				{Status: triage.StatusFailed},
				{Status: triage.StatusPassed},
			},
			want: 3,
		},
		{
			name: "never passed counts all builds",
			history: []triage.BuildHistoryEntry{
				{Status: triage.StatusFailed},
				{Status: triage.StatusFailed},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{BuildHistory: tt.history}).BuildsSincePass
			if got != tt.want {
				t.Errorf("BuildsSincePass = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLastStatus(t *testing.T) {
	tests := []struct {
		name string
		prev string
		want string
	}{
		{name: "previous failed", prev: triage.StatusFailed, want: triage.StatusFailed},
		{name: "previous passed", prev: triage.StatusPassed, want: triage.StatusPassed},
		{name: "no previous build", prev: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{PreviousBuildStatus: tt.prev}).LastStatus
			if got != tt.want {
				t.Errorf("LastStatus = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeCategoryHint(t *testing.T) {
	tests := []struct {
		name      string
		category  string
		wantValue string
	}{
		{name: "infrastructure passthrough", category: "infrastructure", wantValue: "infrastructure"},
		{name: "test_bug passthrough", category: "test_bug", wantValue: "test_bug"},
		{name: "product_bug passthrough", category: "product_bug", wantValue: "product_bug"},
		{name: "empty defaults to to_investigate", category: "", wantValue: triage.CategoryToInvestigate},
		{name: "whitespace defaults to to_investigate", category: "   ", wantValue: triage.CategoryToInvestigate},
		{name: "trims surrounding whitespace", category: "  test_bug  ", wantValue: "test_bug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triage.Analyze(triage.Input{Category: tt.category}).CategoryHint
			if got.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", got.Value, tt.wantValue)
			}
			if got.Confidence != "low" {
				t.Errorf("Confidence = %q, want %q", got.Confidence, "low")
			}
			if got.Source != "heuristic" {
				t.Errorf("Source = %q, want %q", got.Source, "heuristic")
			}
		})
	}
}

// TestAnalyzeZeroInput verifies Analyze never panics and yields sane defaults
// for a completely empty input.
func TestAnalyzeZeroInput(t *testing.T) {
	got := triage.Analyze(triage.Input{})

	if got.FastFail.FastFail {
		t.Error("FastFail should be false for zero input")
	}
	if got.FailurePhase != triage.PhaseTestBody {
		t.Errorf("FailurePhase = %q, want %q", got.FailurePhase, triage.PhaseTestBody)
	}
	if got.RetryConsistency != triage.RetrySingle {
		t.Errorf("RetryConsistency = %q, want %q", got.RetryConsistency, triage.RetrySingle)
	}
	if got.RepeatedStatusPattern != nil {
		t.Errorf("RepeatedStatusPattern = %+v, want nil", got.RepeatedStatusPattern)
	}
	if got.BuildsSincePass != 0 {
		t.Errorf("BuildsSincePass = %d, want 0", got.BuildsSincePass)
	}
	if got.CategoryHint.Value != triage.CategoryToInvestigate {
		t.Errorf("CategoryHint.Value = %q, want %q", got.CategoryHint.Value, triage.CategoryToInvestigate)
	}
}

// TestSignalsJSONTags verifies the output serializes with the expected
// snake_case keys for the MCP layer.
func TestSignalsJSONTags(t *testing.T) {
	in := triage.Input{
		DurationMs:   100,
		ErrorMessage: "request failed with status 503",
		BuildHistory: []triage.BuildHistoryEntry{
			{Status: triage.StatusPassed, DurationMs: 4000},
		},
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
}

func assertI64Ptr(t *testing.T, name string, got, want *int64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", name, fmtI64(got), fmtI64(want))
	case *got != *want:
		t.Errorf("%s = %d, want %d", name, *got, *want)
	}
}

func assertF64Ptr(t *testing.T, name string, got, want *float64) {
	t.Helper()
	const epsilon = 1e-9
	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", name, fmtF64(got), fmtF64(want))
	case *got-*want > epsilon || *want-*got > epsilon:
		t.Errorf("%s = %g, want %g", name, *got, *want)
	}
}

func fmtI64(p *int64) any {
	if p == nil {
		return "nil"
	}
	return *p
}

func fmtF64(p *float64) any {
	if p == nil {
		return "nil"
	}
	return *p
}
