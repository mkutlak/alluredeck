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

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
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

func TestGetReportTimeline_SQLiteFastPath(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "sqliteproj"

	// Create project dir (no report files — SQLite should serve data).
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectsDirectory: projectsDir}
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	st := storage.NewLocalStore(cfg)
	bs := store.NewBuildStore(db, zap.NewNop())
	ps := store.NewProjectStore(db, zap.NewNop())
	ts := store.NewTestResultStore(db, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, projectID)
	_ = bs.InsertBuild(ctx, projectID, 5)
	buildID, _ := ts.GetBuildID(ctx, projectID, 5)

	start1, stop1 := int64(1700000000000), int64(1700000005000)
	start2, stop2 := int64(1700000001000), int64(1700000003000)
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildID, ProjectID: projectID, TestName: "Login", FullName: "com.Login", Status: "passed", DurationMs: 5000, HistoryID: "h1", StartMs: &start1, StopMs: &stop1, Thread: "t-1", Host: "node-1"},
		{BuildID: buildID, ProjectID: projectID, TestName: "Logout", FullName: "com.Logout", Status: "failed", DurationMs: 2000, HistoryID: "h2", StartMs: &start2, StopMs: &stop2, Thread: "t-2", Host: "node-1"},
	})

	lockManager := store.NewLockManager()
	r := runner.NewAllure(cfg, st, bs, lockManager, ts, zap.NewNop())
	h := NewAllureHandler(cfg, r, nil, ps, bs, store.NewKnownIssueStore(db), ts, st)

	// Request with numeric report_id "5" — should hit SQLite fast path.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/sqliteproj/reports/5/timeline", nil)
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
		t.Fatalf("expected 2 test cases from SQLite, got %d", len(testCases))
	}

	// Verify ordering: Logout (start=1700000001000) after Login (start=1700000000000).
	tc0 := testCases[0].(map[string]any)
	tc1 := testCases[1].(map[string]any)
	if tc0["name"] != "Login" {
		t.Errorf("first test should be Login, got %v", tc0["name"])
	}
	if tc1["name"] != "Logout" {
		t.Errorf("second test should be Logout, got %v", tc1["name"])
	}

	// Verify timeline fields.
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

	// Verify duration is computed.
	if dur, _ := tc0["duration"].(float64); int64(dur) != 5000 {
		t.Errorf("expected duration=5000, got %v", dur)
	}
}

func TestGetReportTimeline_SQLiteFallbackToS3(t *testing.T) {
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
	for i := 0; i < 5001; i++ {
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
