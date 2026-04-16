package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

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

// TestStoreReport_CopiesOnlyVariableDirs verifies that StoreReport copies
// only data/, widgets/, and history/ — not static assets like index.html or plugins/.
func TestStoreReport_CopiesOnlyVariableDirs(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildNumber := 3

	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	makeFullLatestReport(t, latestDir)

	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildNumber); err != nil {
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
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), "myproject", 1); err != nil {
		t.Fatalf("StoreReport on empty latest: %v", err)
	}
}

// TestStoreReport_LatestNotExist verifies that StoreReport returns nil when
// the latest/ directory does not exist at all.
func TestStoreReport_LatestNotExist(t *testing.T) {
	dir := t.TempDir()
	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), "nonexistent", 1); err != nil {
		t.Fatalf("StoreReport when latest/ missing: %v", err)
	}
}

// TestStoreReport_MissingOptionalDir verifies that StoreReport gracefully skips
// variable dirs that are absent from latest/ and copies those that do exist.
func TestStoreReport_MissingOptionalDir(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildNumber := 2

	// Only data/ present — no widgets/ or history/.
	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "data", "results.json"), `{}`)

	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildNumber); err != nil {
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
// InsertBuild fails, the error is returned instead of swallowed.
func TestStoreAndPruneBuild_InsertBuildErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(10)
	slug := "err-proj"

	cfg := &config.Config{ProjectsPath: dir}
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()
	mocks.Builds.InsertBuildFn = func(_ context.Context, _ int64, _ int) error {
		return store.ErrBuildNotFound // any non-nil error
	}
	a := NewAllure(AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})

	err := a.storeAndPruneBuild(context.Background(), projectID, slug, slug, dir, 1, store.CIMetadata{}, nil)
	if err == nil {
		t.Fatal("expected error from storeAndPruneBuild when InsertBuild fails, got nil")
	}
	if !strings.Contains(err.Error(), "insert build") {
		t.Errorf("expected error containing %q, got: %v", "insert build", err)
	}
}

// TestRecordBuild_RecordsInDB verifies that recordBuild calls InsertBuild and
// the build is subsequently visible via ListBuilds.
func TestRecordBuild_RecordsInDB(t *testing.T) {
	dir := t.TempDir()
	projectID := int64(11)
	slug := "record-proj"

	cfg := &config.Config{ProjectsPath: dir}
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()

	// Configure ListBuilds to return the build that recordBuild should have inserted.
	expectedBuild := store.Build{ProjectID: projectID, BuildNumber: 1}
	mocks.Builds.ListBuildsFn = func(_ context.Context, _ int64) ([]store.Build, error) {
		return []store.Build{expectedBuild}, nil
	}

	a := NewAllure(AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})

	if err := a.recordBuild(context.Background(), projectID, slug, 1); err != nil {
		t.Fatalf("recordBuild: %v", err)
	}

	builds, err := a.buildStore.ListBuilds(context.Background(), projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if builds[0].BuildNumber != 1 {
		t.Errorf("expected build_number=1, got %d", builds[0].BuildNumber)
	}
}

// TestStoreReport_WidgetsReadable verifies that after a partial store,
// readBuildStats can still read statistics from the copied widgets/ dir.
func TestStoreReport_WidgetsReadable(t *testing.T) {
	dir := t.TempDir()
	projectID := "myproject"
	buildNumber := 1

	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "widgets", "summary.json"),
		summaryJSON(10, 8, 1, 0, 1, 0))

	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildNumber); err != nil {
		t.Fatalf("StoreReport: %v", err)
	}

	stats, err := a.store.ReadBuildStats(context.Background(), projectID, buildNumber)
	if err != nil {
		t.Fatalf("ReadBuildStats after partial store: %v", err)
	}
	if stats.Total != 10 || stats.Passed != 8 {
		t.Errorf("unexpected stats: total=%d passed=%d (want total=10 passed=8)",
			stats.Total, stats.Passed)
	}
}
