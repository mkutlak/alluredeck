package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// dryRunMatchCap is the maximum number of matching messages counted during
// the propose_known_issue dry-run preview.
const dryRunMatchCap = 1000

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------

// ProposeClassifyDefectInput holds parameters for the propose_classify_defect tool.
type ProposeClassifyDefectInput struct {
	ProjectID          int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	FingerprintHash    string `json:"fingerprint_hash" jsonschema:"Defect fingerprint hash. Obtain it from list_defects or diagnose_failure output; never construct one."`
	ProposedCategory   string `json:"proposed_category" jsonschema:"New defect category: product_bug, test_bug, infrastructure, or to_investigate."`
	ProposedResolution string `json:"proposed_resolution,omitempty" jsonschema:"New defect resolution: open, fixed, muted, or wont_fix. Omit to leave the resolution unchanged."`
	Rationale          string `json:"rationale,omitempty" jsonschema:"Optional free-text explanation for the reclassification, shown to the human reviewer."`
}

// ProposeClassifyDefectOutput is the structured output for propose_classify_defect.
type ProposeClassifyDefectOutput struct {
	ProposalID int64  `json:"proposal_id"`
	ReviewURL  string `json:"review_url"`
	// DuplicateOf is set when an identical pending proposal already existed;
	// ProposalID/ReviewURL above then echo that existing proposal and nothing
	// new was written. Absent on a genuine insert.
	DuplicateOf *DuplicateProposal `json:"duplicate_of,omitempty"`
}

// ProposeKnownIssueInput holds parameters for the propose_known_issue tool.
type ProposeKnownIssueInput struct {
	ProjectID          int      `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	ErrorMessageSample string   `json:"error_message_sample" jsonschema:"Sample error message this rule is meant to match; shown to the human reviewer alongside the pattern."`
	ProposedCategory   string   `json:"proposed_category" jsonschema:"Known-issue category (e.g. infrastructure). Free text, distinct from a defect category."`
	RegexPattern       string   `json:"regex_pattern" jsonschema:"Go-syntax regular expression matched against failure messages. Must compile."`
	AppliesToStatus    []string `json:"applies_to_status,omitempty" jsonschema:"Optional subset of test statuses this rule applies to (e.g. failed, broken). Omit to apply to all statuses."`
	Rationale          string   `json:"rationale,omitempty" jsonschema:"Optional free-text explanation for the rule, shown to the human reviewer."`
}

// ProposeKnownIssueOutput is the structured output for propose_known_issue.
type ProposeKnownIssueOutput struct {
	ProposalID       int64  `json:"proposal_id"`
	ReviewURL        string `json:"review_url"`
	DryRunMatchCount int    `json:"dry_run_match_count"`
	// DuplicateOf is set when an identical pending proposal already existed;
	// ProposalID/ReviewURL above then echo that existing proposal and nothing
	// new was written. Absent on a genuine insert.
	DuplicateOf *DuplicateProposal `json:"duplicate_of,omitempty"`
}

// ProposeMarkFlakyInput holds parameters for the propose_mark_flaky tool.
type ProposeMarkFlakyInput struct {
	ProjectID    int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	TestFullName string `json:"test_full_name" jsonschema:"Full name of the test to mark flaky, as shown in AllureDeck."`
	HistoryID    string `json:"history_id" jsonschema:"history_id of the test. Obtain it from find_test_by_name or list_failing_tests; do not construct one."`
	Rationale    string `json:"rationale,omitempty" jsonschema:"Optional free-text explanation for the flaky flag, shown to the human reviewer."`
}

// ProposeMarkFlakyOutput is the structured output for propose_mark_flaky.
type ProposeMarkFlakyOutput struct {
	ProposalID int64  `json:"proposal_id"`
	ReviewURL  string `json:"review_url"`
	// DuplicateOf is set when an identical pending proposal already existed;
	// ProposalID/ReviewURL above then echo that existing proposal and nothing
	// new was written. Absent on a genuine insert.
	DuplicateOf *DuplicateProposal `json:"duplicate_of,omitempty"`
}

// DuplicateProposal describes an existing pending proposal that already
// matches the identity of a newly requested one. Its presence on a propose_*
// output means the call performed no write: the identical proposal is
// already awaiting human review.
type DuplicateProposal struct {
	ProposalID int64     `json:"proposal_id"`
	ReviewURL  string    `json:"review_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// RegisterMutatingToolsWithURL registers the three MCP proposal tools on s.
//
// publicURL is the base URL used to build review links (e.g.
// "https://app.example.com"); pass an empty string to fall back to relative
// review paths. signingKey authenticates the multi-round-trip confirmation
// RequestState; pass nil to disable confirmation and write directly.
// ---------------------------------------------------------------------------

// Tool identifiers used to bind a confirmation to the tool that requested it.
const (
	kindClassifyDefect = "propose_classify_defect"
	kindKnownIssue     = "propose_known_issue"
	kindMarkFlaky      = "propose_mark_flaky"
)

// RegisterMutatingToolsWithURL registers the three MCP mutating tools and the
// list_proposals read tool on s.
func RegisterMutatingToolsWithURL(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindClassifyDefect,
		Title:       "Propose AllureDeck defect classification",
		Annotations: proposalAnnotations(),
		Description: "Propose a defect reclassification for a failing test fingerprint. Requires editor role and an API key with allow_mcp_writes=true. Creates a pending proposal that a human reviewer must approve before it takes effect. Safe to call repeatedly: if an identical proposal is already pending for the same project/fingerprint/category, no new proposal is created and the existing one is returned in duplicate_of. Call list_proposals first to check what is already pending.",
	}, proposeClassifyDefectHandler(stores, logger, publicURL, signingKey))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindKnownIssue,
		Title:       "Propose AllureDeck known-issue rule",
		Annotations: proposalAnnotations(),
		Description: "Propose a new known-issue regex rule for a project. Requires editor role and an API key with allow_mcp_writes=true. Performs a dry-run match count against recent failures before inserting the proposal. Safe to call repeatedly: if an identical rule is already pending for the same project/pattern, no new proposal is created and the existing one is returned in duplicate_of. Call list_proposals first to check what is already pending.",
	}, proposeKnownIssueHandler(stores, logger, publicURL, signingKey))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindMarkFlaky,
		Title:       "Propose AllureDeck flaky-test marking",
		Annotations: proposalAnnotations(),
		Description: "Propose marking a specific test as flaky by (test_full_name, history_id). Requires editor role and an API key with allow_mcp_writes=true. Creates a pending proposal for human review. Safe to call repeatedly: if an identical proposal is already pending for the same project/history_id, no new proposal is created and the existing one is returned in duplicate_of. Call list_proposals first to check what is already pending.",
	}, proposeMarkFlakyHandler(stores, logger, publicURL, signingKey))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "list_proposals",
		Title:       "List AllureDeck MCP proposals",
		Annotations: readOnlyAnnotations(),
		Description: "List MCP-proposed defect classifications, known-issue rules, and flaky-test markings for a project, optionally filtered by kind and review status. Call this before propose_classify_defect, propose_known_issue, or propose_mark_flaky to check whether an equivalent proposal is already pending, rather than re-proposing the same thing on every run.",
	}, listProposalsHandler(stores, logger, publicURL))
}

// ---------------------------------------------------------------------------
// RBAC helpers
// ---------------------------------------------------------------------------

// proposer identifies who is creating a proposal.
type proposer struct {
	// Label is the human-readable actor recorded in the audit log: an email
	// address for API-key callers, the JWT sub otherwise.
	Label string
	// UserID is the users.id written to proposals.proposer_user_id. That column
	// is NOT NULL with a foreign key to users(id), so a zero value cannot be
	// persisted — requireProposer rejects it up front.
	UserID int64
	// APIKeyID is the api_keys.id when the caller authenticated with a key,
	// and 0 for JWT callers.
	APIKeyID int64
}

// checkMutatingAuthFromInfo is the testable inner helper that takes an explicit
// *mcpauth.TokenInfo. Tests call this directly to bypass the context round-trip.
//
// It enforces the role and allow_mcp_writes gate, then resolves the proposer.
// It deliberately does not reject a proposer with no users row: that is a
// distinct failure with its own message, raised by requireProposer at the point
// of writing.
func checkMutatingAuthFromInfo(info *mcpauth.TokenInfo) (proposer, error) {
	role, _ := info.Extra["role"].(string)
	allowWritesRaw := info.Extra["allow_mcp_writes"]

	// Normalize allow_mcp_writes — stored as string "true"/"false" for both
	// API-key and JWT paths (see mcp/auth.go).
	allowWrites := false
	switch v := allowWritesRaw.(type) {
	case string:
		allowWrites = v == "true"
	case bool:
		allowWrites = v
	}

	if !isMutatingRole(role) || !allowWrites {
		return proposer{}, fmt.Errorf("forbidden: requires editor role and api key with allow_mcp_writes=true")
	}

	p := proposer{}

	p.Label, _ = info.Extra["user_id"].(string)
	if p.Label == "" {
		p.Label = info.UserID
	}

	// user_db_id is the numeric users.id resolved during authentication. It is
	// absent or zero for an env-config user, which has no users row.
	p.UserID = extraInt64(info.Extra["user_db_id"])
	p.APIKeyID = extraInt64(info.Extra["api_key_id"])

	return p, nil
}

// extraInt64 reads a numeric TokenInfo.Extra value. Values set in-process are
// int64; a value that has round-tripped through JSON arrives as float64.
func extraInt64(raw any) int64 {
	switch v := raw.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}

// requireProposer rejects a caller that cannot be attributed to a users row.
//
// proposals.proposer_user_id is NOT NULL with a foreign key to users(id), so
// writing without a real user produces an opaque 23503 from PostgreSQL. This
// turns that into a message that says what is wrong and how to fix it.
//
// It affects API keys owned by an env-config user (ADMIN_USERNAME and friends),
// which exist only in configuration and have no users row to reference.
func requireProposer(p proposer) error {
	if p.UserID == 0 {
		return fmt.Errorf(
			"cannot record a proposal for %q: this API key is owned by a configuration-file user, "+
				"which has no account in the database to attribute the proposal to. "+
				"Use a key belonging to a registered user account", p.Label)
	}
	return nil
}

// isMutatingRole returns true for roles that are allowed to create proposals.
func isMutatingRole(role string) bool {
	return role == "editor" || role == "admin"
}

// reviewURL builds the human-review URL for a proposal.
func reviewURL(publicURL, proposalType string, id int64) string {
	if publicURL == "" {
		return fmt.Sprintf("/admin/proposals/%s/%d", proposalType, id)
	}
	return fmt.Sprintf("%s/admin/proposals/%s/%d", publicURL, proposalType, id)
}

// auditMetadata serialises a small key-value map to JSON for the audit log.
func auditMetadata(kv map[string]any) json.RawMessage {
	b, _ := json.Marshal(kv)
	return b
}

// orNotSet renders an omitted optional field for a confirmation prompt, so the
// user sees that a field was left blank rather than an empty gap.
func orNotSet(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

// formatMatchCount renders the dry-run tally, marking the point where counting
// stopped so "1000" is not mistaken for an exact total.
func formatMatchCount(n int) string {
	if n >= dryRunMatchCap {
		return fmt.Sprintf("%d or more", dryRunMatchCap)
	}
	return strconv.Itoa(n)
}

// breadthWarning flags a regex that matches a large share of the sample. Such a
// pattern usually means the author meant to match one failure mode and instead
// wrote something that swallows most of the project's failures.
func breadthWarning(matched, sampled int) string {
	if sampled == 0 || matched*2 <= sampled {
		return ""
	}
	return " This matches over half the sample — check the pattern is not broader than intended."
}

// ---------------------------------------------------------------------------
// propose_classify_defect handler
// ---------------------------------------------------------------------------

func proposeClassifyDefectHandler(stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeClassifyDefectInput) (*mcpsdk.CallToolResult, ProposeClassifyDefectOutput, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeClassifyDefectInput) (*mcpsdk.CallToolResult, ProposeClassifyDefectOutput, error) {
		info := mcpauth.TokenInfoFromContext(ctx)
		return execProposeClassifyDefect(ctx, req, in, info, stores, logger, publicURL, signingKey)
	}
}

// ExecProposeClassifyDefectForTest is exported for white-box unit tests that
// cannot inject TokenInfo via HTTP context (InMemoryTransport bypasses auth
// middleware). Production code uses proposeClassifyDefectHandler which reads
// TokenInfo from ctx.
var ExecProposeClassifyDefectForTest = execProposeClassifyDefect

// execProposeClassifyDefect is the testable core for propose_classify_defect.
func execProposeClassifyDefect(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
	in ProposeClassifyDefectInput,
	info *mcpauth.TokenInfo,
	stores *bootstrap.Stores,
	logger *zap.Logger,
	publicURL string,
	signingKey []byte,
) (*mcpsdk.CallToolResult, ProposeClassifyDefectOutput, error) {
	prop, err := checkMutatingAuthFromInfo(info)
	if err != nil {
		return nil, ProposeClassifyDefectOutput{}, err
	}

	if in.ProjectID <= 0 {
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("project_id must be positive")
	}
	if in.FingerprintHash == "" {
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("fingerprint_hash is required")
	}
	if in.ProposedCategory == "" {
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("proposed_category is required")
	}

	if err := requireProposer(prop); err != nil {
		return nil, ProposeClassifyDefectOutput{}, err
	}

	// Idempotency: an identical pending proposal already awaiting review means
	// this call is a re-run (e.g. a nightly agent), not a new finding. Return
	// it instead of queuing a duplicate for the same human reviewer.
	if dup, err := stores.DefectProposals.FindPendingDuplicate(ctx, in.ProjectID, in.FingerprintHash, in.ProposedCategory); err != nil {
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("checking for duplicate defect-classification proposal: %w", err)
	} else if dup != nil {
		dupURL := reviewURL(publicURL, "defect", dup.ID)
		out := ProposeClassifyDefectOutput{
			ProposalID: dup.ID,
			ReviewURL:  dupURL,
			DuplicateOf: &DuplicateProposal{
				ProposalID: dup.ID,
				ReviewURL:  dupURL,
				CreatedAt:  dup.CreatedAt,
			},
		}
		digest := fmt.Sprintf("An identical pending defect-classification proposal already exists (id=%d, created %s) — nothing new was written.", dup.ID, dup.CreatedAt.Format(time.RFC3339))
		return textResult(digest), out, nil
	}

	prompt := fmt.Sprintf(
		"Reclassify this defect in AllureDeck project %d?\n\n  Fingerprint: %s\n  New category: %s\n  New resolution: %s\n\nThis records a proposal for a human reviewer to approve; the defect's current classification is unchanged until then.",
		in.ProjectID, in.FingerprintHash, in.ProposedCategory, orNotSet(in.ProposedResolution),
	)
	gate, ask, err := evaluateWriteGate(req, signingKey, kindClassifyDefect, prop.Label, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeClassifyDefectOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_classify_defect", zap.String("user", prop.Label))
		return ask, ProposeClassifyDefectOutput{}, nil
	case gateDeclined:
		return declinedResult("defect-classification proposal"), ProposeClassifyDefectOutput{}, nil
	case gateWrite:
	}

	p := &store.DefectProposal{
		ProjectID:          in.ProjectID,
		FingerprintHash:    in.FingerprintHash,
		ProposedCategory:   in.ProposedCategory,
		ProposedResolution: in.ProposedResolution,
		Rationale:          in.Rationale,
		ProposerUserID:     prop.UserID,
		ProposerAPIKeyID:   prop.APIKeyID,
		Status:             store.ProposalStatusPending,
	}

	proposalID, err := stores.DefectProposals.Create(ctx, p)
	if err != nil {
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("creating defect proposal: %w", err)
	}

	// Audit log — best-effort: if it fails we log and continue since the
	// proposal is already persisted. No transactional rollback is available
	// without exposing a raw pgx pool here.
	auditErr := stores.Audit.Record(ctx, store.AuditEvent{
		ActorLabel: prop.Label,
		TargetType: "proposal",
		TargetID:   strconv.FormatInt(proposalID, 10),
		Action:     store.AuditActionMCPProposeDefectClassify,
		Outcome:    store.AuditOutcomeSuccess,
		Metadata: auditMetadata(map[string]any{
			"project_id":       in.ProjectID,
			"fingerprint_hash": in.FingerprintHash,
			"category":         in.ProposedCategory,
		}),
	})
	if auditErr != nil {
		logger.Error("mcp: failed to record audit event for propose_classify_defect",
			zap.Int64("proposal_id", proposalID),
			zap.Error(auditErr),
		)
		return nil, ProposeClassifyDefectOutput{}, fmt.Errorf("recording audit event: %w", auditErr)
	}

	out := ProposeClassifyDefectOutput{
		ProposalID: proposalID,
		ReviewURL:  reviewURL(publicURL, "defect", proposalID),
	}
	return nil, out, nil
}

// ---------------------------------------------------------------------------
// propose_known_issue handler
// ---------------------------------------------------------------------------

func proposeKnownIssueHandler(stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeKnownIssueInput) (*mcpsdk.CallToolResult, ProposeKnownIssueOutput, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeKnownIssueInput) (*mcpsdk.CallToolResult, ProposeKnownIssueOutput, error) {
		info := mcpauth.TokenInfoFromContext(ctx)
		return execProposeKnownIssue(ctx, req, in, info, stores, logger, publicURL, signingKey)
	}
}

// ExecProposeKnownIssueForTest is exported for white-box unit tests.
var ExecProposeKnownIssueForTest = execProposeKnownIssue

// execProposeKnownIssue is the testable core for propose_known_issue.
func execProposeKnownIssue(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
	in ProposeKnownIssueInput,
	info *mcpauth.TokenInfo,
	stores *bootstrap.Stores,
	logger *zap.Logger,
	publicURL string,
	signingKey []byte,
) (*mcpsdk.CallToolResult, ProposeKnownIssueOutput, error) {
	prop, err := checkMutatingAuthFromInfo(info)
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, err
	}

	if in.ProjectID <= 0 {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("project_id must be positive")
	}
	if in.RegexPattern == "" {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("regex_pattern is required")
	}

	re, err := regexp.Compile(in.RegexPattern)
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("regex_pattern does not compile: %w", err)
	}

	// Checked before the dry-run scan: there is no point scanning the project's
	// recent failures on behalf of a caller whose proposal could never be
	// recorded.
	if err := requireProposer(prop); err != nil {
		return nil, ProposeKnownIssueOutput{}, err
	}

	// Idempotency: an identical pending rule already awaiting review means this
	// call is a re-run, not a new finding. Checked before the dry-run scan so a
	// duplicate call skips that cost too.
	if dup, err := stores.KnownIssueProposals.FindPendingDuplicate(ctx, in.ProjectID, in.RegexPattern); err != nil {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("checking for duplicate known-issue proposal: %w", err)
	} else if dup != nil {
		dupURL := reviewURL(publicURL, "known_issue", dup.ID)
		out := ProposeKnownIssueOutput{
			ProposalID:       dup.ID,
			ReviewURL:        dupURL,
			DryRunMatchCount: dup.DryRunMatchCount,
			DuplicateOf: &DuplicateProposal{
				ProposalID: dup.ID,
				ReviewURL:  dupURL,
				CreatedAt:  dup.CreatedAt,
			},
		}
		digest := fmt.Sprintf("An identical pending known-issue proposal already exists (id=%d, created %s) — nothing new was written.", dup.ID, dup.CreatedAt.Format(time.RFC3339))
		return textResult(digest), out, nil
	}

	// Dry-run: count recent failure messages that match the regex (capped at dryRunMatchCap).
	messages, err := stores.TestResult.ListRecentMessages(ctx, int64(in.ProjectID), dryRunMatchCap+1)
	if err != nil {
		logger.Warn("mcp: dry-run message scan failed (proceeding without count)",
			zap.Int("project_id", in.ProjectID),
			zap.Error(err),
		)
		// Non-fatal: proceed with count=0.
		messages = nil
	}

	matchCount := 0
	for _, msg := range messages {
		if re.MatchString(msg) {
			matchCount++
			if matchCount >= dryRunMatchCap {
				break
			}
		}
	}

	// The dry-run count is the whole reason this confirmation exists: a regex
	// that matches most recent failures would silently reclassify them, and the
	// count is the only signal that distinguishes a targeted rule from an
	// over-broad one. It is therefore computed before the prompt, not after.
	prompt := fmt.Sprintf(
		"Create this known-issue rule in AllureDeck project %d?\n\n  Pattern: %s\n  Category: %s\n\nDry run: matches %s of the last %d failure messages.%s\n\nThis records a proposal for a human reviewer to approve; no failures are reclassified until then.",
		in.ProjectID, in.RegexPattern, orNotSet(in.ProposedCategory),
		formatMatchCount(matchCount), len(messages),
		breadthWarning(matchCount, len(messages)),
	)
	gate, ask, err := evaluateWriteGate(req, signingKey, kindKnownIssue, prop.Label, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_known_issue",
			zap.String("user", prop.Label), zap.Int("dry_run_match_count", matchCount))
		return ask, ProposeKnownIssueOutput{}, nil
	case gateDeclined:
		return declinedResult("known-issue proposal"), ProposeKnownIssueOutput{}, nil
	case gateWrite:
	}

	p := &store.KnownIssueProposal{
		ProjectID:          in.ProjectID,
		ErrorMessageSample: in.ErrorMessageSample,
		ProposedCategory:   in.ProposedCategory,
		RegexPattern:       in.RegexPattern,
		AppliesToStatus:    in.AppliesToStatus,
		Rationale:          in.Rationale,
		DryRunMatchCount:   matchCount,
		ProposerUserID:     prop.UserID,
		ProposerAPIKeyID:   prop.APIKeyID,
		Status:             store.ProposalStatusPending,
	}

	proposalID, err := stores.KnownIssueProposals.Create(ctx, p)
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("creating known-issue proposal: %w", err)
	}

	auditErr := stores.Audit.Record(ctx, store.AuditEvent{
		ActorLabel: prop.Label,
		TargetType: "proposal",
		TargetID:   strconv.FormatInt(proposalID, 10),
		Action:     store.AuditActionMCPProposeKnownIssue,
		Outcome:    store.AuditOutcomeSuccess,
		Metadata: auditMetadata(map[string]any{
			"project_id":    in.ProjectID,
			"regex_pattern": in.RegexPattern,
			"dry_run_count": matchCount,
		}),
	})
	if auditErr != nil {
		logger.Error("mcp: failed to record audit event for propose_known_issue",
			zap.Int64("proposal_id", proposalID),
			zap.Error(auditErr),
		)
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("recording audit event: %w", auditErr)
	}

	out := ProposeKnownIssueOutput{
		ProposalID:       proposalID,
		ReviewURL:        reviewURL(publicURL, "known_issue", proposalID),
		DryRunMatchCount: matchCount,
	}
	return nil, out, nil
}

// ---------------------------------------------------------------------------
// propose_mark_flaky handler
// ---------------------------------------------------------------------------

func proposeMarkFlakyHandler(stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeMarkFlakyInput) (*mcpsdk.CallToolResult, ProposeMarkFlakyOutput, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest, in ProposeMarkFlakyInput) (*mcpsdk.CallToolResult, ProposeMarkFlakyOutput, error) {
		info := mcpauth.TokenInfoFromContext(ctx)
		return execProposeMarkFlaky(ctx, req, in, info, stores, logger, publicURL, signingKey)
	}
}

// ExecProposeMarkFlakyForTest is exported for white-box unit tests.
var ExecProposeMarkFlakyForTest = execProposeMarkFlaky

// execProposeMarkFlaky is the testable core for propose_mark_flaky.
func execProposeMarkFlaky(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
	in ProposeMarkFlakyInput,
	info *mcpauth.TokenInfo,
	stores *bootstrap.Stores,
	logger *zap.Logger,
	publicURL string,
	signingKey []byte,
) (*mcpsdk.CallToolResult, ProposeMarkFlakyOutput, error) {
	prop, err := checkMutatingAuthFromInfo(info)
	if err != nil {
		return nil, ProposeMarkFlakyOutput{}, err
	}

	if in.ProjectID <= 0 {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("project_id must be positive")
	}
	if in.TestFullName == "" {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("test_full_name is required")
	}
	if in.HistoryID == "" {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("history_id is required and must be non-empty")
	}

	if err := requireProposer(prop); err != nil {
		return nil, ProposeMarkFlakyOutput{}, err
	}

	// Idempotency: an identical pending proposal already awaiting review means
	// this call is a re-run, not a new finding. Return it instead of queuing a
	// duplicate for the same human reviewer.
	if dup, err := stores.FlakyProposals.FindPendingDuplicate(ctx, in.ProjectID, in.HistoryID); err != nil {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("checking for duplicate flaky proposal: %w", err)
	} else if dup != nil {
		dupURL := reviewURL(publicURL, "flaky", dup.ID)
		out := ProposeMarkFlakyOutput{
			ProposalID: dup.ID,
			ReviewURL:  dupURL,
			DuplicateOf: &DuplicateProposal{
				ProposalID: dup.ID,
				ReviewURL:  dupURL,
				CreatedAt:  dup.CreatedAt,
			},
		}
		digest := fmt.Sprintf("An identical pending flaky-test proposal already exists (id=%d, created %s) — nothing new was written.", dup.ID, dup.CreatedAt.Format(time.RFC3339))
		return textResult(digest), out, nil
	}

	prompt := fmt.Sprintf(
		"Mark this test as flaky in AllureDeck project %d?\n\n  Test: %s\n  history_id: %s\n\nThis records a proposal for a human reviewer to approve; it does not change triage on its own.",
		in.ProjectID, in.TestFullName, in.HistoryID,
	)
	gate, ask, err := evaluateWriteGate(req, signingKey, kindMarkFlaky, prop.Label, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeMarkFlakyOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_mark_flaky", zap.String("user", prop.Label))
		return ask, ProposeMarkFlakyOutput{}, nil
	case gateDeclined:
		return declinedResult("flaky-test proposal"), ProposeMarkFlakyOutput{}, nil
	case gateWrite:
	}

	p := &store.FlakyProposal{
		ProjectID:        in.ProjectID,
		TestFullName:     in.TestFullName,
		HistoryID:        in.HistoryID,
		Rationale:        in.Rationale,
		ProposerUserID:   prop.UserID,
		ProposerAPIKeyID: prop.APIKeyID,
		Status:           store.ProposalStatusPending,
	}

	proposalID, err := stores.FlakyProposals.Create(ctx, p)
	if err != nil {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("creating flaky proposal: %w", err)
	}

	auditErr := stores.Audit.Record(ctx, store.AuditEvent{
		ActorLabel: prop.Label,
		TargetType: "proposal",
		TargetID:   strconv.FormatInt(proposalID, 10),
		Action:     store.AuditActionMCPProposeFlaky,
		Outcome:    store.AuditOutcomeSuccess,
		Metadata: auditMetadata(map[string]any{
			"project_id":     in.ProjectID,
			"test_full_name": in.TestFullName,
			"history_id":     in.HistoryID,
		}),
	})
	if auditErr != nil {
		logger.Error("mcp: failed to record audit event for propose_mark_flaky",
			zap.Int64("proposal_id", proposalID),
			zap.Error(auditErr),
		)
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("recording audit event: %w", auditErr)
	}

	out := ProposeMarkFlakyOutput{
		ProposalID: proposalID,
		ReviewURL:  reviewURL(publicURL, "flaky", proposalID),
	}
	return nil, out, nil
}

// ---------------------------------------------------------------------------
// list_proposals
// ---------------------------------------------------------------------------

// Proposal kind identifiers used in ListProposalsInput.Kind, ProposalSummary.Kind,
// and reviewURL's proposalType argument (except kindClassifyDefect's review
// path, which is "defect" for historical reasons — see reviewURL call sites
// above).
const (
	proposalKindKnownIssue     = "known_issue"
	proposalKindFlaky          = "flaky"
	proposalKindDefectClassify = "defect_classify"
)

// Default and maximum row counts for list_proposals.
const (
	listProposalsDefaultLimit = 50
	listProposalsMaxLimit     = 200
)

// ListProposalsInput holds parameters for the list_proposals tool.
type ListProposalsInput struct {
	ProjectID int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	Kind      string `json:"kind,omitempty" jsonschema:"Optional proposal kind filter: known_issue, flaky, or defect_classify. Omit to list every kind."`
	Status    string `json:"status,omitempty" jsonschema:"Optional review status filter: pending, approved, or rejected. Omit to list every status."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum proposals to return, most recent first. Defaults to 50, clamped to 200."`
}

// ProposalSummary is one row returned by list_proposals. Only the fields
// relevant to Kind are populated; the others are zero.
type ProposalSummary struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	// Proposer is the proposing user's display name when it can be resolved,
	// and falls back to "user:<id>" otherwise (unknown, deleted, or unnamed
	// user). It is deliberately never an email address: the REST proposal
	// endpoints expose only the numeric proposer id, so this tool must not be
	// the surface that leaks a staff directory.
	Proposer  string `json:"proposer"`
	ReviewURL string `json:"review_url"`

	// Pattern is populated for kind=known_issue: the proposed regex_pattern.
	Pattern string `json:"pattern,omitempty"`

	// TestFullName and HistoryID are populated for kind=flaky.
	TestFullName string `json:"test_full_name,omitempty"`
	HistoryID    string `json:"history_id,omitempty"`

	// FingerprintHash is populated for kind=defect_classify.
	FingerprintHash string `json:"fingerprint_hash,omitempty"`
	// ProposedCategory is populated for kind=known_issue (issue category) and
	// kind=defect_classify (defect category); the two are different concepts
	// sharing a field name, matching store.KnownIssueProposal/DefectProposal.
	ProposedCategory string `json:"proposed_category,omitempty"`
}

// ListProposalsOutput is the structured output for list_proposals.
type ListProposalsOutput struct {
	Items []ProposalSummary `json:"items"`
}

func listProposalsHandler(stores *bootstrap.Stores, _ *zap.Logger, publicURL string) func(ctx context.Context, req *mcpsdk.CallToolRequest, in ListProposalsInput) (*mcpsdk.CallToolResult, ListProposalsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in ListProposalsInput) (*mcpsdk.CallToolResult, ListProposalsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, ListProposalsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.Kind != "" && in.Kind != proposalKindKnownIssue && in.Kind != proposalKindFlaky && in.Kind != proposalKindDefectClassify {
			return nil, ListProposalsOutput{}, fmt.Errorf("kind must be one of %s, %s, %s", proposalKindKnownIssue, proposalKindFlaky, proposalKindDefectClassify)
		}

		var status store.ProposalStatus
		switch in.Status {
		case "":
			// no filter
		case string(store.ProposalStatusPending), string(store.ProposalStatusApproved), string(store.ProposalStatusRejected):
			status = store.ProposalStatus(in.Status)
		default:
			return nil, ListProposalsOutput{}, fmt.Errorf("status must be one of %s, %s, %s",
				store.ProposalStatusPending, store.ProposalStatusApproved, store.ProposalStatusRejected)
		}

		limit := in.Limit
		if limit <= 0 {
			limit = listProposalsDefaultLimit
		}
		if limit > listProposalsMaxLimit {
			limit = listProposalsMaxLimit
		}

		var items []ProposalSummary
		proposers := newProposerResolver(stores)

		if in.Kind == "" || in.Kind == proposalKindKnownIssue {
			rows, err := stores.KnownIssueProposals.List(ctx, in.ProjectID, status, limit)
			if err != nil {
				return nil, ListProposalsOutput{}, fmt.Errorf("listing known-issue proposals: %w", err)
			}
			for _, p := range rows {
				items = append(items, ProposalSummary{
					ID:               p.ID,
					Kind:             proposalKindKnownIssue,
					Status:           string(p.Status),
					CreatedAt:        p.CreatedAt,
					Proposer:         proposers.label(ctx, p.ProposerUserID),
					ReviewURL:        reviewURL(publicURL, "known_issue", p.ID),
					Pattern:          p.RegexPattern,
					ProposedCategory: p.ProposedCategory,
				})
			}
		}

		if in.Kind == "" || in.Kind == proposalKindFlaky {
			rows, err := stores.FlakyProposals.List(ctx, in.ProjectID, status, limit)
			if err != nil {
				return nil, ListProposalsOutput{}, fmt.Errorf("listing flaky proposals: %w", err)
			}
			for _, p := range rows {
				items = append(items, ProposalSummary{
					ID:           p.ID,
					Kind:         proposalKindFlaky,
					Status:       string(p.Status),
					CreatedAt:    p.CreatedAt,
					Proposer:     proposers.label(ctx, p.ProposerUserID),
					ReviewURL:    reviewURL(publicURL, "flaky", p.ID),
					TestFullName: p.TestFullName,
					HistoryID:    p.HistoryID,
				})
			}
		}

		if in.Kind == "" || in.Kind == proposalKindDefectClassify {
			rows, err := stores.DefectProposals.List(ctx, in.ProjectID, status, limit)
			if err != nil {
				return nil, ListProposalsOutput{}, fmt.Errorf("listing defect-classification proposals: %w", err)
			}
			for _, p := range rows {
				items = append(items, ProposalSummary{
					ID:               p.ID,
					Kind:             proposalKindDefectClassify,
					Status:           string(p.Status),
					CreatedAt:        p.CreatedAt,
					Proposer:         proposers.label(ctx, p.ProposerUserID),
					ReviewURL:        reviewURL(publicURL, "defect", p.ID),
					FingerprintHash:  p.FingerprintHash,
					ProposedCategory: p.ProposedCategory,
				})
			}
		}

		// Each per-kind query above already returns at most `limit` rows, most
		// recent first, so merging and re-trimming to `limit` here cannot drop
		// a row that belongs in the true overall top-`limit`.
		//
		// SliceStable, not Slice: created_at ties are common (proposals written
		// by one batch share a timestamp), and an unstable sort would order
		// them arbitrarily, so the `[:limit]` trim below could keep a different
		// subset on each identical call.
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		if len(items) > limit {
			items = items[:limit]
		}

		digest := fmt.Sprintf("%d proposal(s) for project %d", len(items), in.ProjectID)
		return textResult(digest), ListProposalsOutput{Items: items}, nil
	}
}

// proposerResolver turns a proposals.proposer_user_id into a display label,
// memoized for the lifetime of one list_proposals call.
//
// The cache matters because proposals cluster on a handful of proposers: a
// 200-row page from one automation account would otherwise issue 200 identical
// GetByID queries. It is deliberately per-call and not shared: a long-lived
// cache would keep serving a renamed or deleted user's old label.
//
// Not safe for concurrent use; listProposalsHandler resolves sequentially.
type proposerResolver struct {
	stores *bootstrap.Stores
	cache  map[int64]string
}

func newProposerResolver(stores *bootstrap.Stores) *proposerResolver {
	return &proposerResolver{stores: stores, cache: make(map[int64]string)}
}

// label returns the display label for userID, resolving it at most once.
func (r *proposerResolver) label(ctx context.Context, userID int64) string {
	if cached, ok := r.cache[userID]; ok {
		return cached
	}
	l := r.resolve(ctx, userID)
	r.cache[userID] = l
	return l
}

// resolve produces the label for one user id.
//
// It returns the user's display name, never their email address. The REST
// proposal endpoints expose only the numeric proposer_user_id (and gate the
// api-key id behind admin), so handing every MCP caller a directory of staff
// email addresses would make this tool the weakest link in that policy. The
// numeric fallback also covers a lookup error, a deleted user, and a user with
// no recorded name, so an absent label never fails the whole call.
func (r *proposerResolver) resolve(ctx context.Context, userID int64) string {
	fallback := fmt.Sprintf("user:%d", userID)
	if r.stores.User == nil || userID == 0 {
		return fallback
	}
	u, err := r.stores.User.GetByID(ctx, userID)
	if err != nil || u == nil {
		return fallback
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	return fallback
}
