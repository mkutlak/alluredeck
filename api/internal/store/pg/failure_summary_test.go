package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// seedFailureSummaryFixture creates a project + one build and returns the
// FailureSummaryStore plus the project/build identifiers.
func seedFailureSummaryFixture(t *testing.T) (fs *pg.FailureSummaryStore, projectID, buildID int64) {
	t.Helper()
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)
	fs = pg.NewFailureSummaryStore(s)

	slug := fmt.Sprintf("failsum-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID = proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err = trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}
	return fs, projectID, buildID
}

func TestFailureSummaryStore_GetMiss(t *testing.T) {
	fs, _, buildID := seedFailureSummaryFixture(t)
	got, err := fs.Get(context.Background(), buildID, "no-such-history")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on cache miss, got %+v", got)
	}
}

func TestFailureSummaryStore_UpsertThenGet(t *testing.T) {
	fs, projectID, buildID := seedFailureSummaryFixture(t)
	ctx := context.Background()

	in := store.FailureSummary{
		BuildID:       buildID,
		HistoryID:     "h1",
		ProjectID:     projectID,
		InputHash:     "hash-v1",
		Hypothesis:    "The product returned 500.",
		Category:      "product_bug",
		Confidence:    "medium",
		Evidence:      []string{"status 500 from /users", "last passed 3 builds ago"},
		Model:         "llama3.1",
		PromptVersion: 1,
	}
	if err := fs.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := fs.Get(ctx, buildID, "h1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected a cached summary, got nil")
	}
	if got.InputHash != "hash-v1" || got.Hypothesis != in.Hypothesis {
		t.Errorf("scalar fields mismatch: %+v", got)
	}
	if got.Category != "product_bug" || got.Confidence != "medium" || got.Model != "llama3.1" {
		t.Errorf("category/confidence/model mismatch: %+v", got)
	}
	if got.PromptVersion != 1 {
		t.Errorf("prompt_version: got %d, want 1", got.PromptVersion)
	}
	if len(got.Evidence) != 2 || got.Evidence[0] != "status 500 from /users" {
		t.Errorf("evidence round-trip mismatch: %+v", got.Evidence)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at should be populated by the DB default")
	}
}

func TestFailureSummaryStore_UpsertReplaces(t *testing.T) {
	fs, projectID, buildID := seedFailureSummaryFixture(t)
	ctx := context.Background()

	first := store.FailureSummary{
		BuildID: buildID, HistoryID: "h1", ProjectID: projectID,
		InputHash: "hash-v1", Hypothesis: "first guess", Category: "flake",
		Confidence: "low", Evidence: []string{"a"}, Model: "m1", PromptVersion: 1,
	}
	if err := fs.Upsert(ctx, first); err != nil {
		t.Fatalf("Upsert first: %v", err)
	}

	second := store.FailureSummary{
		BuildID: buildID, HistoryID: "h1", ProjectID: projectID,
		InputHash: "hash-v2", Hypothesis: "revised guess", Category: "product_bug",
		Confidence: "high", Evidence: []string{"b", "c"}, Model: "m2", PromptVersion: 1,
	}
	if err := fs.Upsert(ctx, second); err != nil {
		t.Fatalf("Upsert second: %v", err)
	}

	got, err := fs.Get(ctx, buildID, "h1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected a cached summary, got nil")
	}
	if got.InputHash != "hash-v2" || got.Hypothesis != "revised guess" || got.Category != "product_bug" {
		t.Errorf("expected the replaced row, got %+v", got)
	}
	if len(got.Evidence) != 2 {
		t.Errorf("expected replaced evidence of len 2, got %+v", got.Evidence)
	}
}

// TestFailureSummaryStore_EmptyEvidenceRoundTrips verifies nil/empty evidence
// stores as a JSON array and reads back as empty, never NULL.
func TestFailureSummaryStore_EmptyEvidenceRoundTrips(t *testing.T) {
	fs, projectID, buildID := seedFailureSummaryFixture(t)
	ctx := context.Background()

	if err := fs.Upsert(ctx, store.FailureSummary{
		BuildID: buildID, HistoryID: "h2", ProjectID: projectID,
		InputHash: "h", Hypothesis: "no evidence", Category: "test_bug",
		Model: "m", PromptVersion: 1, Evidence: nil,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := fs.Get(ctx, buildID, "h2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || len(got.Evidence) != 0 {
		t.Errorf("expected empty evidence, got %+v", got)
	}
}
