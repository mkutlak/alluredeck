package pg_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// originTest is one test result of a seeded build.
type originTest struct{ history, status string }

// suiteBuild uploads build order into projectID on branch (created on first
// use, so a project's first branch is its default), the way CI does. A
// non-empty pipelineID puts the build in that run; baseline builds pass "".
// Every test gets history_id = its history string and a distinct full name.
func (f *fixture) suiteBuild(projectID int64, order int, branch, pipelineID string, tests ...originTest) int64 {
	f.t.Helper()
	var ci *store.CIMetadata
	if pipelineID != "" {
		ci = &store.CIMetadata{PipelineID: pipelineID, CommitSHA: "cafe1234", Branch: branch}
	}
	return f.suiteBuildCI(projectID, order, branch, ci, tests...)
}

// suiteBuildSHA is suiteBuild for a build that carries a commit SHA and no
// pipeline id, so its run key is the SHA.
func (f *fixture) suiteBuildSHA(projectID int64, order int, branch, sha string, tests ...originTest) int64 {
	f.t.Helper()
	return f.suiteBuildCI(projectID, order, branch, &store.CIMetadata{CommitSHA: sha, Branch: branch}, tests...)
}

func (f *fixture) suiteBuildCI(projectID int64, order int, branch string, ci *store.CIMetadata, tests ...originTest) int64 {
	f.t.Helper()
	id := f.buildIn(projectID, order)
	br, _, err := f.branches.GetOrCreate(f.ctx, projectID, branch)
	if err != nil {
		f.t.Fatalf("GetOrCreate branch %q: %v", branch, err)
	}
	if err := f.builds.UpdateBuildBranchID(f.ctx, projectID, order, br.ID); err != nil {
		f.t.Fatalf("UpdateBuildBranchID: %v", err)
	}
	if ci != nil {
		if err := f.builds.UpdateBuildCIMetadata(f.ctx, projectID, order, *ci); err != nil {
			f.t.Fatalf("UpdateBuildCIMetadata: %v", err)
		}
	}
	rs := make([]store.TestResult, 0, len(tests))
	for _, tc := range tests {
		r := f.result(id, "test "+tc.history, tc.status, tc.history)
		r.ProjectID = projectID
		rs = append(rs, r)
	}
	f.insert(rs...)
	return id
}

// fingerprint tags the result of history in buildID with a defect fingerprint
// of the given category, the way the defect pipeline links them.
func (f *fixture) fingerprint(projectID, buildID int64, history, category string) {
	f.t.Helper()
	var fpID string
	if err := f.s.Pool().QueryRow(f.ctx, `
		INSERT INTO defect_fingerprints
			(project_id, fingerprint_hash, category, first_seen_build_id, last_seen_build_id)
		VALUES ($1, $2, $3, $4, $4) RETURNING id::text`,
		projectID, unique("fp"), category, buildID).Scan(&fpID); err != nil {
		f.t.Fatalf("insert fingerprint: %v", err)
	}
	f.exec("UPDATE test_results SET defect_fingerprint_id=$1 WHERE build_id=$2 AND history_id=$3", fpID, buildID, history)
}

// tags indexes the failures by suite, build_order and history id.
func tags(t *testing.T, rows []store.RunFailureRow) map[[3]string]store.RunFailureRow {
	t.Helper()
	got := map[[3]string]store.RunFailureRow{}
	for i := range rows {
		r := &rows[i]
		k := [3]string{r.Slug, strconv.Itoa(r.BuildNumber), r.HistoryID}
		if _, dup := got[k]; dup {
			t.Fatalf("two failure rows for %v", k)
		}
		got[k] = *r
	}
	return got
}

// wantTags checks every expected origin and that nothing unexpected failed.
func wantTags(t *testing.T, rows []store.RunFailureRow, want map[[3]string]string) {
	t.Helper()
	got := tags(t, rows)
	for k, origin := range want {
		r, ok := got[k]
		if !ok {
			t.Errorf("%v: no failure row; rows = %+v", k, rows)
			continue
		}
		if r.Origin != origin {
			t.Errorf("%v: origin = %q, want %q", k, r.Origin, origin)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d failure rows, want %d: %+v", len(got), len(want), rows)
	}
}

// TestListRunFailures_Origin pins the baseline rule for one suite whose
// default branch is "main" and whose run is on "feature":
//   - new: passed in the baseline build; inherited: failed or broke there.
//   - unknown: absent from the baseline, skipped there, of unknown status
//     there, or without a history id (nothing to match on).
//   - the baseline is the latest build OLDER than the run's build by
//     build_order (the older build is inserted last on purpose, so its
//     database id is the highest), never a newer one, and never another branch.
func TestListRunFailures_Origin(t *testing.T) {
	f := newFixture(t) // the group
	web := f.newProject(f.id)
	const run = "run-origin"

	// Builds are inserted out of build_order on purpose: the baseline is
	// order 2, uploaded BEFORE order 1, so a lookup by database id would
	// pick the wrong one.
	f.suiteBuild(web.ID, 2, "main", "",
		originTest{"passed-there", "passed"},
		originTest{"failed-there", "failed"},
		originTest{"broken-there", "broken"},
		originTest{"skipped-there", "skipped"},
		originTest{"unknown-there", "unknown"},
		originTest{"flipped-later", "passed"},
	)
	f.suiteBuild(web.ID, 1, "main", "",
		originTest{"passed-there", "failed"},       // older than the baseline: ignored
		originTest{"absent-in-baseline", "passed"}, // only in an older build: absent from the baseline
	)
	f.suiteBuild(web.ID, 4, "feature", "",
		originTest{"passed-there", "failed"}, // another branch's build between: ignored
	)
	runBuildID := f.suiteBuild(web.ID, 5, "feature", run,
		originTest{"passed-there", "failed"},
		originTest{"failed-there", "failed"},
		originTest{"broken-there", "broken"},
		originTest{"skipped-there", "failed"},
		originTest{"unknown-there", "failed"},
		originTest{"absent-in-baseline", "failed"},
		originTest{"brand-new", "failed"},
		originTest{"flipped-later", "failed"},
	)
	f.suiteBuild(web.ID, 6, "main", "", // newer than the run's build: ignored
		originTest{"flipped-later", "failed"},
	)
	// A failing test with no history id cannot be matched to anything.
	r := f.result(runBuildID, "no history", "failed", "")
	r.ProjectID = web.ID
	f.insert(r)

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	slug := web.Slug
	wantTags(t, rows, map[[3]string]string{
		{slug, "5", "passed-there"}:       store.FailureOriginNew,
		{slug, "5", "failed-there"}:       store.FailureOriginInherited,
		{slug, "5", "broken-there"}:       store.FailureOriginInherited,
		{slug, "5", "skipped-there"}:      store.FailureOriginUnknown,
		{slug, "5", "unknown-there"}:      store.FailureOriginUnknown,
		{slug, "5", "absent-in-baseline"}: store.FailureOriginUnknown,
		{slug, "5", "brand-new"}:          store.FailureOriginUnknown,
		{slug, "5", "flipped-later"}:      store.FailureOriginNew,
		{slug, "5", ""}:                   store.FailureOriginUnknown,
	})
}

// TestListRunFailures_OriginUnknownWithoutABaselineBuild: no default-branch
// build older than the run's means nothing to compare with. That covers a
// project with no default branch at all (builds without a branch), a default
// branch whose only builds are newer, and a run's first-ever build.
func TestListRunFailures_OriginUnknownWithoutABaselineBuild(t *testing.T) {
	f := newFixture(t)
	const run = "run-no-baseline"
	failing := originTest{"t", "failed"}
	passing := originTest{"t", "passed"}

	noBranch := f.newProject(f.id) // builds carry no branch_id: no default branch
	id := f.buildIn(noBranch.ID, 1)
	if err := f.builds.UpdateBuildCIMetadata(f.ctx, noBranch.ID, 1, store.CIMetadata{PipelineID: run, CommitSHA: "aaa"}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}
	r := f.result(id, "t", "failed", "t")
	r.ProjectID = noBranch.ID
	f.insert(r)

	onlyNewer := f.newProject(f.id)
	f.suiteBuild(onlyNewer.ID, 1, "feature", run, failing) // first branch uploaded is the default: "feature" itself
	f.suiteBuild(onlyNewer.ID, 2, "feature", "", passing)

	first := f.newProject(f.id)
	f.suiteBuild(first.ID, 1, "main", run, failing) // on the default branch, nothing before it

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{noBranch.Slug, "1", "t"}:  store.FailureOriginUnknown,
		{onlyNewer.Slug, "1", "t"}: store.FailureOriginUnknown,
		{first.Slug, "1", "t"}:     store.FailureOriginUnknown,
	})
}

// TestListRunFailures_OriginOnTheDefaultBranch: when the failing build is
// itself on the default branch, the baseline is the previous build there.
func TestListRunFailures_OriginOnTheDefaultBranch(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-on-main"
	f.suiteBuild(web.ID, 1, "main", "",
		originTest{"regressed", "passed"},
		originTest{"still-red", "failed"},
	)
	f.suiteBuild(web.ID, 2, "main", run,
		originTest{"regressed", "failed"},
		originTest{"still-red", "failed"},
	)

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{web.Slug, "2", "regressed"}: store.FailureOriginNew,
		{web.Slug, "2", "still-red"}: store.FailureOriginInherited,
	})
}

// TestListRunFailures_OriginUsesEachSuitesOwnDefaultBranch: defaults are picked
// per child project (the first branch it ever saw), so one run can be on the
// default branch for one suite and off it for another. Each suite is judged
// against its own default branch only, never against the branch the run is on.
func TestListRunFailures_OriginUsesEachSuitesOwnDefaultBranch(t *testing.T) {
	f := newFixture(t)
	const run = "run-two-suites"

	// api: default "main"; the run is on "main" too.
	api := f.newProject(f.id)
	f.suiteBuild(api.ID, 1, "main", "", originTest{"t", "passed"})
	f.suiteBuild(api.ID, 2, "main", run, originTest{"t", "failed"})

	// ui: its first branch was "develop", so that is its default; the run is
	// on "main" here, where an earlier build failed the test.
	ui := f.newProject(f.id)
	f.suiteBuild(ui.ID, 1, "develop", "", originTest{"t", "passed"})
	f.suiteBuild(ui.ID, 2, "main", "", originTest{"t", "failed"})
	f.suiteBuild(ui.ID, 3, "main", run, originTest{"t", "failed"})

	// e2e: default "main" has the test failing, so it is inherited there even
	// though a feature build in between passed it.
	e2e := f.newProject(f.id)
	f.suiteBuild(e2e.ID, 1, "main", "", originTest{"t", "failed"})
	f.suiteBuild(e2e.ID, 2, "feature", "", originTest{"t", "passed"})
	f.suiteBuild(e2e.ID, 3, "feature", run, originTest{"t", "failed"})

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{api.Slug, "2", "t"}: store.FailureOriginNew,
		{ui.Slug, "3", "t"}:  store.FailureOriginNew,
		{e2e.Slug, "3", "t"}: store.FailureOriginInherited,
	})
}

// TestListRunFailures_OriginOfAShardedSuiteIsPerShardBuild: shards of one suite
// upload several builds under one run. Each failing row is judged against the
// latest older default-branch build relative to its OWN build.
func TestListRunFailures_OriginOfAShardedSuiteIsPerShardBuild(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-sharded"
	f.suiteBuild(web.ID, 3, "main", "", originTest{"t", "passed"})
	f.suiteBuild(web.ID, 5, "main", "", originTest{"t", "failed"})
	f.suiteBuild(web.ID, 4, "feature", run, originTest{"t", "failed"}) // shard 1: newest older main build is 3
	f.suiteBuild(web.ID, 6, "feature", run, originTest{"t", "failed"}) // shard 2: newest older main build is 5

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{web.Slug, "4", "t"}: store.FailureOriginNew,
		{web.Slug, "6", "t"}: store.FailureOriginInherited,
	})
}

// TestListRunFailures_OriginSpansTheWholeBaselinePipeline: a sharded suite's
// baseline is the whole older default-branch pipeline, not just the one shard
// build that happens to be newest. A test that ran in another shard of the
// baseline pipeline is new or inherited, not unknown. Where shards disagree
// about one test (a retried shard), a failure anywhere beats a pass, a pass
// beats a skip, and only skipped-everywhere is unknown. Builds that merely
// share the pipeline id on another branch, and other pipelines, do not count.
func TestListRunFailures_OriginSpansTheWholeBaselinePipeline(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-sharded-baseline"

	// Shard B (order 2) is the newest older main build; it is uploaded BEFORE
	// shard A (order 1) so a lookup by database id would not find A first.
	f.suiteBuild(web.ID, 2, "main", "base-pipeline",
		originTest{"in-shard-b-passed", "passed"},
		originTest{"in-shard-b-failed", "failed"},
		originTest{"fail-and-pass", "failed"},
		originTest{"pass-and-skip", "skipped"},
		originTest{"skipped-both", "skipped"},
		originTest{"retried", "passed"},
	)
	f.suiteBuild(web.ID, 1, "main", "base-pipeline",
		originTest{"in-shard-a-passed", "passed"},
		originTest{"in-shard-a-failed", "broken"},
		originTest{"fail-and-pass", "passed"},
		originTest{"pass-and-skip", "passed"},
		originTest{"skipped-both", "skipped"},
		originTest{"retried", "failed"},
	)
	// Same pipeline id, other branch: not the default branch, so ignored.
	f.suiteBuild(web.ID, 3, "feature", "base-pipeline", originTest{"only-on-feature-shard", "passed"})
	// An unrelated feature pipeline in between: never a baseline.
	f.suiteBuild(web.ID, 4, "feature", "feature-pipeline", originTest{"only-in-feature-pipeline", "passed"})

	f.suiteBuild(web.ID, 5, "feature", run,
		originTest{"in-shard-b-passed", "failed"},
		originTest{"in-shard-b-failed", "failed"},
		originTest{"in-shard-a-passed", "failed"},
		originTest{"in-shard-a-failed", "failed"},
		originTest{"fail-and-pass", "failed"},
		originTest{"pass-and-skip", "failed"},
		originTest{"skipped-both", "failed"},
		originTest{"retried", "failed"},
		originTest{"only-on-feature-shard", "failed"},
		originTest{"only-in-feature-pipeline", "failed"},
		originTest{"nowhere", "failed"},
	)

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	slug := web.Slug
	wantTags(t, rows, map[[3]string]string{
		{slug, "5", "in-shard-b-passed"}:        store.FailureOriginNew,
		{slug, "5", "in-shard-b-failed"}:        store.FailureOriginInherited,
		{slug, "5", "in-shard-a-passed"}:        store.FailureOriginNew,       // only in the older shard
		{slug, "5", "in-shard-a-failed"}:        store.FailureOriginInherited, // broken in the older shard
		{slug, "5", "fail-and-pass"}:            store.FailureOriginInherited, // failed in B, passed in A
		{slug, "5", "pass-and-skip"}:            store.FailureOriginNew,       // skipped in B, passed in A
		{slug, "5", "skipped-both"}:             store.FailureOriginUnknown,
		{slug, "5", "retried"}:                  store.FailureOriginInherited, // failed in A, passed in B
		{slug, "5", "only-on-feature-shard"}:    store.FailureOriginUnknown,
		{slug, "5", "only-in-feature-pipeline"}: store.FailureOriginUnknown,
		{slug, "5", "nowhere"}:                  store.FailureOriginUnknown,
	})
}

// TestListRunFailures_OriginOfASingleBuildBaselineStaysOneBuild: a baseline
// build with no pipeline id is judged alone, as before; so is a baseline that
// has a pipeline id but only that one build. An older build never widens it.
func TestListRunFailures_OriginOfASingleBuildBaselineStaysOneBuild(t *testing.T) {
	f := newFixture(t)
	const run = "run-single-baseline"

	plain := f.newProject(f.id)
	f.suiteBuild(plain.ID, 1, "main", "", originTest{"t", "passed"}, originTest{"u", "passed"})
	f.suiteBuild(plain.ID, 2, "main", "", originTest{"u", "failed"})
	f.suiteBuild(plain.ID, 3, "feature", run, originTest{"t", "failed"}, originTest{"u", "failed"})

	alone := f.newProject(f.id)
	f.suiteBuild(alone.ID, 1, "main", "older-pipeline", originTest{"t", "passed"})
	f.suiteBuild(alone.ID, 2, "main", "only-shard", originTest{"u", "passed"})
	f.suiteBuild(alone.ID, 3, "feature", run, originTest{"t", "failed"}, originTest{"u", "failed"})

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{plain.Slug, "3", "t"}: store.FailureOriginUnknown, // absent from the newest older build
		{plain.Slug, "3", "u"}: store.FailureOriginInherited,
		{alone.Slug, "3", "t"}: store.FailureOriginUnknown,
		{alone.Slug, "3", "u"}: store.FailureOriginNew,
	})
}

// TestListRunFailures_OriginNeverUsesTheRunsOwnPipelineAsBaseline: when a
// sharded run is itself on the default branch, the previous default-branch
// build is a sibling shard of the same pipeline. Judging a shard against its
// own pipeline would compare the run with itself, so the baseline is the
// latest older build of a DIFFERENT pipeline; every shard of the run gets it.
func TestListRunFailures_OriginNeverUsesTheRunsOwnPipelineAsBaseline(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-on-main-sharded"
	f.suiteBuild(web.ID, 1, "main", "previous", originTest{"t", "passed"})
	f.suiteBuild(web.ID, 2, "main", "previous", originTest{"u", "passed"})
	f.suiteBuild(web.ID, 4, "main", run, originTest{"t", "failed"})
	f.suiteBuild(web.ID, 6, "main", run, originTest{"t", "failed"}, originTest{"u", "failed"})

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{web.Slug, "4", "t"}: store.FailureOriginNew,
		{web.Slug, "6", "t"}: store.FailureOriginNew,
		{web.Slug, "6", "u"}: store.FailureOriginNew,
	})
}

// TestListRunFailures_OriginOfARetriedJobOnTheDefaultBranch: a CI job that is
// retried uploads a second build under the same run. Both builds of the run are
// judged against the last build of the PREVIOUS run, so a regression is new in
// both; the first attempt is never the baseline of the retry. Covered for a run
// keyed by pipeline id and for one keyed by commit SHA (no pipeline id).
func TestListRunFailures_OriginOfARetriedJobOnTheDefaultBranch(t *testing.T) {
	f := newFixture(t)
	pg := pg.NewPipelineStore(f.s)

	byPipeline := f.newProject(f.id)
	f.suiteBuild(byPipeline.ID, 10, "main", "P9", originTest{"login", "passed"})
	f.suiteBuild(byPipeline.ID, 11, "main", "P10", originTest{"login", "failed"})
	f.suiteBuild(byPipeline.ID, 12, "main", "P10", originTest{"login", "failed"}) // the retry
	rows, err := pg.ListRunFailures(f.ctx, f.id, "P10", 100)
	if err != nil {
		t.Fatalf("ListRunFailures(P10): %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{byPipeline.Slug, "11", "login"}: store.FailureOriginNew,
		{byPipeline.Slug, "12", "login"}: store.FailureOriginNew,
	})

	bySHA := f.newProject(f.id)
	f.suiteBuildSHA(bySHA.ID, 10, "main", "sha-prev", originTest{"login", "passed"})
	f.suiteBuildSHA(bySHA.ID, 11, "main", "sha-now", originTest{"login", "failed"})
	f.suiteBuildSHA(bySHA.ID, 12, "main", "sha-now", originTest{"login", "failed"}) // the retry
	// Same commit, but uploaded under a pipeline id: another run, not in this one.
	f.suiteBuild(bySHA.ID, 13, "main", "P-other", originTest{"login", "failed"})
	rows, err = pg.ListRunFailures(f.ctx, f.id, "sha-now", 100)
	if err != nil {
		t.Fatalf("ListRunFailures(sha-now): %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{bySHA.Slug, "11", "login"}: store.FailureOriginNew,
		{bySHA.Slug, "12", "login"}: store.FailureOriginNew,
	})
}

// TestListRunFailures_OriginOfShardsOnTheDefaultBranch: shards of one run on
// the default branch are never each other's baseline, whether the run is keyed
// by pipeline id or by commit SHA. Every shard is judged against the whole
// previous run, so a test that only ran in the previous run's other shard is
// found, and one failing in every shard of both runs stays inherited.
func TestListRunFailures_OriginOfShardsOnTheDefaultBranch(t *testing.T) {
	f := newFixture(t)
	ps := pg.NewPipelineStore(f.s)

	byPipeline := f.newProject(f.id)
	f.suiteBuild(byPipeline.ID, 20, "main", "P1", originTest{"a", "passed"}, originTest{"setup", "failed"})
	f.suiteBuild(byPipeline.ID, 21, "main", "P1", originTest{"b", "passed"}, originTest{"setup", "failed"})
	f.suiteBuild(byPipeline.ID, 22, "main", "P2", originTest{"c", "failed"}, originTest{"b", "failed"}, originTest{"setup", "failed"})
	f.suiteBuild(byPipeline.ID, 23, "main", "P2", originTest{"a", "failed"}, originTest{"c", "failed"}, originTest{"setup", "failed"})
	rows, err := ps.ListRunFailures(f.ctx, f.id, "P2", 100)
	if err != nil {
		t.Fatalf("ListRunFailures(P2): %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{byPipeline.Slug, "22", "c"}:     store.FailureOriginUnknown, // new in this run, in neither shard of P1
		{byPipeline.Slug, "22", "b"}:     store.FailureOriginNew,
		{byPipeline.Slug, "22", "setup"}: store.FailureOriginInherited,
		{byPipeline.Slug, "23", "a"}:     store.FailureOriginNew, // passed in P1's first shard
		{byPipeline.Slug, "23", "c"}:     store.FailureOriginUnknown,
		{byPipeline.Slug, "23", "setup"}: store.FailureOriginInherited,
	})

	// Without a pipeline id each previous build stands alone, and the sibling
	// shard of the run itself is still not a baseline.
	bySHA := f.newProject(f.id)
	f.suiteBuildSHA(bySHA.ID, 20, "main", "sha-1", originTest{"c", "passed"})
	f.suiteBuildSHA(bySHA.ID, 21, "main", "sha-1b", originTest{"b", "passed"})
	f.suiteBuildSHA(bySHA.ID, 22, "main", "sha-2", originTest{"c", "failed"}, originTest{"b", "failed"})
	f.suiteBuildSHA(bySHA.ID, 23, "main", "sha-2", originTest{"c", "failed"}, originTest{"b", "failed"})
	rows, err = ps.ListRunFailures(f.ctx, f.id, "sha-2", 100)
	if err != nil {
		t.Fatalf("ListRunFailures(sha-2): %v", err)
	}
	wantTags(t, rows, map[[3]string]string{
		{bySHA.Slug, "22", "c"}: store.FailureOriginUnknown, // baseline is build 21, which has only b
		{bySHA.Slug, "22", "b"}: store.FailureOriginNew,
		{bySHA.Slug, "23", "c"}: store.FailureOriginUnknown,
		{bySHA.Slug, "23", "b"}: store.FailureOriginNew,
	})
}

// TestListRunFailures_Environment: a failure is environmental only when its
// stored defect fingerprint is categorised infrastructure; another category or
// no fingerprint at all is not.
func TestListRunFailures_Environment(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-env"
	id := f.suiteBuild(web.ID, 1, "main", run,
		originTest{"infra", "failed"},
		originTest{"bug", "failed"},
		originTest{"investigate", "broken"},
		originTest{"unfingerprinted", "failed"},
	)
	f.fingerprint(web.ID, id, "infra", store.DefectCategoryInfrastructure)
	f.fingerprint(web.ID, id, "bug", store.DefectCategoryProductBug)
	f.fingerprint(web.ID, id, "investigate", "to_investigate")

	rows, err := pg.NewPipelineStore(f.s).ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.HistoryID] = r.Environment
	}
	want := map[string]bool{"infra": true, "bug": false, "investigate": false, "unfingerprinted": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("environment by test = %v, want %v", got, want)
	}
}

// TestListRunFailures_KeepsTheLimit: tagging joins must not multiply or drop
// rows; the cap still applies, slowest first within a build.
func TestListRunFailures_KeepsTheLimit(t *testing.T) {
	f := newFixture(t)
	web := f.newProject(f.id)
	const run = "run-limit"
	f.suiteBuild(web.ID, 1, "main", "", originTest{"a", "passed"}, originTest{"b", "passed"}, originTest{"c", "passed"})
	f.suiteBuild(web.ID, 2, "main", run, originTest{"a", "failed"}, originTest{"b", "failed"}, originTest{"c", "failed"})

	ps := pg.NewPipelineStore(f.s)
	for _, limit := range []int{2, 3, 10} {
		rows, err := ps.ListRunFailures(f.ctx, f.id, run, limit)
		if err != nil {
			t.Fatalf("ListRunFailures(%d): %v", limit, err)
		}
		if want := min(limit, 3); len(rows) != want {
			t.Errorf("limit %d: %d rows, want %d", limit, len(rows), want)
		}
	}
}

// TestListFailuresByPipelineID_TagsLikeTheRun: the project-scoped query is the
// run query with a narrower scope, so the same failures carry the same tags
// (every field, test result id included) as the group run reports for that
// suite. It works for a standalone project too, which has no group.
func TestListFailuresByPipelineID_TagsLikeTheRun(t *testing.T) {
	f := newFixture(t)
	web, api := f.newProject(f.id), f.newProject(f.id)
	const run = "run-agree"
	f.suiteBuild(web.ID, 1, "main", "", originTest{"a", "passed"}, originTest{"b", "failed"})
	webBuild := f.suiteBuild(web.ID, 2, "feature", run, originTest{"a", "failed"}, originTest{"b", "failed"}, originTest{"c", "failed"})
	f.suiteBuild(web.ID, 3, "feature", run, originTest{"a", "failed"}) // a second shard
	f.fingerprint(web.ID, webBuild, "c", store.DefectCategoryInfrastructure)
	f.suiteBuild(api.ID, 1, "main", run, originTest{"x", "failed"}) // a sibling suite: not web's

	ps := pg.NewPipelineStore(f.s)
	group, err := ps.ListRunFailures(f.ctx, f.id, run, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	var fromGroup []store.RunFailureRow
	for _, r := range group {
		if r.ProjectID == web.ID {
			fromGroup = append(fromGroup, r)
		}
	}
	scoped, err := ps.ListFailuresByPipelineID(f.ctx, web.ID, run, 100)
	if err != nil {
		t.Fatalf("ListFailuresByPipelineID: %v", err)
	}
	if len(scoped) != 4 || !reflect.DeepEqual(scoped, fromGroup) {
		t.Errorf("project-scoped rows = %+v\ngroup rows for the suite = %+v\nwant 4 identical rows", scoped, fromGroup)
	}
	byTest := tags(t, scoped)
	if got := byTest[[3]string{web.Slug, "2", "a"}].Origin; got != store.FailureOriginNew {
		t.Errorf("a in shard 2: origin = %q, want new", got)
	}
	if !byTest[[3]string{web.Slug, "2", "c"}].Environment {
		t.Errorf("c in shard 2: environment = false, want true")
	}

	// A standalone project: the same tags with no group around it.
	solo := newFixture(t)
	solo.suiteBuild(solo.id, 1, "main", "", originTest{"a", "passed"})
	solo.suiteBuild(solo.id, 2, "main", run, originTest{"a", "failed"})
	rows, err := pg.NewPipelineStore(solo.s).ListFailuresByPipelineID(solo.ctx, solo.id, run, 100)
	if err != nil {
		t.Fatalf("ListFailuresByPipelineID(standalone): %v", err)
	}
	wantTags(t, rows, map[[3]string]string{{solo.project.Slug, "2", "a"}: store.FailureOriginNew})

	if rows, err := ps.ListFailuresByPipelineID(f.ctx, web.ID, "no-such-pipeline", 100); err != nil || len(rows) != 0 {
		t.Errorf("unknown pipeline = %+v, %v; want no rows", rows, err)
	}
}
