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
	ErrTestResultNotFound        = errors.New("test result not found")
	ErrWebhookNotFound           = errors.New("webhook not found")
	ErrPreferencesNotFound       = errors.New("preferences not found")
	ErrRefreshFamilyNotFound     = errors.New("refresh token family not found")
	// ErrEmailAlreadyLinked is returned by UpsertByOIDC when the email is
	// already bound to a different (provider, provider_sub) identity. F-5 of
	// SECURITY_REVIEW.md uses this to enforce one account per email across
	// all OIDC (and local) providers.
	ErrEmailAlreadyLinked = errors.New("email already linked to a different identity")
)

// ProjectReader covers project lookup and listing queries.
type ProjectReader interface {
	GetProject(ctx context.Context, id int64) (*Project, error)
	GetProjectBySlug(ctx context.Context, slug string) (*Project, error)
	GetProjectBySlugAny(ctx context.Context, slug string) (*Project, error)
	ListProjects(ctx context.Context) ([]Project, error)
	ListProjectsPaginated(ctx context.Context, page, perPage int) ([]Project, int, error)
	ListProjectsPaginatedTopLevel(ctx context.Context, page, perPage int) ([]Project, int, error)
	ProjectExists(ctx context.Context, id int64) (bool, error)
}

// ProjectWriter covers project creation, deletion, and mutation.
type ProjectWriter interface {
	CreateProject(ctx context.Context, slug string) (*Project, error)
	CreateProjectWithParent(ctx context.Context, slug string, parentID int64) (*Project, error)
	DeleteProject(ctx context.Context, id int64) error
	RenameProject(ctx context.Context, id int64, newSlug string) error
	SetReportType(ctx context.Context, id int64, reportType string) error
}

// ProjectHierarchyStorer covers parent/child project relationship operations.
type ProjectHierarchyStorer interface {
	ListChildren(ctx context.Context, parentID int64) ([]Project, error)
	ListChildIDs(ctx context.Context, parentID int64) ([]string, error)
	HasChildren(ctx context.Context, projectID int64) (bool, error)
	SetParent(ctx context.Context, projectID, parentID int64) error
	ClearParent(ctx context.Context, projectID int64) error
}

// ProjectHierarchyReader composes project lookup with parent/child hierarchy
// operations — the role used by handlers that resolve projects and inspect or
// mutate parent/child relationships but never create, rename, or delete projects.
type ProjectHierarchyReader interface {
	ProjectReader
	ProjectHierarchyStorer
}

// ProjectStorer is the interface for project operations.
type ProjectStorer interface {
	ProjectReader
	ProjectWriter
	ProjectHierarchyStorer
}

// BuildWriter covers build lifecycle and CI ingestion writes.
type BuildWriter interface {
	NextBuildNumber(ctx context.Context, projectID int64) (int, error)
	InsertBuild(ctx context.Context, projectID int64, buildNumber int) error
	UpdateBuildStats(ctx context.Context, projectID int64, buildNumber int, stats BuildStats) error
	UpdateBuildCIMetadata(ctx context.Context, projectID int64, buildNumber int, ciMeta CIMetadata) error
	UpdateBuildEnvironment(ctx context.Context, projectID int64, buildNumber int, env map[string]string) error
	UpdateBuildBranchID(ctx context.Context, projectID int64, buildNumber int, branchID int64) error
	SetLatest(ctx context.Context, projectID int64, buildNumber int) error
	SetLatestBranch(ctx context.Context, projectID int64, buildNumber int, branchID *int64) error
	SetHasPlaywrightReport(ctx context.Context, projectID int64, buildNumber int, value bool) error
}

// BuildReader covers build lookup and listing queries.
type BuildReader interface {
	GetBuildByNumber(ctx context.Context, projectID int64, buildNumber int) (Build, error)
	GetPreviousBuild(ctx context.Context, projectID int64, buildNumber int) (Build, error)
	GetLatestBuild(ctx context.Context, projectID int64) (Build, error)
	ListBuilds(ctx context.Context, projectID int64) ([]Build, error)
	ListBuildsPaginated(ctx context.Context, projectID int64, page, perPage int) ([]Build, int, error)
	ListBuildsPaginatedBranch(ctx context.Context, projectID int64, page, perPage int, branchID *int64) ([]Build, int, error)
	ListBuildsInRange(ctx context.Context, projectID int64, branchID *int64, from, to time.Time, limit int) ([]Build, int, error)
	// BuildExists reports whether a build with the given build_id (primary key)
	// belongs to the given project. Used by MCP tools to fail fast before
	// performing heavier queries — build_id ≠ build_number (UI URL uses build_number).
	BuildExists(ctx context.Context, projectID, buildID int64) (bool, error)
	// GetBuildByID returns the build row for a given project_id + build_id (primary
	// key). Returns store.ErrBuildNotFound when the build does not exist or belongs
	// to a different project.
	GetBuildByID(ctx context.Context, projectID, buildID int64) (Build, error)
}

// BuildPruner covers build retention and deletion operations.
type BuildPruner interface {
	PruneBuilds(ctx context.Context, projectID int64, keep int) ([]int, error)
	PruneBuildsBranch(ctx context.Context, projectID int64, keep int, branchID *int64) ([]int, error)
	PruneBuildsByAge(ctx context.Context, projectID int64, olderThan time.Time) ([]int, error)
	DeleteBuild(ctx context.Context, projectID int64, buildNumber int) error
	DeleteAllBuilds(ctx context.Context, projectID int64) error
}

// BuildDashboardReader covers cross-project dashboard aggregation queries.
type BuildDashboardReader interface {
	GetDashboardData(ctx context.Context, sparklineDepth int) ([]DashboardProject, error)
}

// BuildStorer is the interface for build operations.
type BuildStorer interface {
	BuildWriter
	BuildReader
	BuildPruner
	BuildDashboardReader
}

// TestResultWriter covers test result ingestion writes.
type TestResultWriter interface {
	InsertBatch(ctx context.Context, results []TestResult) error
	InsertBatchFull(ctx context.Context, buildID int64, projectID int64, results []*parser.Result) error
}

// TestResultPruner covers test result deletion operations.
type TestResultPruner interface {
	DeleteByBuild(ctx context.Context, buildID int64) error
	DeleteByProject(ctx context.Context, projectID int64) error
}

// TestResultReader covers test result lookup, history, and analytics queries.
type TestResultReader interface {
	GetBuildID(ctx context.Context, projectID int64, buildNumber int) (int64, error)
	ListSlowest(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListLeastReliable(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]LowPerformingTest, error)
	ListTimeline(ctx context.Context, projectID int64, buildID int64, limit int) ([]TimelineRow, error)
	ListTimelineMulti(ctx context.Context, projectID int64, buildIDs []int64, limit int) ([]MultiTimelineRow, error)
	ListFailedByBuild(ctx context.Context, projectID int64, buildID int64, limit int) ([]TestResult, error)
	ListStabilityByBuild(ctx context.Context, projectID int64, buildID int64) ([]TestResult, error)
	GetTestHistory(ctx context.Context, projectID int64, historyID string, branchID *int64, limit int) ([]TestHistoryEntry, error)
	CompareBuildsByHistoryID(ctx context.Context, projectID int64, buildIDA, buildIDB int64) ([]DiffEntry, error)
}

// TestResultDefectReader covers fingerprinting and defect-oriented test result queries.
type TestResultDefectReader interface {
	ListFailedForFingerprinting(ctx context.Context, projectID int64, buildID int64) ([]FailedTestResult, error)
	// SearchByName returns up to limit test results whose full_name matches
	// the given substring (case-insensitive). Used by the find_test_by_name MCP tool.
	SearchByName(ctx context.Context, projectID int64, substring string, limit int) ([]*TestResult, error)
	// ListRecentMessages returns up to limit distinct non-empty status_message
	// values from failed/broken test_results for a project. Used by the
	// propose_known_issue dry-run to estimate how many recent failures a regex
	// would match without a full table scan.
	ListRecentMessages(ctx context.Context, projectID int64, limit int) ([]string, error)
	// GetDefectFingerprintID returns the defect_fingerprint_id linked to the
	// test_results row identified by (projectID, buildID, historyID). The
	// returned pointer is nil when the row exists but has no linked fingerprint.
	// Returns ErrTestResultNotFound when no matching row exists.
	GetDefectFingerprintID(ctx context.Context, projectID int64, buildID int64, historyID string) (*string, error)
	// GetFailedStepPath reconstructs the failed-step trail for the test_results
	// row identified by (projectID, buildID, historyID). It walks the test_steps
	// tree, picking failed/broken steps at each level and descending into the
	// first such child. It returns the ordered list of step names from the root
	// failed step to the deepest failed step, plus the status_message of that
	// deepest failed step (the most specific error text available). Both are
	// empty when the test has no recorded steps or no failed step. Returns
	// ErrTestResultNotFound when no matching test_results row exists.
	GetFailedStepPath(ctx context.Context, projectID int64, buildID int64, historyID string) (path []string, errorMessage string, err error)
	// MarkFlakyByHistoryID sets flaky=true on the most-recent test_results row
	// matching (project_id, history_id, full_name). Used by the proposal approval
	// flow to apply a flaky-proposal without modifying historical rows.
	MarkFlakyByHistoryID(ctx context.Context, projectID int64, historyID, fullName string) error
}

// TestResultStorer is the interface for test result operations.
type TestResultStorer interface {
	TestResultWriter
	TestResultPruner
	TestResultReader
	TestResultDefectReader
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

	// RevokeAllForUser sets status='revoked' for every active family belonging
	// to the given user_id. Returns the number of families revoked. Used by
	// password change/reset and account deactivation to invalidate every live
	// session in one call. Missing-user is not an error: the count is simply 0.
	RevokeAllForUser(ctx context.Context, userID string) (int, error)

	// DeleteExpired removes every row whose expires_at is strictly before NOW().
	// It returns the number of rows deleted.
	DeleteExpired(ctx context.Context) (int, error)
}

// AuditLogger persists security-sensitive events for incident response and
// compliance. Calls are best-effort from the caller's perspective:
// implementations must NOT fail the request they are auditing — handlers log
// the error and continue serving the response.
//
// Records are append-only. There is no Update or Delete method; retention is
// applied externally (e.g. by a future scheduled job that drops rows older
// than N days).
type AuditLogger interface {
	// Record inserts a single audit event. Implementations MUST populate
	// occurred_at server-side when AuditEvent.OccurredAt is the zero value
	// so callers do not need to provide a timestamp.
	Record(ctx context.Context, evt AuditEvent) error
	// ListRecent returns the most recent audit events, newest first, capped
	// at limit. The returned slice may be shorter than limit. Used by the
	// (future) admin audit-log page; OK to be slow.
	ListRecent(ctx context.Context, limit int) ([]AuditEvent, error)
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
	ListByBuild(ctx context.Context, projectID int64, buildID int64, mimeFilter, testStatus string, limit, offset int) ([]TestAttachment, int, error)
	// ListByTestResult returns the attachments belonging to a single test
	// result, identified by (projectID, buildID, historyID). It resolves the
	// test_results primary key the same way GetFailedStepPath /
	// GetDefectFingerprintID do, then scopes test_attachments via
	// test_result_id so callers receive only that test's own attachments
	// rather than every attachment in the build.
	ListByTestResult(ctx context.Context, projectID int64, buildID int64, historyID string, limit int) ([]TestAttachment, error)
	GetBySource(ctx context.Context, buildID int64, source string) (*TestAttachment, error)
	// GetByID returns a single attachment row by its primary key. Returns
	// ErrAttachmentNotFound when no row exists.
	GetByID(ctx context.Context, id int64) (*TestAttachment, error)
	// GetLocation resolves the file-storage location of an attachment by joining
	// test_attachments → test_results → builds → projects. It returns the
	// owning project's storage key, the build order number, the source
	// filename and the MIME type. Returns ErrAttachmentNotFound when no row
	// exists. Used to stream attachment blobs via signed download URLs and to
	// inline attachment content in MCP resources.
	GetLocation(ctx context.Context, id int64) (*AttachmentLocation, error)
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
	// DeleteAllForUser hard-deletes every API key owned by username. Returns
	// the number of rows deleted. Used by account deactivation and password
	// reset to ensure no stale credentials remain after a user-lifecycle event.
	// Missing-user is not an error: the count is simply 0.
	DeleteAllForUser(ctx context.Context, username string) (int, error)
}

// Locker serialises per-project operations using PostgreSQL advisory locks
// for multi-instance safety. Implemented by *pg.Store.
type Locker interface {
	AcquireLock(ctx context.Context, key string) (func(), error)
}

// ListUsersParams holds filters and pagination for UserStorer.ListPaginated.
type ListUsersParams struct {
	Limit  int
	Offset int
	Search string
	Role   string
	Active *bool
}

// UserAuthStorer covers user lookup and identity operations used by authentication flows.
type UserAuthStorer interface {
	// UpsertByOIDC inserts or updates a user record using the OIDC subject claim as the key.
	// On conflict (same provider + provider_sub), updates email, name, role, last_login, updated_at.
	UpsertByOIDC(ctx context.Context, provider, sub, email, name, role string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	// UpdateLastLogin refreshes the user's last_login (and updated_at) columns to
	// the current time. Returns ErrUserNotFound if no row exists. Callers should
	// treat failures as best-effort and not fail the surrounding request.
	UpdateLastLogin(ctx context.Context, id int64) error
	// RelinkOIDC rebinds an existing users row to a new (provider, provider_sub)
	// identity and refreshes last_login. Used by F-5 of SECURITY_REVIEW.md when
	// OIDC_AUTO_LINK_BY_EMAIL is enabled and the IdP marked the colliding email
	// as verified — the operator has explicitly opted in to letting a verified
	// OIDC sign-in take over an existing account. Returns ErrUserNotFound when
	// no row matches id.
	RelinkOIDC(ctx context.Context, id int64, provider, sub string) error
}

// UserAdminStorer covers user administration: creation, listing, and lifecycle.
type UserAdminStorer interface {
	// CreateLocal inserts a new local (password-based) user. email is stored as
	// received; callers should normalise (lowercase/trim) before invoking.
	CreateLocal(ctx context.Context, email, name, passwordHash, role string) (*User, error)
	// List returns every user ordered by created_at DESC. Retained for
	// back-compat; new code should prefer ListPaginated.
	List(ctx context.Context) ([]User, error)
	// ListPaginated returns a filtered page of users and the total row count
	// matching the filters (ignoring Limit/Offset).
	ListPaginated(ctx context.Context, params ListUsersParams) ([]User, int, error)
	// UpdateRole changes the user's role. Returns ErrUserNotFound if no row exists.
	UpdateRole(ctx context.Context, id int64, role string) error
	// UpdateActive toggles the user's is_active flag. Returns ErrUserNotFound if no row exists.
	UpdateActive(ctx context.Context, id int64, active bool) error
	Deactivate(ctx context.Context, id int64) error
}

// UserProfileStorer covers self-service user profile and credential updates.
type UserProfileStorer interface {
	// UpdateProfile updates the user's display name. Returns ErrUserNotFound if no row exists.
	UpdateProfile(ctx context.Context, id int64, name string) error
	// UpdatePasswordHash replaces the user's password_hash (and bumps updated_at).
	// Returns ErrUserNotFound if no row exists. Callers must already have
	// generated the bcrypt hash — the store never sees the plaintext password.
	UpdatePasswordHash(ctx context.Context, id int64, passwordHash string) error
}

// UserStorer manages user records for authentication and JIT provisioning.
type UserStorer interface {
	UserAuthStorer
	UserAdminStorer
	UserProfileStorer
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

// DefectWriter covers defect fingerprint ingestion: upsert, linking, and lifecycle automation.
type DefectWriter interface {
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
}

// DefectReader covers defect fingerprint lookup, listing, and summary queries.
type DefectReader interface {
	// GetByHash retrieves a fingerprint by its project-scoped hash, returning ErrDefectNotFound if absent.
	GetByHash(ctx context.Context, projectID int64, hash string) (*DefectFingerprint, error)
	// GetByID retrieves a single defect fingerprint by its UUID, returning ErrDefectNotFound if absent.
	GetByID(ctx context.Context, defectID string) (*DefectFingerprint, error)
	// ListByProject returns a paginated list of defects for a project with optional filters.
	ListByProject(ctx context.Context, projectID int64, filter DefectFilter) ([]DefectListRow, int, error)
	// ListByBuild returns a paginated list of defects observed in a specific build.
	ListByBuild(ctx context.Context, projectID int64, buildID int64, filter DefectFilter) ([]DefectListRow, int, error)
	// GetTestResults returns paginated test results linked to a defect, optionally scoped to a build.
	GetTestResults(ctx context.Context, defectID string, buildID *int64, page, perPage int) ([]TestResult, int, error)
	// GetProjectSummary returns aggregated defect counts for a project.
	GetProjectSummary(ctx context.Context, projectID int64) (*DefectProjectSummary, error)
	// GetBuildSummary returns aggregated defect counts for a single build.
	GetBuildSummary(ctx context.Context, projectID int64, buildID int64) (*DefectBuildSummary, error)
}

// DefectClassifier covers manual defect reclassification operations.
type DefectClassifier interface {
	// UpdateDefect updates the category, resolution, or known-issue link for a single defect.
	UpdateDefect(ctx context.Context, defectID string, category, resolution *string, knownIssueID *int64) error
	// BulkUpdate applies category and/or resolution changes to multiple defects atomically.
	BulkUpdate(ctx context.Context, defectIDs []string, category, resolution *string) error
}

// DefectFingerprintWriter is the defect role used by the report runners during
// ingestion: it composes DefectWriter (upsert/link/clean-build/auto-resolve/
// regression) with the single GetByHash lookup needed to resolve a freshly
// upserted fingerprint's UUID before linking test results to it.
type DefectFingerprintWriter interface {
	DefectWriter
	// GetByHash retrieves a fingerprint by its project-scoped hash, returning ErrDefectNotFound if absent.
	GetByHash(ctx context.Context, projectID int64, hash string) (*DefectFingerprint, error)
}

// DefectStorer manages defect fingerprint lifecycle: upsert, linking, resolution, and querying.
type DefectStorer interface {
	DefectWriter
	DefectReader
	DefectClassifier
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

// DefectProposalStorer manages MCP-proposed defect reclassifications.
type DefectProposalStorer interface {
	// Create inserts a new defect proposal and returns its assigned ID.
	Create(ctx context.Context, p *DefectProposal) (int64, error)
	// Get retrieves a single defect proposal by ID.
	Get(ctx context.Context, id int64) (*DefectProposal, error)
	// ListPending returns pending proposals for a project with cursor-based pagination.
	// cursor is an opaque token from the previous call; "" starts from the beginning.
	// Returns items, the next cursor (empty when no more pages), and any error.
	ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*DefectProposal, string, error)
	// MarkReviewed sets status + reviewed_by_user_id + reviewed_at on a proposal.
	MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status ProposalStatus) error
}

// KnownIssueProposalStorer manages MCP-proposed known-issue rules.
type KnownIssueProposalStorer interface {
	// Create inserts a new known-issue proposal and returns its assigned ID.
	Create(ctx context.Context, p *KnownIssueProposal) (int64, error)
	// Get retrieves a single known-issue proposal by ID.
	Get(ctx context.Context, id int64) (*KnownIssueProposal, error)
	// ListPending returns pending proposals for a project with cursor-based pagination.
	ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*KnownIssueProposal, string, error)
	// MarkReviewed sets status + reviewed_by_user_id + reviewed_at on a proposal.
	MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status ProposalStatus) error
}

// FlakyProposalStorer manages MCP-proposed flaky-test flags.
type FlakyProposalStorer interface {
	// Create inserts a new flaky proposal and returns its assigned ID.
	Create(ctx context.Context, p *FlakyProposal) (int64, error)
	// Get retrieves a single flaky proposal by ID.
	Get(ctx context.Context, id int64) (*FlakyProposal, error)
	// ListPending returns pending proposals for a project with cursor-based pagination.
	ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*FlakyProposal, string, error)
	// MarkReviewed sets status + reviewed_by_user_id + reviewed_at on a proposal.
	MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status ProposalStatus) error
}
