package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// newOverlayHandler returns an HTTP handler that serves Allure report files
// with a transparent fallback from numbered build directories to reports/latest/.
//
// After the partial-copy StoreReport, numbered build dirs (e.g. reports/3/)
// contain only variable content: data/, widgets/, history/. Static assets such
// as index.html, app.js, and plugins/ live only in reports/latest/. The overlay
// handler serves those static assets from latest/ so that browsing historical
// builds works without storing full copies.
//
// Path structure (after StripPrefix removal of the route prefix):
//
//	{projectID}/reports/{N}/{rest}
//
// Resolution order:
//  1. File exists at {projectsDir}/{path}   → serve directly.
//  2. N is a numeric build ID and file is absent from build dir
//     → fall back to {projectsDir}/{projectID}/reports/latest/{rest}.
//  3. Neither location has the file → 404.
func newOverlayHandler(projectsDir string, ps store.ProjectStorer) http.Handler {
	fs := http.FileServer(http.Dir(projectsDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Canonicalise the URL path to prevent traversal and double-slash issues.
		cleanURL := path.Clean("/" + r.URL.Path)

		// Resolve the project segment (slug or numeric ID) to the on-disk storage key.
		trimmed := strings.TrimPrefix(cleanURL, "/")
		parts := strings.SplitN(trimmed, "/", 4)
		if len(parts) >= 1 && ps != nil {
			if sk, ok := resolveStorageKey(r.Context(), ps, parts[0]); ok && sk != parts[0] {
				parts[0] = sk
				cleanURL = "/" + strings.Join(parts, "/")
			}
		}

		diskPath := filepath.Join(projectsDir, filepath.FromSlash(cleanURL))

		if _, err := os.Stat(diskPath); err == nil { //nolint:gosec // G703: path constructed from sanitized URL path segments
			r2 := r.Clone(r.Context())
			r2.URL.Path = stripIndexHTML(cleanURL)
			fs.ServeHTTP(w, r2)
			return
		}

		// Try fallback: for numeric build dirs rewrite the path to latest/.
		// Expected structure (after leading "/"): {storageKey}/reports/{N}/{rest}
		if len(parts) >= 3 && parts[1] == "reports" && middleware.IsNumericID(parts[2]) {
			rest := ""
			if len(parts) == 4 {
				rest = "/" + parts[3]
			}
			latestURL := "/" + parts[0] + "/reports/latest" + rest
			latestDisk := filepath.Join(projectsDir, filepath.FromSlash(latestURL))
			if _, err := os.Stat(latestDisk); err == nil { //nolint:gosec // G703: path constructed from sanitized URL path segments
				r2 := r.Clone(r.Context())
				r2.URL.Path = stripIndexHTML(latestURL)
				fs.ServeHTTP(w, r2)
				return
			}
		}

		http.NotFound(w, r)
	})
}

// stripIndexHTML removes a trailing "index.html" component from a URL path.
// http.FileServer redirects "/path/index.html" → "/path/" with a 301; stripping
// upfront avoids that redirect and lets FileServer serve the file directly from
// its parent directory.
func stripIndexHTML(urlPath string) string {
	if strings.HasSuffix(urlPath, "/index.html") {
		return strings.TrimSuffix(urlPath, "index.html")
	}
	return urlPath
}

// resolveStorageKey resolves a raw URL path value (numeric id or slug) to the
// project's StorageKey. Returns the key and true on success, empty string and
// false when no matching project is found.
func resolveStorageKey(ctx context.Context, ps store.ProjectStorer, pathValue string) (string, bool) {
	if id, err := strconv.ParseInt(pathValue, 10, 64); err == nil {
		if p, err := ps.GetProject(ctx, id); err == nil {
			return p.StorageKey, true
		}
	}
	if p, err := ps.GetProjectBySlugAny(ctx, pathValue); err == nil {
		return p.StorageKey, true
	}
	return "", false
}

// newPlaywrightReportHandler returns an HTTP handler that serves Playwright HTML report
// files. Unlike the Allure overlay, no fallback is needed — each numbered build directory
// is a complete self-contained report (index.html, data/, etc.).
//
// The handler reads path parameters directly from the request context (set by Go 1.22+
// pattern matching): {projectID} and {reportID}. The wildcard segment {rest...} provides
// the remaining file path within the report directory.
func newPlaywrightReportHandler(st storage.Store, ps store.ProjectStorer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		reportID := r.PathValue("reportID")
		rest := r.PathValue("rest")

		storageKey, ok := resolveStorageKey(r.Context(), ps, projectID)
		if !ok {
			http.NotFound(w, r)
			return
		}

		// Serve index.html when no sub-path is requested.
		if rest == "" || rest == "/" {
			rest = "index.html"
		}

		// Build the subPath as "{reportID}/{rest}" — matches ReadPlaywrightFile's convention.
		subPath := reportID + "/" + strings.TrimPrefix(rest, "/")

		rc, contentType, err := st.ReadPlaywrightFile(r.Context(), storageKey, subPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = rc.Close() }()

		w.Header().Set("Content-Type", contentType)
		_, _ = io.Copy(w, rc)
	})
}

// newS3ReportHandler returns an HTTP handler that serves Allure report files from S3.
// For numbered builds, it tries the build dir first, then falls back to latest/ for
// static assets (same overlay pattern as the local filesystem handler).
func newS3ReportHandler(st storage.Store, ps store.ProjectStorer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expected path (after StripPrefix): {projectID}/reports/{reportID}/{rest}
		cleanURL := path.Clean("/" + r.URL.Path)
		trimmed := strings.TrimPrefix(cleanURL, "/")
		parts := strings.SplitN(trimmed, "/", 4)
		if len(parts) < 3 || parts[1] != "reports" {
			http.NotFound(w, r)
			return
		}
		projectID := parts[0]
		if ps != nil {
			if sk, ok := resolveStorageKey(r.Context(), ps, projectID); ok {
				projectID = sk
			}
		}
		reportID := parts[2]
		filePath := ""
		if len(parts) == 4 {
			filePath = parts[3]
		}
		// Strip trailing index.html so directory index is served
		filePath = strings.TrimSuffix(filePath, "/index.html")
		if filePath == "" {
			filePath = "index.html"
		}

		// Try direct serve from the requested build dir
		rc, contentType, err := st.OpenReportFile(r.Context(), projectID, reportID, filePath)
		if err == nil {
			defer func() { _ = rc.Close() }()
			w.Header().Set("Content-Type", contentType)
			_, _ = io.Copy(w, rc)
			return
		}

		// Overlay fallback: for numeric build IDs, try latest/
		if middleware.IsNumericID(reportID) {
			rc, contentType, err = st.OpenReportFile(r.Context(), projectID, "latest", filePath)
			if err == nil {
				defer func() { _ = rc.Close() }()
				w.Header().Set("Content-Type", contentType)
				_, _ = io.Copy(w, rc)
				return
			}
		}

		http.NotFound(w, r)
	})
}
