package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

// TestReportHandler_Widgets serves the categories and environment widgets of
// a report; a missing widget file is an empty list, not an error.
func TestReportHandler_Widgets(t *testing.T) {
	t.Parallel()
	categories, environment := (*ReportHandler).GetReportCategories, (*ReportHandler).GetReportEnvironment
	const (
		twoCategories = `[{"name":"Product defects","matchedStatistic":{"failed":3,"total":3}},{"name":"Test defects","matchedStatistic":{"broken":2,"total":2}}]`
		oneEnvEntry   = `[{"name":"Version","values":["1.2.3"]}]`
	)
	rows := []struct {
		name     string
		endpoint func(*ReportHandler, http.ResponseWriter, *http.Request)
		files    map[string]string // relative to the project's reports dir
		project  string            // project_id path value; "" uses the seeded project
		reportID string
		want     int
		wantLen  int
	}{
		{"categories", categories, map[string]string{"latest/widgets/categories.json": twoCategories}, "", "latest", http.StatusOK, 2},
		{"categories missing file", categories, nil, "", "latest", http.StatusOK, 0},
		{"categories invalid project id", categories, nil, "../evil", "latest", http.StatusBadRequest, 0},
		{"environment of a numbered build", environment, map[string]string{"5/widgets/environment.json": oneEnvEntry}, "", "5", http.StatusOK, 1},
		{"environment missing file", environment, nil, "", "latest", http.StatusOK, 0},
		{"environment invalid project id", environment, nil, "../evil", "latest", http.StatusBadRequest, 0},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeReportTree(t, filepath.Join(dir, "proj", "reports"), tc.files)
			h, mocks := newTestReportHandler(t, dir)
			p, _ := mocks.Projects.CreateProject(context.Background(), "proj")
			project := tc.project
			if project == "" {
				project = strconv.FormatInt(p.ID, 10)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project+"/reports/"+tc.reportID, nil)
			req.SetPathValue("project_id", project)
			req.SetPathValue("report_id", tc.reportID)
			rr := httptest.NewRecorder()
			tc.endpoint(h, rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			var resp struct {
				Data []any `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Data == nil || len(resp.Data) != tc.wantLen {
				t.Errorf("data = %v, want an array of %d", resp.Data, tc.wantLen)
			}
		})
	}
}
