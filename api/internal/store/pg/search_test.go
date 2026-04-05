//go:build integration

package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestPGSearchStore_SearchTests_FindsByTestName inserts a build + test result with
// a known test_name and verifies that searching for a substring returns it.
func TestPGSearchStore_SearchTests_FindsByTestName(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectID := fmt.Sprintf("search-test-findbyname-%d", time.Now().UnixNano())
	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	const buildNumber = 1
	if err := buildStore.InsertBuild(ctx, projectID, buildNumber); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	if err := buildStore.SetLatest(ctx, projectID, buildNumber); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	buildID, err := pg.NewTestResultStore(s, logger).GetBuildID(ctx, projectID, buildNumber)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	const testName = "LoginFlow_ValidCredentials_ShouldSucceed"
	_, execErr := s.Pool().Exec(ctx,
		`INSERT INTO test_results (build_id, project_id, history_id, test_name, full_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		buildID, projectID, "hist-001", testName, "com.example."+testName, "passed")
	if execErr != nil {
		t.Fatalf("insert test_result: %v", execErr)
	}

	searchStore := pg.NewSearchStore(s, logger)
	results, err := searchStore.SearchTests(ctx, "LoginFlow", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}

	var found bool
	for _, r := range results {
		if r.ProjectID == projectID && r.TestName == testName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find test %q in project %q, got results: %v", testName, projectID, results)
	}
}

// TestPGSearchStore_SearchTests_ReturnsEmptyOnNoMatch verifies that searching for
// a string that matches nothing returns an empty (non-nil) slice.
func TestPGSearchStore_SearchTests_ReturnsEmptyOnNoMatch(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	searchStore := pg.NewSearchStore(s, logger)
	results, err := searchStore.SearchTests(ctx, "zzznomatchxxx99999", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if results == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %v", len(results), results)
	}
}

// TestPGSearchStore_SearchTests_OnlyFromLatestBuild inserts two builds for the same
// project, marks only one as latest, and verifies that search only returns results
// from the latest build.
func TestPGSearchStore_SearchTests_OnlyFromLatestBuild(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectID := fmt.Sprintf("search-test-latestonly-%d", time.Now().UnixNano())
	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	testResultStore := pg.NewTestResultStore(s, logger)

	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// Build 1 — not latest.
	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild 1: %v", err)
	}
	buildID1, err := testResultStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID 1: %v", err)
	}
	_, err = s.Pool().Exec(ctx,
		`INSERT INTO test_results (build_id, project_id, history_id, test_name, full_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		buildID1, projectID, "hist-old", "OldBuildOnlyTest", "com.example.OldBuildOnlyTest", "failed")
	if err != nil {
		t.Fatalf("insert old test_result: %v", err)
	}

	// Build 2 — latest.
	if err := buildStore.InsertBuild(ctx, projectID, 2); err != nil {
		t.Fatalf("InsertBuild 2: %v", err)
	}
	if err := buildStore.SetLatest(ctx, projectID, 2); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}
	buildID2, err := testResultStore.GetBuildID(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("GetBuildID 2: %v", err)
	}
	_, err = s.Pool().Exec(ctx,
		`INSERT INTO test_results (build_id, project_id, history_id, test_name, full_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		buildID2, projectID, "hist-new", "LatestBuildTest", "com.example.LatestBuildTest", "passed")
	if err != nil {
		t.Fatalf("insert latest test_result: %v", err)
	}

	searchStore := pg.NewSearchStore(s, logger)

	// "OldBuildOnlyTest" must not appear — build 1 is not latest.
	oldResults, err := searchStore.SearchTests(ctx, "OldBuildOnly", 10)
	if err != nil {
		t.Fatalf("SearchTests old: %v", err)
	}
	for _, r := range oldResults {
		if r.ProjectID == projectID {
			t.Errorf("expected no result from non-latest build, got: %v", r)
		}
	}

	// "LatestBuildTest" must appear — build 2 is latest.
	latestResults, err := searchStore.SearchTests(ctx, "LatestBuild", 10)
	if err != nil {
		t.Fatalf("SearchTests latest: %v", err)
	}
	var foundLatest bool
	for _, r := range latestResults {
		if r.ProjectID == projectID && r.TestName == "LatestBuildTest" {
			foundLatest = true
			break
		}
	}
	if !foundLatest {
		t.Errorf("expected result from latest build, got: %v", latestResults)
	}
}

// TestPGSearchStore_SearchProjects_FindsByID creates a project with a known ID and
// verifies that searching for a substring of the ID returns it.
func TestPGSearchStore_SearchProjects_FindsByID(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	projectID := fmt.Sprintf("search-proj-findbyid-%s", unique)
	projectStore := pg.NewProjectStore(s, logger)

	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	searchStore := pg.NewSearchStore(s, logger)
	// Search by the unique suffix to avoid collisions with other projects.
	results, err := searchStore.SearchProjects(ctx, unique, 10)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}

	var found bool
	for _, r := range results {
		if r.ID == projectID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected project %q in results, got: %v", projectID, results)
	}
}
