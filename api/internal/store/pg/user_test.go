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

// testEmail constructs a unique e-mail local-part-per-run so concurrent
// integration test sessions do not collide on the partial-unique index.
func testEmail(t *testing.T, local string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", local, time.Now().UnixNano()) + "@" + "pgtest.local"
}

func TestPGUserStore_CreateLocal(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	email := testEmail(t, "alice")
	u, err := us.CreateLocal(ctx, email, "Alice", "hash-a", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if u.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if u.Provider != "local" {
		t.Errorf("Provider = %q, want local", u.Provider)
	}
	if !u.IsActive {
		t.Error("expected new user to be active")
	}
	if u.PasswordHash != "hash-a" {
		t.Errorf("PasswordHash = %q, want hash-a", u.PasswordHash)
	}

	// Duplicate active email → ErrDuplicateEntry.
	if _, err := us.CreateLocal(ctx, email, "Alice2", "hash-b", "viewer"); !errors.Is(err, store.ErrDuplicateEntry) {
		t.Errorf("duplicate CreateLocal err = %v, want ErrDuplicateEntry", err)
	}
}

func TestPGUserStore_UpdateRole(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	u, err := us.CreateLocal(ctx, testEmail(t, "role"), "Role User", "hash", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := us.UpdateRole(ctx, u.ID, "editor"); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	got, err := us.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Role != "editor" {
		t.Errorf("Role = %q, want editor", got.Role)
	}

	if err := us.UpdateRole(ctx, 9999999, "admin"); !errors.Is(err, store.ErrUserNotFound) {
		t.Errorf("UpdateRole missing id err = %v, want ErrUserNotFound", err)
	}
}

func TestPGUserStore_UpdateActive(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	u, err := us.CreateLocal(ctx, testEmail(t, "active"), "Active User", "hash", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := us.UpdateActive(ctx, u.ID, false); err != nil {
		t.Fatalf("UpdateActive: %v", err)
	}
	got, err := us.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.IsActive {
		t.Error("expected is_active=false after UpdateActive")
	}
	// Deactivating freed the partial-unique slot; creating again with the
	// same email must now succeed.
	if _, err := us.CreateLocal(ctx, u.Email, "Replacement", "hash2", "viewer"); err != nil {
		t.Errorf("CreateLocal after deactivation: %v", err)
	}

	if err := us.UpdateActive(ctx, 9999999, true); !errors.Is(err, store.ErrUserNotFound) {
		t.Errorf("UpdateActive missing id err = %v, want ErrUserNotFound", err)
	}
}

func TestPGUserStore_UpdateProfile(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	u, err := us.CreateLocal(ctx, testEmail(t, "profile"), "Old Name", "hash", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := us.UpdateProfile(ctx, u.ID, "New Name"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, err := us.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want New Name", got.Name)
	}
}

func TestPGUserStore_ListPaginated_SearchRoleActive(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	// Seed a small population with a unique prefix so other tests cannot
	// influence the counts and filters.
	prefix := fmt.Sprintf("ldp%d", time.Now().UnixNano())
	emails := []struct {
		email    string
		name     string
		role     string
		active   bool
		matching bool
	}{
		{prefix + "-a" + "@" + "pgtest.local", prefix + " Alice", "viewer", true, true},
		{prefix + "-b" + "@" + "pgtest.local", prefix + " Bob", "editor", true, true},
		{prefix + "-c" + "@" + "pgtest.local", prefix + " Carol", "viewer", false, true},
	}
	for _, e := range emails {
		u, err := us.CreateLocal(ctx, e.email, e.name, "hash", e.role)
		if err != nil {
			t.Fatalf("CreateLocal %s: %v", e.email, err)
		}
		if !e.active {
			if err := us.UpdateActive(ctx, u.ID, false); err != nil {
				t.Fatalf("UpdateActive: %v", err)
			}
		}
	}

	// Search by prefix should return all three rows.
	rows, total, err := us.ListPaginated(ctx, store.ListUsersParams{Limit: 10, Offset: 0, Search: prefix})
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Errorf("search total/len = %d/%d, want 3/3", total, len(rows))
	}

	// Filter to role=viewer.
	rows, total, err = us.ListPaginated(ctx, store.ListUsersParams{Limit: 10, Search: prefix, Role: "viewer"})
	if err != nil {
		t.Fatalf("ListPaginated viewer: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Errorf("role viewer total/len = %d/%d, want 2/2", total, len(rows))
	}

	// Filter to active=false + role=viewer.
	inactive := false
	rows, total, err = us.ListPaginated(ctx, store.ListUsersParams{Limit: 10, Search: prefix, Role: "viewer", Active: &inactive})
	if err != nil {
		t.Fatalf("ListPaginated viewer inactive: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Errorf("viewer+inactive total/len = %d/%d, want 1/1", total, len(rows))
	}

	// Pagination math: limit=1 on the 3-row prefix set yields total=3 and len=1.
	rows, total, err = us.ListPaginated(ctx, store.ListUsersParams{Limit: 1, Offset: 0, Search: prefix})
	if err != nil {
		t.Fatalf("ListPaginated page1: %v", err)
	}
	if total != 3 || len(rows) != 1 {
		t.Errorf("page1 total/len = %d/%d, want 3/1", total, len(rows))
	}
	rows, total, err = us.ListPaginated(ctx, store.ListUsersParams{Limit: 1, Offset: 2, Search: prefix})
	if err != nil {
		t.Fatalf("ListPaginated page3: %v", err)
	}
	if total != 3 || len(rows) != 1 {
		t.Errorf("page3 total/len = %d/%d, want 3/1", total, len(rows))
	}
}

func TestPGUserStore_UpdatePasswordHash(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	u, err := us.CreateLocal(ctx, testEmail(t, "pwdhash"), "Pwd Hash User", "hash-original", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := us.UpdatePasswordHash(ctx, u.ID, "hash-rotated"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}
	got, err := us.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PasswordHash != "hash-rotated" {
		t.Errorf("PasswordHash = %q, want hash-rotated", got.PasswordHash)
	}

	if err := us.UpdatePasswordHash(ctx, 9999999, "hash-missing"); !errors.Is(err, store.ErrUserNotFound) {
		t.Errorf("UpdatePasswordHash missing id err = %v, want ErrUserNotFound", err)
	}
}

func TestPGUserStore_GetByEmail_CaseInsensitive(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	us := pg.NewUserStore(s)

	local := fmt.Sprintf("CaseMix-%d", time.Now().UnixNano())
	email := local + "@" + "pgtest.local"
	if _, err := us.CreateLocal(ctx, email, "Case User", "hash", "viewer"); err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	got, err := us.GetByEmail(ctx, "CASEMIX"+email[len(local):])
	if err != nil {
		t.Fatalf("GetByEmail upper: %v", err)
	}
	if got == nil || got.Email != email {
		t.Errorf("GetByEmail returned %v, want case-insensitive match on %s", got, email)
	}
}
