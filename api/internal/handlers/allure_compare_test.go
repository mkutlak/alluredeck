package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestCompareHandler_CompareBuilds(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	// Builds 1 and 2 have IDs 10 and 20; the diff is served only for that pair.
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, buildNumber int) (int64, error) {
		if buildNumber == 1 || buildNumber == 2 {
			return int64(buildNumber) * 10, nil
		}
		return 0, store.ErrBuildNotFound
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, a, b int64) ([]store.DiffEntry, error) {
		if a != 10 || b != 20 {
			return nil, errors.New("unexpected build ids")
		}
		return []store.DiffEntry{
			{TestName: "LoginTest", FullName: "pkg.LoginTest", HistoryID: "h1", StatusA: "passed", StatusB: "failed", DurationA: 1000, DurationB: 2000, Category: store.DiffRegressed},
			{TestName: "NewTest", FullName: "pkg.NewTest", HistoryID: "h3", StatusB: "passed", DurationB: 300, Category: store.DiffAdded},
		}, nil
	}
	withStore, noStore := newTestCompareHandler(t, mocks), NewCompareHandler(nil, nil)
	rows := []struct {
		name    string
		h       *CompareHandler
		project string
		query   string
		want    int
		wantMsg string
	}{
		{"diff", withStore, "1", "a=1&b=2", http.StatusOK, ""},
		// Without a test-result store the comparison is empty, not an error.
		{"no store", noStore, "1", "a=1&b=2", http.StatusOK, ""},
		{"missing both", withStore, "1", "", http.StatusBadRequest, ""},
		{"missing b", withStore, "1", "a=1", http.StatusBadRequest, ""},
		{"missing a", withStore, "1", "b=2", http.StatusBadRequest, ""},
		{"a not integer", withStore, "1", "a=foo&b=2", http.StatusBadRequest, ""},
		{"b not integer", withStore, "1", "a=1&b=bar", http.StatusBadRequest, ""},
		{"a zero", withStore, "1", "a=0&b=2", http.StatusBadRequest, ""},
		{"b zero", withStore, "1", "a=1&b=0", http.StatusBadRequest, ""},
		{"a negative", withStore, "1", "a=-1&b=2", http.StatusBadRequest, ""},
		{"same build", withStore, "1", "a=1&b=1", http.StatusBadRequest, "build_a and build_b must be different"},
		{"unknown build", withStore, "1", "a=1&b=99", http.StatusBadRequest, ""},
		{"invalid project id", withStore, "../evil", "a=1&b=2", http.StatusBadRequest, ""},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/compare?"+tc.query, nil)
			req.SetPathValue("project_id", tc.project)
			rr := httptest.NewRecorder()
			tc.h.CompareBuilds(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			var resp struct {
				Data struct {
					BuildA  int              `json:"build_a"`
					BuildB  int              `json:"build_b"`
					Summary compareSummary   `json:"summary"`
					Tests   []map[string]any `json:"tests"`
				} `json:"data"`
				Metadata ResponseMeta `json:"metadata"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if tc.wantMsg != "" && resp.Metadata.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", resp.Metadata.Message, tc.wantMsg)
			}
			if tc.want != http.StatusOK {
				return
			}
			d := resp.Data
			if d.BuildA != 1 || d.BuildB != 2 || d.Tests == nil {
				t.Fatalf("data = %+v, want build_a 1, build_b 2 and a tests array", d)
			}
			if tc.h == noStore {
				return
			}
			if want := (compareSummary{Regressed: 1, Added: 1, Total: 2}); d.Summary != want || len(d.Tests) != 2 {
				t.Fatalf("summary = %+v with %d tests, want %+v with 2", d.Summary, len(d.Tests), want)
			}
			for _, field := range []string{"test_name", "full_name", "history_id", "status_a", "status_b", "duration_a", "duration_b", "category"} {
				if _, ok := d.Tests[0][field]; !ok {
					t.Errorf("test entry lacks %q", field)
				}
			}
			if delta := d.Tests[0]["duration_delta"]; delta != float64(1000) {
				t.Errorf("duration_delta = %v, want 1000", delta)
			}
		})
	}
}
