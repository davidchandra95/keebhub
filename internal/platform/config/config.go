// Package config loads and validates process configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDatabaseURL = "postgres://keebhub:keebhub@localhost:5432/keebhub?sslmode=disable"
	defaultBodyLimit   = int64(1 << 20)
)

// LookupEnv matches os.LookupEnv and makes configuration loading testable.
type LookupEnv func(string) (string, bool)

// Config is the complete process configuration parsed at startup.
type Config struct {
	Environment         string
	BaseURL             string
	HTTPAddr            string
	DatabaseURL         string
	LogLevel            string
	StaticDir           string
	MigrationsDir       string
	SessionCookieName   string
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
	ReadinessTimeout    time.Duration
	ShutdownTimeout     time.Duration
	HTTPBodyLimit       int64
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
}

// Load reads configuration from the current process environment.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads configuration through lookup and validates all values.
func LoadFrom(lookup LookupEnv) (Config, error) {
	cfg := Config{
		Environment:         valueOrDefault(lookup, "APP_ENV", "development"),
		BaseURL:             valueOrDefault(lookup, "APP_BASE_URL", "http://localhost:8080"),
		HTTPAddr:            valueOrDefault(lookup, "HTTP_ADDR", ":8080"),
		DatabaseURL:         valueOrDefault(lookup, "DATABASE_URL", defaultDatabaseURL),
		LogLevel:            valueOrDefault(lookup, "LOG_LEVEL", "info"),
		StaticDir:           valueOrDefault(lookup, "STATIC_DIR", "web/dist"),
		MigrationsDir:       valueOrDefault(lookup, "MIGRATIONS_DIR", "db/migrations"),
		SessionCookieName:   valueOrDefault(lookup, "SESSION_COOKIE_NAME", "keebhub_session"),
		DiscordClientID:     valueOrDefault(lookup, "DISCORD_CLIENT_ID", ""),
		DiscordClientSecret: valueOrDefault(lookup, "DISCORD_CLIENT_SECRET", ""),
		DiscordRedirectURI:  valueOrDefault(lookup, "DISCORD_REDIRECT_URI", ""),
	}

	var err error
	if cfg.ReadinessTimeout, err = durationValue(lookup, "READINESS_TIMEOUT", time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationValue(lookup, "SHUTDOWN_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnLifetime, err = durationValue(lookup, "DB_MAX_CONN_LIFETIME", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnIdleTime, err = durationValue(lookup, "DB_MAX_CONN_IDLE_TIME", 5*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.HTTPBodyLimit, err = int64Value(lookup, "HTTP_BODY_LIMIT_BYTES", defaultBodyLimit); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConns, err = int32Value(lookup, "DB_MAX_CONNS", 10); err != nil {
		return Config{}, err
	}
	if cfg.DBMinConns, err = int32Value(lookup, "DB_MIN_CONNS", 1); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks cross-field and production safety requirements.
func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return fmt.Errorf("APP_ENV must be development, test, or production")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return fmt.Errorf("APP_BASE_URL must be an absolute HTTP or HTTPS origin")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return fmt.Errorf("APP_BASE_URL must use http or https")
	}
	if baseURL.Path != "" && baseURL.Path != "/" {
		return fmt.Errorf("APP_BASE_URL must not contain a path")
	}
	if c.Environment == "production" && baseURL.Scheme != "https" {
		return fmt.Errorf("APP_BASE_URL must use https in production")
	}

	if strings.TrimSpace(c.HTTPAddr) == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL must not be empty")
	}
	if strings.TrimSpace(c.StaticDir) == "" {
		return fmt.Errorf("STATIC_DIR must not be empty")
	}
	if strings.TrimSpace(c.MigrationsDir) == "" {
		return fmt.Errorf("MIGRATIONS_DIR must not be empty")
	}
	if strings.TrimSpace(c.SessionCookieName) == "" {
		return fmt.Errorf("SESSION_COOKIE_NAME must not be empty")
	}
	if err := c.validateDiscord(baseURL); err != nil {
		return err
	}
	if c.ReadinessTimeout <= 0 {
		return fmt.Errorf("READINESS_TIMEOUT must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive")
	}
	if c.HTTPBodyLimit <= 0 {
		return fmt.Errorf("HTTP_BODY_LIMIT_BYTES must be positive")
	}
	if c.DBMinConns < 0 {
		return fmt.Errorf("DB_MIN_CONNS must not be negative")
	}
	if c.DBMaxConns <= 0 {
		return fmt.Errorf("DB_MAX_CONNS must be positive")
	}
	if c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}
	if c.DBMaxConnLifetime <= 0 {
		return fmt.Errorf("DB_MAX_CONN_LIFETIME must be positive")
	}
	if c.DBMaxConnIdleTime <= 0 {
		return fmt.Errorf("DB_MAX_CONN_IDLE_TIME must be positive")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}

	return nil
}

func (c Config) DiscordConfigured() bool {
	return c.DiscordClientID != "" && c.DiscordClientSecret != "" && c.DiscordRedirectURI != ""
}

func (c Config) validateDiscord(baseURL *url.URL) error {
	clientIDSet := c.DiscordClientID != ""
	clientSecretSet := c.DiscordClientSecret != ""
	if clientIDSet != clientSecretSet {
		return fmt.Errorf("DISCORD_CLIENT_ID and DISCORD_CLIENT_SECRET must be configured together")
	}
	if c.Environment == "production" && !c.DiscordConfigured() {
		return fmt.Errorf("discord OAuth configuration is required in production")
	}
	if !clientIDSet && c.DiscordRedirectURI == "" {
		return nil
	}
	if clientIDSet && c.DiscordRedirectURI == "" {
		return fmt.Errorf("DISCORD_REDIRECT_URI is required when Discord credentials are configured")
	}

	redirect, err := url.Parse(c.DiscordRedirectURI)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" || redirect.User != nil || redirect.RawQuery != "" || redirect.Fragment != "" {
		return fmt.Errorf("DISCORD_REDIRECT_URI must be an absolute callback URL")
	}
	if !strings.EqualFold(redirect.Scheme, baseURL.Scheme) || !strings.EqualFold(redirect.Host, baseURL.Host) || redirect.Path != "/auth/discord/callback" {
		return fmt.Errorf("DISCORD_REDIRECT_URI must equal APP_BASE_URL with /auth/discord/callback")
	}
	return nil
}

func valueOrDefault(lookup LookupEnv, key, fallback string) string {
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func durationValue(lookup LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return duration, nil
}

func int32Value(lookup LookupEnv, key string, fallback int32) (int32, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return int32(parsed), nil
}

func int64Value(lookup LookupEnv, key string, fallback int64) (int64, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
