package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface compliance check.
var _ store.KnownIssueProposalStorer = (*KnownIssueProposalStore)(nil)

// KnownIssueProposalStore provides operations on the known_issue_proposals table.
type KnownIssueProposalStore struct {
	pool *pgxpool.Pool
}

// NewKnownIssueProposalStore creates a KnownIssueProposalStore backed by the given PGStore.
func NewKnownIssueProposalStore(s *PGStore) *KnownIssueProposalStore {
	return &KnownIssueProposalStore{pool: s.pool}
}

// Create inserts a new known-issue proposal and returns its assigned ID.
// Nullable fields (error_message_sample, proposed_resolution, rationale,
// proposer_api_key_id) are stored as NULL when zero/empty.
func (s *KnownIssueProposalStore) Create(ctx context.Context, p *store.KnownIssueProposal) (int64, error) {
	var apiKeyID *int64
	if p.ProposerAPIKeyID != 0 {
		apiKeyID = &p.ProposerAPIKeyID
	}
	var errMsgSample *string
	if p.ErrorMessageSample != "" {
		errMsgSample = &p.ErrorMessageSample
	}
	var resolution *string
	if p.ProposedResolution != "" {
		resolution = &p.ProposedResolution
	}
	var rationale *string
	if p.Rationale != "" {
		rationale = &p.Rationale
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO known_issue_proposals (
			project_id, error_message_sample, proposed_category, proposed_resolution,
			rationale, regex_pattern, applies_to_status, dry_run_match_count,
			proposer_user_id, proposer_api_key_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		p.ProjectID, errMsgSample, p.ProposedCategory, resolution,
		rationale, p.RegexPattern, p.AppliesToStatus, p.DryRunMatchCount,
		p.ProposerUserID, apiKeyID, p.Status,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create known issue proposal: %w", err)
	}
	return id, nil
}

// Get retrieves a single known-issue proposal by ID.
func (s *KnownIssueProposalStore) Get(ctx context.Context, id int64) (*store.KnownIssueProposal, error) {
	var p store.KnownIssueProposal
	var errMsgSample, resolution, rationale *string
	var apiKeyID, reviewedBy *int64

	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, error_message_sample, proposed_category, proposed_resolution,
		       rationale, regex_pattern, applies_to_status, dry_run_match_count,
		       proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM known_issue_proposals
		WHERE id = $1`, id,
	).Scan(
		&p.ID, &p.ProjectID, &errMsgSample, &p.ProposedCategory, &resolution,
		&rationale, &p.RegexPattern, &p.AppliesToStatus, &p.DryRunMatchCount,
		&p.ProposerUserID, &apiKeyID, &p.Status,
		&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("known issue proposal not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get known issue proposal: %w", err)
	}

	if errMsgSample != nil {
		p.ErrorMessageSample = *errMsgSample
	}
	if resolution != nil {
		p.ProposedResolution = *resolution
	}
	if rationale != nil {
		p.Rationale = *rationale
	}
	if apiKeyID != nil {
		p.ProposerAPIKeyID = *apiKeyID
	}
	if reviewedBy != nil {
		p.ReviewedByUserID = *reviewedBy
	}
	return &p, nil
}

// ListPending returns pending known-issue proposals for a project using
// cursor-based pagination keyed on id DESC. cursor is the opaque token
// returned by a previous call; "" starts from the beginning.
func (s *KnownIssueProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.KnownIssueProposal, string, error) {
	if limit <= 0 {
		limit = 20
	}

	args := []any{projectID, store.ProposalStatusPending}
	q := `
		SELECT id, project_id, error_message_sample, proposed_category, proposed_resolution,
		       rationale, regex_pattern, applies_to_status, dry_run_match_count,
		       proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM known_issue_proposals
		WHERE project_id = $1 AND status = $2`

	if cursor != "" {
		cursorID, err := decodeCursorID(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		args = append(args, cursorID)
		q += fmt.Sprintf(" AND id < $%d", len(args))
	}

	args = append(args, limit+1)
	q += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list pending known issue proposals: %w", err)
	}
	defer rows.Close()

	var items []*store.KnownIssueProposal
	for rows.Next() {
		p := &store.KnownIssueProposal{}
		var errMsgSample, resolution, rationale *string
		var apiKeyID, reviewedBy *int64

		if err := rows.Scan(
			&p.ID, &p.ProjectID, &errMsgSample, &p.ProposedCategory, &resolution,
			&rationale, &p.RegexPattern, &p.AppliesToStatus, &p.DryRunMatchCount,
			&p.ProposerUserID, &apiKeyID, &p.Status,
			&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan known issue proposal: %w", err)
		}
		if errMsgSample != nil {
			p.ErrorMessageSample = *errMsgSample
		}
		if resolution != nil {
			p.ProposedResolution = *resolution
		}
		if rationale != nil {
			p.Rationale = *rationale
		}
		if apiKeyID != nil {
			p.ProposerAPIKeyID = *apiKeyID
		}
		if reviewedBy != nil {
			p.ReviewedByUserID = *reviewedBy
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate known issue proposal rows: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		items = items[:limit]
		nextCursor = encodeCursorID(items[limit-1].ID)
	}
	return items, nextCursor, nil
}

// MarkReviewed sets status, reviewed_by_user_id, and reviewed_at on a proposal.
func (s *KnownIssueProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	now := time.Now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE known_issue_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		status, reviewedBy, now, id,
	)
	if err != nil {
		return fmt.Errorf("mark known issue proposal reviewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("known issue proposal not found: id=%d", id)
	}
	return nil
}
