package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// MemRefreshTokenFamilyStore is an in-memory RefreshTokenFamilyStorer for tests.
// Behavior mirrors the PostgreSQL implementation: GetByID returns (nil, nil) for
// not-found, status mutations return ErrRefreshFamilyNotFound, Rotate is atomic
// under the mutex.
type MemRefreshTokenFamilyStore struct {
	mu       sync.Mutex
	families map[string]store.RefreshTokenFamily
}

// NewMemRefreshTokenFamilyStore returns a ready-to-use empty store.
func NewMemRefreshTokenFamilyStore() *MemRefreshTokenFamilyStore {
	return &MemRefreshTokenFamilyStore{families: make(map[string]store.RefreshTokenFamily)}
}

func (m *MemRefreshTokenFamilyStore) Create(_ context.Context, f store.RefreshTokenFamily) error {
	if f.FamilyID == "" {
		return fmt.Errorf("create refresh token family: family_id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if f.Status == "" {
		f.Status = store.RefreshTokenFamilyStatusActive
	}
	if f.Provider == "" {
		f.Provider = "local"
	}
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	m.families[f.FamilyID] = f
	return nil
}

func (m *MemRefreshTokenFamilyStore) GetByID(_ context.Context, familyID string) (*store.RefreshTokenFamily, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.families[familyID]
	if !ok {
		return nil, nil
	}
	cp := f
	return &cp, nil
}

func (m *MemRefreshTokenFamilyStore) Rotate(_ context.Context, familyID, newJTI string, graceSeconds int) error {
	if graceSeconds < 0 {
		graceSeconds = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.families[familyID]
	if !ok {
		return fmt.Errorf("%w: family_id=%s", store.ErrRefreshFamilyNotFound, familyID)
	}
	prev := f.CurrentJTI
	graceUntil := time.Now().UTC().Add(time.Duration(graceSeconds) * time.Second)
	f.PreviousJTI = &prev
	f.GraceUntil = &graceUntil
	f.CurrentJTI = newJTI
	f.UpdatedAt = time.Now().UTC()
	m.families[familyID] = f
	return nil
}

func (m *MemRefreshTokenFamilyStore) MarkCompromised(_ context.Context, familyID string) error {
	return m.setStatus(familyID, store.RefreshTokenFamilyStatusCompromised)
}

func (m *MemRefreshTokenFamilyStore) Revoke(_ context.Context, familyID string) error {
	return m.setStatus(familyID, store.RefreshTokenFamilyStatusRevoked)
}

func (m *MemRefreshTokenFamilyStore) setStatus(familyID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.families[familyID]
	if !ok {
		return fmt.Errorf("%w: family_id=%s", store.ErrRefreshFamilyNotFound, familyID)
	}
	f.Status = status
	f.UpdatedAt = time.Now().UTC()
	m.families[familyID] = f
	return nil
}

func (m *MemRefreshTokenFamilyStore) DeleteExpired(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	deleted := 0
	for id := range m.families {
		if m.families[id].ExpiresAt.Before(now) {
			delete(m.families, id)
			deleted++
		}
	}
	return deleted, nil
}
