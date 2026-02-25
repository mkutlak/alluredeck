package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/runner"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/storage"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

// Sentinel errors for HTTP request validation.
var (
	ErrProjectRequired       = errors.New("project_id is required")
	ErrProjectTooLong        = errors.New("project_id must not exceed 100 characters")
	ErrProjectInvalidChars   = errors.New("project_id contains invalid characters")
	ErrProjectReserved       = errors.New("project_id is reserved")
	ErrProjectInvalid        = errors.New("invalid project_id")
	ErrResultsRequired       = errors.New("'results' array is required in the body")
	ErrResultsEmpty          = errors.New("'results' array is empty")
	ErrFileNameRequired      = errors.New("'file_name' attribute is required for all results")
	ErrDuplicateFileNames    = errors.New("duplicated file names in 'results'")
	ErrContentBase64Required = errors.New("'content_base64' attribute is required")
	ErrNoFilesProvided       = errors.New("no files provided in 'files[]' field")

	// errUnsupportedContentType is returned by parseResultsBody when the
	// Content-Type header is neither application/json nor multipart/form-data.
	errUnsupportedContentType = errors.New("unsupported Content-Type")
)

// AllureHandler handles HTTP requests for Allure report management.
type AllureHandler struct {
	cfg          *config.Config
	runner       *runner.Allure
	projectStore *store.ProjectStore
	buildStore   *store.BuildStore
	store        storage.Store
}

// NewAllureHandler creates and returns a new AllureHandler.
func NewAllureHandler(cfg *config.Config, r *runner.Allure, projectStore *store.ProjectStore, buildStore *store.BuildStore, st storage.Store) *AllureHandler {
	return &AllureHandler{
		cfg:          cfg,
		runner:       r,
		projectStore: projectStore,
		buildStore:   buildStore,
		store:        st,
	}
}

// ProjectsResponse holds the response structure for basic project listing.
type ProjectsResponse struct {
	Data     map[string]map[string]string `json:"data"`
	MetaData map[string]string            `json:"meta_data"`
}

// reservedProjectNames lists names that clash with API route segments
//
//nolint:gochecknoglobals // read-only constant-like lookup table for reserved project names
var reservedProjectNames = map[string]bool{
	"login":           true,
	"logout":          true,
	"version":         true,
	"config":          true,
	"swagger":         true,
	"projects":        true,
	"generate-report": true,
	"clean-history":   true,
	"clean-results":   true,
	"send-results":    true,
	".":               true,
	"..":              true,
}

// validateProjectID rejects project IDs that could cause path traversal or
// shadow API routes. Returns an error message suitable for the API response.
func validateProjectID(projectsDir, projectID string) error {
	if projectID == "" {
		return ErrProjectRequired
	}
	if len(projectID) > 100 {
		return ErrProjectTooLong
	}
	if strings.ContainsAny(projectID, "/\\") || strings.Contains(projectID, "..") {
		return ErrProjectInvalidChars
	}
	if reservedProjectNames[projectID] {
		return fmt.Errorf("project_id %q: %w", projectID, ErrProjectReserved)
	}
	// Belt-and-suspenders: verify the resolved path stays under projectsDir
	resolved := filepath.Join(projectsDir, projectID)
	rel, err := filepath.Rel(projectsDir, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ErrProjectInvalid
	}
	return nil
}

// safeProjectID resolves to "default" when empty, then validates
func safeProjectID(projectsDir, raw string) (string, error) {
	if raw == "" {
		raw = "default"
	}
	if err := validateProjectID(projectsDir, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// GetProjects returns a list of all existing projects.
func (h *AllureHandler) GetProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dbProjects, err := h.projectStore.ListProjects(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error listing projects: %v", err)},
		})
		return
	}

	projects := make(map[string]map[string]string)
	for _, p := range dbProjects {
		projects[p.ID] = map[string]string{
			"project_id": p.ID,
			"created_at": p.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	_ = json.NewEncoder(w).Encode(ProjectsResponse{
		Data:     projects,
		MetaData: map[string]string{"message": "Projects successfully obtained"},
	})
}

// CreateProject creates a new project directory.
func (h *AllureHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var reqBody struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": "Invalid JSON payload"},
		})
		return
	}

	projectID := strings.TrimSpace(reqBody.ID)
	if err := validateProjectID(h.cfg.ProjectsDirectory, projectID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	err := h.runner.CreateProject(projectID)
	if err != nil {
		if errors.Is(err, runner.ErrProjectExists) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta_data": map[string]string{"message": err.Error()},
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error creating project: %v", err)},
		})
		return
	}

	// Register in database (INSERT OR IGNORE so an already-synced project is not an error).
	if dbErr := h.projectStore.CreateProject(r.Context(), projectID); dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectExists) {
			// Log but don't fail — filesystem project was already created successfully.
			_ = dbErr
		}
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]map[string]string{
			projectID: {"project_id": projectID},
		},
		"meta_data": map[string]string{"message": "Project successfully created"},
	})
}

// GenerateReport triggers Allure report generation for a project.
func (h *AllureHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	execName := r.URL.Query().Get("execution_name")
	execFrom := r.URL.Query().Get("execution_from")
	execType := r.URL.Query().Get("execution_type")
	storeResultsStr := r.URL.Query().Get("store_results")
	var storeResults bool
	if storeResultsStr == "" {
		// When store_results is not specified, default to the server's KEEP_HISTORY setting.
		storeResults = h.cfg.KeepHistory
	} else {
		storeResults = storeResultsStr == "1" || strings.EqualFold(storeResultsStr, "true")
	}

	out, err := h.runner.GenerateReport(projectID, execName, execFrom, execType, storeResults)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error generating report: %v", err)},
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]string{
			"project_id": projectID,
			"output":     out,
		},
		"meta_data": map[string]string{"message": "Report successfully generated"},
	})
}

// CleanHistory removes all historical report snapshots for a project.
func (h *AllureHandler) CleanHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	if err := h.runner.CleanHistory(projectID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error cleaning history: %v", err)},
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":      map[string]string{"output": ""},
		"meta_data": map[string]string{"message": "History successfully cleaned"},
	})
}

// CleanResults removes all test result files for a project.
func (h *AllureHandler) CleanResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	if err := h.runner.CleanResults(projectID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error cleaning results: %v", err)},
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":      map[string]string{"output": ""},
		"meta_data": map[string]string{"message": "Results successfully cleaned"},
	})
}

// GetEmailableReport renders the emailable HTML report for a project and returns it.
// The rendered report is also saved to reports/emailable-report-render/index.html.
func (h *AllureHandler) GetEmailableReport(w http.ResponseWriter, r *http.Request) {
	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	htmlBytes, err := h.runner.RenderEmailableReport(projectID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error rendering emailable report: %v", err)},
		})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(htmlBytes) //nolint:gosec // G705: htmlBytes is rendered from a trusted template, not from user input
}

// SendResults handles uploading allure result files to a project.
//
// Supports two Content-Type modes:
//   - application/json:       body {"results":[{"file_name":"...","content_base64":"..."}]}
//   - multipart/form-data:    field "files[]" containing the files directly
//
// An optional query param force_project_creation=true auto-creates the project.
func (h *AllureHandler) SendResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	// Ensure project exists (auto-create if requested)
	exists, err := h.store.ProjectExists(r.Context(), projectID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Failed to check project: %v", err)},
		})
		return
	}
	if !exists {
		if r.URL.Query().Get("force_project_creation") == "true" {
			if err := h.runner.CreateProject(projectID); err != nil && !errors.Is(err, runner.ErrProjectExists) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": fmt.Sprintf("Failed to create project: %v", err)},
				})
				return
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta_data": map[string]string{"message": fmt.Sprintf("project_id '%s' not found", projectID)},
			})
			return
		}
	}

	// Limit request body to 100 MB to prevent memory exhaustion (AUDIT 2.2).
	const maxBodyBytes = 100 << 20 // 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	processedFiles, failedFiles, err := h.parseResultsBody(r, projectID)
	if errors.Is(err, errUnsupportedContentType) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{
				"message": "Content-Type must be application/json or multipart/form-data",
			},
		})
		return
	}

	if err != nil {
		code := http.StatusBadRequest
		msg := err.Error()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			code = http.StatusRequestEntityTooLarge
			msg = "request body too large"
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": msg},
		})
		return
	}

	if len(failedFiles) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Problems with files: %v", failedFiles)},
		})
		return
	}

	if h.cfg.APIRespLessVerbose {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Results successfully sent for project_id '%s'", projectID)},
		})
		return
	}

	currentFileNames, _ := h.store.ListResultFiles(r.Context(), projectID)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"current_files":         currentFileNames,
			"current_files_count":   len(currentFileNames),
			"failed_files":          failedFiles,
			"failed_files_count":    len(failedFiles),
			"processed_files":       processedFiles,
			"processed_files_count": len(processedFiles),
			"sent_files_count":      len(processedFiles) + len(failedFiles),
		},
		"meta_data": map[string]string{"message": fmt.Sprintf("Results successfully sent for project_id '%s'", projectID)},
	})
}

// parseResultsBody routes the request to the appropriate parser based on Content-Type.
// Returns errUnsupportedContentType when the Content-Type is not recognized.
func (h *AllureHandler) parseResultsBody(r *http.Request, projectID string) (processed []string, failed []map[string]string, err error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		return h.sendJSONResults(r, projectID)
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return h.sendMultipartResults(r, projectID)
	default:
		return nil, nil, errUnsupportedContentType
	}
}

// sendJSONResults processes JSON body: {"results":[{"file_name":"...","content_base64":"..."}]}
func (h *AllureHandler) sendJSONResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
	var body struct {
		Results []struct {
			FileName      string `json:"file_name"`
			ContentBase64 string `json:"content_base64"`
		} `json:"results"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if body.Results == nil {
		return nil, nil, ErrResultsRequired
	}
	if len(body.Results) == 0 {
		return nil, nil, ErrResultsEmpty
	}

	// Check for duplicate file names
	seen := make(map[string]bool, len(body.Results))
	for _, res := range body.Results {
		if res.FileName == "" {
			return nil, nil, ErrFileNameRequired
		}
		if seen[res.FileName] {
			return nil, nil, ErrDuplicateFileNames
		}
		seen[res.FileName] = true
	}

	for i := range body.Results {
		safeName := secureFilename(body.Results[i].FileName)
		if body.Results[i].ContentBase64 == "" {
			return nil, nil, fmt.Errorf("'content_base64' required for %q: %w", body.Results[i].FileName, ErrContentBase64Required)
		}

		// Stream base64 decode directly to disk via io.Copy to avoid holding a
		// full decoded []byte in memory alongside the original base64 string.
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(body.Results[i].ContentBase64))
		// Release the base64 string now that the decoder holds its own reference,
		// allowing the GC to reclaim it while io.Copy runs.
		body.Results[i].ContentBase64 = ""

		if err := h.store.WriteResultFile(r.Context(), projectID, safeName, decoder); err != nil {
			return nil, nil, fmt.Errorf("decode base64 for %q: %w", body.Results[i].FileName, err)
		}

		processed = append(processed, safeName)
	}

	return processed, failed, nil
}

// sendMultipartResults processes multipart/form-data with files[] field
func (h *AllureHandler) sendMultipartResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
	const maxMemory = 32 << 20 // 32 MB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return nil, nil, fmt.Errorf("parse multipart form: %w", err)
	}

	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		return nil, nil, ErrNoFilesProvided
	}

	for _, fh := range files {
		safeName := secureFilename(fh.Filename)
		f, err := fh.Open()
		if err != nil {
			failed = append(failed, map[string]string{"file_name": safeName, "message": err.Error()})
			continue
		}

		err = h.store.WriteResultFile(r.Context(), projectID, safeName, f)
		_ = f.Close()
		if err != nil {
			failed = append(failed, map[string]string{"file_name": safeName, "message": err.Error()})
		} else {
			processed = append(processed, safeName)
		}
	}

	return processed, failed, nil
}

// secureFilename strips path components so only the base filename remains
func secureFilename(name string) string {
	return filepath.Base(filepath.Clean(name))
}

// ReportHistoryEntry holds metadata for a single generated report.
type ReportHistoryEntry struct {
	ReportID    string           `json:"report_id"`
	IsLatest    bool             `json:"is_latest"`
	GeneratedAt *string          `json:"generated_at"`
	DurationMs  *int64           `json:"duration_ms"`
	Statistic   *AllureStatistic `json:"statistic"`
}

// AllureStatistic mirrors the statistic block in Allure's widgets/summary.json.
type AllureStatistic struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Skipped int `json:"skipped"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// allureSummaryFile is the shape of widgets/summary.json we care about.
type allureSummaryFile struct {
	Statistic *AllureStatistic `json:"statistic"`
	Time      *struct {
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	} `json:"time"`
}

// GetReportHistory returns metadata for all generated reports of a project.
// Numbered reports are served from the database (cached stats, no filesystem scan).
// The "latest" entry is still read from the filesystem since it is always regenerated.
func (h *AllureHandler) GetReportHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	ctx := context.Background()

	// Fetch numbered builds from DB (sorted descending by build_order).
	builds, err := h.buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error reading report history: %v", err)},
		})
		return
	}

	// Initialize as empty slice so JSON encodes [] instead of null when there are no reports.
	reports := make([]ReportHistoryEntry, 0)

	// Check for "latest" dir via store — always regenerated, not tracked in DB.
	if exists, _ := h.store.LatestReportExists(r.Context(), projectID); exists {
		entry := h.buildReportEntry(r.Context(), projectID, "latest")
		entry.IsLatest = true
		reports = append(reports, entry)
	}

	// Convert DB builds to response entries.
	for i := range builds {
		reports = append(reports, buildEntryFromDB(&builds[i]))
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"project_id": projectID,
			"reports":    reports,
		},
		"meta_data": map[string]string{"message": "Report history successfully obtained"},
	})
}

// buildEntryFromDB converts a store.Build to a ReportHistoryEntry without filesystem I/O.
func buildEntryFromDB(b *store.Build) ReportHistoryEntry {
	reportID := strconv.Itoa(b.BuildOrder)
	entry := ReportHistoryEntry{
		ReportID: reportID,
		IsLatest: b.IsLatest,
	}
	t := b.CreatedAt.UTC().Format(time.RFC3339)
	entry.GeneratedAt = &t
	entry.DurationMs = b.DurationMs

	if b.StatTotal != nil && *b.StatTotal > 0 {
		entry.Statistic = &AllureStatistic{
			Passed:  derefInt(b.StatPassed),
			Failed:  derefInt(b.StatFailed),
			Broken:  derefInt(b.StatBroken),
			Skipped: derefInt(b.StatSkipped),
			Unknown: derefInt(b.StatUnknown),
			Total:   *b.StatTotal,
		}
	}
	return entry
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// readJSONViaStore reads a project-relative path via the store and unmarshals JSON into v.
// Returns true only when both the read and unmarshal succeed.
func (h *AllureHandler) readJSONViaStore(ctx context.Context, projectID, relPath string, v any) bool {
	data, err := h.store.ReadFile(ctx, projectID, relPath)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// applySummaryFile reads widgets/summary.json (Allure 2) and fills entry in-place.
// Returns true when the file was read and applied successfully.
func (h *AllureHandler) applySummaryFile(ctx context.Context, projectID string, entry *ReportHistoryEntry, widgetsRelPath string) bool {
	var s allureSummaryFile
	if !h.readJSONViaStore(ctx, projectID, widgetsRelPath+"/summary.json", &s) {
		return false
	}
	entry.Statistic = s.Statistic
	if s.Time != nil {
		if s.Time.Stop != 0 {
			t := time.Unix(0, s.Time.Stop*int64(time.Millisecond)).UTC().Format(time.RFC3339)
			entry.GeneratedAt = &t
		}
		if s.Time.Duration != 0 {
			entry.DurationMs = &s.Time.Duration
		}
	}
	return true
}

// buildReportEntry reads report metadata from widget files via the store.
// Tries widgets/summary.json first (Allure 2), then widgets/statistic.json (Allure 3).
func (h *AllureHandler) buildReportEntry(ctx context.Context, projectID, name string) ReportHistoryEntry {
	entry := ReportHistoryEntry{ReportID: name}
	widgetsRelPath := "reports/" + name + "/widgets"

	// Allure 2: widgets/summary.json contains statistic + time nested under keys.
	if h.applySummaryFile(ctx, projectID, &entry, widgetsRelPath) {
		return entry
	}

	// Allure 3: widgets/statistic.json has statistic fields at root level.
	var stat AllureStatistic
	if h.readJSONViaStore(ctx, projectID, widgetsRelPath+"/statistic.json", &stat) && stat.Total > 0 {
		entry.Statistic = &stat
	}
	return entry
}

// DeleteProject removes an entire project and all its reports.
func (h *AllureHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	raw := r.PathValue("project_id")
	projectID, err := url.PathUnescape(raw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": "invalid project_id encoding"},
		})
		return
	}

	if err := validateProjectID(h.cfg.ProjectsDirectory, projectID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	if err := h.runner.DeleteProject(projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta_data": map[string]string{"message": fmt.Sprintf("project_id %q not found", projectID)},
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error deleting project: %v", err)},
		})
		return
	}

	// Remove from database. Non-fatal: project may not be in DB (pre-SQLite projects).
	if dbErr := h.projectStore.DeleteProject(r.Context(), projectID); dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectNotFound) {
			_ = dbErr // log-only; filesystem delete already succeeded
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":      map[string]string{"project_id": projectID},
		"meta_data": map[string]string{"message": "Project successfully deleted"},
	})
}

// DeleteReport removes a single numbered report directory for a project.
func (h *AllureHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	projectID, err := safeProjectID(h.cfg.ProjectsDirectory, r.URL.Query().Get("project_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": err.Error()},
		})
		return
	}

	reportID := r.URL.Query().Get("report_id")
	if reportID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": "report_id query parameter is required"},
		})
		return
	}

	if err := h.runner.DeleteReport(projectID, reportID); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_data": map[string]string{"message": fmt.Sprintf("Error deleting report: %v", err)},
		})
		return
	}

	// Remove build record from database. Non-fatal if not found (pre-SQLite report).
	if buildOrder, err := strconv.Atoi(reportID); err == nil {
		_ = h.buildStore.DeleteBuild(r.Context(), projectID, buildOrder)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":      map[string]string{"report_id": reportID, "project_id": projectID},
		"meta_data": map[string]string{"message": fmt.Sprintf("Report %q successfully deleted", reportID)},
	})
}
