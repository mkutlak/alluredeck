package pg

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// SyncMetadata syncs projects via the storage store and imports any projects and builds
// not yet recorded in the PostgreSQL database.
// TODO(test): Add TestSyncMetadata_DoesNotDuplicateChildProjects once a lightweight
// DB-backed test harness is available for the pg package. The fix is covered at the
// InsertOrIgnore level by TestInsertOrIgnore_ChildSlugExists in project_test.go.
func SyncMetadata(ctx context.Context, st storage.Store, projectStore *ProjectStore, buildStore *BuildStore, logger *zap.Logger) error {
	projects, err := st.ListProjects(ctx)
	if err != nil {
		logger.Info("SyncMetadata: could not list projects (non-fatal)", zap.Error(err))
		return nil
	}

	for _, slug := range projects {
		if err := pgSyncProject(ctx, st, projectStore, buildStore, slug); err != nil {
			logger.Error("SyncMetadata: skipping project", zap.String("project_slug", slug), zap.Error(err))
		}
	}
	return nil
}

// pgSyncProject inserts or updates a single project and all its builds in the PostgreSQL database.
func pgSyncProject(ctx context.Context, st storage.Store, ps *ProjectStore, bs *BuildStore, slug string) error {
	if err := ps.InsertOrIgnore(ctx, slug); err != nil {
		return err
	}

	storageOrders, err := st.ListReportBuilds(ctx, slug)
	if err != nil {
		return fmt.Errorf("list report builds for project %q: %w", slug, err)
	}
	if len(storageOrders) == 0 {
		return nil
	}

	// Look up the project's numeric ID for build operations.
	// Use GetProjectBySlugAny so that child-only slugs (inserted by a previous
	// upload before InsertOrIgnore ran) are found even when no top-level row exists.
	proj, err := ps.GetProjectBySlugAny(ctx, slug)
	if err != nil {
		return fmt.Errorf("get project by slug %q: %w", slug, err)
	}

	existing, err := bs.ExistingBuildNumbers(ctx, proj.ID)
	if err != nil {
		return err
	}

	var missing []int
	for _, bo := range storageOrders {
		if _, ok := existing[bo]; !ok {
			missing = append(missing, bo)
		}
	}

	if err := bs.InsertMissingBuilds(ctx, proj.ID, missing); err != nil {
		return err
	}

	needStats, err := bs.BuildsWithMissingStats(ctx, proj.ID)
	if err != nil {
		return err
	}
	return bs.BatchSyncStats(ctx, proj.ID, slug, needStats, st)
}
