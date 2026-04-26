package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	noopHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("AlwaysSetHeaders", func(t *testing.T) {
		t.Parallel()
		h := SecurityHeaders(&config.Config{TLS: false})(noopHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		expectations := map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "default-src 'self'",
			"Referrer-Policy":         "strict-origin-when-cross-origin",
			"Permissions-Policy":      "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		}
		for header, want := range expectations {
			if got := rr.Header().Get(header); got != want {
				t.Errorf("%s header: got %q, want %q", header, got, want)
			}
		}
	})

	t.Run("HSTSOmittedWhenTLSFalse", func(t *testing.T) {
		t.Parallel()
		h := SecurityHeaders(&config.Config{TLS: false})(noopHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS should be empty when TLS=false; got %q", got)
		}
	})

	t.Run("HSTSPresentWhenTLSTrue", func(t *testing.T) {
		t.Parallel()
		h := SecurityHeaders(&config.Config{TLS: true})(noopHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		want := "max-age=31536000; includeSubDomains"
		if got := rr.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("HSTS header: got %q, want %q", got, want)
		}
	})

	t.Run("NilConfigOmitsHSTS", func(t *testing.T) {
		t.Parallel()
		// Defensive: nil cfg must not panic and must omit HSTS.
		h := SecurityHeaders(nil)(noopHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS should be empty when cfg is nil; got %q", got)
		}
		if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("nosniff still expected with nil cfg; got %q", got)
		}
	})

	t.Run("PassesThroughResponseStatus", func(t *testing.T) {
		t.Parallel()
		h := SecurityHeaders(&config.Config{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusTeapot {
			t.Errorf("downstream status not preserved: got %d", rr.Code)
		}
	})
}
