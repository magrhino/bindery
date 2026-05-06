// telemetry-server is a tiny HTTP service that counts active Bindery installs.
// It accepts anonymous pings from Bindery instances and returns the latest
// published version so clients can surface an update badge.
//
// Endpoints:
//
//	POST /api/ping   — upsert install record, return latest version
//	GET  /api/stats  — active/total counts + version breakdown (token-gated)
//	GET  /health     — liveness probe
package main

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type server struct {
	db            *sql.DB
	latestVersion string
	statsToken    string
}

type pingRequest struct {
	InstallID string `json:"install_id"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type pingResponse struct {
	LatestVersion string `json:"latest_version"`
}

type statsResponse struct {
	Active30d int            `json:"active_30d"`
	Total     int            `json:"total"`
	Versions  map[string]int `json:"versions"`
}

func main() {
	dbPath := env("DB_PATH", "/data/telemetry.db")
	addr := env("ADDR", ":8080")
	latestVersion := env("LATEST_VERSION", "v1.4.1")
	statsToken := env("STATS_TOKEN", "")

	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS installs (
		install_id  TEXT PRIMARY KEY,
		version     TEXT NOT NULL,
		os          TEXT NOT NULL,
		arch        TEXT NOT NULL,
		first_seen  DATETIME NOT NULL,
		last_seen   DATETIME NOT NULL
	)`); err != nil {
		slog.Error("migrate db", "error", err)
		os.Exit(1)
	}

	s := &server{db: db, latestVersion: latestVersion, statsToken: statsToken}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/ping", s.handlePing)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("telemetry-server starting", "addr", addr, "latest", latestVersion)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
}

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
	var req pingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.InstallID == "" {
		http.Error(w, "install_id required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(r.Context(), `
		INSERT INTO installs (install_id, version, os, arch, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(install_id) DO UPDATE SET
			version   = excluded.version,
			os        = excluded.os,
			arch      = excluded.arch,
			last_seen = excluded.last_seen
	`, req.InstallID, req.Version, req.OS, req.Arch, now, now)
	if err != nil {
		slog.Warn("upsert install", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("ping", "id", req.InstallID[:min(8, len(req.InstallID))], "version", req.Version, "os", req.OS, "arch", req.Arch)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pingResponse{LatestVersion: s.latestVersion})
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.statsToken != "" {
		tok := r.Header.Get("Authorization")
		if tok != "Bearer "+s.statsToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)

	var active, total int
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM installs WHERE last_seen >= ?`, cutoff,
	).Scan(&active); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM installs`,
	).Scan(&total); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows, err := s.db.QueryContext(r.Context(),
		`SELECT version, COUNT(*) FROM installs WHERE last_seen >= ? GROUP BY version ORDER BY COUNT(*) DESC`, cutoff)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	versions := make(map[string]int)
	for rows.Next() {
		var ver string
		var count int
		if err := rows.Scan(&ver, &count); err == nil {
			versions[ver] = count
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statsResponse{
		Active30d: active,
		Total:     total,
		Versions:  versions,
	})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
