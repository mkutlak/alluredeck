package pg_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestListFlakyImpact_AggregatesFlakyAndRetryData seeds three builds for one
// project: two flaky/retried runs of "t1" (in builds 1 and 3) plus one clean
// run (build 2), and one never-flaky test "t2". It verifies ListFlakyImpact
// aggregates flaky_count, retry_sum, wasted_ms (retries*duration_ms) and
// failure_rate for "t1" across the window, reports the correct first/last
// seen build order/id and the last-seen build's CI URL, and excludes "t2"
// entirely (HAVING flaky_count > 0).
func TestListFlakyImpact_AggregatesFlakyAndRetryData(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)
	analyticsStore := pg.NewAnalyticsStore(s)

	slug := fmt.Sprintf("test-flaky-impact-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// Build 1: t1 flaky (2 retries, 1000ms), t2 clean.
	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild 1: %v", err)
	}
	build1ID, err := trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID 1: %v", err)
	}

	// Build 2: t1 clean but failed (no retries), t2 clean.
	if err := buildStore.InsertBuild(ctx, projectID, 2); err != nil {
		t.Fatalf("InsertBuild 2: %v", err)
	}
	build2ID, err := trStore.GetBuildID(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("GetBuildID 2: %v", err)
	}

	// Build 3: t1 flaky again (1 retry, 800ms), t2 clean. Also carries a CI URL.
	if err := buildStore.InsertBuild(ctx, projectID, 3); err != nil {
		t.Fatalf("InsertBuild 3: %v", err)
	}
	build3ID, err := trStore.GetBuildID(ctx, projectID, 3)
	if err != nil {
		t.Fatalf("GetBuildID 3: %v", err)
	}
	const ciURL = "https://ci.example.com/build/3"
	if err := buildStore.UpdateBuildCIMetadata(ctx, projectID, 3, store.CIMetadata{BuildURL: ciURL}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	results := []store.TestResult{
		{BuildID: build1ID, ProjectID: projectID, TestName: "t1", FullName: "spec/foo.ts > t1", Status: "passed", DurationMs: 1000, Flaky: true, Retries: 2},
		{BuildID: build1ID, ProjectID: projectID, TestName: "t2", FullName: "spec/foo.ts > t2", Status: "passed", DurationMs: 500, Flaky: false, Retries: 0},
		{BuildID: build2ID, ProjectID: projectID, TestName: "t1", FullName: "spec/foo.ts > t1", Status: "failed", DurationMs: 1200, Flaky: false, Retries: 0},
		{BuildID: build2ID, ProjectID: projectID, TestName: "t2", FullName: "spec/foo.ts > t2", Status: "passed", DurationMs: 500, Flaky: false, Retries: 0},
		{BuildID: build3ID, ProjectID: projectID, TestName: "t1", FullName: "spec/foo.ts > t1", Status: "passed", DurationMs: 800, Flaky: true, Retries: 1},
		{BuildID: build3ID, ProjectID: projectID, TestName: "t2", FullName: "spec/foo.ts > t2", Status: "passed", DurationMs: 500, Flaky: false, Retries: 0},
	}
	if err := trStore.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	impact, err := analyticsStore.ListFlakyImpact(ctx, projectID, nil, 10, 10)
	if err != nil {
		t.Fatalf("ListFlakyImpact: %v", err)
	}
	if len(impact) != 1 {
		t.Fatalf("impact count: got %d, want 1 (only t1 has flaky occurrences)", len(impact))
	}

	fi := impact[0]
	if fi.FullName != "spec/foo.ts > t1" {
		t.Errorf("FullName: got %q, want %q", fi.FullName, "spec/foo.ts > t1")
	}
	if fi.FlakyCount != 2 {
		t.Errorf("FlakyCount: got %d, want 2", fi.FlakyCount)
	}
	if fi.RetrySum != 3 {
		t.Errorf("RetrySum: got %d, want 3", fi.RetrySum)
	}
	const wantWastedMs = 2*1000 + 0*1200 + 1*800
	if fi.WastedMs != wantWastedMs {
		t.Errorf("WastedMs: got %d, want %d", fi.WastedMs, wantWastedMs)
	}
	wantFailureRate := 1.0 / 3.0
	if math.Abs(fi.FailureRate-wantFailureRate) > 0.001 {
		t.Errorf("FailureRate: got %f, want ~%f", fi.FailureRate, wantFailureRate)
	}
	if fi.Runs != 3 {
		t.Errorf("Runs: got %d, want 3", fi.Runs)
	}
	if fi.BuildsAffected != 2 {
		t.Errorf("BuildsAffected: got %d, want 2", fi.BuildsAffected)
	}
	if fi.FirstSeenBuildOrder != 1 {
		t.Errorf("FirstSeenBuildOrder: got %d, want 1", fi.FirstSeenBuildOrder)
	}
	if fi.FirstSeenBuildID != build1ID {
		t.Errorf("FirstSeenBuildID: got %d, want %d", fi.FirstSeenBuildID, build1ID)
	}
	if fi.LastSeenBuildOrder != 3 {
		t.Errorf("LastSeenBuildOrder: got %d, want 3", fi.LastSeenBuildOrder)
	}
	if fi.LastSeenBuildID != build3ID {
		t.Errorf("LastSeenBuildID: got %d, want %d", fi.LastSeenBuildID, build3ID)
	}
	if fi.CIBuildURL != ciURL {
		t.Errorf("CIBuildURL: got %q, want %q", fi.CIBuildURL, ciURL)
	}
	if fi.LastSeenAt.IsZero() {
		t.Error("LastSeenAt: got zero value, want build 3's created_at")
	}

	// Sanity: LastSeenAt matches build 3's own created_at within a small tolerance.
	build3, err := buildStore.GetBuildByID(ctx, projectID, build3ID)
	if err != nil {
		t.Fatalf("GetBuildByID build3: %v", err)
	}
	if diff := fi.LastSeenAt.Sub(build3.CreatedAt); diff > time.Second || diff < -time.Second {
		t.Errorf("LastSeenAt: got %v, want ~%v (build3.CreatedAt)", fi.LastSeenAt, build3.CreatedAt)
	}
}

// TestListFlakyImpact_RespectsLimitAndBranch verifies the limit parameter caps
// the result count and that a branchID filter excludes tests whose builds
// belong to a different branch.
func TestListFlakyImpact_RespectsLimitAndBranch(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)
	branchStore := pg.NewBranchStore(s)
	analyticsStore := pg.NewAnalyticsStore(s)

	slug := fmt.Sprintf("test-flaky-impact-branch-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	mainBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "main")
	if err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	featureBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "feature")
	if err != nil {
		t.Fatalf("GetOrCreate feature: %v", err)
	}

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild 1: %v", err)
	}
	build1ID, err := trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID 1: %v", err)
	}
	if err := buildStore.UpdateBuildBranchID(ctx, projectID, 1, mainBranch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID 1: %v", err)
	}

	if err := buildStore.InsertBuild(ctx, projectID, 2); err != nil {
		t.Fatalf("InsertBuild 2: %v", err)
	}
	build2ID, err := trStore.GetBuildID(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("GetBuildID 2: %v", err)
	}
	if err := buildStore.UpdateBuildBranchID(ctx, projectID, 2, featureBranch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID 2: %v", err)
	}

	results := []store.TestResult{
		{BuildID: build1ID, ProjectID: projectID, TestName: "main-only", FullName: "spec/branch.ts > main-only", Status: "passed", DurationMs: 100, Flaky: true, Retries: 1},
		{BuildID: build2ID, ProjectID: projectID, TestName: "feature-only", FullName: "spec/branch.ts > feature-only", Status: "passed", DurationMs: 100, Flaky: true, Retries: 1},
	}
	if err := trStore.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Unfiltered: both flaky tests show up.
	all, err := analyticsStore.ListFlakyImpact(ctx, projectID, nil, 10, 10)
	if err != nil {
		t.Fatalf("ListFlakyImpact (unfiltered): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered impact count: got %d, want 2", len(all))
	}

	// Limit caps the number of rows returned.
	limited, err := analyticsStore.ListFlakyImpact(ctx, projectID, nil, 10, 1)
	if err != nil {
		t.Fatalf("ListFlakyImpact (limit=1): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited impact count: got %d, want 1", len(limited))
	}

	// Branch filter: only the main-branch build's test shows up.
	mainOnly, err := analyticsStore.ListFlakyImpact(ctx, projectID, &mainBranch.ID, 10, 10)
	if err != nil {
		t.Fatalf("ListFlakyImpact (branch=main): %v", err)
	}
	if len(mainOnly) != 1 {
		t.Fatalf("main-branch impact count: got %d, want 1", len(mainOnly))
	}
	if mainOnly[0].FullName != "spec/branch.ts > main-only" {
		t.Errorf("main-branch impact FullName: got %q, want %q", mainOnly[0].FullName, "spec/branch.ts > main-only")
	}
}
