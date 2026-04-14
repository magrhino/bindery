package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	// DBPath/DataDir are platform-dependent as of #7. CI runs on linux so
	// here we only assert the linux invariant; per-platform coverage lives
	// in TestDefaultDBPath / TestDefaultDataDir below.
	if runtime.GOOS == "linux" {
		if cfg.DBPath != "/config/bindery.db" {
			t.Errorf("expected default db path /config/bindery.db, got %s", cfg.DBPath)
		}
		if cfg.DataDir != "/config" {
			t.Errorf("expected default data dir /config, got %s", cfg.DataDir)
		}
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("BINDERY_PORT", "9999")
	t.Setenv("BINDERY_LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.Port != "9999" {
		t.Errorf("expected port 9999, got %s", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
}

// TestDefaultDBPath_Linux pins the linux default at `/config/bindery.db`.
// Existing Docker / Helm / bare-metal installs mount `/config`, so a drift
// here breaks every deployed user; guard it explicitly.
func TestDefaultDBPath_Linux(t *testing.T) {
	got := defaultDBPath("linux", func() (string, error) { return "/home/user/.config", nil })
	if got != "/config/bindery.db" {
		t.Errorf("linux default must be /config/bindery.db (unchanged), got %s", got)
	}
}

// TestDefaultDBPath_Windows is the #7 regression guard — double-clicking
// bindery.exe was hitting os.MkdirAll("/config", …) and exiting before the
// user could read the error. Default now routes through UserConfigDir
// (%APPDATA% on Windows).
func TestDefaultDBPath_Windows(t *testing.T) {
	got := defaultDBPath("windows", func() (string, error) { return `C:\Users\u\AppData\Roaming`, nil })
	want := filepath.Join(`C:\Users\u\AppData\Roaming`, "Bindery", "bindery.db")
	if got != want {
		t.Errorf("windows default: want %s, got %s", want, got)
	}
}

func TestDefaultDBPath_Darwin(t *testing.T) {
	got := defaultDBPath("darwin", func() (string, error) { return "/Users/u/Library/Application Support", nil })
	want := filepath.Join("/Users/u/Library/Application Support", "Bindery", "bindery.db")
	if got != want {
		t.Errorf("darwin default: want %s, got %s", want, got)
	}
}

// TestDefaultDBPath_FallsBackOnDirError — if UserConfigDir errors on a
// non-linux platform, fall back to the linux default. db.Open's preflight
// will still surface a clear "not writable" message rather than a silent
// SQLite error.
func TestDefaultDBPath_FallsBackOnDirError(t *testing.T) {
	got := defaultDBPath("windows", func() (string, error) { return "", errors.New("no APPDATA") })
	if got != "/config/bindery.db" {
		t.Errorf("expected fallback /config/bindery.db on UserConfigDir error, got %s", got)
	}
}

func TestDefaultDataDir_Linux(t *testing.T) {
	got := defaultDataDir("linux", func() (string, error) { return "/home/user/.config", nil })
	if got != "/config" {
		t.Errorf("linux default must be /config, got %s", got)
	}
}

func TestDefaultDataDir_Windows(t *testing.T) {
	got := defaultDataDir("windows", func() (string, error) { return `C:\Users\u\AppData\Roaming`, nil })
	want := filepath.Join(`C:\Users\u\AppData\Roaming`, "Bindery")
	if got != want {
		t.Errorf("windows default: want %s, got %s", want, got)
	}
	if !strings.HasSuffix(got, "Bindery") {
		t.Errorf("windows data dir should end in Bindery/: %s", got)
	}
}

func TestDefaultDataDir_FallsBackOnDirError(t *testing.T) {
	got := defaultDataDir("darwin", func() (string, error) { return "", os.ErrNotExist })
	if got != "/config" {
		t.Errorf("expected fallback /config on UserConfigDir error, got %s", got)
	}
}
