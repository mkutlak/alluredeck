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
var _ store.FlakyProposalStorer = (*FlakyProposalStore)(nil)

// FlakyProposalStore provides operations on the flaky_proposals table.
type FlakyProposalStore struct {
	pool *pgxpool.Pool
}

// NewFlakyProposalStore creates a FlakyProposalStore backed by the given PGStore.
func NewFlakyProposalStore(s *PGStore) *FlakyProposalStore {
	return &FlakyProposalStore{pool: s.pool}
}

// Create inserts a new flaky proposal and returns its assigned ID.
// Nullable fields (rationale, proposer_api_key_id) are stored as NULL when zero/empty.
func (s *FlakyProposalStore) Create(ctx context.Context, p *store.FlakyProposal) (int64, error) {
	var apiKeyID *int64
	if p.ProposerAPIKeyID != 0 {
		apiKeyID = &p.ProposerAPIKeyID
	}
	var rationale *string
	if p.Rationale != "" {
		rationale = &p.Rationale
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO flaky_proposals (
			project_id, test_full_name, history_id, rationale,
			proposer_user_id, proposer_api_key_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		p.ProjectID, p.TestFullName, p.HistoryID, rationale,
		p.ProposerUserID, apiKeyID, p.Status,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create flaky proposal: %w", err)
	}
	return id, nil
}

// Get retrieves a single flaky proposal by ID.
func (s *FlakyProposalStore) Get(ctx context.Context, id int64) (*store.FlakyProposal, error) {
	var p store.FlakyProposal
	var rationale *string
	var apiKeyID, reviewedBy *int64

	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, test_full_name, history_id, rationale,
		       proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM flaky_proposals
		WHERE id = $1`, id,
	).Scan(
		&p.ID, &p.ProjectID, &p.TestFullName, &p.HistoryID, &rationale,
		&p.ProposerUserID, &apiKeyID, &p.Status,
		&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("flaky proposal not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get flaky proposal: %w", err)
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

// ListPending returns pending flaky proposals for a project using cursor-based
// pagination keyed on id DESC. cursor is the opaque token returned by a
// previous call; "" starts from the beginning.
func (s *FlakyProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.FlakyProposal, string, error) {
	if limit <= 0 {
		limit = 20
	}

	args := []any{projectID, store.ProposalStatusPending}
	q := `
		SELECT id, project_id, test_full_name, history_id, rationale,
		       proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM flaky_proposals
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
		return nil, "", fmt.Errorf("list pending flaky proposals: %w", err)
	}
	defer rows.Close()

	var items []*store.FlakyProposal
	for rows.Next() {
		p := &store.FlakyProposal{}
		var rationale *string
		var apiKeyID, reviewedBy *int64

		if err := rows.Scan(
			&p.ID, &p.ProjectID, &p.TestFullName, &p.HistoryID, &rationale,
			&p.ProposerUserID, &apiKeyID, &p.Status,
			&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan flaky proposal: %w", err)
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
		return nil, "", fmt.Errorf("iterate flaky proposal rows: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		items = items[:limit]
		nextCursor = encodeCursorID(items[limit-1].ID)
	}
	return items, nextCursor, nil
}

// MarkReviewed sets status, reviewed_by_user_id, and reviewed_at on a proposal.
func (s *FlakyProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	now := time.Now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE flaky_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		status, reviewedBy, now, id,
	)
	if err != nil {
		return fmt.Errorf("mark flaky proposal reviewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("flaky proposal not found: id=%d", id)
	}
	return nil
}
