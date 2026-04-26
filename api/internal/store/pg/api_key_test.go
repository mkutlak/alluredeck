//go:build integration

package pg_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

func TestPGAPIKeyStore_CreateAndGet(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)

	key := &store.APIKey{
		Name:     fmt.Sprintf("test-key-%d", time.Now().UnixNano()),
		Prefix:   "ald_a1b2c3d4",
		KeyHash:  fmt.Sprintf("hash-%d", time.Now().UnixNano()),
		Username: "testuser",
		Role:     "admin",
	}

	created, err := ks.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID after Create")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt after Create")
	}

	got, err := ks.GetByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByHash ID = %d, want %d", got.ID, created.ID)
	}
}

func TestPGAPIKeyStore_ListByUsername(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	username := fmt.Sprintf("user-%d", time.Now().UnixNano())

	for i := range 3 {
		_, err := ks.Create(ctx, &store.APIKey{
			Name:     fmt.Sprintf("key-%d", i),
			Prefix:   "ald_aaaaaaaa",
			KeyHash:  fmt.Sprintf("hash-%d-%d", i, time.Now().UnixNano()),
			Username: username,
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("Create key %d: %v", i, err)
		}
	}

	keys, err := ks.ListByUsername(ctx, username)
	if err != nil {
		t.Fatalf("ListByUsername: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestPGAPIKeyStore_GetByHash_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	_, err := ks.GetByHash(ctx, "nonexistent-hash")
	if !isAPIKeyNotFound(err) {
		t.Errorf("expected ErrAPIKeyNotFound, got %v", err)
	}
}

func TestPGAPIKeyStore_UpdateLastUsed(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	key := &store.APIKey{
		Name:     fmt.Sprintf("lu-key-%d", time.Now().UnixNano()),
		Prefix:   "ald_a1b2c3d4",
		KeyHash:  fmt.Sprintf("lu-hash-%d", time.Now().UnixNano()),
		Username: "testuser",
		Role:     "admin",
	}
	created, err := ks.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ks.UpdateLastUsed(ctx, created.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}

	got, err := ks.GetByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.LastUsed == nil {
		t.Error("expected LastUsed to be set after UpdateLastUsed")
	}
}

func TestPGAPIKeyStore_Delete(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	key := &store.APIKey{
		Name:     fmt.Sprintf("del-key-%d", time.Now().UnixNano()),
		Prefix:   "ald_a1b2c3d4",
		KeyHash:  fmt.Sprintf("del-hash-%d", time.Now().UnixNano()),
		Username: "owner",
		Role:     "admin",
	}
	created, err := ks.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wrong username → not found
	if err := ks.Delete(ctx, created.ID, "other-user"); !isAPIKeyNotFound(err) {
		t.Errorf("expected ErrAPIKeyNotFound for wrong user, got %v", err)
	}

	// Correct username → success
	if err := ks.Delete(ctx, created.ID, "owner"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Second delete → not found
	if err := ks.Delete(ctx, created.ID, "owner"); !isAPIKeyNotFound(err) {
		t.Errorf("expected ErrAPIKeyNotFound after deletion, got %v", err)
	}
}

func TestPGAPIKeyStore_CountByUsername(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	username := fmt.Sprintf("count-user-%d", time.Now().UnixNano())

	count, err := ks.CountByUsername(ctx, username)
	if err != nil {
		t.Fatalf("CountByUsername (empty): %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	_, err = ks.Create(ctx, &store.APIKey{
		Name:     "k1",
		Prefix:   "ald_aaaaaaaa",
		KeyHash:  fmt.Sprintf("c-hash-%d", time.Now().UnixNano()),
		Username: username,
		Role:     "viewer",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	count, err = ks.CountByUsername(ctx, username)
	if err != nil {
		t.Fatalf("CountByUsername: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func isAPIKeyNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, store.ErrAPIKeyNotFound)
}

// TestPGAPIKeyStore_DeleteAllForUser verifies the bulk-delete invariants used
// by F-2: every key owned by the target user is removed, the count matches,
// and keys owned by other users are untouched.
func TestPGAPIKeyStore_DeleteAllForUser(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	alice := fmt.Sprintf("alice-%d@x.test", time.Now().UnixNano())
	bob := fmt.Sprintf("bob-%d@x.test", time.Now().UnixNano())

	// Three keys for Alice.
	for i := range 3 {
		if _, err := ks.Create(ctx, &store.APIKey{
			Name:     fmt.Sprintf("alice-key-%d", i),
			Prefix:   "ald_aaaaaaaa",
			KeyHash:  fmt.Sprintf("alice-hash-%d-%d", i, time.Now().UnixNano()),
			Username: alice,
			Role:     "viewer",
		}); err != nil {
			t.Fatalf("Create alice key %d: %v", i, err)
		}
	}
	// One key for Bob — must survive.
	if _, err := ks.Create(ctx, &store.APIKey{
		Name:     "bob-key",
		Prefix:   "ald_bbbbbbbb",
		KeyHash:  fmt.Sprintf("bob-hash-%d", time.Now().UnixNano()),
		Username: bob,
		Role:     "viewer",
	}); err != nil {
		t.Fatalf("Create bob key: %v", err)
	}

	deleted, err := ks.DeleteAllForUser(ctx, alice)
	if err != nil {
		t.Fatalf("DeleteAllForUser: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}

	// Alice has no keys left.
	if count, err := ks.CountByUsername(ctx, alice); err != nil {
		t.Fatalf("CountByUsername alice: %v", err)
	} else if count != 0 {
		t.Errorf("alice CountByUsername = %d, want 0", count)
	}
	// Bob's key is untouched.
	if count, err := ks.CountByUsername(ctx, bob); err != nil {
		t.Fatalf("CountByUsername bob: %v", err)
	} else if count != 1 {
		t.Errorf("bob CountByUsername = %d, want 1 (other users untouched)", count)
	}
}

// TestPGAPIKeyStore_DeleteAllForUser_None returns (0, nil) when the user has
// no API keys. Idempotent by design.
func TestPGAPIKeyStore_DeleteAllForUser_None(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	ks := pg.NewAPIKeyStore(s)
	deleted, err := ks.DeleteAllForUser(ctx, fmt.Sprintf("ghost-%d@x.test", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("DeleteAllForUser: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 for unknown user", deleted)
	}
}
