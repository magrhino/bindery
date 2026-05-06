package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

// mockProvider is a test double for the Provider interface.
type mockProvider struct {
	name              string
	searchBooks       []models.Book
	searchBookErr     error
	searchAuthors     []models.Author
	searchAuthErr     error
	getAuthor         *models.Author
	getAuthorErr      error
	getBook           *models.Book
	getBookErr        error
	getBookCalls      int
	gotBookIDs        []string
	getEditions       []models.Edition
	getEditionsErr    error
	getByISBN         *models.Book
	getByISBNErr      error
	getByISBNCalls    int
	gotISBNs          []string
	searchBookQueries []string
	// authorWorks implements worksProvider interface
	authorWorks    []models.Book
	authorWorksErr error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) SearchAuthors(_ context.Context, _ string) ([]models.Author, error) {
	return m.searchAuthors, m.searchAuthErr
}
func (m *mockProvider) SearchBooks(_ context.Context, query string) ([]models.Book, error) {
	m.searchBookQueries = append(m.searchBookQueries, query)
	return m.searchBooks, m.searchBookErr
}
func (m *mockProvider) GetAuthor(_ context.Context, _ string) (*models.Author, error) {
	return m.getAuthor, m.getAuthorErr
}
func (m *mockProvider) GetBook(_ context.Context, foreignID string) (*models.Book, error) {
	m.getBookCalls++
	m.gotBookIDs = append(m.gotBookIDs, foreignID)
	return m.getBook, m.getBookErr
}
func (m *mockProvider) GetEditions(_ context.Context, _ string) ([]models.Edition, error) {
	return m.getEditions, m.getEditionsErr
}
func (m *mockProvider) GetBookByISBN(_ context.Context, isbn string) (*models.Book, error) {
	m.getByISBNCalls++
	m.gotISBNs = append(m.gotISBNs, isbn)
	return m.getByISBN, m.getByISBNErr
}

// worksProvider implementation (optional, only attached when needed).
type mockWorksProvider struct {
	mockProvider
}

func (m *mockWorksProvider) GetAuthorWorks(_ context.Context, _ string) ([]models.Book, error) {
	return m.authorWorks, m.authorWorksErr
}

type mockAuthorWorksByNameProvider struct {
	mockProvider
	authorWorksByName    []models.Book
	authorWorksByNameErr error
	gotAuthorName        string
	calls                int
}

func (m *mockAuthorWorksByNameProvider) GetAuthorWorksByName(_ context.Context, authorName string) ([]models.Book, error) {
	m.calls++
	m.gotAuthorName = authorName
	return m.authorWorksByName, m.authorWorksByNameErr
}

func TestAggregator_SearchAuthors(t *testing.T) {
	want := []models.Author{{Name: "Frank Herbert", ForeignID: "OL123A"}}
	primary := &mockProvider{name: "ol", searchAuthors: want}
	agg := newTestAggregator(primary)

	got, err := agg.SearchAuthors(context.Background(), "Herbert")
	if err != nil {
		t.Fatalf("SearchAuthors: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Frank Herbert" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestAggregator_SearchAuthors_Error(t *testing.T) {
	primary := &mockProvider{name: "ol", searchAuthErr: errors.New("network error")}
	agg := newTestAggregator(primary)

	_, err := agg.SearchAuthors(context.Background(), "Herbert")
	if err == nil {
		t.Fatal("expected error to be propagated")
	}
}

func TestAggregator_SearchBooks(t *testing.T) {
	want := []models.Book{{Title: "Dune", ForeignID: "OL456W"}}
	primary := &mockProvider{name: "ol", searchBooks: want}
	agg := newTestAggregator(primary)

	got, err := agg.SearchBooks(context.Background(), "Dune")
	if err != nil {
		t.Fatalf("SearchBooks: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Dune" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestAggregator_GetAuthor_Success(t *testing.T) {
	author := &models.Author{Name: "Ursula K. Le Guin", ForeignID: "OL111A"}
	primary := &mockProvider{name: "ol", getAuthor: author}
	agg := newTestAggregator(primary)

	got, err := agg.GetAuthor(context.Background(), "OL111A")
	if err != nil {
		t.Fatalf("GetAuthor: %v", err)
	}
	if got.Name != "Ursula K. Le Guin" {
		t.Errorf("Name: want 'Ursula K. Le Guin', got %q", got.Name)
	}
}

func TestAggregator_GetAuthor_Cached(t *testing.T) {
	calls := 0
	primary := &mockProvider{name: "ol", getAuthor: &models.Author{Name: "Isaac Asimov"}}
	agg := newTestAggregator(primary)
	// Wrap to count calls
	origGetAuthor := primary.getAuthor

	_, _ = agg.GetAuthor(context.Background(), "OL999A")
	calls++                 // first call
	primary.getAuthor = nil // second call should use cache, not nil author
	got, err := agg.GetAuthor(context.Background(), "OL999A")
	if err != nil {
		t.Fatalf("GetAuthor (cached): %v", err)
	}
	if got.Name != origGetAuthor.Name {
		t.Errorf("expected cached author, got %+v", got)
	}
	_ = calls
}

func TestAggregator_GetAuthor_Error(t *testing.T) {
	primary := &mockProvider{name: "ol", getAuthorErr: errors.New("not found")}
	agg := newTestAggregator(primary)

	_, err := agg.GetAuthor(context.Background(), "OL999A")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestAggregator_GetBook_LongDescription(t *testing.T) {
	// A book with a long description should NOT trigger enrichment.
	longDesc := string(make([]byte, 100)) // 100-char description
	for i := range longDesc {
		_ = i
	}
	longDesc = "This is a very long book description that exceeds the fifty character minimum and should never be enriched by secondary providers."
	book := &models.Book{Title: "Dune", Description: longDesc}

	enricherCalled := false
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{Description: "Should not be used"}},
	}
	// We'll detect if enricher was called by overriding its SearchBooks
	_ = enricherCalled
	primary := &mockProvider{name: "ol", getBook: book}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{enricher},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetBook(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.Description != longDesc {
		t.Errorf("description should not be overwritten when long enough")
	}
}

func TestAggregator_GetBook_ShortDescription_Enriched(t *testing.T) {
	shortDesc := "Short."
	richerDesc := "A much richer description that is longer than the short one from the primary provider."

	primary := &mockProvider{
		name:    "ol",
		getBook: &models.Book{Title: "Foundation", Description: shortDesc},
	}
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{Description: richerDesc}},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{enricher},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetBook(context.Background(), "OL789W")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.Description != richerDesc {
		t.Errorf("expected enriched description %q, got %q", richerDesc, got.Description)
	}
}

func TestAggregator_GetBook_Enrichment_RatingFilled(t *testing.T) {
	primary := &mockProvider{
		name:    "ol",
		getBook: &models.Book{Title: "Short", Description: "x", AverageRating: 0},
	}
	enricher := &mockProvider{
		name:        "hc",
		searchBooks: []models.Book{{Description: "Some desc", AverageRating: 4.5, RatingsCount: 100}},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{enricher},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetBook(context.Background(), "OL001W")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.AverageRating != 4.5 {
		t.Errorf("rating: want 4.5, got %f", got.AverageRating)
	}
	if got.RatingsCount != 100 {
		t.Errorf("ratingsCount: want 100, got %d", got.RatingsCount)
	}
}

func TestAggregator_GetBook_Cached(t *testing.T) {
	primary := &mockProvider{name: "ol", getBook: &models.Book{Title: "Cached Book", Description: "A sufficiently long description for caching test purposes here."}}
	agg := newTestAggregator(primary)

	first, _ := agg.GetBook(context.Background(), "OL111W")
	primary.getBook = nil // clear so second call must use cache

	second, err := agg.GetBook(context.Background(), "OL111W")
	if err != nil {
		t.Fatalf("GetBook (cache): %v", err)
	}
	if second.Title != first.Title {
		t.Errorf("cached book mismatch: got %q", second.Title)
	}
}

func TestAggregator_GetBook_Error(t *testing.T) {
	primary := &mockProvider{name: "ol", getBookErr: errors.New("lookup failed")}
	agg := newTestAggregator(primary)

	_, err := agg.GetBook(context.Background(), "OL999W")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestAggregator_GetBook_RoutesProviderPrefixes(t *testing.T) {
	primary := &mockProvider{name: "openlibrary", getBook: &models.Book{Title: "Wrong"}}
	google := &mockProvider{name: "googlebooks", getBook: &models.Book{ForeignID: "gb:vol1", Title: "Google Book", MetadataProvider: "googlebooks"}}
	hardcover := &mockProvider{name: "hardcover", getBook: &models.Book{ForeignID: "hc:book", Title: "Hardcover Book", MetadataProvider: "hardcover"}}
	dnb := &mockProvider{name: "dnb", getBook: &models.Book{ForeignID: "dnb:123", Title: "DNB Book", MetadataProvider: "dnb"}}
	agg := newTestAggregator(primary, google, hardcover, dnb)

	tests := []struct {
		foreignID string
		wantTitle string
		provider  *mockProvider
	}{
		{foreignID: "gb:vol1", wantTitle: "Google Book", provider: google},
		{foreignID: "hc:book", wantTitle: "Hardcover Book", provider: hardcover},
		{foreignID: "dnb:123", wantTitle: "DNB Book", provider: dnb},
	}
	for _, tt := range tests {
		got, err := agg.GetBook(context.Background(), tt.foreignID)
		if err != nil {
			t.Fatalf("GetBook(%q): %v", tt.foreignID, err)
		}
		if got == nil || got.Title != tt.wantTitle {
			t.Fatalf("GetBook(%q) = %+v, want %s", tt.foreignID, got, tt.wantTitle)
		}
		if tt.provider.getBookCalls != 1 || tt.provider.gotBookIDs[0] != tt.foreignID {
			t.Fatalf("%s calls=%d ids=%v, want one %s", tt.provider.name, tt.provider.getBookCalls, tt.provider.gotBookIDs, tt.foreignID)
		}
	}
	if primary.getBookCalls != 0 {
		t.Fatalf("primary get calls = %d, want 0", primary.getBookCalls)
	}
}

func TestAggregator_GetAuthor_RoutesProviderPrefixes(t *testing.T) {
	primary := &mockProvider{name: "openlibrary", getAuthor: &models.Author{Name: "Wrong"}}
	hardcover := &mockProvider{name: "hardcover", getAuthor: &models.Author{ForeignID: "hc:author", Name: "Hardcover Author", MetadataProvider: "hardcover"}}
	agg := newTestAggregator(primary, hardcover)

	got, err := agg.GetAuthor(context.Background(), "hc:author")
	if err != nil {
		t.Fatalf("GetAuthor: %v", err)
	}
	if got == nil || got.Name != "Hardcover Author" {
		t.Fatalf("got %+v, want Hardcover Author", got)
	}
}

func TestAggregator_GetEditions_Success(t *testing.T) {
	editions := []models.Edition{{Title: "1st ed."}, {Title: "2nd ed."}}
	primary := &mockProvider{name: "ol", getEditions: editions}
	agg := newTestAggregator(primary)

	got, err := agg.GetEditions(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("GetEditions: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 editions, got %d", len(got))
	}
}

func TestAggregator_GetEditions_Cached(t *testing.T) {
	editions := []models.Edition{{Title: "Paperback"}}
	primary := &mockProvider{name: "ol", getEditions: editions}
	agg := newTestAggregator(primary)

	_, _ = agg.GetEditions(context.Background(), "OL999W")
	primary.getEditions = nil // clear; second call must use cache

	got, err := agg.GetEditions(context.Background(), "OL999W")
	if err != nil {
		t.Fatalf("GetEditions (cache): %v", err)
	}
	if len(got) != 1 || got[0].Title != "Paperback" {
		t.Errorf("cached editions mismatch: %+v", got)
	}
}

func TestAggregator_GetBookByISBN_PrimaryHitStopsBeforeEnrichers(t *testing.T) {
	book := &models.Book{Title: "The Left Hand of Darkness", Description: "A novel long enough description to pass the enrichment check easily."}
	primary := &mockProvider{name: "ol", getByISBN: book}
	enricher := &mockProvider{name: "hardcover", getByISBN: &models.Book{Title: "Wrong Book"}}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.GetBookByISBN(context.Background(), "9780441478125")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got.Title != "The Left Hand of Darkness" {
		t.Errorf("Title: want 'The Left Hand of Darkness', got %q", got.Title)
	}
	if primary.getByISBNCalls != 1 {
		t.Errorf("primary calls = %d, want 1", primary.getByISBNCalls)
	}
	if enricher.getByISBNCalls != 0 {
		t.Errorf("enricher calls = %d, want 0 after primary hit", enricher.getByISBNCalls)
	}
}

func TestAggregator_GetBookByISBN_SearchesRegisteredEnrichers(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider string
	}{
		{name: "google books", provider: "googlebooks"},
		{name: "hardcover", provider: "hardcover"},
		{name: "dnb", provider: "dnb"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			primary := &mockProvider{name: "ol"}
			enricher := &mockProvider{
				name:      tt.provider,
				getByISBN: &models.Book{Title: tt.provider + " ISBN Book", MetadataProvider: tt.provider},
			}
			agg := newTestAggregator(primary, enricher)

			got, err := agg.GetBookByISBN(context.Background(), "9780000000002")
			if err != nil {
				t.Fatalf("GetBookByISBN: %v", err)
			}
			if got == nil {
				t.Fatal("expected secondary provider result")
			}
			if got.MetadataProvider != tt.provider {
				t.Fatalf("MetadataProvider = %q, want %q", got.MetadataProvider, tt.provider)
			}
			if primary.getByISBNCalls != 1 || enricher.getByISBNCalls != 1 {
				t.Fatalf("calls primary=%d enricher=%d, want 1/1", primary.getByISBNCalls, enricher.getByISBNCalls)
			}
		})
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesSecondaryHitToPrimary(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{{
			ForeignID: "OL-PHM",
			Title:     "Project Hail Mary",
			Author:    &models.Author{Name: "Andy Weir"},
		}},
		getBook: &models.Book{
			ForeignID:        "OL-PHM",
			Title:            "Project Hail Mary",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Andy Weir", ForeignID: "OL-A"},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:vol-phm",
			Title:            "Project Hail Mary",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Andy Weir"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780593135204")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-PHM" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary book", got)
	}
	if len(primary.searchBookQueries) != 1 || primary.searchBookQueries[0] != "Project Hail Mary Andy Weir" {
		t.Fatalf("search queries = %v, want title+author query", primary.searchBookQueries)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesSecondaryHitPrefersExactPrimaryTitle(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{
			{
				ForeignID: "OL-ANTHOLOGY",
				Title:     "The Martian / Artemis / Project Hail Mary",
				Author:    &models.Author{Name: "Andy Weir"},
			},
			{
				ForeignID: "OL-PHM",
				Title:     "Project Hail Mary",
				Author:    &models.Author{Name: "Andy Weir"},
			},
		},
		getBook: &models.Book{
			ForeignID:        "OL-PHM",
			Title:            "Project Hail Mary",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Andy Weir", ForeignID: "OL-A"},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:vol-phm",
			Title:            "Project Hail Mary",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Andy Weir"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780593135204")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-PHM" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want exact-title OpenLibrary book", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL-PHM" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL-PHM", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_KeepsSecondaryHitWhenPrimaryMatchAmbiguous(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{
			{ForeignID: "OL1W", Title: "The Book", Author: &models.Author{Name: "A. Author"}},
			{ForeignID: "OL2W", Title: "The Book", Author: &models.Author{Name: "A. Author"}},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:ambiguous",
			Title:            "The Book",
			Description:      "Secondary provider description long enough to avoid enrichment.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "A. Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000007")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "gb:ambiguous" {
		t.Fatalf("got %+v, want secondary provider result", got)
	}
}

func TestAggregator_GetBookByISBN_ContinuesAfterProviderError(t *testing.T) {
	primary := &mockProvider{name: "ol", getByISBNErr: errors.New("openlibrary down")}
	enricher := &mockProvider{name: "dnb", getByISBN: &models.Book{Title: "DNB ISBN Book", MetadataProvider: "dnb"}}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.GetBookByISBN(context.Background(), "9783453198975")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.Title != "DNB ISBN Book" {
		t.Fatalf("got %+v, want DNB ISBN Book", got)
	}
}

func TestAggregator_GetBookByISBN_SkipsUnconfiguredProviders(t *testing.T) {
	primary := &mockProvider{name: "ol"}
	unconfigured := &mockProvider{name: "hardcover", getByISBNErr: ErrProviderNotConfigured}
	dnb := &mockProvider{name: "dnb", getByISBN: &models.Book{Title: "DNB ISBN Book", MetadataProvider: "dnb"}}
	agg := newTestAggregator(primary, unconfigured, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453198975")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.MetadataProvider != "dnb" {
		t.Fatalf("got %+v, want dnb result", got)
	}
}

func TestAggregator_GetBookByISBN_AllProvidersMiss(t *testing.T) {
	primary := &mockProvider{name: "ol"}
	enricher := &mockProvider{name: "dnb"}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.GetBookByISBN(context.Background(), "0000000000")
	if err != nil {
		t.Fatalf("GetBookByISBN(nil): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing ISBN, got %+v", got)
	}
}

func TestAggregator_GetBookByISBN_AllConfiguredProvidersFail(t *testing.T) {
	primary := &mockProvider{name: "ol", getByISBNErr: errors.New("openlibrary down")}
	enricher := &mockProvider{name: "dnb", getByISBNErr: errors.New("dnb down")}
	agg := newTestAggregator(primary, enricher)

	_, err := agg.GetBookByISBN(context.Background(), "9780000000003")
	if err == nil {
		t.Fatal("expected error when all configured providers fail")
	}
}

func TestAggregator_GetBookByISBN_FirstSuccessfulProviderWins(t *testing.T) {
	primary := &mockProvider{name: "ol"}
	first := &mockProvider{name: "googlebooks", getByISBN: &models.Book{Title: "First", MetadataProvider: "googlebooks"}}
	second := &mockProvider{name: "dnb", getByISBN: &models.Book{Title: "Second", MetadataProvider: "dnb"}}
	agg := newTestAggregator(primary, first, second)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000004")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.Title != "First" {
		t.Fatalf("got %+v, want first provider result", got)
	}
	if second.getByISBNCalls != 0 {
		t.Fatalf("second provider calls = %d, want 0", second.getByISBNCalls)
	}
}

func TestAggregator_GetBookByISBN_EnrichesShortDescription(t *testing.T) {
	primary := &mockProvider{name: "ol", getByISBN: &models.Book{Title: "Sparse ISBN", Description: "Short."}}
	enricher := &mockProvider{
		name: "googlebooks",
		searchBooks: []models.Book{{
			Description:   "A fuller description from a configured enricher that should replace the sparse ISBN result.",
			AverageRating: 4.2,
			RatingsCount:  12,
		}},
	}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000005")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil {
		t.Fatal("expected book")
	}
	if got.Description == "Short." {
		t.Fatalf("expected enriched description, got %q", got.Description)
	}
	if got.AverageRating != 4.2 || got.RatingsCount != 12 {
		t.Fatalf("rating/count = %f/%d, want 4.2/12", got.AverageRating, got.RatingsCount)
	}
}

func TestAggregator_GetBookByISBN_CachesSecondaryProviderHit(t *testing.T) {
	primary := &mockProvider{name: "ol"}
	enricher := &mockProvider{name: "hardcover", getByISBN: &models.Book{Title: "Cached Secondary", MetadataProvider: "hardcover"}}
	agg := newTestAggregator(primary, enricher)

	_, err := agg.GetBookByISBN(context.Background(), "9780000000006")
	if err != nil {
		t.Fatalf("first GetBookByISBN: %v", err)
	}
	enricher.getByISBN = nil

	got, err := agg.GetBookByISBN(context.Background(), "9780000000006")
	if err != nil {
		t.Fatalf("cached GetBookByISBN: %v", err)
	}
	if got == nil || got.Title != "Cached Secondary" {
		t.Fatalf("cached book = %+v, want Cached Secondary", got)
	}
	if primary.getByISBNCalls != 1 || enricher.getByISBNCalls != 1 {
		t.Fatalf("calls primary=%d enricher=%d, want 1/1 after cache hit", primary.getByISBNCalls, enricher.getByISBNCalls)
	}
}

func TestAggregator_GetBookByISBN_CachesCleanMisses(t *testing.T) {
	primary := &mockProvider{name: "ol"}
	enricher := &mockProvider{name: "dnb"}
	agg := newTestAggregator(primary, enricher)

	for i := 0; i < 2; i++ {
		got, err := agg.GetBookByISBN(context.Background(), "0000000000")
		if err != nil {
			t.Fatalf("GetBookByISBN #%d: %v", i+1, err)
		}
		if got != nil {
			t.Fatalf("GetBookByISBN #%d = %+v, want nil", i+1, got)
		}
	}
	if primary.getByISBNCalls != 1 || enricher.getByISBNCalls != 1 {
		t.Fatalf("calls primary=%d enricher=%d, want 1/1 after cached miss", primary.getByISBNCalls, enricher.getByISBNCalls)
	}
}

func TestAggregator_GetAuthorWorks_WorksProvider(t *testing.T) {
	books := []models.Book{{Title: "Dune"}, {Title: "Dune Messiah"}}
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: books},
	}
	agg := &Aggregator{
		primary: primary,
		cache:   newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorks(context.Background(), "OL123A")
	if err != nil {
		t.Fatalf("GetAuthorWorks: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 works, got %d", len(got))
	}
	if got[0].Title != "Dune" {
		t.Errorf("first title: want 'Dune', got %q", got[0].Title)
	}
}

func TestAggregator_GetAuthorWorks_Fallback(t *testing.T) {
	// Primary does not implement worksProvider → falls back to SearchBooks.
	books := []models.Book{{Title: "Foundation"}, {Title: "Foundation and Empire"}}
	primary := &mockProvider{name: "gb", searchBooks: books}
	agg := &Aggregator{
		primary: primary,
		cache:   newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorks(context.Background(), "OL999A")
	if err != nil {
		t.Fatalf("GetAuthorWorks (fallback): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 works from fallback, got %d", len(got))
	}
}

func TestAggregator_GetAuthorWorks_Cached(t *testing.T) {
	books := []models.Book{{Title: "Ender's Game"}}
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: books},
	}
	agg := &Aggregator{
		primary: primary,
		cache:   newTTLCache(time.Minute),
	}

	_, _ = agg.GetAuthorWorks(context.Background(), "OL555A")
	primary.authorWorks = nil // clear; next call must hit cache

	got, err := agg.GetAuthorWorks(context.Background(), "OL555A")
	if err != nil {
		t.Fatalf("GetAuthorWorks (cache): %v", err)
	}
	if len(got) != 1 || got[0].Title != "Ender's Game" {
		t.Errorf("cached works mismatch: %+v", got)
	}
}

func TestAggregator_GetAuthorWorksForAuthor_MergesSupplementalByTitle(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider: mockProvider{name: "hardcover"},
		authorWorksByName: []models.Book{
			{
				ForeignID:        "hc:dune",
				Title:            "Dune",
				Description:      "A desert planet.",
				ImageURL:         "https://img/dune.jpg",
				AverageRating:    4.5,
				RatingsCount:     1000,
				MetadataProvider: "hardcover",
			},
			{ForeignID: "hc:children-of-dune", Title: "Children of Dune", MetadataProvider: "hardcover"},
		},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if hardcover.gotAuthorName != "Frank Herbert" {
		t.Fatalf("supplemental author name = %q", hardcover.gotAuthorName)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 merged works, got %d: %+v", len(got), got)
	}
	if got[0].ForeignID != "OL1W" || got[0].MetadataProvider != "openlibrary" {
		t.Fatalf("primary identity should win duplicate title: %+v", got[0])
	}
	if got[0].ImageURL != "https://img/dune.jpg" || got[0].AverageRating != 4.5 || got[0].Description == "" {
		t.Fatalf("supplemental metadata was not merged: %+v", got[0])
	}
	if got[1].ForeignID != "hc:children-of-dune" {
		t.Fatalf("supplemental-only book missing: %+v", got[1])
	}
}

func TestAggregator_GetAuthorWorksForAuthor_MergesSupplementalIntoFirstDuplicateTitle(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", MetadataProvider: "openlibrary"},
			{ForeignID: "OL2W", Title: "Dune", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider: mockProvider{name: "hardcover"},
		authorWorksByName: []models.Book{
			{
				ForeignID:        "hc:dune",
				Title:            "Dune",
				ImageURL:         "https://img/dune.jpg",
				AverageRating:    4.5,
				MetadataProvider: "hardcover",
			},
		},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected duplicate primary works to remain for downstream dedup, got %d: %+v", len(got), got)
	}
	if got[0].ImageURL != "https://img/dune.jpg" || got[0].AverageRating != 4.5 {
		t.Fatalf("first duplicate did not receive supplemental metadata: %+v", got[0])
	}
	if got[1].ImageURL != "" || got[1].AverageRating != 0 {
		t.Fatalf("supplemental metadata merged into later duplicate: %+v", got[1])
	}
}

func TestAggregator_GetAuthorWorksForAuthor_EnrichesMissingCoversAfterSupplement(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", MetadataProvider: "openlibrary"},
			{ForeignID: "OL2W", Title: "Heretics of Dune", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider: mockProvider{name: "hardcover"},
		authorWorksByName: []models.Book{
			{ForeignID: "hc:dune", Title: "Dune", ImageURL: "https://img/dune.jpg", MetadataProvider: "hardcover"},
		},
	}
	google := &mockProvider{
		name:        "googlebooks",
		searchBooks: []models.Book{{ImageURL: "https://books.google.com/heretics.jpg"}},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover, google},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 works, got %d: %+v", len(got), got)
	}
	if got[0].ImageURL != "https://img/dune.jpg" {
		t.Fatalf("matched supplemental cover was not merged: %+v", got[0])
	}
	if got[1].ImageURL != "https://books.google.com/heretics.jpg" {
		t.Fatalf("missing cover was not enriched after supplement: %+v", got[1])
	}
}

func TestAggregator_GetAuthorWorksForAuthor_ContinuesWhenSupplementFails(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{{ForeignID: "OL1W", Title: "Dune", ImageURL: "cover"}}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider:         mockProvider{name: "hardcover"},
		authorWorksByNameErr: errors.New("hardcover unavailable"),
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 1 || got[0].ForeignID != "OL1W" {
		t.Fatalf("expected primary result after supplement failure, got %+v", got)
	}
}

func TestAggregator_GetAuthorWorksForAuthor_DoesNotCacheUnconfiguredSupplement(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", ImageURL: "cover", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider:         mockProvider{name: "hardcover"},
		authorWorksByNameErr: ErrProviderNotConfigured,
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 1 || got[0].ForeignID != "OL1W" {
		t.Fatalf("expected primary-only result, got %+v", got)
	}

	hardcover.authorWorksByNameErr = nil
	hardcover.authorWorksByName = []models.Book{{ForeignID: "hc:children-of-dune", Title: "Children of Dune", MetadataProvider: "hardcover"}}
	got, err = agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor after config: %v", err)
	}
	if hardcover.calls != 2 {
		t.Fatalf("supplement calls = %d, want 2", hardcover.calls)
	}
	if len(got) != 2 || got[1].ForeignID != "hc:children-of-dune" {
		t.Fatalf("expected supplemental result after config, got %+v", got)
	}
}

func TestAggregator_GetAuthorWorksForAuthor_DoesNotCacheFailedSupplement(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", ImageURL: "cover", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider:         mockProvider{name: "hardcover"},
		authorWorksByNameErr: errors.New("hardcover unavailable"),
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 1 || got[0].ForeignID != "OL1W" {
		t.Fatalf("expected primary-only result, got %+v", got)
	}

	hardcover.authorWorksByNameErr = nil
	hardcover.authorWorksByName = []models.Book{{ForeignID: "hc:dune-messiah", Title: "Dune Messiah", MetadataProvider: "hardcover"}}
	got, err = agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor after recovery: %v", err)
	}
	if hardcover.calls != 2 {
		t.Fatalf("supplement calls = %d, want 2", hardcover.calls)
	}
	if len(got) != 2 || got[1].ForeignID != "hc:dune-messiah" {
		t.Fatalf("expected supplemental result after recovery, got %+v", got)
	}
}

func TestAggregator_GetAuthorWorksForAuthor_CachesSuccessfulSupplement(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{name: "ol", authorWorks: []models.Book{
			{ForeignID: "OL1W", Title: "Dune", ImageURL: "cover", MetadataProvider: "openlibrary"},
		}},
	}
	hardcover := &mockAuthorWorksByNameProvider{
		mockProvider: mockProvider{name: "hardcover"},
		authorWorksByName: []models.Book{
			{ForeignID: "hc:children-of-dune", Title: "Children of Dune", MetadataProvider: "hardcover"},
		},
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{hardcover},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected merged works, got %+v", got)
	}

	hardcover.authorWorksByName = nil
	got, err = agg.GetAuthorWorksForAuthor(context.Background(), models.Author{ForeignID: "OL123A", Name: "Frank Herbert"})
	if err != nil {
		t.Fatalf("GetAuthorWorksForAuthor cached: %v", err)
	}
	if hardcover.calls != 1 {
		t.Fatalf("supplement calls = %d, want 1", hardcover.calls)
	}
	if len(got) != 2 || got[1].ForeignID != "hc:children-of-dune" {
		t.Fatalf("expected cached supplemental result, got %+v", got)
	}
}

func TestAggregator_EnrichAudiobook_NonAudiobook(t *testing.T) {
	agg := newTestAggregator(&mockProvider{name: "ol"})
	book := &models.Book{Title: "Ebook", MediaType: models.MediaTypeEbook, ASIN: "B001"}
	if err := agg.EnrichAudiobook(context.Background(), book); err != nil {
		t.Fatalf("EnrichAudiobook (ebook): %v", err)
	}
}

func TestAggregator_EnrichAudiobook_NilBook(t *testing.T) {
	agg := newTestAggregator(&mockProvider{name: "ol"})
	if err := agg.EnrichAudiobook(context.Background(), nil); err != nil {
		t.Fatalf("EnrichAudiobook(nil): %v", err)
	}
}

func TestAggregator_EnrichAudiobook_NoASIN(t *testing.T) {
	agg := newTestAggregator(&mockProvider{name: "ol"})
	book := &models.Book{Title: "Audiobook", MediaType: models.MediaTypeAudiobook, ASIN: ""}
	if err := agg.EnrichAudiobook(context.Background(), book); err != nil {
		t.Fatalf("EnrichAudiobook (no ASIN): %v", err)
	}
}

// TestAggregator_GetAuthorAudiobooks_Unconfigured verifies the nil-audible
// path used by every test aggregator returns an empty result instead of
// panicking — the aggregator is constructed without an audible.Client in
// unit tests, and callers rely on a safe fallback.
func TestAggregator_GetAuthorAudiobooks_Unconfigured(t *testing.T) {
	agg := newTestAggregator(&mockProvider{name: "ol"})
	books, err := agg.GetAuthorAudiobooks(context.Background(), "Frank Herbert")
	if err != nil {
		t.Fatalf("GetAuthorAudiobooks (nil client): %v", err)
	}
	if books != nil {
		t.Errorf("want nil, got %v", books)
	}
}

// TestAggregator_GetAuthorAudiobooks_EmptyName guards against the trivial
// case where an unnamed author triggers an unfiltered Audible browse.
func TestAggregator_GetAuthorAudiobooks_EmptyName(t *testing.T) {
	agg := newTestAggregator(&mockProvider{name: "ol"})
	books, err := agg.GetAuthorAudiobooks(context.Background(), "   ")
	if err != nil {
		t.Fatalf("GetAuthorAudiobooks (empty): %v", err)
	}
	if books != nil {
		t.Errorf("want nil, got %v", books)
	}
}

func TestAggregator_EnrichBook_SkipsOnSearchError(t *testing.T) {
	primary := &mockProvider{
		name:    "ol",
		getBook: &models.Book{Title: "Error Test", Description: "x"},
	}
	enricher := &mockProvider{
		name:          "hc",
		searchBookErr: errors.New("hardcover unavailable"),
	}
	agg := &Aggregator{
		primary:   primary,
		enrichers: []Provider{enricher},
		cache:     newTTLCache(time.Minute),
	}

	got, err := agg.GetBook(context.Background(), "OL002W")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	// Description should remain unchanged since enricher errored.
	if got.Description != "x" {
		t.Errorf("description changed unexpectedly: %q", got.Description)
	}
}

func TestAggregator_enrichBook_FillsCoverWhenMissing(t *testing.T) {
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{Description: "A description.", ImageURL: "https://books.google.com/cover.jpg"}},
	}
	agg := &Aggregator{enrichers: []Provider{enricher}, cache: newTTLCache(time.Minute)}

	book := &models.Book{Title: "Sapiens", ImageURL: ""}
	agg.enrichBook(context.Background(), book)
	if book.ImageURL != "https://books.google.com/cover.jpg" {
		t.Errorf("expected cover to be filled from enricher, got %q", book.ImageURL)
	}
}

func TestAggregator_enrichBook_KeepsExistingCover(t *testing.T) {
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{ImageURL: "https://books.google.com/other.jpg"}},
	}
	agg := &Aggregator{enrichers: []Provider{enricher}, cache: newTTLCache(time.Minute)}

	existing := "https://covers.openlibrary.org/b/id/123-L.jpg"
	book := &models.Book{Title: "Dune", ImageURL: existing}
	agg.enrichBook(context.Background(), book)
	if book.ImageURL != existing {
		t.Errorf("existing cover should not be replaced, got %q", book.ImageURL)
	}
}

func TestAggregator_GetBook_NoCover_EnrichedFromProvider(t *testing.T) {
	// Book has a long description so the old trigger wouldn't fire — but
	// ImageURL is empty so enrichment must still run.
	primary := &mockProvider{
		name: "ol",
		getBook: &models.Book{
			Title:       "21 Lessons for the 21st Century",
			Description: "A sufficiently long description that would previously have skipped enrichment entirely.",
			ImageURL:    "",
		},
	}
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{ImageURL: "https://books.google.com/cover-en.jpg"}},
	}
	agg := &Aggregator{primary: primary, enrichers: []Provider{enricher}, cache: newTTLCache(time.Minute)}

	got, err := agg.GetBook(context.Background(), "OL123W")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.ImageURL != "https://books.google.com/cover-en.jpg" {
		t.Errorf("expected cover from enricher, got %q", got.ImageURL)
	}
}

func TestAggregator_GetAuthorWorks_CoversEnrichedForMissingOnes(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{
			name: "ol",
			authorWorks: []models.Book{
				{ForeignID: "OL1W", Title: "Sapiens", ImageURL: ""},
				{ForeignID: "OL2W", Title: "Homo Deus", ImageURL: "https://covers.openlibrary.org/b/id/999-L.jpg"},
			},
		},
	}
	enricher := &mockProvider{
		name:        "gb",
		searchBooks: []models.Book{{ImageURL: "https://books.google.com/sapiens.jpg"}},
	}
	agg := &Aggregator{primary: primary, enrichers: []Provider{enricher}, cache: newTTLCache(time.Minute)}

	got, err := agg.GetAuthorWorks(context.Background(), "OL123A")
	if err != nil {
		t.Fatalf("GetAuthorWorks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 works, got %d", len(got))
	}
	// Book without OL cover should get enriched cover
	if got[0].ImageURL != "https://books.google.com/sapiens.jpg" {
		t.Errorf("Sapiens: expected enriched cover, got %q", got[0].ImageURL)
	}
	// Book with OL cover should keep it
	if got[1].ImageURL != "https://covers.openlibrary.org/b/id/999-L.jpg" {
		t.Errorf("Homo Deus: expected OL cover preserved, got %q", got[1].ImageURL)
	}
}

func TestAggregator_GetAuthorWorks_NoEnrichersNoCovers(t *testing.T) {
	// With no enrichers, works without covers stay coverless — no panic.
	primary := &mockWorksProvider{
		mockProvider: mockProvider{
			name:        "ol",
			authorWorks: []models.Book{{ForeignID: "OL1W", Title: "No Cover Book", ImageURL: ""}},
		},
	}
	agg := &Aggregator{primary: primary, cache: newTTLCache(time.Minute)}

	got, err := agg.GetAuthorWorks(context.Background(), "OL456A")
	if err != nil {
		t.Fatalf("GetAuthorWorks: %v", err)
	}
	if got[0].ImageURL != "" {
		t.Errorf("expected empty cover with no enrichers, got %q", got[0].ImageURL)
	}
}

func TestTTLCache_SetAndGet(t *testing.T) {
	c := newTTLCache(time.Minute)
	c.set("key1", "value1")
	v, ok := c.get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if v.(string) != "value1" {
		t.Errorf("want 'value1', got %q", v)
	}
}

func TestTTLCache_Miss(t *testing.T) {
	c := newTTLCache(time.Minute)
	_, ok := c.get("missing")
	if ok {
		t.Error("expected cache miss for unknown key")
	}
}

func TestTTLCache_Expiry(t *testing.T) {
	c := newTTLCache(time.Nanosecond)
	c.set("k", "v")
	time.Sleep(2 * time.Millisecond)
	_, ok := c.get("k")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestTTLCache_Cleanup(t *testing.T) {
	c := newTTLCache(time.Nanosecond)
	c.set("a", 1)
	c.set("b", 2)
	time.Sleep(2 * time.Millisecond)
	c.cleanup()

	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()
	if n != 0 {
		t.Errorf("expected 0 items after cleanup, got %d", n)
	}
}

// newTestAggregator creates an aggregator with a real TTL cache.
func newTestAggregator(primary Provider, enrichers ...Provider) *Aggregator {
	return &Aggregator{
		primary:   primary,
		enrichers: enrichers,
		cache:     newTTLCache(time.Minute),
	}
}
