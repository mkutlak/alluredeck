package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

type originsCall struct {
	group  int64
	runKey string
	limit  int
}

// originsRow is one failing test of project 10.
func originsRow(fullName, testName, origin string, environment bool) store.RunFailureRow {
	return store.RunFailureRow{ProjectID: 10, FullName: fullName, TestName: testName, Origin: origin, Environment: environment}
}

// originsHandler serves the all-groups feed over one old run (pipelineID,
// commitSHA) of group 7 whose single suite has failed failing tests, and
// answers ListRunFailures with fn.
func originsHandler(t *testing.T, pipelineID string, failed int, fn func(context.Context, int64, string, int) ([]store.RunFailureRow, error)) *PipelineHandler {
	t.Helper()
	total := failed + 5
	rows := []store.PipelineRunRow{
		pipelineRow(store.PipelineRunRow{
			PipelineID: pipelineID, CommitSHA: "abc1234", CreatedAt: awaitingNow.Add(-5 * time.Hour),
			ProjectID: 10, Slug: "suite-10", BuildNumber: 3, BuildID: 300, GroupProjectID: 7,
		}, 5, failed, 0, total, 1000),
	}
	h := newAwaitingHandler(t, rows, nil)
	h.pipelineStore.(*testutil.MockPipelineStore).ListRunFailuresFn = fn
	return h
}

// serveOrigins returns the status and the decoded aggregate of the first run.
func serveOrigins(t *testing.T, h *PipelineHandler) (int, map[string]any) {
	t.Helper()
	code, body := serveJSON(t, h.GetAllPipelineRuns, http.MethodGet, "/api/v1/pipeline-runs", "")
	if code != http.StatusOK {
		return code, nil
	}
	return code, aggregateOf(t, body, "data.0.aggregate")
}

// TestPipelineOrigins: the counts follow the UI's mergeRunFailures + countOrigins
// so a collapsed row reads the same as the expanded panel's header.
func TestPipelineOrigins(t *testing.T) {
	tests := []struct {
		name string
		rows []store.RunFailureRow
		want map[string]any
	}{
		{name: "new and inherited",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginNew, false),
				originsRow("b", "b", store.FailureOriginNew, false),
				originsRow("c", "c", store.FailureOriginInherited, false),
			},
			want: map[string]any{"new": 2.0, "inherited": 1.0, "environment": 0.0}},
		{name: "twins of one test count once, as inherited",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginNew, false),
				originsRow("a", "a", store.FailureOriginInherited, false),
			},
			want: map[string]any{"new": 0.0, "inherited": 1.0, "environment": 0.0}},
		{name: "twins merge the same whatever the order",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginInherited, false),
				originsRow("a", "a", store.FailureOriginNew, false),
			},
			want: map[string]any{"new": 0.0, "inherited": 1.0, "environment": 0.0}},
		{name: "a known origin beats unknown among twins",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginUnknown, false),
				originsRow("a", "a", store.FailureOriginNew, false),
			},
			want: map[string]any{"new": 1.0, "inherited": 0.0, "environment": 0.0}},
		{name: "new with environment counts as inherited and environment",
			rows: []store.RunFailureRow{originsRow("a", "a", store.FailureOriginNew, true)},
			want: map[string]any{"new": 0.0, "inherited": 1.0, "environment": 1.0}},
		{name: "environment on one twin is kept",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginNew, true),
				originsRow("a", "a", store.FailureOriginNew, false),
			},
			want: map[string]any{"new": 0.0, "inherited": 1.0, "environment": 1.0}},
		{name: "empty full_name dedupes on test_name",
			rows: []store.RunFailureRow{
				originsRow("", "login", store.FailureOriginNew, false),
				originsRow("", "login", store.FailureOriginInherited, false),
				originsRow("", "logout", store.FailureOriginNew, false),
			},
			want: map[string]any{"new": 1.0, "inherited": 1.0, "environment": 0.0}},
		{name: "same full_name in two projects is two tests",
			rows: []store.RunFailureRow{
				{ProjectID: 10, FullName: "a", Origin: store.FailureOriginNew},
				{ProjectID: 11, FullName: "a", Origin: store.FailureOriginNew},
			},
			want: map[string]any{"new": 2.0, "inherited": 0.0, "environment": 0.0}},
		{name: "unknown is counted",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginUnknown, false),
				originsRow("b", "b", "", false),
				originsRow("c", "c", "something-newer", false),
			},
			want: map[string]any{"new": 0.0, "inherited": 0.0, "environment": 0.0, "unknown": 3.0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := originsHandler(t, "p-1", len(tc.rows), func(context.Context, int64, string, int) ([]store.RunFailureRow, error) {
				return tc.rows, nil
			})
			code, agg := serveOrigins(t, h)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			got, ok := agg["origins"].(map[string]any)
			if !ok {
				t.Fatalf("aggregate.origins = %v, want an object", agg["origins"])
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("origins.%s = %v, want %v (origins %v)", k, got[k], want, got)
				}
			}
		})
	}
}

// TestPipelineOrigins_Omitted: a run that has no failures, a run whose failures
// the store could not list, and a run with more failures than the panel lists
// carry no origins key at all.
func TestPipelineOrigins_Omitted(t *testing.T) {
	over := make([]store.RunFailureRow, defaultRunFailuresLimit+1)
	for i := range over {
		over[i] = originsRow(fmt.Sprintf("t%d", i), "t", store.FailureOriginNew, false)
	}
	tests := []struct {
		name      string
		failed    int
		rows      []store.RunFailureRow
		err       error
		wantCalls int
	}{
		{name: "no failing suites makes no store call", failed: 0, rows: []store.RunFailureRow{originsRow("a", "a", store.FailureOriginNew, false)}, wantCalls: 0},
		{name: "store error", failed: 2, err: errors.New("db down"), wantCalls: 1},
		{name: "more failures than the limit", failed: 600, rows: over, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []originsCall
			h := originsHandler(t, "p-1", tc.failed, func(_ context.Context, group int64, runKey string, limit int) ([]store.RunFailureRow, error) {
				calls = append(calls, originsCall{group, runKey, limit})
				return tc.rows, tc.err
			})
			code, agg := serveOrigins(t, h)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			if len(calls) != tc.wantCalls {
				t.Errorf("ListRunFailures calls = %v, want %d", calls, tc.wantCalls)
			}
			if got, present := agg["origins"]; present {
				t.Errorf("aggregate.origins = %v, want the key absent", got)
			}
			// The run itself is untouched.
			if agg["suites_total"] != float64(1) {
				t.Errorf("suites_total = %v, want 1", agg["suites_total"])
			}
		})
	}
}

// TestPipelineOrigins_Request: the store is asked for one row more than the
// panel lists, under the run's group and its pipeline id, or its commit SHA when
// it has none, the key GetPipelineRun takes.
func TestPipelineOrigins_Request(t *testing.T) {
	for _, tc := range []struct{ name, pipelineID, wantKey string }{
		{"pipeline id", "p-1", "p-1"},
		{"commit sha when there is no pipeline id", "", "abc1234"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []originsCall
			h := originsHandler(t, tc.pipelineID, 1, func(_ context.Context, group int64, runKey string, limit int) ([]store.RunFailureRow, error) {
				calls = append(calls, originsCall{group, runKey, limit})
				return []store.RunFailureRow{originsRow("a", "a", store.FailureOriginNew, false)}, nil
			})
			if code, _ := serveOrigins(t, h); code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			if want := []originsCall{{7, tc.wantKey, defaultRunFailuresLimit + 1}}; !slices.Equal(calls, want) {
				t.Errorf("ListRunFailures calls = %v, want %v", calls, want)
			}
		})
	}
}

// TestPipelineOrigins_AtTheLimit: exactly as many failures as the panel lists
// is not truncated, so the row still gets its counts (GetRunFailures truncates
// on len > limit too).
func TestPipelineOrigins_AtTheLimit(t *testing.T) {
	rows := make([]store.RunFailureRow, defaultRunFailuresLimit)
	for i := range rows {
		rows[i] = originsRow(fmt.Sprintf("t%d", i), "t", store.FailureOriginNew, false)
	}
	h := originsHandler(t, "p-1", defaultRunFailuresLimit, func(context.Context, int64, string, int) ([]store.RunFailureRow, error) {
		return rows, nil
	})
	code, agg := serveOrigins(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	got, _ := agg["origins"].(map[string]any)
	if got["new"] != float64(defaultRunFailuresLimit) {
		t.Errorf("aggregate.origins = %v, want new = %d", agg["origins"], defaultRunFailuresLimit)
	}
}
