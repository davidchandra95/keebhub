package main

import (
	"context"
	"fmt"
	"os"

	"github.com/davidchandra95/keebhub/internal/platform/config"
	"github.com/davidchandra95/keebhub/internal/platform/migrations"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "KeebHub migration failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	if len(arguments) != 1 {
		return fmt.Errorf("usage: migrate up|status")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	switch arguments[0] {
	case "up":
		version, err := migrations.Up(ctx, cfg.DatabaseURL, cfg.MigrationsDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "database migrated to version %d\n", version)
		return err
	case "status":
		statuses, err := migrations.Status(ctx, cfg.DatabaseURL, cfg.MigrationsDir)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			if _, err := fmt.Fprintf(os.Stdout, "%06d %-10s %s\n", status.Source.Version, status.State, status.Source.Path); err != nil {
				return fmt.Errorf("write migration status: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown migration command %q; use up or status", arguments[0])
	}
}
