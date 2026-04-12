package scheduler

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/downloader/sabnzbd"
	"github.com/vavallee/bindery/internal/importer"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

// Scheduler runs background jobs on configurable intervals.
type Scheduler struct {
	cron     *cron.Cron
	scanner  *importer.Scanner
	searcher *indexer.Searcher
	meta     *metadata.Aggregator

	authors   *db.AuthorRepo
	books     *db.BookRepo
	indexers  *db.IndexerRepo
	downloads *db.DownloadRepo
	clients   *db.DownloadClientRepo
	settings  *db.SettingsRepo
}

// New creates a new scheduler.
func New(
	scanner *importer.Scanner,
	searcher *indexer.Searcher,
	meta *metadata.Aggregator,
	authors *db.AuthorRepo,
	books *db.BookRepo,
	indexers *db.IndexerRepo,
	downloads *db.DownloadRepo,
	clients *db.DownloadClientRepo,
	settings *db.SettingsRepo,
) *Scheduler {
	return &Scheduler{
		cron:      cron.New(cron.WithSeconds()),
		scanner:   scanner,
		searcher:  searcher,
		meta:      meta,
		authors:   authors,
		books:     books,
		indexers:  indexers,
		downloads: downloads,
		clients:   clients,
		settings:  settings,
	}
}

// Start registers and runs all background jobs.
func (s *Scheduler) Start() {
	// Check downloads every 60 seconds
	s.cron.AddFunc("@every 60s", func() {
		slog.Debug("job: check downloads")
		s.scanner.CheckDownloads(context.Background())
	})

	// Search for wanted books every 12 hours
	s.cron.AddFunc("@every 12h", func() {
		slog.Info("job: search wanted books")
		s.searchWanted()
	})

	// Refresh author metadata every 24 hours
	s.cron.AddFunc("@every 24h", func() {
		slog.Info("job: refresh metadata")
		s.refreshMetadata()
	})

	// Scan library every 6 hours
	s.cron.AddFunc("@every 6h", func() {
		slog.Info("job: scan library")
		s.scanner.ScanLibrary(context.Background())
	})

	s.cron.Start()
	slog.Info("scheduler started", "jobs", len(s.cron.Entries()))
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	slog.Info("scheduler stopped")
}

func (s *Scheduler) searchWanted() {
	ctx := context.Background()

	wanted, err := s.books.ListByStatus(ctx, models.BookStatusWanted)
	if err != nil {
		slog.Error("failed to list wanted books", "error", err)
		return
	}
	if len(wanted) == 0 {
		return
	}

	idxs, err := s.indexers.List(ctx)
	if err != nil {
		slog.Error("failed to list indexers", "error", err)
		return
	}

	client, err := s.clients.GetFirstEnabled(ctx)
	if err != nil || client == nil {
		slog.Debug("no download client available, skipping wanted search")
		return
	}

	// Read preferred language once for all books in this run
	lang := "en"
	if langSetting, _ := s.settings.Get(ctx, "search.preferredLanguage"); langSetting != nil {
		lang = langSetting.Value
	}

	for _, book := range wanted {
		results := s.searcher.SearchBook(ctx, idxs, book.Title, "")
		results = indexer.FilterByLanguage(results, lang)
		if len(results) == 0 {
			continue
		}

		// Auto-grab the top result
		best := results[0]
		slog.Info("auto-grabbing wanted book",
			"book", book.Title,
			"result", best.Title,
			"indexer", best.IndexerName,
			"size", best.Size,
		)

		dl := &models.Download{
			GUID:             best.GUID,
			BookID:           &book.ID,
			IndexerID:        &best.IndexerID,
			DownloadClientID: &client.ID,
			Title:            best.Title,
			NZBURL:           best.NZBURL,
			Size:             best.Size,
			Status:           models.DownloadStatusQueued,
			Protocol:         "usenet",
		}

		// Check for duplicate
		existing, _ := s.downloads.GetByGUID(ctx, best.GUID)
		if existing != nil {
			continue
		}

		if err := s.downloads.Create(ctx, dl); err != nil {
			slog.Error("failed to create download record", "error", err)
			continue
		}

		sab := sabnzbd.New(client.Host, client.Port, client.APIKey, client.UseSSL)
		resp, err := sab.AddURL(ctx, best.NZBURL, best.Title, client.Category, 0)
		if err != nil {
			slog.Error("failed to send to SABnzbd", "title", best.Title, "error", err)
			s.downloads.SetError(ctx, dl.ID, err.Error())
			continue
		}

		if len(resp.NzoIDs) > 0 {
			nzoID := resp.NzoIDs[0]
			s.downloads.SetNzoID(ctx, dl.ID, nzoID)
		}
		s.downloads.UpdateStatus(ctx, dl.ID, models.DownloadStatusDownloading)
		slog.Info("sent to SABnzbd", "title", best.Title)
	}
}

func (s *Scheduler) refreshMetadata() {
	ctx := context.Background()

	authors, err := s.authors.List(ctx)
	if err != nil {
		slog.Error("failed to list authors", "error", err)
		return
	}

	for _, author := range authors {
		if !author.Monitored {
			continue
		}

		updated, err := s.meta.GetAuthor(ctx, author.ForeignID)
		if err != nil {
			slog.Warn("failed to refresh author", "author", author.Name, "error", err)
			continue
		}

		// Update changed fields
		author.Description = updated.Description
		if updated.ImageURL != "" {
			author.ImageURL = updated.ImageURL
		}
		author.AverageRating = updated.AverageRating
		author.RatingsCount = updated.RatingsCount
		s.authors.Update(ctx, &author)

		slog.Debug("refreshed author", "author", author.Name)
	}
}
