package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// spyBuildStore implements store.BuildStorer with no-op responses.
type spyBuildStore struct{}

func (s *spyBuildStore) NextBuildNumber(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (s *spyBuildStore) InsertBuild(_ context.Context, _ int64, _ int) error { return nil }
func (s *spyBuildStore) UpdateBuildStats(_ context.Context, _ int64, _ int, _ store.BuildStats) error {
	return nil
}
func (s *spyBuildStore) UpdateBuildCIMetadata(_ context.Context, _ int64, _ int, _ store.CIMetadata) error {
	return nil
}
func (s *spyBuildStore) GetBuildByNumber(_ context.Context, _ int64, _ int) (store.Build, error) {
	return store.Build{}, nil
}
func (s *spyBuildStore) GetPreviousBuild(_ context.Context, _ int64, _ int) (store.Build, error) {
	return store.Build{}, nil
}
func (s *spyBuildStore) GetLatestBuild(_ context.Context, _ int64) (store.Build, error) {
	return store.Build{}, nil
}
func (s *spyBuildStore) ListBuilds(_ context.Context, _ int64) ([]store.Build, error) {
	return nil, nil
}
func (s *spyBuildStore) ListBuildsPaginated(_ context.Context, _ int64, _, _ int) ([]store.Build, int, error) {
	return nil, 0, nil
}
func (s *spyBuildStore) PruneBuilds(_ context.Context, _ int64, _ int) ([]int, error) {
	return nil, nil
}
func (s *spyBuildStore) SetLatest(_ context.Context, _ int64, _ int) error { return nil }
func (s *spyBuildStore) DeleteAllBuilds(_ context.Context, _ int64) error  { return nil }
func (s *spyBuildStore) GetDashboardData(_ context.Context, _ int) ([]store.DashboardProject, error) {
	return nil, nil
}
func (s *spyBuildStore) DeleteBuild(_ context.Context, _ int64, _ int) error { return nil }
func (s *spyBuildStore) UpdateBuildBranchID(_ context.Context, _ int64, _ int, _ int64) error {
	return nil
}
func (s *spyBuildStore) SetLatestBranch(_ context.Context, _ int64, _ int, _ *int64) error {
	return nil
}
func (s *spyBuildStore) PruneBuildsBranch(_ context.Context, _ int64, _ int, _ *int64) ([]int, error) {
	return nil, nil
}
func (s *spyBuildStore) PruneBuildsByAge(_ context.Context, _ int64, _ time.Time) ([]int, error) {
	return nil, nil
}
func (s *spyBuildStore) ListBuildsPaginatedBranch(_ context.Context, _ int64, _, _ int, _ *int64) ([]store.Build, int, error) {
	return nil, 0, nil
}
func (s *spyBuildStore) ListBuildsInRange(_ context.Context, _ int64, _ *int64, _, _ time.Time, _ int) ([]store.Build, int, error) {
	return nil, 0, nil
}

func (s *spyBuildStore) SetHasPlaywrightReport(_ context.Context, _ int64, _ int, _ bool) error {
	return nil
}

// spyTestResultStore implements store.TestResultStorer and records InsertBatchFull calls.
type spyTestResultStore struct {
	insertBatchFullCount int
	lastBuildID          int64
	lastProjectID        int64
	lastResults          []*parser.Result
	insertBatchFullErr   error
}

func (s *spyTestResultStore) InsertBatch(_ context.Context, _ []store.TestResult) error {
	return nil
}
func (s *spyTestResultStore) InsertBatchFull(_ context.Context, buildID int64, projectID int64, results []*parser.Result) error {
	s.insertBatchFullCount++
	s.lastBuildID = buildID
	s.lastProjectID = projectID
	s.lastResults = results
	return s.insertBatchFullErr
}
func (s *spyTestResultStore) GetBuildID(_ context.Context, _ int64, _ int) (int64, error) {
	return 42, nil
}
func (s *spyTestResultStore) ListSlowest(_ context.Context, _ int64, _, _ int, _ *int64) ([]store.LowPerformingTest, error) {
	return nil, nil
}
func (s *spyTestResultStore) ListLeastReliable(_ context.Context, _ int64, _, _ int, _ *int64) ([]store.LowPerformingTest, error) {
	return nil, nil
}
func (s *spyTestResultStore) ListTimeline(_ context.Context, _ int64, _ int64, _ int) ([]store.TimelineRow, error) {
	return nil, nil
}
func (s *spyTestResultStore) ListFailedByBuild(_ context.Context, _ int64, _ int64, _ int) ([]store.TestResult, error) {
	return nil, nil
}
func (s *spyTestResultStore) GetTestHistory(_ context.Context, _ int64, _ string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
	return nil, nil
}
func (s *spyTestResultStore) DeleteByBuild(_ context.Context, _ int64) error   { return nil }
func (s *spyTestResultStore) DeleteByProject(_ context.Context, _ int64) error { return nil }
func (s *spyTestResultStore) CompareBuildsByHistoryID(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
	return nil, nil
}
func (s *spyTestResultStore) ListTimelineMulti(_ context.Context, _ int64, _ []int64, _ int) ([]store.MultiTimelineRow, error) {
	return nil, nil
}
func (s *spyTestResultStore) ListFailedForFingerprinting(_ context.Context, _ int64, _ int64) ([]store.FailedTestResult, error) {
	return nil, nil
}

// mockStore returns a storage.MockStore wired for storeAndPruneBuild success:
// ReadBuildStats returns non-empty stats, ReadDir returns empty (no stability entries).
func mockStoreForParsing() *storage.MockStore {
	return &storage.MockStore{
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{Total: 1, Passed: 1}, nil
		},
		// ReadDir returns empty → parseStabilityEntries succeeds with zero entries.
	}
}

// mustWriteResultFile creates a minimal valid Allure result JSON file.
func mustWriteResultFile(t *testing.T, dir, filename string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, _ := json.Marshal(map[string]any{
		"name":      "Test Case One",
		"status":    "passed",
		"historyId": "hist-abc",
		"start":     int64(1000),
		"stop":      int64(2000),
	})
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// TestStoreAndPruneBuild_CallsInsertBatchFull verifies that storeAndPruneBuild
// calls InsertBatchFull with parsed result files from the results/ directory.
func TestStoreAndPruneBuild_CallsInsertBatchFull(t *testing.T) {
	tmpDir := t.TempDir()
	projectID := int64(1)
	buildNumber := 1

	resultsDir := filepath.Join(tmpDir, "results")
	mustWriteResultFile(t, resultsDir, "abc-result.json")

	spy := &spyTestResultStore{}
	a := &Allure{
		cfg:             &config.Config{ProjectsPath: tmpDir},
		store:           mockStoreForParsing(),
		buildStore:      &spyBuildStore{},
		testResultStore: spy,
		logger:          zap.NewNop(),
	}

	if err := a.storeAndPruneBuild(context.Background(), projectID, "test-project", "test-project", "", tmpDir, buildNumber, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if spy.insertBatchFullCount == 0 {
		t.Error("InsertBatchFull was not called")
	}
	if spy.lastBuildID != 42 {
		t.Errorf("InsertBatchFull buildID = %d, want 42", spy.lastBuildID)
	}
	if spy.lastProjectID != projectID {
		t.Errorf("InsertBatchFull projectID = %q, want %q", spy.lastProjectID, projectID)
	}
	if len(spy.lastResults) == 0 {
		t.Error("InsertBatchFull called with empty results slice")
	}
}

// TestStoreAndPruneBuild_InsertBatchFullFailure_NonFatal verifies that an
// InsertBatchFull error does not cause storeAndPruneBuild to return an error.
func TestStoreAndPruneBuild_InsertBatchFullFailure_NonFatal(t *testing.T) {
	tmpDir := t.TempDir()

	resultsDir := filepath.Join(tmpDir, "results")
	mustWriteResultFile(t, resultsDir, "xyz-result.json")

	spy := &spyTestResultStore{insertBatchFullErr: errors.New("db unavailable")}
	a := &Allure{
		cfg:             &config.Config{ProjectsPath: tmpDir},
		store:           mockStoreForParsing(),
		buildStore:      &spyBuildStore{},
		testResultStore: spy,
		logger:          zap.NewNop(),
	}

	err := a.storeAndPruneBuild(context.Background(), int64(2), "proj", "proj", "", tmpDir, 1, store.CIMetadata{}, nil)
	if err != nil {
		t.Fatalf("storeAndPruneBuild must not fail when InsertBatchFull errors, got: %v", err)
	}
}

// TestStoreAndPruneBuild_NoResultFiles_SkipsInsertBatchFull verifies that
// InsertBatchFull is NOT called when there are no raw result files.
func TestStoreAndPruneBuild_NoResultFiles_SkipsInsertBatchFull(t *testing.T) {
	tmpDir := t.TempDir()
	// results/ dir exists but is empty (no *-result.json files).
	if err := os.MkdirAll(filepath.Join(tmpDir, "results"), 0o755); err != nil {
		t.Fatal(err)
	}

	spy := &spyTestResultStore{}
	a := &Allure{
		cfg:             &config.Config{ProjectsPath: tmpDir},
		store:           mockStoreForParsing(),
		buildStore:      &spyBuildStore{},
		testResultStore: spy,
		logger:          zap.NewNop(),
	}

	if err := a.storeAndPruneBuild(context.Background(), int64(3), "proj", "proj", "", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if spy.insertBatchFullCount != 0 {
		t.Errorf("InsertBatchFull should not be called when results/ is empty, called %d times", spy.insertBatchFullCount)
	}
}
