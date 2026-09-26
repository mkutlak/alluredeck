package testutil

import (
	"context"
	"io"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// MockStorage is a function-field test double for storage.Store. Set function
// fields to control behavior; unset fields return zero values with no error.
type MockStorage struct {
	HealthCheckFn                 func(ctx context.Context) error
	CreateProjectFn               func(ctx context.Context, projectID string) error
	DeleteProjectFn               func(ctx context.Context, projectID string) error
	RenameProjectFn               func(ctx context.Context, oldID, newID string) error
	ProjectExistsFn               func(ctx context.Context, projectID string) (bool, error)
	ListProjectsFn                func(ctx context.Context) ([]string, error)
	WriteResultFileFn             func(ctx context.Context, projectID, batchID, filename string, r io.Reader) error
	ListResultFilesFn             func(ctx context.Context, projectID, batchID string) ([]string, error)
	CleanBatchFn                  func(ctx context.Context, projectID, batchID string) error
	CleanResultsFn                func(ctx context.Context, projectID string) error
	ListResultBatchesFn           func(ctx context.Context, projectID string) ([]string, error)
	PrepareLocalFn                func(ctx context.Context, projectID string, progress storage.ProgressFn) (string, error)
	CleanupLocalFn                func(localProjectDir string) error
	PublishReportFn               func(ctx context.Context, projectID string, buildNumber int, localProjectDir string, progress storage.ProgressFn) error
	DeleteReportFn                func(ctx context.Context, projectID, reportID string) error
	PruneReportDirsFn             func(ctx context.Context, projectID string, buildNumbers []int) error
	KeepHistoryFn                 func(ctx context.Context, projectID, batchID string) error
	CleanHistoryFn                func(ctx context.Context, projectID string) error
	ReadBuildStatsFn              func(ctx context.Context, projectID string, buildNumber int) (storage.BuildStats, error)
	ReadFileFn                    func(ctx context.Context, projectID, relPath string) ([]byte, error)
	ReadDirFn                     func(ctx context.Context, projectID, relPath string) ([]storage.DirEntry, error)
	OpenReportFileFn              func(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error)
	ListReportBuildsFn            func(ctx context.Context, projectID string) ([]int, error)
	LatestReportExistsFn          func(ctx context.Context, projectID string) (bool, error)
	ResultsDirHashFn              func(ctx context.Context, projectID string) (string, error)
	WriteRawBlobFn                func(ctx context.Context, key string, r io.Reader) error
	OpenBlobFn                    func(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteBlobFn                  func(ctx context.Context, key string) error
	ListStagingBlobsFn            func(ctx context.Context, olderThan time.Duration) ([]string, error)
	WritePlaywrightFileFn         func(ctx context.Context, projectID, subPath string, r io.Reader) error
	PlaywrightReportExistsFn      func(ctx context.Context, projectID string, buildNumber int) (bool, error)
	CopyPlaywrightLatestToBuildFn func(ctx context.Context, projectID string, buildNumber int) error
	CleanPlaywrightLatestFn       func(ctx context.Context, projectID string) error
	ListPlaywrightDataFilesFn     func(ctx context.Context, projectID string, buildNumber int) ([]string, error)
	ReadPlaywrightFileFn          func(ctx context.Context, projectID, subPath string) (io.ReadCloser, string, error)
}

// Ensure MockStorage implements storage.Store at compile time.
var _ storage.Store = (*MockStorage)(nil)

// HealthCheck implements storage.Store.
func (m *MockStorage) HealthCheck(ctx context.Context) error {
	if m.HealthCheckFn != nil {
		return m.HealthCheckFn(ctx)
	}
	return nil
}

// CreateProject implements storage.Store.
func (m *MockStorage) CreateProject(ctx context.Context, projectID string) error {
	if m.CreateProjectFn != nil {
		return m.CreateProjectFn(ctx, projectID)
	}
	return nil
}

// DeleteProject implements storage.Store.
func (m *MockStorage) DeleteProject(ctx context.Context, projectID string) error {
	if m.DeleteProjectFn != nil {
		return m.DeleteProjectFn(ctx, projectID)
	}
	return nil
}

// RenameProject implements storage.Store.
func (m *MockStorage) RenameProject(ctx context.Context, oldID, newID string) error {
	if m.RenameProjectFn != nil {
		return m.RenameProjectFn(ctx, oldID, newID)
	}
	return nil
}

// ProjectExists implements storage.Store.
func (m *MockStorage) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	if m.ProjectExistsFn != nil {
		return m.ProjectExistsFn(ctx, projectID)
	}
	return false, nil
}

// ListProjects implements storage.Store.
func (m *MockStorage) ListProjects(ctx context.Context) ([]string, error) {
	if m.ListProjectsFn != nil {
		return m.ListProjectsFn(ctx)
	}
	return nil, nil
}

// WriteResultFile implements storage.Store.
func (m *MockStorage) WriteResultFile(ctx context.Context, projectID, batchID, filename string, r io.Reader) error {
	if m.WriteResultFileFn != nil {
		return m.WriteResultFileFn(ctx, projectID, batchID, filename, r)
	}
	return nil
}

// ListResultFiles implements storage.Store.
func (m *MockStorage) ListResultFiles(ctx context.Context, projectID, batchID string) ([]string, error) {
	if m.ListResultFilesFn != nil {
		return m.ListResultFilesFn(ctx, projectID, batchID)
	}
	return nil, nil
}

// CleanBatch implements storage.Store.
func (m *MockStorage) CleanBatch(ctx context.Context, projectID, batchID string) error {
	if m.CleanBatchFn != nil {
		return m.CleanBatchFn(ctx, projectID, batchID)
	}
	return nil
}

// CleanResults implements storage.Store.
func (m *MockStorage) CleanResults(ctx context.Context, projectID string) error {
	if m.CleanResultsFn != nil {
		return m.CleanResultsFn(ctx, projectID)
	}
	return nil
}

// ListResultBatches implements storage.Store.
func (m *MockStorage) ListResultBatches(ctx context.Context, projectID string) ([]string, error) {
	if m.ListResultBatchesFn != nil {
		return m.ListResultBatchesFn(ctx, projectID)
	}
	return nil, nil
}

// PrepareLocal implements storage.Store.
func (m *MockStorage) PrepareLocal(ctx context.Context, projectID string, progress storage.ProgressFn) (string, error) {
	if m.PrepareLocalFn != nil {
		return m.PrepareLocalFn(ctx, projectID, progress)
	}
	return "", nil
}

// CleanupLocal implements storage.Store.
func (m *MockStorage) CleanupLocal(localProjectDir string) error {
	if m.CleanupLocalFn != nil {
		return m.CleanupLocalFn(localProjectDir)
	}
	return nil
}

// PublishReport implements storage.Store.
func (m *MockStorage) PublishReport(ctx context.Context, projectID string, buildNumber int, localProjectDir string, progress storage.ProgressFn) error {
	if m.PublishReportFn != nil {
		return m.PublishReportFn(ctx, projectID, buildNumber, localProjectDir, progress)
	}
	return nil
}

// DeleteReport implements storage.Store.
func (m *MockStorage) DeleteReport(ctx context.Context, projectID, reportID string) error {
	if m.DeleteReportFn != nil {
		return m.DeleteReportFn(ctx, projectID, reportID)
	}
	return nil
}

// PruneReportDirs implements storage.Store.
func (m *MockStorage) PruneReportDirs(ctx context.Context, projectID string, buildNumbers []int) error {
	if m.PruneReportDirsFn != nil {
		return m.PruneReportDirsFn(ctx, projectID, buildNumbers)
	}
	return nil
}

// KeepHistory implements storage.Store.
func (m *MockStorage) KeepHistory(ctx context.Context, projectID, batchID string) error {
	if m.KeepHistoryFn != nil {
		return m.KeepHistoryFn(ctx, projectID, batchID)
	}
	return nil
}

// CleanHistory implements storage.Store.
func (m *MockStorage) CleanHistory(ctx context.Context, projectID string) error {
	if m.CleanHistoryFn != nil {
		return m.CleanHistoryFn(ctx, projectID)
	}
	return nil
}

// ReadBuildStats implements storage.Store.
func (m *MockStorage) ReadBuildStats(ctx context.Context, projectID string, buildNumber int) (storage.BuildStats, error) {
	if m.ReadBuildStatsFn != nil {
		return m.ReadBuildStatsFn(ctx, projectID, buildNumber)
	}
	return storage.BuildStats{}, nil
}

// ReadFile implements storage.Store.
func (m *MockStorage) ReadFile(ctx context.Context, projectID, relPath string) ([]byte, error) {
	if m.ReadFileFn != nil {
		return m.ReadFileFn(ctx, projectID, relPath)
	}
	return nil, nil
}

// ReadDir implements storage.Store.
func (m *MockStorage) ReadDir(ctx context.Context, projectID, relPath string) ([]storage.DirEntry, error) {
	if m.ReadDirFn != nil {
		return m.ReadDirFn(ctx, projectID, relPath)
	}
	return nil, nil
}

// OpenReportFile implements storage.Store.
func (m *MockStorage) OpenReportFile(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
	if m.OpenReportFileFn != nil {
		return m.OpenReportFileFn(ctx, projectID, reportID, filePath)
	}
	return nil, "", nil
}

// ListReportBuilds implements storage.Store.
func (m *MockStorage) ListReportBuilds(ctx context.Context, projectID string) ([]int, error) {
	if m.ListReportBuildsFn != nil {
		return m.ListReportBuildsFn(ctx, projectID)
	}
	return nil, nil
}

// LatestReportExists implements storage.Store.
func (m *MockStorage) LatestReportExists(ctx context.Context, projectID string) (bool, error) {
	if m.LatestReportExistsFn != nil {
		return m.LatestReportExistsFn(ctx, projectID)
	}
	return false, nil
}

// ResultsDirHash implements storage.Store.
func (m *MockStorage) ResultsDirHash(ctx context.Context, projectID string) (string, error) {
	if m.ResultsDirHashFn != nil {
		return m.ResultsDirHashFn(ctx, projectID)
	}
	return "", nil
}

// WritePlaywrightFile implements storage.Store.
func (m *MockStorage) WritePlaywrightFile(ctx context.Context, projectID, subPath string, r io.Reader) error {
	if m.WritePlaywrightFileFn != nil {
		return m.WritePlaywrightFileFn(ctx, projectID, subPath, r)
	}
	return nil
}

// PlaywrightReportExists implements storage.Store.
func (m *MockStorage) PlaywrightReportExists(ctx context.Context, projectID string, buildNumber int) (bool, error) {
	if m.PlaywrightReportExistsFn != nil {
		return m.PlaywrightReportExistsFn(ctx, projectID, buildNumber)
	}
	return false, nil
}

// CopyPlaywrightLatestToBuild implements storage.Store.
func (m *MockStorage) CopyPlaywrightLatestToBuild(ctx context.Context, projectID string, buildNumber int) error {
	if m.CopyPlaywrightLatestToBuildFn != nil {
		return m.CopyPlaywrightLatestToBuildFn(ctx, projectID, buildNumber)
	}
	return nil
}

// CleanPlaywrightLatest implements storage.Store.
func (m *MockStorage) CleanPlaywrightLatest(ctx context.Context, projectID string) error {
	if m.CleanPlaywrightLatestFn != nil {
		return m.CleanPlaywrightLatestFn(ctx, projectID)
	}
	return nil
}

// ListPlaywrightDataFiles implements storage.Store.
func (m *MockStorage) ListPlaywrightDataFiles(ctx context.Context, projectID string, buildNumber int) ([]string, error) {
	if m.ListPlaywrightDataFilesFn != nil {
		return m.ListPlaywrightDataFilesFn(ctx, projectID, buildNumber)
	}
	return nil, nil
}

// ReadPlaywrightFile implements storage.Store.
func (m *MockStorage) ReadPlaywrightFile(ctx context.Context, projectID, subPath string) (io.ReadCloser, string, error) {
	if m.ReadPlaywrightFileFn != nil {
		return m.ReadPlaywrightFileFn(ctx, projectID, subPath)
	}
	return nil, "", nil
}

// WriteRawBlob implements storage.Store.
func (m *MockStorage) WriteRawBlob(ctx context.Context, key string, r io.Reader) error {
	if m.WriteRawBlobFn != nil {
		return m.WriteRawBlobFn(ctx, key, r)
	}
	return nil
}

// OpenBlob implements storage.Store.
func (m *MockStorage) OpenBlob(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.OpenBlobFn != nil {
		return m.OpenBlobFn(ctx, key)
	}
	return nil, nil
}

// DeleteBlob implements storage.Store.
func (m *MockStorage) DeleteBlob(ctx context.Context, key string) error {
	if m.DeleteBlobFn != nil {
		return m.DeleteBlobFn(ctx, key)
	}
	return nil
}

// ListStagingBlobs implements storage.Store.
func (m *MockStorage) ListStagingBlobs(ctx context.Context, olderThan time.Duration) ([]string, error) {
	if m.ListStagingBlobsFn != nil {
		return m.ListStagingBlobsFn(ctx, olderThan)
	}
	return nil, nil
}
