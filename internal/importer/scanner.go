package importer

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/downloader/sabnzbd"
	"github.com/vavallee/bindery/internal/models"
)

// Scanner checks for completed downloads and imports them into the library.
type Scanner struct {
	downloads  *db.DownloadRepo
	clients    *db.DownloadClientRepo
	books      *db.BookRepo
	authors    *db.AuthorRepo
	history    *db.HistoryRepo
	renamer    *Renamer
	libraryDir string
}

// NewScanner creates an import scanner.
func NewScanner(downloads *db.DownloadRepo, clients *db.DownloadClientRepo,
	books *db.BookRepo, authors *db.AuthorRepo, history *db.HistoryRepo, libraryDir, namingTemplate string) *Scanner {
	return &Scanner{
		downloads:  downloads,
		clients:    clients,
		books:      books,
		authors:    authors,
		history:    history,
		renamer:    NewRenamer(namingTemplate),
		libraryDir: libraryDir,
	}
}

// CheckDownloads polls SABnzbd for status changes and updates the local download records.
func (s *Scanner) CheckDownloads(ctx context.Context) {
	client, err := s.clients.GetFirstEnabled(ctx)
	if err != nil || client == nil {
		return
	}

	sab := sabnzbd.New(client.Host, client.Port, client.APIKey, client.UseSSL)

	// Check history for completed downloads (no category filter — match by NZO ID)
	history, err := sab.GetHistory(ctx, "", 50)
	if err != nil {
		slog.Debug("failed to fetch SABnzbd history", "error", err)
		return
	}

	for _, slot := range history.Slots {
		dl, err := s.downloads.GetByNzoID(ctx, slot.NzoID)
		if err != nil || dl == nil {
			continue
		}

		switch slot.Status {
		case "Completed":
			if dl.Status == models.DownloadStatusDownloading || dl.Status == models.DownloadStatusQueued {
				slog.Info("download completed", "title", dl.Title, "path", slot.Path)
				s.downloads.UpdateStatus(ctx, dl.ID, models.DownloadStatusCompleted)
				s.tryImport(ctx, dl, slot.Path)
			}
		case "Failed":
			if dl.Status != models.DownloadStatusFailed {
				slog.Warn("download failed", "title", dl.Title, "message", slot.FailMessage)
				s.downloads.SetError(ctx, dl.ID, slot.FailMessage)
				eventData, _ := json.Marshal(map[string]string{"message": slot.FailMessage})
				s.history.Create(ctx, &models.HistoryEvent{
					BookID:      dl.BookID,
					EventType:   models.HistoryEventDownloadFailed,
					SourceTitle: dl.Title,
					Data:        string(eventData),
				})
			}
		}
	}
}

// tryImport attempts to import a completed download into the library.
func (s *Scanner) tryImport(ctx context.Context, dl *models.Download, downloadPath string) {
	if s.libraryDir == "" {
		slog.Warn("no library directory configured, skipping import")
		return
	}

	// Find book files in the download path
	var bookFiles []string
	filepath.Walk(downloadPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if IsBookFile(path) {
			bookFiles = append(bookFiles, path)
		}
		return nil
	})

	if len(bookFiles) == 0 {
		slog.Warn("no book files found in download", "path", downloadPath)
		return
	}

	// Resolve the book and author for naming
	var book *models.Book
	var author *models.Author
	if dl.BookID != nil {
		book, _ = s.books.GetByID(ctx, *dl.BookID)
		if book != nil {
			author, _ = s.authors.GetByID(ctx, book.AuthorID)
		}
	}

	for _, srcFile := range bookFiles {
		if book == nil {
			// Try to match from filename
			parsed := ParseFilename(srcFile)
			slog.Info("unmatched import", "title", parsed.Title, "author", parsed.Author, "file", srcFile)
			continue
		}

		destPath := s.renamer.DestPath(s.libraryDir, author, book, srcFile)
		slog.Info("importing book", "src", srcFile, "dst", destPath)

		if err := MoveFile(srcFile, destPath); err != nil {
			slog.Error("failed to import", "src", srcFile, "error", err)
			continue
		}

		// Update book status and file path
		s.books.SetFilePath(ctx, book.ID, destPath)
		s.downloads.UpdateStatus(ctx, dl.ID, models.DownloadStatusImported)
		slog.Info("book imported", "title", book.Title, "path", destPath)

		eventData, _ := json.Marshal(map[string]string{"path": destPath})
		s.history.Create(ctx, &models.HistoryEvent{
			BookID:      dl.BookID,
			EventType:   models.HistoryEventBookImported,
			SourceTitle: dl.Title,
			Data:        string(eventData),
		})
	}
}

// ScanLibrary walks the library directory for book files not yet tracked in the database
// and reconciles found files with existing "wanted" book records.
func (s *Scanner) ScanLibrary(ctx context.Context) {
	if s.libraryDir == "" {
		return
	}

	// Collect all book files on disk
	var foundFiles []string
	filepath.Walk(s.libraryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if IsBookFile(path) {
			foundFiles = append(foundFiles, path)
		}
		return nil
	})

	slog.Info("library scan found files", "path", s.libraryDir, "count", len(foundFiles))

	if len(foundFiles) == 0 {
		return
	}

	// Load all books from the DB for reconciliation
	allBooks, err := s.books.List(ctx)
	if err != nil {
		slog.Error("library scan: failed to list books", "error", err)
		return
	}

	// Build a set of file paths already tracked in the DB
	trackedPaths := make(map[string]bool, len(allBooks))
	for _, b := range allBooks {
		if b.FilePath != "" {
			trackedPaths[b.FilePath] = true
		}
	}

	var reconciled, unmatched int
	for _, path := range foundFiles {
		// Skip files already tracked
		if trackedPaths[path] {
			continue
		}

		// Parse the filename to extract title/author hints
		parsed := ParseFilename(path)

		// Search existing books for a title match (case-insensitive substring)
		matched := false
		if parsed.Title != "" {
			lowerTitle := strings.ToLower(parsed.Title)
			for _, b := range allBooks {
				if b.Status == models.BookStatusWanted &&
					strings.Contains(strings.ToLower(b.Title), lowerTitle) {
					// Match found — update file path and status
					if err := s.books.SetFilePath(ctx, b.ID, path); err != nil {
						slog.Error("library scan: failed to update book", "id", b.ID, "error", err)
						continue
					}
					slog.Info("library scan: reconciled book", "title", b.Title, "path", path)
					trackedPaths[path] = true
					reconciled++
					matched = true
					break
				}
			}
		}

		if !matched {
			slog.Debug("library scan: unmatched file", "path", path, "parsedTitle", parsed.Title, "parsedAuthor", parsed.Author)
			unmatched++
		}
	}

	slog.Info("library scan complete", "path", s.libraryDir, "bookFiles", len(foundFiles),
		"reconciled", reconciled, "unmatched", unmatched)
}
