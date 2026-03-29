package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGAttachmentStore provides attachment queries backed by PostgreSQL.
type PGAttachmentStore struct {
	pool *pgxpool.Pool
}

// NewAttachmentStore creates a PGAttachmentStore backed by the given PGStore.
func NewAttachmentStore(s *PGStore) *PGAttachmentStore {
	return &PGAttachmentStore{pool: s.pool}
}

// ListByBuild returns attachment metadata for all attachments belonging to a
// specific build, with optional MIME type prefix filtering and pagination.
func (a *PGAttachmentStore) ListByBuild(ctx context.Context, projectID string, buildID int64, mimeFilter string, limit, offset int) ([]store.TestAttachment, int, error) {
	var rows pgx.Rows
	var err error

	if mimeFilter != "" {
		rows, err = a.pool.Query(ctx, `
			SELECT ta.id, ta.test_result_id, ta.test_step_id, ta.name, ta.source, ta.mime_type, ta.size_bytes,
			       tr.test_name, tr.status,
			       COUNT(*) OVER() AS total
			FROM test_attachments ta
			JOIN test_results tr ON tr.id = ta.test_result_id
			WHERE tr.build_id = $1
			  AND tr.project_id = $2
			  AND ta.mime_type LIKE $3 || '%'
			ORDER BY tr.test_name, ta.id
			LIMIT $4 OFFSET $5`,
			buildID, projectID, mimeFilter, limit, offset,
		)
	} else {
		rows, err = a.pool.Query(ctx, `
			SELECT ta.id, ta.test_result_id, ta.test_step_id, ta.name, ta.source, ta.mime_type, ta.size_bytes,
			       tr.test_name, tr.status,
			       COUNT(*) OVER() AS total
			FROM test_attachments ta
			JOIN test_results tr ON tr.id = ta.test_result_id
			WHERE tr.build_id = $1
			  AND tr.project_id = $2
			ORDER BY tr.test_name, ta.id
			LIMIT $3 OFFSET $4`,
			buildID, projectID, limit, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list attachments by build: %w", err)
	}
	defer rows.Close()

	var result []store.TestAttachment
	var total int
	for rows.Next() {
		var at store.TestAttachment
		if err := rows.Scan(&at.ID, &at.TestResultID, &at.TestStepID, &at.Name, &at.Source, &at.MimeType, &at.SizeBytes, &at.TestName, &at.TestStatus, &total); err != nil {
			return nil, 0, fmt.Errorf("scan attachment: %w", err)
		}
		result = append(result, at)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate attachments: %w", err)
	}
	if result == nil {
		result = []store.TestAttachment{}
	}
	return result, total, nil
}

// GetBySource returns the attachment metadata for the given source path within
// the specified build. Returns store.ErrAttachmentNotFound when no match exists.
func (a *PGAttachmentStore) GetBySource(ctx context.Context, buildID int64, source string) (*store.TestAttachment, error) {
	var at store.TestAttachment
	err := a.pool.QueryRow(ctx, `
		SELECT ta.id, ta.test_result_id, ta.test_step_id, ta.name, ta.source, ta.mime_type, ta.size_bytes
		FROM test_attachments ta
		JOIN test_results tr ON tr.id = ta.test_result_id
		WHERE tr.build_id = $1
		  AND ta.source = $2
		LIMIT 1`,
		buildID, source,
	).Scan(&at.ID, &at.TestResultID, &at.TestStepID, &at.Name, &at.Source, &at.MimeType, &at.SizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrAttachmentNotFound
		}
		return nil, fmt.Errorf("get attachment by source: %w", err)
	}
	return &at, nil
}

// Compile-time interface compliance check.
var _ store.AttachmentStorer = (*PGAttachmentStore)(nil)
