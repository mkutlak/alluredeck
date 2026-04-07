package runner

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PlaywrightRunner processes uploaded Playwright HTML reports.
type PlaywrightRunner struct {
	cfg             *config.Config
	store           storage.Store
	buildStore      store.BuildStorer
	lockManager     store.Locker
	testResultStore store.TestResultStorer
	branchStore     store.BranchStorer
	defectStore     store.DefectStorer
	logger          *zap.Logger
}

// PlaywrightRunnerDeps holds the dependencies for creating a PlaywrightRunner.
type PlaywrightRunnerDeps struct {
	Config          *config.Config
	Store           storage.Store
	BuildStore      store.BuildStorer
	Locker          store.Locker
	TestResultStore store.TestResultStorer
	BranchStore     store.BranchStorer
	DefectStore     store.DefectStorer
	Logger          *zap.Logger
}

// NewPlaywrightRunner creates a new PlaywrightRunner.
func NewPlaywrightRunner(deps PlaywrightRunnerDeps) *PlaywrightRunner {
	return &PlaywrightRunner{
		cfg:             deps.Config,
		store:           deps.Store,
		buildStore:      deps.BuildStore,
		lockManager:     deps.Locker,
		testResultStore: deps.TestResultStore,
		branchStore:     deps.BranchStore,
		defectStore:     deps.DefectStore,
		logger:          deps.Logger,
	}
}

// IngestReport processes an already-uploaded Playwright HTML report for a project.
// It reads the report from playwright-reports/latest/, parses it, copies files to
// the numbered build directory, and stores test results and stats in the database.
func (pr *PlaywrightRunner) IngestReport(ctx context.Context, projectID, execName, execFrom, ciBranch, ciCommitSHA string) (string, error) {
	// 1. Acquire per-project lock to serialize concurrent report ingestion.
	unlock, err := pr.lockManager.AcquireLock(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("acquire lock: %w", err)
	}
	defer unlock()

	// 2. Get next build order atomically from the database.
	buildNumber, err := pr.buildStore.NextBuildNumber(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("next build order: %w", err)
	}

	// 3. Read index.html from playwright-reports/latest/ in storage.
	indexReader, _, err := pr.store.ReadPlaywrightFile(ctx, projectID, "latest/index.html")
	if err != nil {
		return "", fmt.Errorf("read playwright index.html: %w", err)
	}
	defer func() { _ = indexReader.Close() }()

	// 4. Extract and parse the Playwright report.
	reportJSON, fileJSONs, err := parser.ExtractPlaywrightData(indexReader)
	if err != nil {
		return "", fmt.Errorf("extract playwright data: %w", err)
	}

	results, meta, err := parser.ParsePlaywrightReport(reportJSON, fileJSONs)
	if err != nil {
		return "", fmt.Errorf("parse playwright report: %w", err)
	}

	// 5. Copy playwright-reports/latest/ to playwright-reports/{buildNumber}/.
	if err := pr.store.CopyPlaywrightLatestToBuild(ctx, projectID, buildNumber); err != nil {
		return "", fmt.Errorf("copy playwright report to build: %w", err)
	}

	// 6. Insert build record.
	if err := pr.buildStore.InsertBuild(ctx, projectID, buildNumber); err != nil {
		return "", fmt.Errorf("insert build: %w", err)
	}

	// 7. Mark this build as having a Playwright report.
	if err := pr.buildStore.SetHasPlaywrightReport(ctx, projectID, buildNumber, true); err != nil {
		pr.logger.Error("failed to set has_playwright_report",
			zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
	}

	// 8. Compute and store BuildStats from PlaywrightMeta.
	stats := store.BuildStats{
		Passed:     meta.Stats.Expected,
		Failed:     meta.Stats.Unexpected,
		Skipped:    meta.Stats.Skipped,
		Total:      meta.Stats.Total,
		DurationMs: meta.Duration,
		FlakyCount: meta.Stats.Flaky,
	}
	if err := pr.buildStore.UpdateBuildStats(ctx, projectID, buildNumber, stats); err != nil {
		pr.logger.Error("failed to update build stats",
			zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
	}

	// 9. Store CI metadata — fall back to report metadata if CI params not provided.
	ciMeta := store.CIMetadata{
		Provider:  execName,
		BuildURL:  execFrom,
		Branch:    ciBranch,
		CommitSHA: ciCommitSHA,
	}
	if ciMeta.Branch == "" {
		ciMeta.Branch = meta.Branch
	}
	if ciMeta.CommitSHA == "" {
		ciMeta.CommitSHA = meta.CommitSHA
	}
	if ciMeta.BuildURL == "" {
		ciMeta.BuildURL = meta.BuildURL
	}
	if ciMeta.Provider != "" || ciMeta.BuildURL != "" || ciMeta.Branch != "" || ciMeta.CommitSHA != "" {
		if err := pr.buildStore.UpdateBuildCIMetadata(ctx, projectID, buildNumber, ciMeta); err != nil {
			pr.logger.Warn("failed to store CI metadata",
				zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
		}
	}

	// 10. Resolve branch — auto-create if branch is available.
	var resolvedBranchID *int64
	branch := ciBranch
	if branch == "" {
		branch = meta.Branch
	}
	if branch != "" && pr.branchStore != nil {
		b, _, branchErr := pr.branchStore.GetOrCreate(ctx, projectID, branch)
		if branchErr != nil {
			pr.logger.Warn("failed to get/create branch",
				zap.String("project_id", projectID),
				zap.String("branch", branch),
				zap.Error(branchErr))
		} else {
			resolvedBranchID = &b.ID
		}
	}
	if resolvedBranchID != nil {
		if err := pr.buildStore.UpdateBuildBranchID(ctx, projectID, buildNumber, *resolvedBranchID); err != nil {
			pr.logger.Warn("failed to set build branch_id",
				zap.String("project_id", projectID),
				zap.Int("build_number", buildNumber),
				zap.Error(err))
		}
	}

	// 11. Insert per-test results.
	if pr.testResultStore != nil {
		buildID, err := pr.testResultStore.GetBuildID(ctx, projectID, buildNumber)
		if err == nil {
			testResults := make([]store.TestResult, 0, len(results))
			for _, r := range results {
				var startMs, stopMs *int64
				if r.StartMs != 0 {
					s, e := r.StartMs, r.StopMs
					startMs, stopMs = &s, &e
				}

				var thread, host string
				for _, lbl := range r.Labels {
					switch lbl.Name {
					case "thread":
						thread = lbl.Value
					case "host":
						host = lbl.Value
					}
				}

				testResults = append(testResults, store.TestResult{
					BuildID:    buildID,
					ProjectID:  projectID,
					TestName:   r.Name,
					FullName:   r.FullName,
					Status:     r.Status,
					HistoryID:  r.HistoryID,
					DurationMs: r.DurationMs,
					Flaky:      false,
					Retries:    0,
					StartMs:    startMs,
					StopMs:     stopMs,
					Thread:     thread,
					Host:       host,
				})
			}
			if err := pr.testResultStore.InsertBatch(ctx, testResults); err != nil {
				pr.logger.Warn("failed to insert test results",
					zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
			}

			// Enrich with full parsed data (labels, parameters, steps, attachments).
			if len(results) > 0 {
				if enrichErr := pr.testResultStore.InsertBatchFull(ctx, buildID, projectID, results); enrichErr != nil {
					pr.logger.Warn("failed to enrich test results",
						zap.String("project_id", projectID), zap.Error(enrichErr))
				}
			}

			// Compute defect fingerprints for failures.
			if pr.defectStore != nil && len(results) > 0 {
				if fpErr := pr.computeDefectFingerprints(ctx, projectID, buildID, results); fpErr != nil {
					pr.logger.Warn("failed to compute defect fingerprints",
						zap.String("project_id", projectID), zap.Error(fpErr))
				}
			}
		} else {
			pr.logger.Warn("failed to get build id",
				zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
		}
	}

	// 12. Set latest and prune.
	if err := pr.buildStore.SetLatestBranch(ctx, projectID, buildNumber, resolvedBranchID); err != nil {
		pr.logger.Error("failed to set latest build",
			zap.String("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(err))
	}

	if pr.cfg.KeepHistory {
		removed, err := pr.buildStore.PruneBuildsBranch(ctx, projectID, pr.cfg.KeepHistoryLatest, resolvedBranchID)
		if err != nil {
			pr.logger.Error("failed to prune builds",
				zap.String("project_id", projectID), zap.Error(err))
		} else if err := pr.store.PruneReportDirs(ctx, projectID, removed); err != nil {
			pr.logger.Error("failed to prune report dirs",
				zap.String("project_id", projectID), zap.Error(err))
		}

		if pr.cfg.KeepHistoryMaxAgeDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -pr.cfg.KeepHistoryMaxAgeDays)
			aged, err := pr.buildStore.PruneBuildsByAge(ctx, projectID, cutoff)
			if err == nil {
				_ = pr.store.PruneReportDirs(ctx, projectID, aged)
			}
		}
	}

	// 13. Clean up the staging directory now that files have been copied to the build.
	if err := pr.store.CleanPlaywrightLatest(ctx, projectID); err != nil {
		pr.logger.Warn("failed to clean playwright latest",
			zap.String("project_id", projectID), zap.Error(err))
	}

	// 14. Return success.
	return strconv.Itoa(buildNumber), nil
}

// computeDefectFingerprints queries failed test results, groups them by
// normalised fingerprint, upserts fingerprints into the defect store, links
// test results, and runs clean-build / auto-resolve / regression detection.
func (pr *PlaywrightRunner) computeDefectFingerprints(ctx context.Context, projectID string, buildID int64, _ []*parser.Result) error {
	failed, err := pr.testResultStore.ListFailedForFingerprinting(ctx, projectID, buildID)
	if err != nil {
		return fmt.Errorf("list failed for fingerprinting: %w", err)
	}

	if len(failed) == 0 {
		// No failures — still update clean build counts and auto-resolve.
		if err := pr.defectStore.UpdateCleanBuildCounts(ctx, projectID, buildID); err != nil {
			return fmt.Errorf("update clean build counts: %w", err)
		}
		if _, err := pr.defectStore.AutoResolveFixed(ctx, projectID, 3); err != nil {
			return fmt.Errorf("auto resolve fixed: %w", err)
		}
		return nil
	}

	// Group failures by normalised fingerprint.
	fpMap := ComputeFingerprintsForResults(failed)

	// Convert to store types and upsert.
	fingerprints := make([]store.DefectFingerprint, 0, len(fpMap))
	for _, fp := range fpMap {
		fingerprints = append(fingerprints, store.DefectFingerprint{
			FingerprintHash:   fp.Hash,
			NormalizedMessage: fp.NormalizedMessage,
			SampleTrace:       fp.NormalizedTrace,
			Category:          fp.Category,
			OccurrenceCount:   len(fp.TestResultIDs),
		})
	}

	if err := pr.defectStore.UpsertFingerprints(ctx, projectID, buildID, fingerprints); err != nil {
		return fmt.Errorf("upsert fingerprints: %w", err)
	}

	// Link test results to their fingerprint IDs.
	for _, fp := range fpMap {
		dfp, err := pr.defectStore.GetByHash(ctx, projectID, fp.Hash)
		if err != nil {
			pr.logger.Warn("failed to lookup fingerprint by hash",
				zap.String("hash", fp.Hash), zap.Error(err))
			continue
		}
		if err := pr.defectStore.LinkTestResults(ctx, dfp.ID, buildID, fp.TestResultIDs); err != nil {
			pr.logger.Warn("failed to link test results to fingerprint",
				zap.String("fingerprint_id", dfp.ID), zap.Error(err))
		}
	}

	// Update clean build counts for fingerprints not seen in this build.
	if err := pr.defectStore.UpdateCleanBuildCounts(ctx, projectID, buildID); err != nil {
		return fmt.Errorf("update clean build counts: %w", err)
	}

	// Auto-resolve fingerprints that have been clean for 3 consecutive builds.
	if _, err := pr.defectStore.AutoResolveFixed(ctx, projectID, 3); err != nil {
		return fmt.Errorf("auto resolve fixed: %w", err)
	}

	// Detect regressions and log if any found.
	regressions, err := pr.defectStore.DetectRegressions(ctx, projectID, buildID)
	if err != nil {
		pr.logger.Warn("failed to detect regressions",
			zap.String("project_id", projectID), zap.Error(err))
	} else if len(regressions) > 0 {
		pr.logger.Info("detected defect regressions",
			zap.String("project_id", projectID),
			zap.Int("count", len(regressions)))
	}

	return nil
}

