package store_test

import (
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func openTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
