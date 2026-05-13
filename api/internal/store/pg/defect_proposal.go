package pg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface compliance check.
var _ store.DefectProposalStorer = (*DefectProposalStore)(nil)

// DefectProposalStore provides operations on the defect_proposals table.
type DefectProposalStore struct {
	pool *pgxpool.Pool
}

// NewDefectProposalStore creates a DefectProposalStore backed by the given PGStore.
func NewDefectProposalStore(s *PGStore) *DefectProposalStore {
	return &DefectProposalStore{pool: s.pool}
}

// Create inserts a new defect proposal and returns its assigned ID.
// proposer_api_key_id and proposed_resolution/rationale/reviewed_by are nullable;
// zero/empty values are stored as NULL.
func (s *DefectProposalStore) Create(ctx context.Context, p *store.DefectProposal) (int64, error) {
	var apiKeyID *int64
	if p.ProposerAPIKeyID != 0 {
		apiKeyID = &p.ProposerAPIKeyID
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
		INSERT INTO defect_proposals (
			project_id, fingerprint_hash, proposed_category, proposed_resolution,
			rationale, proposer_user_id, proposer_api_key_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		p.ProjectID, p.FingerprintHash, p.ProposedCategory, resolution,
		rationale, p.ProposerUserID, apiKeyID, p.Status,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create defect proposal: %w", err)
	}
	return id, nil
}

// Get retrieves a single defect proposal by ID.
func (s *DefectProposalStore) Get(ctx context.Context, id int64) (*store.DefectProposal, error) {
	var p store.DefectProposal
	var resolution, rationale *string
	var apiKeyID, reviewedBy *int64

	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, fingerprint_hash, proposed_category, proposed_resolution,
		       rationale, proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM defect_proposals
		WHERE id = $1`, id,
	).Scan(
		&p.ID, &p.ProjectID, &p.FingerprintHash, &p.ProposedCategory, &resolution,
		&rationale, &p.ProposerUserID, &apiKeyID, &p.Status,
		&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("defect proposal not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get defect proposal: %w", err)
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

// ListPending returns pending defect proposals for a project using cursor-based
// pagination keyed on (created_at DESC, id DESC). cursor is the opaque token
// returned by a previous call; "" starts from the beginning.
func (s *DefectProposalStore) ListPending(ctx context.Context, projectID int, limit int, cursor string) ([]*store.DefectProposal, string, error) {
	if limit <= 0 {
		limit = 20
	}

	args := []any{projectID, store.ProposalStatusPending}
	q := `
		SELECT id, project_id, fingerprint_hash, proposed_category, proposed_resolution,
		       rationale, proposer_user_id, proposer_api_key_id, status,
		       reviewed_by_user_id, reviewed_at, created_at
		FROM defect_proposals
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
		return nil, "", fmt.Errorf("list pending defect proposals: %w", err)
	}
	defer rows.Close()

	var items []*store.DefectProposal
	for rows.Next() {
		p := &store.DefectProposal{}
		var resolution, rationale *string
		var apiKeyID, reviewedBy *int64

		if err := rows.Scan(
			&p.ID, &p.ProjectID, &p.FingerprintHash, &p.ProposedCategory, &resolution,
			&rationale, &p.ProposerUserID, &apiKeyID, &p.Status,
			&reviewedBy, &p.ReviewedAt, &p.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan defect proposal: %w", err)
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
		return nil, "", fmt.Errorf("iterate defect proposal rows: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		items = items[:limit]
		nextCursor = encodeCursorID(items[limit-1].ID)
	}
	return items, nextCursor, nil
}

// MarkReviewed sets status, reviewed_by_user_id, and reviewed_at on a proposal.
func (s *DefectProposalStore) MarkReviewed(ctx context.Context, id int64, reviewedBy int64, status store.ProposalStatus) error {
	now := time.Now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE defect_proposals
		SET status = $1, reviewed_by_user_id = $2, reviewed_at = $3
		WHERE id = $4`,
		status, reviewedBy, now, id,
	)
	if err != nil {
		return fmt.Errorf("mark defect proposal reviewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("defect proposal not found: id=%d", id)
	}
	return nil
}

// encodeCursorID encodes an int64 ID as an opaque cursor string.
func encodeCursorID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// decodeCursorID decodes a cursor string back to an int64 ID.
func decodeCursorID(cursor string) (int64, error) {
	return strconv.ParseInt(cursor, 10, 64)
}
