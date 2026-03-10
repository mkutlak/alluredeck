package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoStore(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := NoStore(inner)

	t.Run("SetsCacheControlNoStore", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheNoStore {
			t.Errorf("expected Cache-Control %q, got %q", CacheNoStore, got)
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("OverwritesExistingHeader", func(t *testing.T) {
		t.Parallel()
		h := NoStore(inner)

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rr := httptest.NewRecorder()
		// Simulate a prior middleware having set a different Cache-Control.
		rr.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheNoStore {
			t.Errorf("expected Cache-Control %q after overwrite, got %q", CacheNoStore, got)
		}
	})
}

func TestCacheControl(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("MutableDirective", func(t *testing.T) {
		t.Parallel()
		handler := CacheControl(CacheMutable)(inner)
		req := httptest.NewRequest(http.MethodGet, "/projects", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheMutable {
			t.Errorf("expected Cache-Control %q, got %q", CacheMutable, got)
		}
	})

	t.Run("ShortLivedDirective", func(t *testing.T) {
		t.Parallel()
		handler := CacheControl(CacheShortLived)(inner)
		req := httptest.NewRequest(http.MethodGet, "/analytics", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheShortLived {
			t.Errorf("expected Cache-Control %q, got %q", CacheShortLived, got)
		}
	})

	t.Run("CallsNextHandler", func(t *testing.T) {
		t.Parallel()
		called := false
		h := CacheControl(CacheMutable)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if !called {
			t.Error("expected next handler to be called")
		}
	})
}

func TestReportCache(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := ReportCache(inner)

	t.Run("NumericReportID_Immutable", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/projects/proj1/reports/42/categories", nil)
		req.SetPathValue("report_id", "42")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheImmutable {
			t.Errorf("expected Cache-Control %q for numeric report_id, got %q", CacheImmutable, got)
		}
	})

	t.Run("LargeNumericReportID_Immutable", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/projects/proj1/reports/1000/categories", nil)
		req.SetPathValue("report_id", "1000")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheImmutable {
			t.Errorf("expected Cache-Control %q for large numeric report_id, got %q", CacheImmutable, got)
		}
	})

	t.Run("LatestReportID_ShortLived", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/projects/proj1/reports/latest/categories", nil)
		req.SetPathValue("report_id", "latest")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheShortLived {
			t.Errorf("expected Cache-Control %q for latest report_id, got %q", CacheShortLived, got)
		}
	})

	t.Run("EmptyReportID_ShortLived", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/projects/proj1/reports//categories", nil)
		req.SetPathValue("report_id", "")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		got := rr.Header().Get("Cache-Control")
		if got != CacheShortLived {
			t.Errorf("expected Cache-Control %q for empty report_id, got %q", CacheShortLived, got)
		}
	})

	t.Run("CallsNextHandler", func(t *testing.T) {
		t.Parallel()
		called := false
		h := ReportCache(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("report_id", "latest")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if !called {
			t.Error("expected next handler to be called")
		}
	})
}

func TestIsNumericID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{"1000", true},
		{"0", true},
		{"1", true},
		{"latest", false},
		{"", false},
		{"abc", false},
		{"12abc", false},
		{"abc12", false},
		{"-1", false},
		{"1.5", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := isNumericID(tc.input)
			if got != tc.want {
				t.Errorf("isNumericID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
