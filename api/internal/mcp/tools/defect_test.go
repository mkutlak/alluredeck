package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func buildStoresDefect(mocks *testutil.MockStores) *bootstrap.Stores {
	return &bootstrap.Stores{
		Defect: mocks.Defects,
	}
}

func decodeGetDefectCluster(t *testing.T, res *mcpsdk.CallToolResult) tools.GetDefectClusterOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.GetDefectClusterOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal GetDefectClusterOutput: %v", err)
	}
	return out
}

func decodeListDefects(t *testing.T, res *mcpsdk.CallToolResult) tools.ListDefectsOutput {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out tools.ListDefectsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListDefectsOutput: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// get_defect_cluster
// ---------------------------------------------------------------------------

func TestGetDefectCluster_HappyPath(t *testing.T) {
	fp := &store.DefectFingerprint{
		ID:                "uuid-1",
		ProjectID:         1,
		FingerprintHash:   "abc123hash",
		NormalizedMessage: "connection refused",
		Category:          store.DefectCategoryInfrastructure,
		Resolution:        store.DefectResolutionOpen,
		OccurrenceCount:   5,
		FirstSeenBuildID:  1,
		LastSeenBuildID:   5,
	}

	// Use a stub store that returns the fingerprint directly via GetByHash.
	ds := &stubDefectStore{fp: fp}
	stores := &bootstrap.Stores{Defect: ds}

	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_defect_cluster",
		Arguments: map[string]any{"project_id": 1, "fingerprint_hash": "abc123hash"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeGetDefectCluster(t, res)
	if out.FingerprintHash != "abc123hash" {
		t.Errorf("want fingerprint_hash=abc123hash, got %q", out.FingerprintHash)
	}
	if out.Category != store.DefectCategoryInfrastructure {
		t.Errorf("want category=infrastructure, got %q", out.Category)
	}
	if out.OccurrenceCount != 5 {
		t.Errorf("want occurrence_count=5, got %d", out.OccurrenceCount)
	}
}

func TestGetDefectCluster_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDefect(mocks))
	ctx := context.Background()

	// Missing fingerprint_hash.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_defect_cluster",
		Arguments: map[string]any{"project_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for empty fingerprint_hash")
	}

	// Missing project_id.
	res2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_defect_cluster",
		Arguments: map[string]any{"fingerprint_hash": "abc"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res2.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

// ---------------------------------------------------------------------------
// list_defects
// ---------------------------------------------------------------------------

func TestListDefects_HappyPath(t *testing.T) {
	rows := []store.DefectListRow{
		{DefectFingerprint: store.DefectFingerprint{ID: "d1", FingerprintHash: "h1", Category: store.DefectCategoryProductBug, Resolution: store.DefectResolutionOpen, OccurrenceCount: 3}},
		{DefectFingerprint: store.DefectFingerprint{ID: "d2", FingerprintHash: "h2", Category: store.DefectCategoryTestBug, Resolution: store.DefectResolutionFixed, OccurrenceCount: 1}},
	}

	ds := &stubDefectStore{listRows: rows}
	stores := &bootstrap.Stores{Defect: ds}

	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_defects",
		Arguments: map[string]any{"project_id": 1, "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}

	out := decodeListDefects(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.Items[0].FingerprintHash != "h1" {
		t.Errorf("want fingerprint_hash=h1, got %q", out.Items[0].FingerprintHash)
	}
}

func TestListDefects_InvalidInput(t *testing.T) {
	mocks := testutil.New()
	cs := setupTestServer(t, buildStoresDefect(mocks))
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_defects",
		Arguments: map[string]any{"project_id": 0},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("want IsError=true for project_id=0")
	}
}

func TestListDefects_Pagination(t *testing.T) {
	all := []store.DefectListRow{
		{DefectFingerprint: store.DefectFingerprint{ID: "d1"}},
		{DefectFingerprint: store.DefectFingerprint{ID: "d2"}},
		{DefectFingerprint: store.DefectFingerprint{ID: "d3"}},
	}
	ds := &stubDefectStore{listRows: all, pagedList: true}
	stores := &bootstrap.Stores{Defect: ds}

	cs := setupTestServer(t, stores)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "list_defects",
		Arguments: map[string]any{"project_id": 1, "limit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	out := decodeListDefects(t, res)
	if len(out.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(out.Items))
	}
	if out.NextCursor == "" {
		t.Error("want non-empty next_cursor")
	}
}

// ---------------------------------------------------------------------------
// stubDefectStore — minimal in-test DefectStorer
// ---------------------------------------------------------------------------

// stubDefectStore satisfies store.DefectStorer with controllable behaviour.
type stubDefectStore struct {
	fp        *store.DefectFingerprint
	listRows  []store.DefectListRow
	pagedList bool // when true, slice by filter.Page/PerPage
}

func (s *stubDefectStore) UpsertFingerprints(_ context.Context, _ int64, _ int64, _ []store.DefectFingerprint) error {
	return nil
}
func (s *stubDefectStore) LinkTestResults(_ context.Context, _ string, _ int64, _ []int64) error {
	return nil
}
func (s *stubDefectStore) UpdateCleanBuildCounts(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (s *stubDefectStore) AutoResolveFixed(_ context.Context, _ int64, _ int) (int, error) {
	return 0, nil
}
func (s *stubDefectStore) DetectRegressions(_ context.Context, _ int64, _ int64) ([]string, error) {
	return nil, nil
}
func (s *stubDefectStore) GetByHash(_ context.Context, _ int64, _ string) (*store.DefectFingerprint, error) {
	if s.fp == nil {
		return nil, store.ErrDefectNotFound
	}
	cp := *s.fp
	return &cp, nil
}
func (s *stubDefectStore) ListByProject(_ context.Context, _ int64, f store.DefectFilter) ([]store.DefectListRow, int, error) {
	if !s.pagedList {
		return s.listRows, len(s.listRows), nil
	}
	// Paginate: perPage includes the +1 has-more sentinel.
	start := (f.Page - 1) * (f.PerPage - 1)
	if start >= len(s.listRows) {
		return nil, len(s.listRows), nil
	}
	end := min(start+f.PerPage, len(s.listRows))
	return s.listRows[start:end], len(s.listRows), nil
}
func (s *stubDefectStore) ListByBuild(_ context.Context, _ int64, _ int64, _ store.DefectFilter) ([]store.DefectListRow, int, error) {
	return nil, 0, nil
}
func (s *stubDefectStore) GetByID(_ context.Context, _ string) (*store.DefectFingerprint, error) {
	if s.fp == nil {
		return nil, store.ErrDefectNotFound
	}
	cp := *s.fp
	return &cp, nil
}
func (s *stubDefectStore) GetTestResults(_ context.Context, _ string, _ *int64, _, _ int) ([]store.TestResult, int, error) {
	return nil, 0, nil
}
func (s *stubDefectStore) GetProjectSummary(_ context.Context, _ int64) (*store.DefectProjectSummary, error) {
	return &store.DefectProjectSummary{ByCategory: map[string]int{}}, nil
}
func (s *stubDefectStore) GetBuildSummary(_ context.Context, _ int64, _ int64) (*store.DefectBuildSummary, error) {
	return &store.DefectBuildSummary{ByCategory: map[string]int{}, ByResolution: map[string]int{}}, nil
}
func (s *stubDefectStore) UpdateDefect(_ context.Context, _ string, _, _ *string, _ *int64) error {
	return nil
}
func (s *stubDefectStore) BulkUpdate(_ context.Context, _ []string, _, _ *string) error {
	return nil
}
