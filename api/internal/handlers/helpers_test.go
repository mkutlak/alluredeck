package handlers

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// openTestStore creates a temporary SQLiteStore and registers cleanup.
func openTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// openTestDB creates a temporary *sql.DB for tests that need raw DB access (e.g. system_test).
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openTestStore(t).DB()
}

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

	logger := zap.NewNop()
	st := storage.NewLocalStore(cfg)

	// Sync filesystem fixtures → DB so numbered reports are visible.
	if err := store.SyncMetadata(context.Background(), st, db, logger); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	buildStore := store.NewBuildStore(db, logger)
	lockManager := store.NewLockManager()
	r := runner.NewAllure(cfg, st, buildStore, lockManager, nil, logger)
	return NewAllureHandler(cfg, r, nil,
		store.NewProjectStore(db, logger), buildStore, store.NewKnownIssueStore(db),
		nil, store.NewSearchStore(db, logger), st)
}

// newTestAllureHandlerWithDB creates an AllureHandler wired to an externally-provided
// SQLiteStore. Use this when tests pre-seed DB state before handler construction.
// Includes a TestResultStore; the caller controls the full DB lifecycle.
func newTestAllureHandlerWithDB(t *testing.T, projectsDir string, db *store.SQLiteStore) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	bs := store.NewBuildStore(db, logger)
	lockManager := store.NewLockManager()
	ts := store.NewTestResultStore(db, logger)
	r := runner.NewAllure(cfg, st, bs, lockManager, ts, logger)
	return NewAllureHandler(cfg, r, nil,
		store.NewProjectStore(db, logger), bs, store.NewKnownIssueStore(db), ts, nil, st)
}

// newTestAllureHandlerWithJobManager builds an AllureHandler with a real JobManager
// backed by the provided generator, for async job handler tests.
func newTestAllureHandlerWithJobManager(t *testing.T, projectsDir string, gen runner.ReportGenerator) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: projectsDir, KeepHistory: false}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := zap.NewNop()
	st := storage.NewLocalStore(cfg)
	buildStore := store.NewBuildStore(db, logger)
	lockManager := store.NewLockManager()
	r := runner.NewAllure(cfg, st, buildStore, lockManager, nil, logger)

	jm := runner.NewJobManager(gen, 2, logger)
	jm.Start(context.Background())
	t.Cleanup(func() { jm.Shutdown() })

	return NewAllureHandler(cfg, r, jm,
		store.NewProjectStore(db, logger), buildStore, store.NewKnownIssueStore(db), nil, nil, st)
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
