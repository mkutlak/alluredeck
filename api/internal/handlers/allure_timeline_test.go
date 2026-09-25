package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestGetReportTimeline covers both sources: generated report files for a
// handler without a test-result store, and the database fast path, which also
// resolves "latest" to the newest build.
func TestGetReportTimeline(t *testing.T) {
	const reportResult = `{"name":%q,"status":"passed","time":{"start":%d,"stop":%d,"duration":%d},"labels":[{"name":"thread","value":%q},{"name":"host","value":"node-1"}]}`
	tests := []struct {
		name      string
		db        bool
		projectID string            // "" targets the seeded project
		files     map[string]string // reports/latest/data/test-results; nil means no directory
		reportID  string
		want      int
		wantJSON  map[string]any
	}{
		// Cases come back sorted by start, with thread/host taken from labels.
		{name: "generated report files", files: map[string]string{
			"a.json": fmt.Sprintf(reportResult, "Logout test", 1700000001000, 1700000003000, 2000, "worker-2"),
			"b.json": fmt.Sprintf(reportResult, "Login test", 1700000000000, 1700000005000, 5000, "worker-1"),
			"c.json": fmt.Sprintf(reportResult, "Profile test", 1700000002000, 1700000006000, 4000, "worker-1"),
		}, reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{
			"data.test_cases#": 3, "data.summary.total": 3, "data.summary.truncated": false,
			"data.test_cases.0.name": "Login test", "data.test_cases.0.thread": "worker-1", "data.test_cases.0.host": "node-1",
		}},
		// Raw results carry top-level start/stop instead of a "time" object.
		{name: "raw results", files: map[string]string{
			"raw.json": `{"name":"Raw test","status":"passed","start":1700000000000,"stop":1700000002000}`,
		}, reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{
			"data.test_cases.0.start": int64(1700000000000), "data.test_cases.0.duration": 2000,
		}},
		{name: "empty results dir", files: map[string]string{}, reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{"data.test_cases#": 0}},
		{name: "missing results dir", reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{"data.test_cases#": 0}},
		{name: "invalid project id", projectID: "../evil", reportID: "latest", want: http.StatusBadRequest},
		{name: "db numeric report id", db: true, reportID: "5", want: http.StatusOK, wantJSON: map[string]any{
			"data.test_cases#": 2, "data.summary.total": 2,
			"data.test_cases.0.name": "Login", "data.test_cases.1.name": "Logout",
			"data.test_cases.0.thread": "t-1", "data.test_cases.0.host": "node-1", "data.test_cases.0.duration": 5000,
		}},
		{name: "db latest resolves to newest build", db: true, reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{
			"data.test_cases#": 1, "data.test_cases.0.name": "ResolvedTest",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectsDir := t.TempDir()
			var h *ReportHandler
			var mocks *testutil.MockStores
			if tc.db {
				mocks = testutil.New()
				mocks.Builds.GetLatestBuildFn = func(context.Context, int64) (store.Build, error) { return store.Build{BuildNumber: 7}, nil }
				mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, n int) (int64, error) {
					return map[int]int64{5: 100, 7: 200}[n], nil
				}
				mocks.TestResults.ListTimelineFn = func(_ context.Context, _ int64, buildID int64, _ int) ([]store.TimelineRow, error) {
					return map[int64][]store.TimelineRow{
						100: {
							{TestName: "Login", Status: "passed", StartMs: 1700000000000, StopMs: 1700000005000, Thread: "t-1", Host: "node-1"},
							{TestName: "Logout", Status: "failed", StartMs: 1700000001000, StopMs: 1700000003000, Thread: "t-2", Host: "node-1"},
						},
						200: {{TestName: "ResolvedTest", Status: "passed", StartMs: 1700000000000, StopMs: 1700000002000}},
					}[buildID], nil
				}
				h = newTestReportHandlerWithMocks(t, projectsDir, mocks)
			} else {
				h, mocks = newTestReportHandler(t, projectsDir)
			}
			proj, err := mocks.Projects.CreateProject(context.Background(), "timelineproj")
			if err != nil {
				t.Fatal(err)
			}
			resultsDir := filepath.Join(projectsDir, "timelineproj", "reports", "latest", "data", "test-results")
			writeTimelineResults(t, resultsDir, tc.files)
			id := tc.projectID
			if id == "" {
				id = strconv.FormatInt(proj.ID, 10)
			}
			code, body := serveJSON(t, h.GetReportTimeline, http.MethodGet, "/api/v1/projects/"+id+"/reports/"+tc.reportID+"/timeline", "",
				"project_id", id, "report_id", tc.reportID)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
		})
	}
}

// TestGetReportTimeline_Truncation caps the timeline at timelineMaxItems while
// the summary still counts every result.
func TestGetReportTimeline_Truncation(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)
	proj, err := mocks.Projects.CreateProject(context.Background(), "truncproj")
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string, timelineMaxItems+1)
	for i := range timelineMaxItems + 1 {
		start := 1700000000000 + int64(i)*1000
		files[fmt.Sprintf("test-%05d.json", i)] = fmt.Sprintf(`{"name":"Test %d","status":"passed","time":{"start":%d,"stop":%d,"duration":1000}}`, i, start, start+1000)
	}
	writeTimelineResults(t, filepath.Join(projectsDir, "truncproj", "reports", "latest", "data", "test-results"), files)
	id := strconv.FormatInt(proj.ID, 10)
	code, body := serveJSON(t, h.GetReportTimeline, http.MethodGet, "/api/v1/projects/"+id+"/reports/latest/timeline", "",
		"project_id", id, "report_id", "latest")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	wantJSON(t, body, map[string]any{"data.test_cases#": timelineMaxItems, "data.summary.truncated": true, "data.summary.total": timelineMaxItems + 1})
}

// writeTimelineResults writes result files into dir; nil files leaves dir absent.
func writeTimelineResults(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if files == nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
