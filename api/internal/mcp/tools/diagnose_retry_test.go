package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// TestDiagnoseFailure_RetryAttempts is the end-to-end proof that the retry
// signals are alive. triage could always classify a retry sequence, but
// nothing fed it the attempts, so every test read "single" and
// same_status_across_retries could never be true. Attempts are fetched only
// for a retried test (the common case costs no query), a fetch failure costs
// the signal and nothing else, and each attempt message is capped at 500.
func TestDiagnoseFailure_RetryAttempts(t *testing.T) {
	attempt := func(i int, status, msg string) store.TestAttemptRow {
		return store.TestAttemptRow{AttemptIndex: i, Status: status, StatusMessage: msg}
	}
	long := strings.Repeat("y", 3000)
	const auth500 = "POST /api/Auth failed with status 500"

	tests := []struct {
		name           string
		retries        int
		attempts       []store.TestAttemptRow
		attemptsErr    error
		primary        string // the failed step's message the status pattern anchors on
		want           string
		wantCalls      int
		wantEmitted    int
		wantMsgLen     int   // when set, every emitted message has this many runes
		wantSameStatus *bool // same_status_across_retries, when a pattern is expected
	}{
		{name: "attempts failing differently are varying", retries: 1,
			attempts: []store.TestAttemptRow{attempt(0, "failed", "expected 200, got 500"), attempt(1, "timedOut", "Test timeout of 30000ms exceeded")},
			want:     triage.RetryVarying, wantCalls: 1, wantEmitted: 2},
		{name: "attempts failing identically are consistent", retries: 1,
			attempts: []store.TestAttemptRow{attempt(0, "failed", "connection refused"), attempt(1, "failed", "connection refused")},
			want:     triage.RetryConsistent, wantCalls: 1, wantEmitted: 2},
		{name: "same message but a different status is still varying", retries: 1,
			attempts: []store.TestAttemptRow{attempt(0, "failed", ""), attempt(1, "passed", "")},
			want:     triage.RetryVarying, wantCalls: 1, wantEmitted: 2},
		{name: "a test never retried stays single without querying", retries: 0,
			attempts: []store.TestAttemptRow{attempt(0, "failed", "never read"), attempt(1, "failed", "never read")},
			want:     triage.RetrySingle},
		{name: "attempt fetch failure is non-fatal", retries: 3, attemptsErr: context.DeadlineExceeded,
			want: triage.RetrySingle, wantCalls: 1},
		{name: "attempt messages are capped", retries: 1,
			attempts: []store.TestAttemptRow{attempt(0, "failed", long), attempt(1, "failed", long)},
			want:     triage.RetryConsistent, wantCalls: 1, wantEmitted: 2, wantMsgLen: 500},
		// The HTTP status parsed from each attempt is compared, so the pattern
		// holds even when the surrounding text differs.
		{name: "every attempt hit the same status", retries: 1, primary: auth500,
			attempts: []store.TestAttemptRow{attempt(0, "failed", auth500), attempt(1, "failed", "request rejected, status: 500")},
			want:     triage.RetryVarying, wantCalls: 1, wantEmitted: 2, wantSameStatus: new(true)},
		{name: "a diverging status is not a repeated pattern", retries: 1, primary: auth500,
			attempts: []store.TestAttemptRow{attempt(0, "failed", auth500), attempt(1, "failed", "POST /api/Auth failed with status 503")},
			want:     triage.RetryVarying, wantCalls: 1, wantEmitted: 2, wantSameStatus: new(false)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := failed("h1")
			row.Retries = tc.retries
			f := newDiagFixture(t, row)
			f.TestResults.GetFailedStepPathFn = func(context.Context, int64, int64, string) ([]string, string, error) {
				return []string{"Before Hooks", "login"}, tc.primary, nil
			}
			calls := 0
			f.TestResults.GetAttemptsFn = func(context.Context, int64, int64, string) ([]store.TestAttemptRow, error) {
				calls++
				return tc.attempts, tc.attemptsErr
			}
			out, _ := f.run(t, nil)
			d := out.FailingTests[0]

			if d.Signals.RetryConsistency != tc.want || calls != tc.wantCalls || d.Retries != tc.retries {
				t.Errorf("retry_consistency %q after %d attempt queries (retries %d), want %q after %d (retries %d)",
					d.Signals.RetryConsistency, calls, d.Retries, tc.want, tc.wantCalls, tc.retries)
			}
			if len(d.RetryAttempts) != tc.wantEmitted {
				t.Fatalf("retry_attempts = %+v, want %d entries", d.RetryAttempts, tc.wantEmitted)
			}
			for i, got := range d.RetryAttempts {
				wantMsg := tc.attempts[i].StatusMessage
				if tc.wantMsgLen > 0 {
					wantMsg = string([]rune(wantMsg)[:tc.wantMsgLen])
				}
				if got.AttemptIndex != tc.attempts[i].AttemptIndex || got.Status != tc.attempts[i].Status || got.ErrorMessage != wantMsg {
					t.Errorf("retry_attempts[%d] = %+v, want %+v (message %d runes)", i, got, tc.attempts[i], len([]rune(wantMsg)))
				}
			}
			if tc.wantSameStatus != nil {
				p := d.Signals.RepeatedStatusPattern
				if p == nil || p.StatusCode != 500 || p.SameStatusAcrossRetries != *tc.wantSameStatus {
					t.Errorf("repeated_status_pattern = %+v, want 500 with same_status_across_retries=%v", p, *tc.wantSameStatus)
				}
			}
		})
	}
}
