package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// packImportFixture seeds an audiobook book and a download whose release title
// is the four-book pack from #2276, with two audiobook files on disk.
func packImportFixture(t *testing.T, releaseTitle, bookTitle string) (
	*Scanner, *models.Download, string, func() *models.Download,
) {
	t.Helper()
	libraryDir := t.TempDir()
	s, dl, dlRepo, bookRepo, ctx := dataLossFixture(t, libraryDir, "copy")

	book, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	book.MediaType = models.MediaTypeAudiobook
	book.Title = bookTitle
	if err := bookRepo.Update(ctx, book); err != nil {
		t.Fatal(err)
	}
	// Production hands tryImportInternal the download row it loaded, whose
	// Title is the release name. There is no repo setter for it, so the
	// in-memory field is set directly; nothing under test reads it back from
	// the row.
	dl.Title = releaseTitle

	downloadPath := t.TempDir()
	for _, name := range []string{"book1-track01.mp3", "book2-track01.mp3"} {
		if err := os.WriteFile(filepath.Join(downloadPath, name), []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reload := func() *models.Download {
		got, err := dlRepo.GetByID(ctx, dl.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	return s, dl, downloadPath, reload
}

// TestImport_BlocksMultiBookPack is the importer half of #2276. The decision
// engine stops a pack being auto-selected, but one can still reach the
// importer: hand-picked from interactive search, grabbed by an older version,
// or already queued across the upgrade. It must be refused before anything is
// written, not placed wholesale into one book's folder.
func TestImport_BlocksMultiBookPack(t *testing.T) {
	const pack = "Red Rising Series - Books 1 - 4 by Pierce Brown [ENG / M4B MP3] [VIP]"
	s, dl, downloadPath, reload := packImportFixture(t, pack, "Red Rising")

	s.tryImportInternal(t.Context(), dl, downloadPath, "", "", "", nil, nil)

	got := reload()
	if got.Status != models.StateImportBlocked {
		t.Fatalf("status = %q, want %q", got.Status, models.StateImportBlocked)
	}
	if !strings.Contains(got.ErrorMessage, "Books 1 - 4") {
		t.Errorf("message %q does not quote the words it judged", got.ErrorMessage)
	}
	if !strings.Contains(got.ErrorMessage, "Red Rising") {
		t.Errorf("message %q does not name the book it was linked to", got.ErrorMessage)
	}

	// Nothing may have been placed.
	dest, err := s.renamer.AudiobookDestDir(s.audiobookDir, &models.Author{Name: "Author A"}, &models.Book{Title: "Red Rising"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("destination %q was created for a blocked pack", dest)
	}
	// The import path is recorded so manual import can still place the files.
	if got.ImportPath == "" {
		t.Error("import path was not recorded, so manual import has nothing to work from")
	}
}

// TestImport_SingleBookReleaseUnaffected pins that an ordinary release still
// imports. A false positive in this guard silently stops a book importing.
func TestImport_SingleBookReleaseUnaffected(t *testing.T) {
	s, dl, downloadPath, reload := packImportFixture(t, "Red Rising - Pierce Brown [M4B]", "Red Rising")

	s.tryImportInternal(t.Context(), dl, downloadPath, "", "", "", nil, nil)

	if got := reload(); got.Status == models.StateImportBlocked {
		t.Fatalf("a single-book release was blocked: %s", got.ErrorMessage)
	}
}

// TestImport_PackAllowedWhenTheBookIsTheBundle covers the escape hatch: for
// someone tracking a box set as one book record, the pack IS the book and the
// one-destination problem does not arise.
func TestImport_PackAllowedWhenTheBookIsTheBundle(t *testing.T) {
	const pack = "Red Rising Series - Books 1 - 4 by Pierce Brown [ENG / M4B MP3] [VIP]"
	s, dl, downloadPath, reload := packImportFixture(t, pack, "Red Rising Books 1-4")

	s.tryImportInternal(t.Context(), dl, downloadPath, "", "", "", nil, nil)

	if got := reload(); got.Status == models.StateImportBlocked {
		t.Fatalf("pack blocked for a book that is itself the bundle: %s", got.ErrorMessage)
	}
}

// TestImport_PackAllowedWithExplicitFormatHint pins the manual-import
// override, on the same principle as the video guard: a human driving manual
// import has already declared what these files are.
func TestImport_PackAllowedWithExplicitFormatHint(t *testing.T) {
	const pack = "Red Rising Series - Books 1 - 4 by Pierce Brown [ENG / M4B MP3] [VIP]"
	s, dl, downloadPath, reload := packImportFixture(t, pack, "Red Rising")

	s.tryImportInternal(t.Context(), dl, downloadPath, "", "", models.MediaTypeAudiobook, nil, nil)

	if got := reload(); got.Status == models.StateImportBlocked {
		t.Fatalf("explicit manual import was blocked: %s", got.ErrorMessage)
	}
}
