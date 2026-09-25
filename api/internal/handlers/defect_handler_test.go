package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// fakeDefectTestResultsStore decorates MemDefectStore to return seeded test
// results from GetTestResults, since MemDefectStore.GetTestResults always
// returns an empty slice. This is needed to exercise the flaky/retries field
// mapping in defectTestResp.
type fakeDefectTestResultsStore struct {
	*testutil.MemDefectStore
	results []store.TestResult
}

var _ store.DefectStorer = (*fakeDefectTestResultsStore)(nil)

func (f *fakeDefectTestResultsStore) GetTestResults(_ context.Context, _ string, _ *int64, _, _ int) ([]store.TestResult, int, error) {
	return f.results, len(f.results), nil
}

func TestDefectHandler(t *testing.T) {
	t.Parallel()
	ds := &fakeDefectTestResultsStore{MemDefectStore: testutil.NewMemDefectStore(), results: []store.TestResult{
		{BuildID: 5, TestName: "t1", FullName: "suite.t1", Status: "failed", Flaky: true, Retries: 2},
	}}
	h := NewDefectHandler(ds, testutil.NewMemProjectStore(), zap.NewNop())
	rows := []struct {
		name     string
		endpoint func(*DefectHandler, http.ResponseWriter, *http.Request)
		defectID string
		body     string
		want     int
		check    func(t *testing.T, resp map[string]any)
	}{
		{"list project defects", (*DefectHandler).ListProjectDefects, "", "", http.StatusOK,
			func(t *testing.T, resp map[string]any) {
				data, _ := resp["data"].([]any)
				pg, _ := resp["pagination"].(map[string]any)
				if data == nil || len(data) != 0 || pg["total"] == nil {
					t.Errorf("data = %v, pagination = %v, want an empty array and a total", resp["data"], resp["pagination"])
				}
			}},
		{"project summary", (*DefectHandler).GetProjectDefectSummary, "", "", http.StatusOK, nil},
		{"build summary", (*DefectHandler).GetBuildDefectSummary, "", "", http.StatusOK, nil},
		// The DTO carries flaky/retries so the UI can badge defect-linked tests.
		{"defect tests", (*DefectHandler).GetDefectTests, "fp-1", "", http.StatusOK,
			func(t *testing.T, resp map[string]any) {
				data, _ := resp["data"].([]any)
				if len(data) != 1 {
					t.Fatalf("data = %v, want 1 test", resp["data"])
				}
				if tr, _ := data[0].(map[string]any); tr["flaky"] != true || tr["retries"] != float64(2) {
					t.Errorf("test = %v, want flaky true and retries 2", tr)
				}
			}},
		{"unknown defect", (*DefectHandler).GetDefect, "nonexistent-id", "", http.StatusNotFound, nil},
		{"update with invalid category", (*DefectHandler).UpdateDefect, "some-id", `{"category":"not_a_valid_category"}`, http.StatusBadRequest, nil},
		{"bulk update without ids", (*DefectHandler).BulkUpdateDefects, "", `{"defect_ids":[],"resolution":"fixed"}`, http.StatusBadRequest, nil},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/defects", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("project_id", "1")
			req.SetPathValue("build_id", "1")
			req.SetPathValue("defect_id", tc.defectID)
			rr := httptest.NewRecorder()
			tc.endpoint(h, rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp["data"] == nil {
				t.Fatal("response has no data")
			}
			if tc.check != nil {
				tc.check(t, resp)
			}
		})
	}
}
