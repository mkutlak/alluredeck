package store

import "time"

// APIKey represents an API key for programmatic access.
type APIKey struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	KeyHash   string     `json:"-"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at"`
	LastUsed  *time.Time `json:"last_used"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateAPIKeyRequest holds the fields needed to create an API key.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Project represents a registered allure project.
type Project struct {
	ID        string
	CreatedAt time.Time
	Tags      []string
}

// BuildStats holds aggregated test statistics for a build.
type BuildStats struct {
	Passed         int
	Failed         int
	Broken         int
	Skipped        int
	Unknown        int
	Total          int
	DurationMs     int64
	FlakyCount     int
	RetriedCount   int
	NewFailedCount int
	NewPassedCount int
}

// CIMetadata holds CI/CD context associated with a build.
type CIMetadata struct {
	Provider  string
	BuildURL  string
	Branch    string
	CommitSHA string
}

// Build represents a single report generation run for a project.
type Build struct {
	ID             int64
	ProjectID      string
	BuildOrder     int
	CreatedAt      time.Time
	StatPassed     *int
	StatFailed     *int
	StatBroken     *int
	StatSkipped    *int
	StatUnknown    *int
	StatTotal      *int
	DurationMs     *int64
	FlakyCount     *int
	RetriedCount   *int
	NewFailedCount *int
	NewPassedCount *int
	IsLatest       bool
	CIProvider     *string
	CIBuildURL     *string
	CIBranch       *string
	CICommitSHA    *string
}

// SparklinePoint holds pass-rate data for a single build in a trend sparkline.
type SparklinePoint struct {
	BuildOrder int
	PassRate   float64
	CreatedAt  time.Time
}

// DashboardProject bundles a project with its latest build and sparkline data.
type DashboardProject struct {
	ProjectID string
	CreatedAt time.Time
	Tags      []string
	Latest    *Build
	Sparkline []SparklinePoint
}

// TestResult represents a single test execution result stored in the database.
type TestResult struct {
	BuildID    int64
	ProjectID  string
	TestName   string
	FullName   string
	Status     string
	HistoryID  string
	DurationMs int64
	Flaky      bool
	Retries    int
	NewFailed  bool
	NewPassed  bool
	StartMs    *int64
	StopMs     *int64
	Thread     string
	Host       string
}

// LowPerformingTest holds aggregated metrics for a test that performs poorly.
type LowPerformingTest struct {
	TestName   string
	FullName   string
	HistoryID  string
	Metric     float64
	BuildCount int
	Trend      []float64
}

// TimelineRow holds timeline data for a single test execution.
type TimelineRow struct {
	TestName string
	FullName string
	Status   string
	StartMs  int64
	StopMs   int64
	Thread   string
	Host     string
}

// TestHistoryEntry holds data about a single run in a test's execution history.
type TestHistoryEntry struct {
	BuildOrder  int
	BuildID     int64
	Status      string
	DurationMs  int64
	CreatedAt   time.Time
	CICommitSHA *string
}

// KnownIssue represents a known test failure that has been acknowledged.
type KnownIssue struct {
	ID          int64
	ProjectID   string
	TestName    string
	Pattern     string
	TicketURL   string
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Branch represents a git branch associated with a project.
type Branch struct {
	ID        int64
	ProjectID string
	Name      string
	IsDefault bool
	CreatedAt time.Time
}

// ProjectMatch is returned by search for project queries.
type ProjectMatch struct {
	ID        string
	CreatedAt time.Time
}

// TestMatch is returned by search for test queries.
type TestMatch struct {
	ProjectID string
	TestName  string
	FullName  string
	Status    string
}

// DiffCategory describes how a test changed between two builds.
type DiffCategory string

const (
	DiffRegressed DiffCategory = "regressed"
	DiffFixed     DiffCategory = "fixed"
	DiffAdded     DiffCategory = "added"
	DiffRemoved   DiffCategory = "removed"
)

// DiffEntry represents a single test in a build comparison result.
type DiffEntry struct {
	TestName  string
	FullName  string
	HistoryID string
	StatusA   string
	StatusB   string
	DurationA int64
	DurationB int64
	Category  DiffCategory
}
