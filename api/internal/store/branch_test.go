package store_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestBranchStore_GetOrCreate_FirstBranchIsDefault(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-proj-1")

	branch, created, err := bs.GetOrCreate(ctx, "br-proj-1", "main")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if !created {
		t.Error("expected created=true for new branch")
	}
	if !branch.IsDefault {
		t.Error("first branch should be default")
	}
	if branch.Name != "main" {
		t.Errorf("Name = %q, want %q", branch.Name, "main")
	}
	if branch.ProjectID != "br-proj-1" {
		t.Errorf("ProjectID = %q, want %q", branch.ProjectID, "br-proj-1")
	}
}

func TestBranchStore_GetOrCreate_SecondBranchNotDefault(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-proj-2")
	_, _, _ = bs.GetOrCreate(ctx, "br-proj-2", "main")

	branch, created, err := bs.GetOrCreate(ctx, "br-proj-2", "feature-x")
	if err != nil {
		t.Fatalf("GetOrCreate second branch: %v", err)
	}
	if !created {
		t.Error("expected created=true for new branch")
	}
	if branch.IsDefault {
		t.Error("second branch should NOT be default")
	}
}

func TestBranchStore_GetOrCreate_ExistingBranchNotCreated(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-proj-3")
	first, _, _ := bs.GetOrCreate(ctx, "br-proj-3", "main")

	second, created, err := bs.GetOrCreate(ctx, "br-proj-3", "main")
	if err != nil {
		t.Fatalf("GetOrCreate existing: %v", err)
	}
	if created {
		t.Error("expected created=false for existing branch")
	}
	if second.ID != first.ID {
		t.Errorf("expected same ID: got %d, want %d", second.ID, first.ID)
	}
}

func TestBranchStore_List(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-list-proj")
	_, _, _ = bs.GetOrCreate(ctx, "br-list-proj", "main")
	_, _, _ = bs.GetOrCreate(ctx, "br-list-proj", "dev")
	_, _, _ = bs.GetOrCreate(ctx, "br-list-proj", "feature-y")

	branches, err := bs.List(ctx, "br-list-proj")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(branches) != 3 {
		t.Errorf("expected 3 branches, got %d", len(branches))
	}
	defaults := 0
	for _, b := range branches {
		if b.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("expected exactly 1 default branch, got %d", defaults)
	}
}

func TestBranchStore_List_Empty(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-empty-proj")

	branches, err := bs.List(ctx, "br-empty-proj")
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 branches, got %d", len(branches))
	}
}

func TestBranchStore_GetDefault(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-default-proj")
	_, _, _ = bs.GetOrCreate(ctx, "br-default-proj", "main")
	_, _, _ = bs.GetOrCreate(ctx, "br-default-proj", "dev")

	def, err := bs.GetDefault(ctx, "br-default-proj")
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def.Name != "main" {
		t.Errorf("default branch = %q, want %q", def.Name, "main")
	}
	if !def.IsDefault {
		t.Error("GetDefault should return branch with IsDefault=true")
	}
}

func TestBranchStore_GetDefault_NotFound(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-nodefault-proj")

	_, err := bs.GetDefault(ctx, "br-nodefault-proj")
	if err == nil {
		t.Fatal("expected error for project with no branches")
	}
	if !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestBranchStore_SetDefault(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-setdef-proj")
	main, _, _ := bs.GetOrCreate(ctx, "br-setdef-proj", "main")
	dev, _, _ := bs.GetOrCreate(ctx, "br-setdef-proj", "dev")

	if err := bs.SetDefault(ctx, "br-setdef-proj", dev.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	def, err := bs.GetDefault(ctx, "br-setdef-proj")
	if err != nil {
		t.Fatalf("GetDefault after SetDefault: %v", err)
	}
	if def.ID != dev.ID {
		t.Errorf("expected default=%d (dev), got %d", dev.ID, def.ID)
	}

	updated, _ := bs.GetByName(ctx, "br-setdef-proj", "main")
	if updated.IsDefault {
		t.Errorf("main branch (id=%d) should not be default after SetDefault to dev", main.ID)
	}
}

func TestBranchStore_SetDefault_WrongProject(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-sd-proj-a")
	_ = ps.CreateProject(ctx, "br-sd-proj-b")
	brA, _, _ := bs.GetOrCreate(ctx, "br-sd-proj-a", "main")

	err := bs.SetDefault(ctx, "br-sd-proj-b", brA.ID)
	if err == nil {
		t.Fatal("expected error when setting branch from different project as default")
	}
	if !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestBranchStore_Delete(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-del-proj")
	_, _, _ = bs.GetOrCreate(ctx, "br-del-proj", "main")
	feature, _, _ := bs.GetOrCreate(ctx, "br-del-proj", "feature-z")

	if err := bs.Delete(ctx, "br-del-proj", feature.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	branches, _ := bs.List(ctx, "br-del-proj")
	if len(branches) != 1 {
		t.Errorf("expected 1 branch after delete, got %d", len(branches))
	}
	if branches[0].Name != "main" {
		t.Errorf("remaining branch = %q, want %q", branches[0].Name, "main")
	}
}

func TestBranchStore_Delete_DefaultBranchFails(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-del-default-proj")
	main, _, _ := bs.GetOrCreate(ctx, "br-del-default-proj", "main")

	err := bs.Delete(ctx, "br-del-default-proj", main.ID)
	if err == nil {
		t.Fatal("expected error when deleting default branch")
	}
	if !errors.Is(err, store.ErrCannotDeleteDefaultBranch) {
		t.Errorf("expected ErrCannotDeleteDefaultBranch, got %v", err)
	}
}

func TestBranchStore_Delete_NotFound(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-del-nf-proj")

	err := bs.Delete(ctx, "br-del-nf-proj", 9999)
	if err == nil {
		t.Fatal("expected error for non-existent branch")
	}
	if !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestBranchStore_GetByName(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-gbn-proj")
	_, _, _ = bs.GetOrCreate(ctx, "br-gbn-proj", "main")

	b, err := bs.GetByName(ctx, "br-gbn-proj", "main")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if b.Name != "main" {
		t.Errorf("Name = %q, want %q", b.Name, "main")
	}
}

func TestBranchStore_GetByName_NotFound(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-gbn-nf-proj")

	_, err := bs.GetByName(ctx, "br-gbn-nf-proj", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent branch name")
	}
	if !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestBranchStore_IsolatesProjects(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBranchStore(s)
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "br-iso-a")
	_ = ps.CreateProject(ctx, "br-iso-b")

	brA, _, _ := bs.GetOrCreate(ctx, "br-iso-a", "main")
	brB, _, _ := bs.GetOrCreate(ctx, "br-iso-b", "main")

	if brA.ID == brB.ID {
		t.Error("branches in different projects should have different IDs")
	}

	listA, _ := bs.List(ctx, "br-iso-a")
	listB, _ := bs.List(ctx, "br-iso-b")
	if len(listA) != 1 || len(listB) != 1 {
		t.Errorf("expected 1 branch per project, got A=%d B=%d", len(listA), len(listB))
	}
}
