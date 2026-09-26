package pg_test

import (
	"errors"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestAttachmentStore_GetLocation exercises the test_attachments →
// test_results → builds → projects join. Regression guard for a query that
// selected the non-existent builds.build_number (the column is build_order),
// failing every call with SQLSTATE 42703. An unknown id is
// store.ErrAttachmentNotFound, not a raw SQL error.
func TestAttachmentStore_GetLocation(t *testing.T) {
	f := newFixture(t)
	const buildOrder = 7
	b := f.build(buildOrder)
	f.insert(f.result(b, "com.example.AuthTest", "failed", "hist-attach-loc"))
	tr, err := f.results.GetByHistoryID(f.ctx, f.id, b, "hist-attach-loc")
	if err != nil || tr == nil {
		t.Fatalf("GetByHistoryID = %v, %v", tr, err)
	}
	var attachmentID int64
	if err := f.s.Pool().QueryRow(f.ctx,
		`INSERT INTO test_attachments (test_result_id, name, source, mime_type, size_bytes)
		 VALUES ($1, 'Response Body', 'response-body-abc123.txt', 'text/plain', 626) RETURNING id`,
		tr.ID).Scan(&attachmentID); err != nil {
		t.Fatalf("insert test_attachment: %v", err)
	}

	attachments := pg.NewAttachmentStore(f.s)
	loc, err := attachments.GetLocation(f.ctx, attachmentID)
	if err != nil {
		t.Fatalf("GetLocation: %v", err)
	}
	want := store.AttachmentLocation{
		ProjectID: f.id, BuildNumber: buildOrder, StorageKey: f.project.StorageKey,
		Source: "response-body-abc123.txt", MimeType: "text/plain", SizeBytes: 626,
	}
	if *loc != want {
		t.Errorf("GetLocation = %+v, want %+v", *loc, want)
	}

	if _, err := attachments.GetLocation(f.ctx, 999999999); !errors.Is(err, store.ErrAttachmentNotFound) {
		t.Errorf("GetLocation(unknown) error = %v, want store.ErrAttachmentNotFound", err)
	}
}
