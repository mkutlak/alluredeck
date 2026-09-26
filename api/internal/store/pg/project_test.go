package pg_test

import "testing"

// TestInsertOrIgnore_ChildSlugExists is the regression test for the
// duplicate-project bug introduced by migration 0031: InsertOrIgnore must not
// create a top-level row when a child project already holds the slug.
func TestInsertOrIgnore_ChildSlugExists(t *testing.T) {
	f := newFixture(t)
	child := f.newProject(f.id)
	if err := f.projects.InsertOrIgnore(f.ctx, child.Slug); err != nil {
		t.Fatalf("InsertOrIgnore: %v", err)
	}
	if n := f.count("SELECT COUNT(*) FROM projects WHERE slug = $1", child.Slug); n != 1 {
		t.Errorf("project rows with slug %q = %d, want 1 (duplicate created)", child.Slug, n)
	}
}
