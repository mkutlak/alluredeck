package pg

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PGStore is the main PostgreSQL database handle wrapping a pgxpool connection pool.
type PGStore struct {
	pool  *pgxpool.Pool
	sqlDB *sql.DB // backed by pool via stdlib.OpenDBFromPool
}

// Open creates a new PGStore, applying all pending goose migrations before returning.
func Open(ctx context.Context, cfg *config.Config) (*PGStore, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	if cfg.DBMaxOpenConns > 0 {
		poolCfg.MaxConns = int32(cfg.DBMaxOpenConns) //nolint:gosec // G115: bounded by config
	}
	if cfg.DBMaxIdleConns > 0 {
		poolCfg.MinConns = int32(cfg.DBMaxIdleConns) //nolint:gosec // G115: bounded by config
	}
	if cfg.DBConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.DBConnMaxLifetime
	} else {
		poolCfg.MaxConnLifetime = 5 * time.Minute
	}
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)

	s := &PGStore{pool: pool, sqlDB: sqlDB}
	if err := s.applyMigrations(); err != nil {
		_ = sqlDB.Close()
		pool.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	if err := s.applyRiverMigrations(ctx); err != nil {
		_ = sqlDB.Close()
		pool.Close()
		return nil, fmt.Errorf("apply River migrations: %w", err)
	}
	return s, nil
}

// Pool returns the underlying *pgxpool.Pool for sub-stores.
func (s *PGStore) Pool() *pgxpool.Pool {
	return s.pool
}

// DB returns a *sql.DB backed by the connection pool.
// Used by SystemHandler for readiness probing.
func (s *PGStore) DB() *sql.DB {
	return s.sqlDB
}

// Close closes the sql.DB wrapper and the underlying connection pool.
func (s *PGStore) Close() error {
	if err := s.sqlDB.Close(); err != nil {
		return fmt.Errorf("close sql.DB: %w", err)
	}
	s.pool.Close()
	return nil
}

// applyRiverMigrations runs River's own schema migrations (river_jobs, river_leaders, etc.).
func (s *PGStore) applyRiverMigrations(ctx context.Context) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(s.pool), nil)
	if err != nil {
		return fmt.Errorf("create River migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("run River migrations: %w", err)
	}
	return nil
}

// applyMigrations runs all pending goose SQL migrations embedded in the binary.
func (s *PGStore) applyMigrations() error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(s.sqlDB, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
