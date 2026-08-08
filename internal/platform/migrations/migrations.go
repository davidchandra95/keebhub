// Package migrations runs versioned Goose migrations without package-level dialect state.
package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Up applies every pending migration and returns the resulting database version.
func Up(ctx context.Context, databaseURL, directory string) (version int64, returnErr error) {
	provider, err := newProvider(databaseURL, directory)
	if err != nil {
		return 0, err
	}
	defer func() {
		returnErr = errors.Join(returnErr, provider.Close())
	}()

	if _, err := provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	version, err = provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read migration version: %w", err)
	}
	return version, nil
}

// Status returns migration status ordered by version.
func Status(ctx context.Context, databaseURL, directory string) (statuses []*goose.MigrationStatus, returnErr error) {
	provider, err := newProvider(databaseURL, directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		returnErr = errors.Join(returnErr, provider.Close())
	}()

	statuses, err = provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("read migration status: %w", err)
	}
	return statuses, nil
}

func newProvider(databaseURL, directory string) (*goose.Provider, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open migration database: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS(directory),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		closeErr := db.Close()
		return nil, errors.Join(fmt.Errorf("create migration provider: %w", err), closeErr)
	}
	return provider, nil
}
