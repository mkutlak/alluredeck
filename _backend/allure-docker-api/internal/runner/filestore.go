package runner

// FileStore abstracts filesystem operations on Allure project data.
// The current FileSystem struct implements this interface.
// A future S3FileStore will implement the same interface for cloud storage.
type FileStore interface {
	CleanResults(projectID string) error
	CleanHistory(projectID string) error
	KeepHistory(projectID string) error
	DeleteReport(projectID, reportID string) error
}
