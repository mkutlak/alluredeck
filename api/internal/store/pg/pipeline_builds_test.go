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

// pipelineBuildsFixture creates a project holding `builds` builds that all
// share one ci_pipeline_id — the Playwright shard shape, where every shard job
// uploads its own build under a single pipeline id — plus one build under a
// different pipeline id that must never be returned.
func pipelineBuildsFixture(t *testing.T, namePrefix string, builds int) (*pg.PipelineStore, int64, string) {
	t.Helper()

	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), proj.ID) })

	pipelineID := slug + "-pipeline"
	for i := 1; i <= builds; i++ {
		if err := buildStore.InsertBuild(ctx, proj.ID, i); err != nil {
			t.Fatalf("InsertBuild(%d): %v", i, err)
		}
		if err := buildStore.UpdateBuildCIMetadata(ctx, proj.ID, i, store.CIMetadata{PipelineID: pipelineID}); err != nil {
			t.Fatalf("UpdateBuildCIMetadata(%d): %v", i, err)
		}
	}
	// A neighbouring build from another pipeline, to prove the filter holds.
	if err := buildStore.InsertBuild(ctx, proj.ID, builds+1); err != nil {
		t.Fatalf("InsertBuild(other): %v", err)
	}
	if err := buildStore.UpdateBuildCIMetadata(ctx, proj.ID, builds+1, store.CIMetadata{PipelineID: pipelineID + "-other"}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata(other): %v", err)
	}

	return pg.NewPipelineStore(s), proj.ID, pipelineID
}

// TestListBuildsByPipelineID_FetchesOneMoreThanLimit pins the truncation
// protocol. The query used to be unbounded, so a runaway pipeline id could pull
// an unlimited number of build rows into memory. It now takes a limit — and
// returns up to limit+1 rows, so the caller can tell "exactly limit builds" from
// "more than limit builds" and say so, instead of silently presenting a
// truncated list as complete.
func TestListBuildsByPipelineID_FetchesOneMoreThanLimit(t *testing.T) {
	ps, projectID, pipelineID := pipelineBuildsFixture(t, "pipeline-builds-limit", 6)
	ctx := context.Background()

	got, err := ps.ListBuildsByPipelineID(ctx, projectID, pipelineID, 3)
	if err != nil {
		t.Fatalf("ListBuildsByPipelineID: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d builds, want 4 (limit 3 + 1 truncation probe)", len(got))
	}
	for i, b := range got {
		if b.BuildNumber != i+1 {
			t.Errorf("build[%d].BuildNumber = %d, want %d — rows must come back in build_order ASC", i, b.BuildNumber, i+1)
		}
		if b.CIPipelineID == nil || *b.CIPipelineID != pipelineID {
			t.Errorf("build[%d] belongs to another pipeline: %v", i, b.CIPipelineID)
		}
	}
}

// TestListBuildsByPipelineID_UnderLimitReturnsEverything is the other half of
// the protocol: when the pipeline has no more builds than the caller asked for,
// the extra row simply does not exist and the caller sees a complete list.
func TestListBuildsByPipelineID_UnderLimitReturnsEverything(t *testing.T) {
	ps, projectID, pipelineID := pipelineBuildsFixture(t, "pipeline-builds-under", 2)
	ctx := context.Background()

	got, err := ps.ListBuildsByPipelineID(ctx, projectID, pipelineID, 50)
	if err != nil {
		t.Fatalf("ListBuildsByPipelineID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d builds, want 2", len(got))
	}
}

// TestListBuildsByPipelineID_NonPositiveLimitFallsBackToDefault guards the
// footgun a bare "LIMIT $3" would create: PostgreSQL rejects a negative LIMIT
// outright and reads LIMIT 0 as "no rows", so a caller that forgot to set a
// limit would get an error or an empty result rather than data. A non-positive
// limit means "unspecified" and is served with the store's own default.
func TestListBuildsByPipelineID_NonPositiveLimitFallsBackToDefault(t *testing.T) {
	ps, projectID, pipelineID := pipelineBuildsFixture(t, "pipeline-builds-zero", 3)
	ctx := context.Background()

	for _, limit := range []int{0, -1} {
		got, err := ps.ListBuildsByPipelineID(ctx, projectID, pipelineID, limit)
		if err != nil {
			t.Fatalf("ListBuildsByPipelineID(limit=%d): %v", limit, err)
		}
		if len(got) != 3 {
			t.Errorf("limit=%d: got %d builds, want 3", limit, len(got))
		}
	}
}
