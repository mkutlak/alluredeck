package middleware

import "net/http"

// Cache-Control directive constants.
const (
	CacheImmutable  = "public, max-age=86400, immutable"
	CacheShortLived = "public, max-age=300"
	CacheMutable    = "public, max-age=30"
	CacheNoStore    = "no-store"
)

// NoStore sets Cache-Control: no-store on the response. Use for write
// endpoints (POST/PUT/DELETE), auth routes, and config/version.
func NoStore(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", CacheNoStore)
		next(w, r)
	}
}

// CacheControl returns a middleware that sets the given Cache-Control directive.
// Use for GET endpoints with known cacheability (mutable lists, analytics).
func CacheControl(directive string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", directive)
			next(w, r)
		}
	}
}

// ReportCache inspects the report_id path value to choose a cache tier.
// Numeric IDs (historical builds) get immutable caching; "latest" and
// everything else get short-lived caching.
func ReportCache(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID := r.PathValue("report_id")
		if isNumericID(reportID) {
			w.Header().Set("Cache-Control", CacheImmutable)
		} else {
			w.Header().Set("Cache-Control", CacheShortLived)
		}
		next(w, r)
	}
}

// isNumericID returns true if s is a non-empty string of ASCII digits.
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
