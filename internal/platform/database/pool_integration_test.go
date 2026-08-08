package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/platform/config"
	"github.com/davidchandra95/keebhub/internal/platform/database"
)

func TestPoolConnectsToPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, config.Config{
		DatabaseURL:       databaseURL,
		DBMaxConns:        4,
		DBMinConns:        1,
		DBMaxConnLifetime: 30 * time.Minute,
		DBMaxConnIdleTime: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if pool.Config().MaxConns != 4 || pool.Config().MinConns != 1 {
		t.Errorf("pool sizes = %d/%d, want 1/4", pool.Config().MinConns, pool.Config().MaxConns)
	}
}
