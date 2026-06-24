package store

import (
	"encoding/json"
	"time"
)

// ProposalStatus represents the review state of an MCP proposal.
type ProposalStatus string

const (
	ProposalStatusPending  ProposalStatus = "pending"
	ProposalStatusApproved ProposalStatus = "approved"
	ProposalStatusRejected ProposalStatus = "rejected"
)

// User represents an authenticated user in the system.
// Provider is 'local' for password-based users and 'oidc' for SSO users.
type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	Provider     string     `json:"provider"`
	ProviderSub  string     `json:"-"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// APIKey represents an API key for programmatic access.
type APIKey struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Prefix         string     `json:"prefix"`
	KeyHash        string     `json:"-"`
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	ExpiresAt      *time.Time `json:"expires_at"`
	LastUsed       *time.Time `json:"last_used"`
	CreatedAt      time.Time  `json:"created_at"`
	AllowMCPWrites bool       `json:"allow_mcp_writes"`
	// ProjectIDs is an optional allow-list of project IDs this key may access.
	// An empty slice means the key is instance-wide (unrestricted).
	ProjectIDs []int64 `json:"project_ids,omitempty"`
}

// Project represents a registered allure project.
type Project struct {
	ID          int64
	Slug        string
	StorageKey  string
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
	Provider    string
	BuildURL    string
	Branch      string
	CommitSHA   string
	PipelineID  string
	PipelineURL string
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
	CIPipelineID        *string
	CIPipelineURL       *string
	HasPlaywrightReport bool
	Environment         map[string]string
	BranchID            *int64
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
// The handler groups these by PipelineID (if set) or CommitSHA and computes aggregates.
type PipelineRunRow struct {
	PipelineID  string
	PipelineURL string
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
	StatSkipped *int
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

// AttachmentLocation carries everything needed to resolve an attachment's blob
// in the file-storage backend: the owning project's storage key, the build
// order number, the source filename, and the MIME type. It is produced by
// AttachmentStorer.GetLocation by joining test_attachments → test_results →
// builds → projects.
type AttachmentLocation struct {
	StorageKey  string
	BuildNumber int
	Source      string
	MimeType    string
	SizeBytes   int64
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

// Webhook event name constants. These are the only values accepted in
// Webhook.Events and in the event filtering logic. Keep backwards-compatible:
// "report_completed" was the original implicit default.
const (
	WebhookEventReportCompleted    = "report_completed"
	WebhookEventReportFailed       = "report_failed"
	WebhookEventRegressionDetected = "regression_detected"
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

// Audit action constants enumerate every security-sensitive operation that the
// API records to the audit_log table. Handlers reference these by name (not by
// string literal) so the linter catches typos at compile time.
const (
	AuditActionLoginSuccess      = "auth.login.success"
	AuditActionLoginFailure      = "auth.login.failure"
	AuditActionLogout            = "auth.logout"
	AuditActionRefreshSuccess    = "auth.refresh.success"
	AuditActionRefreshCompromise = "auth.refresh.compromise"
	AuditActionUserCreate        = "users.create"
	AuditActionUserUpdateRole    = "users.update.role"
	AuditActionUserUpdateActive  = "users.update.active"
	AuditActionUserDelete        = "users.delete"
	AuditActionPasswordChange    = "users.password_change"
	AuditActionPasswordReset     = "users.password_reset"
	AuditActionAPIKeyCreate      = "api_keys.create"
	AuditActionAPIKeyDelete      = "api_keys.delete"
	// AuditActionSessionRevokeAll captures bulk refresh-token-family revocations
	// triggered by password change/reset or account deactivation. Metadata
	// includes the trigger and the count of families revoked.
	AuditActionSessionRevokeAll = "auth.session.revoke_all"
	// AuditActionAPIKeyCascadeDelete captures bulk API-key deletions triggered
	// by password reset or account deactivation. Metadata includes the trigger
	// and the count of keys deleted.
	AuditActionAPIKeyCascadeDelete = "api_keys.cascade_delete"
)

// Audit outcome constants are the only values accepted by the audit_log
// outcome CHECK constraint.
const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
)

// Audit target_type constants categorise the entity an action operated on.
// Keep these short, lowercase, and stable — they are written into the database
// and consumed by future incident-response tooling.
const (
	AuditTargetUser    = "user"
	AuditTargetAPIKey  = "api_key"
	AuditTargetSession = "session"
)

// AuditEvent represents a single row in the audit_log table.
//
// Identity:
//   - ActorID is nil for unauthenticated events (e.g. a failed login that
//     never resolved a users row). When set, it points at users.id.
//   - ActorLabel is a denormalised email/username so the row stays readable
//     after the actor is deleted or renamed.
//
// Provenance:
//   - IP / UserAgent / RequestID are populated from the HTTP request the event
//     originated from. They are best-effort: empty strings are acceptable when
//     the event is recorded outside a request context.
//
// Metadata is optional JSON for action-specific details (e.g. {"old_role":
// "viewer", "new_role": "editor"} on role changes). Callers serialise their
// own JSON; the store treats it as opaque bytes.
type AuditEvent struct {
	ID         int64
	OccurredAt time.Time
	ActorID    *int64
	ActorLabel string
	TargetType string
	TargetID   string
	Action     string
	Outcome    string
	IP         string
	UserAgent  string
	RequestID  string
	Metadata   json.RawMessage
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

// MCP proposal audit action constants.
const (
	AuditActionMCPProposeDefectClassify = "mcp.propose_defect_classify"
	AuditActionMCPProposeKnownIssue     = "mcp.propose_known_issue"
	AuditActionMCPProposeFlaky          = "mcp.propose_flaky"
	AuditActionMCPProposalApprove       = "mcp.proposal_approve"
	AuditActionMCPProposalReject        = "mcp.proposal_reject"
)

// DefectProposal represents an MCP-proposed reclassification of a defect fingerprint.
// ProposedResolution and Rationale are nullable in the DB; empty string when null.
// ProposerAPIKeyID is nullable; 0 when null.
// ReviewedByUserID is nullable; 0 when null.
type DefectProposal struct {
	ID                 int64
	ProjectID          int
	FingerprintHash    string
	ProposedCategory   string
	ProposedResolution string // nullable in DB; "" when null
	Rationale          string // nullable in DB; "" when null
	ProposerUserID     int64
	ProposerAPIKeyID   int64 // nullable in DB; 0 when null
	Status             ProposalStatus
	ReviewedByUserID   int64 // nullable in DB; 0 when null
	ReviewedAt         *time.Time
	CreatedAt          time.Time
}

// KnownIssueProposal represents an MCP-proposed new known-issue rule.
// ErrorMessageSample replaces FingerprintHash (regex matches by message, not hash).
// AppliesToStatus is stored as TEXT[] in the DB.
type KnownIssueProposal struct {
	ID                 int64
	ProjectID          int
	ErrorMessageSample string // nullable in DB; "" when null
	ProposedCategory   string
	ProposedResolution string // nullable in DB; "" when null
	Rationale          string // nullable in DB; "" when null
	RegexPattern       string
	AppliesToStatus    []string
	DryRunMatchCount   int
	ProposerUserID     int64
	ProposerAPIKeyID   int64 // nullable in DB; 0 when null
	Status             ProposalStatus
	ReviewedByUserID   int64 // nullable in DB; 0 when null
	ReviewedAt         *time.Time
	CreatedAt          time.Time
}

// FlakyProposal represents an MCP-proposed flaky-test flag.
// Keyed by (test_full_name, history_id) — no fingerprint or category fields.
type FlakyProposal struct {
	ID               int64
	ProjectID        int
	TestFullName     string
	HistoryID        string
	Rationale        string // nullable in DB; "" when null
	ProposerUserID   int64
	ProposerAPIKeyID int64 // nullable in DB; 0 when null
	Status           ProposalStatus
	ReviewedByUserID int64 // nullable in DB; 0 when null
	ReviewedAt       *time.Time
	CreatedAt        time.Time
}
