package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ErrProjectExists is returned when creating a project that already exists
var ErrProjectExists = errors.New("project already exists")

// Sentinel errors for allure runner operations.
var (
	ErrStatsNotFound   = errors.New("build stats not found")
	ErrAllureCmdFailed = errors.New("allure command failed")
)

// stabilityEntry is used to parse test-result JSON files for stability data.
type stabilityEntry struct {
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	Status        string `json:"status"`
	HistoryID     string `json:"historyId"`
	NewFailed     bool   `json:"newFailed"`
	NewPassed     bool   `json:"newPassed"`
	RetriesCount  int    `json:"retriesCount"`
	StatusDetails *struct {
		Flaky bool `json:"flaky"`
	} `json:"statusDetails"`
	Time *struct {
		Start    int64 `json:"start"`
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	} `json:"time"`
	Start    int64 `json:"start"`    // Allure 3 top-level fallback
	Stop     int64 `json:"stop"`     // Allure 3 top-level fallback
	Duration int64 `json:"duration"` // Allure 3 top-level fallback
	Labels   []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"labels"`
}

// GenerateBatchID returns a random 32-character hex string for use as a
// per-upload batch identifier under the results directory.
func GenerateBatchID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Allure represents the core Allure report generation process
type Allure struct {
	cfg             *config.Config
	store           storage.Store
	buildStore      store.BuildStorer
	lockManager     store.Locker
	testResultStore store.TestResultStorer
	branchStore     store.BranchStorer
	defectStore     store.DefectStorer
	attachmentStore store.AttachmentStorer
	logger          *zap.Logger
}

// AllureDeps holds the dependencies for creating an Allure runner.
type AllureDeps struct {
	Config          *config.Config
	Store           storage.Store
	BuildStore      store.BuildStorer
	Locker          store.Locker
	TestResultStore store.TestResultStorer
	BranchStore     store.BranchStorer
	DefectStore     store.DefectStorer
	AttachmentStore store.AttachmentStorer
	Logger          *zap.Logger
}

// NewAllure creates a new Allure runner
func NewAllure(deps AllureDeps) *Allure {
	return &Allure{
		cfg:             deps.Config,
		store:           deps.Store,
		buildStore:      deps.BuildStore,
		lockManager:     deps.Locker,
		testResultStore: deps.TestResultStore,
		branchStore:     deps.BranchStore,
		defectStore:     deps.DefectStore,
		attachmentStore: deps.AttachmentStore,
		logger:          deps.Logger,
	}
}

// ExecutorJSON holds the executor metadata written to results/executor.json before report generation.
type ExecutorJSON struct {
	ReportName  string `json:"reportName"`
	BuildName   string `json:"buildName"`
	BuildNumber string `json:"buildNumber"`
	Name        string `json:"name"`
	ReportURL   string `json:"reportUrl"`
	BuildURL    string `json:"buildUrl"`
	Type        string `json:"type"`
}

// writeExecutorJSON writes executor metadata to the results directory.
// If storeResults is false the executor file is written as an empty JSON object.
func writeExecutorJSON(resultsDir, projectID, execName, execFrom, execType string, buildNumber int, storeResults bool) error {
	executorPath := filepath.Join(resultsDir, "executor.json")
	if storeResults {
		executorData := ExecutorJSON{
			ReportName:  projectID,
			BuildName:   fmt.Sprintf("%s #%d", projectID, buildNumber),
			BuildNumber: strconv.Itoa(buildNumber),
			Name:        execName,
			ReportURL:   fmt.Sprintf("../%d/index.html", buildNumber),
			BuildURL:    execFrom,
			Type:        execType,
		}
		d, err := json.MarshalIndent(executorData, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal executor.json: %w", err)
		}
		//nolint:gosec // G306: 0o644 required for allure CLI to read executor.json
		if err := os.WriteFile(executorPath, d, 0o644); err != nil {
			return fmt.Errorf("write executor.json: %w", err)
		}
		return nil
	}
	//nolint:gosec // G306: 0o644 required for allure CLI to read executor.json
	if err := os.WriteFile(executorPath, []byte("{}"), 0o644); err != nil {
		return fmt.Errorf("write executor.json: %w", err)
	}
	return nil
}

// parseStabilityEntries reads test-result JSON files from the generated report
// and returns stability entries for processing.
func (a *Allure) parseStabilityEntries(ctx context.Context, storageKey, reportID string) ([]stabilityEntry, error) {
	relBase := "reports/" + reportID + "/data/test-results"
	entries, err := a.store.ReadDir(ctx, storageKey, relBase)
	if err != nil {
		return nil, fmt.Errorf("read test-results dir: %w", err)
	}

	var results []stabilityEntry
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		data, err := a.store.ReadFile(ctx, storageKey, relBase+"/"+entry.Name)
		if err != nil {
			a.logger.Warn("skipping test-result file for stability",
				zap.String("file", entry.Name), zap.Error(err))
			continue
		}
		var se stabilityEntry
		if json.Unmarshal(data, &se) != nil {
			continue
		}
		results = append(results, se)
	}
	return results, nil
}

// storeAndPruneBuild stores a report snapshot and records it in the database.
// storageKey is used for storage (filesystem/S3) operations; projectID (int64) is used for DB operations.
// slug is the human-readable identifier used for logging only.
// batchID scopes the results directory to the upload batch subdirectory.
func (a *Allure) storeAndPruneBuild(ctx context.Context, projectID int64, slug, storageKey, batchID, localProjectDir string, buildNumber int, ciMeta store.CIMetadata, branchID *int64) error {
	if err := a.store.PublishReport(ctx, storageKey, buildNumber, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	if err := a.buildStore.InsertBuild(ctx, projectID, buildNumber); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	// Associate build with branch if resolved.
	if branchID != nil {
		if err := a.buildStore.UpdateBuildBranchID(ctx, projectID, buildNumber, *branchID); err != nil {
			a.logger.Warn("failed to set build branch_id",
				zap.String("slug", slug),
				zap.Int("build_number", buildNumber),
				zap.Error(err))
		}
	}
	if stats, err := a.store.ReadBuildStats(ctx, storageKey, buildNumber); err == nil {
		storeStats := store.BuildStats{
			Passed:     stats.Passed,
			Failed:     stats.Failed,
			Broken:     stats.Broken,
			Skipped:    stats.Skipped,
			Unknown:    stats.Unknown,
			Total:      stats.Total,
			DurationMs: stats.DurationMs,
		}

		// Parse stability data from generated report JSON files.
		if stabilityEntries, err := a.parseStabilityEntries(ctx, storageKey, "latest"); err == nil {
			for i := range stabilityEntries {
				se := &stabilityEntries[i]
				if se.StatusDetails != nil && se.StatusDetails.Flaky {
					storeStats.FlakyCount++
				}
				if se.RetriesCount > 0 {
					storeStats.RetriedCount++
				}
				if se.NewFailed {
					storeStats.NewFailedCount++
				}
				if se.NewPassed {
					storeStats.NewPassedCount++
				}
			}

			// Insert per-test results if testResultStore is available.
			if a.testResultStore != nil {
				buildID, err := a.testResultStore.GetBuildID(ctx, projectID, buildNumber)
				if err == nil {
					testResults := make([]store.TestResult, 0, len(stabilityEntries))
					for i := range stabilityEntries {
						se := &stabilityEntries[i]
						dur := se.Duration
						if se.Time != nil {
							dur = se.Time.Duration
						}
						flaky := se.StatusDetails != nil && se.StatusDetails.Flaky

						// Extract start/stop: nested Time takes priority, top-level as fallback.
						var startMs, stopMs *int64
						if se.Time != nil && se.Time.Start != 0 {
							s, e := se.Time.Start, se.Time.Stop
							startMs, stopMs = &s, &e
						} else if se.Start != 0 {
							s, e := se.Start, se.Stop
							startMs, stopMs = &s, &e
						}

						// Extract thread/host from labels.
						var thread, host string
						for _, lbl := range se.Labels {
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
							TestName:   se.Name,
							FullName:   se.FullName,
							Status:     se.Status,
							HistoryID:  se.HistoryID,
							DurationMs: dur,
							Flaky:      flaky,
							Retries:    se.RetriesCount,
							NewFailed:  se.NewFailed,
							NewPassed:  se.NewPassed,
							StartMs:    startMs,
							StopMs:     stopMs,
							Thread:     thread,
							Host:       host,
						})
					}
					if err := a.testResultStore.InsertBatch(ctx, testResults); err != nil {
						a.logger.Warn("failed to insert test results",
							zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
					}
					// Enrich with full parsed data (labels, parameters, steps, attachments).
					resultsDir := filepath.Join(localProjectDir, "results", batchID)
					if parsedResults, parseErr := parser.ParseDir(resultsDir); parseErr != nil {
						a.logger.Warn("failed to parse raw results for enrichment",
							zap.String("slug", slug), zap.Error(parseErr))
					} else if len(parsedResults) > 0 {
						reportDataDir := filepath.Join(localProjectDir, "reports", "latest", "data")
						parser.ResolveAttachments(parsedResults, reportDataDir)
						if enrichErr := a.testResultStore.InsertBatchFull(ctx, buildID, projectID, parsedResults); enrichErr != nil {
							a.logger.Warn("failed to enrich test results",
								zap.String("slug", slug), zap.Error(enrichErr))
						}
						if a.defectStore != nil && len(parsedResults) > 0 {
							if fpErr := a.computeDefectFingerprints(ctx, projectID, slug, buildID, parsedResults); fpErr != nil {
								a.logger.Warn("failed to compute defect fingerprints",
									zap.String("slug", slug), zap.Error(fpErr))
							}
						}
					}
				} else {
					a.logger.Warn("failed to get build id for test results",
						zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
				}
			}
		} else {
			a.logger.Warn("failed to parse stability entries",
				zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		}

		if err := a.buildStore.UpdateBuildStats(ctx, projectID, buildNumber, storeStats); err != nil {
			a.logger.Error("failed to cache build stats",
				zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		}
	}
	if ciMeta.Provider != "" || ciMeta.BuildURL != "" || ciMeta.Branch != "" || ciMeta.CommitSHA != "" {
		if err := a.buildStore.UpdateBuildCIMetadata(ctx, projectID, buildNumber, ciMeta); err != nil {
			a.logger.Warn("failed to store CI metadata",
				zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		}
	}

	// Copy any pending Playwright report from latest/ to the numbered build directory.
	a.copyPlaywrightReport(ctx, projectID, slug, storageKey, buildNumber)

	if err := a.buildStore.SetLatestBranch(ctx, projectID, buildNumber, branchID); err != nil {
		a.logger.Error("failed to set latest build",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
	}
	return nil
}

// recordBuild records the build in the database for pruning without publishing
// a report snapshot. Used when storeResults=false but KeepHistory=true.
func (a *Allure) recordBuild(ctx context.Context, projectID int64, slug string, buildNumber int) error {
	if err := a.buildStore.InsertBuild(ctx, projectID, buildNumber); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	if err := a.buildStore.SetLatestBranch(ctx, projectID, buildNumber, nil); err != nil {
		a.logger.Error("failed to set latest build (recordBuild)",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
	}
	return nil
}

// GenerateReport implements generateAllureReport.sh
func (a *Allure) GenerateReport(ctx context.Context, projectID int64, slug, storageKey, batchID, execName, execFrom, execType string, storeResults bool, ciBranch, ciCommitSHA string) (string, error) {
	if execName == "" {
		execName = "Automatic Execution"
	}
	if execType == "" {
		execType = "another"
	}

	// 1. Acquire per-project lock to serialize concurrent report generation.
	unlock, err := a.lockManager.AcquireLock(ctx, slug)
	if err != nil {
		return "", fmt.Errorf("acquire generation lock: %w", err)
	}
	defer unlock()

	// 2. Get next build order atomically from the database.
	buildNumber, err := a.buildStore.NextBuildNumber(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("next build order: %w", err)
	}

	// 3. PrepareLocal returns the project dir (local) or a temp dir (S3).
	localProjectDir, err := a.store.PrepareLocal(ctx, storageKey)
	if err != nil {
		return "", fmt.Errorf("prepare local dir for %q: %w", slug, err)
	}
	defer func() { _ = a.store.CleanupLocal(localProjectDir) }()

	resultsDir := filepath.Join(localProjectDir, "results", batchID)

	// 4. Write executor.json directly — always local (temp dir in S3 mode)
	if err := writeExecutorJSON(resultsDir, slug, execName, execFrom, execType, buildNumber, storeResults); err != nil {
		return "", err
	}

	// 5. Generate Allure 3 Config (written to results dir — allure reads it automatically)
	configPath := filepath.Join(resultsDir, "allurereport.config.json")
	configData := map[string]any{
		"plugins": []string{"awesome"},
	}
	cf, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal allurereport.config.json: %w", err)
	}
	//nolint:gosec // G306: 0o644 required for allure CLI to read config
	if err := os.WriteFile(configPath, cf, 0o644); err != nil {
		return "", fmt.Errorf("write allurereport.config.json: %w", err)
	}

	// 6. Generate Report — allure 3 reads allurereport.config.json from the results dir automatically
	latestReportDir := filepath.Join(localProjectDir, "reports", "latest")

	// 6a–6c. Preserve history, clear stale output, run allure generate.
	if err := a.runAllureGenerate(ctx, slug, storageKey, batchID, latestReportDir, localProjectDir); err != nil {
		return "", err
	}

	// Resolve branch — auto-create if ci_branch is provided.
	var resolvedBranchID *int64
	if ciBranch != "" && a.branchStore != nil {
		branch, _, branchErr := a.branchStore.GetOrCreate(ctx, projectID, ciBranch)
		if branchErr != nil {
			a.logger.Warn("failed to get/create branch",
				zap.String("slug", slug),
				zap.String("branch", ciBranch),
				zap.Error(branchErr))
		} else {
			resolvedBranchID = &branch.ID
		}
	}

	// 7. Store Report and record in database
	if a.cfg.KeepHistory {
		if storeResults {
			ciMeta := store.CIMetadata{
				Provider:  execName,
				BuildURL:  execFrom,
				Branch:    ciBranch,
				CommitSHA: ciCommitSHA,
			}
			if err := a.storeAndPruneBuild(ctx, projectID, slug, storageKey, batchID, localProjectDir, buildNumber, ciMeta, resolvedBranchID); err != nil {
				return "", err
			}
		} else {
			if err := a.recordBuild(ctx, projectID, slug, buildNumber); err != nil {
				return "", err
			}
		}
	}

	// 8. Keep Latest History (Cleanup old reports)
	if err := a.KeepLatestHistory(ctx, projectID, slug, storageKey, resolvedBranchID); err != nil {
		return "", err
	}

	// 9. Clean up the batch directory now that the report has been generated.
	if batchID != "" {
		if err := a.store.CleanBatch(ctx, storageKey, batchID); err != nil {
			a.logger.Warn("post-generation batch cleanup failed",
				zap.String("slug", slug),
				zap.String("storage_key", storageKey),
				zap.String("batch_id", batchID),
				zap.Error(err))
		}
	}

	return strconv.Itoa(buildNumber), nil
}

// CleanHistory delegates to the store module and regenerates.
// projectID is the numeric surrogate key; slug is the human-readable identifier; storageKey is used for storage operations.
func (a *Allure) CleanHistory(ctx context.Context, projectID int64, slug, storageKey string) error {
	if err := a.store.CleanHistory(ctx, storageKey); err != nil {
		return fmt.Errorf("clean history for %q: %w", slug, err)
	}

	// Clean build and test-result records from the database.
	if err := a.buildStore.DeleteAllBuilds(ctx, projectID); err != nil {
		return fmt.Errorf("clean history db builds for %q: %w", slug, err)
	}
	if a.testResultStore != nil {
		if err := a.testResultStore.DeleteByProject(ctx, projectID); err != nil {
			return fmt.Errorf("clean history db test results for %q: %w", slug, err)
		}
	}

	checkSecs := strings.ToUpper(a.cfg.CheckResultsEverySeconds)
	if checkSecs != "NONE" {
		if err := a.store.KeepHistory(ctx, storageKey, ""); err != nil {
			return fmt.Errorf("keep history for %q: %w", slug, err)
		}

		if _, err := a.GenerateReport(ctx, projectID, slug, storageKey, "", "", "", "", false, "", ""); err != nil {
			return err
		}
	}

	return nil
}

// KeepHistory delegates to the store module.
// batchID determines the target subdirectory for the history copy.
func (a *Allure) KeepHistory(ctx context.Context, storageKey, batchID string) error {
	if err := a.store.KeepHistory(ctx, storageKey, batchID); err != nil {
		return fmt.Errorf("keep history for %q: %w", storageKey, err)
	}
	return nil
}

// DeleteProject removes the entire project (filesystem + S3).
func (a *Allure) DeleteProject(ctx context.Context, storageKey string) error {
	if err := a.store.DeleteProject(ctx, storageKey); err != nil {
		return fmt.Errorf("delete project %q: %w", storageKey, err)
	}
	return nil
}

// DeleteReport removes a single numbered report directory for a project.
func (a *Allure) DeleteReport(ctx context.Context, projectID int64, slug, storageKey, reportID string) error {
	if err := a.store.DeleteReport(ctx, storageKey, reportID); err != nil {
		return fmt.Errorf("delete report %q for %q: %w", reportID, slug, err)
	}

	// Clean the corresponding build and test-result records from the database.
	if buildNumber, err := strconv.Atoi(reportID); err == nil {
		if dbErr := a.buildStore.DeleteBuild(ctx, projectID, buildNumber); dbErr != nil {
			a.logger.Warn("failed to delete build from db",
				zap.Int64("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(dbErr))
		}
		if a.testResultStore != nil {
			if buildID, idErr := a.testResultStore.GetBuildID(ctx, projectID, buildNumber); idErr == nil {
				if dbErr := a.testResultStore.DeleteByBuild(ctx, buildID); dbErr != nil {
					a.logger.Warn("failed to delete test results from db",
						zap.Int64("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(dbErr))
				}
			}
		}
	}

	return nil
}

// CleanResults delegates to the store module
func (a *Allure) CleanResults(ctx context.Context, storageKey string) error {
	if err := a.store.CleanResults(ctx, storageKey); err != nil {
		return fmt.Errorf("clean results for %q: %w", storageKey, err)
	}
	return nil
}

// CreateProject creates the necessary directories for a new project
func (a *Allure) CreateProject(ctx context.Context, storageKey string) error {
	projectDir := filepath.Join(a.cfg.ProjectsPath, storageKey)

	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("%w: %s", ErrProjectExists, storageKey)
	}

	if err := a.store.CreateProject(ctx, storageKey); err != nil {
		return fmt.Errorf("create project %q: %w", storageKey, err)
	}
	return nil
}

// StoreReport copies variable-content subdirs of the latest report to a numbered snapshot.
// This thin wrapper exists for backward compatibility with tests; new code should call
// store.PublishReport directly with the localProjectDir from PrepareLocal.
func (a *Allure) StoreReport(ctx context.Context, storageKey string, buildNumber int) error {
	localProjectDir := filepath.Join(a.cfg.ProjectsPath, storageKey)
	if err := a.store.PublishReport(ctx, storageKey, buildNumber, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	return nil
}

// KeepLatestHistory removes the oldest historical report directories when count exceeds keepLatest.
// Uses the database to determine which builds to prune, then removes their filesystem directories.
func (a *Allure) KeepLatestHistory(ctx context.Context, projectID int64, slug, storageKey string, branchID *int64) error {
	if !a.cfg.KeepHistory {
		return nil
	}
	removed, err := a.buildStore.PruneBuildsBranch(ctx, projectID, a.cfg.KeepHistoryLatest, branchID)
	if err != nil {
		return fmt.Errorf("prune builds from db: %w", err)
	}

	if err := a.store.PruneReportDirs(ctx, storageKey, removed); err != nil {
		return fmt.Errorf("prune report dirs: %w", err)
	}

	if a.cfg.KeepHistoryMaxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -a.cfg.KeepHistoryMaxAgeDays)
		aged, err := a.buildStore.PruneBuildsByAge(ctx, projectID, cutoff)
		if err != nil {
			return fmt.Errorf("prune builds by age: %w", err)
		}
		if err := a.store.PruneReportDirs(ctx, storageKey, aged); err != nil {
			return fmt.Errorf("prune aged report dirs: %w", err)
		}
	}

	return nil
}

// runAllureGenerate preserves history trends, clears the stale latest report
// directory, and runs `allure generate` to produce a fresh report.
// batchID scopes both the history copy and the allure input directory.
func (a *Allure) runAllureGenerate(ctx context.Context, slug, storageKey, batchID, latestReportDir, localProjectDir string) error {
	if err := a.store.KeepHistory(ctx, storageKey, batchID); err != nil {
		a.logger.Error("KeepHistory failed", zap.String("slug", slug), zap.Error(err))
	}
	if err := os.RemoveAll(latestReportDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing latest report dir: %w", err)
	}
	return a.runAllureCmd(ctx, "generate", "--output", latestReportDir, "--cwd", localProjectDir, filepath.Join("results", batchID))
}

func (a *Allure) runAllureCmd(ctx context.Context, args ...string) error {
	const allureTimeout = 5 * time.Minute
	cmdCtx, cancel := context.WithTimeout(ctx, allureTimeout)
	defer cancel()
	//nolint:gosec // G204: "allure" is a fixed binary name, not user-controlled
	cmd := exec.CommandContext(cmdCtx, "allure", args...)
	var outBuff, errBuff bytes.Buffer
	cmd.Stdout = &outBuff
	cmd.Stderr = &errBuff
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: subprocess: %w, stderr: %s", ErrAllureCmdFailed, err, errBuff.String())
	}
	if outBuff.Len() > 0 {
		a.logger.Debug("allure command output", zap.String("stdout", outBuff.String()))
	}
	return nil
}

// copyPlaywrightReport copies a Playwright report from playwright-reports/latest/
// to the numbered build directory (if one was uploaded). CopyPlaywrightLatestToBuild
// is a no-op when latest/ is absent or empty. After copying, sets
// has_playwright_report on the build, extracts attachment metadata from data/,
// inserts it, then cleans the latest/ directory.
func (a *Allure) copyPlaywrightReport(ctx context.Context, projectID int64, slug, storageKey string, buildNumber int) {
	if err := a.store.CopyPlaywrightLatestToBuild(ctx, storageKey, buildNumber); err != nil {
		a.logger.Warn("failed to copy playwright latest to build",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		return
	}

	// Check whether the copy produced a valid report directory.
	exists, err := a.store.PlaywrightReportExists(ctx, storageKey, buildNumber)
	if err != nil {
		a.logger.Warn("failed to check playwright report existence",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		return
	}
	if !exists {
		return
	}

	if err := a.buildStore.SetHasPlaywrightReport(ctx, projectID, buildNumber, true); err != nil {
		a.logger.Warn("failed to set has_playwright_report",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
	}

	// Extract attachment metadata from data/ and insert into test_attachments.
	if a.attachmentStore != nil && a.testResultStore != nil {
		files, err := a.store.ListPlaywrightDataFiles(ctx, storageKey, buildNumber)
		if err != nil {
			a.logger.Warn("failed to list playwright data files",
				zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
		} else if len(files) > 0 {
			buildID, idErr := a.testResultStore.GetBuildID(ctx, projectID, buildNumber)
			if idErr != nil {
				a.logger.Warn("failed to get build id for playwright attachments",
					zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(idErr))
			} else {
				attachments := make([]store.TestAttachment, 0, len(files))
				for _, f := range files {
					mime := mimeTypeFromExt(filepath.Ext(f))
					if mime == "" {
						continue // skip .dat and unknown files
					}
					attachments = append(attachments, store.TestAttachment{
						Name:     filepath.Base(f),
						Source:   "data/" + f,
						MimeType: mime,
					})
				}
				if len(attachments) > 0 {
					if insertErr := a.attachmentStore.InsertBuildAttachments(ctx, buildID, projectID, attachments); insertErr != nil {
						a.logger.Warn("failed to insert playwright attachments",
							zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(insertErr))
					}
				}
			}
		}
	}

	if err := a.store.CleanPlaywrightLatest(ctx, storageKey); err != nil {
		a.logger.Warn("failed to clean playwright latest",
			zap.String("slug", slug), zap.Int("build_number", buildNumber), zap.Error(err))
	}
}

// mimeTypeFromExt returns the MIME type for a known Playwright attachment file
// extension. Returns an empty string for extensions that should be skipped (e.g. .dat).
func mimeTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".zip":
		return "application/zip"
	case ".webm":
		return "video/webm"
	case ".mp4":
		return "video/mp4"
	case ".txt", ".log":
		return "text/plain"
	case ".dat":
		return "" // allure step metadata — skip
	default:
		return ""
	}
}

// computeDefectFingerprints queries failed test results, groups them by
// normalised fingerprint, upserts fingerprints into the defect store, links
// test results, and runs clean-build / auto-resolve / regression detection.
func (a *Allure) computeDefectFingerprints(ctx context.Context, projectID int64, slug string, buildID int64, _ []*parser.Result) error {
	failed, err := a.testResultStore.ListFailedForFingerprinting(ctx, projectID, buildID)
	if err != nil {
		return fmt.Errorf("list failed for fingerprinting: %w", err)
	}

	if len(failed) == 0 {
		// No failures — still update clean build counts and auto-resolve.
		if err := a.defectStore.UpdateCleanBuildCounts(ctx, projectID, buildID); err != nil {
			return fmt.Errorf("update clean build counts: %w", err)
		}
		if _, err := a.defectStore.AutoResolveFixed(ctx, projectID, 3); err != nil {
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

	if err := a.defectStore.UpsertFingerprints(ctx, projectID, buildID, fingerprints); err != nil {
		return fmt.Errorf("upsert fingerprints: %w", err)
	}

	// Link test results to their fingerprint IDs.
	for _, fp := range fpMap {
		dfp, err := a.defectStore.GetByHash(ctx, projectID, fp.Hash)
		if err != nil {
			a.logger.Warn("failed to lookup fingerprint by hash",
				zap.String("hash", fp.Hash), zap.Error(err))
			continue
		}
		if err := a.defectStore.LinkTestResults(ctx, dfp.ID, buildID, fp.TestResultIDs); err != nil {
			a.logger.Warn("failed to link test results to fingerprint",
				zap.String("fingerprint_id", dfp.ID), zap.Error(err))
		}
	}

	// Update clean build counts for fingerprints not seen in this build.
	if err := a.defectStore.UpdateCleanBuildCounts(ctx, projectID, buildID); err != nil {
		return fmt.Errorf("update clean build counts: %w", err)
	}

	// Auto-resolve fingerprints that have been clean for 3 consecutive builds.
	if _, err := a.defectStore.AutoResolveFixed(ctx, projectID, 3); err != nil {
		return fmt.Errorf("auto resolve fixed: %w", err)
	}

	// Detect regressions and log if any found.
	regressions, err := a.defectStore.DetectRegressions(ctx, projectID, buildID)
	if err != nil {
		a.logger.Warn("failed to detect regressions",
			zap.String("slug", slug), zap.Error(err))
	} else if len(regressions) > 0 {
		a.logger.Info("detected defect regressions",
			zap.String("slug", slug),
			zap.Int("count", len(regressions)))
	}

	return nil
}
