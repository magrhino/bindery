package db

import (
	"context"
	"errors"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestBookRepoFillMissingAudiobookDuration(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	books := NewBookRepo(database)
	author := mkAuthor(t, NewAuthorRepo(database), ctx, "OL-DURATION-A")
	book := mkBook(t, books, ctx, author.ID, "hc:duration-book", "Duration Book", models.BookStatusWanted)
	book.MetadataProvider = "hardcover"
	book.MediaType = models.MediaTypeAudiobook
	book.ASIN = "B000AUDIO1"
	if err := books.Update(ctx, book); err != nil {
		t.Fatal(err)
	}

	fetched := *book
	fetched.DurationSeconds = 36000
	updated, current, err := books.FillMissingAudiobookDuration(ctx, &fetched)
	if err != nil || !updated || current != 36000 {
		t.Fatalf("initial duration fill: updated=%v current=%d err=%v", updated, current, err)
	}

	stored, err := books.GetByID(ctx, book.ID)
	if err != nil || stored == nil {
		t.Fatalf("load stored book: %v", err)
	}
	if stored.DurationSeconds != 36000 {
		t.Fatalf("stored duration = %d, want 36000", stored.DurationSeconds)
	}
	stored.Monitored = false
	stored.Status = models.BookStatusSkipped
	stored.Narrator = "Concurrent narrator"
	if err := books.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}

	fetched.DurationSeconds = 72000
	updated, current, err = books.FillMissingAudiobookDuration(ctx, &fetched)
	if err != nil || updated || current != 36000 {
		t.Fatalf("existing duration should win: updated=%v current=%d err=%v", updated, current, err)
	}
	stored, err = books.GetByID(ctx, book.ID)
	if err != nil || stored == nil {
		t.Fatalf("reload stored book: %v", err)
	}
	if stored.DurationSeconds != 36000 || stored.Monitored || stored.Status != models.BookStatusSkipped || stored.Narrator != "Concurrent narrator" {
		t.Fatalf("guarded write changed concurrent fields: %+v", stored)
	}

	stored.ASIN = "B000OTHER1"
	stored.DurationSeconds = 42000
	if err := books.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}
	updated, current, err = books.FillMissingAudiobookDuration(ctx, &fetched)
	if err != nil || updated || current != 0 {
		t.Fatalf("changed ASIN must not return another narration's duration: updated=%v current=%d err=%v", updated, current, err)
	}
	stored, err = books.GetByID(ctx, book.ID)
	if err != nil || stored == nil {
		t.Fatalf("reload changed book: %v", err)
	}
	if stored.ASIN != "B000OTHER1" || stored.DurationSeconds != 42000 {
		t.Fatalf("changed book was overwritten: %+v", stored)
	}

	fetched.MediaType = models.MediaTypeEbook
	updated, current, err = books.FillMissingAudiobookDuration(ctx, &fetched)
	if err != nil || updated || current != 0 {
		t.Fatalf("ebook must not receive audio duration: updated=%v current=%d err=%v", updated, current, err)
	}
	fetched.MediaType = models.MediaTypeAudiobook
	fetched.DurationSeconds = 0
	if _, _, err := books.FillMissingAudiobookDuration(ctx, &fetched); err == nil {
		t.Fatal("zero candidate duration should be rejected")
	}
	if _, _, err := books.FillMissingAudiobookDuration(ctx, nil); err == nil {
		t.Fatal("nil book should be rejected")
	}
	fetched.DurationSeconds = 72000
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := books.FillMissingAudiobookDuration(cancelled, &fetched); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write error = %v, want context.Canceled", err)
	}
}

func TestBookRepoFillMissingAudiobookDurationForBoth(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	books := NewBookRepo(database)
	author := mkAuthor(t, NewAuthorRepo(database), ctx, "OL-DURATION-B")
	book := mkBook(t, books, ctx, author.ID, "hc:both-book", "Both Book", models.BookStatusWanted)
	book.MetadataProvider = "hardcover"
	book.MediaType = models.MediaTypeBoth
	book.DurationSeconds = 5400
	if err := books.Update(ctx, book); err != nil {
		t.Fatal(err)
	}
	book.DurationSeconds = 6300
	updated, current, err := books.FillMissingAudiobookDuration(ctx, book)
	if err != nil || updated || current != 5400 {
		t.Fatalf("known dual-format duration should win: updated=%v current=%d err=%v", updated, current, err)
	}
}
