//go:build integration

package pg_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// newFamilyID returns a random v4-shaped UUID (no google/uuid dependency).
func newFamilyID(t *testing.T) string {
	t.Helper()
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	h := hex.EncodeToString(buf[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// familyFixture is a refresh-token family store plus a helper that creates an
// active family for userID expiring after ttl.
func familyFixture(t *testing.T) (*pg.RefreshTokenFamilyStore, func(userID string, ttl time.Duration) store.RefreshTokenFamily) {
	rs := pg.NewRefreshTokenFamilyStore(openTestStore(t))
	return rs, func(userID string, ttl time.Duration) store.RefreshTokenFamily {
		t.Helper()
		fam := store.RefreshTokenFamily{
			FamilyID: newFamilyID(t), UserID: userID, Role: "viewer", Provider: "local",
			CurrentJTI: unique("jti"), Status: store.RefreshTokenFamilyStatusActive, ExpiresAt: time.Now().UTC().Add(ttl),
		}
		if err := rs.Create(context.Background(), fam); err != nil {
			t.Fatalf("Create: %v", err)
		}
		return fam
	}
}

// TestPGRefreshTokenFamilyStore_CreateAndGetByID round-trips a family; an
// unknown id reads as (nil, nil) and an empty FamilyID is rejected.
func TestPGRefreshTokenFamilyStore_CreateAndGetByID(t *testing.T) {
	rs, create := familyFixture(t)
	ctx := context.Background()
	fam := create(unique("user"), 24*time.Hour)

	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil || got == nil {
		t.Fatalf("GetByID = %v, %v", got, err)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() || got.ExpiresAt.Sub(fam.ExpiresAt).Abs() > time.Second {
		t.Errorf("timestamps: created %v updated %v expires %v, want set and expires ~%v",
			got.CreatedAt, got.UpdatedAt, got.ExpiresAt, fam.ExpiresAt)
	}
	got.CreatedAt, got.UpdatedAt, got.ExpiresAt = fam.CreatedAt, fam.UpdatedAt, fam.ExpiresAt
	if *got != fam {
		t.Errorf("GetByID = %+v, want %+v (no previous JTI, no grace)", *got, fam)
	}

	if got, err := rs.GetByID(ctx, newFamilyID(t)); err != nil || got != nil {
		t.Errorf("GetByID(unknown) = %+v, %v; want nil, nil", got, err)
	}
	fam.FamilyID = ""
	if err := rs.Create(ctx, fam); err == nil {
		t.Error("Create with an empty FamilyID: want error, got nil")
	}
}

// TestPGRefreshTokenFamilyStore_Rotate moves the current JTI to previous and
// opens a grace window of ~graceSeconds from now.
func TestPGRefreshTokenFamilyStore_Rotate(t *testing.T) {
	rs, create := familyFixture(t)
	ctx := context.Background()
	fam := create(unique("user"), 24*time.Hour)
	newJTI := unique("new-jti")
	before := time.Now().UTC()
	const grace = 30 * time.Second
	if err := rs.Rotate(ctx, fam.FamilyID, newJTI, int(grace/time.Second)); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil || got == nil {
		t.Fatalf("GetByID after rotate = %v, %v", got, err)
	}
	if got.CurrentJTI != newJTI || got.PreviousJTI == nil || *got.PreviousJTI != fam.CurrentJTI {
		t.Errorf("JTIs current=%q previous=%v, want %q and %q", got.CurrentJTI, got.PreviousJTI, newJTI, fam.CurrentJTI)
	}
	lo, hi := before.Add(grace-2*time.Second), time.Now().UTC().Add(grace+2*time.Second)
	if got.GraceUntil == nil || got.GraceUntil.Before(lo) || got.GraceUntil.After(hi) {
		t.Errorf("GraceUntil = %v, want within [%v, %v]", got.GraceUntil, lo, hi)
	}
	if got.UpdatedAt.Before(got.CreatedAt) {
		t.Errorf("UpdatedAt %v before CreatedAt %v", got.UpdatedAt, got.CreatedAt)
	}
}

// TestPGRefreshTokenFamilyStore_StatusTransitions: MarkCompromised and Revoke
// set their status; every mutator on an unknown family is
// ErrRefreshFamilyNotFound.
func TestPGRefreshTokenFamilyStore_StatusTransitions(t *testing.T) {
	rs, create := familyFixture(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		apply  func(id string) error
		status string // "" = no row to inspect
	}{
		{"MarkCompromised", func(id string) error { return rs.MarkCompromised(ctx, id) }, store.RefreshTokenFamilyStatusCompromised},
		{"Revoke", func(id string) error { return rs.Revoke(ctx, id) }, store.RefreshTokenFamilyStatusRevoked},
		{"Rotate", func(id string) error { return rs.Rotate(ctx, id, "new-jti", 30) }, ""},
	}
	for _, tt := range tests {
		if tt.status != "" {
			fam := create(unique("user"), 24*time.Hour)
			if err := tt.apply(fam.FamilyID); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if got, err := rs.GetByID(ctx, fam.FamilyID); err != nil || got == nil || got.Status != tt.status {
				t.Errorf("%s: GetByID = %+v, %v; want status %q", tt.name, got, err, tt.status)
			}
		}
		if err := tt.apply(newFamilyID(t)); !errors.Is(err, store.ErrRefreshFamilyNotFound) {
			t.Errorf("%s(unknown) err = %v, want ErrRefreshFamilyNotFound", tt.name, err)
		}
	}
}

// TestPGRefreshTokenFamilyStore_RevokeAllForUser (F-2): only the user's
// families transition, only active ones are counted, other users' families
// survive, and a repeat (no active families left) is (0, nil).
func TestPGRefreshTokenFamilyStore_RevokeAllForUser(t *testing.T) {
	rs, create := familyFixture(t)
	ctx := context.Background()
	user, other := unique("user"), unique("other")
	active := []store.RefreshTokenFamily{create(user, 24*time.Hour), create(user, 24*time.Hour)}
	if err := rs.Revoke(ctx, create(user, 24*time.Hour).FamilyID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	otherFam := create(other, 24*time.Hour)

	for _, want := range []int{2, 0} {
		if n, err := rs.RevokeAllForUser(ctx, user); err != nil || n != want {
			t.Errorf("RevokeAllForUser = %d, %v; want %d", n, err, want)
		}
	}
	for fam, want := range map[string]string{
		active[0].FamilyID: store.RefreshTokenFamilyStatusRevoked,
		active[1].FamilyID: store.RefreshTokenFamilyStatusRevoked,
		otherFam.FamilyID:  store.RefreshTokenFamilyStatusActive,
	} {
		if got, err := rs.GetByID(ctx, fam); err != nil || got == nil || got.Status != want {
			t.Errorf("family %s = %+v, %v; want status %q", fam, got, err, want)
		}
	}
}

// TestPGRefreshTokenFamilyStore_DeleteExpired deletes expired families and
// keeps unexpired ones.
func TestPGRefreshTokenFamilyStore_DeleteExpired(t *testing.T) {
	rs, create := familyFixture(t)
	ctx := context.Background()
	expired, fresh := create(unique("user"), -time.Hour), create(unique("user"), time.Hour)
	if n, err := rs.DeleteExpired(ctx); err != nil || n < 1 {
		t.Errorf("DeleteExpired = %d, %v; want >= 1", n, err)
	}
	if got, err := rs.GetByID(ctx, expired.FamilyID); err != nil || got != nil {
		t.Errorf("expired family = %+v, %v; want deleted", got, err)
	}
	if got, err := rs.GetByID(ctx, fresh.FamilyID); err != nil || got == nil {
		t.Errorf("fresh family = %+v, %v; want it to survive", got, err)
	}
}
