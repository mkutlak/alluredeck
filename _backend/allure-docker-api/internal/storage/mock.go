package storage

import (
	"context"
	"io"
)

// MockStore is a test double for Store. Set function fields to control behavior.
// Unset fields return zero values with no error.
type MockStore struct {
	CreateProjectFn      func(ctx context.Context, projectID string) error
	DeleteProjectFn      func(ctx context.Context, projectID string) error
	ProjectExistsFn      func(ctx context.Context, projectID string) (bool, error)
	ListProjectsFn       func(ctx context.Context) ([]string, error)
	WriteResultFileFn    func(ctx context.Context, projectID, filename string, r io.Reader) error
	ListResultFilesFn    func(ctx context.Context, projectID string) ([]string, error)
	CleanResultsFn       func(ctx context.Context, projectID string) error
	PrepareLocalFn       func(ctx context.Context, projectID string) (string, error)
	CleanupLocalFn       func(localProjectDir string) error
	PublishReportFn      func(ctx context.Context, projectID string, buildOrder int, localProjectDir string) error
	DeleteReportFn       func(ctx context.Context, projectID, reportID string) error
	PruneReportDirsFn    func(ctx context.Context, projectID string, buildOrders []int) error
	KeepHistoryFn        func(ctx context.Context, projectID string) error
	CleanHistoryFn       func(ctx context.Context, projectID string) error
	ReadBuildStatsFn     func(ctx context.Context, projectID string, buildOrder int) (BuildStats, error)
	ReadFileFn           func(ctx context.Context, projectID, relPath string) ([]byte, error)
	ReadDirFn            func(ctx context.Context, projectID, relPath string) ([]DirEntry, error)
	OpenReportFileFn     func(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error)
	ListReportBuildsFn   func(ctx context.Context, projectID string) ([]int, error)
	LatestReportExistsFn func(ctx context.Context, projectID string) (bool, error)
	ResultsDirHashFn     func(ctx context.Context, projectID string) (string, error)
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
func (m *MockStore) WriteResultFile(ctx context.Context, projectID, filename string, r io.Reader) error {
	if m.WriteResultFileFn != nil {
		return m.WriteResultFileFn(ctx, projectID, filename, r)
	}
	return nil
}

// ListResultFiles implements Store.
func (m *MockStore) ListResultFiles(ctx context.Context, projectID string) ([]string, error) {
	if m.ListResultFilesFn != nil {
		return m.ListResultFilesFn(ctx, projectID)
	}
	return nil, nil
}

// CleanResults implements Store.
func (m *MockStore) CleanResults(ctx context.Context, projectID string) error {
	if m.CleanResultsFn != nil {
		return m.CleanResultsFn(ctx, projectID)
	}
	return nil
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
func (m *MockStore) PublishReport(ctx context.Context, projectID string, buildOrder int, localProjectDir string) error {
	if m.PublishReportFn != nil {
		return m.PublishReportFn(ctx, projectID, buildOrder, localProjectDir)
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
func (m *MockStore) PruneReportDirs(ctx context.Context, projectID string, buildOrders []int) error {
	if m.PruneReportDirsFn != nil {
		return m.PruneReportDirsFn(ctx, projectID, buildOrders)
	}
	return nil
}

// KeepHistory implements Store.
func (m *MockStore) KeepHistory(ctx context.Context, projectID string) error {
	if m.KeepHistoryFn != nil {
		return m.KeepHistoryFn(ctx, projectID)
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
func (m *MockStore) ReadBuildStats(ctx context.Context, projectID string, buildOrder int) (BuildStats, error) {
	if m.ReadBuildStatsFn != nil {
		return m.ReadBuildStatsFn(ctx, projectID, buildOrder)
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
