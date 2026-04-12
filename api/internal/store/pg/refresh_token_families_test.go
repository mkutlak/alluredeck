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

// newFamilyID returns a random 32-char hex string padded into a UUID shape so
// every test run uses a unique primary key without introducing a new
// dependency on google/uuid.
func newFamilyID(t *testing.T) string {
	t.Helper()
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	// Format as a v4-shaped UUID so the PG UUID type accepts it.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	h := hex.EncodeToString(buf[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func newTestFamily(t *testing.T) store.RefreshTokenFamily {
	t.Helper()
	now := time.Now().UTC()
	return store.RefreshTokenFamily{
		FamilyID:   newFamilyID(t),
		UserID:     fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Role:       "viewer",
		Provider:   "local",
		CurrentJTI: fmt.Sprintf("jti-%d", time.Now().UnixNano()),
		Status:     store.RefreshTokenFamilyStatusActive,
		ExpiresAt:  now.Add(24 * time.Hour),
	}
}

func isRefreshFamilyNotFound(err error) bool {
	return errors.Is(err, store.ErrRefreshFamilyNotFound)
}

// ---------------------------------------------------------------------------
// Create + GetByID
// ---------------------------------------------------------------------------

func TestPGRefreshTokenFamilyStore_CreateAndGetByID(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	fam := newTestFamily(t)

	if err := rs.Create(ctx, fam); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil family after create")
	}
	if got.FamilyID != fam.FamilyID {
		t.Errorf("FamilyID = %q, want %q", got.FamilyID, fam.FamilyID)
	}
	if got.UserID != fam.UserID {
		t.Errorf("UserID = %q, want %q", got.UserID, fam.UserID)
	}
	if got.Role != fam.Role {
		t.Errorf("Role = %q, want %q", got.Role, fam.Role)
	}
	if got.Provider != fam.Provider {
		t.Errorf("Provider = %q, want %q", got.Provider, fam.Provider)
	}
	if got.CurrentJTI != fam.CurrentJTI {
		t.Errorf("CurrentJTI = %q, want %q", got.CurrentJTI, fam.CurrentJTI)
	}
	if got.PreviousJTI != nil {
		t.Errorf("PreviousJTI = %v, want nil", got.PreviousJTI)
	}
	if got.GraceUntil != nil {
		t.Errorf("GraceUntil = %v, want nil", got.GraceUntil)
	}
	if got.Status != store.RefreshTokenFamilyStatusActive {
		t.Errorf("Status = %q, want %q", got.Status, store.RefreshTokenFamilyStatusActive)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
	if got.ExpiresAt.Sub(fam.ExpiresAt).Abs() > time.Second {
		t.Errorf("ExpiresAt drift = %v, want <= 1s", got.ExpiresAt.Sub(fam.ExpiresAt))
	}
}

func TestPGRefreshTokenFamilyStore_GetByID_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	got, err := rs.GetByID(ctx, newFamilyID(t))
	if err != nil {
		t.Fatalf("GetByID: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("expected nil family for missing ID, got %+v", got)
	}
}

func TestPGRefreshTokenFamilyStore_Create_RejectsEmptyFamilyID(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	fam := newTestFamily(t)
	fam.FamilyID = ""

	if err := rs.Create(ctx, fam); err == nil {
		t.Fatal("expected error for empty FamilyID, got nil")
	}
}

// ---------------------------------------------------------------------------
// Rotate
// ---------------------------------------------------------------------------

func TestPGRefreshTokenFamilyStore_Rotate(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	fam := newTestFamily(t)
	if err := rs.Create(ctx, fam); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newJTI := fmt.Sprintf("new-jti-%d", time.Now().UnixNano())
	before := time.Now().UTC()
	const graceSeconds = 30
	if err := rs.Rotate(ctx, fam.FamilyID, newJTI, graceSeconds); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil {
		t.Fatalf("GetByID after rotate: %v", err)
	}
	if got == nil {
		t.Fatal("expected family after rotate")
	}
	if got.CurrentJTI != newJTI {
		t.Errorf("CurrentJTI = %q, want %q", got.CurrentJTI, newJTI)
	}
	if got.PreviousJTI == nil || *got.PreviousJTI != fam.CurrentJTI {
		t.Errorf("PreviousJTI = %v, want %q", got.PreviousJTI, fam.CurrentJTI)
	}
	if got.GraceUntil == nil {
		t.Fatal("expected GraceUntil to be set after rotate")
	}
	// grace_until must be strictly in the future relative to the pre-rotate
	// timestamp and roughly equal to now + graceSeconds.
	minGrace := before.Add(graceSeconds * time.Second)
	if got.GraceUntil.Before(minGrace.Add(-2 * time.Second)) {
		t.Errorf("GraceUntil = %v, want >= %v", got.GraceUntil, minGrace.Add(-2*time.Second))
	}
	maxGrace := time.Now().UTC().Add(graceSeconds * time.Second).Add(2 * time.Second)
	if got.GraceUntil.After(maxGrace) {
		t.Errorf("GraceUntil = %v, want <= %v", got.GraceUntil, maxGrace)
	}
	if !got.UpdatedAt.After(fam.CreatedAt.Add(-time.Second)) {
		t.Errorf("UpdatedAt = %v, want >= %v", got.UpdatedAt, fam.CreatedAt)
	}
}

func TestPGRefreshTokenFamilyStore_Rotate_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	err := rs.Rotate(ctx, newFamilyID(t), "new-jti", 30)
	if !isRefreshFamilyNotFound(err) {
		t.Errorf("expected ErrRefreshFamilyNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// MarkCompromised / Revoke
// ---------------------------------------------------------------------------

func TestPGRefreshTokenFamilyStore_MarkCompromised(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	fam := newTestFamily(t)
	if err := rs.Create(ctx, fam); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := rs.MarkCompromised(ctx, fam.FamilyID); err != nil {
		t.Fatalf("MarkCompromised: %v", err)
	}

	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != store.RefreshTokenFamilyStatusCompromised {
		t.Errorf("Status = %q, want %q", got.Status, store.RefreshTokenFamilyStatusCompromised)
	}
}

func TestPGRefreshTokenFamilyStore_MarkCompromised_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	err := rs.MarkCompromised(ctx, newFamilyID(t))
	if !isRefreshFamilyNotFound(err) {
		t.Errorf("expected ErrRefreshFamilyNotFound, got %v", err)
	}
}

func TestPGRefreshTokenFamilyStore_Revoke(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	fam := newTestFamily(t)
	if err := rs.Create(ctx, fam); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := rs.Revoke(ctx, fam.FamilyID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	got, err := rs.GetByID(ctx, fam.FamilyID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != store.RefreshTokenFamilyStatusRevoked {
		t.Errorf("Status = %q, want %q", got.Status, store.RefreshTokenFamilyStatusRevoked)
	}
}

func TestPGRefreshTokenFamilyStore_Revoke_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)
	err := rs.Revoke(ctx, newFamilyID(t))
	if !isRefreshFamilyNotFound(err) {
		t.Errorf("expected ErrRefreshFamilyNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteExpired
// ---------------------------------------------------------------------------

func TestPGRefreshTokenFamilyStore_DeleteExpired(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	rs := pg.NewRefreshTokenFamilyStore(s)

	// Row that is already expired — must be deleted.
	expired := newTestFamily(t)
	expired.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	if err := rs.Create(ctx, expired); err != nil {
		t.Fatalf("Create expired: %v", err)
	}

	// Row that has not yet expired — must survive.
	fresh := newTestFamily(t)
	fresh.ExpiresAt = time.Now().UTC().Add(time.Hour)
	if err := rs.Create(ctx, fresh); err != nil {
		t.Fatalf("Create fresh: %v", err)
	}

	n, err := rs.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n < 1 {
		t.Errorf("DeleteExpired count = %d, want >= 1", n)
	}

	gone, err := rs.GetByID(ctx, expired.FamilyID)
	if err != nil {
		t.Fatalf("GetByID expired: %v", err)
	}
	if gone != nil {
		t.Errorf("expected expired family to be deleted, got %+v", gone)
	}

	still, err := rs.GetByID(ctx, fresh.FamilyID)
	if err != nil {
		t.Fatalf("GetByID fresh: %v", err)
	}
	if still == nil {
		t.Error("expected fresh family to survive DeleteExpired")
	}
}
