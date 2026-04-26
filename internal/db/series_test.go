package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestSeriesCreateOrGet_Idempotent(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)

	s := &models.Series{
		ForeignID: "ol-series:dune-chronicles",
		Title:     "Dune Chronicles",
	}

	// First call should insert.
	if err := repo.CreateOrGet(ctx, s); err != nil {
		t.Fatalf("first CreateOrGet: %v", err)
	}
	if s.ID == 0 {
		t.Fatal("expected non-zero ID after first CreateOrGet")
	}
	firstID := s.ID

	// Second call with the same foreign_id should return the same ID.
	s2 := &models.Series{
		ForeignID: "ol-series:dune-chronicles",
		Title:     "Dune Chronicles",
	}
	if err := repo.CreateOrGet(ctx, s2); err != nil {
		t.Fatalf("second CreateOrGet: %v", err)
	}
	if s2.ID != firstID {
		t.Errorf("expected same ID %d on second call, got %d", firstID, s2.ID)
	}

	// Verify only one row exists.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 series row, got %d", len(list))
	}
}

func TestSeriesLinkBook(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	authorRepo := NewAuthorRepo(database)
	bookRepo := NewBookRepo(database)
	seriesRepo := NewSeriesRepo(database)

	// Seed author + book.
	author := &models.Author{
		ForeignID: "OL1A", Name: "Frank Herbert", SortName: "Herbert, Frank",
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	book := &models.Book{
		ForeignID: "OL1W", AuthorID: author.ID, Title: "Dune", SortTitle: "Dune",
		Status: models.BookStatusWanted, Genres: []string{}, MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	// Upsert series and link book.
	s := &models.Series{ForeignID: "ol-series:dune-chronicles", Title: "Dune Chronicles"}
	if err := seriesRepo.CreateOrGet(ctx, s); err != nil {
		t.Fatalf("CreateOrGet: %v", err)
	}
	if err := seriesRepo.LinkBook(ctx, s.ID, book.ID, "1", true); err != nil {
		t.Fatalf("LinkBook: %v", err)
	}

	// GetByID should return the series with the book attached.
	got, err := seriesRepo.GetByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected series, got nil")
	}
	if got.Title != "Dune Chronicles" {
		t.Errorf("title: want %q, got %q", "Dune Chronicles", got.Title)
	}
	if len(got.Books) != 1 {
		t.Fatalf("expected 1 series_book, got %d", len(got.Books))
	}
	sb := got.Books[0]
	if sb.PositionInSeries != "1" {
		t.Errorf("position: want %q, got %q", "1", sb.PositionInSeries)
	}
	if !sb.PrimarySeries {
		t.Error("expected primary_series=true")
	}
	if sb.Book == nil || sb.Book.Title != "Dune" {
		t.Errorf("expected joined book 'Dune', got %v", sb.Book)
	}

	// LinkBook is idempotent (INSERT OR IGNORE).
	if err := seriesRepo.LinkBook(ctx, s.ID, book.ID, "1", true); err != nil {
		t.Errorf("second LinkBook should be idempotent, got: %v", err)
	}

	// Cascade: deleting the book should remove the series_books row.
	if err := bookRepo.Delete(ctx, book.ID); err != nil {
		t.Fatalf("delete book: %v", err)
	}
	got, err = seriesRepo.GetByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetByID after book delete: %v", err)
	}
	if len(got.Books) != 0 {
		t.Errorf("expected 0 series_books after book delete, got %d", len(got.Books))
	}
}

func TestSeriesList(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)

	// Empty list.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0, got %d", len(list))
	}

	// Add two series.
	for _, title := range []string{"Alpha Series", "Beta Series"} {
		s := &models.Series{ForeignID: "ol-series:" + title, Title: title}
		if err := repo.CreateOrGet(ctx, s); err != nil {
			t.Fatalf("CreateOrGet %q: %v", title, err)
		}
	}

	list, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 series, got %d", len(list))
	}
}

func TestSeriesHardcoverLinkCRUD(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "Stormlight Archive"}
	if err := repo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}

	link := &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   "hc-series:1",
		HardcoverProviderID: "1",
		HardcoverTitle:      "The Stormlight Archive",
		HardcoverAuthorName: "Brandon Sanderson",
		HardcoverBookCount:  10,
		Confidence:          0.82,
		LinkedBy:            "auto",
	}
	if err := repo.UpsertHardcoverLink(ctx, link); err != nil {
		t.Fatalf("upsert link: %v", err)
	}
	if link.ID == 0 {
		t.Fatal("expected stored link id")
	}

	got, err := repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got == nil || got.HardcoverTitle != "The Stormlight Archive" || got.LinkedBy != "auto" {
		t.Fatalf("unexpected link: %+v", got)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list series: %v", err)
	}
	if list[0].HardcoverLink == nil || list[0].HardcoverLink.HardcoverSeriesID != "hc-series:1" {
		t.Fatalf("expected hydrated link in list, got %+v", list[0].HardcoverLink)
	}

	link.HardcoverTitle = "Stormlight Archive"
	link.LinkedBy = "manual"
	link.Confidence = 1
	if err := repo.UpsertHardcoverLink(ctx, link); err != nil {
		t.Fatalf("update link: %v", err)
	}
	got, err = repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get updated link: %v", err)
	}
	if got.HardcoverTitle != "Stormlight Archive" || got.LinkedBy != "manual" || got.Confidence != 1 {
		t.Fatalf("unexpected updated link: %+v", got)
	}

	if err := repo.DeleteHardcoverLink(ctx, series.ID); err != nil {
		t.Fatalf("delete link: %v", err)
	}
	got, err = repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get deleted link: %v", err)
	}
	if got != nil {
		t.Fatalf("expected deleted link, got %+v", got)
	}
}
