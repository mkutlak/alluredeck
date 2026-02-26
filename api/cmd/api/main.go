package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	_ "github.com/mkutlak/alluredeck/api/internal/swagger"
)

// @title           AllureDeck API
// @version         2.0
// @description     This is an API service for managing Allure reports.
// @host            localhost:8080
// @BasePath        /
func main() {
	cfg := config.LoadConfig()

	// Fail fast if security is enabled with an insecure default secret (AUDIT 1.3).
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Open SQLite metadata store.
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to open database at %s: %v", cfg.DatabasePath, err)
	}
	defer func() { _ = db.Close() }()

	projectStore := store.NewProjectStore(db)
	buildStore := store.NewBuildStore(db)
	blacklistStore := store.NewBlacklistStore(db)
	lockManager := store.NewLockManager()

	dataStore, err := createDataStore(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err) //nolint:gocritic // exitAfterDefer: in-process SQLite; OS cleans up on exit
	}

	// Import existing filesystem projects and builds into the database on startup.
	if err := store.SyncMetadata(context.Background(), dataStore, db); err != nil {
		log.Printf("WARNING: metadata sync failed (non-fatal): %v", err)
	}

	jwtManager := security.NewJWTManager(cfg, blacklistStore)
	systemHandler := handlers.NewSystemHandler(cfg)
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)

	allureCore := runner.NewAllure(cfg, dataStore, buildStore, lockManager)
	allureHandler := handlers.NewAllureHandler(cfg, allureCore, projectStore, buildStore, dataStore)

	backgroundWatcher := runner.NewWatcher(cfg, allureCore, projectStore, dataStore)

	mux := http.NewServeMux()

	// Swagger UI
	mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Overlay file server — serves generated Allure HTML reports.
	// Numbered build dirs contain only variable content (data/, widgets/, history/).
	// Static assets (index.html, JS, CSS, plugins/) are served from reports/latest/
	// via a fallback overlay, achieving ~90% disk reduction per build.
	// Backward compatible: old full-copy builds are served directly from their dir.
	var overlayFS http.Handler
	if cfg.StorageType == "s3" {
		overlayFS = newS3ReportHandler(dataStore)
	} else {
		overlayFS = newOverlayHandler(cfg.ProjectsDirectory)
	}
	// Allure report pages are third-party static content requiring inline scripts/styles.
	// Override the strict API CSP with a permissive one for report routes.
	// Build frame-ancestors from CORS_ALLOWED_ORIGINS so only the configured
	// dashboard origin can embed reports in iframes. Falls back to '*' if no
	// origins are configured (e.g. local dev without explicit CORS setting).
	frameAncestors := strings.Join(cfg.CORSAllowedOrigins, " ")
	if frameAncestors == "" {
		frameAncestors = "*"
	}
	reportCSP := "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob: https:; frame-ancestors " + frameAncestors
	reportHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", reportCSP)
		// Remove X-Frame-Options set by SecurityHeaders middleware; CSP frame-ancestors
		// is the modern replacement and takes precedence in all current browsers.
		w.Header().Del("X-Frame-Options")
		overlayFS.ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/projects/", http.StripPrefix("/api/v1/projects/", reportHandler))
	mux.Handle("/projects/", http.StripPrefix("/projects/", reportHandler))

	// Rate limiter for login endpoint — 5 req/s, burst 10, 15min stale TTL (REVIEW #8).
	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute)
	limiterDone := make(chan struct{})
	loginLimiter.StartCleanup(5*time.Minute, limiterDone)

	registerRoutes(mux, "", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler)
	registerRoutes(mux, "/api/v1", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler)

	// Chain middleware: Recovery → SecurityHeaders → CSRF → CORS → mux (AUDIT 3.1, 2.6, REVIEW #11).
	handler := middleware.Recovery(
		middleware.SecurityHeaders(
			middleware.CSRFMiddleware(cfg)(
				middleware.CORSMiddleware(cfg, mux),
			),
		),
	)

	addr := "0.0.0.0:" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,  // AUDIT 2.5
		WriteTimeout: 60 * time.Second,  // AUDIT 2.5
		IdleTimeout:  120 * time.Second, // AUDIT 2.5
	}

	// Graceful shutdown on SIGTERM / SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Start JWT blacklist cleanup goroutine — prunes expired JTIs every 15 min (AUDIT 2.1).
	jwtManager.StartCleanup(ctx, 15*time.Minute)

	if cfg.StorageType != "s3" {
		backgroundWatcher.Start()
	} else {
		log.Println("Background file watcher is disabled (StorageType=s3)")
	}

	go func() {
		log.Printf("Starting Allure API server on %s (DevMode: %v)\n", addr, cfg.DevMode)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	awaitShutdown(ctx, srv, backgroundWatcher, cfg, limiterDone)
}

// awaitShutdown waits for the context to be cancelled then drains the HTTP
// server and stops the background watcher before returning.
func awaitShutdown(ctx context.Context, srv *http.Server, watcher *runner.Watcher, cfg *config.Config, limiterDone chan struct{}) {
	<-ctx.Done()
	log.Println("Shutdown signal received, draining connections...")

	close(limiterDone) // stop rate limiter cleanup goroutine

	if cfg.StorageType != "s3" {
		watcher.Stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped cleanly")
}

// createDataStore initialises the storage backend based on StorageType config.
func createDataStore(cfg *config.Config) (storage.Store, error) {
	switch cfg.StorageType {
	case "s3":
		st, err := storage.NewS3Store(cfg)
		if err != nil {
			return nil, fmt.Errorf("init S3 store: %w", err)
		}
		return st, nil
	default:
		return storage.NewLocalStore(cfg), nil
	}
}

// registerRoutes mounts all API routes under an optional URL prefix.
// Having this helper ensures both the bare and /api/v1 prefixed
// routes are always identical — adding one route adds both automatically.
func registerRoutes(
	mux *http.ServeMux,
	prefix string,
	cfg *config.Config,
	jwtManager *security.JWTManager,
	loginLimiter *middleware.IPRateLimiter,
	system *handlers.SystemHandler,
	authHandler *handlers.AuthHandler,
	allure *handlers.AllureHandler,
) {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return middleware.AuthMiddleware(cfg, jwtManager, false)(h)
	}
	rateLimit := middleware.RateLimitMiddleware(loginLimiter)

	// RBAC wrappers (REVIEW #9): auth + role enforcement.
	adminOnly := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("admin")(h))
	}
	// viewerUp: when MakeViewerEndptsPub is true, these endpoints are public (no auth).
	viewerUp := func(h http.HandlerFunc) http.HandlerFunc {
		if cfg.MakeViewerEndptsPub {
			return h
		}
		return auth(middleware.RequireRole("viewer")(h))
	}

	// Public endpoints
	mux.HandleFunc("GET "+prefix+"/version", system.Version)
	mux.HandleFunc("GET "+prefix+"/config", system.ConfigEndpoint)
	mux.HandleFunc("POST "+prefix+"/login", rateLimit(authHandler.Login))

	// Auth only (no specific role required)
	mux.HandleFunc("DELETE "+prefix+"/logout", auth(authHandler.Logout))

	// Viewer+ endpoints (public when MakeViewerEndptsPub=true)
	mux.HandleFunc("GET "+prefix+"/projects", viewerUp(allure.GetProjects))
	mux.HandleFunc("GET "+prefix+"/emailable-report/render", viewerUp(allure.GetEmailableReport))
	mux.HandleFunc("GET "+prefix+"/report-history", viewerUp(allure.GetReportHistory))

	// Admin only endpoints
	mux.HandleFunc("POST "+prefix+"/projects", adminOnly(allure.CreateProject))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}", adminOnly(allure.DeleteProject))
	mux.HandleFunc("POST "+prefix+"/generate-report", adminOnly(allure.GenerateReport))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/history", adminOnly(allure.CleanHistory))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/results", adminOnly(allure.CleanResults))
	mux.HandleFunc("POST "+prefix+"/send-results", adminOnly(allure.SendResults))
	mux.HandleFunc("DELETE "+prefix+"/report", adminOnly(allure.DeleteReport))
}
