package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestAllureHandler creates an AllureHandler backed by stateful in-memory
// stores. After construction it scans projectsDir and populates the build store
// with any numbered report directories found, so GetReportHistory tests see the
// expected DB-backed entries without a real PostgreSQL connection.
func newTestAllureHandler(t *testing.T, projectsDir string) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.MemBuilds, mocks.Locker, nil, nil, nil, logger)
	h := NewAllureHandler(cfg, r, nil,
		mocks.Projects, mocks.MemBuilds, mocks.KnownIssues, nil, mocks.Search, st, logger)
	syncTestBuildsFromFilesystem(t, projectsDir, mocks.MemBuilds)
	return h
}

// syncTestBuildsFromFilesystem scans projectsDir for numbered report directories
// and inserts them into builds, optionally populating stats from summary.json.
func syncTestBuildsFromFilesystem(t *testing.T, projectsDir string, builds *testutil.MemBuildStore) {
	t.Helper()
	projectEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}
	ctx := context.Background()
	for _, pe := range projectEntries {
		if !pe.IsDir() {
			continue
		}
		projectID := pe.Name()
		reportsDir := filepath.Join(projectsDir, projectID, "reports")
		reportEntries, err := os.ReadDir(reportsDir)
		if err != nil {
			continue
		}
		for _, re := range reportEntries {
			if !re.IsDir() {
				continue
			}
			order, err := strconv.Atoi(re.Name())
			if err != nil {
				continue // skip "latest" and any non-numeric dirs
			}
			if err := builds.InsertBuild(ctx, projectID, order); err != nil {
				t.Logf("syncTestBuildsFromFilesystem: InsertBuild %s/%d: %v", projectID, order, err)
				continue
			}
			// Parse summary.json for stats if present.
			data, err := os.ReadFile(filepath.Join(reportsDir, re.Name(), "widgets", "summary.json"))
			if err != nil {
				continue
			}
			var summary struct {
				Statistic struct {
					Passed  int `json:"passed"`
					Failed  int `json:"failed"`
					Broken  int `json:"broken"`
					Skipped int `json:"skipped"`
					Unknown int `json:"unknown"`
					Total   int `json:"total"`
				} `json:"statistic"`
				Time struct {
					Duration int64 `json:"duration"`
				} `json:"time"`
			}
			if json.Unmarshal(data, &summary) != nil {
				continue
			}
			_ = builds.UpdateBuildStats(ctx, projectID, order, store.BuildStats{
				Passed:     summary.Statistic.Passed,
				Failed:     summary.Statistic.Failed,
				Broken:     summary.Statistic.Broken,
				Skipped:    summary.Statistic.Skipped,
				Unknown:    summary.Statistic.Unknown,
				Total:      summary.Statistic.Total,
				DurationMs: summary.Time.Duration,
			})
		}
	}
}

// newTestAllureHandlerAndMocks is like newTestAllureHandler but also returns the
// mock bundle so callers can pre-seed stores or make assertions against them.
func newTestAllureHandlerAndMocks(t *testing.T, projectsDir string) (*AllureHandler, *testutil.MockStores) {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.MemBuilds, mocks.Locker, nil, nil, nil, logger)
	h := NewAllureHandler(cfg, r, nil,
		mocks.Projects, mocks.MemBuilds, mocks.KnownIssues, nil, mocks.Search, st, logger)
	syncTestBuildsFromFilesystem(t, projectsDir, mocks.MemBuilds)
	return h, mocks
}

// newTestAllureHandlerWithMocks creates an AllureHandler wired to caller-provided
// mock stores. Use this when tests pre-seed mock Fn fields before handler construction.
func newTestAllureHandlerWithMocks(t *testing.T, projectsDir string, mocks *testutil.MockStores) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	r := runner.NewAllure(cfg, st, mocks.Builds, mocks.Locker, mocks.TestResults, mocks.Branches, nil, logger)
	return NewAllureHandler(cfg, r, nil,
		mocks.Projects, mocks.Builds, mocks.KnownIssues, mocks.TestResults, mocks.Search, st, logger)
}

// newTestAllureHandlerWithJobManager builds an AllureHandler with a real JobManager
// backed by the provided generator, for async job handler tests.
// Uses mock stores since job manager tests do not require DB-level persistence.
func newTestAllureHandlerWithJobManager(t *testing.T, projectsDir string, gen runner.ReportGenerator) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir, KeepHistory: false}
	logger := zap.NewNop()
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.Builds, mocks.Locker, nil, nil, nil, logger)

	jm := runner.NewMemJobManager(gen, 2, logger)
	jm.Start(context.Background())
	t.Cleanup(func() { jm.Shutdown() })

	return NewAllureHandler(cfg, r, jm,
		mocks.Projects, mocks.Builds, mocks.KnownIssues, nil, nil, st, logger)
}

// newTestReportHandler creates a ReportHandler backed by stateful in-memory stores.
// After construction it scans projectsDir and populates the build store with any
// numbered report directories found, so GetReportHistory tests see the expected
// DB-backed entries without a real PostgreSQL connection.
func newTestReportHandler(t *testing.T, projectsDir string) (*ReportHandler, *testutil.MockStores) {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.MemBuilds, mocks.Locker, nil, nil, nil, logger)
	// testResultStore is nil so GetReportTimeline's "latest" resolution is skipped,
	// letting filesystem-based tests work correctly (same as original newTestAllureHandler).
	h := NewReportHandler(nil, r, mocks.MemBuilds, mocks.Branches, nil, mocks.KnownIssues, st, cfg, logger)
	syncTestBuildsFromFilesystem(t, projectsDir, mocks.MemBuilds)
	return h, mocks
}

// newTestReportHandlerWithMocks creates a ReportHandler wired to caller-provided
// mock stores. Use this when tests pre-seed mock Fn fields before handler construction.
func newTestReportHandlerWithMocks(t *testing.T, projectsDir string, mocks *testutil.MockStores) *ReportHandler {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	r := runner.NewAllure(cfg, st, mocks.Builds, mocks.Locker, mocks.TestResults, mocks.Branches, nil, logger)
	return NewReportHandler(nil, r, mocks.Builds, mocks.Branches, mocks.TestResults, mocks.KnownIssues, st, cfg, logger)
}

// newTestReportHandlerWithJobManager builds a ReportHandler with a real JobManager
// backed by the provided generator, for async job handler tests.
func newTestReportHandlerWithJobManager(t *testing.T, projectsDir string, gen runner.ReportGenerator) *ReportHandler {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir, KeepHistory: false}
	logger := zap.NewNop()
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.Builds, mocks.Locker, nil, nil, nil, logger)

	jm := runner.NewMemJobManager(gen, 2, logger)
	jm.Start(context.Background())
	t.Cleanup(func() { jm.Shutdown() })

	return NewReportHandler(jm, r, mocks.Builds, mocks.Branches, nil, nil, st, cfg, logger)
}

// newTestProjectHandler creates a ProjectHandler backed by stateful in-memory stores.
func newTestProjectHandler(t *testing.T, projectsDir string) (*ProjectHandler, *testutil.MockStores) {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.MemBuilds, mocks.Locker, nil, nil, nil, logger)
	h := NewProjectHandler(mocks.Projects, r, st, cfg, logger)
	return h, mocks
}

// writeSummaryJSON creates widgets/summary.json under the given report dir.
func writeSummaryJSON(t *testing.T, reportDir string, content string) {
	t.Helper()
	widgetsDir := filepath.Join(reportDir, "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetsDir, "summary.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
