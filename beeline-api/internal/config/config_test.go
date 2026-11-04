package config

import (
	"log/slog"
	"testing"
)

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(lookupFrom(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 || cfg.Env != EnvDevelopment || cfg.LogLevel != slog.LevelInfo || cfg.MigrateOnStart {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.IntentsBaseURL != "https://1click.chaindefuser.com" {
		t.Fatalf("unexpected intents base URL %q", cfg.IntentsBaseURL)
	}
}

func TestLoadParsesOrigins(t *testing.T) {
	cfg, err := Load(lookupFrom(map[string]string{
		"DATABASE_URL":         "postgres://x",
		"CORS_ALLOWED_ORIGINS": " http://a.test , ,http://b.test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[1] != "http://b.test" {
		t.Fatalf("unexpected origins: %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]map[string]string{
		"missing database":   {},
		"bad port":           {"DATABASE_URL": "x", "PORT": "99999"},
		"bad env":            {"DATABASE_URL": "x", "APP_ENV": "staging"},
		"bad log level":      {"DATABASE_URL": "x", "LOG_LEVEL": "loud"},
		"wildcard cors prod": {"DATABASE_URL": "x", "APP_ENV": "production", "CORS_ALLOWED_ORIGINS": "*"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(lookupFrom(env)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
