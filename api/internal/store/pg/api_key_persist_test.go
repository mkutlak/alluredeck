package pg_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestAPIKeyStore_CreatePersists reads each column back through GetByHash,
// the path the auth layer uses. Regression for allow_mcp_writes, which the
// INSERT omitted: PostgreSQL stored the default false while Create returned
// the caller's true, so REST and UI reported MCP write access that was never
// granted — asserting on Create's return value cannot catch that.
func TestAPIKeyStore_CreatePersists(t *testing.T) {
	keys := pg.NewAPIKeyStore(openTestStore(t))
	ctx := context.Background()
	tests := []struct {
		name       string
		mcpWrites  bool
		projectIDs []int64
	}{
		{"allow_mcp_writes enabled", true, nil},
		{"allow_mcp_writes disabled", false, nil},
		{"scoped key keeps project_ids", false, []int64{11, 22}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := unique("persist")
			created, err := keys.Create(ctx, &store.APIKey{
				Name: "persist-test", Prefix: "ald_persist", KeyHash: hash, Username: "persist@example.com",
				Role: "editor", AllowMCPWrites: tt.mcpWrites, ProjectIDs: tt.projectIDs,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			t.Cleanup(func() { _ = keys.Delete(ctx, created.ID, "persist@example.com") })
			got, err := keys.GetByHash(ctx, hash)
			if err != nil {
				t.Fatalf("GetByHash: %v", err)
			}
			if got.AllowMCPWrites != tt.mcpWrites || !slices.Equal(got.ProjectIDs, tt.projectIDs) {
				t.Errorf("round-trip allow_mcp_writes=%v project_ids=%v, want %v %v",
					got.AllowMCPWrites, got.ProjectIDs, tt.mcpWrites, tt.projectIDs)
			}
		})
	}
}
