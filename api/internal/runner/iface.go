package runner

import "context"

// ReportGenerator is the interface satisfied by *Allure.
type ReportGenerator interface {
	GenerateReport(ctx context.Context, projectID int64, slug, storageKey, batchID, execName, execFrom, execType string,
		storeResults bool, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) (string, error)
}

// LocalReportGenerator is the optional sibling interface used by the staged
// async upload worker. It runs report generation against an already-prepared
// pod-local directory (results extracted from the staging tar.gz blob) and
// avoids the per-file MinIO round-trip the legacy staged worker incurred.
//
// Implementations live alongside ReportGenerator on *Allure. The staged worker
// type-asserts on this interface so non-Allure ReportGenerators (test doubles)
// continue to work via the legacy GenerateReport entry point.
type LocalReportGenerator interface {
	GenerateReportFromLocalDir(ctx context.Context, projectID int64,
		slug, storageKey, batchID, execName, execFrom, execType string,
		storeResults bool,
		ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string,
		localProjectDir string,
	) (string, error)
}

// compile-time checks
var (
	_ ReportGenerator      = (*Allure)(nil)
	_ LocalReportGenerator = (*Allure)(nil)
)
