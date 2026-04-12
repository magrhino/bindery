// Package config loads Bindery's runtime configuration from environment
// variables with sensible defaults.
package config

import "os"

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port        string
	DBPath      string
	DataDir     string
	LogLevel    string
	APIKey      string
	DownloadDir string
	LibraryDir  string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:        envOr("BINDERY_PORT", "8787"),
		DBPath:      envOr("BINDERY_DB_PATH", "/config/bindery.db"),
		DataDir:     envOr("BINDERY_DATA_DIR", "/config"),
		LogLevel:    envOr("BINDERY_LOG_LEVEL", "info"),
		APIKey:      envOr("BINDERY_API_KEY", ""),
		DownloadDir: envOr("BINDERY_DOWNLOAD_DIR", "/downloads"),
		LibraryDir:  envOr("BINDERY_LIBRARY_DIR", "/books"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
