package parser_test

import (
	"encoding/json"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// pwAttemptsReport builds a one-test Playwright report.json whose test carries
// the given result attempts. Each attempt is (status, errors...).
func pwAttemptsReport(attempts []map[string]any) []byte {
	reportData := map[string]any{
		"metadata":  map[string]any{},
		"startTime": float64(1700000000000),
		"duration":  float64(3000),
		"stats":     map[string]any{"total": 1, "unexpected": 1},
		"files": []any{
			map[string]any{
				"fileId":   "file1",
				"fileName": "tests/retry.spec.ts",
				"tests": []any{
					map[string]any{
						"testId":      "t-retry",
						"title":       "retried test",
						"projectName": "chromium",
						"outcome":     "unexpected",
						"path":        []any{"Retry"},
						"duration":    float64(3000),
						"tags":        []any{},
						"ok":          false,
						"results":     toAnySlice(attempts),
					},
				},
			},
		},
	}
	b, err := json.Marshal(reportData)
	if err != nil {
		panic(err)
	}
	return b
}

func toAnySlice(in []map[string]any) []any {
	out := make([]any, 0, len(in))
	for _, m := range in {
		out = append(out, m)
	}
	return out
}

// TestParsePlaywrightReport_Attempts pins the per-attempt capture: the parser
// must record EVERY entry of test.results, not just the last one, because the
// last-attempt-only shape is what left triage's retry-consistency signal
// permanently reporting "single".
func TestParsePlaywrightReport_Attempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		attempts []map[string]any
		want     []parser.Attempt
	}{
		{
			name: "two attempts, different errors",
			attempts: []map[string]any{
				{
					"startTime": "2023-11-14T12:00:00Z", "duration": float64(1000),
					"retry": 0, "steps": []any{}, "status": "failed",
					"errors": []any{"expected 200, got 500"}, "attachments": []any{},
				},
				{
					"startTime": "2023-11-14T12:00:02Z", "duration": float64(1000),
					"retry": 1, "steps": []any{}, "status": "timedOut",
					"errors": []any{"Test timeout of 30000ms exceeded"}, "attachments": []any{},
				},
			},
			want: []parser.Attempt{
				{Index: 0, Status: "failed", StatusMessage: "expected 200, got 500"},
				{Index: 1, Status: "timedOut", StatusMessage: "Test timeout of 30000ms exceeded"},
			},
		},
		{
			name: "multiple errors on one attempt are joined",
			attempts: []map[string]any{
				{
					"startTime": "2023-11-14T12:00:00Z", "duration": float64(500),
					"retry": 0, "steps": []any{}, "status": "failed",
					"errors":      []any{"first error", map[string]any{"message": "second error"}},
					"attachments": []any{},
				},
			},
			want: []parser.Attempt{
				{Index: 0, Status: "failed", StatusMessage: "first error\nsecond error"},
			},
		},
		{
			name: "a passing retry carries an empty message",
			attempts: []map[string]any{
				{
					"startTime": "2023-11-14T12:00:00Z", "duration": float64(500),
					"retry": 0, "steps": []any{}, "status": "failed",
					"errors": []any{"boom"}, "attachments": []any{},
				},
				{
					"startTime": "2023-11-14T12:00:01Z", "duration": float64(500),
					"retry": 1, "steps": []any{}, "status": "passed",
					"errors": []any{}, "attachments": []any{},
				},
			},
			want: []parser.Attempt{
				{Index: 0, Status: "failed", StatusMessage: "boom"},
				{Index: 1, Status: "passed", StatusMessage: ""},
			},
		},
		{
			name: "a single attempt still records one entry",
			attempts: []map[string]any{
				{
					"startTime": "2023-11-14T12:00:00Z", "duration": float64(500),
					"retry": 0, "steps": []any{}, "status": "failed",
					"errors": []any{"only failure"}, "attachments": []any{},
				},
			},
			want: []parser.Attempt{
				{Index: 0, Status: "failed", StatusMessage: "only failure"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results, _, err := parser.ParsePlaywrightReport(pwAttemptsReport(tc.attempts), nil)
			if err != nil {
				t.Fatalf("ParsePlaywrightReport: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("results count: got %d, want 1", len(results))
			}
			got := results[0].Attempts
			if len(got) != len(tc.want) {
				t.Fatalf("attempts count: got %d, want %d (%+v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("attempt %d: got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParsePlaywrightReport_NoResults_NoAttempts verifies a test with no result
// entries (e.g. skipped before it ever ran) yields no attempts rather than a
// one-element slice of zero values.
func TestParsePlaywrightReport_NoResults_NoAttempts(t *testing.T) {
	t.Parallel()

	results, _, err := parser.ParsePlaywrightReport(pwAttemptsReport(nil), nil)
	if err != nil {
		t.Fatalf("ParsePlaywrightReport: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results count: got %d, want 1", len(results))
	}
	if len(results[0].Attempts) != 0 {
		t.Errorf("attempts: got %+v, want empty", results[0].Attempts)
	}
}
