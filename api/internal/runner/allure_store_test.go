package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// mustWriteFile creates parent dirs and writes content to path.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: test helper needs 0o755 to create temp directories
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test helper uses standard file permissions
		t.Fatalf("write %s: %v", path, err)
	}
}

// summaryJSON returns a minimal widgets/summary.json payload.
func summaryJSON(total, passed, failed, broken, skipped, unknown int) string {
	type statistic struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Broken  int `json:"broken"`
		Skipped int `json:"skipped"`
		Unknown int `json:"unknown"`
	}
	data, _ := json.Marshal(map[string]any{
		"statistic": statistic{
			Total: total, Passed: passed, Failed: failed,
			Broken: broken, Skipped: skipped, Unknown: unknown,
		},
	})
	return string(data)
}

// makeFullLatestReport populates latest/ with both static and variable content.
func makeFullLatestReport(t *testing.T, latestDir string) {
	t.Helper()
	// Static assets — should NOT be copied by partial StoreReport.
	for _, f := range []string{"index.html", "app.js", "styles.css"} {
		mustWriteFile(t, filepath.Join(latestDir, f), "static")
	}
	mustWriteFile(t, filepath.Join(latestDir, "plugins", "plugin.js"), "plugin")

	// Variable dirs — MUST be copied.
	mustWriteFile(t, filepath.Join(latestDir, "data", "results.json"), `{"total":5}`)
	mustWriteFile(t, filepath.Join(latestDir, "widgets", "summary.json"),
		summaryJSON(5, 3, 1, 0, 0, 1))
	mustWriteFile(t, filepath.Join(latestDir, "history", "history.json"), `[]`)
}

// newTestAllureDir constructs an Allure instance pointed at projectsDir.
func newTestAllureDir(t *testing.T, projectsDir string) *Allure {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir}
	st := storage.NewLocalStore(cfg)
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	bs := store.NewBuildStore(s)
	lm := store.NewLockManager()
	return NewAllure(cfg, st, bs, lm)
}

// TestStoreReport_CopiesOnlyVariableDirs verifies that StoreReport copies
// only data/, widgets/, and history/ — not static assets like index.html or plugins/.
func TestStoreReport_CopiesOnlyVariableDirs(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildOrder := 3

	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	makeFullLatestReport(t, latestDir)

	a := newTestAllureDir(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildOrder); err != nil {
		t.Fatalf("StoreReport: %v", err)
	}

	buildDir := filepath.Join(dir, projectID, "reports", "3")

	// Variable dirs must be present.
	for _, d := range []string{"data", "widgets", "history"} {
		if _, err := os.Stat(filepath.Join(buildDir, d)); os.IsNotExist(err) {
			t.Errorf("expected %s/ in build dir, not found", d)
		}
	}

	// Static files must NOT be present.
	for _, f := range []string{"index.html", "app.js", "styles.css"} {
		if _, err := os.Stat(filepath.Join(buildDir, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s NOT in build dir, but it exists", f)
		}
	}
	if _, err := os.Stat(filepath.Join(buildDir, "plugins")); !os.IsNotExist(err) {
		t.Error("expected plugins/ NOT in build dir, but it exists")
	}
}

// TestStoreReport_EmptyLatest verifies that StoreReport is a no-op when
// latest/ exists but is empty.
func TestStoreReport_EmptyLatest(t *testing.T) {
	dir := t.TempDir()
	latestDir := filepath.Join(dir, "myproject", "reports", "latest")
	if err := os.MkdirAll(latestDir, 0o755); err != nil { //nolint:gosec // G301: test setup needs readable temp directory
		t.Fatal(err)
	}

	a := newTestAllureDir(t, dir)
	if err := a.StoreReport(context.Background(), "myproject", 1); err != nil {
		t.Fatalf("StoreReport on empty latest: %v", err)
	}
}

// TestStoreReport_LatestNotExist verifies that StoreReport returns nil when
// the latest/ directory does not exist at all.
func TestStoreReport_LatestNotExist(t *testing.T) {
	dir := t.TempDir()
	a := newTestAllureDir(t, dir)
	if err := a.StoreReport(context.Background(), "nonexistent", 1); err != nil {
		t.Fatalf("StoreReport when latest/ missing: %v", err)
	}
}

// TestStoreReport_MissingOptionalDir verifies that StoreReport gracefully skips
// variable dirs that are absent from latest/ and copies those that do exist.
func TestStoreReport_MissingOptionalDir(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildOrder := 2

	// Only data/ present — no widgets/ or history/.
	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "data", "results.json"), `{}`)

	a := newTestAllureDir(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildOrder); err != nil {
		t.Fatalf("StoreReport with partial dirs: %v", err)
	}

	buildDir := filepath.Join(dir, projectID, "reports", "2")
	if _, err := os.Stat(filepath.Join(buildDir, "data")); os.IsNotExist(err) {
		t.Error("data/ should be copied into build dir")
	}
	for _, d := range []string{"widgets", "history"} {
		if _, err := os.Stat(filepath.Join(buildDir, d)); !os.IsNotExist(err) {
			t.Errorf("%s/ should not exist (was absent from latest/)", d)
		}
	}
}

// TestStoreAndPruneBuild_InsertBuildErrorPropagates verifies that when
// InsertBuild fails (e.g. closed DB), the error is returned instead of swallowed.
func TestStoreAndPruneBuild_InsertBuildErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	projectID := "err-proj"

	cfg := &config.Config{ProjectsDirectory: dir}
	st := storage.NewLocalStore(cfg)
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	bs := store.NewBuildStore(s)
	lm := store.NewLockManager()
	a := NewAllure(cfg, st, bs, lm)

	// Close the DB so InsertBuild will fail.
	_ = s.Close()

	err = a.storeAndPruneBuild(context.Background(), projectID, dir, 1)
	if err == nil {
		t.Fatal("expected error from storeAndPruneBuild when DB is closed, got nil")
	}
	if !strings.Contains(err.Error(), "insert build") {
		t.Errorf("expected error containing %q, got: %v", "insert build", err)
	}
}

// TestRecordBuild_RecordsInDB verifies that recordBuild inserts the build
// into the database (for pruning) without publishing a report snapshot.
func TestRecordBuild_RecordsInDB(t *testing.T) {
	dir := t.TempDir()
	projectID := "record-proj"

	cfg := &config.Config{ProjectsDirectory: dir}
	st := storage.NewLocalStore(cfg)
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Create project in DB to satisfy FK constraint.
	ps := store.NewProjectStore(s)
	if err := ps.CreateProject(context.Background(), projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	bs := store.NewBuildStore(s)
	lm := store.NewLockManager()
	a := NewAllure(cfg, st, bs, lm)

	if err := a.recordBuild(context.Background(), projectID, 1); err != nil {
		t.Fatalf("recordBuild: %v", err)
	}

	builds, err := a.buildStore.ListBuilds(context.Background(), projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if builds[0].BuildOrder != 1 {
		t.Errorf("expected build_order=1, got %d", builds[0].BuildOrder)
	}
}

// TestStoreReport_WidgetsReadable verifies that after a partial store,
// readBuildStats can still read statistics from the copied widgets/ dir.
func TestStoreReport_WidgetsReadable(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildOrder := 1

	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "widgets", "summary.json"),
		summaryJSON(10, 8, 1, 0, 1, 0))

	a := newTestAllureDir(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildOrder); err != nil {
		t.Fatalf("StoreReport: %v", err)
	}

	stats, err := a.store.ReadBuildStats(context.Background(), projectID, buildOrder)
	if err != nil {
		t.Fatalf("ReadBuildStats after partial store: %v", err)
	}
	if stats.Total != 10 || stats.Passed != 8 {
		t.Errorf("unexpected stats: total=%d passed=%d (want total=10 passed=8)",
			stats.Total, stats.Passed)
	}
}
