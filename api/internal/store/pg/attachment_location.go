package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// GetLocation resolves the file-storage location of an attachment by joining
// test_attachments → test_results → builds → projects. Returns
// store.ErrAttachmentNotFound when the attachment (or any join target) is
// missing.
//
// The owning project_id is selected alongside the storage coordinates so a
// caller that only holds an attachment id can check the row against the
// project it was asked about before serving any bytes.
func (a *AttachmentStore) GetLocation(ctx context.Context, id int64) (*store.AttachmentLocation, error) {
	var loc store.AttachmentLocation
	err := a.pool.QueryRow(ctx, `
		SELECT tr.project_id, p.storage_key, b.build_order, ta.source, ta.mime_type, ta.size_bytes
		FROM test_attachments ta
		JOIN test_results tr ON tr.id = ta.test_result_id
		JOIN builds b        ON b.id = tr.build_id
		JOIN projects p      ON p.id = tr.project_id
		WHERE ta.id = $1`,
		id,
	).Scan(&loc.ProjectID, &loc.StorageKey, &loc.BuildNumber, &loc.Source, &loc.MimeType, &loc.SizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrAttachmentNotFound
		}
		return nil, fmt.Errorf("get attachment location: %w", err)
	}
	return &loc, nil
}
