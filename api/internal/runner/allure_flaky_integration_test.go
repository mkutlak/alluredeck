package runner

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestStoreAndPruneBuild_Allure3RetriedFlaky_Postgres runs the real Allure 3
// output through the real stores (TEST_POSTGRES_URL): the retried flaky test
// must end up as ONE flaky test_results row (InsertBatch's stability row and
// InsertBatchFull's raw row merge on the raw historyId, flaky OR-ed), so the
// build tallies, the stability listing and the Flaky Impact card all see it once.
func TestStoreAndPruneBuild_Allure3RetriedFlaky_Postgres(t *testing.T) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := pg.Open(ctx, &config.Config{DatabaseURL: url, RunMigrations: true})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	logger := zap.NewNop()
	projects, builds, results := pg.NewProjectStore(s, logger), pg.NewBuildStore(s, logger), pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("flaky-it-%d", time.Now().UnixNano())
	p, err := projects.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projects.DeleteProject(context.Background(), p.ID) })
	order, err := builds.ReserveBuild(ctx, p.ID)
	if err != nil {
		t.Fatalf("ReserveBuild: %v", err)
	}

	tmpDir := t.TempDir()
	a := &Allure{
		cfg: &config.Config{ProjectsPath: tmpDir}, store: stabilityStore(t, loadAllure3Fixture(t, tmpDir)),
		buildStore: builds, testResultStore: results, logger: logger,
	}
	if err := a.storeAndPruneBuild(ctx, p.ID, slug, slug, "batch1", tmpDir, order, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	b, err := builds.GetBuildByNumber(ctx, p.ID, order)
	if err != nil {
		t.Fatalf("GetBuildByNumber: %v", err)
	}
	flakyCount, retriedCount := -1, -1 // -1: column unset
	if b.FlakyCount != nil {
		flakyCount = *b.FlakyCount
	}
	if b.RetriedCount != nil {
		retriedCount = *b.RetriedCount
	}
	if flakyCount != 1 || retriedCount != 1 {
		t.Errorf("builds flaky_count/retried_count = %d/%d, want 1/1", flakyCount, retriedCount)
	}

	var rows int
	var flaky bool
	var retries int
	if err := s.Pool().QueryRow(ctx,
		"SELECT count(*), bool_or(flaky), max(retries) FROM test_results WHERE build_id=$1 AND full_name='suite.flakyTest'",
		b.ID).Scan(&rows, &flaky, &retries); err != nil {
		t.Fatalf("query test_results: %v", err)
	}
	if rows != 1 || !flaky || retries != 1 {
		t.Errorf("flakyTest rows = %d (flaky=%v, retries=%d), want exactly 1 flaky row with retries 1", rows, flaky, retries)
	}

	stability, err := results.ListStabilityByBuild(ctx, p.ID, b.ID)
	if err != nil {
		t.Fatalf("ListStabilityByBuild: %v", err)
	}
	if len(stability) != 1 || stability[0].FullName != "suite.flakyTest" {
		t.Errorf("stability listing = %+v, want the flaky test exactly once", stability)
	}

	impact, err := pg.NewAnalyticsStore(s).ListFlakyImpact(ctx, p.ID, nil, 10, 10)
	if err != nil {
		t.Fatalf("ListFlakyImpact: %v", err)
	}
	if len(impact) != 1 || impact[0].FullName != "suite.flakyTest" || impact[0].FlakyCount != 1 {
		t.Errorf("flaky impact = %+v, want suite.flakyTest with flaky_count 1", impact)
	}
}
