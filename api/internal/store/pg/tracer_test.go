package pg

import (
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestOTelTracerWiredIntoPool verifies that the pgx OTel tracer is applied to a
// pool config. It parses a dummy DSN (no live DB required) and checks that
// ConnConfig.Tracer is non-nil and of the expected type.
func TestOTelTracerWiredIntoPool(t *testing.T) {
	t.Parallel()

	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/testdb")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	applyOTelTracer(poolCfg)

	if poolCfg.ConnConfig.Tracer == nil {
		t.Fatal("expected non-nil tracer on ConnConfig.Tracer after applyOTelTracer")
	}

	if _, ok := poolCfg.ConnConfig.Tracer.(*otelpgx.Tracer); !ok {
		t.Errorf("expected *otelpgx.Tracer, got %T", poolCfg.ConnConfig.Tracer)
	}
}
