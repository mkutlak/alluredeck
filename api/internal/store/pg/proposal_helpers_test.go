package pg_test

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// openProposalTestStore opens a PGStore using TEST_POSTGRES_URL; skips if unset.
func openProposalTestStore(t *testing.T) *pg.PGStore {
	t.Helper()
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping proposal store integration test")
	}
	s, err := pg.Open(context.Background(), &config.Config{DatabaseURL: url, RunMigrations: true})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedProposalFixtures creates a project and a local user to satisfy the
// proposal tables' FK constraints (project_id -> projects, proposer_user_id
// -> users), returning their generated IDs. slugSuffix must be unique per
// call so concurrent test runs (and re-runs against a persistent test DB)
// don't collide on the projects.slug / users.email unique constraints.
func seedProposalFixtures(t *testing.T, s *pg.PGStore, slugSuffix string) (projectID int, userID int64) {
	t.Helper()
	ctx := context.Background()

	projects := pg.NewProjectStore(s, zap.NewNop())
	proj, err := projects.CreateProject(ctx, "proposal-test-"+slugSuffix)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	users := pg.NewUserStore(s)
	email := "proposal-test-" + slugSuffix + "@example.com"
	u, err := users.CreateLocal(ctx, email, "Proposal Tester", "hash", "editor")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	return int(proj.ID), u.ID
}

// uniqueSuffix returns a per-call unique string suitable for slugs/emails.
func uniqueSuffix() string {
	return time.Now().Format("150405.000000000")
}
