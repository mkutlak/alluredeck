package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"go.uber.org/zap"
)

func openTestProjectStore(t *testing.T) (*store.SQLiteStore, *store.ProjectStore) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "tags_test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ps := store.NewProjectStore(db, zap.NewNop())
	return db, ps
}

func TestSetTags_RoundTrip(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	if err := ps.CreateProject(ctx, "tagproj"); err != nil {
		t.Fatal(err)
	}

	tags := []string{"backend", "nightly"}
	if err := ps.SetTags(ctx, "tagproj", tags); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	p, err := ps.GetProject(ctx, "tagproj")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	if len(p.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(p.Tags), p.Tags)
	}
	if p.Tags[0] != "backend" || p.Tags[1] != "nightly" {
		t.Errorf("unexpected tags: %v", p.Tags)
	}
}

func TestSetTags_EmptyTags(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	if err := ps.CreateProject(ctx, "emptytagproj"); err != nil {
		t.Fatal(err)
	}

	// Set some tags first.
	if err := ps.SetTags(ctx, "emptytagproj", []string{"old"}); err != nil {
		t.Fatal(err)
	}

	// Clear tags with empty slice.
	if err := ps.SetTags(ctx, "emptytagproj", []string{}); err != nil {
		t.Fatalf("SetTags with empty slice: %v", err)
	}

	p, err := ps.GetProject(ctx, "emptytagproj")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d: %v", len(p.Tags), p.Tags)
	}
}

func TestSetTags_NonExistentProject(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	err := ps.SetTags(ctx, "ghost-project", []string{"tag1"})
	if err == nil {
		t.Fatal("expected error for non-existent project, got nil")
	}
}

func TestListProjectsPaginated_TagFilter(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	for _, id := range []string{"proj-a", "proj-b", "proj-c"} {
		if err := ps.CreateProject(ctx, id); err != nil {
			t.Fatal(err)
		}
	}

	if err := ps.SetTags(ctx, "proj-a", []string{"backend", "nightly"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetTags(ctx, "proj-b", []string{"frontend"}); err != nil {
		t.Fatal(err)
	}
	// proj-c has no tags

	projects, total, err := ps.ListProjectsPaginated(ctx, 1, 10, "backend")
	if err != nil {
		t.Fatalf("ListProjectsPaginated with tag filter: %v", err)
	}

	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ID != "proj-a" {
		t.Errorf("expected proj-a, got %q", projects[0].ID)
	}
}

func TestListProjectsPaginated_NoTagFilter(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	for _, id := range []string{"proj-x", "proj-y"} {
		if err := ps.CreateProject(ctx, id); err != nil {
			t.Fatal(err)
		}
	}

	projects, total, err := ps.ListProjectsPaginated(ctx, 1, 10, "")
	if err != nil {
		t.Fatalf("ListProjectsPaginated without tag filter: %v", err)
	}

	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestGetProject_TagsDefaultEmpty(t *testing.T) {
	_, ps := openTestProjectStore(t)
	ctx := context.Background()

	if err := ps.CreateProject(ctx, "newproj"); err != nil {
		t.Fatal(err)
	}

	p, err := ps.GetProject(ctx, "newproj")
	if err != nil {
		t.Fatal(err)
	}
	if p.Tags == nil {
		t.Error("Tags should not be nil (should be empty slice)")
	}
	if len(p.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(p.Tags))
	}
}
