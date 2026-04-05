package handlers

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ProjectEntry holds a single project in the paginated project listing.
type ProjectEntry struct {
	ProjectID  string   `json:"project_id"`
	ReportType string   `json:"report_type"`
	CreatedAt  string   `json:"created_at"`
	ParentID   *string  `json:"parent_id,omitempty"`
	Children   []string `json:"children,omitempty"`
}

// ReportHistoryEntry holds metadata for a single generated report.
type ReportHistoryEntry struct {
	ReportID       string           `json:"report_id"`
	IsLatest       bool             `json:"is_latest"`
	GeneratedAt    *string          `json:"generated_at"`
	DurationMs     *int64           `json:"duration_ms"`
	Statistic      *AllureStatistic `json:"statistic"`
	FlakyCount     *int             `json:"flaky_count,omitempty"`
	RetriedCount   *int             `json:"retried_count,omitempty"`
	NewFailedCount *int             `json:"new_failed_count,omitempty"`
	NewPassedCount *int             `json:"new_passed_count,omitempty"`
	CIProvider     *string          `json:"ci_provider,omitempty"`
	CIBuildURL     *string          `json:"ci_build_url,omitempty"`
	CIBranch       *string          `json:"ci_branch,omitempty"`
	CICommitSHA    *string          `json:"ci_commit_sha,omitempty"`
}

// AllureStatistic mirrors the statistic block in Allure's widgets/summary.json.
type AllureStatistic struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Skipped int `json:"skipped"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// allureSummaryFile is the shape of widgets/summary.json we care about.
type allureSummaryFile struct {
	Statistic *AllureStatistic `json:"statistic"`
	Time      *struct {
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	} `json:"time"`
}

// EnvironmentEntry represents one row in the Allure environment widget.
type EnvironmentEntry struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// CategoryMatchedStatistic holds the defect count breakdown for one category.
type CategoryMatchedStatistic struct {
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Known   int `json:"known"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// CategoryEntry represents one row in the Allure categories widget.
type CategoryEntry struct {
	Name             string                    `json:"name"`
	MatchedStatistic *CategoryMatchedStatistic `json:"matchedStatistic"`
}

// testResultTiming holds the start/stop epoch milliseconds from an Allure test result file.
type testResultTiming struct {
	Start int64 `json:"start"`
	Stop  int64 `json:"stop"`
}

// countingReader wraps an io.Reader and tracks cumulative bytes read.
// When the limit is exceeded it returns an explicit error instead of silently
// truncating (which io.LimitReader would do).
type countingReader struct {
	r        io.Reader
	n        int64
	limit    int64
	exceeded bool
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	if cr.n > cr.limit {
		cr.exceeded = true
		return n, fmt.Errorf("decompressed size exceeds %d bytes: %w", cr.limit, ErrArchiveDecompBomb)
	}
	return n, err
}

// buildEntryFromDB converts a store.Build to a ReportHistoryEntry without filesystem I/O.
func buildEntryFromDB(b *store.Build) ReportHistoryEntry {
	reportID := strconv.Itoa(b.BuildNumber)
	entry := ReportHistoryEntry{
		ReportID: reportID,
		IsLatest: b.IsLatest,
	}
	t := b.CreatedAt.UTC().Format(time.RFC3339)
	entry.GeneratedAt = &t
	entry.DurationMs = b.DurationMs
	entry.FlakyCount = b.FlakyCount
	entry.RetriedCount = b.RetriedCount
	entry.NewFailedCount = b.NewFailedCount
	entry.NewPassedCount = b.NewPassedCount
	entry.CIProvider = b.CIProvider
	entry.CIBuildURL = b.CIBuildURL
	entry.CIBranch = b.CIBranch
	entry.CICommitSHA = b.CICommitSHA

	if b.StatTotal != nil && *b.StatTotal > 0 {
		entry.Statistic = &AllureStatistic{
			Passed:  derefInt(b.StatPassed),
			Failed:  derefInt(b.StatFailed),
			Broken:  derefInt(b.StatBroken),
			Skipped: derefInt(b.StatSkipped),
			Unknown: derefInt(b.StatUnknown),
			Total:   *b.StatTotal,
		}
	}
	return entry
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// secureFilename strips path components so only the base filename remains
func secureFilename(name string) string {
	return filepath.Base(filepath.Clean(name))
}
