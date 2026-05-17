// Command backfill is a one-shot maintenance tool that re-derives defect
// fingerprints after the 0042 status_message backfill migration.
//
// Allure 3 ("awesome") reports leave the test-level statusDetails empty; the
// real error message lives on the deepest failed step. Migration 0042 copies
// that message into test_results.status_message for rows ingested before the
// parser fix. Because defect fingerprints are derived from status_message, any
// fingerprints computed before the backfill are stale — they were built from a
// blank message that hashed to the literal "<no message>" fallback.
//
// This command runs after the migration (the migration is applied automatically
// when the database pool is opened) and re-runs fingerprinting for every build,
// reusing runner.(*Allure).BackfillFingerprints — the same code path used by
// live ingestion — so the fingerprinting algorithm is never duplicated.
//
// IMPORTANT: the 0042 migration is applied automatically the moment the
// database pool is opened — simply running this command (even with -dry-run)
// applies it. Pause report ingestion before running so no new builds are
// ingested mid-backfill.
//
// Usage:
//
//	backfill                     # re-fingerprint every project/build
//	backfill -dry-run            # report what would change without writing
//	backfill -project 7          # restrict to project_id 7
//	backfill -since-build 1500   # restrict to builds with id >= 1500
//
// A single build or project failure is logged as a warning and the run
// continues; a summary of successes and failures is printed at the end.
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
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

// backfillFlags holds the parsed command-line scoping options.
type backfillFlags struct {
	// dryRun reports affected builds without writing fingerprints.
	dryRun bool
	// projectID, when > 0, restricts the run to a single project.
	projectID int64
	// sinceBuild, when > 0, restricts the run to builds whose id is >= it.
	sinceBuild int64
}

func main() {
	os.Exit(run())
}

// run executes the backfill and returns a process exit code. main is kept a
// thin os.Exit(run()) wrapper so run's deferred cleanup (logger sync, pool
// close) always completes before the process exits.
func run() int {
	dryRun := flag.Bool("dry-run", false, "report affected builds without re-computing fingerprints")
	projectID := flag.Int64("project", 0, "restrict the run to a single project_id (default: all projects)")
	sinceBuild := flag.Int64("since-build", 0, "restrict the run to builds with id >= this value (default: all builds)")
	flag.Usage = usage
	flag.Parse()

	flags := backfillFlags{
		dryRun:     *dryRun,
		projectID:  *projectID,
		sinceBuild: *sinceBuild,
	}

	cfg, encKey, logger := mustLoadConfig()
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// InitStores opens the pool and runs all pending goose migrations,
	// including 0042_backfill_failed_step_messages.sql. dataStore is nil:
	// this command performs no filesystem/S3 access.
	stores, err := bootstrap.InitStores(ctx, cfg, bootstrap.DefaultPoolConfig(), encKey, nil, logger)
	if err != nil {
		logger.Error("failed to open PostgreSQL database", zap.Error(err))
		return 1
	}
	defer func() { _ = stores.Close() }()

	logger.Info("migrations applied; status_message backfill complete")
	logger.Info("backfill scope",
		zap.Bool("dry_run", flags.dryRun),
		zap.Int64("project_filter", flags.projectID),
		zap.Int64("since_build_filter", flags.sinceBuild))

	// A minimal Allure runner. computeDefectFingerprints (reached via
	// BackfillFingerprints) only touches the test-result store, the defect
	// store, and the logger — the storage/lock/branch dependencies are unused
	// on this path, so they are intentionally left nil.
	allureCore := runner.NewAllure(runner.AllureDeps{
		Config:          cfg,
		TestResultStore: stores.TestResult,
		DefectStore:     stores.Defect,
		Logger:          logger,
	})

	result := backfillFingerprints(ctx, allureCore, stores, flags, logger)

	logger.Info("backfill summary",
		zap.Int("builds_succeeded", result.succeeded),
		zap.Int("builds_failed", len(result.failedBuildIDs)),
		zap.Int("failed_tests", result.totalFailedTests),
		zap.Int64s("failed_build_ids", result.failedBuildIDs))

	switch {
	case flags.dryRun:
		logger.Info("dry-run complete; no fingerprints were modified")
	case len(result.failedBuildIDs) > 0:
		// Partial failure: surface a non-zero exit so operators/CI notice,
		// but the work that did succeed is preserved.
		logger.Error("backfill completed with failures",
			zap.Int("builds_failed", len(result.failedBuildIDs)),
			zap.Int64s("failed_build_ids", result.failedBuildIDs))
		return 1
	default:
		logger.Info("backfill complete")
	}
	return 0
}

// backfillResult accumulates per-run outcomes for the final summary.
type backfillResult struct {
	// succeeded counts builds re-fingerprinted (or, in dry-run, reported).
	succeeded int
	// totalFailedTests is the total number of failed/broken tests examined.
	totalFailedTests int
	// failedBuildIDs lists the build ids whose re-fingerprinting failed.
	failedBuildIDs []int64
}

// backfillFingerprints walks the scoped projects and builds and re-derives
// defect fingerprints from the (now backfilled) test_results.status_message
// values. It is continue-on-error: a single project or build failure is logged
// as a warning and the run proceeds, so one bad build cannot abort the whole
// backfill. The accumulated outcome is returned for the caller's summary.
func backfillFingerprints(ctx context.Context, allureCore *runner.Allure, stores *bootstrap.Stores, flags backfillFlags, logger *zap.Logger) backfillResult {
	var result backfillResult

	projects, err := stores.Project.ListProjects(ctx)
	if err != nil {
		// Listing projects is the entry point; without it there is nothing to
		// do. Log and return an empty result rather than aborting hard.
		logger.Error("list projects failed; nothing to backfill", zap.Error(err))
		return result
	}

	for _, p := range projects {
		// Honour the -project scope filter.
		if flags.projectID > 0 && p.ID != flags.projectID {
			continue
		}

		builds, err := stores.Build.ListBuilds(ctx, p.ID)
		if err != nil {
			logger.Warn("list builds failed; skipping project",
				zap.Int64("project_id", p.ID),
				zap.String("slug", p.Slug),
				zap.Error(err))
			continue
		}

		for i := range builds {
			b := &builds[i]

			// Honour the -since-build scope filter.
			if flags.sinceBuild > 0 && b.ID < flags.sinceBuild {
				continue
			}

			// Abort cleanly on SIGINT/SIGTERM rather than churning through
			// every remaining build.
			if ctx.Err() != nil {
				logger.Warn("backfill interrupted; stopping early", zap.Error(ctx.Err()))
				return result
			}

			failed, err := stores.TestResult.ListFailedForFingerprinting(ctx, p.ID, b.ID)
			if err != nil {
				logger.Warn("list failed tests failed; skipping build",
					zap.Int64("project_id", p.ID),
					zap.String("slug", p.Slug),
					zap.Int64("build_id", b.ID),
					zap.Int("build_number", b.BuildNumber),
					zap.Error(err))
				result.failedBuildIDs = append(result.failedBuildIDs, b.ID)
				continue
			}
			if len(failed) == 0 {
				continue
			}

			if flags.dryRun {
				logger.Info("would re-fingerprint build",
					zap.Int64("project_id", p.ID),
					zap.String("slug", p.Slug),
					zap.Int64("build_id", b.ID),
					zap.Int("build_number", b.BuildNumber),
					zap.Int("failed_tests", len(failed)))
				result.succeeded++
				result.totalFailedTests += len(failed)
				continue
			}

			if err := allureCore.BackfillFingerprints(ctx, p.ID, p.Slug, b.ID); err != nil {
				logger.Warn("re-fingerprint build failed; continuing",
					zap.Int64("project_id", p.ID),
					zap.String("slug", p.Slug),
					zap.Int64("build_id", b.ID),
					zap.Int("build_number", b.BuildNumber),
					zap.Error(err))
				result.failedBuildIDs = append(result.failedBuildIDs, b.ID)
				continue
			}

			logger.Info("re-fingerprinted build",
				zap.Int64("project_id", p.ID),
				zap.String("slug", p.Slug),
				zap.Int64("build_id", b.ID),
				zap.Int("build_number", b.BuildNumber),
				zap.Int("failed_tests", len(failed)))
			result.succeeded++
			result.totalFailedTests += len(failed)
		}
	}

	return result
}

// usage prints the command's help text, including the operational warning that
// the 0042 migration auto-applies the moment the database pool is opened.
func usage() {
	fmt.Fprint(os.Stderr, `backfill — re-derive defect fingerprints after the 0042 status_message migration.

The 0042 migration is applied automatically when the database pool is opened,
so simply running this command (even with -dry-run) applies it. Pause report
ingestion before running so no new builds are ingested mid-backfill.

The run is continue-on-error: a single project/build failure is logged as a
warning and the run proceeds; a summary of successes and failures is printed at
the end. A run with any per-build failures exits non-zero.

Usage:
  backfill [flags]

Flags:
`)
	flag.PrintDefaults()
}

// mustLoadConfig loads and validates configuration and initialises the logger.
// It mirrors the helper in cmd/api and cmd/mcp so all three binaries read
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
