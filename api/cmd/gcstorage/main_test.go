package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// sweepFixture wires a MockStore with two project prefixes ("42" keyed to a
// child project, "loose" matching nothing) plus report dirs 1,2,3 under "42",
// of which only build 2 still exists in the database.
func sweepFixture(t *testing.T) ([]store.Project, *testutil.MockBuildStore, *storage.MockStore, *[]int) {
	t.Helper()
	projects := []store.Project{
		{ID: 7, Slug: "child", StorageKey: "42"},
	}
	builds := &testutil.MockBuildStore{
		ListBuildsFn: func(_ context.Context, projectID int64) ([]store.Build, error) {
			if projectID != 7 {
				t.Errorf("ListBuilds called for unexpected project %d", projectID)
			}
			return []store.Build{{ProjectID: 7, BuildNumber: 2}}, nil
		},
	}
	var pruned []int
	dataStore := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"42", "loose"}, nil
		},
		ListReportBuildsFn: func(_ context.Context, projectID string) ([]int, error) {
			if projectID != "42" {
				t.Errorf("ListReportBuilds called for unexpected prefix %q", projectID)
			}
			return []int{1, 2, 3}, nil
		},
		PruneReportDirsFn: func(_ context.Context, projectID string, buildNumbers []int) error {
			if projectID != "42" {
				t.Errorf("PruneReportDirs called for unexpected prefix %q", projectID)
			}
			pruned = append(pruned, buildNumbers...)
			return nil
		},
	}
	return projects, builds, dataStore, &pruned
}

// TestSweepStorageOrphans_DryRun asserts the default mode finds the orphaned
// report dirs (1 and 3) and the unknown prefix without deleting anything.
func TestSweepStorageOrphans_DryRun(t *testing.T) {
	projects, builds, dataStore, pruned := sweepFixture(t)

	result := sweepStorageOrphans(context.Background(), projects, builds, dataStore,
		sweepFlags{apply: false}, zap.NewNop())

	if result.orphanDirs != 2 {
		t.Errorf("expected 2 orphan dirs (builds 1,3), got %d", result.orphanDirs)
	}
	if result.dirsDeleted != 0 || len(*pruned) != 0 {
		t.Errorf("dry-run must not delete anything, got deleted=%d pruned=%v", result.dirsDeleted, *pruned)
	}
	if !slices.Equal(result.unknownPrefixes, []string{"loose"}) {
		t.Errorf("expected unknown prefix [loose], got %v", result.unknownPrefixes)
	}
	if result.failures != 0 {
		t.Errorf("expected no failures, got %d", result.failures)
	}
}

// TestSweepStorageOrphans_Apply asserts -apply deletes exactly the orphaned
// dirs, addressed by storage key, and still never touches the unknown prefix.
func TestSweepStorageOrphans_Apply(t *testing.T) {
	projects, builds, dataStore, pruned := sweepFixture(t)

	result := sweepStorageOrphans(context.Background(), projects, builds, dataStore,
		sweepFlags{apply: true}, zap.NewNop())

	if result.dirsDeleted != 2 {
		t.Errorf("expected 2 dirs deleted, got %d", result.dirsDeleted)
	}
	slices.Sort(*pruned)
	if !slices.Equal(*pruned, []int{1, 3}) {
		t.Errorf("expected builds 1,3 pruned (2 still exists in DB), got %v", *pruned)
	}
	if !slices.Equal(result.unknownPrefixes, []string{"loose"}) {
		t.Errorf("expected unknown prefix [loose] untouched, got %v", result.unknownPrefixes)
	}
}

// TestSweepStorageOrphans_ProjectFilterAndErrors asserts the -project filter
// skips other projects and suppresses unknown-prefix reporting (a scoped run
// only reports its own project), and that a ListReportBuilds failure is
// counted but does not abort the run.
func TestSweepStorageOrphans_ProjectFilterAndErrors(t *testing.T) {
	projects := []store.Project{
		{ID: 7, Slug: "child", StorageKey: "42"},
		{ID: 8, Slug: "other", StorageKey: "43"},
	}
	builds := &testutil.MockBuildStore{
		ListBuildsFn: func(_ context.Context, _ int64) ([]store.Build, error) {
			return nil, nil
		},
	}
	dataStore := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"42", "43", "loose"}, nil
		},
		ListReportBuildsFn: func(_ context.Context, projectID string) ([]int, error) {
			if projectID == "43" {
				t.Error("project filter did not skip storage_key 43")
			}
			return nil, errors.New("boom")
		},
	}

	result := sweepStorageOrphans(context.Background(), projects, builds, dataStore,
		sweepFlags{apply: true, projectID: 7}, zap.NewNop())

	if result.projectsScanned != 1 {
		t.Errorf("expected 1 project scanned under -project filter, got %d", result.projectsScanned)
	}
	if result.failures != 1 {
		t.Errorf("expected the ListReportBuilds failure to be counted once, got %d", result.failures)
	}
	if len(result.unknownPrefixes) != 0 {
		t.Errorf("scoped run must not report unrelated unknown prefixes, got %v", result.unknownPrefixes)
	}
}
