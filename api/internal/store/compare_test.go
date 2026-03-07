package store_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// seedCompareData creates a project with two builds and returns their build IDs.
func seedCompareData(t *testing.T, s *store.SQLiteStore, projectID string) (buildIDA, buildIDB int64) {
	t.Helper()
	ctx := context.Background()
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ts := store.NewTestResultStore(s, zap.NewNop())

	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := bs.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild 1: %v", err)
	}
	if err := bs.InsertBuild(ctx, projectID, 2); err != nil {
		t.Fatalf("InsertBuild 2: %v", err)
	}

	idA, err := ts.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID build 1: %v", err)
	}
	idB, err := ts.GetBuildID(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("GetBuildID build 2: %v", err)
	}
	return idA, idB
}

func TestCompareBuildsByHistoryID_Regressed(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-regressed"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "LoginTest", FullName: "pkg.LoginTest", Status: "passed", DurationMs: 1000, HistoryID: "h1"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "LoginTest", FullName: "pkg.LoginTest", Status: "failed", DurationMs: 2000, HistoryID: "h1"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Category != store.DiffRegressed {
		t.Errorf("expected category=%q, got %q", store.DiffRegressed, e.Category)
	}
	if e.StatusA != "passed" {
		t.Errorf("expected statusA=%q, got %q", "passed", e.StatusA)
	}
	if e.StatusB != "failed" {
		t.Errorf("expected statusB=%q, got %q", "failed", e.StatusB)
	}
	if e.TestName != "LoginTest" {
		t.Errorf("expected test_name=%q, got %q", "LoginTest", e.TestName)
	}
	if e.DurationA != 1000 {
		t.Errorf("expected durationA=1000, got %d", e.DurationA)
	}
	if e.DurationB != 2000 {
		t.Errorf("expected durationB=2000, got %d", e.DurationB)
	}
}

func TestCompareBuildsByHistoryID_Fixed(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-fixed"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "SignupTest", FullName: "pkg.SignupTest", Status: "failed", DurationMs: 500, HistoryID: "h2"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "SignupTest", FullName: "pkg.SignupTest", Status: "passed", DurationMs: 300, HistoryID: "h2"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Category != store.DiffFixed {
		t.Errorf("expected category=%q, got %q", store.DiffFixed, e.Category)
	}
	if e.StatusA != "failed" || e.StatusB != "passed" {
		t.Errorf("expected statusA=failed statusB=passed, got %q %q", e.StatusA, e.StatusB)
	}
}

func TestCompareBuildsByHistoryID_Added(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-added"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	// Only in build B
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "NewTest", FullName: "pkg.NewTest", Status: "passed", DurationMs: 400, HistoryID: "h3"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Category != store.DiffAdded {
		t.Errorf("expected category=%q, got %q", store.DiffAdded, e.Category)
	}
	if e.StatusA != "" {
		t.Errorf("expected statusA empty, got %q", e.StatusA)
	}
	if e.StatusB != "passed" {
		t.Errorf("expected statusB=%q, got %q", "passed", e.StatusB)
	}
	if e.DurationA != 0 {
		t.Errorf("expected durationA=0, got %d", e.DurationA)
	}
	if e.DurationB != 400 {
		t.Errorf("expected durationB=400, got %d", e.DurationB)
	}
}

func TestCompareBuildsByHistoryID_Removed(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-removed"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	// Only in build A
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "OldTest", FullName: "pkg.OldTest", Status: "passed", DurationMs: 200, HistoryID: "h4"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Category != store.DiffRemoved {
		t.Errorf("expected category=%q, got %q", store.DiffRemoved, e.Category)
	}
	if e.StatusA != "passed" {
		t.Errorf("expected statusA=%q, got %q", "passed", e.StatusA)
	}
	if e.StatusB != "" {
		t.Errorf("expected statusB empty, got %q", e.StatusB)
	}
	if e.DurationA != 200 {
		t.Errorf("expected durationA=200, got %d", e.DurationA)
	}
	if e.DurationB != 0 {
		t.Errorf("expected durationB=0, got %d", e.DurationB)
	}
}

func TestCompareBuildsByHistoryID_Unchanged(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-unchanged"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	// Same status in both builds — should be excluded from results
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "StableTest", FullName: "pkg.StableTest", Status: "passed", DurationMs: 100, HistoryID: "h5"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "StableTest", FullName: "pkg.StableTest", Status: "passed", DurationMs: 110, HistoryID: "h5"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for unchanged test, got %d", len(entries))
	}
}

func TestCompareBuildsByHistoryID_EmptyBuilds(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-empty"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if entries == nil {
		t.Error("expected non-nil slice, got nil")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestCompareBuildsByHistoryID_EmptyHistoryID(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	projectID := "cmp-nohid"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	// Rows with empty history_id should be excluded
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "NoHID", FullName: "pkg.NoHID", Status: "passed", DurationMs: 100, HistoryID: ""},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "NoHID", FullName: "pkg.NoHID", Status: "failed", DurationMs: 200, HistoryID: ""},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (empty history_id excluded), got %d", len(entries))
	}
}

func TestCompareBuildsByHistoryID_BrokenRegressed(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	// passed → broken also counts as regressed
	projectID := "cmp-broken"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "BrokenTest", FullName: "pkg.BrokenTest", Status: "passed", DurationMs: 100, HistoryID: "h6"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "BrokenTest", FullName: "pkg.BrokenTest", Status: "broken", DurationMs: 150, HistoryID: "h6"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Category != store.DiffRegressed {
		t.Errorf("expected category=%q, got %q", store.DiffRegressed, entries[0].Category)
	}
}

func TestCompareBuildsByHistoryID_BrokenFixed(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	// broken → passed counts as fixed
	projectID := "cmp-brokenfixed"
	buildIDA, buildIDB := seedCompareData(t, s, projectID)

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDA, ProjectID: projectID, TestName: "BrokenFixed", FullName: "pkg.BrokenFixed", Status: "broken", DurationMs: 100, HistoryID: "h7"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: buildIDB, ProjectID: projectID, TestName: "BrokenFixed", FullName: "pkg.BrokenFixed", Status: "passed", DurationMs: 90, HistoryID: "h7"},
	})

	entries, err := ts.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		t.Fatalf("CompareBuildsByHistoryID: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Category != store.DiffFixed {
		t.Errorf("expected category=%q, got %q", store.DiffFixed, entries[0].Category)
	}
}
