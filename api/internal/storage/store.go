package storage

import (
	"context"
	"errors"
	"io"
)

// Sentinel errors for storage operations.
var (
	ErrStatsNotFound   = errors.New("build stats not found")
	ErrReportNotFound  = errors.New("report not found")
	ErrProjectNotFound = errors.New("project not found")
)

// DirEntry represents a file or directory entry returned by ReadDir.
type DirEntry struct {
	Name    string
	Size    int64
	ModTime int64 // Unix nanoseconds
	IsDir   bool
}

// BuildStats holds statistics for a completed Allure report build.
type BuildStats struct {
	Passed     int
	Failed     int
	Broken     int
	Skipped    int
	Unknown    int
	Total      int
	DurationMs int64
}

// Store abstracts all storage operations for Allure project data.
// LocalStore implements this for local filesystem.
// S3Store implements this for S3/MinIO (added later).
type Store interface {
	// Project lifecycle
	CreateProject(ctx context.Context, projectID string) error
	DeleteProject(ctx context.Context, projectID string) error
	RenameProject(ctx context.Context, oldID, newID string) error
	ProjectExists(ctx context.Context, projectID string) (bool, error)
	ListProjects(ctx context.Context) ([]string, error)

	// Results management
	WriteResultFile(ctx context.Context, projectID, filename string, r io.Reader) error
	ListResultFiles(ctx context.Context, projectID string) ([]string, error)
	CleanResults(ctx context.Context, projectID string) error

	// Report generation lifecycle (local working dir)
	// PrepareLocal returns the local project directory to use for allure CLI operations.
	// For LocalStore this is the real project dir; for S3Store it's a temp dir with downloaded data.
	PrepareLocal(ctx context.Context, projectID string) (localProjectDir string, err error)
	// CleanupLocal cleans up any temp resources created by PrepareLocal.
	// For LocalStore this is a no-op; for S3Store it removes the temp dir.
	CleanupLocal(localProjectDir string) error

	// Report storage
	PublishReport(ctx context.Context, projectID string, buildOrder int, localProjectDir string) error
	DeleteReport(ctx context.Context, projectID, reportID string) error
	PruneReportDirs(ctx context.Context, projectID string, buildOrders []int) error

	// History
	KeepHistory(ctx context.Context, projectID string) error
	CleanHistory(ctx context.Context, projectID string) error

	// Report reading
	ReadBuildStats(ctx context.Context, projectID string, buildOrder int) (BuildStats, error)
	ReadFile(ctx context.Context, projectID, relPath string) ([]byte, error)
	ReadDir(ctx context.Context, projectID, relPath string) ([]DirEntry, error)
	OpenReportFile(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error)

	// Metadata
	ListReportBuilds(ctx context.Context, projectID string) ([]int, error)
	LatestReportExists(ctx context.Context, projectID string) (bool, error)
	// ResultsDirHash returns a hash of the results directory contents for change detection.
	// Returns ("", nil) for S3Store (watcher is disabled in S3 mode).
	ResultsDirHash(ctx context.Context, projectID string) (string, error)
}
