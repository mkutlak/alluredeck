package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCacheHeaders: each cache middleware overwrites any Cache-Control set
// earlier in the chain and still calls the next handler. ReportCache marks
// numbered (historical) builds immutable and everything else short-lived.
func TestCacheHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mw       func(http.HandlerFunc) http.HandlerFunc
		reportID string
		want     string
	}{
		{"NoStore", NoStore, "", CacheNoStore},
		{"CacheControl", CacheControl(CacheMutable), "", CacheMutable},
		{"ReportCache numbered build", ReportCache, "42", CacheImmutable},
		{"ReportCache latest", ReportCache, "latest", CacheShortLived},
	}
	for _, tc := range tests {
		called := false
		h := tc.mw(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("report_id", tc.reportID)
		rr := httptest.NewRecorder()
		rr.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("Cache-Control"); got != tc.want || !called {
			t.Errorf("%s: Cache-Control %q (next called: %v), want %q", tc.name, got, called, tc.want)
		}
	}
}

func TestIsNumericID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{"0", true},
		{"latest", false},
		{"", false},
		{"12abc", false},
		{"abc12", false},
		{"-1", false},
	}
	for _, tc := range tests {
		if got := IsNumericID(tc.input); got != tc.want {
			t.Errorf("IsNumericID(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
