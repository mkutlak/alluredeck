package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

var errKnownIssueBoom = errors.New("known issue store boom")

func newBuildTestsHandler(t *testing.T, trs *testutil.MockTestResultStore, kis *testutil.MockKnownIssueStore, ps *testutil.MemProjectStore) *BuildTestsHandler {
	t.Helper()
	return NewBuildTestsHandler(trs, kis, ps, zap.NewNop())
}

func buildTestsRequest(h *BuildTestsHandler, projectID, buildID, query string) *httptest.ResponseRecorder {
	path := "/api/v1/projects/" + projectID + "/builds/" + buildID + "/tests"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("build_id", buildID)
	rr := httptest.NewRecorder()
	h.ListBuildTests(rr, req)
	return rr
}

func TestBuildTestsHandler_KnownAndNewPartition(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	trs := &testutil.MockTestResultStore{
		ListFailedByBuildFn: func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{
				{
					TestName: "known test", FullName: "pkg.known test", Status: "failed",
					DurationMs: 100, HistoryID: "h1", Flaky: true, Retries: 2, NewFailed: false,
					StatusMessage: "assertion failed\nstack trace line 2\nstack trace line 3",
				},
				{
					TestName: "new test", FullName: "pkg.new test", Status: "broken",
					DurationMs: 200, HistoryID: "h2", Flaky: false, Retries: 0, NewFailed: true,
					StatusMessage: "boom",
				},
			}, nil
		},
	}
	kis := &testutil.MockKnownIssueStore{
		ListFn: func(_ context.Context, _ int64, _ bool) ([]store.KnownIssue, error) {
			return []store.KnownIssue{{TestName: "known test"}}, nil
		},
	}

	h := newBuildTestsHandler(t, trs, kis, projStore)
	rr := buildTestsRequest(h, formatID(proj.ID), "42", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []buildTestResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(resp.Data))
	}

	byName := map[string]buildTestResp{}
	for _, tr := range resp.Data {
		byName[tr.TestName] = tr
	}

	known, ok := byName["known test"]
	if !ok {
		t.Fatalf("missing known test in response: %+v", resp.Data)
	}
	if !known.Known {
		t.Error("known test should have known=true")
	}
	if !known.Flaky || known.Retries != 2 {
		t.Errorf("known test flaky/retries = %v/%d, want true/2", known.Flaky, known.Retries)
	}
	if known.ErrorMessage != "assertion failed" {
		t.Errorf("known test error_message = %q, want first line only", known.ErrorMessage)
	}
	if known.FullName != "pkg.known test" || known.Status != "failed" || known.DurationMs != 100 || known.HistoryID != "h1" {
		t.Errorf("known test passthrough fields wrong: %+v", known)
	}

	newT, ok := byName["new test"]
	if !ok {
		t.Fatalf("missing new test in response: %+v", resp.Data)
	}
	if newT.Known {
		t.Error("new test should have known=false")
	}
	if !newT.NewFailed {
		t.Error("new test should have new_failed=true")
	}
	if newT.ErrorMessage != "boom" {
		t.Errorf("new test error_message = %q, want boom", newT.ErrorMessage)
	}
}

func TestBuildTestsHandler_SkipsKnownIssueQueryWhenNoFailures(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	trs := &testutil.MockTestResultStore{
		ListFailedByBuildFn: func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{}, nil
		},
	}
	knownIssueCalled := false
	kis := &testutil.MockKnownIssueStore{
		ListFn: func(_ context.Context, _ int64, _ bool) ([]store.KnownIssue, error) {
			knownIssueCalled = true
			return nil, nil
		},
	}

	h := newBuildTestsHandler(t, trs, kis, projStore)
	rr := buildTestsRequest(h, formatID(proj.ID), "42", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if knownIssueCalled {
		t.Error("known issue store should not be queried when there are zero failures")
	}

	var resp struct {
		Data []buildTestResp `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data == nil {
		t.Error("expected data to be an empty slice, not null")
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 tests, got %d", len(resp.Data))
	}
}

func TestBuildTestsHandler_StatusPassed_BadRequest(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	h := newBuildTestsHandler(t, &testutil.MockTestResultStore{}, &testutil.MockKnownIssueStore{}, projStore)
	rr := buildTestsRequest(h, formatID(proj.ID), "42", "status=passed")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBuildTestsHandler_InvalidBuildID(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	h := newBuildTestsHandler(t, &testutil.MockTestResultStore{}, &testutil.MockKnownIssueStore{}, projStore)

	for _, buildID := range []string{"abc", "0", "-1"} {
		rr := buildTestsRequest(h, formatID(proj.ID), buildID, "")
		if rr.Code != http.StatusBadRequest {
			t.Errorf("build_id=%q: expected 400, got %d: %s", buildID, rr.Code, rr.Body.String())
		}
	}
}

func TestBuildTestsHandler_LimitDefaultAndCap(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	var capturedLimit int
	trs := &testutil.MockTestResultStore{
		ListFailedByBuildFn: func(_ context.Context, _ int64, _ int64, limit int) ([]store.TestResult, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	h := newBuildTestsHandler(t, trs, &testutil.MockKnownIssueStore{}, projStore)

	buildTestsRequest(h, formatID(proj.ID), "42", "")
	if capturedLimit != 50 {
		t.Errorf("default limit = %d, want 50", capturedLimit)
	}

	buildTestsRequest(h, formatID(proj.ID), "42", "limit=1000")
	if capturedLimit != 200 {
		t.Errorf("capped limit = %d, want 200", capturedLimit)
	}

	buildTestsRequest(h, formatID(proj.ID), "42", "limit=75")
	if capturedLimit != 75 {
		t.Errorf("explicit limit = %d, want 75", capturedLimit)
	}

	rr := buildTestsRequest(h, formatID(proj.ID), "42", "limit=-5")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("negative limit: expected 400, got %d", rr.Code)
	}

	rr = buildTestsRequest(h, formatID(proj.ID), "42", "limit=abc")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("non-numeric limit: expected 400, got %d", rr.Code)
	}
}

func TestBuildTestsHandler_KnownIssueStoreError(t *testing.T) {
	projStore := testutil.NewMemProjectStore()
	proj, _ := projStore.CreateProject(context.Background(), "demo")

	trs := &testutil.MockTestResultStore{
		ListFailedByBuildFn: func(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
			return []store.TestResult{{TestName: "t1", FullName: "pkg.t1", Status: "failed"}}, nil
		},
	}
	kis := &testutil.MockKnownIssueStore{
		ListFn: func(_ context.Context, _ int64, _ bool) ([]store.KnownIssue, error) {
			return nil, errKnownIssueBoom
		},
	}

	h := newBuildTestsHandler(t, trs, kis, projStore)
	rr := buildTestsRequest(h, formatID(proj.ID), "42", "")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFirstErrorLine(t *testing.T) {
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
		{"very long line truncated to 300 runes", longRunes(400), longRunes(300)},
		{"long line with newline truncated", longRunes(400) + "\nrest", longRunes(300)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstErrorLine(tt.msg); got != tt.want {
				t.Errorf("firstErrorLine(%q) = %q (len %d), want %q (len %d)", truncateForDisplay(tt.msg), got, len([]rune(got)), truncateForDisplay(tt.want), len([]rune(tt.want)))
			}
		})
	}
}

func longRunes(n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = 'a'
	}
	return string(runes)
}

func truncateForDisplay(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

func formatID(id int64) string {
	return fmt.Sprintf("%d", id)
}
