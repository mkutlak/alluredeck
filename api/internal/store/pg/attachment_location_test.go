package pg_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestAttachmentStore_GetLocation_ResolvesBuildOrder exercises the
// test_attachments → test_results → builds → projects join. It is a regression
// guard for a query that selected a non-existent builds.build_number column
// (the real column is build_order); before the fix every call failed with
// SQLSTATE 42703.
func TestAttachmentStore_GetLocation_ResolvesBuildOrder(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("attach-loc-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	const buildOrder = 7
	if err := buildStore.InsertBuild(ctx, projectID, buildOrder); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err := pg.NewTestResultStore(s, logger).GetBuildID(ctx, projectID, buildOrder)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	var testResultID int64
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO test_results (build_id, project_id, history_id, test_name, full_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		buildID, projectID, "hist-attach-loc", "AuthTest", "com.example.AuthTest", "failed",
	).Scan(&testResultID); err != nil {
		t.Fatalf("insert test_result: %v", err)
	}

	const (
		attName = "Response Body"
		attSrc  = "response-body-abc123.txt"
		attMime = "text/plain"
		attSize = int64(626)
	)
	var attachmentID int64
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO test_attachments (test_result_id, name, source, mime_type, size_bytes)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		testResultID, attName, attSrc, attMime, attSize,
	).Scan(&attachmentID); err != nil {
		t.Fatalf("insert test_attachment: %v", err)
	}

	loc, err := pg.NewAttachmentStore(s).GetLocation(ctx, attachmentID)
	if err != nil {
		t.Fatalf("GetLocation: %v", err)
	}
	if loc.BuildNumber != buildOrder {
		t.Errorf("BuildNumber = %d, want %d (builds.build_order)", loc.BuildNumber, buildOrder)
	}
	if loc.StorageKey != proj.StorageKey {
		t.Errorf("StorageKey = %q, want %q", loc.StorageKey, proj.StorageKey)
	}
	if loc.Source != attSrc {
		t.Errorf("Source = %q, want %q", loc.Source, attSrc)
	}
	if loc.MimeType != attMime {
		t.Errorf("MimeType = %q, want %q", loc.MimeType, attMime)
	}
	if loc.SizeBytes != attSize {
		t.Errorf("SizeBytes = %d, want %d", loc.SizeBytes, attSize)
	}
}

// TestAttachmentStore_GetLocation_NotFound verifies an unknown attachment id
// yields store.ErrAttachmentNotFound rather than a raw SQL error.
func TestAttachmentStore_GetLocation_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	_, err := pg.NewAttachmentStore(s).GetLocation(ctx, 999999999)
	if !errors.Is(err, store.ErrAttachmentNotFound) {
		t.Fatalf("GetLocation(unknown) error = %v, want store.ErrAttachmentNotFound", err)
	}
}
