//go:build integration

package pg_test

import (
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestPGSearchStore: SearchTests full-text matches test names in latest builds
// only — a test seen solely in a superseded build stays hidden — and answers a
// miss with an empty, non-nil slice. Postgres FTS matches whole lexemes and
// does not split camelCase, so queries use whole tokens. SearchProjects
// matches a slug substring.
func TestPGSearchStore(t *testing.T) {
	f := newFixture(t)
	old, latest := f.build(1), f.build(2)
	if err := f.builds.SetLatest(f.ctx, f.id, 2); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}
	for _, r := range []struct {
		buildID  int64
		testName string
	}{{old, "OldBuildOnlyTest"}, {latest, "LoginFlow_ValidCredentials_ShouldSucceed"}, {latest, "LatestBuildTest"}} {
		tr := f.result(r.buildID, "com.example."+r.testName, "passed", r.testName)
		tr.TestName = r.testName
		f.insert(tr)
	}
	ss := pg.NewSearchStore(f.s, zap.NewNop())

	for _, tt := range []struct {
		query string
		want  string // test name expected from this project; "" = none
	}{
		{"LoginFlow", "LoginFlow_ValidCredentials_ShouldSucceed"},
		{"LatestBuildTest", "LatestBuildTest"},
		{"OldBuildOnlyTest", ""},
		{"zzznomatchxxx99999", ""},
	} {
		results, err := ss.SearchTests(f.ctx, tt.query, 10)
		if err != nil || results == nil {
			t.Fatalf("SearchTests(%q) = %v, %v; want a non-nil slice", tt.query, results, err)
		}
		var got []string
		for _, r := range results {
			if r.ProjectID == f.id {
				got = append(got, r.TestName)
			}
		}
		if strings.Join(got, ",") != tt.want {
			t.Errorf("SearchTests(%q) in this project = %q, want %q", tt.query, got, tt.want)
		}
	}

	suffix := strings.TrimPrefix(f.project.Slug, "pgtest-")
	projects, err := ss.SearchProjects(f.ctx, suffix, 10)
	if err != nil || len(projects) != 1 || projects[0].Slug != f.project.Slug {
		t.Errorf("SearchProjects(%q) = %+v, %v; want only %q", suffix, projects, err, f.project.Slug)
	}
}
