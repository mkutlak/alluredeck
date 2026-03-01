package store_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestBuildStore_NextOrder_First(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	ps := store.NewProjectStore(s, zap.NewNop())
	_ = ps.CreateProject(ctx, "p1")

	order, err := bs.NextBuildOrder(ctx, "p1")
	if err != nil {
		t.Fatalf("NextBuildOrder: %v", err)
	}
	if order != 1 {
		t.Errorf("expected 1, got %d", order)
	}
}

func TestBuildStore_NextOrder_Increments(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "p2")
	_ = bs.InsertBuild(ctx, "p2", 1)
	_ = bs.InsertBuild(ctx, "p2", 2)

	order, err := bs.NextBuildOrder(ctx, "p2")
	if err != nil {
		t.Fatalf("NextBuildOrder: %v", err)
	}
	if order != 3 {
		t.Errorf("expected 3, got %d", order)
	}
}

func TestBuildStore_UpdateStats(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "stats-proj")
	_ = bs.InsertBuild(ctx, "stats-proj", 1)

	stats := store.BuildStats{Passed: 10, Failed: 2, Total: 12}
	if err := bs.UpdateBuildStats(ctx, "stats-proj", 1, stats); err != nil {
		t.Fatalf("UpdateBuildStats: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "stats-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	b := builds[0]
	if b.StatPassed == nil || *b.StatPassed != 10 {
		t.Errorf("StatPassed: got %v", b.StatPassed)
	}
	if b.StatTotal == nil || *b.StatTotal != 12 {
		t.Errorf("StatTotal: got %v", b.StatTotal)
	}
}

func TestBuildStore_ListDescending(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "order-proj")
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "order-proj", i)
	}

	builds, err := bs.ListBuilds(ctx, "order-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 5 {
		t.Fatalf("expected 5, got %d", len(builds))
	}
	// Should be descending: 5, 4, 3, 2, 1
	for i, b := range builds {
		expected := 5 - i
		if b.BuildOrder != expected {
			t.Errorf("builds[%d].BuildOrder = %d, want %d", i, b.BuildOrder, expected)
		}
	}
}

func TestBuildStore_PruneKeepsNewest(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "prune-proj")
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "prune-proj", i)
	}

	removed, err := bs.PruneBuilds(ctx, "prune-proj", 3)
	if err != nil {
		t.Fatalf("PruneBuilds: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("expected 2 removed, got %d: %v", len(removed), removed)
	}

	remaining, _ := bs.ListBuilds(ctx, "prune-proj")
	if len(remaining) != 3 {
		t.Errorf("expected 3 remaining, got %d", len(remaining))
	}
	// Remaining should be 5, 4, 3 (newest)
	for i, b := range remaining {
		expected := 5 - i
		if b.BuildOrder != expected {
			t.Errorf("remaining[%d] = %d, want %d", i, b.BuildOrder, expected)
		}
	}
}

// TestBuildStore_ListBuilds_InvalidCreatedAt verifies that a build with a
// corrupt created_at value is still returned with zero-value CreatedAt.
func TestBuildStore_ListBuilds_InvalidCreatedAt(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "bad-ts")
	_ = bs.InsertBuild(ctx, "bad-ts", 1)

	// Corrupt the created_at value via raw SQL.
	_, err := s.DB().ExecContext(ctx,
		"UPDATE builds SET created_at='not-a-timestamp' WHERE project_id=? AND build_order=?",
		"bad-ts", 1)
	if err != nil {
		t.Fatalf("corrupt created_at: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "bad-ts")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if !builds[0].CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt for invalid timestamp, got %v", builds[0].CreatedAt)
	}
}

func TestBuildStore_UpdateBuildCIMetadata(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "ci-proj")
	_ = bs.InsertBuild(ctx, "ci-proj", 1)

	ciMeta := store.CIMetadata{
		Provider:  "GitHub Actions",
		BuildURL:  "https://github.com/org/repo/actions/runs/123",
		Branch:    "main",
		CommitSHA: "abc1234",
	}
	if err := bs.UpdateBuildCIMetadata(ctx, "ci-proj", 1, ciMeta); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "ci-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	b := builds[0]
	if b.CIProvider == nil || *b.CIProvider != "GitHub Actions" {
		t.Errorf("CIProvider: got %v", b.CIProvider)
	}
	if b.CIBuildURL == nil || *b.CIBuildURL != "https://github.com/org/repo/actions/runs/123" {
		t.Errorf("CIBuildURL: got %v", b.CIBuildURL)
	}
	if b.CIBranch == nil || *b.CIBranch != "main" {
		t.Errorf("CIBranch: got %v", b.CIBranch)
	}
	if b.CICommitSHA == nil || *b.CICommitSHA != "abc1234" {
		t.Errorf("CICommitSHA: got %v", b.CICommitSHA)
	}
}

func TestBuildStore_UpdateBuildCIMetadata_Partial(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "ci-partial")
	_ = bs.InsertBuild(ctx, "ci-partial", 1)

	// Only provider and branch, no BuildURL or CommitSHA.
	ciMeta := store.CIMetadata{
		Provider: "Jenkins",
		Branch:   "feature/ci",
	}
	if err := bs.UpdateBuildCIMetadata(ctx, "ci-partial", 1, ciMeta); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "ci-partial")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	b := builds[0]
	if b.CIProvider == nil || *b.CIProvider != "Jenkins" {
		t.Errorf("CIProvider: got %v", b.CIProvider)
	}
	if b.CIBranch == nil || *b.CIBranch != "feature/ci" {
		t.Errorf("CIBranch: got %v", b.CIBranch)
	}
	// Empty string fields should result in nil (not stored).
	if b.CIBuildURL != nil {
		t.Errorf("CIBuildURL: expected nil for empty field, got %v", *b.CIBuildURL)
	}
	if b.CICommitSHA != nil {
		t.Errorf("CICommitSHA: expected nil for empty field, got %v", *b.CICommitSHA)
	}
}

func TestBuildStore_ListBuildsPaginated_IncludesCIMetadata(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "ci-paginated")
	_ = bs.InsertBuild(ctx, "ci-paginated", 1)

	ciMeta := store.CIMetadata{
		Provider:  "GitLab CI",
		BuildURL:  "https://gitlab.com/org/repo/-/pipelines/456",
		Branch:    "develop",
		CommitSHA: "def5678",
	}
	if err := bs.UpdateBuildCIMetadata(ctx, "ci-paginated", 1, ciMeta); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	builds, _, err := bs.ListBuildsPaginated(ctx, "ci-paginated", 1, 20)
	if err != nil {
		t.Fatalf("ListBuildsPaginated: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	b := builds[0]
	if b.CIProvider == nil || *b.CIProvider != "GitLab CI" {
		t.Errorf("CIProvider: got %v", b.CIProvider)
	}
	if b.CIBranch == nil || *b.CIBranch != "develop" {
		t.Errorf("CIBranch: got %v", b.CIBranch)
	}
}

func TestBuildStore_SetLatest(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "latest-proj")
	_ = bs.InsertBuild(ctx, "latest-proj", 1)
	_ = bs.InsertBuild(ctx, "latest-proj", 2)
	_ = bs.InsertBuild(ctx, "latest-proj", 3)

	if err := bs.SetLatest(ctx, "latest-proj", 3); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	builds, _ := bs.ListBuilds(ctx, "latest-proj")
	for _, b := range builds {
		if b.BuildOrder == 3 && !b.IsLatest {
			t.Error("build 3 should be latest")
		}
		if b.BuildOrder != 3 && b.IsLatest {
			t.Errorf("build %d should not be latest", b.BuildOrder)
		}
	}
}

func TestBuildStore_GetBuildByOrder(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "gbo-proj")
	_ = bs.InsertBuild(ctx, "gbo-proj", 1)
	_ = bs.UpdateBuildStats(ctx, "gbo-proj", 1, store.BuildStats{
		Passed: 10, Failed: 2, Broken: 1, Skipped: 3, Unknown: 0, Total: 16,
		DurationMs: 45000,
	})

	// Found case.
	b, err := bs.GetBuildByOrder(ctx, "gbo-proj", 1)
	if err != nil {
		t.Fatalf("GetBuildByOrder: %v", err)
	}
	if b.BuildOrder != 1 {
		t.Errorf("BuildOrder = %d, want 1", b.BuildOrder)
	}
	if b.ProjectID != "gbo-proj" {
		t.Errorf("ProjectID = %q, want %q", b.ProjectID, "gbo-proj")
	}
	if b.StatPassed == nil || *b.StatPassed != 10 {
		t.Errorf("StatPassed = %v, want 10", b.StatPassed)
	}
	if b.StatTotal == nil || *b.StatTotal != 16 {
		t.Errorf("StatTotal = %v, want 16", b.StatTotal)
	}

	// Not-found case.
	_, err = bs.GetBuildByOrder(ctx, "gbo-proj", 99)
	if err == nil {
		t.Fatal("expected error for non-existent build")
	}
	if !errors.Is(err, store.ErrBuildNotFound) {
		t.Errorf("expected ErrBuildNotFound, got %v", err)
	}
}

func TestBuildStore_GetPreviousBuild(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "gpb-proj")
	_ = bs.InsertBuild(ctx, "gpb-proj", 1)
	_ = bs.InsertBuild(ctx, "gpb-proj", 2)
	_ = bs.InsertBuild(ctx, "gpb-proj", 5) // non-contiguous gap

	// Previous of build 2 is build 1.
	prev, err := bs.GetPreviousBuild(ctx, "gpb-proj", 2)
	if err != nil {
		t.Fatalf("GetPreviousBuild(2): %v", err)
	}
	if prev.BuildOrder != 1 {
		t.Errorf("previous of 2: got BuildOrder %d, want 1", prev.BuildOrder)
	}

	// Previous of build 5 is build 2 (non-contiguous gap).
	prev, err = bs.GetPreviousBuild(ctx, "gpb-proj", 5)
	if err != nil {
		t.Fatalf("GetPreviousBuild(5): %v", err)
	}
	if prev.BuildOrder != 2 {
		t.Errorf("previous of 5: got BuildOrder %d, want 2", prev.BuildOrder)
	}

	// No previous for the first build.
	_, err = bs.GetPreviousBuild(ctx, "gpb-proj", 1)
	if err == nil {
		t.Fatal("expected error for no previous build")
	}
	if !errors.Is(err, store.ErrBuildNotFound) {
		t.Errorf("expected ErrBuildNotFound, got %v", err)
	}
}

func TestBuildStore_GetLatestBuild(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "glb-proj")
	_ = bs.InsertBuild(ctx, "glb-proj", 1)
	_ = bs.InsertBuild(ctx, "glb-proj", 2)
	_ = bs.SetLatest(ctx, "glb-proj", 2)

	// Found case.
	b, err := bs.GetLatestBuild(ctx, "glb-proj")
	if err != nil {
		t.Fatalf("GetLatestBuild: %v", err)
	}
	if b.BuildOrder != 2 {
		t.Errorf("BuildOrder = %d, want 2", b.BuildOrder)
	}
	if !b.IsLatest {
		t.Error("expected IsLatest=true")
	}

	// Not-found case: no latest flag set.
	_ = ps.CreateProject(ctx, "glb-empty")
	_ = bs.InsertBuild(ctx, "glb-empty", 1)
	_, err = bs.GetLatestBuild(ctx, "glb-empty")
	if err == nil {
		t.Fatal("expected error when no build is marked latest")
	}
	if !errors.Is(err, store.ErrBuildNotFound) {
		t.Errorf("expected ErrBuildNotFound, got %v", err)
	}
}

func TestBuildStore_DeleteAllBuilds(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "del-all-proj")
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "del-all-proj", i)
	}

	// Verify builds exist before deletion.
	builds, _ := bs.ListBuilds(ctx, "del-all-proj")
	if len(builds) != 5 {
		t.Fatalf("setup: expected 5 builds, got %d", len(builds))
	}

	if err := bs.DeleteAllBuilds(ctx, "del-all-proj"); err != nil {
		t.Fatalf("DeleteAllBuilds: %v", err)
	}

	// All builds should be gone.
	remaining, err := bs.ListBuilds(ctx, "del-all-proj")
	if err != nil {
		t.Fatalf("ListBuilds after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 builds after DeleteAllBuilds, got %d", len(remaining))
	}
}

func TestBuildStore_DeleteAllBuilds_IsolatesProjects(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "del-iso-a")
	_ = ps.CreateProject(ctx, "del-iso-b")
	_ = bs.InsertBuild(ctx, "del-iso-a", 1)
	_ = bs.InsertBuild(ctx, "del-iso-a", 2)
	_ = bs.InsertBuild(ctx, "del-iso-b", 1)

	if err := bs.DeleteAllBuilds(ctx, "del-iso-a"); err != nil {
		t.Fatalf("DeleteAllBuilds: %v", err)
	}

	// Project A should have no builds.
	aBuilds, _ := bs.ListBuilds(ctx, "del-iso-a")
	if len(aBuilds) != 0 {
		t.Errorf("project A: expected 0 builds, got %d", len(aBuilds))
	}

	// Project B should be untouched.
	bBuilds, _ := bs.ListBuilds(ctx, "del-iso-b")
	if len(bBuilds) != 1 {
		t.Errorf("project B: expected 1 build, got %d", len(bBuilds))
	}
}

func TestBuildStore_DeleteAllBuilds_EmptyProject(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "del-empty")

	// Should not error on project with no builds.
	if err := bs.DeleteAllBuilds(ctx, "del-empty"); err != nil {
		t.Fatalf("DeleteAllBuilds on empty project: %v", err)
	}
}

func TestBuildStore_SetLatest_ClearsPreviousOnly(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "prev-proj")
	_ = bs.InsertBuild(ctx, "prev-proj", 1)
	_ = bs.InsertBuild(ctx, "prev-proj", 2)
	_ = bs.InsertBuild(ctx, "prev-proj", 3)

	// First call: mark build 2 as latest.
	if err := bs.SetLatest(ctx, "prev-proj", 2); err != nil {
		t.Fatalf("SetLatest(2): %v", err)
	}

	// Second call: move latest to build 3.
	if err := bs.SetLatest(ctx, "prev-proj", 3); err != nil {
		t.Fatalf("SetLatest(3): %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "prev-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	for _, b := range builds {
		if b.BuildOrder == 3 && !b.IsLatest {
			t.Error("build 3 should be latest")
		}
		if b.BuildOrder != 3 && b.IsLatest {
			t.Errorf("build %d should not be latest after two SetLatest calls", b.BuildOrder)
		}
	}
}
