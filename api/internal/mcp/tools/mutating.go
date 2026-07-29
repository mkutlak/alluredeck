package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
	ProjectID          int    `json:"project_id"`
	FingerprintHash    string `json:"fingerprint_hash"`
	ProposedCategory   string `json:"proposed_category"`
	ProposedResolution string `json:"proposed_resolution,omitempty"`
	Rationale          string `json:"rationale,omitempty"`
}

// ProposeClassifyDefectOutput is the structured output for propose_classify_defect.
type ProposeClassifyDefectOutput struct {
	ProposalID int64  `json:"proposal_id"`
	ReviewURL  string `json:"review_url"`
}

// ProposeKnownIssueInput holds parameters for the propose_known_issue tool.
type ProposeKnownIssueInput struct {
	ProjectID          int      `json:"project_id"`
	ErrorMessageSample string   `json:"error_message_sample"`
	ProposedCategory   string   `json:"proposed_category"`
	RegexPattern       string   `json:"regex_pattern"`
	AppliesToStatus    []string `json:"applies_to_status,omitempty"`
	Rationale          string   `json:"rationale,omitempty"`
}

// ProposeKnownIssueOutput is the structured output for propose_known_issue.
type ProposeKnownIssueOutput struct {
	ProposalID       int64  `json:"proposal_id"`
	ReviewURL        string `json:"review_url"`
	DryRunMatchCount int    `json:"dry_run_match_count"`
}

// ProposeMarkFlakyInput holds parameters for the propose_mark_flaky tool.
type ProposeMarkFlakyInput struct {
	ProjectID    int    `json:"project_id"`
	TestFullName string `json:"test_full_name"`
	HistoryID    string `json:"history_id"`
	Rationale    string `json:"rationale,omitempty"`
}

// ProposeMarkFlakyOutput is the structured output for propose_mark_flaky.
type ProposeMarkFlakyOutput struct {
	ProposalID int64  `json:"proposal_id"`
	ReviewURL  string `json:"review_url"`
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

// RegisterMutatingToolsWithURL registers the three MCP mutating tools on s.
func RegisterMutatingToolsWithURL(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindClassifyDefect,
		Title:       "Propose AllureDeck defect classification",
		Annotations: proposalAnnotations(),
		Description: "Propose a defect reclassification for a failing test fingerprint. Requires editor role and an API key with allow_mcp_writes=true. Creates a pending proposal that a human reviewer must approve before it takes effect.",
	}, proposeClassifyDefectHandler(stores, logger, publicURL, signingKey))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindKnownIssue,
		Title:       "Propose AllureDeck known-issue rule",
		Annotations: proposalAnnotations(),
		Description: "Propose a new known-issue regex rule for a project. Requires editor role and an API key with allow_mcp_writes=true. Performs a dry-run match count against recent failures before inserting the proposal.",
	}, proposeKnownIssueHandler(stores, logger, publicURL, signingKey))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        kindMarkFlaky,
		Title:       "Propose AllureDeck flaky-test marking",
		Annotations: proposalAnnotations(),
		Description: "Propose marking a specific test as flaky by (test_full_name, history_id). Requires editor role and an API key with allow_mcp_writes=true. Creates a pending proposal for human review.",
	}, proposeMarkFlakyHandler(stores, logger, publicURL, signingKey))
}

// ---------------------------------------------------------------------------
// RBAC helpers
// ---------------------------------------------------------------------------

// checkMutatingAuthFromInfo is the testable inner helper that takes an explicit
// *mcpauth.TokenInfo. Tests call this directly to bypass the context round-trip.
func checkMutatingAuthFromInfo(info *mcpauth.TokenInfo) (userID string, apiKeyID int64, err error) {
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
		return "", 0, fmt.Errorf("forbidden: requires editor role and api key with allow_mcp_writes=true")
	}

	userID, _ = info.Extra["user_id"].(string)
	if userID == "" {
		userID = info.UserID
	}

	if raw, ok := info.Extra["api_key_id"]; ok {
		switch v := raw.(type) {
		case int64:
			apiKeyID = v
		case float64:
			apiKeyID = int64(v)
		}
	}

	return userID, apiKeyID, nil
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

// userIDToInt64 converts a string user_id to int64 for the ProposerUserID field.
// Returns 0 if the string is not numeric (e.g. a username string).
func userIDToInt64(s string) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return 0
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
	userID, apiKeyID, err := checkMutatingAuthFromInfo(info)
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

	prompt := fmt.Sprintf(
		"Reclassify this defect in AllureDeck project %d?\n\n  Fingerprint: %s\n  New category: %s\n  New resolution: %s\n\nThis records a proposal for a human reviewer to approve; the defect's current classification is unchanged until then.",
		in.ProjectID, in.FingerprintHash, in.ProposedCategory, orNotSet(in.ProposedResolution),
	)
	gate, ask, err := evaluateWriteGate(req, signingKey, kindClassifyDefect, userID, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeClassifyDefectOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_classify_defect", zap.String("user", userID))
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
		ProposerUserID:     userIDToInt64(userID),
		ProposerAPIKeyID:   apiKeyID,
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
		ActorLabel: userID,
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
	userID, apiKeyID, err := checkMutatingAuthFromInfo(info)
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
	gate, ask, err := evaluateWriteGate(req, signingKey, kindKnownIssue, userID, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_known_issue",
			zap.String("user", userID), zap.Int("dry_run_match_count", matchCount))
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
		ProposerUserID:     userIDToInt64(userID),
		ProposerAPIKeyID:   apiKeyID,
		Status:             store.ProposalStatusPending,
	}

	proposalID, err := stores.KnownIssueProposals.Create(ctx, p)
	if err != nil {
		return nil, ProposeKnownIssueOutput{}, fmt.Errorf("creating known-issue proposal: %w", err)
	}

	auditErr := stores.Audit.Record(ctx, store.AuditEvent{
		ActorLabel: userID,
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
	userID, apiKeyID, err := checkMutatingAuthFromInfo(info)
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

	prompt := fmt.Sprintf(
		"Mark this test as flaky in AllureDeck project %d?\n\n  Test: %s\n  history_id: %s\n\nThis records a proposal for a human reviewer to approve; it does not change triage on its own.",
		in.ProjectID, in.TestFullName, in.HistoryID,
	)
	gate, ask, err := evaluateWriteGate(req, signingKey, kindMarkFlaky, userID, prompt, in, time.Now())
	if err != nil {
		return nil, ProposeMarkFlakyOutput{}, err
	}
	switch gate {
	case gateAsk:
		logger.Debug("mcp: awaiting user confirmation for propose_mark_flaky", zap.String("user", userID))
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
		ProposerUserID:   userIDToInt64(userID),
		ProposerAPIKeyID: apiKeyID,
		Status:           store.ProposalStatusPending,
	}

	proposalID, err := stores.FlakyProposals.Create(ctx, p)
	if err != nil {
		return nil, ProposeMarkFlakyOutput{}, fmt.Errorf("creating flaky proposal: %w", err)
	}

	auditErr := stores.Audit.Record(ctx, store.AuditEvent{
		ActorLabel: userID,
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
