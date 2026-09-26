//go:build integration

package pg_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

func testEmail(local string) string { return unique(local) + "@pgtest.local" }

// TestPGUserStore_CreateLocal: a new local user is active with its hash, and
// its email is unique case-insensitively — also against a deactivated row
// (migration 0039, idx_users_email_global), so re-onboarding reactivates the
// existing row instead of stealing the email.
func TestPGUserStore_CreateLocal(t *testing.T) {
	us := pg.NewUserStore(openTestStore(t))
	ctx := context.Background()
	email := testEmail("CaseMix")
	u, err := us.CreateLocal(ctx, email, "Alice", "hash-a", "viewer")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if u.ID == 0 || u.Provider != "local" || !u.IsActive || u.PasswordHash != "hash-a" {
		t.Errorf("CreateLocal = %+v, want an active local user with hash-a", u)
	}
	if got, err := us.GetByEmail(ctx, strings.ToUpper(email)); err != nil || got == nil || got.ID != u.ID {
		t.Errorf("GetByEmail(upper-cased) = %+v, %v; want user %d", got, err, u.ID)
	}
	if _, err := us.CreateLocal(ctx, email, "Alice2", "hash-b", "viewer"); !errors.Is(err, store.ErrDuplicateEntry) {
		t.Errorf("duplicate CreateLocal err = %v, want ErrDuplicateEntry", err)
	}
	if err := us.UpdateActive(ctx, u.ID, false); err != nil {
		t.Fatalf("UpdateActive: %v", err)
	}
	if _, err := us.CreateLocal(ctx, email, "Replacement", "hash-c", "viewer"); !errors.Is(err, store.ErrDuplicateEntry) {
		t.Errorf("CreateLocal after deactivation err = %v, want ErrDuplicateEntry", err)
	}
}

// TestPGUserStore_Updates: each update is visible through GetByID, and an
// unknown id is ErrUserNotFound.
func TestPGUserStore_Updates(t *testing.T) {
	us := pg.NewUserStore(openTestStore(t))
	ctx := context.Background()
	const missing = 9999999
	sub := unique("relink")
	tests := []struct {
		name   string
		update func(id int64) error
		ok     func(u *store.User) bool
	}{
		{"UpdateRole", func(id int64) error { return us.UpdateRole(ctx, id, "editor") },
			func(u *store.User) bool { return u.Role == "editor" }},
		{"UpdateActive", func(id int64) error { return us.UpdateActive(ctx, id, false) },
			func(u *store.User) bool { return !u.IsActive }},
		{"UpdateProfile", func(id int64) error { return us.UpdateProfile(ctx, id, "New Name") },
			func(u *store.User) bool { return u.Name == "New Name" }},
		{"UpdatePasswordHash", func(id int64) error { return us.UpdatePasswordHash(ctx, id, "hash-rotated") },
			func(u *store.User) bool { return u.PasswordHash == "hash-rotated" }},
		{"RelinkOIDC", func(id int64) error { return us.RelinkOIDC(ctx, id, "keycloak", "kc|"+sub) },
			func(u *store.User) bool { return u.Provider == "keycloak" && u.ProviderSub == "kc|"+sub }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := us.CreateLocal(ctx, testEmail("upd"), "Old Name", "hash", "viewer")
			if err != nil {
				t.Fatalf("CreateLocal: %v", err)
			}
			if err := tt.update(u.ID); err != nil {
				t.Fatalf("update: %v", err)
			}
			if got, err := us.GetByID(ctx, u.ID); err != nil || !tt.ok(got) {
				t.Errorf("GetByID after update = %+v, %v", got, err)
			}
			if err := tt.update(missing); !errors.Is(err, store.ErrUserNotFound) {
				t.Errorf("update of a missing id err = %v, want ErrUserNotFound", err)
			}
		})
	}
}

// TestPGUserStore_ListPaginated_SearchRoleActive filters a uniquely prefixed
// population by search, role and active, and pages it.
func TestPGUserStore_ListPaginated_SearchRoleActive(t *testing.T) {
	us := pg.NewUserStore(openTestStore(t))
	ctx := context.Background()
	prefix := unique("ldp")
	for _, u := range []struct {
		name, role string
		active     bool
	}{{"Alice", "viewer", true}, {"Bob", "editor", true}, {"Carol", "viewer", false}} {
		created, err := us.CreateLocal(ctx, prefix+"-"+u.name+"@pgtest.local", prefix+" "+u.name, "hash", u.role)
		if err != nil {
			t.Fatalf("CreateLocal: %v", err)
		}
		if !u.active {
			if err := us.UpdateActive(ctx, created.ID, false); err != nil {
				t.Fatalf("UpdateActive: %v", err)
			}
		}
	}
	inactive := false
	for _, tt := range []struct {
		name   string
		params store.ListUsersParams
		total  int
		page   string // names on the page, ordered by email
	}{
		{"search", store.ListUsersParams{Limit: 10, Search: prefix}, 3, "Alice Bob Carol"},
		{"role", store.ListUsersParams{Limit: 10, Search: prefix, Role: "viewer"}, 2, "Alice Carol"},
		{"role + inactive", store.ListUsersParams{Limit: 10, Search: prefix, Role: "viewer", Active: &inactive}, 1, "Carol"},
		{"page 1", store.ListUsersParams{Limit: 1, Search: prefix}, 3, "Alice"},
		{"page 3", store.ListUsersParams{Limit: 1, Offset: 2, Search: prefix}, 3, "Carol"},
	} {
		rows, total, err := us.ListPaginated(ctx, tt.params)
		var page []string
		for _, u := range rows {
			page = append(page, strings.TrimPrefix(u.Name, prefix+" "))
		}
		if err != nil || total != tt.total || strings.Join(page, " ") != tt.page {
			t.Errorf("%s: total %d, page %q, %v; want %d, %q", tt.name, total, page, err, tt.total, tt.page)
		}
	}
}

// TestPGUserStore_UpsertByOIDC (F-5): the same (provider, sub) updates the
// row in place through its ON CONFLICT path; a different provider with the
// same email is store.ErrEmailAlreadyLinked, not a raw 23505 from the
// partial-unique index.
func TestPGUserStore_UpsertByOIDC(t *testing.T) {
	us := pg.NewUserStore(openTestStore(t))
	ctx := context.Background()
	email, sub := testEmail("carol"), unique("okta|")
	first, err := us.UpsertByOIDC(ctx, "okta", sub, email, "Carol", "viewer")
	if err != nil {
		t.Fatalf("first UpsertByOIDC: %v", err)
	}
	second, err := us.UpsertByOIDC(ctx, "okta", sub, email, "Carol Updated", "editor")
	if err != nil || second.ID != first.ID || second.Name != "Carol Updated" || second.Role != "editor" {
		t.Errorf("same-provider UpsertByOIDC = %+v, %v; want row %d updated in place", second, err, first.ID)
	}
	if _, err := us.UpsertByOIDC(ctx, "keycloak", unique("kc|"), email, "Carol K.", "viewer"); !errors.Is(err, store.ErrEmailAlreadyLinked) {
		t.Errorf("cross-provider UpsertByOIDC err = %v, want ErrEmailAlreadyLinked", err)
	}
}
