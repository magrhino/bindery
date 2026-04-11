package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()

	if cfg.Port != "8787" {
		t.Errorf("expected default port 8787, got %s", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got %s", cfg.LogLevel)
	}
	if cfg.DBPath != "/config/bindery.db" {
		t.Errorf("expected default db path /config/bindery.db, got %s", cfg.DBPath)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("BINDERY_PORT", "9999")
	os.Setenv("BINDERY_LOG_LEVEL", "debug")
	defer os.Unsetenv("BINDERY_PORT")
	defer os.Unsetenv("BINDERY_LOG_LEVEL")

	cfg := Load()

	if cfg.Port != "9999" {
		t.Errorf("expected port 9999, got %s", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
}
