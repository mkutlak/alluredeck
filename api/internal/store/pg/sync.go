package pg

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// SyncMetadata syncs projects via the storage store and imports any projects and builds
// not yet recorded in the PostgreSQL database.
func SyncMetadata(ctx context.Context, st storage.Store, projectStore *ProjectStore, buildStore *BuildStore, logger *zap.Logger) error {
	storageDirs, err := st.ListProjects(ctx)
	if err != nil {
		logger.Info("SyncMetadata: could not list projects (non-fatal)", zap.Error(err))
		return nil
	}

	dbProjects, err := projectStore.ListProjects(ctx)
	if err != nil {
		logger.Error("SyncMetadata: could not list DB projects", zap.Error(err))
		return nil
	}

	byStorageKey := make(map[string]*store.Project, len(dbProjects))
	for i := range dbProjects {
		byStorageKey[dbProjects[i].StorageKey] = &dbProjects[i]
	}

	for _, storageKey := range storageDirs {
		proj, ok := byStorageKey[storageKey]
		if !ok {
			logger.Warn("SyncMetadata: orphaned storage directory, skipping", zap.String("storage_key", storageKey))
			continue
		}
		if err := pgSyncProject(ctx, st, buildStore, proj); err != nil {
			logger.Error("SyncMetadata: skipping project", zap.String("storage_key", storageKey), zap.Error(err))
		}
	}
	return nil
}

// pgSyncProject syncs all builds for a single project into the PostgreSQL database.
func pgSyncProject(ctx context.Context, st storage.Store, bs *BuildStore, project *store.Project) error {
	storageOrders, err := st.ListReportBuilds(ctx, project.StorageKey)
	if err != nil {
		return fmt.Errorf("list report builds for project %q: %w", project.StorageKey, err)
	}
	if len(storageOrders) == 0 {
		return nil
	}

	existing, err := bs.ExistingBuildNumbers(ctx, project.ID)
	if err != nil {
		return err
	}

	var missing []int
	for _, bo := range storageOrders {
		if _, ok := existing[bo]; !ok {
			missing = append(missing, bo)
		}
	}

	if err := bs.InsertMissingBuilds(ctx, project.ID, missing); err != nil {
		return err
	}

	needStats, err := bs.BuildsWithMissingStats(ctx, project.ID)
	if err != nil {
		return err
	}
	return bs.BatchSyncStats(ctx, project.ID, project.StorageKey, needStats, st)
}
