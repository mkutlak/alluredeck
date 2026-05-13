package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AttachmentStore provides attachment queries backed by PostgreSQL.
type AttachmentStore struct {
	pool *pgxpool.Pool
}

// NewAttachmentStore creates a AttachmentStore backed by the given PGStore.
func NewAttachmentStore(s *PGStore) *AttachmentStore {
	return &AttachmentStore{pool: s.pool}
}

// ListByBuild returns attachment metadata for all attachments belonging to a
// specific build, with optional MIME type prefix filtering, optional test status
// filtering, and pagination.
func (a *AttachmentStore) ListByBuild(ctx context.Context, projectID int64, buildID int64, mimeFilter, testStatus string, limit, offset int) ([]store.TestAttachment, int, error) {
	var rows pgx.Rows
	var err error

	switch {
	case mimeFilter != "" && testStatus != "":
		rows, err = a.pool.Query(ctx, `
			SELECT ta.id, ta.test_result_id, ta.test_step_id, ta.name, ta.source, ta.mime_type, ta.size_bytes,
			       tr.test_name, tr.status,
			       COUNT(*) OVER() AS total
			FROM test_attachments ta
			JOIN test_results tr ON tr.id = ta.test_result_id
			WHERE tr.build_id = $1
			  AND tr.project_id = $2
			  AND ta.mime_type LIKE $3 || '%'
			  AND tr.status = $4
			ORDER BY tr.test_name, ta.id
			LIMIT $5 OFFSET $6`,
			buildID, projectID, mimeFilter, testStatus, limit, offset,
		)
	case mimeFilter != "":
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
	case testStatus != "":
		rows, err = a.pool.Query(ctx, `
			SELECT ta.id, ta.test_result_id, ta.test_step_id, ta.name, ta.source, ta.mime_type, ta.size_bytes,
			       tr.test_name, tr.status,
			       COUNT(*) OVER() AS total
			FROM test_attachments ta
			JOIN test_results tr ON tr.id = ta.test_result_id
			WHERE tr.build_id = $1
			  AND tr.project_id = $2
			  AND tr.status = $3
			ORDER BY tr.test_name, ta.id
			LIMIT $4 OFFSET $5`,
			buildID, projectID, testStatus, limit, offset,
		)
	default:
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
func (a *AttachmentStore) GetBySource(ctx context.Context, buildID int64, source string) (*store.TestAttachment, error) {
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

// InsertBuildAttachments inserts build-level attachments (e.g. from a Playwright
// data/ directory) that are not linked to a specific test result. Each attachment
// is inserted with NULL test_result_id and test_step_id.
func (a *AttachmentStore) InsertBuildAttachments(ctx context.Context, _ int64, _ int64, attachments []store.TestAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, att := range attachments {
		if _, err := tx.Exec(ctx,
			`INSERT INTO test_attachments(name, source, mime_type, size_bytes)
			 VALUES ($1,$2,$3,$4)`,
			att.Name, att.Source, att.MimeType, att.SizeBytes,
		); err != nil {
			return fmt.Errorf("insert build attachment %q: %w", att.Source, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetByID returns attachment metadata for the given primary-key ID.
// Returns store.ErrAttachmentNotFound when no row exists.
func (a *AttachmentStore) GetByID(ctx context.Context, id int64) (*store.TestAttachment, error) {
	var at store.TestAttachment
	err := a.pool.QueryRow(ctx, `
		SELECT id, test_result_id, test_step_id, name, source, mime_type, size_bytes
		FROM test_attachments
		WHERE id = $1`,
		id,
	).Scan(&at.ID, &at.TestResultID, &at.TestStepID, &at.Name, &at.Source, &at.MimeType, &at.SizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrAttachmentNotFound
		}
		return nil, fmt.Errorf("get attachment by id: %w", err)
	}
	return &at, nil
}

// Compile-time interface compliance check.
var _ store.AttachmentStorer = (*AttachmentStore)(nil)
