package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface check.
var _ store.DefectStorer = (*MemDefectStore)(nil)

// MemDefectStore is a thread-safe in-memory DefectStorer for tests.
// All methods return sensible zero values (empty slices, zero counts, nil errors).
// GetByID and GetByHash return store.ErrDefectNotFound for unknown entries.
type MemDefectStore struct {
	mu           sync.RWMutex
	fingerprints map[string]*store.DefectFingerprint // keyed by ID
	byHash       map[string]string                   // "projectID\x00hash" -> ID
}

// NewMemDefectStore returns an initialised MemDefectStore.
func NewMemDefectStore() *MemDefectStore {
	return &MemDefectStore{
		fingerprints: make(map[string]*store.DefectFingerprint),
		byHash:       make(map[string]string),
	}
}

func (m *MemDefectStore) UpsertFingerprints(_ context.Context, _ string, _ int64, _ []store.DefectFingerprint) error {
	return nil
}

func (m *MemDefectStore) LinkTestResults(_ context.Context, _ string, _ int64, _ []int64) error {
	return nil
}

func (m *MemDefectStore) UpdateCleanBuildCounts(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *MemDefectStore) AutoResolveFixed(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (m *MemDefectStore) DetectRegressions(_ context.Context, _ string, _ int64) ([]string, error) {
	return []string{}, nil
}

func (m *MemDefectStore) GetByHash(_ context.Context, projectID, hash string) (*store.DefectFingerprint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := projectID + "\x00" + hash
	id, ok := m.byHash[key]
	if !ok {
		return nil, fmt.Errorf("%w: hash=%s", store.ErrDefectNotFound, hash)
	}
	fp := m.fingerprints[id]
	cp := *fp
	return &cp, nil
}

func (m *MemDefectStore) ListByProject(_ context.Context, _ string, _ store.DefectFilter) ([]store.DefectListRow, int, error) {
	return []store.DefectListRow{}, 0, nil
}

func (m *MemDefectStore) ListByBuild(_ context.Context, _ string, _ int64, _ store.DefectFilter) ([]store.DefectListRow, int, error) {
	return []store.DefectListRow{}, 0, nil
}

func (m *MemDefectStore) GetByID(_ context.Context, defectID string) (*store.DefectFingerprint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	fp, ok := m.fingerprints[defectID]
	if !ok {
		return nil, fmt.Errorf("%w: id=%s", store.ErrDefectNotFound, defectID)
	}
	cp := *fp
	return &cp, nil
}

func (m *MemDefectStore) GetTestResults(_ context.Context, _ string, _ *int64, _, _ int) ([]store.TestResult, int, error) {
	return []store.TestResult{}, 0, nil
}

func (m *MemDefectStore) GetProjectSummary(_ context.Context, _ string) (*store.DefectProjectSummary, error) {
	return &store.DefectProjectSummary{
		ByCategory: map[string]int{},
	}, nil
}

func (m *MemDefectStore) GetBuildSummary(_ context.Context, _ string, _ int64) (*store.DefectBuildSummary, error) {
	return &store.DefectBuildSummary{
		ByCategory:   map[string]int{},
		ByResolution: map[string]int{},
	}, nil
}

func (m *MemDefectStore) UpdateDefect(_ context.Context, _ string, _, _ *string, _ *int64) error {
	return nil
}

func (m *MemDefectStore) BulkUpdate(_ context.Context, _ []string, _, _ *string) error {
	return nil
}
