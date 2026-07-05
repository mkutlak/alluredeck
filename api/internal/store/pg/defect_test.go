package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// seedRegressionFixture creates a project, a single build, and a defect
// fingerprint with one occurrence row for that build. It returns the store
// handles plus the project/build/fingerprint identifiers needed by the
// regression tests.
func seedRegressionFixture(t *testing.T, slugPrefix string) (ds *pg.DefectStore, projectID, buildID int64, fingerprintID string) {
	t.Helper()
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)
	ds = pg.NewDefectStore(s)

	slug := fmt.Sprintf("%s-%d", slugPrefix, time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID = proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err = trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	fp := store.DefectFingerprint{
		FingerprintHash:   "hash-" + slug,
		NormalizedMessage: "connection refused",
		SampleTrace:       "trace",
		Category:          store.DefectCategoryProductBug,
		OccurrenceCount:   1,
	}
	if err := ds.UpsertFingerprints(ctx, projectID, buildID, []store.DefectFingerprint{fp}); err != nil {
		t.Fatalf("UpsertFingerprints: %v", err)
	}
	got, err := ds.GetByHash(ctx, projectID, fp.FingerprintHash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	fingerprintID = got.ID

	// LinkTestResults only requires a non-empty slice of test_result_ids to
	// create the defect_occurrences row; it does not enforce that those ids
	// exist (test_result_count is a plain counter column), so a placeholder id
	// is sufficient to stand up an occurrence row for this build.
	if err := ds.LinkTestResults(ctx, fingerprintID, buildID, []int64{1}); err != nil {
		t.Fatalf("LinkTestResults: %v", err)
	}

	return ds, projectID, buildID, fingerprintID
}

// TestMarkRegressions_SetsIsRegressionFlag verifies MarkRegressions flips
// is_regression=true on the defect_occurrences row for the given build and
// fingerprint IDs, and that passing an empty slice is a no-op.
func TestMarkRegressions_SetsIsRegressionFlag(t *testing.T) {
	ds, projectID, buildID, fingerprintID := seedRegressionFixture(t, "test-mark-regressions")
	ctx := context.Background()

	// Empty slice is a no-op: no error, flag stays false.
	if err := ds.MarkRegressions(ctx, buildID, nil); err != nil {
		t.Fatalf("MarkRegressions(nil) returned error: %v", err)
	}

	regressions, err := ds.ListRegressionsForBuild(ctx, projectID, buildID)
	if err != nil {
		t.Fatalf("ListRegressionsForBuild (pre-mark): %v", err)
	}
	if len(regressions) != 0 {
		t.Fatalf("expected no regressions before MarkRegressions, got %d", len(regressions))
	}

	if err := ds.MarkRegressions(ctx, buildID, []string{fingerprintID}); err != nil {
		t.Fatalf("MarkRegressions: %v", err)
	}

	regressionsAfter, err := ds.ListRegressionsForBuild(ctx, projectID, buildID)
	if err != nil {
		t.Fatalf("ListRegressionsForBuild (post-mark): %v", err)
	}
	if len(regressionsAfter) != 1 {
		t.Fatalf("expected 1 regression after MarkRegressions, got %d", len(regressionsAfter))
	}
}

// TestListRegressionsForBuild_ReturnsMarkedRegressions verifies that once a
// fingerprint's occurrence has been marked via MarkRegressions,
// ListRegressionsForBuild surfaces it with the expected fields.
func TestListRegressionsForBuild_ReturnsMarkedRegressions(t *testing.T) {
	ds, projectID, buildID, fingerprintID := seedRegressionFixture(t, "test-list-regressions-build")
	ctx := context.Background()

	if err := ds.MarkRegressions(ctx, buildID, []string{fingerprintID}); err != nil {
		t.Fatalf("MarkRegressions: %v", err)
	}

	regressions, err := ds.ListRegressionsForBuild(ctx, projectID, buildID)
	if err != nil {
		t.Fatalf("ListRegressionsForBuild: %v", err)
	}
	if len(regressions) != 1 {
		t.Fatalf("regressions count: got %d, want 1", len(regressions))
	}

	r := regressions[0]
	if r.ID != fingerprintID {
		t.Errorf("r.ID: got %q, want %q", r.ID, fingerprintID)
	}
	if r.NormalizedMessage != "connection refused" {
		t.Errorf("r.NormalizedMessage: got %q, want %q", r.NormalizedMessage, "connection refused")
	}
	if r.Category != store.DefectCategoryProductBug {
		t.Errorf("r.Category: got %q, want %q", r.Category, store.DefectCategoryProductBug)
	}
	if r.OccurrenceCount != 1 {
		t.Errorf("r.OccurrenceCount: got %d, want 1", r.OccurrenceCount)
	}
	if r.BuildOrder != 1 {
		t.Errorf("r.BuildOrder: got %d, want 1", r.BuildOrder)
	}

	// A different project must not see this build's regressions.
	other, err := ds.ListRegressionsForBuild(ctx, projectID+999999, buildID)
	if err != nil {
		t.Fatalf("ListRegressionsForBuild (wrong project): %v", err)
	}
	if len(other) != 0 {
		t.Errorf("expected no regressions for mismatched project, got %d", len(other))
	}
}

// TestListRegressionsSince_GroupsByProject verifies that regressions observed
// since a cutoff time are grouped by project, each carrying its slug and its
// own regressions, and that a cutoff in the future excludes everything.
func TestListRegressionsSince_GroupsByProject(t *testing.T) {
	ds, projectID, buildID, fingerprintID := seedRegressionFixture(t, "test-list-regressions-since")
	ctx := context.Background()

	if err := ds.MarkRegressions(ctx, buildID, []string{fingerprintID}); err != nil {
		t.Fatalf("MarkRegressions: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	grouped, err := ds.ListRegressionsSince(ctx, since)
	if err != nil {
		t.Fatalf("ListRegressionsSince: %v", err)
	}

	var found *store.ProjectRegressions
	for i := range grouped {
		if grouped[i].ProjectID == projectID {
			found = &grouped[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("ListRegressionsSince: no group found for project_id=%d among %d groups", projectID, len(grouped))
	}
	if len(found.Regressions) != 1 {
		t.Fatalf("found.Regressions count: got %d, want 1", len(found.Regressions))
	}
	if found.Regressions[0].ID != fingerprintID {
		t.Errorf("found.Regressions[0].ID: got %q, want %q", found.Regressions[0].ID, fingerprintID)
	}

	// A cutoff in the future must exclude this project's regression.
	future := time.Now().Add(1 * time.Hour)
	groupedFuture, err := ds.ListRegressionsSince(ctx, future)
	if err != nil {
		t.Fatalf("ListRegressionsSince (future cutoff): %v", err)
	}
	for i := range groupedFuture {
		if groupedFuture[i].ProjectID == projectID {
			t.Errorf("expected project %d absent for a future cutoff, but it was present", projectID)
		}
	}
}

// TestListByBuild_SetsIsRegression verifies that listDefects (via ListByBuild)
// populates DefectListRow.IsRegression from defect_occurrences.is_regression.
func TestListByBuild_SetsIsRegression(t *testing.T) {
	ds, projectID, buildID, fingerprintID := seedRegressionFixture(t, "test-listbuild-isregression")
	ctx := context.Background()

	// Before marking, IsRegression must be false.
	rowsBefore, _, err := ds.ListByBuild(ctx, projectID, buildID, store.DefectFilter{})
	if err != nil {
		t.Fatalf("ListByBuild (pre-mark): %v", err)
	}
	if len(rowsBefore) != 1 {
		t.Fatalf("rowsBefore count: got %d, want 1", len(rowsBefore))
	}
	if rowsBefore[0].IsRegression {
		t.Error("rowsBefore[0].IsRegression: got true, want false before MarkRegressions")
	}

	if err := ds.MarkRegressions(ctx, buildID, []string{fingerprintID}); err != nil {
		t.Fatalf("MarkRegressions: %v", err)
	}

	rowsAfter, _, err := ds.ListByBuild(ctx, projectID, buildID, store.DefectFilter{})
	if err != nil {
		t.Fatalf("ListByBuild (post-mark): %v", err)
	}
	if len(rowsAfter) != 1 {
		t.Fatalf("rowsAfter count: got %d, want 1", len(rowsAfter))
	}
	if !rowsAfter[0].IsRegression {
		t.Error("rowsAfter[0].IsRegression: got false, want true after MarkRegressions")
	}

	// ListByProject (buildID == nil path) must not error and defaults IsRegression to false.
	rowsProject, _, err := ds.ListByProject(ctx, projectID, store.DefectFilter{})
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(rowsProject) != 1 {
		t.Fatalf("rowsProject count: got %d, want 1", len(rowsProject))
	}
	if rowsProject[0].IsRegression {
		t.Error("rowsProject[0].IsRegression: got true, want false (ListByProject has no build scope)")
	}
}
