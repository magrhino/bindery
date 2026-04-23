package abs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

func newABSImporterFixture(t *testing.T) (*Importer, *db.AuthorRepo, *db.BookRepo, *db.SeriesRepo, *db.EditionRepo, *db.ABSProvenanceRepo, *db.ABSImportRunRepo, *db.ABSImportRunEntityRepo, *db.ABSReviewItemRepo, *db.ABSMetadataConflictRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	seriesRepo := db.NewSeriesRepo(database)
	editionRepo := db.NewEditionRepo(database)
	provenanceRepo := db.NewABSProvenanceRepo(database)
	runRepo := db.NewABSImportRunRepo(database)
	runEntityRepo := db.NewABSImportRunEntityRepo(database)
	reviewRepo := db.NewABSReviewItemRepo(database)
	conflictRepo := db.NewABSMetadataConflictRepo(database)

	importer := NewImporter(
		authorRepo,
		db.NewAuthorAliasRepo(database),
		bookRepo,
		editionRepo,
		seriesRepo,
		db.NewSettingsRepo(database),
		runRepo,
		runEntityRepo,
		provenanceRepo,
		reviewRepo,
		conflictRepo,
	)
	return importer, authorRepo, bookRepo, seriesRepo, editionRepo, provenanceRepo, runRepo, runEntityRepo, reviewRepo, conflictRepo
}

func sampleABSItem() NormalizedLibraryItem {
	return NormalizedLibraryItem{
		ItemID:        "li-project-hail-mary",
		LibraryID:     "lib-books",
		Title:         "Project Hail Mary",
		Description:   "A lone astronaut must save the earth.",
		Language:      "eng",
		ASIN:          "B08FHBV4ZX",
		PublishedDate: "2021-05-04",
		Authors: []NormalizedAuthor{
			{ID: "author-andy-weir", Name: "Andy Weir"},
		},
		Series: []NormalizedSeries{
			{ID: "series-bobiverse", Name: "Standalone", Sequence: "1"},
		},
		Narrators: []string{"Ray Porter"},
		AudioFiles: []NormalizedAudioFile{
			{INO: "audio-1", Path: "/abs/Project Hail Mary/part1.m4b"},
			{INO: "audio-2", Path: "/abs/Project Hail Mary/part2.m4b"},
		},
		EbookPath:       "/abs/Project Hail Mary/book.epub",
		EbookINO:        "ebook-1",
		DurationSeconds: 57600,
	}
}

type stubABSMetadataProvider struct {
	searchAuthors        []models.Author
	searchAuthorsByQuery map[string][]models.Author
	authors              map[string]*models.Author
	books                map[string]*models.Book
	booksByISBN          map[string]*models.Book
	works                map[string][]models.Book
}

func (p *stubABSMetadataProvider) Name() string { return "stub" }
func (p *stubABSMetadataProvider) SearchAuthors(_ context.Context, query string) ([]models.Author, error) {
	if p.searchAuthorsByQuery != nil {
		return append([]models.Author(nil), p.searchAuthorsByQuery[query]...), nil
	}
	return append([]models.Author(nil), p.searchAuthors...), nil
}
func (p *stubABSMetadataProvider) SearchBooks(context.Context, string) ([]models.Book, error) {
	return nil, nil
}
func (p *stubABSMetadataProvider) GetAuthor(_ context.Context, foreignID string) (*models.Author, error) {
	if p.authors == nil {
		return nil, nil
	}
	return p.authors[foreignID], nil
}
func (p *stubABSMetadataProvider) GetBook(_ context.Context, foreignID string) (*models.Book, error) {
	if p.books == nil {
		return nil, nil
	}
	return p.books[foreignID], nil
}
func (p *stubABSMetadataProvider) GetEditions(context.Context, string) ([]models.Edition, error) {
	return nil, nil
}
func (p *stubABSMetadataProvider) GetBookByISBN(_ context.Context, isbn string) (*models.Book, error) {
	if p.booksByISBN == nil {
		return nil, nil
	}
	return p.booksByISBN[isbn], nil
}
func (p *stubABSMetadataProvider) GetAuthorWorks(_ context.Context, foreignID string) ([]models.Book, error) {
	if p.works == nil {
		return nil, nil
	}
	return append([]models.Book(nil), p.works[foreignID]...), nil
}

func TestImporter_NormalizedAuthorMatchLinksExistingAuthor(t *testing.T) {
	t.Parallel()

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	authorRepo := db.NewAuthorRepo(database)
	aliasRepo := db.NewAuthorAliasRepo(database)
	bookRepo := db.NewBookRepo(database)
	importer := NewImporter(
		authorRepo,
		aliasRepo,
		bookRepo,
		db.NewEditionRepo(database),
		db.NewSeriesRepo(database),
		db.NewSettingsRepo(database),
		db.NewABSImportRunRepo(database),
		db.NewABSImportRunEntityRepo(database),
		db.NewABSProvenanceRepo(database),
		db.NewABSReviewItemRepo(database),
		db.NewABSMetadataConflictRepo(database),
	)

	existing := &models.Author{
		ForeignID:        "OL23919A",
		Name:             "J. K. Rowling",
		SortName:         "Rowling, J. K.",
		MetadataProvider: "openlibrary",
		Monitored:        true,
	}
	if err := authorRepo.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	item := sampleABSItem()
	item.ItemID = "li-rowling-1"
	item.Title = "Harry Potter and the Philosopher's Stone"
	item.Authors = []NormalizedAuthor{{ID: "author-rowling", Name: "J.K. Rowling"}}
	item.AudioFiles = nil
	item.ASIN = ""
	item.EbookPath = "/abs/HP1.epub"
	item.EbookINO = "ebook-rowling-1"
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.AuthorsCreated != 0 || stats.AuthorsLinked != 1 {
		t.Fatalf("stats = %+v, want linked existing author", stats)
	}
	authors, err := authorRepo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors = %d, want 1", len(authors))
	}
	aliases, err := aliasRepo.ListByAuthor(context.Background(), authors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 || aliases[0].Name != "J.K. Rowling" {
		t.Fatalf("aliases = %+v, want J.K. Rowling alias", aliases)
	}
}

func TestImporter_RelinksInitialedAuthorUsingFallbackSearch(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		searchAuthorsByQuery: map[string][]models.Author{
			"J.R.R. Tolkien": {{ForeignID: "OL26320A", Name: "J.R.R. Tolkien"}},
		},
		authors: map[string]*models.Author{
			"OL26320A": {
				ForeignID:        "OL26320A",
				Name:             "J.R.R. Tolkien",
				SortName:         "Tolkien, J.R.R.",
				Description:      "Author of The Hobbit.",
				MetadataProvider: "openlibrary",
			},
		},
		works: map[string][]models.Book{
			"OL26320A": {{ForeignID: "OL-HOBBIT", Title: "The Hobbit", SortTitle: "The Hobbit", Language: "eng", MetadataProvider: "openlibrary", Status: models.BookStatusWanted}},
		},
		books: map[string]*models.Book{
			"OL-HOBBIT": {ForeignID: "OL-HOBBIT", Title: "The Hobbit", SortTitle: "The Hobbit", Language: "eng", MetadataProvider: "openlibrary", Status: models.BookStatusWanted},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))

	item := sampleABSItem()
	item.ItemID = "li-hobbit"
	item.Title = "The Hobbit"
	item.Authors = []NormalizedAuthor{{ID: "author-tolkien", Name: "J. R. R. Tolkien"}}
	item.EbookPath = "/abs/The Hobbit/book.epub"
	item.EbookINO = "ebook-hobbit"
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.MetadataRelinked == 0 {
		t.Fatalf("metadataRelinked = %d, want author relink", stats.MetadataRelinked)
	}
	authors, err := authorRepo.List(context.Background())
	if err != nil || len(authors) != 1 {
		t.Fatalf("authors = %d err=%v, want 1", len(authors), err)
	}
	if authors[0].ForeignID != "OL26320A" || authors[0].Name != "J.R.R. Tolkien" {
		t.Fatalf("author = %+v, want upstream Tolkien", authors[0])
	}
	books, err := bookRepo.ListByAuthor(context.Background(), authors[0].ID)
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %d err=%v, want 1", len(books), err)
	}
}

func TestImporter_CleansABSDescriptionBeforeStoring(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	item := sampleABSItem()
	item.Description = `<p><b>First paragraph.</b></p><p>Second&nbsp;paragraph.</p>`
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	if _, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %d err=%v, want 1", len(books), err)
	}
	want := "First paragraph.\n\nSecond paragraph."
	if books[0].Description != want {
		t.Fatalf("description = %q, want %q", books[0].Description, want)
	}
}

func TestImporter_UnmatchedBookQueuesReview(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, _, _, _, _, _, reviewRepo, _ := newABSImporterFixture(t)
	existing := &models.Author{
		ForeignID:        "OL23919A",
		Name:             "Andy Weir",
		SortName:         "Weir, Andy",
		MetadataProvider: "openlibrary",
		Monitored:        true,
	}
	if err := authorRepo.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	item := sampleABSItem()
	item.ItemID = "li-unmatched-book"
	item.Title = "Completely Unmatched Title"
	item.Authors = []NormalizedAuthor{{ID: "author-andy-weir", Name: "Andy Weir"}}
	item.AudioFiles = nil
	item.ASIN = ""
	item.EbookPath = "/abs/unmatched-book.epub"
	item.EbookINO = "ebook-unmatched"
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.ReviewQueued != 1 || stats.BooksCreated != 0 {
		t.Fatalf("stats = %+v, want queued review without creating book", stats)
	}
	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatalf("books = %d, want 0", len(books))
	}
	reviews, err := reviewRepo.ListByStatus(context.Background(), "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ReviewReason != reviewReasonUnmatchedBook {
		t.Fatalf("reviews = %+v, want unmatched_book review", reviews)
	}
}

func TestImporter_UnmatchedAuthorReviewMessageReportsAuthor(t *testing.T) {
	t.Parallel()

	importer, _, _, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	item := sampleABSItem()
	item.ItemID = "li-onyx-storm"
	item.Title = "Onyx Storm"
	item.Authors = []NormalizedAuthor{{ID: "author-rebecca-yarros", Name: "Rebecca Yarros"}}
	item.ASIN = ""
	item.AudioFiles = nil
	item.EbookPath = "/abs/onyx-storm.epub"
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.ReviewQueued != 1 {
		t.Fatalf("stats = %+v, want one review item", stats)
	}
	progress := importer.Progress()
	if len(progress.Results) != 1 || !strings.Contains(progress.Results[0].Message, "Rebecca Yarros") {
		t.Fatalf("result message = %+v, want author name reported", progress.Results)
	}
}

func TestImporter_ImportReviewUsesResolvedAuthor(t *testing.T) {
	t.Parallel()

	importer, authorRepo, _, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		authors: map[string]*models.Author{
			"OL123A": {
				ForeignID:        "OL123A",
				Name:             "Brandon Sanderson",
				SortName:         "Sanderson, Brandon",
				MetadataProvider: "openlibrary",
				Monitored:        true,
			},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))
	item := sampleABSItem()
	item.ItemID = "li-bands"
	item.Title = "The Bands of Mourning (2 of 2)"
	item.Authors = []NormalizedAuthor{{ID: "author-abs-brandon", Name: "Brandon Sanderson"}}
	item.ResolvedAuthorForeignID = "OL123A"
	item.ResolvedAuthorName = "Brandon Sanderson"

	if _, err := importer.ImportReview(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}, item); err != nil {
		t.Fatalf("ImportReview: %v", err)
	}

	authors, err := authorRepo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors = %d, want 1", len(authors))
	}
	if authors[0].ForeignID != "OL123A" || authors[0].MetadataProvider == providerAudiobookshelf {
		t.Fatalf("author = %+v, want upstream Brandon Sanderson", authors[0])
	}
}

func TestImporter_ImportReviewUsesResolvedBook(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		authors: map[string]*models.Author{
			"OL123A": {ForeignID: "OL123A", Name: "Brandon Sanderson", SortName: "Sanderson, Brandon", MetadataProvider: "openlibrary", Monitored: true},
		},
		books: map[string]*models.Book{
			"OL456W": {
				ForeignID:        "OL456W",
				Title:            "The Bands of Mourning",
				SortTitle:        "The Bands of Mourning",
				Description:      "A Wax and Wayne novel.",
				MetadataProvider: "openlibrary",
			},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))
	item := sampleABSItem()
	item.ItemID = "li-bands"
	item.Title = "The Bands of Mourning"
	item.Authors = []NormalizedAuthor{{ID: "author-abs-brandon", Name: "Brandon Sanderson"}}
	item.ResolvedAuthorForeignID = "OL123A"
	item.ResolvedAuthorName = "Brandon Sanderson"
	item.ResolvedBookForeignID = "OL456W"
	item.ResolvedBookTitle = "The Bands of Mourning"
	item.EditedTitle = "The Bands of Mourning"

	if _, err := importer.ImportReview(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}, item); err != nil {
		t.Fatalf("ImportReview: %v", err)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].ForeignID != "OL456W" || books[0].Title != "The Bands of Mourning" {
		t.Fatalf("book = %+v, want selected upstream book", books[0])
	}
}

func TestImporter_IdempotentRerunAndProvenanceTraceable(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, seriesRepo, editionRepo, provenanceRepo, runRepo, _, _, _ := newABSImporterFixture(t)
	item := sampleABSItem()
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{
			PagesScanned:       1,
			ItemsSeen:          1,
			ItemsNormalized:    1,
			ItemsDetailFetched: 0,
		}, nil
	}
	cfg := ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: "lib-books",
		Label:     "Shelf",
		Enabled:   true,
	}

	first, err := importer.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.BooksCreated != 1 || first.AuthorsCreated != 1 || first.SeriesCreated != 1 {
		t.Fatalf("first stats = %+v", first)
	}
	if first.EditionsAdded != 2 {
		t.Fatalf("editionsAdded = %d, want 2", first.EditionsAdded)
	}

	second, err := importer.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.BooksCreated != 0 || second.BooksUpdated != 1 {
		t.Fatalf("second stats = %+v", second)
	}

	authors, _ := authorRepo.List(context.Background())
	if len(authors) != 1 {
		t.Fatalf("authors = %d, want 1", len(authors))
	}
	books, _ := bookRepo.ListIncludingExcluded(context.Background())
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	allSeries, _ := seriesRepo.List(context.Background())
	if len(allSeries) != 1 {
		t.Fatalf("series = %d, want 1", len(allSeries))
	}
	editions, _ := editionRepo.ListByBook(context.Background(), books[0].ID)
	if len(editions) != 2 {
		t.Fatalf("editions = %d, want 2", len(editions))
	}

	links, err := provenanceRepo.ListByLocal(context.Background(), entityTypeBook, books[0].ID)
	if err != nil {
		t.Fatalf("ListByLocal: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("book provenance links = %d, want 1", len(links))
	}
	if links[0].ExternalID != item.ItemID {
		t.Fatalf("book provenance externalID = %q, want %q", links[0].ExternalID, item.ItemID)
	}
	if len(links[0].FileIDs) != 3 {
		t.Fatalf("book provenance file IDs = %v, want 3 entries", links[0].FileIDs)
	}
	if links[0].ImportRunID == nil || *links[0].ImportRunID == 0 {
		t.Fatal("expected provenance to retain latest run id")
	}
	run, err := runRepo.GetByID(context.Background(), *links[0].ImportRunID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if run == nil || run.Status != runStatusCompleted {
		t.Fatalf("run = %+v, want completed run", run)
	}
}

func TestImporter_MetadataEnrichmentRelinksAuthorAndBook(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, _, _, provenanceRepo, _, _, _, _ := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		searchAuthors: []models.Author{{ForeignID: "OL-ANDY", Name: "Andy Weir"}},
		authors: map[string]*models.Author{
			"OL-ANDY": {
				ForeignID:        "OL-ANDY",
				Name:             "Andy Weir",
				SortName:         "Weir, Andy",
				ImageURL:         "https://img.example.com/andy.jpg",
				MetadataProvider: "openlibrary",
			},
		},
		booksByISBN: map[string]*models.Book{
			"9780593135204": {ForeignID: "OL-PHM", Title: "Project Hail Mary"},
		},
		books: map[string]*models.Book{
			"OL-PHM": {
				ForeignID:        "OL-PHM",
				Title:            "Project Hail Mary",
				ImageURL:         "https://img.example.com/phm.jpg",
				MetadataProvider: "openlibrary",
				Language:         "eng",
			},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))

	item := sampleABSItem()
	item.ISBN = "9780593135204"
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.MetadataRelinked != 2 {
		t.Fatalf("metadataRelinked = %d, want 2", stats.MetadataRelinked)
	}

	authors, err := authorRepo.List(context.Background())
	if err != nil || len(authors) != 1 {
		t.Fatalf("authors = %v err=%v, want 1 author", len(authors), err)
	}
	if authors[0].ForeignID != "OL-ANDY" || authors[0].MetadataProvider != "openlibrary" {
		t.Fatalf("author = %+v, want upstream identity", authors[0])
	}
	if authors[0].ImageURL != "https://img.example.com/andy.jpg" {
		t.Fatalf("author image = %q", authors[0].ImageURL)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %v err=%v, want 1 book", len(books), err)
	}
	if books[0].ForeignID != "OL-PHM" || books[0].MetadataProvider != "openlibrary" {
		t.Fatalf("book = %+v, want upstream identity", books[0])
	}
	if books[0].ImageURL != "https://img.example.com/phm.jpg" {
		t.Fatalf("book image = %q", books[0].ImageURL)
	}

	links, err := provenanceRepo.ListByLocal(context.Background(), entityTypeBook, books[0].ID)
	if err != nil || len(links) != 1 {
		t.Fatalf("book provenance links = %d err=%v", len(links), err)
	}
	if links[0].ExternalID != item.ItemID {
		t.Fatalf("book provenance externalID = %q, want %q", links[0].ExternalID, item.ItemID)
	}
}

func TestImporter_MetadataConflictPersistsAndUsesUpstreamTemporarily(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, conflictRepo := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		searchAuthors: []models.Author{{ForeignID: "OL-ANDY", Name: "Andy Weir"}},
		authors: map[string]*models.Author{
			"OL-ANDY": {ForeignID: "OL-ANDY", Name: "Andy Weir", MetadataProvider: "openlibrary"},
		},
		booksByISBN: map[string]*models.Book{
			"9780593135204": {ForeignID: "OL-PHM", Title: "Project Hail Mary"},
		},
		books: map[string]*models.Book{
			"OL-PHM": {
				ForeignID:        "OL-PHM",
				Title:            "Project Hail Mary",
				Description:      "Upstream version of the story.",
				MetadataProvider: "openlibrary",
				Language:         "eng",
			},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))

	item := sampleABSItem()
	item.ISBN = "9780593135204"
	item.Description = "ABS version of the story."
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.MetadataConflicts != 1 {
		t.Fatalf("metadataConflicts = %d, want 1", stats.MetadataConflicts)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %v err=%v, want 1 book", len(books), err)
	}
	if books[0].Description != "Upstream version of the story." {
		t.Fatalf("book description = %q, want upstream value", books[0].Description)
	}

	conflicts, err := conflictRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(conflicts))
	}
	if conflicts[0].FieldName != "description" || conflicts[0].ResolutionStatus != "unresolved" {
		t.Fatalf("conflict = %+v, want unresolved description conflict", conflicts[0])
	}
	if conflicts[0].AppliedSource != MetadataSourceUpstream {
		t.Fatalf("appliedSource = %q, want upstream", conflicts[0].AppliedSource)
	}
}

func TestImporter_RerunReusesResolvedConflictPreference(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, conflictRepo := newABSImporterFixture(t)
	provider := &stubABSMetadataProvider{
		searchAuthors: []models.Author{{ForeignID: "OL-ANDY", Name: "Andy Weir"}},
		authors: map[string]*models.Author{
			"OL-ANDY": {ForeignID: "OL-ANDY", Name: "Andy Weir", MetadataProvider: "openlibrary"},
		},
		booksByISBN: map[string]*models.Book{
			"9780593135204": {ForeignID: "OL-PHM", Title: "Project Hail Mary"},
		},
		books: map[string]*models.Book{
			"OL-PHM": {
				ForeignID:        "OL-PHM",
				Title:            "Project Hail Mary",
				Description:      "Upstream version of the story.",
				MetadataProvider: "openlibrary",
				Language:         "eng",
			},
		},
	}
	importer.WithMetadata(metadata.NewAggregator(provider))

	item := sampleABSItem()
	item.ISBN = "9780593135204"
	item.Description = "ABS version of the story."
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}
	cfg := ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}

	if _, err := importer.Run(context.Background(), cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	conflicts, err := conflictRepo.List(context.Background())
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %d err=%v, want 1", len(conflicts), err)
	}
	conflicts[0].PreferredSource = MetadataSourceABS
	conflicts[0].AppliedSource = MetadataSourceABS
	conflicts[0].ResolutionStatus = conflictStatusResolved
	if err := conflictRepo.Upsert(context.Background(), &conflicts[0]); err != nil {
		t.Fatalf("Upsert conflict: %v", err)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %d err=%v, want 1", len(books), err)
	}
	if err := ApplyBookConflictValue(&books[0], "description", item.Description); err != nil {
		t.Fatalf("ApplyBookConflictValue: %v", err)
	}
	if err := bookRepo.Update(context.Background(), &books[0]); err != nil {
		t.Fatalf("Update book: %v", err)
	}

	stats, err := importer.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if stats.MetadataAutoResolved == 0 {
		t.Fatalf("metadataAutoResolved = %d, want > 0", stats.MetadataAutoResolved)
	}

	books, err = bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %d err=%v, want 1", len(books), err)
	}
	if books[0].Description != item.Description {
		t.Fatalf("book description = %q, want ABS value", books[0].Description)
	}

	conflicts, err = conflictRepo.List(context.Background())
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %d err=%v, want 1", len(conflicts), err)
	}
	if conflicts[0].ResolutionStatus != conflictStatusResolved || conflicts[0].PreferredSource != MetadataSourceABS {
		t.Fatalf("conflict = %+v, want resolved ABS preference", conflicts[0])
	}
}

func TestImporter_LinksExistingBookUsingNormalizedTitleAndAuthorName(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, _, _, provenanceRepo, _, _, _, _ := newABSImporterFixture(t)
	existingAuthor := &models.Author{
		ForeignID:        "ol:author:le-guin",
		Name:             "Ursula K Le Guin",
		SortName:         "Le Guin, Ursula K",
		MetadataProvider: "openlibrary",
		Monitored:        true,
	}
	if err := authorRepo.Create(context.Background(), existingAuthor); err != nil {
		t.Fatalf("seed author: %v", err)
	}
	existingBook := &models.Book{
		ForeignID:        "ol:book:left-hand",
		AuthorID:         existingAuthor.ID,
		Title:            "The Left Hand of Darkness",
		SortTitle:        "The Left Hand of Darkness",
		Status:           models.BookStatusWanted,
		Monitored:        true,
		AnyEditionOK:     true,
		MediaType:        models.MediaTypeAudiobook,
		MetadataProvider: "openlibrary",
	}
	if err := bookRepo.Create(context.Background(), existingBook); err != nil {
		t.Fatalf("seed book: %v", err)
	}

	item := sampleABSItem()
	item.ItemID = "li-left-hand"
	item.Title = "The Left Hand of Darkness (German Edition)"
	item.Authors = []NormalizedAuthor{{ID: "author-ursula", Name: "  URSULA K LE GUIN  "}}
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: "lib-books",
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.AuthorsLinked != 1 || stats.BooksLinked != 1 {
		t.Fatalf("stats = %+v, want linked author/book", stats)
	}

	books, _ := bookRepo.ListIncludingExcluded(context.Background())
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].ForeignID != "ol:book:left-hand" {
		t.Fatalf("existing book foreign id should be preserved, got %q", books[0].ForeignID)
	}

	links, err := provenanceRepo.ListByLocal(context.Background(), entityTypeBook, existingBook.ID)
	if err != nil {
		t.Fatalf("ListByLocal: %v", err)
	}
	if len(links) != 1 || links[0].ExternalID != "li-left-hand" {
		t.Fatalf("links = %+v, want ABS item provenance on existing book", links)
	}
}

func TestImporter_ReconcilesSharedPathsIntoOwnedState(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	storageRoot := t.TempDir()
	libraryDir := filepath.Join(storageRoot, "books")
	audiobookDir := filepath.Join(storageRoot, "audiobooks")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	if err := os.MkdirAll(audiobookDir, 0o755); err != nil {
		t.Fatalf("mkdir audiobook dir: %v", err)
	}
	ebookPath := filepath.Join(libraryDir, "Andy Weir", "Project Hail Mary.epub")
	if err := os.MkdirAll(filepath.Dir(ebookPath), 0o755); err != nil {
		t.Fatalf("mkdir ebook dir: %v", err)
	}
	if err := os.WriteFile(ebookPath, []byte("ebook"), 0o644); err != nil {
		t.Fatalf("write ebook: %v", err)
	}
	audiobookPath := filepath.Join(audiobookDir, "Andy Weir", "Project Hail Mary")
	if err := os.MkdirAll(audiobookPath, 0o755); err != nil {
		t.Fatalf("mkdir audiobook path: %v", err)
	}

	item := sampleABSItem()
	item.Path = audiobookPath
	item.EbookPath = ebookPath
	importer.WithStoragePaths(libraryDir, audiobookDir, nil)
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.OwnedMarked != 2 {
		t.Fatalf("ownedMarked = %d, want 2", stats.OwnedMarked)
	}
	if stats.PendingManual != 0 {
		t.Fatalf("pendingManual = %d, want 0", stats.PendingManual)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("ListIncludingExcluded: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].Status != models.BookStatusImported {
		t.Fatalf("status = %q, want imported", books[0].Status)
	}
	if books[0].EbookFilePath != ebookPath {
		t.Fatalf("ebook path = %q, want %q", books[0].EbookFilePath, ebookPath)
	}
	if books[0].AudiobookFilePath != audiobookPath {
		t.Fatalf("audiobook path = %q, want %q", books[0].AudiobookFilePath, audiobookPath)
	}
	files, err := bookRepo.ListFiles(context.Background(), books[0].ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("book files = %d, want 2", len(files))
	}
}

func TestImporter_LeavesMissingSharedPathsPendingManual(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	storageRoot := t.TempDir()
	libraryDir := filepath.Join(storageRoot, "books")
	audiobookDir := filepath.Join(storageRoot, "audiobooks")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	if err := os.MkdirAll(audiobookDir, 0o755); err != nil {
		t.Fatalf("mkdir audiobook dir: %v", err)
	}

	item := sampleABSItem()
	item.Path = filepath.Join(audiobookDir, "Andy Weir", "Project Hail Mary")
	item.EbookPath = filepath.Join(libraryDir, "Andy Weir", "Project Hail Mary.epub")
	importer.WithStoragePaths(libraryDir, audiobookDir, nil)
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.OwnedMarked != 0 {
		t.Fatalf("ownedMarked = %d, want 0", stats.OwnedMarked)
	}
	if stats.PendingManual != 2 {
		t.Fatalf("pendingManual = %d, want 2", stats.PendingManual)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("ListIncludingExcluded: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].Status != models.BookStatusWanted {
		t.Fatalf("status = %q, want wanted", books[0].Status)
	}
	files, err := bookRepo.ListFiles(context.Background(), books[0].ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("book files = %d, want 0", len(files))
	}
	progress := importer.Progress()
	if len(progress.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(progress.Results))
	}
	if !strings.Contains(progress.Results[0].Message, "metadata only") {
		t.Fatalf("message = %q, want metadata-only guidance", progress.Results[0].Message)
	}
}

func TestImporter_AppliesPathRemapBeforeOwnedStateReconciliation(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	storageRoot := t.TempDir()
	libraryDir := filepath.Join(storageRoot, "books")
	audiobookDir := filepath.Join(storageRoot, "audiobooks")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	if err := os.MkdirAll(audiobookDir, 0o755); err != nil {
		t.Fatalf("mkdir audiobook dir: %v", err)
	}
	ebookPath := filepath.Join(libraryDir, "audiobookshelf", "Andy Weir", "Project Hail Mary.epub")
	if err := os.MkdirAll(filepath.Dir(ebookPath), 0o755); err != nil {
		t.Fatalf("mkdir ebook dir: %v", err)
	}
	if err := os.WriteFile(ebookPath, []byte("ebook"), 0o644); err != nil {
		t.Fatalf("write ebook: %v", err)
	}
	audiobookPath := filepath.Join(audiobookDir, "audiobookshelf", "Andy Weir", "Project Hail Mary")
	if err := os.MkdirAll(audiobookPath, 0o755); err != nil {
		t.Fatalf("mkdir audiobook path: %v", err)
	}

	item := sampleABSItem()
	item.Path = "/audiobookshelf/Andy Weir/Project Hail Mary"
	item.EbookPath = "/audiobookshelf/Andy Weir/Project Hail Mary.epub"
	importer.WithStoragePaths(libraryDir, audiobookDir, nil)
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		PathRemap: "/audiobookshelf:" + filepath.Join(storageRoot, "books", "audiobookshelf"),
		Label:     "Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.OwnedMarked != 1 || stats.PendingManual != 1 {
		t.Fatalf("stats = %+v, want remapped ebook + pending audiobook", stats)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil || len(books) != 1 {
		t.Fatalf("books = %d err=%v, want 1", len(books), err)
	}
	if books[0].EbookFilePath != ebookPath {
		t.Fatalf("ebook path = %q, want %q", books[0].EbookFilePath, ebookPath)
	}
}

func TestImporter_ReviewFileMappingReportsVisibleRemappedPath(t *testing.T) {
	t.Parallel()

	importer, _, _, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	storageRoot := t.TempDir()
	libraryDir := filepath.Join(storageRoot, "books")
	if err := os.MkdirAll(filepath.Join(libraryDir, "audiobookshelf"), 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	ebookPath := filepath.Join(libraryDir, "audiobookshelf", "Onyx Storm.epub")
	if err := os.WriteFile(ebookPath, []byte("ebook"), 0o644); err != nil {
		t.Fatalf("write ebook: %v", err)
	}
	importer.WithStoragePaths(libraryDir, libraryDir, nil)
	item := sampleABSItem()
	item.EbookPath = "/abs/Onyx Storm.epub"
	item.AudioFiles = nil
	item.Path = ""

	mapping := importer.ReviewFileMapping(context.Background(), ImportConfig{
		PathRemap: "/abs:" + filepath.Join(libraryDir, "audiobookshelf"),
	}, item)
	if !mapping.Found {
		t.Fatalf("mapping = %+v, want found", mapping)
	}
}

func TestImporter_ReviewFileMappingReportsMissingPath(t *testing.T) {
	t.Parallel()

	importer, _, _, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	libraryDir := t.TempDir()
	importer.WithStoragePaths(libraryDir, libraryDir, nil)
	item := sampleABSItem()
	item.EbookPath = filepath.Join(libraryDir, "missing.epub")
	item.AudioFiles = nil
	item.Path = ""

	mapping := importer.ReviewFileMapping(context.Background(), ImportConfig{}, item)
	if mapping.Found || !strings.Contains(mapping.Message, "not visible") {
		t.Fatalf("mapping = %+v, want missing path message", mapping)
	}
}

func TestImporter_RerunKeepsOwnedStateStable(t *testing.T) {
	t.Parallel()

	importer, _, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	storageRoot := t.TempDir()
	libraryDir := filepath.Join(storageRoot, "books")
	audiobookDir := filepath.Join(storageRoot, "audiobooks")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	if err := os.MkdirAll(audiobookDir, 0o755); err != nil {
		t.Fatalf("mkdir audiobook dir: %v", err)
	}
	ebookPath := filepath.Join(libraryDir, "Andy Weir", "Project Hail Mary.epub")
	if err := os.MkdirAll(filepath.Dir(ebookPath), 0o755); err != nil {
		t.Fatalf("mkdir ebook dir: %v", err)
	}
	if err := os.WriteFile(ebookPath, []byte("ebook"), 0o644); err != nil {
		t.Fatalf("write ebook: %v", err)
	}
	audiobookPath := filepath.Join(audiobookDir, "Andy Weir", "Project Hail Mary")
	if err := os.MkdirAll(audiobookPath, 0o755); err != nil {
		t.Fatalf("mkdir audiobook path: %v", err)
	}

	item := sampleABSItem()
	item.Path = audiobookPath
	item.EbookPath = ebookPath
	importer.WithStoragePaths(libraryDir, audiobookDir, nil)
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}
	cfg := ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}

	first, err := importer.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.OwnedMarked != 2 {
		t.Fatalf("first ownedMarked = %d, want 2", first.OwnedMarked)
	}

	second, err := importer.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.OwnedMarked != 0 {
		t.Fatalf("second ownedMarked = %d, want 0", second.OwnedMarked)
	}
	if second.PendingManual != 0 {
		t.Fatalf("second pendingManual = %d, want 0", second.PendingManual)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("ListIncludingExcluded: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].Status != models.BookStatusImported {
		t.Fatalf("status = %q, want imported", books[0].Status)
	}
	files, err := bookRepo.ListFiles(context.Background(), books[0].ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("book files = %d, want 2 after rerun", len(files))
	}
}

func TestImporter_DryRunDoesNotMutateCatalogButPersistsRunSummary(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, _, _, provenanceRepo, runRepo, _, _, _ := newABSImporterFixture(t)
	item := sampleABSItem()
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	stats, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.BooksCreated != 1 || stats.AuthorsCreated != 1 {
		t.Fatalf("dry-run stats = %+v", stats)
	}

	authors, err := authorRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List authors: %v", err)
	}
	if len(authors) != 0 {
		t.Fatalf("authors = %d, want 0 after dry-run", len(authors))
	}
	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("List books: %v", err)
	}
	if len(books) != 0 {
		t.Fatalf("books = %d, want 0 after dry-run", len(books))
	}
	links, err := provenanceRepo.ListByLocal(context.Background(), entityTypeBook, 1)
	if err != nil {
		t.Fatalf("ListByLocal: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("provenance links = %d, want 0 after dry-run", len(links))
	}

	runs, err := runRepo.ListRecent(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(runs) != 1 || !runs[0].DryRun {
		t.Fatalf("runs = %+v, want one dry-run record", runs)
	}
	hydrated := HydrateRun(runs[0])
	if !hydrated.Summary.DryRun || hydrated.Summary.Stats.BooksCreated != 1 {
		t.Fatalf("hydrated run summary = %+v", hydrated.Summary)
	}
}

func TestImporter_RollbackRemovesCreatedBatch(t *testing.T) {
	t.Parallel()

	importer, authorRepo, bookRepo, seriesRepo, editionRepo, provenanceRepo, _, _, _, _ := newABSImporterFixture(t)
	item := sampleABSItem()
	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		if err := fn(ctx, item); err != nil {
			return EnumerationStats{}, err
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: 1, ItemsNormalized: 1}, nil
	}

	if _, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: item.LibraryID,
		Label:     "Shelf",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	runs, err := importer.RecentRuns(context.Background(), 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("RecentRuns = %d err=%v, want 1 run", len(runs), err)
	}
	preview, err := importer.RollbackPreview(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatalf("RollbackPreview: %v", err)
	}
	if preview.Stats.ActionsPlanned == 0 {
		t.Fatalf("preview = %+v, want planned actions", preview)
	}
	result, err := importer.Rollback(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if result.Stats.EntitiesDeleted == 0 {
		t.Fatalf("rollback result = %+v, want deletions", result)
	}
	if result.Status != runStatusRolledBack {
		t.Fatalf("rollback status = %q, want %q", result.Status, runStatusRolledBack)
	}
	run, err := importer.GetRun(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run == nil || run.Status != runStatusRolledBack {
		t.Fatalf("run = %+v, want rolled_back", run)
	}

	authors, _ := authorRepo.List(context.Background())
	if len(authors) != 0 {
		t.Fatalf("authors = %d, want 0 after rollback", len(authors))
	}
	books, _ := bookRepo.ListIncludingExcluded(context.Background())
	if len(books) != 0 {
		t.Fatalf("books = %d, want 0 after rollback", len(books))
	}
	allSeries, _ := seriesRepo.List(context.Background())
	if len(allSeries) != 0 {
		t.Fatalf("series = %d, want 0 after rollback", len(allSeries))
	}
	editions, err := editionRepo.ListByBook(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListByBook: %v", err)
	}
	if len(editions) != 0 {
		t.Fatalf("editions = %d, want 0 after rollback", len(editions))
	}
	link, err := provenanceRepo.GetByExternal(context.Background(), DefaultSourceID, item.LibraryID, entityTypeBook, item.ItemID)
	if err != nil {
		t.Fatalf("GetByExternal: %v", err)
	}
	if link != nil {
		t.Fatalf("book provenance = %+v, want nil after rollback", link)
	}
}
