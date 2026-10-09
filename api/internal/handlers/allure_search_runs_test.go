package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func searchWith(ms *testutil.MockSearchStore) http.HandlerFunc {
	return NewSearchHandler(ms).Search
}

// TestSearch_ReturnsPipelineRunsAndBranches: runs and branches ride in data
// next to projects and tests with the documented JSON shape, and the handler
// forwards the query and the clamped limit to both new store calls. A
// non-UTC timestamp is rendered in UTC, since the format claims a literal Z.
func TestSearch_ReturnsPipelineRunsAndBranches(t *testing.T) {
	var gotRunQ, gotBranchQ string
	var gotRunLim, gotBranchLim int
	ms := &testutil.MockSearchStore{
		SearchPipelineRunsFn: func(_ context.Context, q string, limit int) ([]store.PipelineRunMatch, error) {
			gotRunQ, gotRunLim = q, limit
			return []store.PipelineRunMatch{{
				GroupProjectID: 7, GroupSlug: "checkout", PipelineID: "48213", Branch: "login-retry",
				Timestamp: time.Date(2026, 3, 4, 7, 6, 5, 0, time.FixedZone("CET", 3600)),
			}}, nil
		},
		SearchBranchesFn: func(_ context.Context, q string, limit int) ([]store.BranchMatch, error) {
			gotBranchQ, gotBranchLim = q, limit
			return []store.BranchMatch{{Name: "login-retry"}, {Name: "login-retry-2"}}, nil
		},
	}

	code, body := serveJSON(t, searchWith(ms), http.MethodGet, "/api/v1/search?q=48213&limit=999", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}
	wantJSON(t, body, map[string]any{
		"data.runs#":                   1,
		"data.runs.0.group_project_id": 7,
		"data.runs.0.group_slug":       "checkout",
		"data.runs.0.pipeline_id":      "48213",
		"data.runs.0.branch":           "login-retry",
		"data.runs.0.timestamp":        "2026-03-04T06:06:05Z",
		"data.branches#":               2,
		"data.branches.0.name":         "login-retry",
		"data.branches.1.name":         "login-retry-2",
		"data.projects#":               0,
		"data.tests#":                  0,
	})
	if gotRunQ != "48213" || gotBranchQ != "48213" {
		t.Errorf("queries forwarded = %q / %q, want 48213", gotRunQ, gotBranchQ)
	}
	if gotRunLim != searchMaxLim || gotBranchLim != searchMaxLim {
		t.Errorf("limits forwarded = %d / %d, want %d", gotRunLim, gotBranchLim, searchMaxLim)
	}
}

// TestSearch_NoRunsOrBranchesAreEmptyArrays: a store answering nil, or the
// mock with nothing configured, still yields [] and never null.
func TestSearch_NoRunsOrBranchesAreEmptyArrays(t *testing.T) {
	nilStore := &testutil.MockSearchStore{
		SearchPipelineRunsFn: func(context.Context, string, int) ([]store.PipelineRunMatch, error) { return nil, nil },
		SearchBranchesFn:     func(context.Context, string, int) ([]store.BranchMatch, error) { return nil, nil },
	}
	for name, ms := range map[string]*testutil.MockSearchStore{"unset": {}, "nil slices": nilStore} {
		t.Run(name, func(t *testing.T) {
			code, body := serveJSON(t, searchWith(ms), http.MethodGet, "/api/v1/search?q=nothing", "")
			if code != http.StatusOK {
				t.Fatalf("status = %d: %v", code, body)
			}
			wantJSON(t, body, map[string]any{"data.runs#": 0, "data.branches#": 0})
		})
	}
}

// TestSearch_RunOrBranchStoreErrorIs500: a failing run or branch lookup fails
// the whole search rather than returning a partial result.
func TestSearch_RunOrBranchStoreErrorIs500(t *testing.T) {
	boom := errors.New("boom")
	for name, ms := range map[string]*testutil.MockSearchStore{
		"runs":     {SearchPipelineRunsFn: func(context.Context, string, int) ([]store.PipelineRunMatch, error) { return nil, boom }},
		"branches": {SearchBranchesFn: func(context.Context, string, int) ([]store.BranchMatch, error) { return nil, boom }},
	} {
		t.Run(name, func(t *testing.T) {
			if code, body := serveJSON(t, searchWith(ms), http.MethodGet, "/api/v1/search?q=48213", ""); code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500: %v", code, body)
			}
		})
	}
}

// TestSearch_TrimsSpacesAroundQuery: a pasted "48213 " reaches both new store
// calls as "48213", and a query that is only spaces, or shorter than the
// minimum once trimmed, is a 400. A leading "#" stays part of the query.
func TestSearch_TrimsSpacesAroundQuery(t *testing.T) {
	var gotRunQ, gotBranchQ string
	ms := &testutil.MockSearchStore{
		SearchPipelineRunsFn: func(_ context.Context, q string, _ int) ([]store.PipelineRunMatch, error) {
			gotRunQ = q
			return nil, nil
		},
		SearchBranchesFn: func(_ context.Context, q string, _ int) ([]store.BranchMatch, error) {
			gotBranchQ = q
			return nil, nil
		},
	}
	for _, tt := range []struct {
		query string
		want  string
	}{{"%2048213%20", "48213"}, {"%20%20login-retry%09", "login-retry"}, {"%2348213", "#48213"}} {
		gotRunQ, gotBranchQ = "", ""
		if code, body := serveJSON(t, searchWith(ms), http.MethodGet, "/api/v1/search?q="+tt.query, ""); code != http.StatusOK {
			t.Fatalf("q=%s: status = %d: %v", tt.query, code, body)
		}
		if gotRunQ != tt.want || gotBranchQ != tt.want {
			t.Errorf("q=%s: forwarded %q / %q, want %q", tt.query, gotRunQ, gotBranchQ, tt.want)
		}
	}
	for _, q := range []string{"%20%20", "%20a%20"} {
		if code, body := serveJSON(t, searchWith(ms), http.MethodGet, "/api/v1/search?q="+q, ""); code != http.StatusBadRequest {
			t.Errorf("q=%s: status = %d, want 400: %v", q, code, body)
		}
	}
}
