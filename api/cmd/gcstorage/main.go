// Command gcstorage is a one-shot maintenance tool that deletes orphaned
// report data from storage (local filesystem or S3).
//
// Historically the daily retention scheduler pruned storage under each
// project's SLUG while reports are stored under the project's STORAGE KEY
// (numeric for child projects since migration 0034). Deleting a nonexistent
// prefix is a silent no-op, so every scheduler-pruned child build left its
// report objects behind. The scheduler now prunes by storage key; this command
// cleans up the objects orphaned before that fix.
//
// For every project prefix in storage it diffs the reports/<build_order>/
// directories against the builds table and deletes directories whose build no
// longer exists in the database. Storage prefixes matching no project
// storage_key (deleted or renamed projects) are only reported, never deleted —
// removing them is a deliberate manual follow-up.
//
// Usage:
//
//	gcstorage                    # dry-run: report orphaned report dirs
//	gcstorage -apply             # actually delete the orphaned dirs
//	gcstorage -project 7         # restrict to project_id 7
//
// The run is continue-on-error: a single project failure is logged as a
// warning and the run proceeds; a summary is printed at the end and a run with
// any failures exits non-zero.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

func main() {
	os.Exit(run())
}

// run executes the sweep and returns a process exit code. main is kept a thin
// os.Exit(run()) wrapper so run's deferred cleanup (logger sync, pool close)
// always completes before the process exits.
func run() int {
	apply := flag.Bool("apply", false, "actually delete orphaned report dirs (default: dry-run)")
	projectID := flag.Int64("project", 0, "restrict the run to a single project_id (default: all projects)")
	flag.Usage = usage
	flag.Parse()

	cfg, encKey, logger := mustLoadConfig()
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	stores, err := bootstrap.InitStores(ctx, cfg, bootstrap.DefaultPoolConfig(), encKey, logger)
	if err != nil {
		logger.Error("failed to open PostgreSQL database", zap.Error(err))
		return 1
	}
	defer func() { _ = stores.Close() }()

	dataStore, err := createDataStore(cfg, logger)
	if err != nil {
		logger.Error("failed to init storage", zap.Error(err))
		return 1
	}

	projects, err := stores.Project.ListProjects(ctx)
	if err != nil {
		logger.Error("list projects failed; nothing to sweep", zap.Error(err))
		return 1
	}

	logger.Info("gcstorage scope",
		zap.Bool("apply", *apply),
		zap.Int64("project_filter", *projectID),
		zap.String("storage_type", cfg.StorageType))

	result := sweepStorageOrphans(ctx, projects, stores.Build, dataStore, sweepFlags{apply: *apply, projectID: *projectID}, logger)

	logger.Info("gcstorage summary",
		zap.Int("projects_scanned", result.projectsScanned),
		zap.Int("orphan_dirs", result.orphanDirs),
		zap.Int("dirs_deleted", result.dirsDeleted),
		zap.Strings("unknown_prefixes", result.unknownPrefixes),
		zap.Int("failures", result.failures))

	switch {
	case !*apply:
		logger.Info("dry-run complete; no storage objects were deleted (re-run with -apply to delete)")
	case result.failures > 0:
		logger.Error("sweep completed with failures", zap.Int("failures", result.failures))
		return 1
	default:
		logger.Info("sweep complete")
	}
	if result.failures > 0 {
		return 1
	}
	return 0
}

// sweepFlags holds the parsed command-line scoping options.
type sweepFlags struct {
	// apply deletes orphaned dirs; false reports them without deleting.
	apply bool
	// projectID, when > 0, restricts the run to a single project.
	projectID int64
}

// sweepResult accumulates per-run outcomes for the final summary.
type sweepResult struct {
	projectsScanned int
	// orphanDirs counts report dirs present in storage with no matching build row.
	orphanDirs int
	// dirsDeleted counts orphan dirs actually removed (0 in dry-run).
	dirsDeleted int
	// unknownPrefixes lists storage project prefixes matching no storage_key.
	unknownPrefixes []string
	failures        int
}

// buildLister is the narrow slice of store.BuildStorer the sweep needs.
type buildLister interface {
	ListBuilds(ctx context.Context, projectID int64) ([]store.Build, error)
}

// sweepStorageOrphans walks every project prefix in storage, diffs its
// reports/<build_order>/ dirs against the builds table, and — with apply —
// deletes the dirs whose build row no longer exists. Prefixes matching no
// project storage_key are reported in unknownPrefixes and left untouched. The
// run is continue-on-error: a project failure is logged and counted, and the
// sweep proceeds.
func sweepStorageOrphans(ctx context.Context, projects []store.Project, builds buildLister,
	dataStore storage.Store, flags sweepFlags, logger *zap.Logger) sweepResult {
	var result sweepResult

	byKey := make(map[string]*store.Project, len(projects))
	for i := range projects {
		byKey[projects[i].StorageKey] = &projects[i]
	}

	keys, err := dataStore.ListProjects(ctx)
	if err != nil {
		logger.Error("list storage project prefixes failed; nothing to sweep", zap.Error(err))
		result.failures++
		return result
	}

	for _, key := range keys {
		if ctx.Err() != nil {
			logger.Warn("sweep interrupted; stopping early", zap.Error(ctx.Err()))
			return result
		}

		p, ok := byKey[key]
		if !ok {
			// No project owns this prefix (deleted/renamed project). Flag only —
			// deleting whole project trees is a manual decision. A -project run
			// is scoped to that project, so unrelated prefixes are not reported.
			if flags.projectID > 0 {
				continue
			}
			logger.Warn("storage prefix matches no project storage_key; skipping",
				zap.String("prefix", key))
			result.unknownPrefixes = append(result.unknownPrefixes, key)
			continue
		}
		if flags.projectID > 0 && p.ID != flags.projectID {
			continue
		}
		result.projectsScanned++

		stored, err := dataStore.ListReportBuilds(ctx, key)
		if err != nil {
			logger.Warn("list report dirs failed; skipping project",
				zap.Int64("project_id", p.ID), zap.String("storage_key", key), zap.Error(err))
			result.failures++
			continue
		}
		if len(stored) == 0 {
			continue
		}

		rows, err := builds.ListBuilds(ctx, p.ID)
		if err != nil {
			logger.Warn("list builds failed; skipping project",
				zap.Int64("project_id", p.ID), zap.String("storage_key", key), zap.Error(err))
			result.failures++
			continue
		}
		known := make(map[int]struct{}, len(rows))
		for i := range rows {
			known[rows[i].BuildNumber] = struct{}{}
		}

		var orphans []int
		for _, bo := range stored {
			if _, ok := known[bo]; !ok {
				orphans = append(orphans, bo)
			}
		}
		if len(orphans) == 0 {
			continue
		}
		result.orphanDirs += len(orphans)

		if !flags.apply {
			logger.Info("would delete orphaned report dirs",
				zap.Int64("project_id", p.ID),
				zap.String("slug", p.Slug),
				zap.String("storage_key", key),
				zap.Ints("build_orders", orphans))
			continue
		}
		if err := dataStore.PruneReportDirs(ctx, key, orphans); err != nil {
			logger.Warn("delete orphaned report dirs failed; continuing",
				zap.Int64("project_id", p.ID), zap.String("storage_key", key), zap.Error(err))
			result.failures++
			continue
		}
		result.dirsDeleted += len(orphans)
		logger.Info("deleted orphaned report dirs",
			zap.Int64("project_id", p.ID),
			zap.String("slug", p.Slug),
			zap.String("storage_key", key),
			zap.Ints("build_orders", orphans))
	}

	return result
}

// createDataStore initialises the storage backend based on StorageType config.
// It mirrors the helper in cmd/api so both binaries address storage identically.
func createDataStore(cfg *config.Config, logger *zap.Logger) (storage.Store, error) {
	switch cfg.StorageType {
	case "s3":
		st, err := storage.NewS3Store(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("init S3 store: %w", err)
		}
		return st, nil
	default:
		return storage.NewLocalStore(cfg), nil
	}
}

// usage prints the command's help text.
func usage() {
	fmt.Fprint(os.Stderr, `gcstorage — delete report data orphaned in storage by the slug-keyed retention bug.

Diffs every storage project prefix's reports/<build_order>/ directories against
the builds table and deletes directories whose build no longer exists in the
database. Prefixes matching no project storage_key are reported, never deleted.

The default mode is a dry-run that only reports what would be deleted; pass
-apply to delete. The run is continue-on-error and exits non-zero if any
per-project step failed.

Usage:
  gcstorage [flags]

Flags:
`)
	flag.PrintDefaults()
}

// mustLoadConfig loads and validates configuration and initialises the logger.
// It mirrors the helper in cmd/api and cmd/backfill so all binaries read
// configuration identically. Terminates the process on any fatal error.
func mustLoadConfig() (*config.Config, []byte, *zap.Logger) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := bootstrap.InitLogger(cfg)

	if err := cfg.Validate(); err != nil {
		logger.Fatal("configuration error", zap.Error(err))
	}

	if cfg.SecurityEnabled {
		if err := cfg.HashPasswords(); err != nil {
			logger.Fatal("failed to hash passwords", zap.Error(err))
		}
	}

	encKey := security.DeriveEncryptionKey(cfg.JWTSecret)
	return cfg, encKey, logger
}
