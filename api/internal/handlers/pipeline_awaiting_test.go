package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// awaitingNow is the handler's clock in these tests.
var awaitingNow = time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

// awaitingEndpoint is one of the three endpoints that report a run's aggregate.
type awaitingEndpoint struct {
	name string
	// serve calls the endpoint and returns the aggregate object of the run.
	serve func(t *testing.T, h *PipelineHandler) (int, map[string]any)
	// rowGroup is the group project id the endpoint's store rows carry (the
	// per-parent list leaves it 0) and wantGroup the group ExpectedSuites must
	// be asked about.
	rowGroup, wantGroup int64
}

func aggregateOf(t *testing.T, body any, path string) map[string]any {
	t.Helper()
	agg, ok := jsonAt(body, path).(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object in %v", path, body)
	}
	return agg
}

var awaitingEndpoints = []awaitingEndpoint{
	{
		name: "per-parent list", rowGroup: 0, wantGroup: 1,
		serve: func(t *testing.T, h *PipelineHandler) (int, map[string]any) {
			code, body := serveJSON(t, h.GetPipelineRuns, http.MethodGet, "/api/v1/projects/1/pipeline-runs", "", "project_id", "1")
			if code != http.StatusOK {
				return code, nil
			}
			return code, aggregateOf(t, body, "data.0.aggregate")
		},
	},
	{
		name: "all-groups list", rowGroup: 7, wantGroup: 7,
		serve: func(t *testing.T, h *PipelineHandler) (int, map[string]any) {
			code, body := serveJSON(t, h.GetAllPipelineRuns, http.MethodGet, "/api/v1/pipeline-runs", "")
			if code != http.StatusOK {
				return code, nil
			}
			return code, aggregateOf(t, body, "data.0.aggregate")
		},
	},
	{
		name: "single run", rowGroup: 7, wantGroup: 7,
		serve: func(t *testing.T, h *PipelineHandler) (int, map[string]any) {
			code, body := serveJSON(t, h.GetPipelineRun, http.MethodGet, "/api/v1/projects/1/pipeline-runs/p-1", "",
				"project_id", "1", "run_key", "p-1")
			if code != http.StatusOK {
				return code, nil
			}
			return code, aggregateOf(t, body, "data.aggregate")
		},
	},
}

type awaitingCall struct {
	group  int64
	runKey string
}

// TestAwaitingSuites drives every endpoint over one run that is age old and
// has uploaded the suites with the project ids in present, against a store
// that expects expected. wantAwaiting 0 means the key must be absent.
func TestAwaitingSuites(t *testing.T) {
	tests := []struct {
		name         string
		age          time.Duration
		present      []int64
		expected     []int64
		storeErr     error
		wantAwaiting int
		wantCalls    int
	}{
		{name: "5 minutes old, 3 of 5 present", age: 5 * time.Minute,
			present: []int64{10, 11, 12}, expected: []int64{10, 11, 12, 13, 14}, wantAwaiting: 2, wantCalls: 1},
		{name: "29m59s old is marked", age: 29*time.Minute + 59*time.Second,
			present: []int64{10}, expected: []int64{10, 11}, wantAwaiting: 1, wantCalls: 1},
		{name: "30m00s old is not marked and costs no store call", age: 30 * time.Minute,
			present: []int64{10}, expected: []int64{10, 11}, wantAwaiting: 0, wantCalls: 0},
		{name: "old run costs no store call", age: 5 * time.Hour,
			present: []int64{10}, expected: []int64{10, 11}, wantAwaiting: 0, wantCalls: 0},
		{name: "all expected present", age: time.Minute,
			present: []int64{10, 11, 12}, expected: []int64{10, 11, 12}, wantAwaiting: 0, wantCalls: 1},
		{name: "nothing expected", age: time.Minute,
			present: []int64{10}, expected: nil, wantAwaiting: 0, wantCalls: 1},
		{name: "unexpected suites present do not count", age: time.Minute,
			present: []int64{10, 11, 99}, expected: []int64{10, 11, 12}, wantAwaiting: 1, wantCalls: 1},
		{name: "extra suites beyond the expected ones", age: time.Minute,
			present: []int64{10, 11, 99}, expected: []int64{10, 11}, wantAwaiting: 0, wantCalls: 1},
		{name: "store error leaves the run intact", age: time.Minute,
			present: []int64{10, 11}, expected: []int64{10, 11, 12}, storeErr: errors.New("db down"), wantAwaiting: 0, wantCalls: 1},
	}
	for _, ep := range awaitingEndpoints {
		for _, tc := range tests {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				var rows []store.PipelineRunRow
				for _, id := range tc.present {
					rows = append(rows, pipelineRow(store.PipelineRunRow{
						PipelineID: "p-1", CommitSHA: "abc1234", CreatedAt: awaitingNow.Add(-tc.age),
						ProjectID: id, Slug: fmt.Sprintf("suite-%d", id), BuildNumber: 3, BuildID: id * 100,
						GroupProjectID: ep.rowGroup,
					}, 10, 0, 0, 10, 1000))
				}
				var calls []awaitingCall
				h := newAwaitingHandler(t, rows, func(_ context.Context, group int64, runKey string) ([]int64, error) {
					calls = append(calls, awaitingCall{group, runKey})
					return tc.expected, tc.storeErr
				})
				code, agg := ep.serve(t, h)
				if code != http.StatusOK {
					t.Fatalf("status = %d, want 200", code)
				}
				if len(calls) != tc.wantCalls {
					t.Fatalf("ExpectedSuites calls = %v, want %d", calls, tc.wantCalls)
				}
				if tc.wantCalls == 1 && calls[0] != (awaitingCall{ep.wantGroup, "p-1"}) {
					t.Errorf("ExpectedSuites asked %+v, want group %d run p-1", calls[0], ep.wantGroup)
				}
				got, present := agg["awaiting_suites"]
				if tc.wantAwaiting == 0 {
					if present {
						t.Errorf("awaiting_suites = %v, want the key absent", got)
					}
				} else if got != float64(tc.wantAwaiting) {
					t.Errorf("awaiting_suites = %v, want %d", got, tc.wantAwaiting)
				}
				// The run itself is untouched whatever the verdict.
				if agg["suites_total"] != float64(len(tc.present)) || agg["suites_passed"] != float64(len(tc.present)) {
					t.Errorf("suites_passed/suites_total = %v/%v, want %d/%d", agg["suites_passed"], agg["suites_total"], len(tc.present), len(tc.present))
				}
			})
		}
	}
}

// TestAwaitingSuites_RunKeyIsPipelineIDOrCommitSHA: a run with no pipeline id
// is asked about under its commit SHA, the key GetPipelineRun takes.
func TestAwaitingSuites_RunKeyIsPipelineIDOrCommitSHA(t *testing.T) {
	rows := []store.PipelineRunRow{
		pipelineRow(store.PipelineRunRow{CommitSHA: "abc1234", CreatedAt: awaitingNow.Add(-time.Minute), ProjectID: 10, Slug: "suite-10", BuildNumber: 1, BuildID: 1, GroupProjectID: 7}, 1, 0, 0, 1, 1),
	}
	var calls []awaitingCall
	h := newAwaitingHandler(t, rows, func(_ context.Context, group int64, runKey string) ([]int64, error) {
		calls = append(calls, awaitingCall{group, runKey})
		return []int64{10, 11}, nil
	})
	code, agg := awaitingEndpoints[2].serve(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if want := []awaitingCall{{7, "abc1234"}}; !slices.Equal(calls, want) {
		t.Errorf("ExpectedSuites calls = %v, want %v", calls, want)
	}
	if agg["awaiting_suites"] != float64(1) {
		t.Errorf("awaiting_suites = %v, want 1", agg["awaiting_suites"])
	}
}

// TestAwaitingSuites_EachRunIsJudgedOnItsOwn: on a page of runs only the
// recent one is asked about and marked.
func TestAwaitingSuites_EachRunIsJudgedOnItsOwn(t *testing.T) {
	rows := []store.PipelineRunRow{
		pipelineRow(store.PipelineRunRow{PipelineID: "recent", CommitSHA: "a", CreatedAt: awaitingNow.Add(-time.Minute), ProjectID: 10, Slug: "s10", BuildNumber: 2, BuildID: 2, GroupProjectID: 7}, 1, 0, 0, 1, 1),
		pipelineRow(store.PipelineRunRow{PipelineID: "old", CommitSHA: "b", CreatedAt: awaitingNow.Add(-3 * time.Hour), ProjectID: 10, Slug: "s10", BuildNumber: 1, BuildID: 1, GroupProjectID: 7}, 1, 0, 0, 1, 1),
	}
	var calls []awaitingCall
	h := newAwaitingHandler(t, rows, func(_ context.Context, group int64, runKey string) ([]int64, error) {
		calls = append(calls, awaitingCall{group, runKey})
		return []int64{10, 11}, nil
	})
	code, body := serveJSON(t, h.GetAllPipelineRuns, http.MethodGet, "/api/v1/pipeline-runs", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}
	if want := []awaitingCall{{7, "recent"}}; !slices.Equal(calls, want) {
		t.Errorf("ExpectedSuites calls = %v, want %v", calls, want)
	}
	if got := aggregateOf(t, body, "data.0.aggregate")["awaiting_suites"]; got != float64(1) {
		t.Errorf("recent run awaiting_suites = %v, want 1", got)
	}
	if _, present := aggregateOf(t, body, "data.1.aggregate")["awaiting_suites"]; present {
		t.Errorf("old run carries awaiting_suites")
	}
}

// TestAwaitingSuites_StopsWhenTheRequestIsCancelled: once the request is
// cancelled every remaining run would fail the same way, so the loop asks the
// store for the run in flight and then goes quiet, instead of making a call and
// logging a warning for each run left on the page. The response is still sent.
func TestAwaitingSuites_StopsWhenTheRequestIsCancelled(t *testing.T) {
	var rows []store.PipelineRunRow
	for i := range 3 {
		rows = append(rows, pipelineRow(store.PipelineRunRow{
			PipelineID: fmt.Sprintf("p-%d", i), CommitSHA: "abc1234", CreatedAt: awaitingNow.Add(-time.Minute),
			ProjectID: 10, Slug: "suite-10", BuildNumber: i + 1, BuildID: int64(i + 1), GroupProjectID: 7,
		}, 1, 0, 0, 1, 1))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	h := newAwaitingHandler(t, rows, func(ctx context.Context, _ int64, _ string) ([]int64, error) {
		calls++
		cancel()
		return nil, ctx.Err()
	})
	core, logs := observer.New(zap.WarnLevel)
	h.logger = zap.New(core)

	rr := httptest.NewRecorder()
	h.GetAllPipelineRuns(rr, httptest.NewRequest(http.MethodGet, "/api/v1/pipeline-runs", nil).WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if calls != 1 {
		t.Errorf("ExpectedSuites calls = %d, want 1 (the run in flight when the request was cancelled)", calls)
	}
	if logs.Len() > 1 {
		t.Errorf("%d warnings logged, want at most 1: %v", logs.Len(), logs.All())
	}
}

// newAwaitingHandler is a PipelineHandler over a parent project (id 1) with a
// child, whose store serves rows from every run query and expected from
// ExpectedSuites, and whose clock is awaitingNow.
func newAwaitingHandler(t *testing.T, rows []store.PipelineRunRow, expected func(context.Context, int64, string) ([]int64, error)) *PipelineHandler {
	t.Helper()
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	parent, _ := projects.CreateProject(ctx, "parent")
	_, _ = projects.CreateProjectWithParent(ctx, "child", parent.ID)
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(context.Context, int64, string, int, int) ([]store.PipelineRunRow, int, error) {
			return rows, 1, nil
		},
		ListAllPipelineRunsFn: func(context.Context, string, []int64, int, int) ([]store.PipelineRunRow, int, error) {
			return rows, 1, nil
		},
		GetPipelineRunFn: func(context.Context, int64, string) ([]store.PipelineRunRow, error) { return rows, nil },
		ExpectedSuitesFn: expected,
	}
	h := NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
	h.now = func() time.Time { return awaitingNow }
	return h
}
