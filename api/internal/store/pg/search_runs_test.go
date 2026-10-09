package pg_test

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

func runCI(pipelineID, branch string) store.CIMetadata {
	return store.CIMetadata{PipelineID: pipelineID, Branch: branch, CommitSHA: "abc1234"}
}

func branchNames(ms []store.BranchMatch) string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return strings.Join(names, ",")
}

// TestSearchPipelineRuns_ExactMatchOneRowPerRun: a pipeline id finds its run
// once however many suites and shards uploaded into it, carrying the group
// project, the branch and the newest build's time. A different id, a partial
// id and a longer id match nothing, and a top-level project's build (no group
// above it) is not a feed run.
func TestSearchPipelineRuns_ExactMatchOneRowPerRun(t *testing.T) {
	f := newFixture(t) // the parent group
	a, b := f.newProject(f.id), f.newProject(f.id)
	pid := unique("48213")
	f.runBuild(a.ID, 1, runCI(pid, "login-retry"), 8, 2, 0)
	f.runBuild(a.ID, 2, runCI(pid, "login-retry"), 10, 0, 1) // a second shard of suite a
	f.runBuild(b.ID, 1, runCI(pid, "login-retry"), 5, 0, 0)
	f.runBuild(b.ID, 2, runCI(unique("other"), "main"), 5, 0, 0)
	ss := pg.NewSearchStore(f.s, zap.NewNop())

	before := time.Now().Add(-time.Minute)
	got, err := ss.SearchPipelineRuns(f.ctx, pid, 10)
	if err != nil {
		t.Fatalf("SearchPipelineRuns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 for the whole run: %+v", len(got), got)
	}
	r := got[0]
	if r.GroupProjectID != f.id || r.GroupSlug != f.project.Slug || r.PipelineID != pid || r.Branch != "login-retry" {
		t.Errorf("run = %+v, want group %d/%s pipeline %s branch login-retry", r, f.id, f.project.Slug, pid)
	}
	if r.Timestamp.Before(before) || r.Timestamp.After(time.Now().Add(time.Minute)) {
		t.Errorf("timestamp = %v, want about now", r.Timestamp)
	}

	for _, q := range []string{pid[:len(pid)-1], pid[1:], pid + "9"} {
		if got, err := ss.SearchPipelineRuns(f.ctx, q, 10); err != nil || len(got) != 0 {
			t.Errorf("SearchPipelineRuns(%q) = %+v, %v; want no match (exact ids only)", q, got, err)
		}
	}

	topLevel := unique("77001")
	f.runBuild(f.id, 9, runCI(topLevel, "main"), 1, 0, 0)
	if got, err := ss.SearchPipelineRuns(f.ctx, topLevel, 10); err != nil || len(got) != 0 {
		t.Errorf("top-level project build matched: %+v, %v", got, err)
	}
}

// TestSearchPipelineRuns_SamePipelineIDInTwoGroups: a CI pipeline id is not
// unique across groups, so each group's run is its own row, and runs sort by
// their newest build. spans uploaded first and last with mid's upload between,
// so it sorts first only by its latest build, not its earliest. limit cuts the
// list. A miss answers with an empty non-nil slice.
func TestSearchPipelineRuns_SamePipelineIDInTwoGroups(t *testing.T) {
	spans, mid := newFixture(t), newFixture(t)
	spansChild, midChild := spans.newProject(spans.id), mid.newProject(mid.id)
	pid := unique("shared")
	spans.runBuild(spansChild.ID, 1, runCI(pid, "main"), 1, 0, 0)
	mid.runBuild(midChild.ID, 1, runCI(pid, "release"), 1, 0, 0)
	spans.runBuild(spansChild.ID, 2, runCI(pid, "main"), 1, 0, 0)
	ss := pg.NewSearchStore(spans.s, zap.NewNop())

	got, err := ss.SearchPipelineRuns(spans.ctx, pid, 10)
	if err != nil {
		t.Fatalf("SearchPipelineRuns: %v", err)
	}
	if len(got) != 2 || got[0].GroupProjectID != spans.id || got[1].GroupProjectID != mid.id {
		t.Fatalf("got %+v, want the run with the newest build first, then the other", got)
	}
	if got[0].Branch != "main" || got[1].Branch != "release" {
		t.Errorf("branches = %q, %q, want main, release", got[0].Branch, got[1].Branch)
	}
	if !got[0].Timestamp.After(got[1].Timestamp) {
		t.Errorf("timestamps = %v, %v, want each run stamped with its newest build", got[0].Timestamp, got[1].Timestamp)
	}

	if got, err := ss.SearchPipelineRuns(spans.ctx, pid, 1); err != nil || len(got) != 1 || got[0].GroupProjectID != spans.id {
		t.Errorf("limit 1 = %+v, %v; want only the newest run", got, err)
	}
	if got, err := ss.SearchPipelineRuns(spans.ctx, unique("nope"), 10); err != nil || got == nil || len(got) != 0 {
		t.Errorf("miss = %#v, %v; want an empty non-nil slice", got, err)
	}
}

// TestSearchPipelineRuns_BranchIsTheNewestBuildsBranch: like the feed, a run
// carries the branch of its newest build, even when an older build's branch
// sorts later, and an empty branch when the newest build has none.
func TestSearchPipelineRuns_BranchIsTheNewestBuildsBranch(t *testing.T) {
	f := newFixture(t)
	a, b := f.newProject(f.id), f.newProject(f.id)
	ss := pg.NewSearchStore(f.s, zap.NewNop())

	changed := unique("changed")
	f.runBuild(a.ID, 1, runCI(changed, "zzz-old"), 1, 0, 0)
	f.runBuild(b.ID, 1, runCI(changed, "aaa-newest"), 1, 0, 0)
	unset := unique("unset")
	f.runBuild(a.ID, 2, runCI(unset, "feature/x"), 1, 0, 0)
	f.runBuild(b.ID, 2, store.CIMetadata{PipelineID: unset, CommitSHA: "abc1234"}, 1, 0, 0)

	for pid, want := range map[string]string{changed: "aaa-newest", unset: ""} {
		got, err := ss.SearchPipelineRuns(f.ctx, pid, 10)
		if err != nil || len(got) != 1 || got[0].Branch != want {
			t.Errorf("SearchPipelineRuns(%q) = %+v, %v; want one run on branch %q", pid, got, err, want)
		}
	}
}

// TestSearchBranches_SubstringCaseInsensitive: a branch is found by a
// case-insensitive substring of builds.ci_branch, listed once however many
// builds ran on it, sorted by name, and limit cuts the list. A branch only on
// a top-level project's builds, or only on builds with no run key (neither a
// pipeline id nor a commit SHA, so the feed drops them), would open an empty
// feed and is not offered. A miss answers with an empty non-nil slice.
func TestSearchBranches_SubstringCaseInsensitive(t *testing.T) {
	f := newFixture(t)
	child := f.newProject(f.id)
	u := unique("zq")
	f.runBuild(child.ID, 1, runCI("", "Feature/Login-Retry-"+u), 1, 0, 0)
	f.runBuild(child.ID, 2, runCI("", "Feature/Login-Retry-"+u), 1, 0, 0)
	f.runBuild(child.ID, 3, runCI("", "Feature/Other-"+u), 1, 0, 0)
	f.runBuild(child.ID, 4, runCI("", "Feature/Third-"+u), 1, 0, 0)
	f.runBuild(child.ID, 5, store.CIMetadata{Branch: "Feature/NoRunKey-" + u}, 1, 0, 0)
	f.runBuild(f.id, 6, runCI("", "Feature/TopLevelOnly-"+u), 1, 0, 0)
	ss := pg.NewSearchStore(f.s, zap.NewNop())

	for _, tt := range []struct {
		query string
		limit int
		want  string
	}{
		{"login-RETRY-" + u, 10, "Feature/Login-Retry-" + u},
		{"-" + u, 10, "Feature/Login-Retry-" + u + ",Feature/Other-" + u + ",Feature/Third-" + u},
		{"-" + u, 2, "Feature/Login-Retry-" + u + ",Feature/Other-" + u},
		{"-" + u, 1, "Feature/Login-Retry-" + u},
	} {
		got, err := ss.SearchBranches(f.ctx, tt.query, tt.limit)
		if err != nil || branchNames(got) != tt.want {
			t.Errorf("SearchBranches(%q, %d) = %q, %v; want %q", tt.query, tt.limit, branchNames(got), err, tt.want)
		}
	}
	if got, err := ss.SearchBranches(f.ctx, unique("nope"), 10); err != nil || got == nil || len(got) != 0 {
		t.Errorf("miss = %#v, %v; want an empty non-nil slice", got, err)
	}
}

// TestSearchBranches_WildcardsAreLiteral: % and _ in the query match
// themselves, not any text or any character.
func TestSearchBranches_WildcardsAreLiteral(t *testing.T) {
	f := newFixture(t)
	child := f.newProject(f.id)
	u := unique("wc")
	for i, name := range []string{"a_b-" + u, "axb-" + u, "100%-" + u, "100x-" + u} {
		f.runBuild(child.ID, i+1, runCI("", name), 1, 0, 0)
	}
	ss := pg.NewSearchStore(f.s, zap.NewNop())

	for q, want := range map[string]string{"a_b-" + u: "a_b-" + u, "100%-" + u: "100%-" + u} {
		if got, err := ss.SearchBranches(f.ctx, q, 10); err != nil || branchNames(got) != want {
			t.Errorf("SearchBranches(%q) = %q, %v; want only %q", q, branchNames(got), err, want)
		}
	}
}
