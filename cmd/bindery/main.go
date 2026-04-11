package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vavallee/bindery/internal/api"
	"github.com/vavallee/bindery/internal/config"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/metadata/googlebooks"
	"github.com/vavallee/bindery/internal/metadata/openlibrary"
	"github.com/vavallee/bindery/internal/webui"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cfg := config.Load()

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	slog.Info("starting bindery",
		"version", version,
		"commit", commit,
		"date", date,
		"port", cfg.Port,
	)

	// Database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Metadata providers
	olClient := openlibrary.New()
	var enrichers []metadata.Provider

	// Google Books enricher (optional, needs API key from settings)
	settingsRepo := db.NewSettingsRepo(database)
	if setting, _ := settingsRepo.Get(nil, "google_books_api_key"); setting != nil && setting.Value != "" {
		enrichers = append(enrichers, googlebooks.New(setting.Value))
		slog.Info("google books enrichment enabled")
	}

	metaAgg := metadata.NewAggregator(olClient, enrichers...)
	searchHandler := api.NewSearchHandler(metaAgg)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})

		r.Get("/system/status", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"` + version + `","commit":"` + commit + `","buildDate":"` + date + `"}`))
		})

		// Metadata search (no DB writes, pure lookup)
		r.Get("/search/author", searchHandler.SearchAuthors)
		r.Get("/search/book", searchHandler.SearchBooks)
		r.Get("/book/lookup", searchHandler.LookupByISBN)
	})

	// Serve embedded frontend
	distFS, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		slog.Error("failed to load embedded frontend", "error", err)
		os.Exit(1)
	}
	fileServer := http.FileServer(http.FS(distFS))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:]
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(distFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	slog.Info("listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
