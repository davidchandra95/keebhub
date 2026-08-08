// Package database configures the PostgreSQL connection pool.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/davidchandra95/keebhub/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a lazy pgx pool. Readiness, rather than startup, reports live connectivity.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	return pool, nil
}
