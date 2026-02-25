package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

func TestBlacklistStore_AddAndCheck(t *testing.T) {
	s := openTestStore(t)
	bl := store.NewBlacklistStore(s)
	ctx := context.Background()

	expiry := time.Now().Add(time.Hour)
	if err := bl.AddToBlacklist(ctx, "jti-1", expiry); err != nil {
		t.Fatalf("AddToBlacklist: %v", err)
	}

	blacklisted, err := bl.IsBlacklisted(ctx, "jti-1")
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if !blacklisted {
		t.Error("expected jti-1 to be blacklisted")
	}
}

func TestBlacklistStore_ExpiredNotBlacklisted(t *testing.T) {
	s := openTestStore(t)
	bl := store.NewBlacklistStore(s)
	ctx := context.Background()

	expiry := time.Now().Add(-time.Second) // already expired
	if err := bl.AddToBlacklist(ctx, "expired-jti", expiry); err != nil {
		t.Fatalf("AddToBlacklist: %v", err)
	}

	blacklisted, err := bl.IsBlacklisted(ctx, "expired-jti")
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if blacklisted {
		t.Error("expected expired jti to not be blacklisted")
	}
}

func TestBlacklistStore_UnknownNotBlacklisted(t *testing.T) {
	s := openTestStore(t)
	bl := store.NewBlacklistStore(s)
	ctx := context.Background()

	blacklisted, err := bl.IsBlacklisted(ctx, "unknown-jti")
	if err != nil {
		t.Fatalf("IsBlacklisted: %v", err)
	}
	if blacklisted {
		t.Error("unknown jti should not be blacklisted")
	}
}

func TestBlacklistStore_PruneRemovesExpired(t *testing.T) {
	s := openTestStore(t)
	bl := store.NewBlacklistStore(s)
	ctx := context.Background()

	_ = bl.AddToBlacklist(ctx, "valid", time.Now().Add(time.Hour))
	_ = bl.AddToBlacklist(ctx, "old1", time.Now().Add(-time.Minute))
	_ = bl.AddToBlacklist(ctx, "old2", time.Now().Add(-time.Minute))

	n, err := bl.PruneExpired(ctx)
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pruned, got %d", n)
	}

	// Valid should still be there.
	blacklisted, _ := bl.IsBlacklisted(ctx, "valid")
	if !blacklisted {
		t.Error("valid jti should still be blacklisted after prune")
	}
}
