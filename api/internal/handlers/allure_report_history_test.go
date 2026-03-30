package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func makeGetReportHistoryReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/projects/"+projectID+"/reports",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	return req
}

// ---- tests ------------------------------------------------------------------

func TestGetReportHistory_EmptyDir(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj1"
	// Create project dir but no reports subdir
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	h, _ := newTestReportHandler(t, projectsDir)
	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected resp[\"data\"] to be map[string]any")
	}
	reports, ok := data["reports"].([]any)
	if !ok {
		t.Fatal("expected data[\"reports\"] to be []any")
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestGetReportHistory_MultipleReports(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj2"
	reportsDir := filepath.Join(projectsDir, projectID, "reports")

	// Create numbered reports 1 and 3 plus latest
	for _, name := range []string{"1", "3", "latest"} {
		dir := filepath.Join(reportsDir, name)
		summary := `{"statistic":{"passed":5,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":6},"time":{"stop":1700000000000,"duration":3000}}`
		writeSummaryJSON(t, dir, summary)
	}

	// newTestReportHandler calls syncTestBuildsFromFilesystem which imports builds 1 and 3.
	h, _ := newTestReportHandler(t, projectsDir)
	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected resp[\"data\"] to be map[string]any")
	}
	reports, ok := data["reports"].([]any)
	if !ok {
		t.Fatal("expected data[\"reports\"] to be []any")
	}

	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}

	// First entry must be latest (from filesystem)
	first, ok := reports[0].(map[string]any)
	if !ok {
		t.Fatal("expected reports[0] to be map[string]any")
	}
	if first["report_id"] != "latest" {
		t.Errorf("expected first report_id='latest', got %v", first["report_id"])
	}
	if first["is_latest"] != true {
		t.Errorf("expected is_latest=true for latest entry")
	}

	// Numbered reports must be in descending order: 3, 1
	second, ok2 := reports[1].(map[string]any)
	if !ok2 {
		t.Fatal("expected reports[1] to be map[string]any")
	}
	third, ok3 := reports[2].(map[string]any)
	if !ok3 {
		t.Fatal("expected reports[2] to be map[string]any")
	}
	if second["report_id"] != "3" {
		t.Errorf("expected second report_id='3', got %v", second["report_id"])
	}
	if third["report_id"] != "1" {
		t.Errorf("expected third report_id='1', got %v", third["report_id"])
	}
}

func TestGetReportHistory_MissingSummaryJSON(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj3"
	// Create a report dir without any summary.json
	reportDir := filepath.Join(projectsDir, projectID, "reports", "1")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatal(err)
	}

	h, _ := newTestReportHandler(t, projectsDir)
	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected resp[\"data\"] to be map[string]any")
	}
	reports, ok := data["reports"].([]any)
	if !ok {
		t.Fatal("expected data[\"reports\"] to be []any")
	}

	if len(reports) != 1 {
		t.Fatalf("expected 1 report entry even without summary.json, got %d", len(reports))
	}
	entry, ok := reports[0].(map[string]any)
	if !ok {
		t.Fatal("expected reports[0] to be map[string]any")
	}
	if entry["statistic"] != nil {
		t.Errorf("expected nil statistic when summary.json is missing, got %v", entry["statistic"])
	}
}

func TestGetReportHistory_CancelledContext(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj-cancel"
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil {
		t.Fatal(err)
	}

	h, _ := newTestReportHandler(t, projectsDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately — handler should propagate this to DB queries

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/report-history?project_id="+projectID, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("expected non-200 status with cancelled context, got %d", rr.Code)
	}
}

func TestGetReportHistory_WithCIMetadata(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "ci-metadata-proj"
	reportsDir := filepath.Join(projectsDir, projectID, "reports")

	// Create a numbered report dir for SyncMetadata to import.
	dir := filepath.Join(reportsDir, "1")
	summary := `{"statistic":{"passed":5,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":6},"time":{"stop":1700000000000,"duration":3000}}`
	writeSummaryJSON(t, dir, summary)

	h, _ := newTestReportHandler(t, projectsDir)

	// Set CI metadata on the imported build via the handler's buildStore.
	ctx := context.Background()
	ciMeta := store.CIMetadata{
		Provider:  "GitHub Actions",
		BuildURL:  "https://github.com/org/repo/actions/runs/123",
		Branch:    "main",
		CommitSHA: "abc1234",
	}
	if err := h.buildStore.UpdateBuildCIMetadata(ctx, projectID, 1, ciMeta); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data map")
	}
	reports, ok := data["reports"].([]any)
	if !ok {
		t.Fatal("expected reports slice")
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	entry, ok := reports[0].(map[string]any)
	if !ok {
		t.Fatal("expected entry map")
	}
	if entry["ci_provider"] != "GitHub Actions" {
		t.Errorf("ci_provider: got %v, want 'GitHub Actions'", entry["ci_provider"])
	}
	if entry["ci_build_url"] != "https://github.com/org/repo/actions/runs/123" {
		t.Errorf("ci_build_url: got %v", entry["ci_build_url"])
	}
	if entry["ci_branch"] != "main" {
		t.Errorf("ci_branch: got %v, want 'main'", entry["ci_branch"])
	}
	if entry["ci_commit_sha"] != "abc1234" {
		t.Errorf("ci_commit_sha: got %v, want 'abc1234'", entry["ci_commit_sha"])
	}
}

func TestGetReportHistory_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestReportHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, "../evil"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid project_id, got %d", rr.Code)
	}
}

func TestGetReportHistory_Allure3LatestWithTiming(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "allure3-timing"

	// Allure 3 "latest" report: statistic.json only (no summary.json, no timing)
	latestDir := filepath.Join(projectsDir, projectID, "reports", "latest")
	widgetsDir := filepath.Join(latestDir, "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "statistic.json"),
		[]byte(`{"passed":3,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":4}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test result files with start/stop epoch milliseconds
	// min(start)=1700000000000, max(stop)=1700000005000 → duration=5000ms
	testResultsDir := filepath.Join(latestDir, "data", "test-results")
	if err := os.MkdirAll(testResultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testResultsDir, "a.json"),
		[]byte(`{"start":1700000000000,"stop":1700000002000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testResultsDir, "b.json"),
		[]byte(`{"start":1700000001000,"stop":1700000005000}`), 0o644); err != nil {
		t.Fatal(err)
	}

	h, _ := newTestReportHandler(t, projectsDir)
	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, projectID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data map")
	}
	reports, ok := data["reports"].([]any)
	if !ok || len(reports) == 0 {
		t.Fatalf("expected at least one report, got: %v", reports)
	}
	entry, ok := reports[0].(map[string]any)
	if !ok {
		t.Fatal("expected entry map")
	}
	if entry["is_latest"] != true {
		t.Errorf("expected is_latest=true for first entry")
	}
	// duration_ms = sum of individual test durations = 2000 + 4000 = 6000
	durMs, ok := entry["duration_ms"].(float64)
	if !ok || durMs != 6000 {
		t.Errorf("duration_ms: got %v, want 6000", entry["duration_ms"])
	}
	// generated_at must be a non-empty RFC3339 string
	genAt, ok := entry["generated_at"].(string)
	if !ok || genAt == "" {
		t.Errorf("expected non-empty generated_at, got %v", entry["generated_at"])
	}
}
