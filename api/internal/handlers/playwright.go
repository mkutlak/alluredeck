package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PlaywrightHandler handles Playwright HTML report uploads.
type PlaywrightHandler struct {
	store        storage.Store
	projectStore store.ProjectStorer
	buildStore   store.BuildStorer
	jobManager   runner.JobQueuer
	cfg          *config.Config
	logger       *zap.Logger
}

// NewPlaywrightHandler creates and returns a new PlaywrightHandler.
func NewPlaywrightHandler(st storage.Store, ps store.ProjectStorer, bs store.BuildStorer, jm runner.JobQueuer, cfg *config.Config, logger *zap.Logger) *PlaywrightHandler {
	return &PlaywrightHandler{store: st, projectStore: ps, buildStore: bs, jobManager: jm, cfg: cfg, logger: logger}
}

// UploadReport godoc
// @Summary      Upload Playwright HTML report
// @Description  Uploads a Playwright HTML report as a tar.gz archive. The archive must contain index.html at its root and may include a data/ subdirectory with attachments.
// @Tags         playwright
// @Accept       application/gzip
// @Produce      json
// @Param        project_id              path   string  true   "Project ID"
// @Param        force_project_creation  query  string  false  "Auto-create project if missing"
// @Param        parent_id               query  string  false  "Parent project ID (used with force_project_creation)"
// @Param        build_number            query  int     false  "Build number to pair Playwright report with (skips latest/ staging)"
// @Param        execution_name          query  string  false  "CI provider name (e.g. GitHub Actions)"
// @Param        execution_from          query  string  false  "CI build URL"
// @Param        ci_branch               query  string  false  "Git branch name"
// @Param        ci_commit_sha           query  string  false  "Git commit SHA"
// @Success      200  {object}  map[string]any
// @Success      202  {object}  map[string]any  "Returned when standalone ingestion job is queued"
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      413  {object}  map[string]any
// @Router       /projects/{project_id}/playwright [post]
// resolveProjectID tries to parse the path value as a numeric ID.
// If it fails, treats it as a slug and looks up the project.
// Returns (id, slug, found).
func (h *PlaywrightHandler) resolveProjectID(ctx context.Context, pathValue string) (int64, string, bool) {
	if id, err := strconv.ParseInt(pathValue, 10, 64); err == nil {
		p, err := h.projectStore.GetProject(ctx, id)
		if err == nil {
			return p.ID, p.Slug, true
		}
	}
	p, err := h.projectStore.GetProjectBySlug(ctx, pathValue)
	if err == nil {
		return p.ID, p.Slug, true
	}
	return 0, pathValue, false
}

func (h *PlaywrightHandler) UploadReport(w http.ResponseWriter, r *http.Request) {
	pathValue := r.PathValue("project_id")
	projectID, slug, found := h.resolveProjectID(r.Context(), pathValue)

	// Extend HTTP deadlines — Playwright archives can contain hundreds of files,
	// each requiring an S3 round-trip during extraction.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(5 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Minute))

	parentIDStr := r.URL.Query().Get("parent_id")

	if !found {
		if r.URL.Query().Get("force_project_creation") == "true" {
			slug = pathValue
			if err := h.store.CreateProject(r.Context(), slug); err != nil {
				h.logger.Error("auto-creating project failed", zap.String("slug", slug), zap.Error(err))
				writeError(w, http.StatusInternalServerError, "failed to create project")
				return
			}
			if parentIDStr != "" {
				var parentID int64
				if id, parseErr := strconv.ParseInt(parentIDStr, 10, 64); parseErr == nil {
					parentID = id
				} else {
					parent, lookupErr := h.projectStore.GetProjectBySlugAny(r.Context(), parentIDStr)
					if lookupErr != nil {
						// Parent slug not found — auto-create it as a top-level project,
						// mirroring the behavior of the Allure results upload handler.
						if fsErr := h.store.CreateProject(r.Context(), parentIDStr); fsErr != nil {
							h.logger.Error("auto-creating parent project failed", zap.String("slug", parentIDStr), zap.Error(fsErr))
							writeError(w, http.StatusInternalServerError, "failed to create parent project")
							return
						}
						created, dbErr := h.projectStore.CreateProject(r.Context(), parentIDStr)
						if dbErr != nil && !errors.Is(dbErr, store.ErrProjectExists) {
							h.logger.Error("db parent registration failed", zap.String("slug", parentIDStr), zap.Error(dbErr))
							writeError(w, http.StatusInternalServerError, "failed to register parent project")
							return
						}
						if created != nil {
							parentID = created.ID
						} else {
							p, lookErr := h.projectStore.GetProjectBySlugAny(r.Context(), parentIDStr)
							if lookErr != nil {
								h.logger.Error("parent project lookup after create failed", zap.String("slug", parentIDStr), zap.Error(lookErr))
								writeError(w, http.StatusInternalServerError, "failed to resolve parent project")
								return
							}
							parentID = p.ID
						}
					} else {
						parentID = parent.ID
					}
				}
				project, dbErr := h.projectStore.CreateProjectWithParent(r.Context(), slug, parentID)
				if dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("slug", slug), zap.Error(dbErr))
					}
				}
				if project != nil {
					projectID = project.ID
				}
			} else {
				project, dbErr := h.projectStore.CreateProject(r.Context(), slug)
				if dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("slug", slug), zap.Error(dbErr))
					}
				}
				if project != nil {
					projectID = project.ID
				}
			}
			if projectID == 0 {
				if p, err := h.projectStore.GetProjectBySlug(r.Context(), slug); err == nil {
					projectID = p.ID
				}
			}
		} else {
			writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", pathValue))
			return
		}
	}

	// Mark as Playwright project.
	if err := h.projectStore.SetReportType(r.Context(), projectID, "playwright"); err != nil {
		h.logger.Warn("failed to set project report type", zap.Int64("project_id", projectID), zap.Error(err))
	}

	// Limit request body to prevent memory exhaustion.
	maxBodyBytes := int64(h.cfg.MaxUploadSizeMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	contentType := r.Header.Get("Content-Type")
	switch contentType {
	case "application/gzip",
		"application/x-gzip",
		"application/x-tar+gzip":
		// handled below
	default:
		writeError(w, http.StatusBadRequest, "Content-Type must be application/gzip")
		return
	}

	// Determine target directory: if build_number is provided, write directly
	// to the numbered build dir; otherwise stage in latest/ for the runner.
	targetDir := "latest"
	var buildNumber int
	if bn := r.URL.Query().Get("build_number"); bn != "" {
		var parseErr error
		buildNumber, parseErr = strconv.Atoi(bn)
		if parseErr != nil || buildNumber < 1 {
			writeError(w, http.StatusBadRequest, "build_number must be a positive integer")
			return
		}
		if _, lookupErr := h.buildStore.GetBuildByNumber(r.Context(), projectID, buildNumber); lookupErr != nil {
			if errors.Is(lookupErr, store.ErrBuildNotFound) {
				writeError(w, http.StatusNotFound, fmt.Sprintf("build %d not found for project %d", buildNumber, projectID))
				return
			}
			h.logger.Error("failed to look up build", zap.Int64("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(lookupErr))
			writeError(w, http.StatusInternalServerError, "failed to look up build")
			return
		}
		targetDir = strconv.Itoa(buildNumber)
	}

	if err := h.extractPlaywrightArchive(r, slug, targetDir); err != nil {
		code := http.StatusBadRequest
		msg := err.Error()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			code = http.StatusRequestEntityTooLarge
			msg = "request body too large"
		}
		writeError(w, code, msg)
		return
	}

	// If uploading to a specific build, mark it as having a Playwright report.
	if buildNumber > 0 {
		if setErr := h.buildStore.SetHasPlaywrightReport(r.Context(), projectID, buildNumber, true); setErr != nil {
			h.logger.Error("failed to set has_playwright_report", zap.Int64("project_id", projectID), zap.Int("build_number", buildNumber), zap.Error(setErr))
		}
		writeSuccess(w, http.StatusOK, map[string]any{
			"status": "uploaded",
		}, "Playwright report uploaded successfully")
		return
	}

	// Standalone upload (no build_number): if CI metadata is provided, submit an
	// ingestion job that creates a proper build with test results and CI context.
	execName := r.URL.Query().Get("execution_name")
	execFrom := r.URL.Query().Get("execution_from")
	ciBranch := r.URL.Query().Get("ci_branch")
	ciCommitSHA := r.URL.Query().Get("ci_commit_sha")

	if h.jobManager != nil && (execName != "" || execFrom != "" || ciBranch != "" || ciCommitSHA != "") {
		job := h.jobManager.SubmitPlaywright(projectID, slug, execName, execFrom, ciBranch, ciCommitSHA)
		writeSuccess(w, http.StatusAccepted, map[string]string{"job_id": job.ID}, "Playwright ingestion queued")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"status": "uploaded",
	}, "Playwright report uploaded successfully")
}

// extractPlaywrightArchive extracts a tar.gz Playwright report archive to the
// project's results directory, preserving subdirectory structure. Unlike the
// Allure handler, nested paths (e.g. data/screenshot.png) are preserved.
// It validates that index.html is present in the extracted contents.
func (h *PlaywrightHandler) extractPlaywrightArchive(r *http.Request, projectID, targetDir string) error {
	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		return fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)

	const maxFiles = 10000
	fileCount := 0
	foundIndex := false

	// Upload files to S3 concurrently — Playwright reports can contain hundreds
	// of files and sequential uploads easily exceed HTTP timeouts.
	const uploadConcurrency = 10
	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(uploadConcurrency)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Skip directories (created implicitly when writing nested files).
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		// Skip non-regular files (symlinks, hard links, etc.).
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("archive exceeds maximum file count (%d)", maxFiles)
		}

		// Clean and validate the path — preserve subdirs but prevent traversal.
		cleanName := filepath.ToSlash(filepath.Clean(hdr.Name))
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}
		// Strip leading "./" or "/"
		cleanName = strings.TrimPrefix(cleanName, "./")
		cleanName = strings.TrimPrefix(cleanName, "/")

		if cleanName == "" {
			continue
		}

		if cleanName == "index.html" {
			foundIndex = true
		}

		// Buffer file content so the tar reader can advance while S3 uploads
		// proceed concurrently.
		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("read %q from archive: %w", cleanName, err)
		}

		name := cleanName
		g.Go(func() error {
			return h.store.WritePlaywrightFile(ctx, projectID, targetDir+"/"+name, bytes.NewReader(data))
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if !foundIndex {
		return fmt.Errorf("archive does not contain index.html")
	}

	return nil
}
