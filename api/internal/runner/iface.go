package runner

import "context"

// ReportGenerator is the interface satisfied by *Allure.
type ReportGenerator interface {
	GenerateReport(ctx context.Context, projectID int64, slug, storageKey, batchID, execName, execFrom, execType string,
		storeResults bool, ciBranch, ciCommitSHA string) (string, error)
}

// compile-time check
var _ ReportGenerator = (*Allure)(nil)
