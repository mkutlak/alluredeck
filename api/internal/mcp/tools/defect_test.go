package tools_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// pagedDefectStore pages ListByProject the way the pg implementation does
// (offset = (page-1)*perPage) and records the filter it was handed;
// MemDefectStore's own ListByProject always answers empty.
type pagedDefectStore struct {
	*testutil.MemDefectStore
	rows   []store.DefectListRow
	filter store.DefectFilter
}

var _ store.DefectStorer = (*pagedDefectStore)(nil)

func (s *pagedDefectStore) ListByProject(_ context.Context, _ int64, f store.DefectFilter) ([]store.DefectListRow, int, error) {
	s.filter = f
	start := min((f.Page-1)*f.PerPage, len(s.rows))
	return s.rows[start:min(start+f.PerPage, len(s.rows))], len(s.rows), nil
}

func TestGetDefectCluster(t *testing.T) {
	defects := testutil.NewMemDefectStore()
	defects.Seed(store.DefectFingerprint{
		ID: "uuid-1", ProjectID: 1, FingerprintHash: "abc123hash", NormalizedMessage: "connection refused",
		Category: store.DefectCategoryInfrastructure, Resolution: store.DefectResolutionOpen,
		OccurrenceCount: 5, FirstSeenBuildID: 1, LastSeenBuildID: 5,
	})
	cs := setupTestServer(t, &bootstrap.Stores{Defect: defects})

	out, _ := call[tools.GetDefectClusterOutput](t, cs, "get_defect_cluster", map[string]any{"project_id": 1, "fingerprint_hash": "abc123hash"})
	want := tools.GetDefectClusterOutput{
		ID: "uuid-1", FingerprintHash: "abc123hash", NormalizedMessage: "connection refused",
		Category: store.DefectCategoryInfrastructure, Resolution: store.DefectResolutionOpen,
		OccurrenceCount: 5, FirstSeenBuildID: 1, LastSeenBuildID: 5,
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("output = %+v, want %+v", out, want)
	}
	// The lookup is keyed by project, so another project's hash is not found.
	for _, args := range []map[string]any{{"project_id": 1, "fingerprint_hash": ""}, {"project_id": 0, "fingerprint_hash": "abc"}, {"project_id": 2, "fingerprint_hash": "abc123hash"}} {
		callErr(t, cs, "get_defect_cluster", args)
	}
}

func TestListDefects(t *testing.T) {
	defects := &pagedDefectStore{MemDefectStore: testutil.NewMemDefectStore(), rows: []store.DefectListRow{
		{DefectFingerprint: store.DefectFingerprint{ID: "d1", FingerprintHash: "h1", Category: store.DefectCategoryProductBug,
			Resolution: store.DefectResolutionOpen, OccurrenceCount: 3, LastSeenBuildID: 9}},
	}}
	cs := setupTestServer(t, &bootstrap.Stores{Defect: defects})

	out, _ := call[tools.ListDefectsOutput](t, cs, "list_defects",
		map[string]any{"project_id": 1, "limit": 10, "category": "product_bug", "resolution": "open"})
	want := tools.ListDefectsOutput{Total: 1, Items: []tools.DefectItem{{ID: "d1", FingerprintHash: "h1",
		Category: store.DefectCategoryProductBug, Resolution: store.DefectResolutionOpen, OccurrenceCount: 3, LastSeenBuildID: 9}}}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("output = %+v, want %+v", out, want)
	}
	if f := defects.filter; f.Category != "product_bug" || f.Resolution != "open" || f.Page != 1 || f.PerPage != 10 {
		t.Errorf("filter = %+v, want category=product_bug resolution=open page=1 per_page=10", f)
	}
	callErr(t, cs, "list_defects", map[string]any{"project_id": 0})
}
