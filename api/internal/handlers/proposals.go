package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ProposalsHandler serves the MCP proposal review endpoints used by the admin UI.
// It is only registered when cfg.MCPServerEnabled is true.
type ProposalsHandler struct {
	defectProposals     store.DefectProposalStorer
	knownIssueProposals store.KnownIssueProposalStorer
	flakyProposals      store.FlakyProposalStorer
	knownIssueStore     store.KnownIssueStorer
	testResultStore     store.TestResultStorer
	defectStore         store.DefectStorer
	pool                *pgxpool.Pool
	logger              *zap.Logger
}

// NewProposalsHandler constructs a ProposalsHandler.
func NewProposalsHandler(
	defectProposals store.DefectProposalStorer,
	knownIssueProposals store.KnownIssueProposalStorer,
	flakyProposals store.FlakyProposalStorer,
	knownIssueStore store.KnownIssueStorer,
	testResultStore store.TestResultStorer,
	defectStore store.DefectStorer,
	pool *pgxpool.Pool,
	logger *zap.Logger,
) *ProposalsHandler {
	return &ProposalsHandler{
		defectProposals:     defectProposals,
		knownIssueProposals: knownIssueProposals,
		flakyProposals:      flakyProposals,
		knownIssueStore:     knownIssueStore,
		testResultStore:     testResultStore,
		defectStore:         defectStore,
		pool:                pool,
		logger:              logger,
	}
}

// proposalType is the validated set of proposal resource names accepted in the URL.
type proposalType string

const (
	proposalTypeDefect     proposalType = "defect"
	proposalTypeKnownIssue proposalType = "known_issue"
	proposalTypeFlaky      proposalType = "flaky"
)

// parseProposalType validates and returns the {type} path parameter.
func parseProposalType(w http.ResponseWriter, r *http.Request) (proposalType, bool) {
	t := r.PathValue("type")
	switch proposalType(t) {
	case proposalTypeDefect, proposalTypeKnownIssue, proposalTypeFlaky:
		return proposalType(t), true
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: defect, known_issue, flaky")
		return "", false
	}
}

// actorFromContext extracts the numeric user ID and label from the JWT claims.
// Returns (0, "") when claims are absent (security disabled) — callers treat that
// as a valid no-op actor for audit purposes.
func actorFromContext(r *http.Request) (actorID *int64, actorLabel string) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return nil, ""
	}
	sub, _ := claims["sub"].(string)
	actorLabel = sub
	if id, err := strconv.ParseInt(sub, 10, 64); err == nil {
		actorID = &id
	}
	return actorID, actorLabel
}

// ----------------------------------------------------------------------------
// GET /api/v1/proposals/{type}
// ----------------------------------------------------------------------------

// ListProposals handles GET /api/v1/proposals/{type}
//
// Query params:
//   - status      — default "pending"
//   - project_id  — required for defect and known_issue; optional for flaky
//   - limit        — default 50, max 200
//   - cursor       — opaque pagination token
func (h *ProposalsHandler) ListProposals(w http.ResponseWriter, r *http.Request) {
	pt, ok := parseProposalType(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	status := q.Get("status")
	if status == "" {
		status = string(store.ProposalStatusPending)
	}

	limitStr := q.Get("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 200 {
		limit = 200
	}
	cursor := q.Get("cursor")

	// project_id is required for defect and known_issue.
	var projectID int
	projectIDStr := q.Get("project_id")
	if pt == proposalTypeDefect || pt == proposalTypeKnownIssue {
		if projectIDStr == "" {
			writeError(w, http.StatusBadRequest, "project_id is required for this proposal type")
			return
		}
	}
	if projectIDStr != "" {
		pid, err := strconv.Atoi(projectIDStr)
		if err != nil || pid < 1 {
			writeError(w, http.StatusBadRequest, "project_id must be a positive integer")
			return
		}
		projectID = pid
	}

	// Only "pending" is supported by the ListPending store method; callers
	// requesting a different status receive an empty list (future extension point).
	if status != string(store.ProposalStatusPending) {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "next_cursor": ""})
		return
	}

	// Determine whether the caller is admin (to decide PII visibility).
	isAdmin := false
	if claims, ok := middleware.ClaimsFromContext(r.Context()); ok {
		isAdmin = claims["role"] == "admin"
	}

	switch pt {
	case proposalTypeDefect:
		items, nextCursor, err := h.defectProposals.ListPending(r.Context(), projectID, limit, cursor)
		if err != nil {
			h.logger.Error("list defect proposals", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing proposals")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":       sanitiseDefectProposals(items, isAdmin),
			"next_cursor": nextCursor,
		})

	case proposalTypeKnownIssue:
		items, nextCursor, err := h.knownIssueProposals.ListPending(r.Context(), projectID, limit, cursor)
		if err != nil {
			h.logger.Error("list known issue proposals", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing proposals")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":       sanitiseKnownIssueProposals(items, isAdmin),
			"next_cursor": nextCursor,
		})

	case proposalTypeFlaky:
		items, nextCursor, err := h.flakyProposals.ListPending(r.Context(), projectID, limit, cursor)
		if err != nil {
			h.logger.Error("list flaky proposals", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing proposals")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":       sanitiseFlakyProposals(items, isAdmin),
			"next_cursor": nextCursor,
		})
	}
}

// sanitiseDefectProposals strips proposer_api_key_id when the caller is not admin.
func sanitiseDefectProposals(items []*store.DefectProposal, isAdmin bool) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		m := map[string]any{
			"id":                  p.ID,
			"project_id":          p.ProjectID,
			"fingerprint_hash":    p.FingerprintHash,
			"proposed_category":   p.ProposedCategory,
			"proposed_resolution": p.ProposedResolution,
			"rationale":           p.Rationale,
			"proposer_user_id":    p.ProposerUserID,
			"status":              p.Status,
			"reviewed_by_user_id": p.ReviewedByUserID,
			"reviewed_at":         p.ReviewedAt,
			"created_at":          p.CreatedAt,
		}
		if isAdmin {
			m["proposer_api_key_id"] = p.ProposerAPIKeyID
		}
		out = append(out, m)
	}
	return out
}

func sanitiseKnownIssueProposals(items []*store.KnownIssueProposal, isAdmin bool) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		m := map[string]any{
			"id":                   p.ID,
			"project_id":           p.ProjectID,
			"error_message_sample": p.ErrorMessageSample,
			"proposed_category":    p.ProposedCategory,
			"proposed_resolution":  p.ProposedResolution,
			"rationale":            p.Rationale,
			"regex_pattern":        p.RegexPattern,
			"applies_to_status":    p.AppliesToStatus,
			"dry_run_match_count":  p.DryRunMatchCount,
			"proposer_user_id":     p.ProposerUserID,
			"status":               p.Status,
			"reviewed_by_user_id":  p.ReviewedByUserID,
			"reviewed_at":          p.ReviewedAt,
			"created_at":           p.CreatedAt,
		}
		if isAdmin {
			m["proposer_api_key_id"] = p.ProposerAPIKeyID
		}
		out = append(out, m)
	}
	return out
}

func sanitiseFlakyProposals(items []*store.FlakyProposal, isAdmin bool) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		m := map[string]any{
			"id":                  p.ID,
			"project_id":          p.ProjectID,
			"test_full_name":      p.TestFullName,
			"history_id":          p.HistoryID,
			"rationale":           p.Rationale,
			"proposer_user_id":    p.ProposerUserID,
			"status":              p.Status,
			"reviewed_by_user_id": p.ReviewedByUserID,
			"reviewed_at":         p.ReviewedAt,
			"created_at":          p.CreatedAt,
		}
		if isAdmin {
			m["proposer_api_key_id"] = p.ProposerAPIKeyID
		}
		out = append(out, m)
	}
	return out
}

// ----------------------------------------------------------------------------
// POST /api/v1/proposals/{type}/{id}/approve
// ----------------------------------------------------------------------------

// ApproveProposal handles POST /api/v1/proposals/{type}/{id}/approve (admin only).
// Applies the change, marks the proposal approved, and writes an audit_log row —
// all in a single transaction. On any failure the transaction is rolled back.
func (h *ProposalsHandler) ApproveProposal(w http.ResponseWriter, r *http.Request) {
	pt, ok := parseProposalType(w, r)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	proposalID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || proposalID < 1 {
		writeError(w, http.StatusBadRequest, "proposal id must be a positive integer")
		return
	}

	actorID, actorLabel := actorFromContext(r)

	switch pt {
	case proposalTypeDefect:
		h.approveDefect(w, r, proposalID, actorID, actorLabel)
	case proposalTypeKnownIssue:
		h.approveKnownIssue(w, r, proposalID, actorID, actorLabel)
	case proposalTypeFlaky:
		h.approveFlaky(w, r, proposalID, actorID, actorLabel)
	}
}

func (h *ProposalsHandler) approveDefect(w http.ResponseWriter, r *http.Request, proposalID int64, actorID *int64, actorLabel string) {
	ctx := r.Context()

	proposal, err := h.defectProposals.Get(ctx, proposalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "proposal not found")
		return
	}
	if proposal.Status != store.ProposalStatusPending {
		writeError(w, http.StatusConflict, "proposal is not pending")
		return
	}

	var resolvedActorID int64
	if actorID != nil {
		resolvedActorID = *actorID
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.logger.Error("approve defect proposal: begin tx", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Apply the proposed category/resolution to the defect fingerprint.
	cat := proposal.ProposedCategory
	var res *string
	if proposal.ProposedResolution != "" {
		r2 := proposal.ProposedResolution
		res = &r2
	}
	if _, execErr := tx.Exec(ctx, `
		UPDATE defect_fingerprints
		SET category = $1, resolution = COALESCE($2, resolution), updated_at = NOW()
		WHERE fingerprint_hash = $3 AND project_id = $4`,
		cat, res, proposal.FingerprintHash, proposal.ProjectID,
	); execErr != nil {
		h.logger.Error("approve defect proposal: update fingerprint", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error applying proposal")
		return
	}

	// Mark proposal reviewed.
	now := time.Now()
	if _, execErr := tx.Exec(ctx, `
		UPDATE defect_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		store.ProposalStatusApproved, resolvedActorID, now, proposalID,
	); execErr != nil {
		h.logger.Error("approve defect proposal: mark reviewed", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error marking proposal approved")
		return
	}

	// Write audit_log inside the same transaction.
	meta, _ := json.Marshal(map[string]any{
		"proposal_id":       proposalID,
		"fingerprint_hash":  proposal.FingerprintHash,
		"proposed_category": proposal.ProposedCategory,
	})
	if _, execErr := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_label, target_type, target_id, action, outcome, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actorID, actorLabel, "proposal", fmt.Sprintf("%d", proposalID),
		store.AuditActionMCPProposalApprove, store.AuditOutcomeSuccess, meta,
	); execErr != nil {
		h.logger.Error("approve defect proposal: audit insert", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error writing audit log")
		return
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		h.logger.Error("approve defect proposal: commit", zap.Error(commitErr))
		writeError(w, http.StatusInternalServerError, "transaction commit failed")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"id": proposalID, "status": "approved"}, "Proposal approved")
}

func (h *ProposalsHandler) approveKnownIssue(w http.ResponseWriter, r *http.Request, proposalID int64, actorID *int64, actorLabel string) {
	ctx := r.Context()

	proposal, err := h.knownIssueProposals.Get(ctx, proposalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "proposal not found")
		return
	}
	if proposal.Status != store.ProposalStatusPending {
		writeError(w, http.StatusConflict, "proposal is not pending")
		return
	}

	var resolvedActorID int64
	if actorID != nil {
		resolvedActorID = *actorID
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.logger.Error("approve known issue proposal: begin tx", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Insert the known issue using the proposed regex_pattern as pattern and
	// error_message_sample as test_name (closest field mapping).
	testName := proposal.ErrorMessageSample
	if testName == "" {
		testName = proposal.RegexPattern
	}
	ticketURL := ""
	description := proposal.Rationale
	if _, execErr := tx.Exec(ctx, `
		INSERT INTO known_issues (project_id, test_name, pattern, ticket_url, description, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
		ON CONFLICT (project_id, test_name) DO UPDATE
		  SET pattern = EXCLUDED.pattern,
		      description = EXCLUDED.description,
		      is_active = true,
		      updated_at = NOW()`,
		proposal.ProjectID, testName, proposal.RegexPattern, ticketURL, description,
	); execErr != nil {
		h.logger.Error("approve known issue proposal: insert known issue", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error applying proposal")
		return
	}

	now := time.Now()
	if _, execErr := tx.Exec(ctx, `
		UPDATE known_issue_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		store.ProposalStatusApproved, resolvedActorID, now, proposalID,
	); execErr != nil {
		h.logger.Error("approve known issue proposal: mark reviewed", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error marking proposal approved")
		return
	}

	meta, _ := json.Marshal(map[string]any{
		"proposal_id":   proposalID,
		"regex_pattern": proposal.RegexPattern,
		"project_id":    proposal.ProjectID,
	})
	if _, execErr := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_label, target_type, target_id, action, outcome, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actorID, actorLabel, "proposal", fmt.Sprintf("%d", proposalID),
		store.AuditActionMCPProposalApprove, store.AuditOutcomeSuccess, meta,
	); execErr != nil {
		h.logger.Error("approve known issue proposal: audit insert", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error writing audit log")
		return
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		h.logger.Error("approve known issue proposal: commit", zap.Error(commitErr))
		writeError(w, http.StatusInternalServerError, "transaction commit failed")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"id": proposalID, "status": "approved"}, "Proposal approved")
}

func (h *ProposalsHandler) approveFlaky(w http.ResponseWriter, r *http.Request, proposalID int64, actorID *int64, actorLabel string) {
	ctx := r.Context()

	proposal, err := h.flakyProposals.Get(ctx, proposalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "proposal not found")
		return
	}
	if proposal.Status != store.ProposalStatusPending {
		writeError(w, http.StatusConflict, "proposal is not pending")
		return
	}

	var resolvedActorID int64
	if actorID != nil {
		resolvedActorID = *actorID
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.logger.Error("approve flaky proposal: begin tx", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Mark the most-recent test result matching (project_id, history_id, full_name) as flaky.
	if _, execErr := tx.Exec(ctx, `
		UPDATE test_results
		SET flaky = true
		WHERE id = (
			SELECT id FROM test_results
			WHERE project_id = $1 AND history_id = $2 AND full_name = $3
			ORDER BY id DESC
			LIMIT 1
		)`,
		proposal.ProjectID, proposal.HistoryID, proposal.TestFullName,
	); execErr != nil {
		h.logger.Error("approve flaky proposal: mark flaky", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error applying proposal")
		return
	}

	now := time.Now()
	if _, execErr := tx.Exec(ctx, `
		UPDATE flaky_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		store.ProposalStatusApproved, resolvedActorID, now, proposalID,
	); execErr != nil {
		h.logger.Error("approve flaky proposal: mark reviewed", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error marking proposal approved")
		return
	}

	meta, _ := json.Marshal(map[string]any{
		"proposal_id":    proposalID,
		"test_full_name": proposal.TestFullName,
		"history_id":     proposal.HistoryID,
	})
	if _, execErr := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_label, target_type, target_id, action, outcome, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actorID, actorLabel, "proposal", fmt.Sprintf("%d", proposalID),
		store.AuditActionMCPProposalApprove, store.AuditOutcomeSuccess, meta,
	); execErr != nil {
		h.logger.Error("approve flaky proposal: audit insert", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error writing audit log")
		return
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		h.logger.Error("approve flaky proposal: commit", zap.Error(commitErr))
		writeError(w, http.StatusInternalServerError, "transaction commit failed")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"id": proposalID, "status": "approved"}, "Proposal approved")
}

// ----------------------------------------------------------------------------
// POST /api/v1/proposals/{type}/{id}/reject
// ----------------------------------------------------------------------------

// RejectProposal handles POST /api/v1/proposals/{type}/{id}/reject (admin only).
// Marks the proposal rejected and writes an audit_log row — all in one transaction.
func (h *ProposalsHandler) RejectProposal(w http.ResponseWriter, r *http.Request) {
	pt, ok := parseProposalType(w, r)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	proposalID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || proposalID < 1 {
		writeError(w, http.StatusBadRequest, "proposal id must be a positive integer")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// Decode is best-effort; an empty body or missing reason is fine.
	_ = json.NewDecoder(r.Body).Decode(&body)

	actorID, actorLabel := actorFromContext(r)

	var resolvedActorID int64
	if actorID != nil {
		resolvedActorID = *actorID
	}

	// Determine which table to update based on proposal type.
	var tableName string
	switch pt {
	case proposalTypeDefect:
		tableName = "defect_proposals"
	case proposalTypeKnownIssue:
		tableName = "known_issue_proposals"
	case proposalTypeFlaky:
		tableName = "flaky_proposals"
	}

	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.logger.Error("reject proposal: begin tx", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	tag, execErr := tx.Exec(ctx,
		fmt.Sprintf(`UPDATE %s SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3 WHERE id = $4 AND status = $5`, tableName), //nolint:gosec // tableName is a controlled constant, not user input
		store.ProposalStatusRejected, resolvedActorID, now, proposalID, store.ProposalStatusPending,
	)
	if execErr != nil {
		h.logger.Error("reject proposal: update", zap.String("type", string(pt)), zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error rejecting proposal")
		return
	}
	if tag.RowsAffected() == 0 {
		// Either not found or not pending — report accordingly.
		writeError(w, http.StatusConflict, "proposal not found or is not pending")
		return
	}

	meta, _ := json.Marshal(map[string]any{
		"proposal_id":   proposalID,
		"proposal_type": string(pt),
		"reason":        body.Reason,
	})
	if _, execErr := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_label, target_type, target_id, action, outcome, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actorID, actorLabel, "proposal", fmt.Sprintf("%d", proposalID),
		store.AuditActionMCPProposalReject, store.AuditOutcomeSuccess, meta,
	); execErr != nil {
		h.logger.Error("reject proposal: audit insert", zap.Error(execErr))
		writeError(w, http.StatusInternalServerError, "error writing audit log")
		return
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		h.logger.Error("reject proposal: commit", zap.Error(commitErr))
		writeError(w, http.StatusInternalServerError, "transaction commit failed")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"id": proposalID, "status": "rejected"}, "Proposal rejected")
}
