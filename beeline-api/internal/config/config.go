// Package config loads API configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

type Config struct {
	Port               int
	Env                Env
	LogLevel           slog.Level
	DatabaseURL        string
	CORSAllowedOrigins []string
	MigrateOnStart     bool

	IntentsBaseURL string
	IntentsAPIKey  string
}

// Load reads configuration from the given lookup function (os.LookupEnv in production).
func Load(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}

	var errs []error
	cfg := Config{
		Env:            Env(get("APP_ENV", string(EnvDevelopment))),
		DatabaseURL:    get("DATABASE_URL", ""),
		IntentsBaseURL: get("NEAR_INTENTS_API_BASE_URL", "https://1click.chaindefuser.com"),
		IntentsAPIKey:  get("NEAR_INTENTS_API_KEY", ""),
	}

	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env))
	}

	port, err := strconv.Atoi(get("PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be a valid port number"))
	}
	cfg.Port = port

	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	migrate, err := strconv.ParseBool(get("MIGRATE_ON_START", "false"))
	if err != nil {
		errs = append(errs, fmt.Errorf("MIGRATE_ON_START must be true or false"))
	}
	cfg.MigrateOnStart = migrate

	for _, o := range strings.Split(get("CORS_ALLOWED_ORIGINS", ""), ",") {
		if o = strings.TrimSpace(o); o != "" {
			if o == "*" && cfg.Env == EnvProduction {
				errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS must not be * in production"))
			}
			cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
		}
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	return cfg, errors.Join(errs...)
}

// FromEnv loads configuration from the process environment.
func FromEnv() (Config, error) { return Load(os.LookupEnv) }
