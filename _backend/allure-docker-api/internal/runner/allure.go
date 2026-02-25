package runner

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/storage"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

//go:embed templates/emailable.gohtml
var emailableTemplateContent string

// ErrProjectExists is returned when creating a project that already exists
var ErrProjectExists = errors.New("project already exists")

// Sentinel errors for allure runner operations.
var (
	ErrStatsNotFound   = errors.New("build stats not found")
	ErrAllureCmdFailed = errors.New("allure command failed")
)

const (
	statusPassed  = "passed"
	statusFailed  = "failed"
	statusBroken  = "broken"
	statusSkipped = "skipped"
)

// Allure represents the core Allure report generation process
type Allure struct {
	cfg         *config.Config
	store       storage.Store
	buildStore  *store.BuildStore
	lockManager *store.LockManager
}

// NewAllure creates a new Allure runner
func NewAllure(cfg *config.Config, dataStore storage.Store, buildStore *store.BuildStore, lockManager *store.LockManager) *Allure {
	return &Allure{
		cfg:         cfg,
		store:       dataStore,
		buildStore:  buildStore,
		lockManager: lockManager,
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

// --- Emailable report types ---

// TestCaseTime holds timing information for a test case or step
type TestCaseTime struct {
	Duration int64 `json:"duration"`
}

// TestCaseExtra holds extra metadata attached to a test case
type TestCaseExtra struct {
	Severity string `json:"severity,omitempty"`
}

// TestCaseLabel represents a label attached to a test case
type TestCaseLabel struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TestCaseStep represents a single step within a stage
type TestCaseStep struct {
	Name          string        `json:"name"`
	Status        string        `json:"status"`
	StatusMessage string        `json:"statusMessage,omitempty"`
	StatusTrace   string        `json:"statusTrace,omitempty"`
	Time          *TestCaseTime `json:"time,omitempty"`
}

// TestCaseStage holds before/test/after stage data
type TestCaseStage struct {
	Status        string         `json:"status,omitempty"`
	StatusMessage string         `json:"statusMessage,omitempty"`
	StatusTrace   string         `json:"statusTrace,omitempty"`
	StepsCount    int            `json:"stepsCount,omitempty"`
	Steps         []TestCaseStep `json:"steps,omitempty"`
}

// TestCase represents one Allure test case parsed from the report
type TestCase struct {
	Name         string          `json:"name"`
	Status       string          `json:"status"`
	Description  string          `json:"description,omitempty"`
	Time         *TestCaseTime   `json:"time,omitempty"`
	Extra        *TestCaseExtra  `json:"extra,omitempty"`
	Labels       []TestCaseLabel `json:"labels,omitempty"`
	TestStage    *TestCaseStage  `json:"testStage,omitempty"`
	BeforeStages []TestCaseStage `json:"beforeStages,omitempty"`
	AfterStages  []TestCaseStage `json:"afterStages,omitempty"`
}

// testStats holds pre-computed summary counts and percentages
type testStats struct {
	Total      int
	Passed     int
	Failed     int
	Broken     int
	Skipped    int
	Unknown    int
	PassedPct  float64
	FailedPct  float64
	BrokenPct  float64
	SkippedPct float64
	UnknownPct float64
}

// emailableData is the data passed to the emailable report template
type emailableData struct {
	Title     string
	ProjectID string
	ServerURL string
	Stats     testStats
	TestCases []TestCase
}

// statusBorderClasses maps a test status to its Bootstrap border CSS class.
//
//nolint:gochecknoglobals // read-only lookup table, initialized once at package level
var statusBorderClasses = map[string]string{
	statusPassed:  "border-success",
	statusFailed:  "border-danger",
	statusBroken:  "border-warning",
	statusSkipped: "border-light",
}

// statusBadgeClasses maps a test status to its Bootstrap badge CSS class.
//
//nolint:gochecknoglobals // read-only lookup table, initialized once at package level
var statusBadgeClasses = map[string]string{
	statusPassed:  "badge-success",
	statusFailed:  "badge-danger",
	statusBroken:  "badge-warning",
	statusSkipped: "badge-light",
}

// statusTableClasses maps a test status to its Bootstrap table-row CSS class.
//
//nolint:gochecknoglobals // read-only lookup table, initialized once at package level
var statusTableClasses = map[string]string{
	statusPassed:  "table-success",
	statusFailed:  "table-danger",
	statusBroken:  "table-warning",
	statusSkipped: "table-light",
}

// relevantLabels is the set of Allure label names shown in the emailable report.
//
//nolint:gochecknoglobals // read-only lookup table, initialized once at package level
var relevantLabels = map[string]bool{
	"feature": true, "tag": true, "package": true,
	"suite": true, "testClass": true, "testMethod": true,
}

// emailableTemplateFuncs defines the template helper functions
//
//nolint:gochecknoglobals // read-only template function map, initialized once at package level
var emailableTemplateFuncs = template.FuncMap{
	"statusBorderClass": func(status string) string {
		if cls, ok := statusBorderClasses[status]; ok {
			return cls
		}
		return "border-dark"
	},
	"statusBadgeClass": func(status string) string {
		if cls, ok := statusBadgeClasses[status]; ok {
			return cls
		}
		return "badge-dark"
	},
	"statusTableClass": func(status string) string {
		if cls, ok := statusTableClasses[status]; ok {
			return cls
		}
		return "table-dark"
	},
	"isRelevantLabel": func(name string) bool {
		return relevantLabels[name]
	},
	"formatDurationMs": func(ms int64) string {
		return fmt.Sprintf("%.3f", float64(ms)/1000.0)
	},
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

// storeAndPruneBuild stores a report snapshot and records it in the database.
func (a *Allure) storeAndPruneBuild(ctx context.Context, projectID, localProjectDir string, buildOrder int) error {
	if err := a.store.PublishReport(ctx, projectID, buildOrder, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	if err := a.buildStore.InsertBuild(ctx, projectID, buildOrder); err != nil {
		log.Printf("GenerateReport: failed to record build %d for '%s': %v", buildOrder, projectID, err)
		return nil
	}
	if stats, err := a.store.ReadBuildStats(ctx, projectID, buildOrder); err == nil {
		// Convert storage.BuildStats → store.BuildStats for the DB layer
		storeStats := store.BuildStats{
			Passed:     stats.Passed,
			Failed:     stats.Failed,
			Broken:     stats.Broken,
			Skipped:    stats.Skipped,
			Unknown:    stats.Unknown,
			Total:      stats.Total,
			DurationMs: stats.DurationMs,
		}
		if err := a.buildStore.UpdateBuildStats(ctx, projectID, buildOrder, storeStats); err != nil {
			log.Printf("GenerateReport: failed to cache stats for '%s' build %d: %v", projectID, buildOrder, err)
		}
	}
	if err := a.buildStore.SetLatest(ctx, projectID, buildOrder); err != nil {
		log.Printf("GenerateReport: failed to set latest for '%s' build %d: %v", projectID, buildOrder, err)
	}
	return nil
}

// GenerateReport implements generateAllureReport.sh
func (a *Allure) GenerateReport(projectID, execName, execFrom, execType string, storeResults bool) (string, error) {
	if execName == "" {
		execName = "Automatic Execution"
	}
	if execType == "" {
		execType = "another"
	}

	ctx := context.Background()

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
	plugin := "awesome"
	if a.cfg.LegacyUI {
		plugin = "classic"
	}
	configData := map[string]any{
		"plugins": []string{plugin},
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
	if a.cfg.KeepHistory && storeResults {
		if err := a.storeAndPruneBuild(ctx, projectID, localProjectDir, buildOrder); err != nil {
			return "", err
		}
	}

	// 8. Keep Latest History (Cleanup old reports)
	if err := a.KeepLatestHistory(projectID); err != nil {
		return "", err
	}

	return "Report successfully generated", nil
}

// CleanHistory delegates to the store module and regenerates
func (a *Allure) CleanHistory(projectID string) error {
	ctx := context.Background()
	if err := a.store.CleanHistory(ctx, projectID); err != nil {
		return fmt.Errorf("clean history for %q: %w", projectID, err)
	}

	checkSecs := strings.ToUpper(a.cfg.CheckResultsSecs)
	if checkSecs != "NONE" {
		if err := a.store.KeepHistory(ctx, projectID); err != nil {
			return fmt.Errorf("keep history for %q: %w", projectID, err)
		}

		if _, err := a.GenerateReport(projectID, "", "", "", false); err != nil {
			return err
		}

		if _, err := a.RenderEmailableReport(projectID); err != nil {
			log.Printf("CleanHistory: emailable report render failed for '%s': %v", projectID, err)
		}
	}

	return nil
}

// KeepHistory delegates to the store module
func (a *Allure) KeepHistory(projectID string) error {
	if err := a.store.KeepHistory(context.Background(), projectID); err != nil {
		return fmt.Errorf("keep history for %q: %w", projectID, err)
	}
	return nil
}

// DeleteProject removes the entire project (filesystem + S3).
func (a *Allure) DeleteProject(projectID string) error {
	if err := a.store.DeleteProject(context.Background(), projectID); err != nil {
		return fmt.Errorf("delete project %q: %w", projectID, err)
	}
	return nil
}

// DeleteReport removes a single numbered report directory for a project.
func (a *Allure) DeleteReport(projectID, reportID string) error {
	if err := a.store.DeleteReport(context.Background(), projectID, reportID); err != nil {
		return fmt.Errorf("delete report %q for %q: %w", reportID, projectID, err)
	}
	return nil
}

// loadTestCases reads all JSON test-case files from the relative path within the project.
func (a *Allure) loadTestCases(ctx context.Context, projectID, relPath string) ([]TestCase, error) {
	entries, err := a.store.ReadDir(ctx, projectID, relPath)
	if err != nil {
		return nil, fmt.Errorf("reading test-cases dir: %w", err)
	}

	var testCases []TestCase
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		filePath := relPath + "/" + e.Name
		raw, err := a.store.ReadFile(ctx, projectID, filePath)
		if err != nil {
			log.Printf("RenderEmailableReport: skipping %s: %v", e.Name, err)
			continue
		}
		var tc TestCase
		if err := json.Unmarshal(raw, &tc); err != nil {
			log.Printf("RenderEmailableReport: invalid JSON in %s: %v", e.Name, err)
			continue
		}
		testCases = append(testCases, tc)
	}

	return testCases, nil
}

// renderEmailableToDir parses the embedded template, executes it with data,
// and writes the result to outputDir/index.html. Returns the rendered bytes.
func renderEmailableToDir(outputDir string, data *emailableData) ([]byte, error) {
	tmpl, err := template.New("emailable").Funcs(emailableTemplateFuncs).Parse(emailableTemplateContent)
	if err != nil {
		return nil, fmt.Errorf("parsing emailable template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing emailable template: %w", err)
	}

	//nolint:gosec // G301: 0o755 required for allure web server
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating emailable report dir: %w", err)
	}
	//nolint:gosec // G306: 0o644 required for web server to serve emailable report
	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), buf.Bytes(), 0o644); err != nil {
		return nil, fmt.Errorf("writing emailable report: %w", err)
	}

	return buf.Bytes(), nil
}

// RenderEmailableReport reads test cases from the latest report, renders the
// emailable HTML report from the embedded template, saves it to
// reports/emailable-report-render/index.html, and returns the rendered bytes.
func (a *Allure) RenderEmailableReport(projectID string) ([]byte, error) {
	ctx := context.Background()
	relPath := "reports/latest/data/test-cases"

	testCases, err := a.loadTestCases(ctx, projectID, relPath)
	if err != nil {
		return nil, err
	}

	// Pre-compute statistics (Jinja2 namespace vars handled in Go)
	stats := computeTestStats(testCases)

	data := emailableData{
		Title:     projectID,
		ProjectID: projectID,
		Stats:     stats,
		TestCases: testCases,
	}

	// Output dir is always local — emailable report is served from local filesystem
	outputDir := filepath.Join(a.cfg.ProjectsDirectory, projectID, "reports", "emailable-report-render")
	return renderEmailableToDir(outputDir, &data)
}

// computeTestStats counts test results by status and computes percentages.
func computeTestStats(testCases []TestCase) testStats {
	stats := testStats{Total: len(testCases)}
	for i := range testCases {
		switch testCases[i].Status {
		case statusPassed:
			stats.Passed++
		case statusFailed:
			stats.Failed++
		case statusBroken:
			stats.Broken++
		case statusSkipped:
			stats.Skipped++
		default:
			stats.Unknown++
		}
	}
	if stats.Total > 0 {
		f := float64(stats.Total)
		stats.PassedPct = float64(stats.Passed) * 100 / f
		stats.FailedPct = float64(stats.Failed) * 100 / f
		stats.BrokenPct = float64(stats.Broken) * 100 / f
		stats.SkippedPct = float64(stats.Skipped) * 100 / f
		stats.UnknownPct = float64(stats.Unknown) * 100 / f
	}
	return stats
}

// CleanResults delegates to the store module
func (a *Allure) CleanResults(projectID string) error {
	if err := a.store.CleanResults(context.Background(), projectID); err != nil {
		return fmt.Errorf("clean results for %q: %w", projectID, err)
	}
	return nil
}

// CreateProject creates the necessary directories for a new project
func (a *Allure) CreateProject(projectID string) error {
	projectDir := filepath.Join(a.cfg.ProjectsDirectory, projectID)

	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("%w: %s", ErrProjectExists, projectID)
	}

	if err := a.store.CreateProject(context.Background(), projectID); err != nil {
		return fmt.Errorf("create project %q: %w", projectID, err)
	}
	return nil
}

// StoreReport copies variable-content subdirs of the latest report to a numbered snapshot.
// This thin wrapper exists for backward compatibility with tests; new code should call
// store.PublishReport directly with the localProjectDir from PrepareLocal.
func (a *Allure) StoreReport(projectID string, buildOrder int) error {
	ctx := context.Background()
	localProjectDir := filepath.Join(a.cfg.ProjectsDirectory, projectID)
	if err := a.store.PublishReport(ctx, projectID, buildOrder, localProjectDir); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	return nil
}

// KeepLatestHistory removes the oldest historical report directories when count exceeds keepLatest.
// Uses the database to determine which builds to prune, then removes their filesystem directories.
func (a *Allure) KeepLatestHistory(projectID string) error {
	if !a.cfg.KeepHistory {
		return nil
	}

	ctx := context.Background()
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
		log.Printf("GenerateReport: KeepHistory failed for '%s': %v", projectID, err)
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
	if a.cfg.DevMode && outBuff.Len() > 0 {
		log.Printf("allure output: %s", outBuff.String())
	}
	return nil
}
