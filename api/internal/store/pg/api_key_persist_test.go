package pg_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// openAPIKeyTestStore opens a PGStore using TEST_POSTGRES_URL; skips if unset.
func openAPIKeyTestStore(t *testing.T) *pg.PGStore {
	t.Helper()
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set; skipping api_keys persistence integration test")
	}
	s, err := pg.Open(context.Background(), &config.Config{DatabaseURL: url, RunMigrations: true})
	if err != nil {
		t.Fatalf("pg.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestCreate_PersistsAllowMCPWrites is a regression test for a field that the
// INSERT omitted entirely.
//
// allow_mcp_writes was absent from the column list, so PostgreSQL applied the
// column default of false while Create returned the caller's struct unchanged
// — still carrying true. Both the REST response and the UI therefore reported
// having granted MCP write access that was never stored, and the propose_*
// tools rejected every such key. Reading the value back from the database is
// the only way to catch that: asserting on Create's return value cannot.
func TestCreate_PersistsAllowMCPWrites(t *testing.T) {
	s := openAPIKeyTestStore(t)
	keys := pg.NewAPIKeyStore(s)
	ctx := context.Background()

	for _, want := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[want], func(t *testing.T) {
			hash := "persist-test-" + time.Now().Format("150405.000000000")
			created, err := keys.Create(ctx, &store.APIKey{
				Name:           "persist-test",
				Prefix:         "ald_persist",
				KeyHash:        hash,
				Username:       "persist@example.com",
				Role:           "editor",
				AllowMCPWrites: want,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			t.Cleanup(func() { _ = keys.Delete(ctx, created.ID, "persist@example.com") })

			// Read back through the same path the auth layer uses.
			got, err := keys.GetByHash(ctx, hash)
			if err != nil {
				t.Fatalf("GetByHash: %v", err)
			}
			if got.AllowMCPWrites != want {
				t.Errorf("allow_mcp_writes round-tripped as %v, want %v", got.AllowMCPWrites, want)
			}
		})
	}
}

// TestCreate_PersistsProjectIDs guards the neighbouring column, which is in the
// INSERT but is easy to drop by the same mistake.
func TestCreate_PersistsProjectIDs(t *testing.T) {
	s := openAPIKeyTestStore(t)
	keys := pg.NewAPIKeyStore(s)
	ctx := context.Background()

	hash := "persist-projects-" + time.Now().Format("150405.000000000")
	created, err := keys.Create(ctx, &store.APIKey{
		Name:       "persist-projects",
		Prefix:     "ald_persist",
		KeyHash:    hash,
		Username:   "persist@example.com",
		Role:       "viewer",
		ProjectIDs: []int64{11, 22},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = keys.Delete(ctx, created.ID, "persist@example.com") })

	got, err := keys.GetByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if len(got.ProjectIDs) != 2 || got.ProjectIDs[0] != 11 || got.ProjectIDs[1] != 22 {
		t.Errorf("project_ids round-tripped as %v, want [11 22]", got.ProjectIDs)
	}
}
