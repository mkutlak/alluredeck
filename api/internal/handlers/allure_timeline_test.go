package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetReportTimeline_LatestReport(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "timelineproj"
	resultsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "data", "test-results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fixtures := []string{
		`{"name":"Login test","fullName":"com.example.LoginTest#test","status":"passed","time":{"start":1700000000000,"stop":1700000005000,"duration":5000},"labels":[{"name":"thread","value":"worker-1"},{"name":"host","value":"node-1"}]}`,
		`{"name":"Logout test","fullName":"com.example.LogoutTest#test","status":"failed","time":{"start":1700000001000,"stop":1700000003000,"duration":2000},"labels":[{"name":"thread","value":"worker-2"},{"name":"host","value":"node-1"}]}`,
		`{"name":"Profile test","fullName":"com.example.ProfileTest#test","status":"broken","time":{"start":1700000002000,"stop":1700000006000,"duration":4000},"labels":[{"name":"thread","value":"worker-1"},{"name":"host","value":"node-2"}]}`,
	}
	for i, f := range fixtures {
		if err := os.WriteFile(filepath.Join(resultsDir, fmt.Sprintf("test-%d.json", i)), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/timelineproj/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}
	testCases, ok := data["test_cases"].([]any)
	if !ok {
		t.Fatalf("expected test_cases to be array, got %T", data["test_cases"])
	}
	if len(testCases) != 3 {
		t.Fatalf("expected 3 test cases, got %d", len(testCases))
	}
	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be object, got %T", data["summary"])
	}
	if total, _ := summary["total"].(float64); int(total) != 3 {
		t.Errorf("expected summary.total=3, got %v", summary["total"])
	}
	if truncated, _ := summary["truncated"].(bool); truncated {
		t.Errorf("expected truncated=false")
	}
	// Verify first test case has thread/host labels extracted
	tc0 := testCases[0].(map[string]any)
	if tc0["thread"] == "" && tc0["host"] == "" {
		t.Errorf("expected thread or host to be populated")
	}
}

func TestGetReportTimeline_TopLevelStartStop(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "rawfmt"
	resultsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "data", "test-results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Top-level start/stop format (raw results, no nested "time" object)
	raw := `{"name":"Raw test","status":"passed","start":1700000000000,"stop":1700000002000}`
	if err := os.WriteFile(filepath.Join(resultsDir, "raw.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/rawfmt/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(testCases))
	}
	tc := testCases[0].(map[string]any)
	if start, _ := tc["start"].(float64); int64(start) != 1700000000000 {
		t.Errorf("expected start=1700000000000, got %v", tc["start"])
	}
	if dur, _ := tc["duration"].(float64); int64(dur) != 2000 {
		t.Errorf("expected duration=2000, got %v", tc["duration"])
	}
}

func TestGetReportTimeline_EmptyDir(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "emptyproj"
	resultsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "data", "test-results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/emptyproj/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 0 {
		t.Fatalf("expected 0 test cases, got %d", len(testCases))
	}
}

func TestGetReportTimeline_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "../evil")
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestGetReportTimeline_MissingDir(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "nodirproj"
	// Create project dir but no reports subdirectory
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/nodirproj/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 0 {
		t.Fatalf("expected 0 test cases on missing dir, got %d", len(testCases))
	}
}

func TestGetReportTimeline_DBFastPath(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "dbproj"

	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, pid string, buildOrder int) (int64, error) {
		if pid == projectID && buildOrder == 5 {
			return int64(100), nil
		}
		return 0, nil
	}
	mocks.TestResults.ListTimelineFn = func(_ context.Context, pid string, buildID int64, _ int) ([]store.TimelineRow, error) {
		if pid == projectID && buildID == int64(100) {
			return []store.TimelineRow{
				{TestName: "Login", FullName: "com.Login", Status: "passed", StartMs: 1700000000000, StopMs: 1700000005000, Thread: "t-1", Host: "node-1"},
				{TestName: "Logout", FullName: "com.Logout", Status: "failed", StartMs: 1700000001000, StopMs: 1700000003000, Thread: "t-2", Host: "node-1"},
			}, nil
		}
		return nil, nil
	}

	h := newTestAllureHandlerWithMocks(t, projectsDir, mocks)

	// Request with numeric report_id "5" — should hit DB fast path.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/dbproj/reports/5/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "5")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 2 {
		t.Fatalf("expected 2 test cases from DB, got %d", len(testCases))
	}

	// Verify ordering: Login before Logout (ordered by start time ascending).
	tc0 := testCases[0].(map[string]any)
	tc1 := testCases[1].(map[string]any)
	if tc0["name"] != "Login" {
		t.Errorf("first test should be Login, got %v", tc0["name"])
	}
	if tc1["name"] != "Logout" {
		t.Errorf("second test should be Logout, got %v", tc1["name"])
	}

	// Verify thread/host fields.
	if thread, _ := tc0["thread"].(string); thread != "t-1" {
		t.Errorf("thread = %q, want %q", thread, "t-1")
	}
	if host, _ := tc0["host"].(string); host != "node-1" {
		t.Errorf("host = %q, want %q", host, "node-1")
	}

	// Verify summary.
	summary := data["summary"].(map[string]any)
	if total, _ := summary["total"].(float64); int(total) != 2 {
		t.Errorf("expected total=2, got %v", summary["total"])
	}

	// Verify duration is computed from stop - start.
	if dur, _ := tc0["duration"].(float64); int64(dur) != 5000 {
		t.Errorf("expected duration=5000, got %v", dur)
	}
}

func TestGetReportTimeline_FallbackToS3(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "fallbackproj"

	// Create report files for "latest" — S3 path should serve these.
	resultsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "data", "test-results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"name":"S3 test","fullName":"com.S3Test","status":"passed","time":{"start":1700000000000,"stop":1700000001000,"duration":1000}}`
	if err := os.WriteFile(filepath.Join(resultsDir, "test.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)

	// "latest" is non-numeric — should fall back to S3.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/fallbackproj/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 1 {
		t.Fatalf("expected 1 test case from S3 fallback, got %d", len(testCases))
	}
}

func TestGetReportTimeline_Truncation(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "truncproj"
	resultsDir := filepath.Join(projectsDir, projectID, "reports", "latest", "data", "test-results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create 5001 test result files to trigger truncation
	for i := range 5001 {
		content := fmt.Sprintf(
			`{"name":"Test %d","status":"passed","time":{"start":%d,"stop":%d,"duration":1000}}`,
			i, 1700000000000+int64(i)*1000, 1700000001000+int64(i)*1000,
		)
		if err := os.WriteFile(
			filepath.Join(resultsDir, fmt.Sprintf("test-%05d.json", i)),
			[]byte(content), 0o644,
		); err != nil {
			t.Fatal(err)
		}
	}

	h := newTestAllureHandler(t, projectsDir)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/truncproj/reports/latest/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	testCases := data["test_cases"].([]any)
	if len(testCases) != 5000 {
		t.Fatalf("expected 5000 test cases (truncated), got %d", len(testCases))
	}
	summary := data["summary"].(map[string]any)
	if truncated, _ := summary["truncated"].(bool); !truncated {
		t.Errorf("expected truncated=true")
	}
	if total, _ := summary["total"].(float64); int(total) != 5001 {
		t.Errorf("expected summary.total=5001, got %v", summary["total"])
	}
}
