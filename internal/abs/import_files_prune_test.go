package abs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// pruneFixture creates a book owning the given (format, path) rows.
func pruneFixture(t *testing.T, rows ...[2]string) (*Importer, int64, func() []models.BookFile) {
	t.Helper()
	importer, authorRepo, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	ctx := context.Background()

	author := &models.Author{
		ForeignID: "abs-prune-a", Name: "Prune Author", SortName: "Author, Prune",
		MetadataProvider: "abs", Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatalf("author create: %v", err)
	}
	book := &models.Book{
		ForeignID: "abs-prune-b", AuthorID: author.ID, Title: "Prune Book",
		SortTitle: "prune book", Status: "imported", Genres: []string{},
		MetadataProvider: "abs", Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatalf("book create: %v", err)
	}
	for _, r := range rows {
		if err := bookRepo.AddBookFile(ctx, book.ID, r[0], r[1]); err != nil {
			t.Fatalf("add book file %q: %v", r[1], err)
		}
	}
	list := func() []models.BookFile {
		files, err := bookRepo.ListFiles(ctx, book.ID)
		if err != nil {
			t.Fatalf("list files: %v", err)
		}
		return files
	}
	return importer, book.ID, list
}

// TestPruneVanishedFormatPaths covers #1692: SetFormatFilePath appends, so an
// ABS re-import after a file move left the book owning both the new row and the
// dead old one, and nothing ever cleaned it up (a library scan skips any book
// that still resolves at least one path).
func TestPruneVanishedFormatPaths(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Romantasy", "Book.epub")
	stale := filepath.Join(dir, "Fantasy", "Book.epub")
	if err := os.MkdirAll(filepath.Dir(current), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	importer, bookID, list := pruneFixture(t,
		[2]string{models.MediaTypeEbook, stale},
		[2]string{models.MediaTypeEbook, current},
	)
	importer.pruneVanishedFormatPaths(context.Background(), bookID, models.MediaTypeEbook, current)

	files := list()
	if len(files) != 1 {
		t.Fatalf("expected the dead row pruned, got %d rows: %+v", len(files), files)
	}
	if files[0].Path != current {
		t.Errorf("remaining row = %q, want %q", files[0].Path, current)
	}
}

// TestPruneVanishedFormatPaths_KeepsExistingFiles is the core safety guard: a
// book legitimately holding two copies of a format keeps both. Only rows whose
// file is definitely gone are pruned.
func TestPruneVanishedFormatPaths_KeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "A.epub")
	b := filepath.Join(dir, "B.epub")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	importer, bookID, list := pruneFixture(t,
		[2]string{models.MediaTypeEbook, a},
		[2]string{models.MediaTypeEbook, b},
	)
	importer.pruneVanishedFormatPaths(context.Background(), bookID, models.MediaTypeEbook, b)

	if files := list(); len(files) != 2 {
		t.Errorf("both files still exist on disk; neither row may be pruned, got %d: %+v", len(files), files)
	}
}

// TestPruneVanishedFormatPaths_OtherFormatsUntouched pins the format scoping.
// A missing audiobook folder must survive an ebook reconciliation — the two
// formats move independently, and an ebook import knows nothing about whether
// the audiobook is legitimately absent right now.
func TestPruneVanishedFormatPaths_OtherFormatsUntouched(t *testing.T) {
	dir := t.TempDir()
	currentEbook := filepath.Join(dir, "New", "Book.epub")
	staleEbook := filepath.Join(dir, "Old", "Book.epub")
	missingAudio := filepath.Join(dir, "Old", "Book Audio")
	if err := os.MkdirAll(filepath.Dir(currentEbook), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentEbook, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	importer, bookID, list := pruneFixture(t,
		[2]string{models.MediaTypeEbook, staleEbook},
		[2]string{models.MediaTypeEbook, currentEbook},
		[2]string{models.MediaTypeAudiobook, missingAudio},
	)
	importer.pruneVanishedFormatPaths(context.Background(), bookID, models.MediaTypeEbook, currentEbook)

	files := list()
	if len(files) != 2 {
		t.Fatalf("expected 2 rows (current ebook + untouched audiobook), got %d: %+v", len(files), files)
	}
	var sawAudio bool
	for _, f := range files {
		if f.Format == models.MediaTypeAudiobook && f.Path == missingAudio {
			sawAudio = true
		}
		if f.Path == staleEbook {
			t.Error("the stale ebook row should have been pruned")
		}
	}
	if !sawAudio {
		t.Error("the audiobook row must survive an ebook-scoped prune")
	}
}

// TestPruneVanishedFormatPaths_KeepPathNeverPruned guards the case where the
// just-written path is itself unreadable by the time the prune runs. It is the
// row the importer just committed, so it is never a candidate.
func TestPruneVanishedFormatPaths_KeepPathNeverPruned(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "Vanished.epub")
	importer, bookID, list := pruneFixture(t, [2]string{models.MediaTypeEbook, gone})
	importer.pruneVanishedFormatPaths(context.Background(), bookID, models.MediaTypeEbook, gone)

	if files := list(); len(files) != 1 {
		t.Errorf("the freshly written row must never be pruned, got %d rows: %+v", len(files), files)
	}
}

// TestPruneVanishedFormatPaths_NonENOENTStatErrorKeepsRow is the guard that
// makes this safe to run on every import.
//
// "The file is missing" is not the same as "the file is gone". A stalled or
// unmounted network share, a permission change, or an I/O error all make paths
// unreadable en masse, and pruning on any of those would clear a library's file
// rows wholesale. Only a definite ENOENT prunes.
//
// The unreadable path here has a regular file as its parent directory, so the
// stat fails with ENOTDIR — an error that is emphatically not os.IsNotExist,
// without needing root or a real mount to reproduce.
func TestPruneVanishedFormatPaths_NonENOENTStatErrorKeepsRow(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Current.epub")
	if err := os.WriteFile(current, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	notADir := filepath.Join(dir, "notadir")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(notADir, "Book.epub")
	if _, err := os.Stat(unreadable); os.IsNotExist(err) {
		t.Fatalf("precondition: stat should fail with something other than ENOENT, got %v", err)
	}

	importer, bookID, list := pruneFixture(t,
		[2]string{models.MediaTypeEbook, unreadable},
		[2]string{models.MediaTypeEbook, current},
	)
	importer.pruneVanishedFormatPaths(context.Background(), bookID, models.MediaTypeEbook, current)

	files := list()
	if len(files) != 2 {
		t.Fatalf("a non-ENOENT stat error must leave the row alone, got %d rows: %+v", len(files), files)
	}
}
