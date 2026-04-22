package storage

import (
	"context"
	"io"
)

// MockStore is a test double for Store. Set function fields to control behavior.
// Unset fields return zero values with no error.
type MockStore struct {
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
	PrepareLocalFn                func(ctx context.Context, projectID string) (string, error)
	CleanupLocalFn                func(localProjectDir string) error
	PublishReportFn               func(ctx context.Context, projectID string, buildNumber int, localProjectDir string) error
	DeleteReportFn                func(ctx context.Context, projectID, reportID string) error
	PruneReportDirsFn             func(ctx context.Context, projectID string, buildNumbers []int) error
	KeepHistoryFn                 func(ctx context.Context, projectID, batchID string) error
	CleanHistoryFn                func(ctx context.Context, projectID string) error
	ReadBuildStatsFn              func(ctx context.Context, projectID string, buildNumber int) (BuildStats, error)
	ReadFileFn                    func(ctx context.Context, projectID, relPath string) ([]byte, error)
	ReadDirFn                     func(ctx context.Context, projectID, relPath string) ([]DirEntry, error)
	OpenReportFileFn              func(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error)
	ListReportBuildsFn            func(ctx context.Context, projectID string) ([]int, error)
	LatestReportExistsFn          func(ctx context.Context, projectID string) (bool, error)
	ResultsDirHashFn              func(ctx context.Context, projectID string) (string, error)
	WritePlaywrightFileFn         func(ctx context.Context, projectID, subPath string, r io.Reader) error
	PlaywrightReportExistsFn      func(ctx context.Context, projectID string, buildNumber int) (bool, error)
	CopyPlaywrightLatestToBuildFn func(ctx context.Context, projectID string, buildNumber int) error
	CleanPlaywrightLatestFn       func(ctx context.Context, projectID string) error
	ListPlaywrightDataFilesFn     func(ctx context.Context, projectID string, buildNumber int) ([]string, error)
	ReadPlaywrightFileFn          func(ctx context.Context, projectID, subPath string) (io.ReadCloser, string, error)
}

// Ensure MockStore implements Store at compile time.
var _ Store = (*MockStore)(nil)

// CreateProject implements Store.
func (m *MockStore) CreateProject(ctx context.Context, projectID string) error {
	if m.CreateProjectFn != nil {
		return m.CreateProjectFn(ctx, projectID)
	}
	return nil
}

// DeleteProject implements Store.
func (m *MockStore) DeleteProject(ctx context.Context, projectID string) error {
	if m.DeleteProjectFn != nil {
		return m.DeleteProjectFn(ctx, projectID)
	}
	return nil
}

// RenameProject implements Store.
func (m *MockStore) RenameProject(ctx context.Context, oldID, newID string) error {
	if m.RenameProjectFn != nil {
		return m.RenameProjectFn(ctx, oldID, newID)
	}
	return nil
}

// ProjectExists implements Store.
func (m *MockStore) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	if m.ProjectExistsFn != nil {
		return m.ProjectExistsFn(ctx, projectID)
	}
	return false, nil
}

// ListProjects implements Store.
func (m *MockStore) ListProjects(ctx context.Context) ([]string, error) {
	if m.ListProjectsFn != nil {
		return m.ListProjectsFn(ctx)
	}
	return nil, nil
}

// WriteResultFile implements Store.
func (m *MockStore) WriteResultFile(ctx context.Context, projectID, batchID, filename string, r io.Reader) error {
	if m.WriteResultFileFn != nil {
		return m.WriteResultFileFn(ctx, projectID, batchID, filename, r)
	}
	return nil
}

// ListResultFiles implements Store.
func (m *MockStore) ListResultFiles(ctx context.Context, projectID, batchID string) ([]string, error) {
	if m.ListResultFilesFn != nil {
		return m.ListResultFilesFn(ctx, projectID, batchID)
	}
	return nil, nil
}

// CleanBatch implements Store.
func (m *MockStore) CleanBatch(ctx context.Context, projectID, batchID string) error {
	if m.CleanBatchFn != nil {
		return m.CleanBatchFn(ctx, projectID, batchID)
	}
	return nil
}

// CleanResults implements Store.
func (m *MockStore) CleanResults(ctx context.Context, projectID string) error {
	if m.CleanResultsFn != nil {
		return m.CleanResultsFn(ctx, projectID)
	}
	return nil
}

// ListResultBatches implements Store.
func (m *MockStore) ListResultBatches(ctx context.Context, projectID string) ([]string, error) {
	if m.ListResultBatchesFn != nil {
		return m.ListResultBatchesFn(ctx, projectID)
	}
	return nil, nil
}

// PrepareLocal implements Store.
func (m *MockStore) PrepareLocal(ctx context.Context, projectID string) (string, error) {
	if m.PrepareLocalFn != nil {
		return m.PrepareLocalFn(ctx, projectID)
	}
	return "", nil
}

// CleanupLocal implements Store.
func (m *MockStore) CleanupLocal(localProjectDir string) error {
	if m.CleanupLocalFn != nil {
		return m.CleanupLocalFn(localProjectDir)
	}
	return nil
}

// PublishReport implements Store.
func (m *MockStore) PublishReport(ctx context.Context, projectID string, buildNumber int, localProjectDir string) error {
	if m.PublishReportFn != nil {
		return m.PublishReportFn(ctx, projectID, buildNumber, localProjectDir)
	}
	return nil
}

// DeleteReport implements Store.
func (m *MockStore) DeleteReport(ctx context.Context, projectID, reportID string) error {
	if m.DeleteReportFn != nil {
		return m.DeleteReportFn(ctx, projectID, reportID)
	}
	return nil
}

// PruneReportDirs implements Store.
func (m *MockStore) PruneReportDirs(ctx context.Context, projectID string, buildNumbers []int) error {
	if m.PruneReportDirsFn != nil {
		return m.PruneReportDirsFn(ctx, projectID, buildNumbers)
	}
	return nil
}

// KeepHistory implements Store.
func (m *MockStore) KeepHistory(ctx context.Context, projectID, batchID string) error {
	if m.KeepHistoryFn != nil {
		return m.KeepHistoryFn(ctx, projectID, batchID)
	}
	return nil
}

// CleanHistory implements Store.
func (m *MockStore) CleanHistory(ctx context.Context, projectID string) error {
	if m.CleanHistoryFn != nil {
		return m.CleanHistoryFn(ctx, projectID)
	}
	return nil
}

// ReadBuildStats implements Store.
func (m *MockStore) ReadBuildStats(ctx context.Context, projectID string, buildNumber int) (BuildStats, error) {
	if m.ReadBuildStatsFn != nil {
		return m.ReadBuildStatsFn(ctx, projectID, buildNumber)
	}
	return BuildStats{}, nil
}

// ReadFile implements Store.
func (m *MockStore) ReadFile(ctx context.Context, projectID, relPath string) ([]byte, error) {
	if m.ReadFileFn != nil {
		return m.ReadFileFn(ctx, projectID, relPath)
	}
	return nil, nil
}

// ReadDir implements Store.
func (m *MockStore) ReadDir(ctx context.Context, projectID, relPath string) ([]DirEntry, error) {
	if m.ReadDirFn != nil {
		return m.ReadDirFn(ctx, projectID, relPath)
	}
	return nil, nil
}

// OpenReportFile implements Store.
func (m *MockStore) OpenReportFile(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
	if m.OpenReportFileFn != nil {
		return m.OpenReportFileFn(ctx, projectID, reportID, filePath)
	}
	return nil, "", nil
}

// ListReportBuilds implements Store.
func (m *MockStore) ListReportBuilds(ctx context.Context, projectID string) ([]int, error) {
	if m.ListReportBuildsFn != nil {
		return m.ListReportBuildsFn(ctx, projectID)
	}
	return nil, nil
}

// LatestReportExists implements Store.
func (m *MockStore) LatestReportExists(ctx context.Context, projectID string) (bool, error) {
	if m.LatestReportExistsFn != nil {
		return m.LatestReportExistsFn(ctx, projectID)
	}
	return false, nil
}

// ResultsDirHash implements Store.
func (m *MockStore) ResultsDirHash(ctx context.Context, projectID string) (string, error) {
	if m.ResultsDirHashFn != nil {
		return m.ResultsDirHashFn(ctx, projectID)
	}
	return "", nil
}

// WritePlaywrightFile implements Store.
func (m *MockStore) WritePlaywrightFile(ctx context.Context, projectID, subPath string, r io.Reader) error {
	if m.WritePlaywrightFileFn != nil {
		return m.WritePlaywrightFileFn(ctx, projectID, subPath, r)
	}
	return nil
}

// PlaywrightReportExists implements Store.
func (m *MockStore) PlaywrightReportExists(ctx context.Context, projectID string, buildNumber int) (bool, error) {
	if m.PlaywrightReportExistsFn != nil {
		return m.PlaywrightReportExistsFn(ctx, projectID, buildNumber)
	}
	return false, nil
}

// CopyPlaywrightLatestToBuild implements Store.
func (m *MockStore) CopyPlaywrightLatestToBuild(ctx context.Context, projectID string, buildNumber int) error {
	if m.CopyPlaywrightLatestToBuildFn != nil {
		return m.CopyPlaywrightLatestToBuildFn(ctx, projectID, buildNumber)
	}
	return nil
}

// CleanPlaywrightLatest implements Store.
func (m *MockStore) CleanPlaywrightLatest(ctx context.Context, projectID string) error {
	if m.CleanPlaywrightLatestFn != nil {
		return m.CleanPlaywrightLatestFn(ctx, projectID)
	}
	return nil
}

// ListPlaywrightDataFiles implements Store.
func (m *MockStore) ListPlaywrightDataFiles(ctx context.Context, projectID string, buildNumber int) ([]string, error) {
	if m.ListPlaywrightDataFilesFn != nil {
		return m.ListPlaywrightDataFilesFn(ctx, projectID, buildNumber)
	}
	return nil, nil
}

// ReadPlaywrightFile implements Store.
func (m *MockStore) ReadPlaywrightFile(ctx context.Context, projectID, subPath string) (io.ReadCloser, string, error) {
	if m.ReadPlaywrightFileFn != nil {
		return m.ReadPlaywrightFileFn(ctx, projectID, subPath)
	}
	return nil, "", nil
}
