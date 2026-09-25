package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestBuildTestsHandler_ListBuildTests(t *testing.T) {
	t.Parallel()
	failed := []store.TestResult{
		{TestName: "known test", FullName: "pkg.known test", Status: "failed", DurationMs: 100, HistoryID: "h1", Flaky: true, Retries: 2,
			StatusMessage: "assertion failed\nstack trace line 2\nstack trace line 3"},
		{TestName: "new test", FullName: "pkg.new test", Status: "broken", DurationMs: 200, HistoryID: "h2", NewFailed: true, StatusMessage: "boom"},
	}
	errBoom := errors.New("known issue store boom")
	rows := []struct {
		name      string
		buildID   string
		query     string
		results   []store.TestResult
		issuesErr error // what the known-issue store answers
		want      int
		wantLimit int // limit the store must receive
		wantTests []buildTestResp
	}{
		// Failures are flagged known when an active known issue names them;
		// the error message is the first line only.
		{name: "known and new failures", buildID: "42", results: failed, want: http.StatusOK, wantLimit: 50, wantTests: []buildTestResp{
			{TestName: "known test", FullName: "pkg.known test", Status: "failed", DurationMs: 100, HistoryID: "h1", Flaky: true, Retries: 2, Known: true, ErrorMessage: "assertion failed"},
			{TestName: "new test", FullName: "pkg.new test", Status: "broken", DurationMs: 200, HistoryID: "h2", NewFailed: true, ErrorMessage: "boom"},
		}},
		// With no failures the known-issue store — hot on every build page —
		// is not consulted, so its error cannot surface.
		{name: "no failures skips the known-issue lookup", buildID: "42", issuesErr: errBoom, want: http.StatusOK, wantLimit: 50, wantTests: []buildTestResp{}},
		{name: "known-issue store error", buildID: "42", results: failed[:1], issuesErr: errBoom, want: http.StatusInternalServerError},
		{name: "status other than failed", buildID: "42", query: "status=passed", want: http.StatusBadRequest},
		{name: "build id not a number", buildID: "abc", want: http.StatusBadRequest},
		{name: "build id zero", buildID: "0", want: http.StatusBadRequest},
		{name: "build id negative", buildID: "-1", want: http.StatusBadRequest},
		{name: "limit zero means default", buildID: "42", query: "limit=0", want: http.StatusOK, wantLimit: 50, wantTests: []buildTestResp{}},
		{name: "limit capped", buildID: "42", query: "limit=1000", want: http.StatusOK, wantLimit: 200, wantTests: []buildTestResp{}},
		{name: "explicit limit", buildID: "42", query: "limit=75", want: http.StatusOK, wantLimit: 75, wantTests: []buildTestResp{}},
		{name: "negative limit", buildID: "42", query: "limit=-5", want: http.StatusBadRequest},
		{name: "non-numeric limit", buildID: "42", query: "limit=abc", want: http.StatusBadRequest},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotLimit int
			trs := &testutil.MockTestResultStore{ListFailedByBuildFn: func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
				gotLimit = limit
				return tc.results, nil
			}}
			kis := &testutil.MockKnownIssueStore{ListFn: func(context.Context, int64, bool) ([]store.KnownIssue, error) {
				return []store.KnownIssue{{TestName: "known test"}}, tc.issuesErr
			}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/builds/"+tc.buildID+"/tests?"+tc.query, nil)
			req.SetPathValue("project_id", "1")
			req.SetPathValue("build_id", tc.buildID)
			rr := httptest.NewRecorder()
			NewBuildTestsHandler(trs, kis, testutil.NewMemProjectStore(), zap.NewNop()).ListBuildTests(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			if gotLimit != tc.wantLimit {
				t.Errorf("store limit = %d, want %d", gotLimit, tc.wantLimit)
			}
			var resp struct {
				Data []buildTestResp `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Data == nil || len(resp.Data) != len(tc.wantTests) {
				t.Fatalf("data = %+v, want an array of %d", resp.Data, len(tc.wantTests))
			}
			for i := range tc.wantTests {
				if resp.Data[i] != tc.wantTests[i] {
					t.Errorf("data[%d] = %+v, want %+v", i, resp.Data[i], tc.wantTests[i])
				}
			}
		})
	}
}

func TestFirstErrorLine(t *testing.T) {
	t.Parallel()
	a300, a400 := strings.Repeat("a", 300), strings.Repeat("a", 400)
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"empty", "", ""},
		{"single line", "boom", "boom"},
		{"multi line unix", "line one\nline two\nline three", "line one"},
		{"multi line crlf", "line one\r\nline two", "line one"},
		{"trailing newline", "only line\n", "only line"},
		{"very long line truncated to 300 runes", a400, a300},
		{"long line with newline truncated", a400 + "\nrest", a300},
	}
	for _, tt := range tests {
		if got := firstErrorLine(tt.msg); got != tt.want {
			t.Errorf("%s: firstErrorLine = %.40q... (%d runes), want %.40q... (%d runes)", tt.name, got, len([]rune(got)), tt.want, len([]rune(tt.want)))
		}
	}
}
