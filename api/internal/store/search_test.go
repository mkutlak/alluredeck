package store_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// seedSearch creates projects, builds, and test results for search tests.
// It creates two projects ("alpha-service", "beta-service"), each with two
// builds. Only the second build of each project is marked as latest.
func seedSearch(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	logger := zap.NewNop()

	ps := store.NewProjectStore(s, logger)
	bs := store.NewBuildStore(s, logger)
	ts := store.NewTestResultStore(s, logger)

	for _, id := range []string{"alpha-service", "beta-service"} {
		if err := ps.CreateProject(ctx, id); err != nil {
			t.Fatalf("create project %s: %v", id, err)
		}
		// Build 1 (not latest)
		if err := bs.InsertBuild(ctx, id, 1); err != nil {
			t.Fatalf("insert build 1 for %s: %v", id, err)
		}
		// Build 2 (latest)
		if err := bs.InsertBuild(ctx, id, 2); err != nil {
			t.Fatalf("insert build 2 for %s: %v", id, err)
		}
		if err := bs.SetLatest(ctx, id, 2); err != nil {
			t.Fatalf("set latest for %s: %v", id, err)
		}
	}

	// Get build IDs for inserting test results.
	alphaBuild1, _ := ts.GetBuildID(ctx, "alpha-service", 1)
	alphaBuild2, _ := ts.GetBuildID(ctx, "alpha-service", 2)
	betaBuild2, _ := ts.GetBuildID(ctx, "beta-service", 2)

	results := []store.TestResult{
		// alpha build 1 (NOT latest) — should not appear in search
		{BuildID: alphaBuild1, ProjectID: "alpha-service", TestName: "OldTest", FullName: "com.old.OldTest", Status: "passed"},
		// alpha build 2 (latest)
		{BuildID: alphaBuild2, ProjectID: "alpha-service", TestName: "LoginTest", FullName: "com.auth.LoginTest", Status: "passed"},
		{BuildID: alphaBuild2, ProjectID: "alpha-service", TestName: "LogoutTest", FullName: "com.auth.LogoutTest", Status: "failed"},
		// beta build 2 (latest)
		{BuildID: betaBuild2, ProjectID: "beta-service", TestName: "LoginFlow", FullName: "com.beta.LoginFlow", Status: "broken"},
	}
	if err := ts.InsertBatch(ctx, results); err != nil {
		t.Fatalf("insert test results: %v", err)
	}
}

func TestSearchStore_SearchProjects_SubstringMatch(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchProjects(context.Background(), "alpha", 10)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 project, got %d", len(results))
	}
	if results[0].ID != "alpha-service" {
		t.Errorf("expected alpha-service, got %s", results[0].ID)
	}
}

func TestSearchStore_SearchProjects_CaseInsensitive(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchProjects(context.Background(), "ALPHA", 10)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 project (case-insensitive), got %d", len(results))
	}
}

func TestSearchStore_SearchProjects_NoMatch(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchProjects(context.Background(), "nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 projects, got %d", len(results))
	}
}

func TestSearchStore_SearchProjects_Limit(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	// Both projects match "service", but limit to 1.
	results, err := ss.SearchProjects(context.Background(), "service", 1)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 project (limit), got %d", len(results))
	}
}

func TestSearchStore_SearchProjects_EscapesWildcards(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	// "%" should not act as wildcard — no project name contains literal "%".
	results, err := ss.SearchProjects(context.Background(), "%", 10)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 projects when searching for literal '%%', got %d", len(results))
	}
}

func TestSearchStore_SearchTests_SubstringMatch(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchTests(context.Background(), "Login", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	// Should match LoginTest (alpha) and LoginFlow (beta) — both in latest builds.
	if len(results) != 2 {
		t.Fatalf("expected 2 test matches, got %d", len(results))
	}
}

func TestSearchStore_SearchTests_LatestOnly(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	// "OldTest" exists only in build 1 (not latest) — should not be found.
	results, err := ss.SearchTests(context.Background(), "OldTest", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 tests (not latest), got %d", len(results))
	}
}

func TestSearchStore_SearchTests_FullNameMatch(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	// Search by full_name prefix.
	results, err := ss.SearchTests(context.Background(), "com.auth", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 tests matching full_name, got %d", len(results))
	}
}

func TestSearchStore_SearchTests_Limit(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchTests(context.Background(), "Login", 1)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 test (limit), got %d", len(results))
	}
}

func TestSearchStore_SearchTests_NoMatch(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchTests(context.Background(), "zzz_nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 tests, got %d", len(results))
	}
}

func TestSearchStore_SearchTests_CaseInsensitive(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchTests(context.Background(), "login", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 tests (case-insensitive), got %d", len(results))
	}
}

func TestSearchStore_SearchTests_IncludesStatus(t *testing.T) {
	s := openTestStore(t)
	seedSearch(t, s)
	ss := store.NewSearchStore(s, zap.NewNop())

	results, err := ss.SearchTests(context.Background(), "LogoutTest", 10)
	if err != nil {
		t.Fatalf("SearchTests: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 test, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Errorf("expected status 'failed', got %q", results[0].Status)
	}
	if results[0].ProjectID != "alpha-service" {
		t.Errorf("expected project_id 'alpha-service', got %q", results[0].ProjectID)
	}
}
