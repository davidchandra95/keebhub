package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironmentFile(t *testing.T) {
	t.Run("missing file is ignored", func(t *testing.T) {
		err := loadEnvironmentFile(filepath.Join(t.TempDir(), ".env"))
		if err != nil {
			t.Fatalf("loadEnvironmentFile() error = %v", err)
		}
	})

	t.Run("loads valid values", func(t *testing.T) {
		unsetEnvironment(t, "KEEBHUB_ENV_LOADER_VALUE")
		path := writeEnvironmentFile(t, "KEEBHUB_ENV_LOADER_VALUE=from-file\n")
		if err := loadEnvironmentFile(path); err != nil {
			t.Fatalf("loadEnvironmentFile() error = %v", err)
		}
		if got := os.Getenv("KEEBHUB_ENV_LOADER_VALUE"); got != "from-file" {
			t.Errorf("KEEBHUB_ENV_LOADER_VALUE = %q, want from-file", got)
		}
	})

	t.Run("explicit environment wins", func(t *testing.T) {
		t.Setenv("KEEBHUB_ENV_LOADER_VALUE", "from-process")
		path := writeEnvironmentFile(t, "KEEBHUB_ENV_LOADER_VALUE=from-file\n")
		if err := loadEnvironmentFile(path); err != nil {
			t.Fatalf("loadEnvironmentFile() error = %v", err)
		}
		if got := os.Getenv("KEEBHUB_ENV_LOADER_VALUE"); got != "from-process" {
			t.Errorf("KEEBHUB_ENV_LOADER_VALUE = %q, want from-process", got)
		}
	})

	t.Run("malformed file is rejected", func(t *testing.T) {
		path := writeEnvironmentFile(t, "KEEBHUB_ENV_LOADER_VALUE=\"unterminated\n")
		err := loadEnvironmentFile(path)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("loadEnvironmentFile() error = %v, want path in malformed-file error", err)
		}
	})
}

func TestLoadDevelopmentEnvSkipsNonDevelopmentEnvironments(t *testing.T) {
	for _, environment := range []string{"production", "test"} {
		t.Run(environment, func(t *testing.T) {
			t.Setenv("APP_ENV", environment)
			unsetEnvironment(t, "KEEBHUB_ENV_LOADER_VALUE")
			useTemporaryWorkingDirectory(t)

			if err := os.WriteFile(defaultEnvironmentFile, []byte("KEEBHUB_ENV_LOADER_VALUE=from-file\n"), 0o600); err != nil {
				t.Fatalf("write environment file: %v", err)
			}
			if err := loadDevelopmentEnv(); err != nil {
				t.Fatalf("loadDevelopmentEnv() error = %v", err)
			}
			if got := os.Getenv("KEEBHUB_ENV_LOADER_VALUE"); got != "" {
				t.Errorf("KEEBHUB_ENV_LOADER_VALUE = %q, want empty", got)
			}
		})
	}
}

func TestLoadDevelopmentEnvLoadsDefaultFile(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	unsetEnvironment(t, "KEEBHUB_ENV_LOADER_VALUE")
	useTemporaryWorkingDirectory(t)

	if err := os.WriteFile(defaultEnvironmentFile, []byte("KEEBHUB_ENV_LOADER_VALUE=from-file\n"), 0o600); err != nil {
		t.Fatalf("write environment file: %v", err)
	}
	if err := loadDevelopmentEnv(); err != nil {
		t.Fatalf("loadDevelopmentEnv() error = %v", err)
	}
	if got := os.Getenv("KEEBHUB_ENV_LOADER_VALUE"); got != "from-file" {
		t.Errorf("KEEBHUB_ENV_LOADER_VALUE = %q, want from-file", got)
	}
}

func writeEnvironmentFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write environment file: %v", err)
	}
	return path
}

func unsetEnvironment(t *testing.T, key string) {
	t.Helper()
	value, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		var err error
		if wasSet {
			err = os.Setenv(key, value)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Errorf("restore %s: %v", key, err)
		}
	})
}

func useTemporaryWorkingDirectory(t *testing.T) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
