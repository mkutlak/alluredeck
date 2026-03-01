package runner

import (
	"bytes"
	"context"
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

// Allure represents the core Allure report generation process
type Allure struct {
	cfg             *config.Config
	store           storage.Store
	buildStore      *store.BuildStore
	lockManager     *store.LockManager
	testResultStore *store.TestResultStore
	logger          *zap.Logger
}

// NewAllure creates a new Allure runner
func NewAllure(cfg *config.Config, dataStore storage.Store, buildStore *store.BuildStore, lockManager *store.LockManager, testResultStore *store.TestResultStore, logger *zap.Logger) *Allure {
	return &Allure{
		cfg:             cfg,
		store:           dataStore,
		buildStore:      buildStore,
		lockManager:     lockManager,
		testResultStore: testResultStore,
		logger:          logger,
	}
}

// ExecutorJSON holds the executor metadata written to results/executor.json before report generation.
type ExecutorJSON struct {
	ReportName string `json:"reportName"`
	BuildName  string `json:"buildName"`
	BuildOrder string `json:"buildOrder"`
	Name       string `json:"name"`
	ReportURL  string `json:"reportUrl"`
	BuildURL   string `json:"buildUrl"`
	Type       string `json:"type"`
}

// writeExecutorJSON writes executor metadata to the results directory.
// If storeResults is false the executor file is written as an empty JSON object.
func writeExecutorJSON(resultsDir, projectID, execName, execFrom, execType string, buildOrder int, storeResults bool) error {
	executorPath := filepath.Join(resultsDir, "executor.json")
	if storeResults {
		executorData := ExecutorJSON{
			ReportName: projectID,
			BuildName:  fmt.Sprintf("%s #%d", projectID, buildOrder),
			BuildOrder: strconv.Itoa(buildOrder),
			Name:       execName,
			ReportURL:  fmt.Sprintf("../%d/index.html", buildOrder),
			BuildURL:   execFrom,
			Type:       execType,
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
func (a *Allure) parseStabilityEntries(ctx context.Context, projectID, reportID string) ([]stabilityEntry, error) {
	relBase := "reports/" + reportID + "/data/test-results"
	entries, err := a.store.ReadDir(ctx, projectID, relBase)
	if err != nil {
		return nil, fmt.Errorf("read test-results dir: %w", err)
	}

	var results []stabilityEntry
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		data, err := a.store.ReadFile(ctx, projectID, relBase+"/"+entry.Name)
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
func (a *Allure) storeAndPruneBuild(ctx context.Context, projectID, localProjectDir string, buildOrder int, ciMeta store.CIMetadata) error {
	if err := a.store.PublishReport(ctx, projectID, buildOrder, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	if err := a.buildStore.InsertBuild(ctx, projectID, buildOrder); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	if stats, err := a.store.ReadBuildStats(ctx, projectID, buildOrder); err == nil {
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
		if stabilityEntries, err := a.parseStabilityEntries(ctx, projectID, "latest"); err == nil {
			for _, se := range stabilityEntries {
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
				buildID, err := a.testResultStore.GetBuildID(ctx, projectID, buildOrder)
				if err == nil {
					testResults := make([]store.TestResult, 0, len(stabilityEntries))
					for _, se := range stabilityEntries {
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
							zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
					}
				} else {
					a.logger.Warn("failed to get build id for test results",
						zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
				}
			}
		} else {
			a.logger.Warn("failed to parse stability entries",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
		}

		if err := a.buildStore.UpdateBuildStats(ctx, projectID, buildOrder, storeStats); err != nil {
			a.logger.Error("failed to cache build stats",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
		}
	}
	if ciMeta.Provider != "" || ciMeta.BuildURL != "" || ciMeta.Branch != "" || ciMeta.CommitSHA != "" {
		if err := a.buildStore.UpdateBuildCIMetadata(ctx, projectID, buildOrder, ciMeta); err != nil {
			a.logger.Warn("failed to store CI metadata",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
		}
	}
	if err := a.buildStore.SetLatest(ctx, projectID, buildOrder); err != nil {
		a.logger.Error("failed to set latest build",
			zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
	}
	return nil
}

// recordBuild records the build in the database for pruning without publishing
// a report snapshot. Used when storeResults=false but KeepHistory=true.
func (a *Allure) recordBuild(ctx context.Context, projectID string, buildOrder int) error {
	if err := a.buildStore.InsertBuild(ctx, projectID, buildOrder); err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	if err := a.buildStore.SetLatest(ctx, projectID, buildOrder); err != nil {
		a.logger.Error("failed to set latest build (recordBuild)",
			zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
	}
	return nil
}

// GenerateReport implements generateAllureReport.sh
func (a *Allure) GenerateReport(ctx context.Context, projectID, execName, execFrom, execType string, storeResults bool, ciBranch, ciCommitSHA string) (string, error) {
	if execName == "" {
		execName = "Automatic Execution"
	}
	if execType == "" {
		execType = "another"
	}

	// 1. Acquire per-project lock to serialize concurrent report generation.
	unlock, err := a.lockManager.Acquire(ctx, projectID, "generate")
	if err != nil {
		return "", fmt.Errorf("acquire generation lock: %w", err)
	}
	defer unlock()

	// 2. Get next build order atomically from the database.
	buildOrder, err := a.buildStore.NextBuildOrder(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("next build order: %w", err)
	}

	// 3. PrepareLocal returns the project dir (local) or a temp dir (S3).
	localProjectDir, err := a.store.PrepareLocal(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("prepare local dir for %q: %w", projectID, err)
	}
	defer a.store.CleanupLocal(localProjectDir) //nolint:errcheck // cleanup errors are non-fatal

	resultsDir := filepath.Join(localProjectDir, "results")

	// 4. Write executor.json directly — always local (temp dir in S3 mode)
	if err := writeExecutorJSON(resultsDir, projectID, execName, execFrom, execType, buildOrder, storeResults); err != nil {
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
	if err := a.runAllureGenerate(ctx, projectID, latestReportDir, localProjectDir); err != nil {
		return "", err
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
			if err := a.storeAndPruneBuild(ctx, projectID, localProjectDir, buildOrder, ciMeta); err != nil {
				return "", err
			}
		} else {
			if err := a.recordBuild(ctx, projectID, buildOrder); err != nil {
				return "", err
			}
		}
	}

	// 8. Keep Latest History (Cleanup old reports)
	if err := a.KeepLatestHistory(ctx, projectID); err != nil {
		return "", err
	}

	return "Report successfully generated", nil
}

// CleanHistory delegates to the store module and regenerates
func (a *Allure) CleanHistory(ctx context.Context, projectID string) error {
	if err := a.store.CleanHistory(ctx, projectID); err != nil {
		return fmt.Errorf("clean history for %q: %w", projectID, err)
	}

	// Clean build and test-result records from the database.
	if err := a.buildStore.DeleteAllBuilds(ctx, projectID); err != nil {
		return fmt.Errorf("clean history db builds for %q: %w", projectID, err)
	}
	if a.testResultStore != nil {
		if err := a.testResultStore.DeleteByProject(ctx, projectID); err != nil {
			return fmt.Errorf("clean history db test results for %q: %w", projectID, err)
		}
	}

	checkSecs := strings.ToUpper(a.cfg.CheckResultsSecs)
	if checkSecs != "NONE" {
		if err := a.store.KeepHistory(ctx, projectID); err != nil {
			return fmt.Errorf("keep history for %q: %w", projectID, err)
		}

		if _, err := a.GenerateReport(ctx, projectID, "", "", "", false, "", ""); err != nil {
			return err
		}
	}

	return nil
}

// KeepHistory delegates to the store module
func (a *Allure) KeepHistory(ctx context.Context, projectID string) error {
	if err := a.store.KeepHistory(ctx, projectID); err != nil {
		return fmt.Errorf("keep history for %q: %w", projectID, err)
	}
	return nil
}

// DeleteProject removes the entire project (filesystem + S3).
func (a *Allure) DeleteProject(ctx context.Context, projectID string) error {
	if err := a.store.DeleteProject(ctx, projectID); err != nil {
		return fmt.Errorf("delete project %q: %w", projectID, err)
	}
	return nil
}

// DeleteReport removes a single numbered report directory for a project.
func (a *Allure) DeleteReport(ctx context.Context, projectID, reportID string) error {
	if err := a.store.DeleteReport(ctx, projectID, reportID); err != nil {
		return fmt.Errorf("delete report %q for %q: %w", reportID, projectID, err)
	}

	// Clean the corresponding build and test-result records from the database.
	if buildOrder, err := strconv.Atoi(reportID); err == nil {
		if dbErr := a.buildStore.DeleteBuild(ctx, projectID, buildOrder); dbErr != nil {
			a.logger.Warn("failed to delete build from db",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(dbErr))
		}
		if a.testResultStore != nil {
			if buildID, idErr := a.testResultStore.GetBuildID(ctx, projectID, buildOrder); idErr == nil {
				if dbErr := a.testResultStore.DeleteByBuild(ctx, buildID); dbErr != nil {
					a.logger.Warn("failed to delete test results from db",
						zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(dbErr))
				}
			}
		}
	}

	return nil
}

// CleanResults delegates to the store module
func (a *Allure) CleanResults(ctx context.Context, projectID string) error {
	if err := a.store.CleanResults(ctx, projectID); err != nil {
		return fmt.Errorf("clean results for %q: %w", projectID, err)
	}
	return nil
}

// CreateProject creates the necessary directories for a new project
func (a *Allure) CreateProject(ctx context.Context, projectID string) error {
	projectDir := filepath.Join(a.cfg.ProjectsDirectory, projectID)

	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("%w: %s", ErrProjectExists, projectID)
	}

	if err := a.store.CreateProject(ctx, projectID); err != nil {
		return fmt.Errorf("create project %q: %w", projectID, err)
	}
	return nil
}

// StoreReport copies variable-content subdirs of the latest report to a numbered snapshot.
// This thin wrapper exists for backward compatibility with tests; new code should call
// store.PublishReport directly with the localProjectDir from PrepareLocal.
func (a *Allure) StoreReport(ctx context.Context, projectID string, buildOrder int) error {
	localProjectDir := filepath.Join(a.cfg.ProjectsDirectory, projectID)
	if err := a.store.PublishReport(ctx, projectID, buildOrder, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	return nil
}

// KeepLatestHistory removes the oldest historical report directories when count exceeds keepLatest.
// Uses the database to determine which builds to prune, then removes their filesystem directories.
func (a *Allure) KeepLatestHistory(ctx context.Context, projectID string) error {
	if !a.cfg.KeepHistory {
		return nil
	}
	removed, err := a.buildStore.PruneBuilds(ctx, projectID, a.cfg.KeepHistoryLatest)
	if err != nil {
		return fmt.Errorf("prune builds from db: %w", err)
	}

	if err := a.store.PruneReportDirs(ctx, projectID, removed); err != nil {
		return fmt.Errorf("prune report dirs: %w", err)
	}

	return nil
}

// runAllureGenerate preserves history trends, clears the stale latest report
// directory, and runs `allure generate` to produce a fresh report.
func (a *Allure) runAllureGenerate(ctx context.Context, projectID, latestReportDir, localProjectDir string) error {
	if err := a.store.KeepHistory(ctx, projectID); err != nil {
		a.logger.Error("KeepHistory failed", zap.String("project_id", projectID), zap.Error(err))
	}
	if err := os.RemoveAll(latestReportDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing latest report dir: %w", err)
	}
	return a.runAllureCmd(ctx, "generate", "--output", latestReportDir, "--cwd", localProjectDir, "results")
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
