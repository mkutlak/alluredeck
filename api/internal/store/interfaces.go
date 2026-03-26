package store

import (
	"context"
	"errors"
	"time"

	parser "github.com/mkutlak/alluredeck/api/internal/parser"
)

// Sentinel errors returned by store operations.
// Callers use errors.Is() to check for these without importing implementation packages.
var (
	ErrProjectNotFound           = errors.New("project not found")
	ErrProjectExists             = errors.New("project already exists")
	ErrBuildNotFound             = errors.New("build not found")
	ErrBranchNotFound            = errors.New("branch not found")
	ErrCannotDeleteDefaultBranch = errors.New("cannot delete default branch")
	ErrKnownIssueNotFound        = errors.New("known issue not found")
	ErrDuplicateEntry            = errors.New("unique constraint violation")
	ErrAPIKeyNotFound            = errors.New("api key not found")
	ErrUserNotFound              = errors.New("user not found")
	ErrAttachmentNotFound        = errors.New("attachment not found")
)

// ProjectStorer is the interface for project operations.
type ProjectStorer interface {
	CreateProject(ctx context.Context, id string) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context) ([]Project, error)
	ListProjectsPaginated(ctx context.Context, page, perPage int, tag string) ([]Project, int, error)
	ListAllTags(ctx context.Context) ([]string, error)
	SetTags(ctx context.Context, projectID string, tags []string) error
	DeleteProject(ctx context.Context, id string) error
	ProjectExists(ctx context.Context, id string) (bool, error)
}

// BuildStorer is the interface for build operations.
type BuildStorer interface {
	NextBuildOrder(ctx context.Context, projectID string) (int, error)
	InsertBuild(ctx context.Context, projectID string, buildOrder int) error
	UpdateBuildStats(ctx context.Context, projectID string, buildOrder int, stats BuildStats) error
	UpdateBuildCIMetadata(ctx context.Context, projectID string, buildOrder int, ciMeta CIMetadata) error
	GetBuildByOrder(ctx context.Context, projectID string, buildOrder int) (Build, error)
	GetPreviousBuild(ctx context.Context, projectID string, buildOrder int) (Build, error)
	GetLatestBuild(ctx context.Context, projectID string) (Build, error)
	ListBuilds(ctx context.Context, projectID string) ([]Build, error)
	ListBuildsPaginated(ctx context.Context, projectID string, page, perPage int) ([]Build, int, error)
	PruneBuilds(ctx context.Context, projectID string, keep int) ([]int, error)
	SetLatest(ctx context.Context, projectID string, buildOrder int) error
	DeleteAllBuilds(ctx context.Context, projectID string) error
	GetDashboardData(ctx context.Context, sparklineDepth int, tag string) ([]DashboardProject, error)
	DeleteBuild(ctx context.Context, projectID string, buildOrder int) error
	UpdateBuildBranchID(ctx context.Context, projectID string, buildOrder int, branchID int64) error
	SetLatestBranch(ctx context.Context, projectID string, buildOrder int, branchID *int64) error
	PruneBuildsBranch(ctx context.Context, projectID string, keep int, branchID *int64) ([]int, error)
	PruneBuildsByAge(ctx context.Context, projectID string, olderThan time.Time) ([]int, error)
	ListBuildsPaginatedBranch(ctx context.Context, projectID string, page, perPage int, branchID *int64) ([]Build, int, error)
	ListBuildsInRange(ctx context.Context, projectID string, branchID *int64, from, to time.Time, limit int) ([]Build, int, error)
}

// TestResultStorer is the interface for test result operations.
type TestResultStorer interface {
	InsertBatch(ctx context.Context, results []TestResult) error
	InsertBatchFull(ctx context.Context, buildID int64, projectID string, results []*parser.Result) error
	GetBuildID(ctx context.Context, projectID string, buildOrder int) (int64, error)
	ListSlowest(ctx context.Context, projectID string, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListLeastReliable(ctx context.Context, projectID string, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListTimeline(ctx context.Context, projectID string, buildID int64, limit int) ([]TimelineRow, error)
	ListFailedByBuild(ctx context.Context, projectID string, buildID int64, limit int) ([]TestResult, error)
	GetTestHistory(ctx context.Context, projectID, historyID string, branchID *int64, limit int) ([]TestHistoryEntry, error)
	DeleteByBuild(ctx context.Context, buildID int64) error
	DeleteByProject(ctx context.Context, projectID string) error
	CompareBuildsByHistoryID(ctx context.Context, projectID string, buildIDA, buildIDB int64) ([]DiffEntry, error)
	ListTimelineMulti(ctx context.Context, projectID string, buildIDs []int64, limit int) ([]MultiTimelineRow, error)
}

// KnownIssueStorer is the interface for known issue operations.
type KnownIssueStorer interface {
	Create(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*KnownIssue, error)
	Get(ctx context.Context, id int64) (*KnownIssue, error)
	List(ctx context.Context, projectID string, activeOnly bool) ([]KnownIssue, error)
	ListPaginated(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]KnownIssue, int, error)
	Update(ctx context.Context, id int64, projectID, ticketURL, description string, isActive bool) error
	Delete(ctx context.Context, id int64, projectID string) error
	IsKnown(ctx context.Context, projectID, testName string) (bool, error)
}

// BlacklistStorer is the interface for JWT blacklist operations.
type BlacklistStorer interface {
	AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	PruneExpired(ctx context.Context) (int64, error)
}

// BranchStorer is the interface for branch operations.
type BranchStorer interface {
	GetOrCreate(ctx context.Context, projectID, name string) (*Branch, bool, error)
	List(ctx context.Context, projectID string) ([]Branch, error)
	GetDefault(ctx context.Context, projectID string) (*Branch, error)
	SetDefault(ctx context.Context, projectID string, branchID int64) error
	Delete(ctx context.Context, projectID string, branchID int64) error
	GetByName(ctx context.Context, projectID, name string) (*Branch, error)
}

// SearchStorer is the interface for search operations.
type SearchStorer interface {
	SearchProjects(ctx context.Context, query string, limit int) ([]ProjectMatch, error)
	SearchTests(ctx context.Context, query string, limit int) ([]TestMatch, error)
}

// ErrorCluster holds a grouped failure message and its occurrence count.
type ErrorCluster struct {
	Message string
	Count   int
}

// SuitePassRate holds per-suite pass rate data across recent builds.
type SuitePassRate struct {
	Suite    string
	Total    int
	Passed   int
	PassRate float64
}

// LabelCount holds a label value and the count of distinct tests carrying it.
type LabelCount struct {
	Value string
	Count int
}

// TrendPoint holds per-build statistics for analytics trend charts.
type TrendPoint struct {
	BuildOrder int
	Passed     int
	Failed     int
	Broken     int
	Skipped    int
	Total      int
	PassRate   float64
	DurationMs int64
}

// AnalyticsStorer provides analytics queries over the expanded test data schema.
type AnalyticsStorer interface {
	// ListTopErrors returns the most common failure messages across recent builds.
	ListTopErrors(ctx context.Context, projectID string, builds, limit int, branchID *int64) ([]ErrorCluster, error)
	// ListSuitePassRates returns per-suite pass rates across recent builds.
	ListSuitePassRates(ctx context.Context, projectID string, builds int, branchID *int64) ([]SuitePassRate, error)
	// ListLabelBreakdown returns counts grouped by label value for a given label name.
	ListLabelBreakdown(ctx context.Context, projectID, labelName string, builds int, branchID *int64) ([]LabelCount, error)
	// ListTrendPoints returns per-build statistics for the last N builds, ordered chronologically (oldest first).
	ListTrendPoints(ctx context.Context, projectID string, builds int, branchID *int64) ([]TrendPoint, error)
}

// AttachmentStorer provides queries over test attachment metadata.
type AttachmentStorer interface {
	ListByBuild(ctx context.Context, projectID string, buildID int64, mimeFilter string, limit, offset int) ([]TestAttachment, int, error)
	GetBySource(ctx context.Context, buildID int64, source string) (*TestAttachment, error)
}

// APIKeyStorer is the interface for API key operations.
type APIKeyStorer interface {
	Create(ctx context.Context, key *APIKey) (*APIKey, error)
	ListByUsername(ctx context.Context, username string) ([]APIKey, error)
	GetByHash(ctx context.Context, keyHash string) (*APIKey, error)
	UpdateLastUsed(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64, username string) error
	CountByUsername(ctx context.Context, username string) (int, error)
}

// Locker serialises per-project operations using PostgreSQL advisory locks
// for multi-instance safety. Implemented by *pg.PGStore.
type Locker interface {
	AcquireLock(ctx context.Context, key string) (func(), error)
}

// UserStorer manages user records for authentication and JIT provisioning.
type UserStorer interface {
	// UpsertByOIDC inserts or updates a user record using the OIDC subject claim as the key.
	// On conflict (same provider + provider_sub), updates email, name, role, last_login, updated_at.
	UpsertByOIDC(ctx context.Context, provider, sub, email, name, role string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context) ([]User, error)
	Deactivate(ctx context.Context, id int64) error
}
