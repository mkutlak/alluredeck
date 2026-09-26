//go:build integration

package pg_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestPGAPIKeyStore walks one user's keys through Create, GetByHash,
// UpdateLastUsed, List/CountByUsername and the owner-scoped Delete (IDOR
// prevention), then DeleteAllForUser (F-2): every key of the user goes, other
// users' keys stay, and a repeat deletes nothing.
func TestPGAPIKeyStore(t *testing.T) {
	ks := pg.NewAPIKeyStore(openTestStore(t))
	ctx := context.Background()
	alice, bob := unique("alice")+"@x.test", unique("bob")+"@x.test"
	create := func(username string, i int) *store.APIKey {
		t.Helper()
		k, err := ks.Create(ctx, &store.APIKey{
			Name: fmt.Sprintf("key-%d", i), Prefix: "ald_a1b2c3d4", KeyHash: unique("hash"), Username: username, Role: "viewer",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return k
	}
	count := func(username string, want int) {
		t.Helper()
		keys, err := ks.ListByUsername(ctx, username)
		if err != nil {
			t.Fatalf("ListByUsername: %v", err)
		}
		n, err := ks.CountByUsername(ctx, username)
		if err != nil {
			t.Fatalf("CountByUsername: %v", err)
		}
		if len(keys) != want || n != want {
			t.Errorf("%s: ListByUsername %d keys, CountByUsername %d; want %d", username, len(keys), n, want)
		}
	}

	count(alice, 0)
	first := create(alice, 0)
	if first.ID == 0 || first.CreatedAt.IsZero() {
		t.Errorf("Create = %+v, want ID and CreatedAt set", first)
	}
	if err := ks.UpdateLastUsed(ctx, first.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	got, err := ks.GetByHash(ctx, first.KeyHash)
	if err != nil || got.ID != first.ID || got.LastUsed == nil {
		t.Errorf("GetByHash = %+v, %v; want id %d with LastUsed set", got, err, first.ID)
	}
	if _, err := ks.GetByHash(ctx, "nonexistent-hash"); !errors.Is(err, store.ErrAPIKeyNotFound) {
		t.Errorf("GetByHash(unknown) err = %v, want ErrAPIKeyNotFound", err)
	}
	create(alice, 1)
	create(alice, 2)
	count(alice, 3)

	for _, step := range []struct {
		user string
		want error
	}{{bob, store.ErrAPIKeyNotFound}, {alice, nil}, {alice, store.ErrAPIKeyNotFound}} {
		if err := ks.Delete(ctx, first.ID, step.user); !errors.Is(err, step.want) {
			t.Errorf("Delete(as %s) err = %v, want %v", step.user, err, step.want)
		}
	}
	count(alice, 2)

	create(bob, 0)
	for _, want := range []int{2, 0} {
		if n, err := ks.DeleteAllForUser(ctx, alice); err != nil || n != want {
			t.Errorf("DeleteAllForUser = %d, %v; want %d", n, err, want)
		}
	}
	count(alice, 0)
	count(bob, 1)
}
