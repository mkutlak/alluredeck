package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// TestLocalStore_WriteRawBlob_RoundTrip writes a raw blob, reads it back via
// OpenBlob, and confirms ListStagingBlobs surfaces the key.
func TestLocalStore_WriteRawBlob_RoundTrip(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ls := NewLocalStore(&config.Config{ProjectsPath: tmp})

	key := "staging/abc123.tar.gz"
	body := []byte("fake gzip body")

	if err := ls.WriteRawBlob(context.Background(), key, bytes.NewReader(body)); err != nil {
		t.Fatalf("WriteRawBlob: %v", err)
	}

	rc, err := ls.OpenBlob(context.Background(), key)
	if err != nil {
		t.Fatalf("OpenBlob: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body mismatch: got %q want %q", got, body)
	}

	// File must exist on disk under the staging dir.
	if _, err := os.Stat(filepath.Join(tmp, "staging", "abc123.tar.gz")); err != nil {
		t.Errorf("expected staging file on disk: %v", err)
	}

	// ListStagingBlobs surfaces the key once we relax the age window. Setting
	// olderThan=0 means no age filter, so freshly created blobs appear too.
	keys, err := ls.ListStagingBlobs(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListStagingBlobs: %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Errorf("ListStagingBlobs: got %v, want [%q]", keys, key)
	}
}

// TestLocalStore_DeleteBlob_Idempotent verifies DeleteBlob succeeds when the
// key already exists and silently no-ops when it doesn't.
func TestLocalStore_DeleteBlob_Idempotent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ls := NewLocalStore(&config.Config{ProjectsPath: tmp})

	key := "staging/missing.tar.gz"
	// Missing — no error.
	if err := ls.DeleteBlob(context.Background(), key); err != nil {
		t.Fatalf("DeleteBlob (missing): %v", err)
	}

	// Create then delete.
	if err := ls.WriteRawBlob(context.Background(), key, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("WriteRawBlob: %v", err)
	}
	if err := ls.DeleteBlob(context.Background(), key); err != nil {
		t.Fatalf("DeleteBlob (present): %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(key))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be removed, stat err=%v", err)
	}
}

// TestLocalStore_ListStagingBlobs_AgeFilter verifies the olderThan filter
// excludes recently written blobs.
func TestLocalStore_ListStagingBlobs_AgeFilter(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ls := NewLocalStore(&config.Config{ProjectsPath: tmp})

	if err := ls.WriteRawBlob(context.Background(), "staging/fresh.tar.gz", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("WriteRawBlob: %v", err)
	}

	// Anything older than 1 hour — should be empty.
	keys, err := ls.ListStagingBlobs(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("ListStagingBlobs: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected age filter to exclude fresh blob, got %v", keys)
	}
}
