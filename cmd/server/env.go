package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const defaultEnvironmentFile = ".env"

// loadDevelopmentEnv loads local defaults without replacing process environment values.
// Production deployments must provide configuration through their runtime environment.
func loadDevelopmentEnv() error {
	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment != "" && environment != "development" {
		return nil
	}
	return loadEnvironmentFile(defaultEnvironmentFile)
}

func loadEnvironmentFile(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}
