package pg

import (
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// applyOTelTracer wires the OpenTelemetry pgx query tracer into a pool config.
// Query parameters are excluded from span attributes to avoid capturing PII.
// Call this after ParseConfig and before pgxpool.NewWithConfig.
func applyOTelTracer(cfg *pgxpool.Config) {
	// Default: query parameters are NOT included in span attributes (avoids PII).
	// Use otelpgx.WithIncludeQueryParameters() only if explicitly needed.
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
}
