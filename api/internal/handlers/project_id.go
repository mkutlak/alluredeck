package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// Sentinel errors for project_id validation.
var (
	ErrProjectRequired     = errors.New("project_id is required")
	ErrProjectTooLong      = errors.New("project_id must not exceed 100 characters")
	ErrProjectInvalidChars = errors.New("project_id contains invalid characters")
	ErrProjectReserved     = errors.New("project_id is reserved")
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
