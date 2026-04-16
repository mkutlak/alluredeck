package storage

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// MigrateEntry describes a single storage directory rename.
type MigrateEntry struct {
	OldKey string // current directory name (slug)
	NewKey string // target directory name (numeric ID)
}

// MigrateChildPaths renames storage directories for child projects from their
// old slug-based names to their numeric storage keys. The function is idempotent:
// if the new directory already exists and the old one does not, the entry is skipped.
func MigrateChildPaths(ctx context.Context, st Store, entries []MigrateEntry, logger *zap.Logger) error {
	var errs int
	for _, e := range entries {
		// Skip if already migrated (new key exists, old key doesn't).
		newExists, _ := st.ProjectExists(ctx, e.NewKey)
		if newExists {
			logger.Debug("storage migration: already migrated",
				zap.String("old_key", e.OldKey),
				zap.String("new_key", e.NewKey))
			continue
		}

		// Skip if source doesn't exist (nothing to migrate).
		oldExists, _ := st.ProjectExists(ctx, e.OldKey)
		if !oldExists {
			logger.Debug("storage migration: source not found, skipping",
				zap.String("old_key", e.OldKey),
				zap.String("new_key", e.NewKey))
			continue
		}

		logger.Info("storage migration: renaming project directory",
			zap.String("old_key", e.OldKey),
			zap.String("new_key", e.NewKey))

		if err := st.RenameProject(ctx, e.OldKey, e.NewKey); err != nil {
			logger.Error("storage migration: failed to rename",
				zap.String("old_key", e.OldKey),
				zap.String("new_key", e.NewKey),
				zap.Error(err))
			errs++
			continue
		}
	}

	if errs > 0 {
		return fmt.Errorf("storage migration: %d entries failed", errs)
	}
	return nil
}
