package pg

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// SyncMetadata syncs projects via the storage store and imports any projects and builds
// not yet recorded in the PostgreSQL database.
func SyncMetadata(ctx context.Context, st storage.Store, projectStore *PGProjectStore, buildStore *PGBuildStore, logger *zap.Logger) error {
	projects, err := st.ListProjects(ctx)
	if err != nil {
		logger.Info("SyncMetadata: could not list projects (non-fatal)", zap.Error(err))
		return nil
	}

	for _, projectID := range projects {
		if err := pgSyncProject(ctx, st, projectStore, buildStore, projectID); err != nil {
			logger.Error("SyncMetadata: skipping project", zap.String("project_id", projectID), zap.Error(err))
		}
	}
	return nil
}

// pgSyncProject inserts or updates a single project and all its builds in the PostgreSQL database.
func pgSyncProject(ctx context.Context, st storage.Store, ps *PGProjectStore, bs *PGBuildStore, projectID string) error {
	if err := ps.InsertOrIgnore(ctx, projectID); err != nil {
		return err
	}

	storageOrders, err := st.ListReportBuilds(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list report builds for project %q: %w", projectID, err)
	}
	if len(storageOrders) == 0 {
		return nil
	}

	existing, err := bs.ExistingBuildOrders(ctx, projectID)
	if err != nil {
		return err
	}

	var missing []int
	for _, bo := range storageOrders {
		if _, ok := existing[bo]; !ok {
			missing = append(missing, bo)
		}
	}

	if err := bs.InsertMissingBuilds(ctx, projectID, missing); err != nil {
		return err
	}

	needStats, err := bs.BuildsWithMissingStats(ctx, projectID)
	if err != nil {
		return err
	}
	return bs.BatchSyncStats(ctx, projectID, needStats, st)
}
