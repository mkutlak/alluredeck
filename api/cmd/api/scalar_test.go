package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mkutlak/alluredeck/api/internal/swagger"
)

func TestScalarHandler_ServesHTML(t *testing.T) {
	h := newScalarHandler()
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html content-type, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "@scalar/api-reference") {
		t.Fatal("response body missing @scalar/api-reference script")
	}
}

func TestScalarHandler_ServesSpec(t *testing.T) {
	h := newScalarHandler()
	req := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json content-type, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"swagger"`) {
		t.Fatal("response body missing swagger field")
	}
}

func TestScalarHandler_SetsCSP(t *testing.T) {
	h := newScalarHandler()
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "cdn.jsdelivr.net") {
		t.Fatalf("CSP header missing cdn.jsdelivr.net, got %q", csp)
	}
}
