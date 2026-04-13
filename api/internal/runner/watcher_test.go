package runner

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestWatcher_DoesNotDuplicateChildProjects verifies that checkProjects does NOT call
// CreateProject when GetProjectBySlugAny already finds the slug (e.g. as a child project).
// This is the regression test for the watcher half of the duplicate-project bug.
func TestWatcher_DoesNotDuplicateChildProjects(t *testing.T) {
	ctx := context.Background()
	childSlug := "kid-project"

	// Seed the in-memory store with a child project (parent_id != nil).
	mem := testutil.NewMemProjectStore()
	parentID := int64(99)
	_, err := mem.CreateProjectWithParent(ctx, childSlug, parentID)
	if err != nil {
		t.Fatalf("seed child project: %v", err)
	}

	// Track whether CreateProject is ever called — it must not be.
	createCalled := false
	var projectStore store.ProjectStorer = &createTrackingStore{
		MemProjectStore: mem,
		onCreateProject: func() { createCalled = true },
	}

	st := &storage.MockStore{
		ListProjectsFn: func(ctx context.Context) ([]string, error) {
			return []string{childSlug}, nil
		},
	}

	w := &Watcher{
		cfg:          &config.Config{},
		projectStore: projectStore,
		store:        st,
		logger:       zap.NewNop(),
	}

	w.checkProjects(make(map[string]string))

	if createCalled {
		t.Error("CreateProject was called for a child-project slug — duplicate would have been created")
	}
}

// createTrackingStore wraps MemProjectStore and fires a callback on CreateProject.
type createTrackingStore struct {
	*testutil.MemProjectStore
	onCreateProject func()
}

func (s *createTrackingStore) CreateProject(ctx context.Context, slug string) (*store.Project, error) {
	if s.onCreateProject != nil {
		s.onCreateProject()
	}
	return s.MemProjectStore.CreateProject(ctx, slug)
}
