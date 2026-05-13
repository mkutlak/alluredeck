package bootstrap

import (
	"context"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/observability"
)

// InitOTel bootstraps the OpenTelemetry SDK and returns a composite shutdown
// function. The shutdown must be called after the HTTP server drains and the
// DB pool closes so in-flight spans have time to flush.
//
// When cfg.Observability.Enabled is false this is a fast no-op that installs
// noop providers; the returned shutdown func is a no-op as well.
func InitOTel(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(context.Context) error, error) {
	return observability.Init(ctx, cfg.Observability, logger)
}
