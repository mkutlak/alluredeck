package tools_test

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// diagnoseWithAttempts runs diagnose_failure over a single failing test whose
// stored retry count and attempt rows are the ones given, and returns the
// diagnosed test plus how many times the attempt store was consulted.
func diagnoseWithAttempts(t *testing.T, retries int, attempts []store.TestAttemptRow) (tools.DiagnoseTest, int) {
	t.Helper()

	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Retried",
				Status: "failed", DurationMs: 900, Retries: retries},
		}, nil
	}
	attemptCalls := 0
	mocks.TestResults.GetAttemptsFn = func(_ context.Context, _ int64, _ int64, _ string) ([]store.TestAttemptRow, error) {
		attemptCalls++
		return attempts, nil
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("failing_tests: got %d, want 1", len(out.FailingTests))
	}
	return out.FailingTests[0], attemptCalls
}

// TestDiagnoseFailure_RetryConsistency is the end-to-end proof that the
// retry-consistency signal is alive. triage.Analyze has always been able to
// classify a retry sequence, but nothing populated Input.RetryAttempts, so the
// tool reported "single" for every test no matter how often it was retried.
func TestDiagnoseFailure_RetryConsistency(t *testing.T) {
	tests := []struct {
		name        string
		retries     int
		attempts    []store.TestAttemptRow
		want        string
		wantEmitted int
	}{
		{
			name:    "two attempts failing differently are varying",
			retries: 1,
			attempts: []store.TestAttemptRow{
				{AttemptIndex: 0, Status: "failed", StatusMessage: "expected 200, got 500"},
				{AttemptIndex: 1, Status: "timedOut", StatusMessage: "Test timeout of 30000ms exceeded"},
			},
			want:        triage.RetryVarying,
			wantEmitted: 2,
		},
		{
			name:    "two attempts failing identically are consistent",
			retries: 1,
			attempts: []store.TestAttemptRow{
				{AttemptIndex: 0, Status: "failed", StatusMessage: "connection refused"},
				{AttemptIndex: 1, Status: "failed", StatusMessage: "connection refused"},
			},
			want:        triage.RetryConsistent,
			wantEmitted: 2,
		},
		{
			name:    "same message but a different status is still varying",
			retries: 1,
			attempts: []store.TestAttemptRow{
				{AttemptIndex: 0, Status: "failed", StatusMessage: ""},
				{AttemptIndex: 1, Status: "passed", StatusMessage: ""},
			},
			want:        triage.RetryVarying,
			wantEmitted: 2,
		},
		{
			name:        "a test that was never retried stays single",
			retries:     0,
			attempts:    nil,
			want:        triage.RetrySingle,
			wantEmitted: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := diagnoseWithAttempts(t, tc.retries, tc.attempts)
			if d.Signals.RetryConsistency != tc.want {
				t.Errorf("signals.retry_consistency: got %q, want %q", d.Signals.RetryConsistency, tc.want)
			}
			if len(d.RetryAttempts) != tc.wantEmitted {
				t.Fatalf("retry_attempts: got %d entries (%+v), want %d", len(d.RetryAttempts), d.RetryAttempts, tc.wantEmitted)
			}
			for i, got := range d.RetryAttempts {
				want := tc.attempts[i]
				if got.AttemptIndex != want.AttemptIndex || got.Status != want.Status || got.ErrorMessage != want.StatusMessage {
					t.Errorf("retry_attempts[%d]: got %+v, want index=%d status=%q message=%q",
						i, got, want.AttemptIndex, want.Status, want.StatusMessage)
				}
			}
		})
	}
}

// TestDiagnoseFailure_SameStatusAcrossRetries covers the second signal the dead
// RetryAttempts input starved: repeated_status_pattern.same_status_across_retries
// can only ever be true once the attempts reach triage, and it is the difference
// between "the endpoint 500s every single time" and "one attempt happened to
// 500". It compares the HTTP status parsed out of each attempt's message, so it
// stays true even when the surrounding text differs.
func TestDiagnoseFailure_SameStatusAcrossRetries(t *testing.T) {
	tests := []struct {
		name     string
		attempts []store.TestAttemptRow
		want     bool
	}{
		{
			name: "every attempt hit the same status",
			attempts: []store.TestAttemptRow{
				{AttemptIndex: 0, Status: "failed", StatusMessage: "POST /api/Auth failed with status 500"},
				{AttemptIndex: 1, Status: "failed", StatusMessage: "request rejected, status: 500"},
			},
			want: true,
		},
		{
			name: "a diverging status is not a repeated pattern",
			attempts: []store.TestAttemptRow{
				{AttemptIndex: 0, Status: "failed", StatusMessage: "POST /api/Auth failed with status 500"},
				{AttemptIndex: 1, Status: "failed", StatusMessage: "POST /api/Auth failed with status 503"},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

			mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
				return []store.TestResult{
					{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Auth",
						Status: "failed", DurationMs: 900, Retries: 1},
				}, nil
			}
			// The primary error message is what the pattern is anchored on; the
			// attempts decide whether it repeated.
			mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
				return []string{"Before Hooks", "login"}, "POST /api/Auth failed with status 500", nil
			}
			mocks.TestResults.GetAttemptsFn = func(_ context.Context, _ int64, _ int64, _ string) ([]store.TestAttemptRow, error) {
				return tc.attempts, nil
			}

			cs := setupTestServer(t, buildStoresDiagnose(mocks))
			res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
				Name:      "diagnose_failure",
				Arguments: map[string]any{"project_id": projectID, "build_id": 100},
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %v", res.Content)
			}
			out := decodeDiagnoseFailure(t, res)
			if len(out.FailingTests) != 1 {
				t.Fatalf("failing_tests: got %d, want 1", len(out.FailingTests))
			}
			pattern := out.FailingTests[0].Signals.RepeatedStatusPattern
			if pattern == nil {
				t.Fatal("signals.repeated_status_pattern: got nil, want a pattern for a status-500 message")
			}
			if pattern.StatusCode != 500 {
				t.Errorf("status_code: got %d, want 500", pattern.StatusCode)
			}
			if pattern.SameStatusAcrossRetries != tc.want {
				t.Errorf("same_status_across_retries: got %v, want %v", pattern.SameStatusAcrossRetries, tc.want)
			}
		})
	}
}

// TestDiagnoseFailure_NoRetries_SkipsAttemptQuery pins the cost gate: the
// overwhelming majority of failing tests were never retried, so the attempt
// lookup must not run for them.
func TestDiagnoseFailure_NoRetries_SkipsAttemptQuery(t *testing.T) {
	_, calls := diagnoseWithAttempts(t, 0, []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "never read"},
		{AttemptIndex: 1, Status: "failed", StatusMessage: "never read"},
	})
	if calls != 0 {
		t.Errorf("GetAttempts calls for a test with retries=0: got %d, want 0", calls)
	}
}

// TestDiagnoseFailure_AttemptFetchError_NonFatal verifies the fetch is
// best-effort: attempts are extra context, so losing them must cost the retry
// signal and nothing else.
func TestDiagnoseFailure_AttemptFetchError_NonFatal(t *testing.T) {
	mocks := testutil.New()
	projectID := seedDiagnoseProjectBuild(t, mocks, 100, 28)

	mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
		return []store.TestResult{
			{BuildID: 100, ProjectID: projectID, HistoryID: "h1", FullName: "pkg.Retried",
				Status: "failed", DurationMs: 900, Retries: 3},
		}, nil
	}
	mocks.TestResults.GetAttemptsFn = func(_ context.Context, _ int64, _ int64, _ string) ([]store.TestAttemptRow, error) {
		return nil, context.DeadlineExceeded
	}

	cs := setupTestServer(t, buildStoresDiagnose(mocks))
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "diagnose_failure",
		Arguments: map[string]any{"project_id": projectID, "build_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("an attempt-fetch failure must not fail the diagnosis: %v", res.Content)
	}
	out := decodeDiagnoseFailure(t, res)
	if len(out.FailingTests) != 1 {
		t.Fatalf("failing_tests: got %d, want 1", len(out.FailingTests))
	}
	d := out.FailingTests[0]
	if len(d.RetryAttempts) != 0 {
		t.Errorf("retry_attempts: got %+v, want none", d.RetryAttempts)
	}
	if d.Signals.RetryConsistency != triage.RetrySingle {
		t.Errorf("signals.retry_consistency: got %q, want %q when attempts are unavailable",
			d.Signals.RetryConsistency, triage.RetrySingle)
	}
	if d.Retries != 3 {
		t.Errorf("retries: got %d, want 3 — the stored count survives a failed attempt fetch", d.Retries)
	}
}

// TestDiagnoseFailure_AttemptMessageCapped verifies the output cap. The stored
// message is already capped at 2000 characters on write; the tool payload
// carries many attempts across many tests, so it caps again at 500.
func TestDiagnoseFailure_AttemptMessageCapped(t *testing.T) {
	long := strings.Repeat("y", 3000)
	d, _ := diagnoseWithAttempts(t, 1, []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: long},
		{AttemptIndex: 1, Status: "failed", StatusMessage: long},
	})
	if len(d.RetryAttempts) != 2 {
		t.Fatalf("retry_attempts: got %d, want 2", len(d.RetryAttempts))
	}
	if got := len([]rune(d.RetryAttempts[0].ErrorMessage)); got != 500 {
		t.Errorf("retry_attempts[0].error_message length: got %d, want 500", got)
	}
}

// TestDiagnoseFailure_DescriptionDocumentsRetryConsistency guards the honesty of
// the tool description. The retry-consistency mention was removed while the
// signal was dead; now that it is wired it must be advertised again, or agents
// will not know to read it.
func TestDiagnoseFailure_DescriptionDocumentsRetryConsistency(t *testing.T) {
	var desc string
	for _, tool := range listRegisteredTools(t) {
		if tool.Name == "diagnose_failure" {
			desc = tool.Description
			break
		}
	}
	if desc == "" {
		t.Fatal("diagnose_failure is not registered, or has no description")
	}
	for _, want := range []string{"retry_consistency", "retry_attempts"} {
		if !strings.Contains(desc, want) {
			t.Errorf("diagnose_failure description does not mention %q", want)
		}
	}
}
