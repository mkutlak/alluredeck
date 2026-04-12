package store

import "time"

// User represents an authenticated user in the system.
// Provider is 'local' for password-based users and 'oidc' for SSO users.
type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Provider    string     `json:"provider"`
	ProviderSub string     `json:"-"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	LastLogin   *time.Time `json:"last_login"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

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

// Project represents a registered allure project.
type Project struct {
	ID          int64
	Slug        string
	ParentID    *int64
	DisplayName string
	ReportType  string // "allure" or "playwright"
	CreatedAt   time.Time
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
	ID                  int64
	ProjectID           int64
	BuildNumber         int
	CreatedAt           time.Time
	StatPassed          *int
	StatFailed          *int
	StatBroken          *int
	StatSkipped         *int
	StatUnknown         *int
	StatTotal           *int
	DurationMs          *int64
	FlakyCount          *int
	RetriedCount        *int
	NewFailedCount      *int
	NewPassedCount      *int
	IsLatest            bool
	CIProvider          *string
	CIBuildURL          *string
	CIBranch            *string
	CICommitSHA         *string
	HasPlaywrightReport bool
}

// SparklinePoint holds pass-rate data for a single build in a trend sparkline.
type SparklinePoint struct {
	BuildNumber int
	PassRate    float64
	CreatedAt   time.Time
}

// DashboardProject bundles a project with its latest build and sparkline data.
type DashboardProject struct {
	ProjectID   int64
	Slug        string
	ParentID    *int64
	DisplayName string
	ReportType  string
	CreatedAt   time.Time
	Latest      *Build
	Sparkline   []SparklinePoint
}

// PipelineRunRow is a flat row from the pipeline-runs query.
// The handler groups these by CommitSHA and computes aggregates.
type PipelineRunRow struct {
	CommitSHA   string
	Branch      string
	CIBuildURL  string
	CreatedAt   time.Time
	ProjectID   int64
	Slug        string
	BuildNumber int
	StatPassed  *int
	StatFailed  *int
	StatBroken  *int
	StatTotal   *int
	DurationMs  *int64
}

// TestResult represents a single test execution result stored in the database.
type TestResult struct {
	BuildID    int64
	ProjectID  int64
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

// TestAttachment represents a file attachment associated with a test result.
type TestAttachment struct {
	ID           int64
	TestResultID int64
	TestStepID   *int64
	Name         string
	Source       string
	MimeType     string
	SizeBytes    int64
	TestName     string // joined from test_results
	TestStatus   string // joined from test_results
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
	BuildNumber int
	BuildID     int64
	Status      string
	DurationMs  int64
	CreatedAt   time.Time
	CICommitSHA *string
}

// KnownIssue represents a known test failure that has been acknowledged.
type KnownIssue struct {
	ID          int64
	ProjectID   int64
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
	ProjectID int64
	Name      string
	IsDefault bool
	CreatedAt time.Time
}

// ProjectMatch is returned by search for project queries.
type ProjectMatch struct {
	ID        int64
	Slug      string
	CreatedAt time.Time
}

// TestMatch is returned by search for test queries.
type TestMatch struct {
	ProjectID int64
	Slug      string
	TestName  string
	FullName  string
	Status    string
}

// MultiTimelineRow holds timeline data for a test execution across multiple builds.
type MultiTimelineRow struct {
	BuildID     int64
	BuildNumber int
	TestName    string
	FullName    string
	Status      string
	StartMs     int64
	StopMs      int64
	Thread      string
	Host        string
}

// DiffCategory describes how a test changed between two builds.
type DiffCategory string

const (
	DiffRegressed DiffCategory = "regressed"
	DiffFixed     DiffCategory = "fixed"
	DiffAdded     DiffCategory = "added"
	DiffRemoved   DiffCategory = "removed"
)

// TestStatus represents the execution status of a test result.
type TestStatus string

const (
	TestStatusPassed  TestStatus = "passed"
	TestStatusFailed  TestStatus = "failed"
	TestStatusBroken  TestStatus = "broken"
	TestStatusSkipped TestStatus = "skipped"
	TestStatusUnknown TestStatus = "unknown"
)

// WebhookTargetType represents a supported webhook target platform.
type WebhookTargetType string

const (
	WebhookTargetSlack   WebhookTargetType = "slack"
	WebhookTargetDiscord WebhookTargetType = "discord"
	WebhookTargetTeams   WebhookTargetType = "teams"
	WebhookTargetGeneric WebhookTargetType = "generic"
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

// Webhook represents a per-project webhook notification target.
type Webhook struct {
	ID         string            `json:"id"`
	ProjectID  int64             `json:"project_id"`
	Name       string            `json:"name"`
	TargetType WebhookTargetType `json:"target_type"`
	URL        string            `json:"-"`
	Secret     *string           `json:"-"`
	Template   *string           `json:"template,omitempty"`
	Events     []string          `json:"events"`
	IsActive   bool              `json:"is_active"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// RefreshTokenFamilyStatus values recognised by the refresh-token rotation store.
const (
	RefreshTokenFamilyStatusActive      = "active"
	RefreshTokenFamilyStatusCompromised = "compromised"
	RefreshTokenFamilyStatusRevoked     = "revoked"
)

// RefreshTokenFamily represents a single row in the refresh_token_families table.
// A family tracks the chain of refresh tokens that originated from a single login.
// On each refresh the current_jti is rotated to a new one and recorded in previous_jti
// with a short grace window so benign retries do not trigger reuse detection.
type RefreshTokenFamily struct {
	FamilyID    string
	UserID      string
	Role        string
	Provider    string
	CurrentJTI  string
	PreviousJTI *string
	GraceUntil  *time.Time
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

// WebhookDelivery records one delivery attempt for audit/debugging.
type WebhookDelivery struct {
	ID           string    `json:"id"`
	WebhookID    string    `json:"webhook_id"`
	BuildID      *int64    `json:"build_id,omitempty"`
	Event        string    `json:"event"`
	Payload      string    `json:"payload"`
	StatusCode   *int      `json:"status_code,omitempty"`
	ResponseBody *string   `json:"response_body,omitempty"`
	Error        *string   `json:"error,omitempty"`
	Attempt      int       `json:"attempt"`
	DurationMs   *int      `json:"duration_ms,omitempty"`
	DeliveredAt  time.Time `json:"delivered_at"`
}
