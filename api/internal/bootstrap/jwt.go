package bootstrap

import (
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

// InitJWTManager creates and returns the JWTManager.
// The blacklist store (from *Stores.Blacklist) is required for token revocation.
// Must be called after InitStores so the blacklist store is available.
func InitJWTManager(cfg *config.Config, blacklist security.BlacklistStore, logger *zap.Logger) *security.JWTManager {
	return security.NewJWTManager(cfg, blacklist, logger)
}
