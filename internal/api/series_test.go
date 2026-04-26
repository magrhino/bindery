package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

func seriesFixture(t *testing.T) (*SeriesHandler, *db.SeriesRepo, *db.AuthorRepo, *db.BookRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	repo := db.NewSeriesRepo(database)
	bookRepo := db.NewBookRepo(database)
	authorRepo := db.NewAuthorRepo(database)
	return NewSeriesHandler(repo, bookRepo, authorRepo, nil, &mockBookSearcher{}), repo, authorRepo, bookRepo
}

type stubSeriesProvider struct {
	searchResults []metadata.SeriesSearchResult
	catalogs      map[string]*metadata.SeriesCatalog
	searchErr     error
	catalogErr    error
}

func (s *stubSeriesProvider) Name() string { return "stub" }

func (s *stubSeriesProvider) SearchAuthors(context.Context, string) ([]models.Author, error) {
	return nil, nil
}

func (s *stubSeriesProvider) SearchBooks(context.Context, string) ([]models.Book, error) {
	return nil, nil
}

func (s *stubSeriesProvider) GetAuthor(context.Context, string) (*models.Author, error) {
	return nil, nil
}

func (s *stubSeriesProvider) GetBook(context.Context, string) (*models.Book, error) {
	return nil, nil
}

func (s *stubSeriesProvider) GetEditions(context.Context, string) ([]models.Edition, error) {
	return nil, nil
}

func (s *stubSeriesProvider) GetBookByISBN(context.Context, string) (*models.Book, error) {
	return nil, nil
}

func (s *stubSeriesProvider) SearchSeries(context.Context, string, int) ([]metadata.SeriesSearchResult, error) {
	return s.searchResults, s.searchErr
}

func (s *stubSeriesProvider) GetSeriesCatalog(_ context.Context, foreignID string) (*metadata.SeriesCatalog, error) {
	if s.catalogErr != nil {
		return nil, s.catalogErr
	}
	return s.catalogs[foreignID], nil
}

func seriesFixtureWithProvider(t *testing.T, provider *stubSeriesProvider, searcher BookSearcher) (*SeriesHandler, *db.SeriesRepo, *db.AuthorRepo, *db.BookRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	seriesRepo := db.NewSeriesRepo(database)
	bookRepo := db.NewBookRepo(database)
	authorRepo := db.NewAuthorRepo(database)
	if searcher == nil {
		searcher = &mockBookSearcher{}
	}
	return NewSeriesHandler(seriesRepo, bookRepo, authorRepo, metadata.NewAggregator(provider), searcher), seriesRepo, authorRepo, bookRepo
}

func TestSeriesList_Empty(t *testing.T) {
	h, _, _, _ := seriesFixture(t)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/series", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if bytes.TrimSpace(rec.Body.Bytes())[0] != '[' {
		t.Errorf("expected JSON array, got %s", rec.Body.String())
	}
}

func TestSeriesGet_BadID(t *testing.T) {
	h, _, _, _ := seriesFixture(t)
	rec := httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/series/abc", nil), "id", "abc"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", rec.Code)
	}
}

func TestSeriesGet_NotFound(t *testing.T) {
	h, _, _, _ := seriesFixture(t)
	rec := httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/series/999", nil), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing: expected 404, got %d", rec.Code)
	}
}

// TestSeriesListAndGet_WithData creates a series with linked books so the
// happy path (List returns rows; Get returns the Books array non-null) is
// covered.
func TestSeriesListAndGet_WithData(t *testing.T) {
	h, seriesRepo, authorRepo, bookRepo := seriesFixture(t)
	ctx := contextBackground()

	author := &models.Author{ForeignID: "OL1A", Name: "A", SortName: "A"}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	book := &models.Book{ForeignID: "OL1B", AuthorID: author.ID, Title: "Book One", Status: models.BookStatusWanted}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	s := &models.Series{ForeignID: "OLSER1", Title: "Series One"}
	if err := seriesRepo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.LinkBook(ctx, s.ID, book.ID, "1", true); err != nil {
		t.Fatal(err)
	}

	// List
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/series", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list []models.Series
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 series, got %d", len(list))
	}
	if len(list[0].Books) != 1 || list[0].Books[0].BookID != book.ID {
		t.Fatalf("expected linked book in series list, got %+v", list[0].Books)
	}

	// Get with books
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/series/1", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
	var got models.Series
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Books) != 1 || got.Books[0].BookID != book.ID {
		t.Errorf("expected linked book in series, got %+v", got.Books)
	}
}

func TestSeriesHardcoverSearch(t *testing.T) {
	provider := &stubSeriesProvider{
		searchResults: []metadata.SeriesSearchResult{{
			ForeignID:    "hc-series:42",
			ProviderID:   "42",
			Title:        "The Stormlight Archive",
			AuthorName:   "Brandon Sanderson",
			BookCount:    10,
			ReadersCount: 19323,
			Books:        []string{"The Way of Kings", "Words of Radiance"},
		}},
		catalogs: map[string]*metadata.SeriesCatalog{},
	}
	h, _, _, _ := seriesFixtureWithProvider(t, provider, nil)

	rec := httptest.NewRecorder()
	h.SearchHardcover(rec, httptest.NewRequest(http.MethodGet, "/api/v1/series/hardcover/search?term=stormlight", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []seriesHardcoverSearchResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ForeignID != "hc-series:42" || got[0].BookCount != 10 {
		t.Fatalf("unexpected search results: %+v", got)
	}
}

func TestSeriesHardcoverSearchNormalizesNilBooks(t *testing.T) {
	provider := &stubSeriesProvider{
		searchResults: []metadata.SeriesSearchResult{{
			ForeignID:  "hc-series:42",
			ProviderID: "42",
			Title:      "The Stormlight Archive",
		}},
		catalogs: map[string]*metadata.SeriesCatalog{},
	}
	h, _, _, _ := seriesFixtureWithProvider(t, provider, nil)

	rec := httptest.NewRecorder()
	h.SearchHardcover(rec, httptest.NewRequest(http.MethodGet, "/api/v1/series/hardcover/search?term=stormlight", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []seriesHardcoverSearchResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one search result, got %+v", got)
	}
	if got[0].Books == nil {
		t.Fatalf("expected books to encode as an empty array, got nil")
	}
}

func TestSeriesAutoLinkHardcoverPersistsTopCandidate(t *testing.T) {
	catalog := stormlightCatalog()
	provider := &stubSeriesProvider{
		searchResults: []metadata.SeriesSearchResult{{
			ForeignID:  catalog.ForeignID,
			ProviderID: catalog.ProviderID,
			Title:      catalog.Title,
			AuthorName: catalog.AuthorName,
			BookCount:  catalog.BookCount,
		}},
		catalogs: map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
	}
	h, seriesRepo, _, _ := seriesFixtureWithProvider(t, provider, nil)
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "The Stormlight Archive"}
	if err := seriesRepo.Create(contextBackground(), series); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.AutoLinkHardcover(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/series/1/hardcover-link/auto", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response seriesHardcoverAutoResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.Linked || response.Link == nil || response.Link.HardcoverSeriesID != catalog.ForeignID {
		t.Fatalf("expected persisted auto link, got %+v", response)
	}
}

func TestSeriesAutoLinkHardcoverAmbiguousNoop(t *testing.T) {
	catalogA := &metadata.SeriesCatalog{
		ForeignID:  "hc-series:42",
		ProviderID: "42",
		Title:      "Rhythm of War",
		AuthorName: "Brandon Sanderson",
		BookCount:  1,
		Books:      []metadata.SeriesCatalogBook{},
	}
	catalogB := &metadata.SeriesCatalog{
		ForeignID:  "hc-series:99",
		ProviderID: "99",
		Title:      "Rhythm of War",
		AuthorName: "Brandon Sanderson",
		BookCount:  0,
		Books:      []metadata.SeriesCatalogBook{},
	}
	provider := &stubSeriesProvider{
		searchResults: []metadata.SeriesSearchResult{
			{ForeignID: catalogA.ForeignID, ProviderID: catalogA.ProviderID, Title: catalogA.Title, AuthorName: catalogA.AuthorName, BookCount: catalogA.BookCount},
			{ForeignID: catalogB.ForeignID, ProviderID: catalogB.ProviderID, Title: catalogB.Title, AuthorName: catalogB.AuthorName},
		},
		catalogs: map[string]*metadata.SeriesCatalog{
			catalogA.ForeignID: catalogA,
			catalogB.ForeignID: catalogB,
		},
	}
	h, seriesRepo, _, _ := seriesFixtureWithProvider(t, provider, nil)
	series := &models.Series{ForeignID: "ol-series:rhythm", Title: "Rhythm of War"}
	if err := seriesRepo.Create(contextBackground(), series); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.AutoLinkHardcover(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/series/1/hardcover-link/auto", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response seriesHardcoverAutoResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Linked {
		t.Fatalf("ambiguous result should not persist, got %+v", response)
	}
	link, err := seriesRepo.GetHardcoverLink(contextBackground(), series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if link != nil {
		t.Fatalf("expected no link, got %+v", link)
	}
}

func TestSeriesFillCreatesMissingHardcoverBook(t *testing.T) {
	catalog := stormlightCatalog()
	searcher := newMockBookSearcher()
	h, seriesRepo, _, bookRepo := seriesFixtureWithProvider(t, &stubSeriesProvider{
		catalogs: map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
	}, searcher)
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "The Stormlight Archive"}
	if err := seriesRepo.Create(contextBackground(), series); err != nil {
		t.Fatal(err)
	}
	link := &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   catalog.ForeignID,
		HardcoverProviderID: catalog.ProviderID,
		HardcoverTitle:      catalog.Title,
		HardcoverAuthorName: catalog.AuthorName,
		HardcoverBookCount:  catalog.BookCount,
		Confidence:          1,
		LinkedBy:            "manual",
	}
	if err := seriesRepo.UpsertHardcoverLink(contextBackground(), link); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.Fill(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/series/1/fill", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["queued"] != 1 {
		t.Fatalf("expected one queued book, got %+v", body)
	}
	queued := searcher.waitForCall(t, time.Second)
	if queued.Title != "The Way of Kings" {
		t.Fatalf("unexpected queued book: %+v", queued)
	}
	created, err := bookRepo.GetByForeignID(contextBackground(), "hc:the-way-of-kings")
	if err != nil {
		t.Fatal(err)
	}
	if created == nil {
		t.Fatal("expected Hardcover book to be created")
	}
	if created.MetadataProvider != "hardcover" {
		t.Fatalf("expected metadata provider to be preserved, got %q", created.MetadataProvider)
	}
	if !created.AnyEditionOK {
		t.Fatal("expected anyEditionOk to be preserved")
	}
	books, err := seriesRepo.ListBooksInSeries(contextBackground(), series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ForeignID != "hc:the-way-of-kings" {
		t.Fatalf("expected created book linked to series, got %+v", books)
	}
}

func TestSeriesFillQueuesLocalBooksWhenHardcoverCatalogFails(t *testing.T) {
	catalog := stormlightCatalog()
	searcher := newMockBookSearcher()
	h, seriesRepo, authorRepo, bookRepo := seriesFixtureWithProvider(t, &stubSeriesProvider{
		catalogs:   map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
		catalogErr: errors.New("hardcover unavailable"),
	}, searcher)
	ctx := contextBackground()
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "The Stormlight Archive"}
	if err := seriesRepo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}
	link := &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   catalog.ForeignID,
		HardcoverProviderID: catalog.ProviderID,
		HardcoverTitle:      catalog.Title,
		HardcoverAuthorName: catalog.AuthorName,
		HardcoverBookCount:  catalog.BookCount,
		Confidence:          1,
		LinkedBy:            "manual",
	}
	if err := seriesRepo.UpsertHardcoverLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	author := &models.Author{
		ForeignID:        "hc:brandon-sanderson",
		Name:             "Brandon Sanderson",
		SortName:         "Sanderson, Brandon",
		MetadataProvider: "hardcover",
		Monitored:        true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	book := &models.Book{
		ForeignID:        "hc:words-of-radiance",
		AuthorID:         author.ID,
		Title:            "Words of Radiance",
		SortTitle:        "Words of Radiance",
		Status:           models.BookStatusSkipped,
		Genres:           []string{},
		MetadataProvider: "hardcover",
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}
	if _, err := seriesRepo.LinkBookIfMissing(ctx, series.ID, book.ID, "2", true); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.Fill(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/series/1/fill", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["queued"] != 1 {
		t.Fatalf("expected one local book queued despite provider failure, got %+v", body)
	}
	queued := searcher.waitForCall(t, time.Second)
	if queued.ID != book.ID || queued.Title != "Words of Radiance" {
		t.Fatalf("unexpected queued book: %+v", queued)
	}
	updated, err := bookRepo.GetByID(ctx, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Status != models.BookStatusWanted || !updated.Monitored {
		t.Fatalf("expected local book marked wanted and monitored, got %+v", updated)
	}
}

func stormlightCatalog() *metadata.SeriesCatalog {
	book := models.Book{
		ForeignID:        "hc:the-way-of-kings",
		Title:            "The Way of Kings",
		SortTitle:        "The Way of Kings",
		MetadataProvider: "hardcover",
		Author: &models.Author{
			ForeignID:        "hc:brandon-sanderson",
			Name:             "Brandon Sanderson",
			SortName:         "Sanderson, Brandon",
			MetadataProvider: "hardcover",
		},
	}
	return &metadata.SeriesCatalog{
		ForeignID:  "hc-series:42",
		ProviderID: "42",
		Title:      "The Stormlight Archive",
		AuthorName: "Brandon Sanderson",
		BookCount:  1,
		Books: []metadata.SeriesCatalogBook{{
			ForeignID:  book.ForeignID,
			ProviderID: "101",
			Title:      book.Title,
			Position:   "1",
			UsersCount: 123,
			Book:       book,
		}},
	}
}
