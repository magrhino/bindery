package db

// Regression tests for #2186: BookFileRepo.Add is INSERT OR IGNORE, so
// registering a file at a new location appends a row instead of replacing the
// old one. The derivation used to take the lowest id unconditionally, so a
// book whose folder had been renamed rendered the dead path forever while
// still reporting Imported.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// writeFileAt creates path and any missing parents.
func writeFileAt(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("book"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestRefreshBookStatus_MovedFileRendersLivePath is the issue repro: register
// a path, rename the folder on disk, register the new path, and the book must
// render the file that is actually there.
func TestRefreshBookStatus_MovedFileRendersLivePath(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	root := t.TempDir()
	oldDir := filepath.Join(root, "Old Folder")
	oldPath := writeFileAt(t, filepath.Join(oldDir, "Neuromancer.epub"))

	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, oldPath); err != nil {
		t.Fatalf("AddBookFile old path: %v", err)
	}

	// The user renames the folder on disk; the scan finds the file at its new
	// location and registers it.
	newDir := filepath.Join(root, "New Folder")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatalf("rename folder: %v", err)
	}
	newPath := filepath.Join(newDir, "Neuromancer.epub")
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, newPath); err != nil {
		t.Fatalf("AddBookFile new path: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != newPath {
		t.Errorf("EbookFilePath = %q, want the live path %q", got.EbookFilePath, newPath)
	}
	if got.FilePath != newPath {
		t.Errorf("legacy FilePath = %q, want the live path %q", got.FilePath, newPath)
	}
	if _, err := os.Stat(got.EbookFilePath); err != nil {
		t.Errorf("rendered path does not exist on disk: %v", err)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported", got.Status)
	}

	// The stale row is kept, not deleted: it stays visible on the book's Files
	// tab so the user can deregister it.
	files, err := repo.ListFiles(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want both rows kept, got %d", len(files))
	}
}

// TestRefreshBookStatus_AllFilesMissingKeepsRenderedPath is the hazard test for
// this fix: an unmounted volume makes every path under it vanish at once. The
// derivation must fall back to the pre-fix answer rather than blanking the
// field, so the book keeps its path, keeps reporting Imported, and keeps every
// row. Nothing here may look like "this book lost its files".
func TestRefreshBookStatus_AllFilesMissingKeepsRenderedPath(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	root := t.TempDir()
	mount := filepath.Join(root, "mnt", "nas")
	first := writeFileAt(t, filepath.Join(mount, "Neuromancer.epub"))
	second := writeFileAt(t, filepath.Join(mount, "Neuromancer.mobi"))

	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, first); err != nil {
		t.Fatalf("AddBookFile first: %v", err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, second); err != nil {
		t.Fatalf("AddBookFile second: %v", err)
	}

	// The share drops: every tracked path under it stats as ENOENT at once.
	if err := os.RemoveAll(filepath.Join(root, "mnt")); err != nil {
		t.Fatalf("remove mount: %v", err)
	}

	// Something touches the book and forces a re-derivation. Re-registering a
	// path the book already tracks is the real-world shape of that: the row is
	// a no-op (INSERT OR IGNORE) but the status refresh still runs.
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, first); err != nil {
		t.Fatalf("re-register first: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != first {
		t.Errorf("EbookFilePath = %q, want the unchanged %q", got.EbookFilePath, first)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported (a missing mount must not flip the book back to wanted)", got.Status)
	}
	files, err := repo.ListFiles(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 rows kept after the mount vanished, got %d", len(files))
	}
}

// TestRefreshBookStatus_UnprovableAbsenceKeepsRow pins the asymmetry the fix
// relies on: only a definite "does not exist" demotes a row. Here the first
// row's parent is a regular file, so os.Stat returns ENOTDIR — "we cannot read
// this", not "this moved" — and the row must keep winning even though a later
// row resolves cleanly. This is what stops a stalled mount from silently
// re-pointing a library at the wrong rows.
func TestRefreshBookStatus_UnprovableAbsenceKeepsRow(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	root := t.TempDir()
	blocker := writeFileAt(t, filepath.Join(root, "not-a-dir"))
	unreadable := filepath.Join(blocker, "Neuromancer.epub")
	if _, err := os.Stat(unreadable); os.IsNotExist(err) {
		t.Skipf("platform reports ENOENT rather than ENOTDIR for %q", unreadable)
	}
	live := writeFileAt(t, filepath.Join(root, "Library", "Neuromancer.epub"))

	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, unreadable); err != nil {
		t.Fatalf("AddBookFile unreadable: %v", err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, live); err != nil {
		t.Fatalf("AddBookFile live: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != unreadable {
		t.Errorf("EbookFilePath = %q, want %q kept: an unreadable path is not a vanished one",
			got.EbookFilePath, unreadable)
	}
}

// TestBookColumns_FallsBackToBookFilesRow guards the read-path precedence
// change. Migration 028 backfilled an 'ebook' book_files row from the legacy
// untyped file_path column without filling ebook_file_path, so a book can have
// rows while the column is empty. Reading the column first must still fall
// through to the row for that shape.
func TestBookColumns_FallsBackToBookFilesRow(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)
	files := NewBookFileRepo(database)

	// Add the row directly, bypassing refreshBookStatus, so books.ebook_file_path
	// stays empty exactly as the 028 backfill leaves it.
	if err := files.Add(ctx, book.ID, models.MediaTypeEbook, "/legacy/Neuromancer.epub"); err != nil {
		t.Fatalf("files.Add: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != "/legacy/Neuromancer.epub" {
		t.Errorf("EbookFilePath = %q, want the book_files row to fill the empty column",
			got.EbookFilePath)
	}
}

func TestBookFilePathResolves(t *testing.T) {
	root := t.TempDir()
	present := writeFileAt(t, filepath.Join(root, "Neuromancer.epub"))

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"a file that exists", present, true},
		{"a directory that exists", root, true},
		{"a path that does not exist", filepath.Join(root, "gone.epub"), false},
		{"the empty path", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BookFilePathResolves(tc.path); got != tc.want {
				t.Errorf("BookFilePathResolves(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestMultiFileBookPaths_OnlyBooksWithSameFormatSiblings pins the sweep's
// candidate query: it must find the book that owns two rows of one format and
// skip the book whose two rows are one of each.
func TestMultiFileBookPaths_OnlyBooksWithSameFormatSiblings(t *testing.T) {
	database, author, single := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	dual := &models.Book{
		ForeignID: "OL88888W", AuthorID: author.ID, Title: "Dual", SortTitle: "Dual",
		Monitored: true, Status: models.BookStatusWanted, MediaType: models.MediaTypeBoth,
	}
	if err := repo.Create(ctx, dual); err != nil {
		t.Fatalf("create dual book: %v", err)
	}

	root := t.TempDir()
	// One book with two ebooks: a candidate.
	epub := writeFileAt(t, filepath.Join(root, "Neuromancer.epub"))
	mobi := writeFileAt(t, filepath.Join(root, "Neuromancer.mobi"))
	if err := repo.AddBookFile(ctx, single.ID, models.MediaTypeEbook, epub); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBookFile(ctx, single.ID, models.MediaTypeEbook, mobi); err != nil {
		t.Fatal(err)
	}
	// One book with an ebook and an audiobook: not a candidate, because
	// neither format has a sibling that could have outranked it.
	other := writeFileAt(t, filepath.Join(root, "Dual.epub"))
	audio := writeFileAt(t, filepath.Join(root, "Dual.m4b"))
	if err := repo.AddBookFile(ctx, dual.ID, models.MediaTypeEbook, other); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBookFile(ctx, dual.ID, models.MediaTypeAudiobook, audio); err != nil {
		t.Fatal(err)
	}

	got, err := repo.MultiFileBookPaths(ctx)
	if err != nil {
		t.Fatalf("MultiFileBookPaths: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d (%+v)", len(got), got)
	}
	if got[0].BookID != single.ID {
		t.Errorf("candidate BookID = %d, want %d", got[0].BookID, single.ID)
	}
	if got[0].EbookPath != epub {
		t.Errorf("candidate EbookPath = %q, want %q", got[0].EbookPath, epub)
	}
	if got[0].AudiobookPath != "" {
		t.Errorf("candidate AudiobookPath = %q, want empty", got[0].AudiobookPath)
	}
}

// TestRefreshBookStatus_ExportedEntryPointRederivesPaths covers the entry point
// the library scan's sweep calls: no file is registered or removed, the rows
// are untouched, and the book still moves onto the one that resolves.
func TestRefreshBookStatus_ExportedEntryPointRederivesPaths(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	root := t.TempDir()
	first := writeFileAt(t, filepath.Join(root, "Old", "Neuromancer.epub"))
	second := writeFileAt(t, filepath.Join(root, "New", "Neuromancer.epub"))
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, second); err != nil {
		t.Fatal(err)
	}
	// Both resolved at registration, so the older row won. That is the state a
	// library broken before this fix is left in.
	before, err := repo.GetByID(ctx, book.ID)
	if err != nil || before == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if before.EbookFilePath != first {
		t.Fatalf("fixture: EbookFilePath = %q, want %q", before.EbookFilePath, first)
	}

	if err := os.RemoveAll(filepath.Join(root, "Old")); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefreshBookStatus(ctx, book.ID); err != nil {
		t.Fatalf("RefreshBookStatus: %v", err)
	}

	after, err := repo.GetByID(ctx, book.ID)
	if err != nil || after == nil {
		t.Fatalf("GetByID after refresh: %v", err)
	}
	if after.EbookFilePath != second {
		t.Errorf("EbookFilePath = %q, want the live path %q", after.EbookFilePath, second)
	}
}
