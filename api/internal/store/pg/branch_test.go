package pg_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestBranchList derives a project's branches from the ci_branch of its
// builds (deduplicated) even when the branches table lacks the rows — a silent
// GetOrCreate failure or an upload predating migration 0010 — and lists a
// parent as the union across every child. Regression for the dropdown that
// only ever showed childIds[0]'s branches.
func TestBranchList(t *testing.T) {
	f := newFixture(t) // the parent
	childA, childB, lone := f.newProject(f.id), f.newProject(f.id), f.newProject(0)
	seed := func(projectID int64, order int, branch string) {
		f.buildIn(projectID, order)
		if err := f.builds.UpdateBuildCIMetadata(f.ctx, projectID, order, store.CIMetadata{
			Branch: branch, CommitSHA: fmt.Sprintf("sha-%d-%d", projectID, order),
		}); err != nil {
			t.Fatalf("UpdateBuildCIMetadata: %v", err)
		}
	}
	seed(childA.ID, 1, "master")
	seed(childA.ID, 2, "v2.20")
	for _, name := range []string{"master", "v2.20"} {
		if _, _, err := f.branches.GetOrCreate(f.ctx, childA.ID, name); err != nil {
			t.Fatalf("GetOrCreate %s: %v", name, err)
		}
	}
	seed(childB.ID, 1, "RG-4695-feature") // no branches row
	seed(childB.ID, 2, "RG-4695-feature")
	seed(childB.ID, 3, "hotfix")

	for _, tt := range []struct {
		name      string
		projectID int64
		want      []string
	}{
		{"parent unions every child", f.id, []string{"RG-4695-feature", "hotfix", "master", "v2.20"}},
		{"derived from builds when the table is empty", childB.ID, []string{"RG-4695-feature", "hotfix"}},
		{"no builds, no branches", lone.ID, nil},
	} {
		branches, err := f.branches.List(f.ctx, tt.projectID)
		if err != nil {
			t.Fatalf("%s: List: %v", tt.name, err)
		}
		var got []string
		for _, b := range branches {
			got = append(got, b.Name)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: branches = %v, want %v", tt.name, got, tt.want)
		}
	}
}
