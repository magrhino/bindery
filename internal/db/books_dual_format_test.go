package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// openTestDB opens an in-memory database and seeds one author + one book
// for use in dual-format tests. Returns the db, author, and book.
func openTestDB(t *testing.T) (*sql.DB, *models.Author, *models.Book) {
	t.Helper()
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	ctx := context.Background()

	authorRepo := NewAuthorRepo(database)
	author := &models.Author{
		ForeignID: "OL12345A",
		Name:      "Test Author",
		SortName:  "Author, Test",
		Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatalf("create author: %v", err)
	}

	bookRepo := NewBookRepo(database)
	book := &models.Book{
		ForeignID: "OL99999W",
		AuthorID:  author.ID,
		Title:     "Test Book",
		SortTitle: "Test Book",
		Monitored: true,
		Status:    models.BookStatusWanted,
		MediaType: models.MediaTypeEbook,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatalf("create book: %v", err)
	}

	t.Cleanup(func() { database.Close() })
	return database, author, book
}

func TestSetFormatFilePath_Ebook(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeEbook, "/lib/test.epub"); err != nil {
		t.Fatalf("SetFormatFilePath: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != "/lib/test.epub" {
		t.Errorf("EbookFilePath = %q, want /lib/test.epub", got.EbookFilePath)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported", got.Status)
	}
	// Legacy file_path should be in sync.
	if got.FilePath != "/lib/test.epub" {
		t.Errorf("FilePath = %q, want /lib/test.epub", got.FilePath)
	}
}

func TestSetFormatFilePath_Audiobook(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	// Change the book to audiobook first.
	book.MediaType = models.MediaTypeAudiobook
	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeAudiobook, "/ab/test"); err != nil {
		t.Fatalf("SetFormatFilePath: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AudiobookFilePath != "/ab/test" {
		t.Errorf("AudiobookFilePath = %q, want /ab/test", got.AudiobookFilePath)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported", got.Status)
	}
}

// TestSetFormatFilePath_BothPartial verifies that a 'both' book stays in
// 'wanted' status when only one format has been imported.
func TestSetFormatFilePath_BothPartial(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	book.MediaType = models.MediaTypeBoth
	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Import the ebook only.
	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeEbook, "/lib/test.epub"); err != nil {
		t.Fatalf("SetFormatFilePath ebook: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EbookFilePath != "/lib/test.epub" {
		t.Errorf("EbookFilePath = %q, want /lib/test.epub", got.EbookFilePath)
	}
	if got.AudiobookFilePath != "" {
		t.Errorf("AudiobookFilePath should be empty, got %q", got.AudiobookFilePath)
	}
	// Still waiting for the audiobook — must stay wanted.
	if got.Status == models.BookStatusImported {
		t.Errorf("Status must not be 'imported' until both formats are on disk, got %q", got.Status)
	}
}

// TestSetFormatFilePath_BothComplete verifies that a 'both' book flips to
// 'imported' only after both formats are on disk.
func TestSetFormatFilePath_BothComplete(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	book.MediaType = models.MediaTypeBoth
	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeEbook, "/lib/test.epub"); err != nil {
		t.Fatalf("SetFormatFilePath ebook: %v", err)
	}
	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeAudiobook, "/ab/test"); err != nil {
		t.Fatalf("SetFormatFilePath audiobook: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported after both formats set", got.Status)
	}
	if got.EbookFilePath != "/lib/test.epub" {
		t.Errorf("EbookFilePath = %q", got.EbookFilePath)
	}
	if got.AudiobookFilePath != "/ab/test" {
		t.Errorf("AudiobookFilePath = %q", got.AudiobookFilePath)
	}
}

// TestListByStatus_BothPartial verifies that a 'both' book in the partially-
// satisfied state is still returned by ListByStatus("wanted").
func TestListByStatus_BothPartial(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	book.MediaType = models.MediaTypeBoth
	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Import only ebook — audiobook still needed.
	if err := repo.SetFormatFilePath(ctx, book.ID, models.MediaTypeEbook, "/lib/test.epub"); err != nil {
		t.Fatalf("SetFormatFilePath: %v", err)
	}

	wanted, err := repo.ListByStatus(ctx, models.BookStatusWanted)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	found := false
	for _, b := range wanted {
		if b.ID == book.ID {
			found = true
		}
	}
	if !found {
		t.Error("partially-satisfied 'both' book should still appear on the wanted list")
	}
}
