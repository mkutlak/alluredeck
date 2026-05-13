// Package bootstrap contains shared initialisation helpers that can be called
// by multiple entry-point binaries (cmd/api, cmd/mcp, …) without duplicating
// the wiring logic found in cmd/api/main.go.
package bootstrap

import (
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/logging"
)

// InitLogger creates and returns the root Zap logger.
// devMode=true produces a human-readable console encoder; false produces JSON.
// The logger is also installed as the global zap.L() / zap.S() logger.
func InitLogger(cfg *config.Config) *zap.Logger {
	return logging.Setup(cfg.DevMode, cfg.LogLevel)
}
