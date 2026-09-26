package storage

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestLocalStore_StagingBlobs round-trips a staged upload under the
// "staging/" key prefix: ListStagingBlobs filters by age (0 = no filter) and
// DeleteBlob is idempotent.
func TestLocalStore_StagingBlobs(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()
	const key = "staging/abc123.tar.gz"

	if err := ls.DeleteBlob(ctx, key); err != nil {
		t.Fatalf("DeleteBlob of a missing blob: %v", err)
	}
	if err := ls.WriteRawBlob(ctx, key, strings.NewReader("fake gzip body")); err != nil {
		t.Fatalf("WriteRawBlob: %v", err)
	}
	checkExists(t, root, map[string]bool{"staging/abc123.tar.gz": true})

	rc, err := ls.OpenBlob(ctx, key)
	if err != nil {
		t.Fatalf("OpenBlob: %v", err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(body) != "fake gzip body" {
		t.Errorf("OpenBlob body = %q", body)
	}

	for olderThan, want := range map[time.Duration][]string{0: {key}, time.Hour: nil} {
		if keys, err := ls.ListStagingBlobs(ctx, olderThan); err != nil || !slices.Equal(keys, want) {
			t.Errorf("ListStagingBlobs(%v) = %v, %v; want %v", olderThan, keys, err, want)
		}
	}

	if err := ls.DeleteBlob(ctx, key); err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}
	checkExists(t, root, map[string]bool{key: false})
}
