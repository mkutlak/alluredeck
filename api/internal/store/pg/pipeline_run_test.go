package pg_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// runBuild uploads build order into projectID with the given CI metadata and
// stats, the way a CI job does.
func (f *fixture) runBuild(projectID int64, order int, ci store.CIMetadata, passed, failed, skipped int) int64 {
	f.t.Helper()
	id := f.buildIn(projectID, order)
	if err := f.builds.UpdateBuildCIMetadata(f.ctx, projectID, order, ci); err != nil {
		f.t.Fatalf("UpdateBuildCIMetadata %d: %v", order, err)
	}
	if err := f.builds.UpdateBuildStats(f.ctx, projectID, order, store.BuildStats{
		Passed: passed, Failed: failed, Skipped: skipped, Total: passed + failed + skipped, DurationMs: 1500,
	}); err != nil {
		f.t.Fatalf("UpdateBuildStats %d: %v", order, err)
	}
	return id
}

// TestGetPipelineRun_ReturnsEveryBuildOfTheRun: the rows have the shape the
// feed query returns (group, suite, stats, CI links) so groupPipelineRuns
// builds the same response from them. A sharded suite contributes one row per
// shard, the key is the pipeline id, and neither another pipeline, another
// group, nor a different branch's key leaks in.
func TestGetPipelineRun_ReturnsEveryBuildOfTheRun(t *testing.T) {
	f := newFixture(t) // the parent group
	other := newFixture(t)
	sharded, single := f.newProject(f.id), f.newProject(f.id)
	otherChild := other.newProject(other.id)
	const runKey = "pipeline-under-test"
	ci := func(pipelineID, sha string) store.CIMetadata {
		return store.CIMetadata{
			BuildURL: "https://ci/job", Branch: "feature/x", CommitSHA: sha,
			PipelineID: pipelineID, PipelineURL: "https://ci/pipelines/" + pipelineID,
		}
	}
	shard1 := f.runBuild(sharded.ID, 1, ci(runKey, "abc1234"), 8, 2, 0)
	shard2 := f.runBuild(sharded.ID, 2, ci(runKey, "abc1234"), 10, 0, 1)
	one := f.runBuild(single.ID, 1, ci(runKey, "abc1234"), 5, 0, 0)
	f.runBuild(single.ID, 2, ci("some-other-pipeline", "abc1234"), 5, 0, 0)
	other.runBuild(otherChild.ID, 1, ci(runKey, "abc1234"), 1, 1, 0)

	rows, err := pg.NewPipelineStore(f.s).GetPipelineRun(f.ctx, f.id, runKey)
	if err != nil {
		t.Fatalf("GetPipelineRun: %v", err)
	}
	want := map[int64]struct {
		project            int64
		slug               string
		order              int
		passed, failed, sk int
	}{
		shard1: {sharded.ID, sharded.Slug, 1, 8, 2, 0},
		shard2: {sharded.ID, sharded.Slug, 2, 10, 0, 1},
		one:    {single.ID, single.Slug, 1, 5, 0, 0},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d (two shards + one suite): %+v", len(rows), len(want), rows)
	}
	for i := range rows {
		r := &rows[i]
		w, ok := want[r.BuildID]
		if !ok {
			t.Errorf("unexpected build %d in the run: %+v", r.BuildID, r)
			continue
		}
		if r.ProjectID != w.project || r.Slug != w.slug || r.BuildNumber != w.order {
			t.Errorf("build %d: suite = %d/%s #%d, want %d/%s #%d", r.BuildID, r.ProjectID, r.Slug, r.BuildNumber, w.project, w.slug, w.order)
		}
		if r.GroupProjectID != f.id || r.GroupSlug != f.project.Slug {
			t.Errorf("build %d: group = %d/%s, want %d/%s", r.BuildID, r.GroupProjectID, r.GroupSlug, f.id, f.project.Slug)
		}
		if r.PipelineID != runKey || r.PipelineURL != "https://ci/pipelines/"+runKey || r.CIBuildURL != "https://ci/job" ||
			r.CommitSHA != "abc1234" || r.Branch != "feature/x" {
			t.Errorf("build %d: CI metadata = %+v", r.BuildID, r)
		}
		if r.StatPassed == nil || *r.StatPassed != w.passed || r.StatFailed == nil || *r.StatFailed != w.failed ||
			r.StatSkipped == nil || *r.StatSkipped != w.sk || r.StatTotal == nil || *r.StatTotal != w.passed+w.failed+w.sk ||
			r.DurationMs == nil || *r.DurationMs != 1500 {
			t.Errorf("build %d: stats = %+v", r.BuildID, r)
		}
		if r.CreatedAt.IsZero() || r.CreatedAt.After(time.Now().Add(time.Minute)) {
			t.Errorf("build %d: created_at = %v", r.BuildID, r.CreatedAt)
		}
	}
}

// TestGetPipelineRun_CommitSHAKeysRunsWithoutAPipelineID mirrors the run key
// the feed and ListRunFailures use: the pipeline id when the build has one,
// otherwise the commit SHA. A build with a pipeline id is not reachable by its
// SHA, so one commit cannot show up as two runs.
func TestGetPipelineRun_CommitSHAKeysRunsWithoutAPipelineID(t *testing.T) {
	f := newFixture(t)
	a, b := f.newProject(f.id), f.newProject(f.id)
	const sha = "cafe0123"
	bare := store.CIMetadata{Branch: "main", CommitSHA: sha}
	withPipeline := store.CIMetadata{Branch: "main", CommitSHA: sha, PipelineID: "p-7"}
	bareA := f.runBuild(a.ID, 1, bare, 3, 0, 0)
	bareB := f.runBuild(b.ID, 1, bare, 4, 1, 0)
	f.runBuild(a.ID, 2, withPipeline, 3, 0, 0)

	ps := pg.NewPipelineStore(f.s)
	rows, err := ps.GetPipelineRun(f.ctx, f.id, sha)
	if err != nil {
		t.Fatalf("GetPipelineRun(sha): %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.BuildID] = true
		if r.PipelineID != "" || r.CommitSHA != sha {
			t.Errorf("build %d: pipeline_id=%q commit_sha=%q, want empty pipeline id and the key as sha", r.BuildID, r.PipelineID, r.CommitSHA)
		}
	}
	if len(rows) != 2 || !got[bareA] || !got[bareB] {
		t.Errorf("sha run = %+v, want exactly the two builds that carry no pipeline id", rows)
	}

	if rows, err := ps.GetPipelineRun(f.ctx, f.id, "p-7"); err != nil || len(rows) != 1 {
		t.Errorf("pipeline run p-7 = %+v, %v; want its single build", rows, err)
	}
}

// TestGetPipelineRun_UnknownRunHasNoRows: the store reports "nothing there" as
// an empty result and leaves the 404 to the handler. A key that exists only in
// another group is unknown here.
func TestGetPipelineRun_UnknownRunHasNoRows(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	f.runBuild(f.newProject(f.id).ID, 1, store.CIMetadata{PipelineID: "mine", CommitSHA: "aaa"}, 1, 0, 0)
	other.runBuild(other.newProject(other.id).ID, 1, store.CIMetadata{PipelineID: "theirs", CommitSHA: "bbb"}, 1, 0, 0)

	ps := pg.NewPipelineStore(f.s)
	for _, key := range []string{"no-such-pipeline", "theirs", "bbb"} {
		if rows, err := ps.GetPipelineRun(f.ctx, f.id, key); err != nil || len(rows) != 0 {
			t.Errorf("GetPipelineRun(%q) = %+v, %v; want no rows", key, rows, err)
		}
	}
}

// TestGetPipelineRun_MatchesTheFeedRows pins "same rows as the feed": every
// field of a run's rows is what ListAllPipelineRuns returns for that run, so
// the run page and its feed row cannot disagree.
func TestGetPipelineRun_MatchesTheFeedRows(t *testing.T) {
	f := newFixture(t)
	a, b := f.newProject(f.id), f.newProject(f.id)
	ci := store.CIMetadata{
		Provider: "gitlab", BuildURL: "https://ci/job", Branch: "main", CommitSHA: "abc1234",
		PipelineID: unique("run"), PipelineURL: "https://ci/pipelines/1",
	}
	f.runBuild(a.ID, 1, ci, 3, 1, 0)
	f.runBuild(a.ID, 2, ci, 4, 0, 1)
	f.runBuild(b.ID, 1, ci, 9, 0, 0)

	ps := pg.NewPipelineStore(f.s)
	feed, _, err := ps.ListAllPipelineRuns(f.ctx, "", []int64{f.id}, 1, 10)
	if err != nil {
		t.Fatalf("ListAllPipelineRuns: %v", err)
	}
	got, err := ps.GetPipelineRun(f.ctx, f.id, ci.PipelineID)
	if err != nil {
		t.Fatalf("GetPipelineRun: %v", err)
	}
	if len(feed) != 3 || !reflect.DeepEqual(got, feed) {
		t.Errorf("run rows = %+v\nfeed rows = %+v\nwant the identical 3 rows in the same order", got, feed)
	}
}
