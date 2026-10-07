package pg_test

import (
	"slices"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// expectedFixture is one group with the children the tests upload into. Each
// child's default branch is "main": it is created first, as the first branch a
// project ever uploads is its default.
type expectedFixture struct {
	*fixture
	ps         *pg.PipelineStore
	t0         time.Time
	a, b, c, d int64 // child project ids, ascending
}

func newExpectedFixture(t *testing.T) *expectedFixture {
	t.Helper()
	f := newFixture(t) // the group
	e := &expectedFixture{
		fixture: f,
		ps:      pg.NewPipelineStore(f.s),
		t0:      time.Now().Add(-6 * time.Hour).Truncate(time.Second),
	}
	ids := []*int64{&e.a, &e.b, &e.c, &e.d}
	for _, id := range ids {
		child := f.newProject(f.id)
		*id = child.ID
		if _, _, err := f.branches.GetOrCreate(f.ctx, child.ID, "main"); err != nil {
			t.Fatalf("create default branch: %v", err)
		}
	}
	return e
}

// upload adds one build to project as part of the run named by pipelineID (or
// by sha when pipelineID is empty), made minutes after t0. branch is both the
// build's ci_branch and its branch row; "" leaves both NULL, as on a build
// uploaded before branches existed.
func (e *expectedFixture) upload(project int64, order int, branch, pipelineID, sha string, minutes int) {
	e.t.Helper()
	id := e.buildIn(project, order)
	if branch != "" {
		br, _, err := e.branches.GetOrCreate(e.ctx, project, branch)
		if err != nil {
			e.t.Fatalf("GetOrCreate branch %q: %v", branch, err)
		}
		if err := e.builds.UpdateBuildBranchID(e.ctx, project, order, br.ID); err != nil {
			e.t.Fatalf("UpdateBuildBranchID: %v", err)
		}
	}
	ci := store.CIMetadata{Branch: branch, PipelineID: pipelineID, CommitSHA: sha}
	if err := e.builds.UpdateBuildCIMetadata(e.ctx, project, order, ci); err != nil {
		e.t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}
	e.exec("UPDATE builds SET created_at=$1 WHERE id=$2", e.t0.Add(time.Duration(minutes)*time.Minute), id)
}

// expect asserts the suites ExpectedSuites reports for runKey in the group.
func (e *expectedFixture) expect(runKey string, want ...int64) {
	e.t.Helper()
	got, err := e.ps.ExpectedSuites(e.ctx, e.id, runKey)
	if err != nil {
		e.t.Fatalf("ExpectedSuites(%q): %v", runKey, err)
	}
	if got == nil {
		e.t.Errorf("ExpectedSuites(%q) = nil, want a non-nil slice", runKey)
	}
	if !slices.Equal(got, want) {
		e.t.Errorf("ExpectedSuites(%q) = %v, want %v", runKey, got, want)
	}
}

// TestExpectedSuites_SameBranchWinsOverNewerDefaultBranchRun: the previous run
// of the branch is chosen even though a newer run on the default branch, with
// more suites, sits between it and the run.
func TestExpectedSuites_SameBranchWinsOverNewerDefaultBranchRun(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.a, 1, "feature", "feat-1", "s1", 0)
	e.upload(e.b, 1, "feature", "feat-1", "s1", 1)
	e.upload(e.a, 2, "main", "main-1", "s2", 10)
	e.upload(e.b, 2, "main", "main-1", "s2", 11)
	e.upload(e.c, 2, "main", "main-1", "s2", 12)
	e.upload(e.a, 3, "feature", "feat-2", "s3", 20)

	e.expect("feat-2", e.a, e.b)
}

// TestExpectedSuites_FallsBackToTheLatestDefaultBranchRun: a branch that has no
// earlier run takes the latest earlier run on a default branch, not an older
// default-branch run and not a newer run on another branch.
func TestExpectedSuites_FallsBackToTheLatestDefaultBranchRun(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.a, 1, "main", "main-old", "s1", 0)
	e.upload(e.a, 2, "main", "main-new", "s2", 10)
	e.upload(e.b, 2, "main", "main-new", "s2", 11)
	e.upload(e.c, 2, "main", "main-new", "s2", 12)
	e.upload(e.a, 3, "hotfix", "hotfix-1", "s3", 15) // newer, but neither the branch nor a default branch
	e.upload(e.d, 1, "hotfix", "hotfix-1", "s3", 16)
	e.upload(e.a, 4, "feature", "feat-1", "s4", 20)

	e.expect("feat-1", e.a, e.b, e.c)
}

// TestExpectedSuites_NoPreviousRunIsEmpty covers a first run, a run that is
// only followed by later ones, and a key nothing was uploaded under.
func TestExpectedSuites_NoPreviousRunIsEmpty(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.a, 1, "main", "first", "s1", 0)
	e.upload(e.a, 2, "main", "second", "s2", 10)

	e.expect("first")
	e.expect("no-such-run")
}

// TestExpectedSuites_IsThePreviousRunsWholeDistinctSuiteSet: a sharded suite
// counts once, and a build of the previous run uploaded after the run started
// still counts: the whole previous run is the answer. Builds are inserted
// out of project order, and the answer is ordered by project id.
func TestExpectedSuites_IsThePreviousRunsWholeDistinctSuiteSet(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.c, 1, "main", "prev", "s1", 2)
	e.upload(e.a, 1, "main", "prev", "s1", 0) // two shards of one suite
	e.upload(e.a, 2, "main", "prev", "s1", 1)
	e.upload(e.b, 1, "main", "prev", "s1", 40) // uploaded after the run below started
	e.upload(e.a, 3, "main", "cur", "s2", 10)

	e.expect("cur", e.a, e.b, e.c)
}

// TestExpectedSuites_NeverChoosesTheRunItselfOrLaterRuns: the run's own builds
// and the runs after it never serve as its expectation, so the earliest run
// has none however much came after it, and the run in the middle gets the
// earlier one.
func TestExpectedSuites_NeverChoosesTheRunItselfOrLaterRuns(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.c, 1, "main", "early", "s1", 0)
	e.upload(e.a, 1, "main", "mid", "s2", 10)
	e.upload(e.b, 1, "main", "mid", "s2", 11)
	e.upload(e.a, 2, "main", "late", "s3", 20)
	e.upload(e.b, 2, "main", "late", "s3", 21)
	e.upload(e.d, 1, "main", "late", "s3", 22)

	e.expect("early")
	e.expect("mid", e.c)
	e.expect("late", e.a, e.b)
}

// TestExpectedSuites_IgnoresOtherGroups: a newer run on the same branch in
// another group is no expectation, and a group with no earlier run of its own
// expects nothing.
func TestExpectedSuites_IgnoresOtherGroups(t *testing.T) {
	mine, lone, other := newExpectedFixture(t), newExpectedFixture(t), newExpectedFixture(t)
	mine.upload(mine.a, 1, "main", "mine-old", "s1", 0)
	mine.upload(mine.a, 2, "main", "mine-new", "s2", 20)
	lone.upload(lone.a, 1, "main", "lone", "s3", 20)
	other.upload(other.a, 1, "main", "theirs", "s9", 10)
	other.upload(other.b, 1, "main", "theirs", "s9", 11)

	mine.expect("mine-new", mine.a)
	lone.expect("lone")
}

// TestExpectedSuites_KeyIsPipelineIDOrCommitSHA: the run is found by its
// pipeline id, or by its commit SHA when its builds have no pipeline id, and
// so is the previous run it is compared with. A build with a pipeline id
// that shares a SHA with a SHA-keyed run is a different run.
func TestExpectedSuites_KeyIsPipelineIDOrCommitSHA(t *testing.T) {
	t.Run("sha run after sha run", func(t *testing.T) {
		e := newExpectedFixture(t)
		e.upload(e.a, 1, "main", "", "aaa111", 0)
		e.upload(e.b, 1, "main", "", "aaa111", 1)
		e.upload(e.a, 2, "main", "", "bbb222", 10)

		e.expect("bbb222", e.a, e.b)
		e.expect("aaa111")
	})
	t.Run("pipeline run after sha run of the same commit", func(t *testing.T) {
		e := newExpectedFixture(t)
		e.upload(e.a, 1, "main", "", "c0ffee", 0)
		e.upload(e.c, 1, "main", "", "c0ffee", 1)
		e.upload(e.a, 2, "main", "p-9", "c0ffee", 10)

		e.expect("p-9", e.a, e.c)
		e.expect("c0ffee")
	})
	t.Run("sha run after pipeline run", func(t *testing.T) {
		e := newExpectedFixture(t)
		e.upload(e.b, 1, "main", "p-1", "dddddd", 0)
		e.upload(e.d, 1, "main", "p-1", "dddddd", 1)
		e.upload(e.a, 1, "main", "", "eeeeee", 10)

		e.expect("eeeeee", e.b, e.d)
	})
}

// TestExpectedSuites_RunWithoutABranchFallsBackToTheDefaultBranch: a run whose
// builds carry no ci_branch has no branch to match, so earlier runs that carry
// no branch either are not mistaken for its branch, and the latest default-
// branch run is chosen over the newer run on a feature branch.
func TestExpectedSuites_RunWithoutABranchFallsBackToTheDefaultBranch(t *testing.T) {
	e := newExpectedFixture(t)
	e.upload(e.a, 1, "main", "main-1", "s1", 0)
	e.upload(e.b, 1, "main", "main-1", "s1", 1)
	e.upload(e.a, 2, "feature", "feat-1", "s2", 10)
	e.upload(e.c, 1, "", "nobranch-1", "s3", 15) // no ci_branch, no branch row
	e.upload(e.a, 3, "", "nobranch-2", "s4", 20)

	e.expect("nobranch-2", e.a, e.b)
}

// TestExpectedSuites_BuildsWithoutAKeyBelongToNoRun: a build with neither a
// pipeline id nor a commit SHA (an upload that carried only a branch) is part
// of no run, so a newer one never stands in for the real previous run, whether
// that is found on the run's branch or on the default branch.
func TestExpectedSuites_BuildsWithoutAKeyBelongToNoRun(t *testing.T) {
	t.Run("on the branch", func(t *testing.T) {
		e := newExpectedFixture(t)
		e.upload(e.a, 1, "feature", "feat-1", "s1", 0)
		e.upload(e.b, 1, "feature", "feat-1", "s1", 1)
		e.upload(e.c, 1, "feature", "", "", 5)
		e.upload(e.a, 2, "feature", "feat-2", "s2", 10)

		e.expect("feat-2", e.a, e.b)
	})
	t.Run("on the default branch", func(t *testing.T) {
		e := newExpectedFixture(t)
		e.upload(e.a, 1, "main", "main-1", "s1", 0)
		e.upload(e.b, 1, "main", "main-1", "s1", 1)
		e.upload(e.c, 1, "main", "", "", 5)
		e.upload(e.a, 2, "feature", "feat-1", "s2", 10)

		e.expect("feat-1", e.a, e.b)
	})
}

// TestExpectedSuites_GroupsSharingARunKeyStaySeparate: one CI pipeline (or a
// monorepo commit) can upload to two groups, so the same key names a run in
// each. Every group is answered from its own builds: its previous run, found
// with its own start, lists only its own suites.
func TestExpectedSuites_GroupsSharingARunKeyStaySeparate(t *testing.T) {
	g1, g2 := newExpectedFixture(t), newExpectedFixture(t)
	// G1: "shared" is the previous run, "cur" the run being asked about.
	g1.upload(g1.a, 1, "feature", "shared", "s1", 0)
	g1.upload(g1.b, 1, "feature", "shared", "s1", 1)
	g1.upload(g1.a, 2, "feature", "cur", "s2", 10)
	// G2 uses the same two keys, earlier than G1 does, so a lookup that mixed
	// the groups would move G1's start before its previous run, or add G2's
	// suites to G1's answer.
	g2.upload(g2.c, 1, "other", "shared", "s1", -30)
	g2.upload(g2.d, 1, "other", "shared", "s1", -29)
	g2.upload(g2.b, 1, "other", "cur", "s2", -20)

	g1.expect("cur", g1.a, g1.b)
	g2.expect("cur", g2.c, g2.d)
}
