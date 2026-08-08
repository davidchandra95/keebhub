// Package testdatabase provides isolated PostgreSQL schemas for integration tests.
package testdatabase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/platform/config"
	"github.com/davidchandra95/keebhub/internal/platform/database"
	"github.com/davidchandra95/keebhub/internal/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	Pool             *pgxpool.Pool
	MigrationVersion int64
}

func Open(t *testing.T) Database {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil || parsedURL.Scheme == "" {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}
	schema := "keebhub_test_" + hex.EncodeToString(suffix)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	basePool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if _, err := basePool.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		basePool.Close()
		t.Fatalf("create test schema: %v", err)
	}

	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	isolatedURL := parsedURL.String()
	version, err := migrations.Up(ctx, isolatedURL, migrationsDirectory(t))
	if err != nil {
		_, _ = basePool.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		basePool.Close()
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := database.NewPool(ctx, config.Config{
		DatabaseURL:       isolatedURL,
		DBMaxConns:        8,
		DBMinConns:        0,
		DBMaxConnLifetime: 30 * time.Minute,
		DBMaxConnIdleTime: 5 * time.Minute,
	})
	if err != nil {
		_, _ = basePool.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		basePool.Close()
		t.Fatalf("open isolated pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := basePool.Exec(cleanupCtx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		basePool.Close()
	})

	return Database{Pool: pool, MigrationVersion: version}
}

func migrationsDirectory(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test database source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	directory := filepath.Join(root, "db", "migrations")
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("find migration directory: %v", err)
	}
	return directory
}
