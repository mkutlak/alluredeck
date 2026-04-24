package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/parser"
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
	ErrDefectNotFound            = errors.New("defect fingerprint not found")
	ErrWebhookNotFound           = errors.New("webhook not found")
	ErrPreferencesNotFound       = errors.New("preferences not found")
	ErrRefreshFamilyNotFound     = errors.New("refresh token family not found")
)

// ProjectStorer is the interface for project operations.
type ProjectStorer interface {
	CreateProject(ctx context.Context, slug string) (*Project, error)
	CreateProjectWithParent(ctx context.Context, slug string, parentID int64) (*Project, error)
	GetProject(ctx context.Context, id int64) (*Project, error)
	GetProjectBySlug(ctx context.Context, slug string) (*Project, error)
	GetProjectBySlugAny(ctx context.Context, slug string) (*Project, error)
	ListProjects(ctx context.Context) ([]Project, error)
	ListProjectsPaginated(ctx context.Context, page, perPage int) ([]Project, int, error)
	ListProjectsPaginatedTopLevel(ctx context.Context, page, perPage int) ([]Project, int, error)
	ListChildren(ctx context.Context, parentID int64) ([]Project, error)
	ListChildIDs(ctx context.Context, parentID int64) ([]string, error)
	HasChildren(ctx context.Context, projectID int64) (bool, error)
	SetParent(ctx context.Context, projectID, parentID int64) error
	ClearParent(ctx context.Context, projectID int64) error
	DeleteProject(ctx context.Context, id int64) error
	RenameProject(ctx context.Context, id int64, newSlug string) error
	ProjectExists(ctx context.Context, id int64) (bool, error)
	SetReportType(ctx context.Context, id int64, reportType string) error
}

// BuildStorer is the interface for build operations.
type BuildStorer interface {
	NextBuildNumber(ctx context.Context, projectID int64) (int, error)
	InsertBuild(ctx context.Context, projectID int64, buildNumber int) error
	UpdateBuildStats(ctx context.Context, projectID int64, buildNumber int, stats BuildStats) error
	UpdateBuildCIMetadata(ctx context.Context, projectID int64, buildNumber int, ciMeta CIMetadata) error
	GetBuildByNumber(ctx context.Context, projectID int64, buildNumber int) (Build, error)
	GetPreviousBuild(ctx context.Context, projectID int64, buildNumber int) (Build, error)
	GetLatestBuild(ctx context.Context, projectID int64) (Build, error)
	ListBuilds(ctx context.Context, projectID int64) ([]Build, error)
	ListBuildsPaginated(ctx context.Context, projectID int64, page, perPage int) ([]Build, int, error)
	PruneBuilds(ctx context.Context, projectID int64, keep int) ([]int, error)
	SetLatest(ctx context.Context, projectID int64, buildNumber int) error
	DeleteAllBuilds(ctx context.Context, projectID int64) error
	GetDashboardData(ctx context.Context, sparklineDepth int) ([]DashboardProject, error)
	DeleteBuild(ctx context.Context, projectID int64, buildNumber int) error
	UpdateBuildBranchID(ctx context.Context, projectID int64, buildNumber int, branchID int64) error
	SetLatestBranch(ctx context.Context, projectID int64, buildNumber int, branchID *int64) error
	PruneBuildsBranch(ctx context.Context, projectID int64, keep int, branchID *int64) ([]int, error)
	PruneBuildsByAge(ctx context.Context, projectID int64, olderThan time.Time) ([]int, error)
	ListBuildsPaginatedBranch(ctx context.Context, projectID int64, page, perPage int, branchID *int64) ([]Build, int, error)
	ListBuildsInRange(ctx context.Context, projectID int64, branchID *int64, from, to time.Time, limit int) ([]Build, int, error)
	SetHasPlaywrightReport(ctx context.Context, projectID int64, buildNumber int, value bool) error
}

// TestResultStorer is the interface for test result operations.
type TestResultStorer interface {
	InsertBatch(ctx context.Context, results []TestResult) error
	InsertBatchFull(ctx context.Context, buildID int64, projectID int64, results []*parser.Result) error
	GetBuildID(ctx context.Context, projectID int64, buildNumber int) (int64, error)
	ListSlowest(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListLeastReliable(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListTimeline(ctx context.Context, projectID int64, buildID int64, limit int) ([]TimelineRow, error)
	ListFailedByBuild(ctx context.Context, projectID int64, buildID int64, limit int) ([]TestResult, error)
	GetTestHistory(ctx context.Context, projectID int64, historyID string, branchID *int64, limit int) ([]TestHistoryEntry, error)
	DeleteByBuild(ctx context.Context, buildID int64) error
	DeleteByProject(ctx context.Context, projectID int64) error
	CompareBuildsByHistoryID(ctx context.Context, projectID int64, buildIDA, buildIDB int64) ([]DiffEntry, error)
	ListTimelineMulti(ctx context.Context, projectID int64, buildIDs []int64, limit int) ([]MultiTimelineRow, error)
	ListFailedForFingerprinting(ctx context.Context, projectID int64, buildID int64) ([]FailedTestResult, error)
	ListStabilityByBuild(ctx context.Context, projectID int64, buildID int64) ([]TestResult, error)
}

// KnownIssueStorer is the interface for known issue operations.
type KnownIssueStorer interface {
	Create(ctx context.Context, projectID int64, testName, pattern, ticketURL, description string) (*KnownIssue, error)
	Get(ctx context.Context, id int64) (*KnownIssue, error)
	List(ctx context.Context, projectID int64, activeOnly bool) ([]KnownIssue, error)
	ListPaginated(ctx context.Context, projectID int64, activeOnly bool, page, perPage int) ([]KnownIssue, int, error)
	Update(ctx context.Context, id int64, projectID int64, ticketURL, description string, isActive bool) error
	Delete(ctx context.Context, id int64, projectID int64) error
	IsKnown(ctx context.Context, projectID int64, testName string) (bool, error)
}

// BlacklistStorer is the interface for JWT blacklist operations.
type BlacklistStorer interface {
	AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	PruneExpired(ctx context.Context) (int64, error)
}

// RefreshTokenFamilyStorer is the interface for refresh-token rotation storage
// with reuse detection (OAuth 2.0 BCP). Each login produces a family; every
// refresh rotates current_jti and records the previous value with a short grace
// window. A request that presents an already-rotated refresh JTI outside the
// grace window is token theft and should transition the family to
// RefreshTokenFamilyStatusCompromised.
type RefreshTokenFamilyStorer interface {
	// Create inserts a new refresh-token family row. The caller must populate
	// FamilyID (e.g. with a pre-generated UUID) as well as UserID, Role,
	// Provider, CurrentJTI, and ExpiresAt. Status defaults to 'active' when empty.
	Create(ctx context.Context, family RefreshTokenFamily) error

	// GetByID returns the family row identified by familyID. It returns
	// (nil, nil) when no row exists so callers can distinguish not-found from
	// database errors (matching the BlacklistStorer convention).
	GetByID(ctx context.Context, familyID string) (*RefreshTokenFamily, error)

	// Rotate atomically sets previous_jti = current_jti, current_jti = newJTI,
	// grace_until = NOW() + graceSeconds, and updated_at = NOW() for the given
	// family in a single UPDATE statement. It returns ErrRefreshFamilyNotFound
	// when the family does not exist.
	Rotate(ctx context.Context, familyID, newJTI string, graceSeconds int) error

	// MarkCompromised transitions the family's status to 'compromised'. It
	// returns ErrRefreshFamilyNotFound when the family does not exist.
	MarkCompromised(ctx context.Context, familyID string) error

	// Revoke transitions the family's status to 'revoked'. It returns
	// ErrRefreshFamilyNotFound when the family does not exist.
	Revoke(ctx context.Context, familyID string) error

	// DeleteExpired removes every row whose expires_at is strictly before NOW().
	// It returns the number of rows deleted.
	DeleteExpired(ctx context.Context) (int, error)
}

// BranchStorer is the interface for branch operations.
type BranchStorer interface {
	GetOrCreate(ctx context.Context, projectID int64, name string) (*Branch, bool, error)
	List(ctx context.Context, projectID int64) ([]Branch, error)
	GetDefault(ctx context.Context, projectID int64) (*Branch, error)
	SetDefault(ctx context.Context, projectID int64, branchID int64) error
	Delete(ctx context.Context, projectID int64, branchID int64) error
	GetByName(ctx context.Context, projectID int64, name string) (*Branch, error)
}

// SearchStorer is the interface for search operations.
type SearchStorer interface {
	SearchProjects(ctx context.Context, query string, limit int) ([]ProjectMatch, error)
	SearchTests(ctx context.Context, query string, limit int) ([]TestMatch, error)
}

// ErrorCluster holds a grouped failure message and its occurrence count.
type ErrorCluster struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// SuitePassRate holds per-suite pass rate data across recent builds.
type SuitePassRate struct {
	Suite    string  `json:"suite"`
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	PassRate float64 `json:"pass_rate"`
}

// LabelCount holds a label value and the count of distinct tests carrying it.
type LabelCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// TrendPoint holds per-build statistics for analytics trend charts.
type TrendPoint struct {
	BuildNumber int
	Passed      int
	Failed      int
	Broken      int
	Skipped     int
	Total       int
	PassRate    float64
	DurationMs  int64
}

// AnalyticsStorer provides analytics queries over the expanded test data schema.
type AnalyticsStorer interface {
	// ListTopErrors returns the most common failure messages across recent builds.
	ListTopErrors(ctx context.Context, projectIDs []int64, builds, limit int, branchID *int64) ([]ErrorCluster, error)
	// ListSuitePassRates returns per-suite pass rates across recent builds.
	ListSuitePassRates(ctx context.Context, projectIDs []int64, builds int, branchID *int64) ([]SuitePassRate, error)
	// ListLabelBreakdown returns counts grouped by label value for a given label name.
	ListLabelBreakdown(ctx context.Context, projectIDs []int64, labelName string, builds int, branchID *int64) ([]LabelCount, error)
	// ListTrendPoints returns per-build statistics for the last N builds, ordered chronologically (oldest first).
	ListTrendPoints(ctx context.Context, projectIDs []int64, builds int, branchID *int64) ([]TrendPoint, error)
}

// PipelineStorer provides cross-project pipeline run queries for parent projects.
type PipelineStorer interface {
	// ListPipelineRuns returns builds from child projects of the given parent,
	// grouped by ci_commit_sha via a CTE. Only builds with non-NULL ci_commit_sha
	// are included. Pagination operates on distinct commit SHAs, not individual rows.
	ListPipelineRuns(ctx context.Context, parentID int64, branch string, page, perPage int) ([]PipelineRunRow, int, error)
}

// AttachmentStorer provides queries over test attachment metadata.
type AttachmentStorer interface {
	ListByBuild(ctx context.Context, projectID int64, buildID int64, mimeFilter string, limit, offset int) ([]TestAttachment, int, error)
	GetBySource(ctx context.Context, buildID int64, source string) (*TestAttachment, error)
	// InsertBuildAttachments inserts build-level attachments (e.g. from Playwright
	// data/ directory) that are not linked to a specific test result.
	InsertBuildAttachments(ctx context.Context, buildID int64, projectID int64, attachments []TestAttachment) error
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
// for multi-instance safety. Implemented by *pg.Store.
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

// Defect category constants — mirror the CHECK constraint values stored in defect_fingerprints.category.
const (
	DefectCategoryProductBug     = "product_bug"
	DefectCategoryTestBug        = "test_bug"
	DefectCategoryInfrastructure = "infrastructure"
	DefectCategoryToInvestigate  = "to_investigate"
)

// Defect resolution constants — mirror the CHECK constraint values stored in defect_fingerprints.resolution.
const (
	DefectResolutionOpen    = "open"
	DefectResolutionFixed   = "fixed"
	DefectResolutionMuted   = "muted"
	DefectResolutionWontFix = "wont_fix"
)

// DefectFingerprint represents a deduplicated defect group derived from failed test results.
type DefectFingerprint struct {
	ID                     string `json:"id"`
	ProjectID              int64  `json:"project_id"`
	FingerprintHash        string `json:"fingerprint_hash"`
	NormalizedMessage      string `json:"normalized_message"`
	SampleTrace            string `json:"sample_trace"`
	Category               string `json:"category"`
	Resolution             string `json:"resolution"`
	KnownIssueID           *int64 `json:"known_issue_id,omitempty"`
	FirstSeenBuildID       int64  `json:"first_seen_build_id"`
	LastSeenBuildID        int64  `json:"last_seen_build_id"`
	OccurrenceCount        int    `json:"occurrence_count"`
	ConsecutiveCleanBuilds int    `json:"consecutive_clean_builds"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

// DefectListRow extends DefectFingerprint with build-context fields used in list responses.
type DefectListRow struct {
	DefectFingerprint
	TestResultCountInBuild *int        `json:"test_result_count_in_build,omitempty"`
	FirstSeenBuildNumber   int         `json:"first_seen_build_number"`
	LastSeenBuildNumber    int         `json:"last_seen_build_number"`
	IsRegression           bool        `json:"is_regression"`
	IsNew                  bool        `json:"is_new"`
	KnownIssue             *KnownIssue `json:"known_issue,omitempty"`
}

// DefectFilter holds query parameters for paginated defect list operations.
type DefectFilter struct {
	Resolution string
	Category   string
	Search     string
	SortBy     string
	Order      string
	Page       int
	PerPage    int
}

// DefectBuildSummary holds aggregated defect statistics for a single build.
type DefectBuildSummary struct {
	TotalGroups   int            `json:"total_groups"`
	AffectedTests int            `json:"affected_tests"`
	NewDefects    int            `json:"new_defects"`
	Regressions   int            `json:"regressions"`
	ByCategory    map[string]int `json:"by_category"`
	ByResolution  map[string]int `json:"by_resolution"`
}

// DefectProjectSummary holds aggregated defect statistics across all builds for a project.
type DefectProjectSummary struct {
	Open                 int            `json:"open"`
	Fixed                int            `json:"fixed"`
	Muted                int            `json:"muted"`
	WontFix              int            `json:"wont_fix"`
	RegressionsLastBuild int            `json:"regressions_last_build"`
	ByCategory           map[string]int `json:"by_category"`
}

// FailedTestResult holds the minimal fields needed for fingerprint heuristics.
type FailedTestResult struct {
	ID            int64
	StatusMessage string
	StatusTrace   string
}

// DefectStorer manages defect fingerprint lifecycle: upsert, linking, resolution, and querying.
type DefectStorer interface {
	// UpsertFingerprints inserts new fingerprints or updates existing ones for a build.
	UpsertFingerprints(ctx context.Context, projectID int64, buildID int64, fingerprints []DefectFingerprint) error
	// LinkTestResults associates individual test result rows with a fingerprint for a given build.
	LinkTestResults(ctx context.Context, fingerprintID string, buildID int64, testResultIDs []int64) error
	// UpdateCleanBuildCounts increments consecutive_clean_builds for fingerprints absent from the build.
	UpdateCleanBuildCounts(ctx context.Context, projectID int64, buildID int64) error
	// AutoResolveFixed transitions open fingerprints to "fixed" when they have reached the clean-build threshold.
	AutoResolveFixed(ctx context.Context, projectID int64, threshold int) (int, error)
	// DetectRegressions returns fingerprint IDs that were previously fixed but reappeared in this build.
	DetectRegressions(ctx context.Context, projectID int64, buildID int64) ([]string, error)
	// GetByHash retrieves a fingerprint by its project-scoped hash, returning ErrDefectNotFound if absent.
	GetByHash(ctx context.Context, projectID int64, hash string) (*DefectFingerprint, error)

	// ListByProject returns a paginated list of defects for a project with optional filters.
	ListByProject(ctx context.Context, projectID int64, filter DefectFilter) ([]DefectListRow, int, error)
	// ListByBuild returns a paginated list of defects observed in a specific build.
	ListByBuild(ctx context.Context, projectID int64, buildID int64, filter DefectFilter) ([]DefectListRow, int, error)
	// GetByID retrieves a single defect fingerprint by its UUID, returning ErrDefectNotFound if absent.
	GetByID(ctx context.Context, defectID string) (*DefectFingerprint, error)
	// GetTestResults returns paginated test results linked to a defect, optionally scoped to a build.
	GetTestResults(ctx context.Context, defectID string, buildID *int64, page, perPage int) ([]TestResult, int, error)
	// GetProjectSummary returns aggregated defect counts for a project.
	GetProjectSummary(ctx context.Context, projectID int64) (*DefectProjectSummary, error)
	// GetBuildSummary returns aggregated defect counts for a single build.
	GetBuildSummary(ctx context.Context, projectID int64, buildID int64) (*DefectBuildSummary, error)

	// UpdateDefect updates the category, resolution, or known-issue link for a single defect.
	UpdateDefect(ctx context.Context, defectID string, category, resolution *string, knownIssueID *int64) error
	// BulkUpdate applies category and/or resolution changes to multiple defects atomically.
	BulkUpdate(ctx context.Context, defectIDs []string, category, resolution *string) error
}

// WebhookStorer manages per-project webhook configurations and delivery logs.
type WebhookStorer interface {
	Create(ctx context.Context, wh *Webhook) (*Webhook, error)
	GetByID(ctx context.Context, webhookID string) (*Webhook, error)
	List(ctx context.Context, projectID int64) ([]Webhook, error)
	Update(ctx context.Context, wh *Webhook) error
	Delete(ctx context.Context, webhookID string, projectID int64) error
	ListActiveForEvent(ctx context.Context, projectID int64, event string) ([]Webhook, error)

	InsertDelivery(ctx context.Context, d *WebhookDelivery) error
	ListDeliveries(ctx context.Context, webhookID string, page, perPage int) ([]WebhookDelivery, int, error)
	PruneDeliveries(ctx context.Context, olderThan time.Time) (int64, error)
}

// UserPreferences holds persisted UI preferences for a user.
type UserPreferences struct {
	Username    string          `json:"username"`
	Preferences json.RawMessage `json:"preferences"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// PreferenceStorer manages user UI preferences.
type PreferenceStorer interface {
	GetPreferences(ctx context.Context, username string) (*UserPreferences, error)
	UpsertPreferences(ctx context.Context, username string, preferences json.RawMessage) (*UserPreferences, error)
}
