package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestRegisterRoutes_NumberedReportsAreNotImmutable: build numbers are reused
// once a project's history is cleaned, so build 7 can be a different build
// tomorrow. A browser that was told 7 never changes would keep showing the old
// one for a day; a numbered report is cached for as short a time as "latest".
func TestRegisterRoutes_NumberedReportsAreNotImmutable(t *testing.T) {
	// No project exists, so each handler answers 404 once the cache header is set.
	projects := testutil.NewMemProjectStore()
	hs := allHandlers(t)
	hs.report = handlers.NewReportHandler(handlers.ReportHandlerDeps{ProjectStore: projects, Logger: zap.NewNop()})
	hs.attachment = handlers.NewAttachmentHandler(&testutil.MockAttachmentStore{}, &testutil.MockBuildStore{}, projects, nil, zap.NewNop())
	mux := http.NewServeMux()
	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          &config.Config{MakeViewerEndpointsPublic: true},
		loginLimiter: middleware.NewIPRateLimiter(1000, 1000, time.Minute, false),
		h:            hs,
	})

	for _, path := range []string{
		"/api/v1/projects/1/reports/7/summary",
		"/api/v1/projects/1/reports/7/categories",
		"/api/v1/projects/1/reports/7/attachments",
		"/api/v1/projects/1/reports/latest/summary",
	} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rr.Header().Get("Cache-Control"); got != middleware.CacheShortLived {
			t.Errorf("GET %s: Cache-Control = %q, want %q", path, got, middleware.CacheShortLived)
		}
	}
}
