package pg_test

import (
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestDefectStore_Regressions: MarkRegressions flips is_regression on the
// build's defect_occurrences row (an empty slice is a no-op), and the flag
// surfaces through ListRegressionsForBuild (project-scoped), ListRegressionsSince
// (grouped by project, cut off by time), and ListByBuild — while ListByProject,
// having no build scope, reports false.
func TestDefectStore_Regressions(t *testing.T) {
	f := newFixture(t)
	ds := pg.NewDefectStore(f.s)
	b := f.build(1)
	fp := store.DefectFingerprint{
		FingerprintHash: unique("hash"), NormalizedMessage: "connection refused", SampleTrace: "trace",
		Category: store.DefectCategoryProductBug, OccurrenceCount: 1,
	}
	if err := ds.UpsertFingerprints(f.ctx, f.id, b, []store.DefectFingerprint{fp}); err != nil {
		t.Fatalf("UpsertFingerprints: %v", err)
	}
	got, err := ds.GetByHash(f.ctx, f.id, fp.FingerprintHash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	fpID := got.ID
	// LinkTestResults does not check that the ids exist; a placeholder id
	// stands up the occurrence row for this build.
	if err := ds.LinkTestResults(f.ctx, fpID, b, []int64{1}); err != nil {
		t.Fatalf("LinkTestResults: %v", err)
	}
	isRegression := func(list func() ([]store.DefectListRow, int, error)) bool {
		t.Helper()
		rows, _, err := list()
		if err != nil || len(rows) != 1 {
			t.Fatalf("list defects = %+v, %v; want one row", rows, err)
		}
		return rows[0].IsRegression
	}
	byBuild := func() ([]store.DefectListRow, int, error) {
		return ds.ListByBuild(f.ctx, f.id, b, store.DefectFilter{})
	}
	regressions := func(projectID int64) []store.DefectRegression {
		t.Helper()
		rs, err := ds.ListRegressionsForBuild(f.ctx, projectID, b)
		if err != nil {
			t.Fatalf("ListRegressionsForBuild: %v", err)
		}
		return rs
	}

	if err := ds.MarkRegressions(f.ctx, b, nil); err != nil {
		t.Fatalf("MarkRegressions(nil): %v", err)
	}
	if rs := regressions(f.id); len(rs) != 0 {
		t.Fatalf("regressions before marking = %+v, want none", rs)
	}
	if isRegression(byBuild) {
		t.Fatal("ListByBuild IsRegression = true before MarkRegressions")
	}
	if err := ds.MarkRegressions(f.ctx, b, []string{fpID}); err != nil {
		t.Fatalf("MarkRegressions: %v", err)
	}

	rs := regressions(f.id)
	if len(rs) != 1 || rs[0].ID != fpID || rs[0].NormalizedMessage != "connection refused" ||
		rs[0].Category != store.DefectCategoryProductBug || rs[0].OccurrenceCount != 1 || rs[0].BuildOrder != 1 {
		t.Errorf("ListRegressionsForBuild = %+v, want the marked fingerprint at build order 1", rs)
	}
	if rs := regressions(f.id + 999999); len(rs) != 0 {
		t.Errorf("another project sees %d regressions, want 0", len(rs))
	}
	if !isRegression(byBuild) {
		t.Error("ListByBuild IsRegression = false after MarkRegressions")
	}
	if isRegression(func() ([]store.DefectListRow, int, error) { return ds.ListByProject(f.ctx, f.id, store.DefectFilter{}) }) {
		t.Error("ListByProject IsRegression = true, want false (no build scope)")
	}

	for _, tt := range []struct {
		since time.Time
		want  bool
	}{{time.Now().Add(-time.Hour), true}, {time.Now().Add(time.Hour), false}} {
		grouped, err := ds.ListRegressionsSince(f.ctx, tt.since)
		if err != nil {
			t.Fatalf("ListRegressionsSince: %v", err)
		}
		found := false
		for _, g := range grouped {
			if g.ProjectID == f.id {
				found = true
				if g.Slug != f.project.Slug || len(g.Regressions) != 1 || g.Regressions[0].ID != fpID {
					t.Errorf("project group = %+v, want slug %q with only %s", g, f.project.Slug, fpID)
				}
			}
		}
		if found != tt.want {
			t.Errorf("ListRegressionsSince(%v): project present = %v, want %v", tt.since, found, tt.want)
		}
	}
}
