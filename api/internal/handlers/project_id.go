package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Sentinel errors for project_id validation.
var (
	ErrProjectRequired     = errors.New("project_id is required")
	ErrProjectTooLong      = errors.New("project_id must not exceed 100 characters")
	ErrProjectInvalidChars = errors.New("project_id contains invalid characters")
	ErrProjectReserved     = errors.New("project_id is reserved")
	ErrProjectNumericOnly  = errors.New("project_id must contain at least one letter")
	ErrProjectInvalid      = errors.New("invalid project_id")
)

// validateReportID rejects report IDs that could cause path traversal.
// Accepts "latest" or non-empty all-digit strings (positive integers).
func validateReportID(reportID string) error {
	if reportID == "" {
		return ErrReportIDRequired
	}
	if reportID == "latest" {
		return nil
	}
	for _, c := range reportID {
		if c < '0' || c > '9' {
			return ErrReportIDInvalid
		}
	}
	return nil
}

// validateTicketURL rejects URLs with non-http(s) schemes (e.g. javascript:, data:).
// An empty URL is allowed (optional field). Returns an error for dangerous schemes.
func validateTicketURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrTicketURLInvalidScheme
	}
	switch parsed.Scheme {
	case "http", "https":
		return nil
	default:
		return ErrTicketURLInvalidScheme
	}
}

// extractReportID extracts and validates the "report_id" path parameter.
// On failure it writes a 400 response and returns ("", false).
func extractReportID(w http.ResponseWriter, r *http.Request) (string, bool) {
	reportID := r.PathValue("report_id")
	if err := validateReportID(reportID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return reportID, true
}

// reservedProjectNames lists names that clash with API route segments.
//
//nolint:gochecknoglobals // read-only constant-like lookup table for reserved project names
var reservedProjectNames = map[string]bool{
	"login":   true,
	"logout":  true,
	"version": true,
	"config":  true,
	"swagger": true,
	".":       true,
	"..":      true,
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
	allDigits := true
	for _, r := range projectID {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return ErrProjectNumericOnly
	}
	if strings.ContainsAny(projectID, "/\\") || strings.Contains(projectID, "..") {
		return ErrProjectInvalidChars
	}
	if reservedProjectNames[projectID] {
		return fmt.Errorf("project_id %q: %w", projectID, ErrProjectReserved)
	}
	// Belt-and-suspenders: verify the resolved path stays under projectsDir.
	resolved := filepath.Join(projectsDir, projectID)
	rel, err := filepath.Rel(projectsDir, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ErrProjectInvalid
	}
	return nil
}

// safeProjectID resolves to "default" when empty, then validates.
func safeProjectID(projectsDir, raw string) (string, error) {
	if raw == "" {
		raw = "default"
	}
	if err := validateProjectID(projectsDir, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// extractProjectID extracts, unescapes, and validates the "project_id" path
// parameter. On failure it writes a 400 response and returns ("", false).
func extractProjectID(w http.ResponseWriter, r *http.Request, projectsDir string) (string, bool) {
	raw := r.PathValue("project_id")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id encoding")
		return "", false
	}
	projectID, err := safeProjectID(projectsDir, unescaped)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return projectID, true
}

// extractProjectIntID parses the numeric project ID from the URL path.
func extractProjectIntID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := r.PathValue("project_id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "project_id must be a numeric ID")
		return 0, false
	}
	return id, true
}

// resolveProjectIntID resolves a project path value (numeric ID or slug) to its
// int64 primary key. On failure it writes an error response and returns (0, false).
func resolveProjectIntID(w http.ResponseWriter, r *http.Request, ps store.ProjectStorer) (int64, bool) {
	pathValue := r.PathValue("project_id")
	if pathValue == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return 0, false
	}

	// Fallback to numeric-only parsing when no project store is available.
	if ps == nil {
		return extractProjectIntID(w, r)
	}

	// Numeric ID — accept directly without a DB round-trip.
	if id, parseErr := strconv.ParseInt(pathValue, 10, 64); parseErr == nil {
		return id, true
	}

	// Validate slug before DB lookup.
	if strings.ContainsAny(pathValue, "/\\") || strings.Contains(pathValue, "..") {
		writeError(w, http.StatusBadRequest, ErrProjectInvalidChars.Error())
		return 0, false
	}
	if reservedProjectNames[pathValue] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("project_id %q: %s", pathValue, ErrProjectReserved.Error()))
		return 0, false
	}

	// Fall back to slug lookup (includes child projects).
	project, err := ps.GetProjectBySlugAny(r.Context(), pathValue)
	if err == nil {
		return project.ID, true
	}
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, false
	}
	writeError(w, http.StatusInternalServerError, "error fetching project")
	return 0, false
}
