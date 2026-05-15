package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/mcp"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
)

func main() {
	cfg, encKey, logger := mustLoadConfig()

	if !cfg.MCPServerEnabled {
		logger.Info("MCP server disabled via feature flag", zap.String("env", "ENABLE_MCP_SERVER"))
		_ = logger.Sync()
		os.Exit(0)
	}
	defer func() { _ = logger.Sync() }()

	if cfg.MCPSigningKey == "" {
		logger.Fatal("MCP_SIGNING_KEY must be set when ENABLE_MCP_SERVER=true")
	}

	// Initialise OTel (no-op when cfg.Observability.Enabled is false).
	obsShutdown, err := bootstrap.InitOTel(context.Background(), cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialise observability", zap.Error(err))
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := obsShutdown(shutCtx); err != nil {
			logger.Error("observability shutdown error", zap.Error(err))
		}
	}()

	// Initialise file storage backend (local FS or S3) so resources/read can
	// inline small text/image attachments directly rather than returning signed URLs.
	dataStore, err := createDataStore(cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Open PostgreSQL pool with MCP-specific MaxConns (smaller than cmd/api default).
	poolCfg := bootstrap.PoolConfig{MaxConns: cfg.MCPPoolMaxConns}
	stores, err := bootstrap.InitStores(context.Background(), cfg, poolCfg, encKey, nil, logger)
	if err != nil {
		logger.Fatal("failed to open PostgreSQL database", zap.Error(err))
	}
	defer func() { _ = stores.Close() }()

	jwtManager := bootstrap.InitJWTManager(cfg, stores.Blacklist, logger)

	// Per-request is_active recheck cache (F-3). 30s TTL, 10k entries.
	userActiveCache := middleware.NewUserActiveCache(stores.User, 30*time.Second, 10000)

	// Parse allowed origins from config.
	allowedOrigins := mcp.ParseAllowedOrigins(cfg.MCPAllowedOrigins)

	mcpCfg := mcp.Config{
		AllowedOrigins:  allowedOrigins,
		RateLimitPerMin: cfg.MCPRateLimitPerMin,
		RateLimitBurst:  cfg.MCPRateLimitBurst,
		PublicURL:       cfg.ExternalURL,
		SigningKey:      []byte(cfg.MCPSigningKey),
		DataStore:       dataStore,
	}

	mcpHandler, _, err := mcp.NewServer(mcpCfg, stores, jwtManager, userActiveCache, logger)
	if err != nil {
		logger.Fatal("failed to create MCP server", zap.Error(err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", mcp.HealthHandler)
	mux.HandleFunc("GET /healthz", mcp.HealthHandler)
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.MCPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       5 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Start JWT blacklist cleanup goroutine (prunes expired JTIs every 15 min).
	jwtManager.StartCleanup(ctx, 15*time.Minute)

	go func() {
		logger.Info("starting MCP server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("MCP server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining MCP connections")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("MCP server shutdown error", zap.Error(err))
	}
	logger.Info("MCP server stopped cleanly")
}

// createDataStore initialises the storage backend based on StorageType config.
// Mirrors the same helper in cmd/api/main.go so both binaries read from the
// same storage backend (local FS path or S3 bucket).
func createDataStore(cfg *config.Config, logger *zap.Logger) (storage.Store, error) {
	switch cfg.StorageType {
	case "s3":
		st, err := storage.NewS3Store(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("init S3 store: %w", err)
		}
		return st, nil
	default:
		return storage.NewLocalStore(cfg), nil
	}
}

// mustLoadConfig loads and validates configuration, initialises the logger,
// hashes passwords if security is enabled, and derives the encryption key.
// Terminates the process on any fatal configuration error.
func mustLoadConfig() (*config.Config, []byte, *zap.Logger) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := bootstrap.InitLogger(cfg)

	if err := cfg.Validate(); err != nil {
		logger.Fatal("configuration error", zap.Error(err))
	}

	if cfg.SecurityEnabled {
		if err := cfg.HashPasswords(); err != nil {
			logger.Fatal("failed to hash passwords", zap.Error(err))
		}
	}

	encKey := security.DeriveEncryptionKey(cfg.JWTSecret)
	return cfg, encKey, logger
}
