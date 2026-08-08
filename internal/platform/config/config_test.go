package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/davidchandra95/keebhub/internal/platform/config"
)

func TestLoadFromDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFrom(emptyEnvironment)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want http://localhost:8080", cfg.BaseURL)
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL is empty")
	}
	if cfg.ReadinessTimeout != time.Second {
		t.Errorf("ReadinessTimeout = %s, want 1s", cfg.ReadinessTimeout)
	}
	if cfg.DBMaxConns != 10 || cfg.DBMinConns != 1 {
		t.Errorf("pool sizes = %d/%d, want 1/10", cfg.DBMinConns, cfg.DBMaxConns)
	}
}

func TestLoadFromOverrides(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"APP_BASE_URL":          "https://keebhub.example",
		"APP_ENV":               "production",
		"DATABASE_URL":          "postgres://app:secret@db/keebhub",
		"DB_MAX_CONNS":          "25",
		"DB_MIN_CONNS":          "5",
		"HTTP_ADDR":             ":9000",
		"LOG_LEVEL":             "debug",
		"READINESS_TIMEOUT":     "750ms",
		"SHUTDOWN_TIMEOUT":      "20s",
		"STATIC_DIR":            "/srv/web",
		"DB_MAX_CONN_LIFETIME":  "45m",
		"DB_MAX_CONN_IDLE_TIME": "10m",
		"SESSION_COOKIE_NAME":   "custom_session",
		"DISCORD_CLIENT_ID":     "client-id",
		"DISCORD_CLIENT_SECRET": "client-secret",
		"DISCORD_REDIRECT_URI":  "https://keebhub.example/auth/discord/callback",
	}

	cfg, err := config.LoadFrom(mapEnvironment(env))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.HTTPAddr != ":9000" || cfg.StaticDir != "/srv/web" {
		t.Errorf("unexpected HTTP/static values: %+v", cfg)
	}
	if cfg.DBMinConns != 5 || cfg.DBMaxConns != 25 {
		t.Errorf("pool sizes = %d/%d, want 5/25", cfg.DBMinConns, cfg.DBMaxConns)
	}
	if cfg.ReadinessTimeout != 750*time.Millisecond {
		t.Errorf("ReadinessTimeout = %s, want 750ms", cfg.ReadinessTimeout)
	}
	if !cfg.DiscordConfigured() {
		t.Error("DiscordConfigured() = false")
	}
}

func TestLoadFromRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "production requires HTTPS",
			env: map[string]string{
				"APP_ENV":      "production",
				"APP_BASE_URL": "http://keebhub.example",
			},
			want: "https",
		},
		{
			name: "invalid duration",
			env:  map[string]string{"READINESS_TIMEOUT": "eventually"},
			want: "READINESS_TIMEOUT",
		},
		{
			name: "invalid pool range",
			env: map[string]string{
				"DB_MIN_CONNS": "11",
				"DB_MAX_CONNS": "10",
			},
			want: "DB_MIN_CONNS",
		},
		{
			name: "invalid log level",
			env:  map[string]string{"LOG_LEVEL": "verbose"},
			want: "LOG_LEVEL",
		},
		{
			name: "partial Discord credentials",
			env:  map[string]string{"DISCORD_CLIENT_ID": "client-id"},
			want: "DISCORD_CLIENT_SECRET",
		},
		{
			name: "Discord redirect must match application origin",
			env: map[string]string{
				"DISCORD_CLIENT_ID":     "client-id",
				"DISCORD_CLIENT_SECRET": "client-secret",
				"DISCORD_REDIRECT_URI":  "https://evil.example/auth/discord/callback",
			},
			want: "APP_BASE_URL",
		},
		{
			name: "production requires Discord",
			env: map[string]string{
				"APP_ENV":      "production",
				"APP_BASE_URL": "https://keebhub.example",
			},
			want: "discord OAuth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.LoadFrom(mapEnvironment(tt.env))
			if err == nil {
				t.Fatal("LoadFrom() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func emptyEnvironment(string) (string, bool) {
	return "", false
}

func mapEnvironment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
