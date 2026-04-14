package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// scannerFixture spins up an in-memory DB and wires a Scanner against it.
// All external clients (SABnzbd) stay nil — tests here exercise only the
// Scanner paths that don't hit the download client.
func scannerFixture(t *testing.T, libraryDir string) (*Scanner, *db.BookRepo, *db.AuthorRepo, context.Context) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	books := db.NewBookRepo(database)
	authors := db.NewAuthorRepo(database)
	history := db.NewHistoryRepo(database)
	downloads := db.NewDownloadRepo(database)
	clients := db.NewDownloadClientRepo(database)

	s := NewScanner(downloads, clients, books, authors, history, libraryDir, "", "", "", "")
	return s, books, authors, context.Background()
}

func TestNewScanner_AudiobookDirFallback(t *testing.T) {
	// When audiobookDir is empty the scanner falls back to libraryDir so
	// audiobook imports have a destination without extra configuration.
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	s := NewScanner(
		db.NewDownloadRepo(database),
		db.NewDownloadClientRepo(database),
		db.NewBookRepo(database),
		db.NewAuthorRepo(database),
		db.NewHistoryRepo(database),
		"/lib", "", "", "", "",
	)
	if s.audiobookDir != "/lib" {
		t.Errorf("expected audiobookDir to default to libraryDir, got %q", s.audiobookDir)
	}
	if s.renamer == nil || s.remapper == nil {
		t.Error("expected renamer and remapper to be wired")
	}
}

// TestScanLibrary_EmptyLibraryDir short-circuits before touching the DB.
func TestScanLibrary_EmptyLibraryDir(t *testing.T) {
	s, _, _, ctx := scannerFixture(t, "")
	s.ScanLibrary(ctx) // must not panic, must not read the DB
}

// TestScanLibrary_NoFiles walks an empty library dir and returns early.
func TestScanLibrary_NoFiles(t *testing.T) {
	libDir := t.TempDir()
	s, _, _, ctx := scannerFixture(t, libDir)
	s.ScanLibrary(ctx) // no panic, no error
}

// TestScanLibrary_ReconcilesMatchingBook puts an orphan .epub into the
// library that matches a "wanted" book by title; the scan should attach
// the file path to the book record.
func TestScanLibrary_ReconcilesMatchingBook(t *testing.T) {
	libDir := t.TempDir()
	// "Author Name - Dark Matter.epub" → parsed title "Author Name",
	// but the titleMatch heuristic takes significant words from both
	// sides, so we name the file just "Dark Matter.epub" to keep the
	// match unambiguous.
	epub := filepath.Join(libDir, "Dark Matter.epub")
	if err := os.WriteFile(epub, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, books, authors, ctx := scannerFixture(t, libDir)
	author := &models.Author{ForeignID: "OL1A", Name: "A", SortName: "A"}
	if err := authors.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	// "Dark Matter" vs parsed "Dark Matter" → overlap=2, minOverlap=2 → match.
	book := &models.Book{
		ForeignID: "OL1B", AuthorID: author.ID,
		Title: "Dark Matter", Status: models.BookStatusWanted,
	}
	if err := books.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	s.ScanLibrary(ctx)

	got, err := books.GetByID(ctx, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FilePath != epub {
		t.Errorf("expected book.FilePath to be set to %q, got %q", epub, got.FilePath)
	}
}

// TestScanLibrary_NonBookFilesIgnored — extensions not in bookExtensions
// (jpg, nfo, etc.) should not appear in the walked list.
func TestScanLibrary_NonBookFilesIgnored(t *testing.T) {
	libDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(libDir, "cover.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "info.nfo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _, _, ctx := scannerFixture(t, libDir)
	s.ScanLibrary(ctx) // no panic, nothing to reconcile
}

// TestCheckDownloads_NoEnabledClient returns silently when there's no
// download client to poll. Before v0.7 this hit a nil deref on the client.
func TestCheckDownloads_NoEnabledClient(t *testing.T) {
	s, _, _, ctx := scannerFixture(t, t.TempDir())
	s.CheckDownloads(ctx) // no panic, no DB writes
}
