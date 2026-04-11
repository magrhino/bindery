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
	"github.com/vavallee/bindery/internal/indexer"
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
		"port", cfg.Port,
	)

	// Database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Repos
	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	indexerRepo := db.NewIndexerRepo(database)
	dlClientRepo := db.NewDownloadClientRepo(database)
	downloadRepo := db.NewDownloadRepo(database)
	settingsRepo := db.NewSettingsRepo(database)

	// Metadata providers
	olClient := openlibrary.New()
	var enrichers []metadata.Provider
	if setting, _ := settingsRepo.Get(nil, "google_books_api_key"); setting != nil && setting.Value != "" {
		enrichers = append(enrichers, googlebooks.New(setting.Value))
		slog.Info("google books enrichment enabled")
	}
	metaAgg := metadata.NewAggregator(olClient, enrichers...)

	// Indexer searcher
	idxSearcher := indexer.NewSearcher()

	// API handlers
	searchHandler := api.NewSearchHandler(metaAgg)
	authorHandler := api.NewAuthorHandler(authorRepo, bookRepo, metaAgg)
	bookHandler := api.NewBookHandler(bookRepo)
	indexerHandler := api.NewIndexerHandler(indexerRepo, bookRepo, idxSearcher)
	dlClientHandler := api.NewDownloadClientHandler(dlClientRepo)
	queueHandler := api.NewQueueHandler(downloadRepo, dlClientRepo)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Route("/api/v1", func(r chi.Router) {
		// System
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})
		r.Get("/system/status", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"` + version + `","commit":"` + commit + `","buildDate":"` + date + `"}`))
		})

		// Metadata search
		r.Get("/search/author", searchHandler.SearchAuthors)
		r.Get("/search/book", searchHandler.SearchBooks)
		r.Get("/book/lookup", searchHandler.LookupByISBN)

		// Authors
		r.Get("/author", authorHandler.List)
		r.Post("/author", authorHandler.Create)
		r.Get("/author/{id}", authorHandler.Get)
		r.Put("/author/{id}", authorHandler.Update)
		r.Delete("/author/{id}", authorHandler.Delete)
		r.Post("/author/{id}/refresh", authorHandler.Refresh)

		// Books
		r.Get("/book", bookHandler.List)
		r.Get("/book/{id}", bookHandler.Get)
		r.Put("/book/{id}", bookHandler.Update)
		r.Delete("/book/{id}", bookHandler.Delete)
		r.Post("/book/{id}/search", indexerHandler.SearchBook)

		// Wanted
		r.Get("/wanted/missing", bookHandler.ListWanted)

		// Indexers
		r.Get("/indexer", indexerHandler.List)
		r.Post("/indexer", indexerHandler.Create)
		r.Get("/indexer/{id}", indexerHandler.Get)
		r.Put("/indexer/{id}", indexerHandler.Update)
		r.Delete("/indexer/{id}", indexerHandler.Delete)
		r.Post("/indexer/{id}/test", indexerHandler.Test)
		r.Get("/indexer/search", indexerHandler.SearchQuery)

		// Download clients
		r.Get("/downloadclient", dlClientHandler.List)
		r.Post("/downloadclient", dlClientHandler.Create)
		r.Get("/downloadclient/{id}", dlClientHandler.Get)
		r.Put("/downloadclient/{id}", dlClientHandler.Update)
		r.Delete("/downloadclient/{id}", dlClientHandler.Delete)
		r.Post("/downloadclient/{id}/test", dlClientHandler.Test)

		// Queue
		r.Get("/queue", queueHandler.List)
		r.Post("/queue/grab", queueHandler.Grab)
		r.Delete("/queue/{id}", queueHandler.Delete)
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
