package pg_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// openTestStore opens a PGStore on TEST_POSTGRES_URL (migrations applied) and
// skips the test when the variable is unset. cfg carries optional pool GUCs.
func openTestStore(t *testing.T, cfg ...config.Config) *pg.PGStore {
	t.Helper()
	c := config.Config{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.DatabaseURL = os.Getenv("TEST_POSTGRES_URL"); c.DatabaseURL == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping PostgreSQL integration test")
	}
	c.RunMigrations = true
	s, err := pg.Open(context.Background(), &c)
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var uniqueSeq atomic.Int64

// unique returns a per-call unique string for slugs, emails, hashes, and keys,
// so re-runs against a persistent database never collide.
func unique(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), uniqueSeq.Add(1))
}

// fixture is one fresh project (removed on cleanup) plus the stores most
// tests drive. Its helpers fail the test that created it, so a table test
// builds one fixture per subtest.
type fixture struct {
	t        *testing.T
	ctx      context.Context
	s        *pg.PGStore
	projects *pg.ProjectStore
	builds   *pg.BuildStore
	branches *pg.BranchStore
	results  *pg.TestResultStore
	project  *store.Project
	id       int64 // project.ID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s := openTestStore(t)
	f := &fixture{
		t: t, ctx: context.Background(), s: s,
		projects: pg.NewProjectStore(s, zap.NewNop()),
		builds:   pg.NewBuildStore(s, zap.NewNop()),
		branches: pg.NewBranchStore(s),
		results:  pg.NewTestResultStore(s, zap.NewNop()),
	}
	f.project = f.newProject(0)
	f.id = f.project.ID
	return f
}

// newProject creates another project, a child of parentID when it is non-zero.
func (f *fixture) newProject(parentID int64) *store.Project {
	f.t.Helper()
	var (
		p   *store.Project
		err error
	)
	if parentID == 0 {
		p, err = f.projects.CreateProject(f.ctx, unique("pgtest"))
	} else {
		p, err = f.projects.CreateProjectWithParent(f.ctx, unique("pgtest-child"), parentID)
	}
	if err != nil {
		f.t.Fatalf("create project: %v", err)
	}
	f.t.Cleanup(func() { _ = f.projects.DeleteProject(context.Background(), p.ID) })
	return p
}

// build inserts build_order order into the fixture project and returns its id.
func (f *fixture) build(order int) int64 { return f.buildIn(f.id, order) }

func (f *fixture) buildIn(projectID int64, order int) int64 {
	f.t.Helper()
	if err := f.builds.InsertBuild(f.ctx, projectID, order); err != nil {
		f.t.Fatalf("InsertBuild %d: %v", order, err)
	}
	id, err := f.results.GetBuildID(f.ctx, projectID, order)
	if err != nil {
		f.t.Fatalf("GetBuildID %d: %v", order, err)
	}
	return id
}

func (f *fixture) branch(name string) *store.Branch {
	f.t.Helper()
	b, _, err := f.branches.GetOrCreate(f.ctx, f.id, name)
	if err != nil {
		f.t.Fatalf("GetOrCreate branch %q: %v", name, err)
	}
	return b
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.s.Pool().Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.s.Pool().QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

// result is a stability row for the fixture project; callers adjust the rest.
func (f *fixture) result(buildID int64, fullName, status, historyID string) store.TestResult {
	return store.TestResult{
		BuildID: buildID, ProjectID: f.id, TestName: fullName, FullName: fullName,
		Status: status, HistoryID: historyID, DurationMs: 100,
	}
}

func (f *fixture) insert(rs ...store.TestResult) {
	f.t.Helper()
	if err := f.results.InsertBatch(f.ctx, rs); err != nil {
		f.t.Fatalf("InsertBatch: %v", err)
	}
}

func (f *fixture) insertFull(buildID int64, rs ...*parser.Result) {
	f.t.Helper()
	if err := f.results.InsertBatchFull(f.ctx, buildID, f.id, rs); err != nil {
		f.t.Fatalf("InsertBatchFull: %v", err)
	}
}

// newUser creates a local user; the proposal tables reference users by FK.
func (f *fixture) newUser() int64 {
	f.t.Helper()
	u, err := pg.NewUserStore(f.s).CreateLocal(f.ctx, unique("pgtest")+"@example.com", "Tester", "hash", "editor")
	if err != nil {
		f.t.Fatalf("CreateLocal: %v", err)
	}
	return u.ID
}
