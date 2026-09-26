package main

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestSweepStorageOrphans sweeps three storage prefixes: "42" belongs to child
// project 7 and holds report dirs 1, 2, 3, of which only build 2 is still in
// the database; listing "43" (project 8) fails; "loose" matches no project.
// Orphans are addressed by storage key, unknown prefixes are only reported,
// a failed listing is counted without aborting, and -project scopes the run.
func TestSweepStorageOrphans(t *testing.T) {
	projects := []store.Project{
		{ID: 7, Slug: "child", StorageKey: "42"},
		{ID: 8, Slug: "other", StorageKey: "43"},
	}
	tests := []struct {
		name       string
		flags      sweepFlags
		want       sweepResult
		wantListed []string
		wantPruned []string
	}{
		{"dry run deletes nothing", sweepFlags{},
			sweepResult{projectsScanned: 2, orphanDirs: 2, unknownPrefixes: []string{"loose"}, failures: 1},
			[]string{"42", "43"}, nil},
		{"apply prunes orphans by storage key", sweepFlags{apply: true},
			sweepResult{projectsScanned: 2, orphanDirs: 2, dirsDeleted: 2, unknownPrefixes: []string{"loose"}, failures: 1},
			[]string{"42", "43"}, []string{"42/1", "42/3"}},
		{"project filter skips other projects and unknown prefixes", sweepFlags{apply: true, projectID: 8},
			sweepResult{projectsScanned: 1, failures: 1},
			[]string{"43"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var listed, pruned []string
			builds := &testutil.MockBuildStore{
				ListBuildsFn: func(_ context.Context, projectID int64) ([]store.Build, error) {
					if projectID != 7 {
						t.Errorf("ListBuilds called for unexpected project %d", projectID)
					}
					return []store.Build{{ProjectID: 7, BuildNumber: 2}}, nil
				},
			}
			dataStore := &storage.MockStore{
				ListProjectsFn: func(context.Context) ([]string, error) {
					return []string{"42", "43", "loose"}, nil
				},
				ListReportBuildsFn: func(_ context.Context, key string) ([]int, error) {
					listed = append(listed, key)
					if key == "43" {
						return nil, errors.New("boom")
					}
					return []int{1, 2, 3}, nil
				},
				PruneReportDirsFn: func(_ context.Context, key string, buildNumbers []int) error {
					for _, n := range buildNumbers {
						pruned = append(pruned, key+"/"+strconv.Itoa(n))
					}
					return nil
				},
			}

			got := sweepStorageOrphans(context.Background(), projects, builds, dataStore, tc.flags, zap.NewNop())
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("result = %+v, want %+v", got, tc.want)
			}
			slices.Sort(pruned)
			if !slices.Equal(listed, tc.wantListed) || !slices.Equal(pruned, tc.wantPruned) {
				t.Errorf("listed %v pruned %v, want listed %v pruned %v", listed, pruned, tc.wantListed, tc.wantPruned)
			}
		})
	}
}
