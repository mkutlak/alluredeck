package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// TestSecurityHeaders: every response gets the fixed security headers and
// keeps the downstream status; HSTS is sent only when TLS is on (a nil config
// must not panic).
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	always := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
	}
	for _, tc := range []struct {
		name     string
		cfg      *config.Config
		wantHSTS string
	}{
		{"TLS off", &config.Config{TLS: false}, ""},
		{"TLS on", &config.Config{TLS: true}, "max-age=31536000; includeSubDomains"},
		{"nil config", nil, ""},
	} {
		h := SecurityHeaders(tc.cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusTeapot {
			t.Errorf("%s: downstream status not preserved: got %d", tc.name, rr.Code)
		}
		for header, want := range always {
			if got := rr.Header().Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", tc.name, header, got, want)
			}
		}
		if got := rr.Header().Get("Strict-Transport-Security"); got != tc.wantHSTS {
			t.Errorf("%s: HSTS = %q, want %q", tc.name, got, tc.wantHSTS)
		}
	}
}
