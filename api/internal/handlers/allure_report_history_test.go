package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// newTestAllureHandler creates an AllureHandler backed by a real SQLite store
// (in-memory via t.TempDir) and syncs the given projectsDir into it so that
// filesystem fixtures created before calling this function are reflected in the DB.
func newTestAllureHandler(t *testing.T, projectsDir string) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	st := storage.NewLocalStore(cfg)

	// Sync filesystem fixtures → DB so numbered reports are visible.
	if err := store.SyncMetadata(context.Background(), st, db); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	buildStore := store.NewBuildStore(db)
	lockManager := store.NewLockManager()
	r := runner.NewAllure(cfg, st, buildStore, lockManager)
	return NewAllureHandler(cfg, r, store.NewProjectStore(db), buildStore, st)
}

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

// writeSummaryJSON creates widgets/summary.json under the given report dir.
func writeSummaryJSON(t *testing.T, reportDir string, content string) {
	t.Helper()
	widgetsDir := filepath.Join(reportDir, "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "summary.json"), []byte(content), 0o644); err != nil { //nolint:gosec // G306: test helper uses standard file permissions
		t.Fatal(err)
	}
}

// ---- tests ------------------------------------------------------------------

func TestGetReportHistory_EmptyDir(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj1"
	// Create project dir but no reports subdir
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
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

	// newTestAllureHandler calls SyncFromFilesystem which imports builds 1 and 3.
	h := newTestAllureHandler(t, projectsDir)
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
	if err := os.MkdirAll(reportDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)
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
	if err := os.MkdirAll(filepath.Join(projectsDir, projectID), 0o755); err != nil { //nolint:gosec // G301: test fixture
		t.Fatal(err)
	}

	h := newTestAllureHandler(t, projectsDir)

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

func TestGetReportHistory_InvalidProjectID(t *testing.T) {
	projectsDir := t.TempDir()
	h := newTestAllureHandler(t, projectsDir)

	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, makeGetReportHistoryReq(t, "../evil"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid project_id, got %d", rr.Code)
	}
}
