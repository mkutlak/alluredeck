package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestInsertOrIgnore_ChildSlugExists verifies that InsertOrIgnore does NOT create
// a new top-level project row when a child project with the same slug already exists.
// This is the regression test for the duplicate-project bug introduced by migration 0031.
func TestInsertOrIgnore_ChildSlugExists(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)

	// Create a parent project.
	parentSlug := fmt.Sprintf("test-insign-parent-%d", time.Now().UnixNano())
	parent, err := projectStore.CreateProject(ctx, parentSlug)
	if err != nil {
		t.Fatalf("CreateProject parent: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), parent.ID) })

	// Create a child project with a distinct slug, parented to the above.
	childSlug := fmt.Sprintf("test-insign-child-%d", time.Now().UnixNano())
	child, err := projectStore.CreateProjectWithParent(ctx, childSlug, parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent child: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), child.ID) })

	// InsertOrIgnore with the child slug must not insert a new row.
	if err := projectStore.InsertOrIgnore(ctx, childSlug); err != nil {
		t.Fatalf("InsertOrIgnore: %v", err)
	}

	// Assert exactly one row with this slug exists.
	var count int
	row := s.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM projects WHERE slug = $1", childSlug)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 project row with slug %q, got %d (duplicate created)", childSlug, count)
	}
}
