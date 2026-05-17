package main

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// backfillTestStores wires the three stores backfillFingerprints touches.
//
// It uses the function-field MockXxxStore doubles directly (not testutil.New,
// whose Projects field is a stateful in-memory store) so each test can program
// ListProjects / ListBuilds / ListFailedForFingerprinting precisely. allureCore
// is intentionally never exercised: every test here runs in dry-run mode, which
// skips BackfillFingerprints entirely.
func backfillTestStores(p *testutil.MockProjectStore, b *testutil.MockBuildStore, tr *testutil.MockTestResultStore) *bootstrap.Stores {
	return &bootstrap.Stores{
		Project:    p,
		Build:      b,
		TestResult: tr,
	}
}

// twoProjectTopology returns project/build mocks for a fixed two-project
// topology: project 1 has builds 10,11; project 2 has builds 20,21.
func twoProjectTopology() (*testutil.MockProjectStore, *testutil.MockBuildStore) {
	projects := &testutil.MockProjectStore{
		ListProjectsFn: func(_ context.Context) ([]store.Project, error) {
			return []store.Project{
				{ID: 1, Slug: "alpha"},
				{ID: 2, Slug: "beta"},
			}, nil
		},
	}
	builds := &testutil.MockBuildStore{
		ListBuildsFn: func(_ context.Context, projectID int64) ([]store.Build, error) {
			switch projectID {
			case 1:
				return []store.Build{{ID: 10, ProjectID: 1, BuildNumber: 1}, {ID: 11, ProjectID: 1, BuildNumber: 2}}, nil
			case 2:
				return []store.Build{{ID: 20, ProjectID: 2, BuildNumber: 1}, {ID: 21, ProjectID: 2, BuildNumber: 2}}, nil
			default:
				return nil, nil
			}
		},
	}
	return projects, builds
}

// TestBackfill_DryRunAllScope verifies the default (unscoped) dry-run walks
// every project and build that has failed tests and tallies them.
func TestBackfill_DryRunAllScope(t *testing.T) {
	projects, builds := twoProjectTopology()
	tr := &testutil.MockTestResultStore{
		ListFailedForFingerprintingFn: func(_ context.Context, _ int64, _ int64) ([]store.FailedTestResult, error) {
			return []store.FailedTestResult{{ID: 1}, {ID: 2}}, nil
		},
	}

	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true}, zap.NewNop())

	if res.succeeded != 4 {
		t.Errorf("succeeded: got %d, want 4 (2 projects x 2 builds)", res.succeeded)
	}
	if res.totalFailedTests != 8 {
		t.Errorf("totalFailedTests: got %d, want 8", res.totalFailedTests)
	}
	if len(res.failedBuildIDs) != 0 {
		t.Errorf("failedBuildIDs: got %v, want none", res.failedBuildIDs)
	}
}

// TestBackfill_ProjectScope verifies -project restricts the run to one project.
func TestBackfill_ProjectScope(t *testing.T) {
	projects, builds := twoProjectTopology()

	var queriedProjects []int64
	tr := &testutil.MockTestResultStore{
		ListFailedForFingerprintingFn: func(_ context.Context, projectID int64, _ int64) ([]store.FailedTestResult, error) {
			queriedProjects = append(queriedProjects, projectID)
			return []store.FailedTestResult{{ID: 1}}, nil
		},
	}

	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true, projectID: 2}, zap.NewNop())

	if res.succeeded != 2 {
		t.Errorf("succeeded: got %d, want 2 (project 2 has 2 builds)", res.succeeded)
	}
	for _, pid := range queriedProjects {
		if pid != 2 {
			t.Errorf("project scope leaked: queried project %d, want only 2", pid)
		}
	}
}

// TestBackfill_SinceBuildScope verifies -since-build skips builds with a
// smaller id.
func TestBackfill_SinceBuildScope(t *testing.T) {
	projects, builds := twoProjectTopology()

	var queriedBuilds []int64
	tr := &testutil.MockTestResultStore{
		ListFailedForFingerprintingFn: func(_ context.Context, _ int64, buildID int64) ([]store.FailedTestResult, error) {
			queriedBuilds = append(queriedBuilds, buildID)
			return []store.FailedTestResult{{ID: 1}}, nil
		},
	}

	// Builds are 10,11,20,21; since-build=20 keeps only 20 and 21.
	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true, sinceBuild: 20}, zap.NewNop())

	if res.succeeded != 2 {
		t.Errorf("succeeded: got %d, want 2 (builds 20,21)", res.succeeded)
	}
	for _, bid := range queriedBuilds {
		if bid < 20 {
			t.Errorf("since-build scope leaked: queried build %d, want >= 20", bid)
		}
	}
}

// TestBackfill_ContinueOnError verifies a per-build failure is logged and the
// run continues, with the failed build id recorded in the summary.
func TestBackfill_ContinueOnError(t *testing.T) {
	projects, builds := twoProjectTopology()
	tr := &testutil.MockTestResultStore{
		// Build 11 fails its failed-test lookup; every other build succeeds.
		ListFailedForFingerprintingFn: func(_ context.Context, _ int64, buildID int64) ([]store.FailedTestResult, error) {
			if buildID == 11 {
				return nil, errors.New("simulated query failure")
			}
			return []store.FailedTestResult{{ID: 1}}, nil
		},
	}

	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true}, zap.NewNop())

	// 4 builds total, 1 failed -> 3 succeeded, 1 in failedBuildIDs.
	if res.succeeded != 3 {
		t.Errorf("succeeded: got %d, want 3", res.succeeded)
	}
	if len(res.failedBuildIDs) != 1 || res.failedBuildIDs[0] != 11 {
		t.Errorf("failedBuildIDs: got %v, want [11]", res.failedBuildIDs)
	}
}

// TestBackfill_ListBuildsErrorSkipsProject verifies a ListBuilds failure skips
// that project but does not abort the whole run.
func TestBackfill_ListBuildsErrorSkipsProject(t *testing.T) {
	projects := &testutil.MockProjectStore{
		ListProjectsFn: func(_ context.Context) ([]store.Project, error) {
			return []store.Project{{ID: 1, Slug: "alpha"}, {ID: 2, Slug: "beta"}}, nil
		},
	}
	builds := &testutil.MockBuildStore{
		ListBuildsFn: func(_ context.Context, projectID int64) ([]store.Build, error) {
			if projectID == 1 {
				return nil, errors.New("simulated list builds failure")
			}
			return []store.Build{{ID: 20, ProjectID: 2, BuildNumber: 1}}, nil
		},
	}
	tr := &testutil.MockTestResultStore{
		ListFailedForFingerprintingFn: func(_ context.Context, _ int64, _ int64) ([]store.FailedTestResult, error) {
			return []store.FailedTestResult{{ID: 1}}, nil
		},
	}

	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true}, zap.NewNop())

	// Project 1 skipped entirely; project 2's single build still processed.
	if res.succeeded != 1 {
		t.Errorf("succeeded: got %d, want 1 (project 2 build 20)", res.succeeded)
	}
}

// TestBackfill_BuildsWithNoFailuresSkipped verifies builds with zero failed
// tests are not counted as processed.
func TestBackfill_BuildsWithNoFailuresSkipped(t *testing.T) {
	projects, builds := twoProjectTopology()
	tr := &testutil.MockTestResultStore{
		// Only build 10 has failed tests; the rest are clean.
		ListFailedForFingerprintingFn: func(_ context.Context, _ int64, buildID int64) ([]store.FailedTestResult, error) {
			if buildID == 10 {
				return []store.FailedTestResult{{ID: 1}, {ID: 2}, {ID: 3}}, nil
			}
			return nil, nil
		},
	}

	res := backfillFingerprints(context.Background(), nil, backfillTestStores(projects, builds, tr),
		backfillFlags{dryRun: true}, zap.NewNop())

	if res.succeeded != 1 {
		t.Errorf("succeeded: got %d, want 1 (only build 10 has failures)", res.succeeded)
	}
	if res.totalFailedTests != 3 {
		t.Errorf("totalFailedTests: got %d, want 3", res.totalFailedTests)
	}
}
