package store_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestProjectStore_CreateAndGet(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	if err := ps.CreateProject(ctx, "proj1"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	p, err := ps.GetProject(ctx, "proj1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.ID != "proj1" {
		t.Errorf("got ID %q, want %q", p.ID, "proj1")
	}
}

func TestProjectStore_DuplicateError(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "dup")
	err := ps.CreateProject(ctx, "dup")
	if !errors.Is(err, store.ErrProjectExists) {
		t.Errorf("expected ErrProjectExists, got %v", err)
	}
}

func TestProjectStore_GetNotFound(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_, err := ps.GetProject(ctx, "nonexistent")
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestProjectStore_ListEmpty(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())

	projects, err := ps.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d", len(projects))
	}
}

func TestProjectStore_ListMultiple(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	for _, id := range []string{"alpha", "beta", "gamma"} {
		_ = ps.CreateProject(ctx, id)
	}

	projects, err := ps.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 3 {
		t.Errorf("expected 3 projects, got %d", len(projects))
	}
}

func TestProjectStore_DeleteCascades(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "cascade-proj")
	_, _ = bs.NextBuildOrder(ctx, "cascade-proj")
	_ = bs.InsertBuild(ctx, "cascade-proj", 1)

	if err := ps.DeleteProject(ctx, "cascade-proj"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	builds, _ := bs.ListBuilds(ctx, "cascade-proj")
	if len(builds) != 0 {
		t.Errorf("expected builds to cascade-delete, got %d", len(builds))
	}
}

// TestProjectStore_GetProject_InvalidCreatedAt verifies that a project with a
// corrupt created_at value is still returned with zero-value CreatedAt.
func TestProjectStore_GetProject_InvalidCreatedAt(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "bad-ts")

	// Corrupt the created_at value via raw SQL.
	_, err := s.DB().ExecContext(ctx,
		"UPDATE projects SET created_at='not-a-timestamp' WHERE id=?", "bad-ts")
	if err != nil {
		t.Fatalf("corrupt created_at: %v", err)
	}

	p, err := ps.GetProject(ctx, "bad-ts")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !p.CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt for invalid timestamp, got %v", p.CreatedAt)
	}
}

func TestProjectStore_Exists(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	exists, _ := ps.ProjectExists(ctx, "ghost")
	if exists {
		t.Error("expected false for nonexistent project")
	}

	_ = ps.CreateProject(ctx, "real")
	exists, _ = ps.ProjectExists(ctx, "real")
	if !exists {
		t.Error("expected true for existing project")
	}
}
