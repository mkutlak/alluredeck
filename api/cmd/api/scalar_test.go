package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mkutlak/alluredeck/api/internal/swagger"
)

// TestScalarHandler serves the API reference page and the registered swagger
// spec, both under a CSP that admits the jsDelivr CDN the page loads from.
func TestScalarHandler(t *testing.T) {
	h := newScalarHandler()
	for _, tc := range []struct{ path, wantType, wantBody string }{
		{"/swagger/", "text/html", "@scalar/api-reference"},
		{"/swagger/doc.json", "application/json", `"swagger"`},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
			t.Errorf("%s: Content-Type = %q, want %s", tc.path, ct, tc.wantType)
		}
		if !strings.Contains(rec.Body.String(), tc.wantBody) {
			t.Errorf("%s: body lacks %s", tc.path, tc.wantBody)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "cdn.jsdelivr.net") {
			t.Errorf("%s: CSP %q does not admit cdn.jsdelivr.net", tc.path, csp)
		}
	}
}
