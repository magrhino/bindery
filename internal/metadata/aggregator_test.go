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
	name               string
	searchBooks        []models.Book
	searchBooksByQuery map[string][]models.Book
	searchBookErr      error
	searchAuthors      []models.Author
	searchAuthErr      error
	getAuthor          *models.Author
	getAuthorErr       error
	getBook            *models.Book
	getBookByID        map[string]*models.Book
	getBookErr         error
	getBookCalls       int
	gotBookIDs         []string
	getEditions        []models.Edition
	getEditionsErr     error
	getByISBN          *models.Book
	getByISBNByISBN    map[string]*models.Book
	getByISBNErr       error
	getByISBNCalls     int
	gotISBNs           []string
	searchBookQueries  []string
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
	if m.searchBooksByQuery != nil {
		if books, ok := m.searchBooksByQuery[query]; ok {
			return books, m.searchBookErr
		}
	}
	return m.searchBooks, m.searchBookErr
}
func (m *mockProvider) GetAuthor(_ context.Context, _ string) (*models.Author, error) {
	return m.getAuthor, m.getAuthorErr
}
func (m *mockProvider) GetBook(_ context.Context, foreignID string) (*models.Book, error) {
	m.getBookCalls++
	m.gotBookIDs = append(m.gotBookIDs, foreignID)
	if m.getBookByID != nil {
		return m.getBookByID[foreignID], m.getBookErr
	}
	return m.getBook, m.getBookErr
}
func (m *mockProvider) GetEditions(_ context.Context, _ string) ([]models.Edition, error) {
	return m.getEditions, m.getEditionsErr
}
func (m *mockProvider) GetBookByISBN(_ context.Context, isbn string) (*models.Book, error) {
	m.getByISBNCalls++
	m.gotISBNs = append(m.gotISBNs, isbn)
	if m.getByISBNByISBN != nil {
		return m.getByISBNByISBN[isbn], m.getByISBNErr
	}
	return m.getByISBN, m.getByISBNErr
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalCanonicalQueries(a, b []primaryBookCanonicalQuery) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// worksProvider implementation (optional, only attached when needed).
type mockWorksProvider struct {
	mockProvider
	authorWorksCalls int
}

func (m *mockWorksProvider) GetAuthorWorks(_ context.Context, _ string) ([]models.Book, error) {
	m.authorWorksCalls++
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

func TestAggregator_GetBookByISBN_PrimaryFallbackWinsWhenEnricherDoesNotCanonicalize(t *testing.T) {
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
	if enricher.getByISBNCalls != 1 {
		t.Errorf("enricher calls = %d, want 1 while checking for canonical fallback", enricher.getByISBNCalls)
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
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780593135204": nil,
			"Project Hail Mary Andy Weir": {{
				ForeignID: "OL-PHM",
				Title:     "Project Hail Mary",
				Author:    &models.Author{Name: "Andy Weir"},
			}},
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
		t.Fatalf("got %+v, want canonical OpenLibrary book", got)
	}
	wantQueries := []string{"isbn:9780593135204", "Project Hail Mary Andy Weir"}
	if !equalStringSlices(primary.searchBookQueries, wantQueries) {
		t.Fatalf("search queries = %v, want %v", primary.searchBookQueries, wantQueries)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesSecondaryHitUsingISBNSearch(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780441172719": {{
				ForeignID: "OL-DUNE",
				Title:     "Dune",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"Dune Frank Herbert": {
				{ForeignID: "OL1W", Title: "Dune", Author: &models.Author{Name: "Frank Herbert"}},
				{ForeignID: "OL2W", Title: "Dune", Author: &models.Author{Name: "Frank Herbert"}},
			},
		},
		getBook: &models.Book{
			ForeignID:        "OL-DUNE",
			Title:            "Dune",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:dune",
			Title:            "Dune",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780441172719")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-DUNE" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary book from ISBN search", got)
	}
	wantQueries := []string{"isbn:9780441172719"}
	if !equalStringSlices(primary.searchBookQueries, wantQueries) {
		t.Fatalf("search queries = %v, want %v", primary.searchBookQueries, wantQueries)
	}
}

func TestAggregator_GetBookByISBN_PrimaryISBNSearchDoesNotFallThroughToWrongTitleSearch(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		getByISBN: &models.Book{
			ForeignID:        "OL-CORRECT",
			Title:            "Classic Title",
			Description:      "Primary OpenLibrary ISBN description long enough to avoid enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Jane Author"},
		},
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780000000008": {{
				ForeignID: "OL-CORRECT",
				Title:     "Classic Title",
				Author:    &models.Author{Name: "Jane Author"},
			}},
			"Classic Title Jane Author": {{
				ForeignID: "OL-WRONG",
				Title:     "Classic Title",
				Author:    &models.Author{Name: "Jane Author"},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL-WRONG": {
				ForeignID:        "OL-WRONG",
				Title:            "Classic Title",
				Description:      "Wrong OpenLibrary title-search work description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
		},
	}
	agg := newTestAggregator(primary)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000008")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-CORRECT" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want exact OpenLibrary ISBN work", got)
	}
	wantQueries := []string{"isbn:9780000000008"}
	if !equalStringSlices(primary.searchBookQueries, wantQueries) {
		t.Fatalf("search queries = %v, want %v", primary.searchBookQueries, wantQueries)
	}
	if primary.getBookCalls != 0 {
		t.Fatalf("GetBook calls=%d ids=%v, want no title-search canonical fetch", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_ISBNSearchWinsOverPlausibleWrongTitleSearch(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780000000001": {{
				ForeignID: "OL-CORRECT",
				Title:     "Classic Title",
				Author:    &models.Author{Name: "Jane Author"},
			}},
			"Classic Title Jane Author": {{
				ForeignID: "OL-WRONG",
				Title:     "Classic Title",
				Author:    &models.Author{Name: "Jane Author"},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL-CORRECT": {
				ForeignID:        "OL-CORRECT",
				Title:            "Classic Title",
				Description:      "Correct OpenLibrary ISBN work description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
			"OL-WRONG": {
				ForeignID:        "OL-WRONG",
				Title:            "Classic Title",
				Description:      "Wrong OpenLibrary title-search work description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:classic-title",
			Title:            "Classic Title",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000001")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-CORRECT" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want exact ISBN OpenLibrary work", got)
	}
	wantQueries := []string{"isbn:9780000000001"}
	if !equalStringSlices(primary.searchBookQueries, wantQueries) {
		t.Fatalf("search queries = %v, want %v", primary.searchBookQueries, wantQueries)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesNoiseWordTitleAfterISBNMiss(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780000000003": nil,
			"The Book Jane Author": {{
				ForeignID: "OL-THE-BOOK",
				Title:     "The Book",
				Author:    &models.Author{Name: "Jane Author"},
			}},
		},
		getBook: &models.Book{
			ForeignID:        "OL-THE-BOOK",
			Title:            "The Book",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:the-book",
			Title:            "The Book",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000003")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-THE-BOOK" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary book", got)
	}
	wantQueries := []string{"isbn:9780000000003", "The Book Jane Author"}
	if !equalStringSlices(primary.searchBookQueries, wantQueries) {
		t.Fatalf("search queries = %v, want %v", primary.searchBookQueries, wantQueries)
	}
}

func TestPrimaryBookCanonicalQueries_OrdersFullTitlesBeforeDerivedSegments(t *testing.T) {
	tests := []struct {
		title string
		want  []primaryBookCanonicalQuery
	}{
		{
			title: "Dune – Der Wüstenplanet",
			want: []primaryBookCanonicalQuery{
				{query: "Dune – Der Wüstenplanet Frank Herbert", matchTitle: "Dune – Der Wüstenplanet", allowEditionTitleMatch: true},
				{query: "title:Dune – Der Wüstenplanet author:Frank Herbert", matchTitle: "Dune – Der Wüstenplanet", allowEditionTitleMatch: true},
				{query: "Dune Frank Herbert", matchTitle: "Dune", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantRightSegment},
				{query: "title:Dune author:Frank Herbert", matchTitle: "Dune", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantRightSegment},
			},
		},
		{
			title: "Dune – Der Wüstenplanet: Roman",
			want: []primaryBookCanonicalQuery{
				{query: "Dune – Der Wüstenplanet: Roman Frank Herbert", matchTitle: "Dune – Der Wüstenplanet: Roman", allowEditionTitleMatch: true},
				{query: "title:Dune – Der Wüstenplanet: Roman author:Frank Herbert", matchTitle: "Dune – Der Wüstenplanet: Roman", allowEditionTitleMatch: true},
				{query: "Dune – Der Wüstenplanet Frank Herbert", matchTitle: "Dune – Der Wüstenplanet", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantDescriptor},
				{query: "title:Dune – Der Wüstenplanet author:Frank Herbert", matchTitle: "Dune – Der Wüstenplanet", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantDescriptor},
				{query: "Dune Frank Herbert", matchTitle: "Dune", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantRightSegment},
				{query: "title:Dune author:Frank Herbert", matchTitle: "Dune", exactTitleOnly: true, allowRankTieBreak: true, allowEditionTitleMatch: true, variantKind: canonicalTitleVariantRightSegment},
			},
		},
	}

	for _, tc := range tests {
		got := primaryBookCanonicalQueries("", tc.title, "Frank Herbert", "ger")
		if len(got) < len(tc.want) {
			t.Fatalf("queries for %q = %+v, want at least %d queries", tc.title, got, len(tc.want))
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("queries for %q [%d] = %+v, want %+v (all queries=%+v)", tc.title, i, got[i], tc.want[i], got)
			}
		}
	}
}

func TestPrimaryBookCanonicalQueries_KeepsOriginalNoiseWordTitle(t *testing.T) {
	got := primaryBookCanonicalQueries("", "The Book", "Jane Author", "eng")
	want := []primaryBookCanonicalQuery{
		{query: "The Book Jane Author", matchTitle: "The Book", allowEditionTitleMatch: true},
		{query: "title:The Book author:Jane Author", matchTitle: "The Book", allowEditionTitleMatch: true},
	}
	if !equalCanonicalQueries(got, want) {
		t.Fatalf("queries = %+v, want %+v", got, want)
	}
}

func TestPrimaryBookCanonicalQueries_DoesNotStripRealTrailingDescriptorWords(t *testing.T) {
	got := primaryBookCanonicalQueries("", "Data Science", "Jane Author", "eng")
	for _, query := range got {
		if query.matchTitle == "Data" || query.query == "Data Jane Author" || query.query == "title:Data author:Jane Author" {
			t.Fatalf("generated unsafe derived title query %+v from Data Science (all queries=%+v)", query, got)
		}
	}
}

func TestPrimaryBookCanonicalQueries_GatesGermanNoPunctuationFallbackByLanguageAndDescriptor(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		author   string
		language string
		query    string
		want     bool
	}{
		{
			name:     "german three letter code",
			title:    "Dune der Wüstenplanet Roman",
			author:   "Frank Herbert",
			language: "ger",
			query:    "Dune Frank Herbert",
			want:     true,
		},
		{
			name:     "german two letter code",
			title:    "Dune der Wüstenplanet Roman",
			author:   "Frank Herbert",
			language: "de",
			query:    "Dune Frank Herbert",
			want:     true,
		},
		{
			name:     "german title without descriptor",
			title:    "Dune der Wüstenplanet",
			author:   "Frank Herbert",
			language: "ger",
			query:    "Dune Frank Herbert",
			want:     false,
		},
		{
			name:     "ordinary german genitive title",
			title:    "Haus der Sonne",
			author:   "Jane Author",
			language: "ger",
			query:    "Haus Jane Author",
			want:     false,
		},
		{
			name:     "unknown language",
			title:    "Dune der Wüstenplanet Roman",
			author:   "Frank Herbert",
			language: "",
			query:    "Dune Frank Herbert",
			want:     false,
		},
		{
			name:     "english die verb",
			title:    "Never Die Alone",
			author:   "Donald Goines",
			language: "eng",
			query:    "Never Donald Goines",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := primaryBookCanonicalQueries("", tc.title, tc.author, tc.language)
			found := false
			for _, query := range got {
				if query.query == tc.query {
					found = true
					break
				}
			}
			if found != tc.want {
				t.Fatalf("query %q present=%v, want %v (all queries=%+v)", tc.query, found, tc.want, got)
			}
		})
	}
}

func TestAggregator_GetBookByISBN_DoesNotApplyGermanFallbackToEnglishDieTitle(t *testing.T) {
	tests := []struct {
		name     string
		language string
	}{
		{name: "english language", language: "eng"},
		{name: "unknown language", language: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			primary := &mockProvider{
				name: "openlibrary",
				searchBooksByQuery: map[string][]models.Book{
					"Never Donald Goines": {{
						ForeignID: "OL-NEVER",
						Title:     "Never",
						Author:    &models.Author{Name: "Donald Goines"},
					}},
				},
				getBook: &models.Book{
					ForeignID:        "OL-NEVER",
					Title:            "Never",
					Description:      "Wrong OpenLibrary description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Donald Goines"},
				},
			}
			google := &mockProvider{
				name: "googlebooks",
				getByISBN: &models.Book{
					ForeignID:        "gb:never-die-alone",
					Title:            "Never Die Alone",
					Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
					Language:         tc.language,
					MetadataProvider: "googlebooks",
					Author:           &models.Author{Name: "Donald Goines"},
				},
			}
			agg := newTestAggregator(primary, google)

			got, err := agg.GetBookByISBN(context.Background(), "9780000000002")
			if err != nil {
				t.Fatalf("GetBookByISBN: %v", err)
			}
			if got == nil || got.ForeignID != "gb:never-die-alone" || got.MetadataProvider != "googlebooks" {
				t.Fatalf("got %+v, want original Google Books result", got)
			}
			if primary.getBookCalls != 0 {
				t.Fatalf("GetBook calls=%d ids=%v, want no German fallback canonical fetch", primary.getBookCalls, primary.gotBookIDs)
			}
			for _, query := range primary.searchBookQueries {
				if query == "Never Donald Goines" {
					t.Fatalf("generated German fallback query for English title: %v", primary.searchBookQueries)
				}
			}
		})
	}
}

func TestAggregator_GetBookByISBN_DoesNotApplyGermanFirstWordFallbackWithoutDescriptor(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Haus Jane Author": {{
				ForeignID: "OL-HAUS",
				Title:     "Haus",
				Author:    &models.Author{Name: "Jane Author"},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL-HAUS": {
				ForeignID:        "OL-HAUS",
				Title:            "Haus",
				Description:      "Wrong OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:haus-der-sonne",
			Title:            "Haus der Sonne",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			Language:         "ger",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000010")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "gb:haus-der-sonne" || got.MetadataProvider != "googlebooks" {
		t.Fatalf("got %+v, want original Google Books result", got)
	}
	if primary.getBookCalls != 0 {
		t.Fatalf("GetBook calls=%d ids=%v, want no German first-word canonical fetch", primary.getBookCalls, primary.gotBookIDs)
	}
	for _, query := range primary.searchBookQueries {
		if query == "Haus Jane Author" || query == "title:Haus author:Jane Author" {
			t.Fatalf("generated unsafe German first-word query: %v", primary.searchBookQueries)
		}
	}
}

func TestAggregator_GetBookByISBN_DoesNotCanonicalizeRealTrailingDescriptorWord(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9780000000009": nil,
			"Data Jane Author": {{
				ForeignID: "OL-DATA",
				Title:     "Data",
				Author:    &models.Author{Name: "Jane Author"},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL-DATA": {
				ForeignID:        "OL-DATA",
				Title:            "Data",
				Description:      "Wrong OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:data-science",
			Title:            "Data Science",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000009")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "gb:data-science" || got.MetadataProvider != "googlebooks" {
		t.Fatalf("got %+v, want original Google Books result", got)
	}
	if primary.getBookCalls != 0 {
		t.Fatalf("GetBook calls=%d ids=%v, want no unsafe trailing-word canonical fetch", primary.getBookCalls, primary.gotBookIDs)
	}
	for _, query := range primary.searchBookQueries {
		if query == "Data Jane Author" || query == "title:Data author:Jane Author" {
			t.Fatalf("generated unsafe trailing-word query: %v", primary.searchBookQueries)
		}
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesPrimaryOpenLibraryDuplicateWork(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{
			name: "openlibrary",
			getByISBN: &models.Book{
				ForeignID:        "OL26431102W",
				Title:            "Dune – Der Wüstenplanet",
				Description:      "Duplicate OpenLibrary work description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert", ForeignID: "OL79034A", MetadataProvider: "openlibrary"},
			},
			searchBooksByQuery: map[string][]models.Book{
				"isbn:9783453321229": {{
					ForeignID: "OL26431102W",
					Title:     "Dune – Der Wüstenplanet",
					Author:    &models.Author{Name: "Frank Herbert"},
				}},
				"Dune – Der Wüstenplanet Frank Herbert": {{
					ForeignID: "OL26431102W",
					Title:     "Dune – Der Wüstenplanet",
					Author:    &models.Author{Name: "Frank Herbert"},
				}},
			},
			getBook: &models.Book{
				ForeignID:        "OL893415W",
				Title:            "Dune",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert"},
			},
			authorWorks: []models.Book{
				{ForeignID: "OL26431102W", Title: "Dune – Der Wüstenplanet"},
				{ForeignID: "OL893415W", Title: "Dune"},
			},
		},
	}
	agg := newTestAggregator(primary)

	got, err := agg.GetBookByISBN(context.Background(), "9783453321229")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary work", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
	if len(primary.searchBookQueries) == 0 || primary.searchBookQueries[0] != "isbn:9783453321229" {
		t.Fatalf("first search query = %v, want isbn:9783453321229 first", primary.searchBookQueries)
	}
}

func TestAggregator_GetBookByISBN_ContinuesPrimaryFallbackForSecondaryCanonicalWork(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		getByISBN: &models.Book{
			ForeignID:        "OL24742012W",
			Title:            "DUNE",
			Description:      "Wrong OpenLibrary ISBN hit description long enough to avoid enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Brian Herbert"},
		},
		searchBooksByQuery: map[string][]models.Book{
			"isbn:9783453321229": {{
				ForeignID: "OL24742012W",
				Title:     "DUNE",
				Author:    &models.Author{Name: "Brian Herbert"},
			}},
			"Dune – Der Wüstenplanet: Roman Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"title:Dune – Der Wüstenplanet: Roman author:Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"Dune – Der Wüstenplanet Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"title:Dune – Der Wüstenplanet author:Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"Dune Frank Herbert": {
				{
					ForeignID: "OL893415W",
					Title:     "Dune",
					Author:    &models.Author{Name: "Frank Herbert"},
				},
				{
					ForeignID: "OL24742012W",
					Title:     "DUNE",
					Author:    &models.Author{Name: "Brian Herbert"},
				},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL893415W": {
				ForeignID:        "OL893415W",
				Title:            "Dune",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert"},
			},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:1285431693",
			Title:            "Dune – Der Wüstenplanet: Roman",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			Language:         "ger",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453321229")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want secondary-canonicalized OpenLibrary work", got)
	}
	if primary.getByISBNCalls != 1 || dnb.getByISBNCalls != 1 {
		t.Fatalf("GetBookByISBN calls primary=%d dnb=%d, want 1/1", primary.getByISBNCalls, dnb.getByISBNCalls)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_ReusesCachedAuthorWorksForPrimaryCanonicalization(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{
			name: "openlibrary",
			getByISBNByISBN: map[string]*models.Book{
				"9783453321229": {
					ForeignID:        "OL-DUNE-DE",
					Title:            "Dune – Der Wüstenplanet",
					Description:      "Duplicate OpenLibrary work description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Frank Herbert", ForeignID: "OL79034A", MetadataProvider: "openlibrary"},
				},
				"9783453321243": {
					ForeignID:        "OL-MESSIAH-DE",
					Title:            "Dune Messiah – Der Herr des Wüstenplaneten",
					Description:      "Duplicate OpenLibrary work description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Frank Herbert", ForeignID: "OL79034A", MetadataProvider: "openlibrary"},
				},
			},
			searchBooksByQuery: map[string][]models.Book{
				"isbn:9783453321229": {{
					ForeignID: "OL-DUNE-DE",
					Title:     "Dune – Der Wüstenplanet",
					Author:    &models.Author{Name: "Frank Herbert"},
				}},
				"isbn:9783453321243": {{
					ForeignID: "OL-MESSIAH-DE",
					Title:     "Dune Messiah – Der Herr des Wüstenplaneten",
					Author:    &models.Author{Name: "Frank Herbert"},
				}},
			},
			getBookByID: map[string]*models.Book{
				"OL-DUNE": {
					ForeignID:        "OL-DUNE",
					Title:            "Dune",
					Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Frank Herbert"},
				},
				"OL-MESSIAH": {
					ForeignID:        "OL-MESSIAH",
					Title:            "Dune Messiah",
					Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Frank Herbert"},
				},
			},
			authorWorks: []models.Book{
				{ForeignID: "OL-DUNE-DE", Title: "Dune – Der Wüstenplanet"},
				{ForeignID: "OL-DUNE", Title: "Dune"},
				{ForeignID: "OL-MESSIAH-DE", Title: "Dune Messiah – Der Herr des Wüstenplaneten"},
				{ForeignID: "OL-MESSIAH", Title: "Dune Messiah"},
			},
		},
	}
	agg := newTestAggregator(primary)

	first, err := agg.GetBookByISBN(context.Background(), "9783453321229")
	if err != nil {
		t.Fatalf("GetBookByISBN first: %v", err)
	}
	second, err := agg.GetBookByISBN(context.Background(), "9783453321243")
	if err != nil {
		t.Fatalf("GetBookByISBN second: %v", err)
	}
	if first == nil || first.ForeignID != "OL-DUNE" || first.MetadataProvider != "openlibrary" {
		t.Fatalf("first = %+v, want OL-DUNE", first)
	}
	if second == nil || second.ForeignID != "OL-MESSIAH" || second.MetadataProvider != "openlibrary" {
		t.Fatalf("second = %+v, want OL-MESSIAH", second)
	}
	if primary.authorWorksCalls != 1 {
		t.Fatalf("author works calls = %d, want 1", primary.authorWorksCalls)
	}
	if primary.getBookCalls != 2 || !equalStringSlices(primary.gotBookIDs, []string{"OL-DUNE", "OL-MESSIAH"}) {
		t.Fatalf("GetBook calls=%d ids=%v, want OL-DUNE and OL-MESSIAH", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_AuthorWorksCanonicalizationDoesNotEnrichAllWorks(t *testing.T) {
	primary := &mockWorksProvider{
		mockProvider: mockProvider{
			name: "openlibrary",
			getByISBN: &models.Book{
				ForeignID:        "OL-DUNE-DE",
				Title:            "Dune – Der Wüstenplanet",
				Description:      "Duplicate OpenLibrary work description long enough to avoid enrichment.",
				Language:         "ger",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert", ForeignID: "OL79034A", MetadataProvider: "openlibrary"},
			},
			getBookByID: map[string]*models.Book{
				"OL-DUNE": {
					ForeignID:        "OL-DUNE",
					Title:            "Dune",
					Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
					MetadataProvider: "openlibrary",
					Author:           &models.Author{Name: "Frank Herbert"},
				},
			},
			authorWorks: []models.Book{
				{ForeignID: "OL-DUNE-DE", Title: "Dune – Der Wüstenplanet"},
				{ForeignID: "OL-DUNE", Title: "Dune"},
				{ForeignID: "OL-MESSIAH", Title: "Dune Messiah"},
			},
		},
	}
	enricher := &mockProvider{
		name:        "googlebooks",
		searchBooks: []models.Book{{ImageURL: "https://books.google.com/cover.jpg"}},
	}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.GetBookByISBN(context.Background(), "9783453321229")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-DUNE" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary work", got)
	}
	if primary.authorWorksCalls != 1 {
		t.Fatalf("author works calls = %d, want 1", primary.authorWorksCalls)
	}
	if len(enricher.searchBookQueries) != 0 {
		t.Fatalf("enricher SearchBooks queries = %v, want none during author-works canonicalization", enricher.searchBookQueries)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesDNBNoisyTranslatedTitle(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Dune – Der Wüstenplanet: Roman Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"title:Dune – Der Wüstenplanet: Roman author:Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"Dune – Der Wüstenplanet Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"title:Dune – Der Wüstenplanet author:Frank Herbert": {{
				ForeignID: "OL26431102W",
				Title:     "Dune – Der Wüstenplanet",
				Author:    &models.Author{Name: "Frank Herbert"},
			}},
			"Dune Frank Herbert": {
				{
					ForeignID: "OL893415W",
					Title:     "Dune",
					Author:    &models.Author{Name: "Frank Herbert"},
				},
				{
					ForeignID: "OL27969474W",
					Title:     "Dune",
					Author:    &models.Author{Name: "Frank Herbert"},
				},
			},
		},
		getBook: &models.Book{
			ForeignID:        "OL893415W",
			Title:            "Dune",
			Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:1285431693",
			Title:            "Dune – Der Wüstenplanet: Roman",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			Language:         "ger",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453323131")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary work", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesGermanEditionTitleMatchToTopRankedWork(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Der Wüstenplanet Frank Herbert": {
				{
					ForeignID: "OL893415W",
					Title:     "Dune",
					Author:    &models.Author{Name: "Frank Herbert"},
					Editions: []models.Edition{{
						ForeignID: "OL32663508M",
						Title:     "Der Wüstenplanet.",
						Language:  "ger",
					}},
				},
				{ForeignID: "OL893502W", Title: "Heretics of Dune", Author: &models.Author{Name: "Frank Herbert"}},
				{
					ForeignID: "OL26431102W",
					Title:     "Dune – Der Wüstenplanet",
					Author:    &models.Author{Name: "Frank Herbert"},
				},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL893415W": {
				ForeignID:        "OL893415W",
				Title:            "Dune",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert"},
			},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:1070335703",
			Title:            "Der Wüstenplanet",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			Language:         "ger",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Herbert, Frank"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453317178")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary edition-title match", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_UsesEditionTitleMatchWithoutSourceLanguage(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Der Wüstenplanet Frank Herbert": {{
				ForeignID: "OL893415W",
				Title:     "Dune",
				Author:    &models.Author{Name: "Frank Herbert"},
				Editions: []models.Edition{{
					ForeignID: "OL32663508M",
					Title:     "Der Wüstenplanet",
					Language:  "ger",
				}},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL893415W": {
				ForeignID:        "OL893415W",
				Title:            "Dune",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert"},
			},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:1070335703",
			Title:            "Der Wüstenplanet",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453317178")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary edition-title match", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesDerivedTitleExactDuplicateByRank(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Dune Frank Herbert": {
				{ForeignID: "OL893415W", Title: "Dune", Author: &models.Author{Name: "Frank Herbert"}},
				{ForeignID: "OL27969474W", Title: "Dune", Author: &models.Author{Name: "Frank Herbert"}},
			},
		},
		getBook: &models.Book{
			ForeignID:        "OL893415W",
			Title:            "Dune",
			Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:1070335703",
			Title:            "Dune der Wüstenplanet Roman",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			Language:         "ger",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Herbert, Frank"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453317178")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL893415W" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want top-ranked exact OpenLibrary work", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL893415W" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL893415W", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_CanonicalPrimaryBookUsesRankForExactTitleAuthorDuplicates(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Cien años de soledad Gabriel García Márquez": {
				{
					ForeignID: "OL274505W",
					Title:     "Cien años de soledad",
					Author:    &models.Author{Name: "Gabriel García Márquez"},
				},
				{
					ForeignID: "OL28027117W",
					Title:     "Cien Años de Soledad",
					Author:    &models.Author{Name: "Gabriel García Márquez"},
				},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL274505W": {
				ForeignID:        "OL274505W",
				Title:            "Cien años de soledad",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Gabriel García Márquez"},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "Cien años de soledad",
		Author: &models.Author{Name: "Gabriel García Márquez"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL274505W" {
		t.Fatalf("got %+v, want top-ranked exact OpenLibrary work", got)
	}
}

func TestAggregator_CanonicalPrimaryBookPrefersPrimaryAuthorOverAliasDuplicate(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"One Hundred Years of Solitude Gabriel Garcia Marquez": {
				{
					ForeignID: "OL274505W",
					Title:     "Cien años de soledad",
					Author:    &models.Author{Name: "Gabriel García Márquez"},
					Editions: []models.Edition{{
						ForeignID: "OL30448691M",
						Title:     "One Hundred Years of Solitude",
						Language:  "eng",
					}},
				},
				{
					ForeignID: "OL29355621W",
					Title:     "One Hundred Years of Solitude",
					Author: &models.Author{
						Name:           "Gregory Rabassa",
						AlternateNames: []string{"Gabriel Garcia Marquez", "Gabriel García Márquez"},
					},
				},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL274505W": {
				ForeignID:        "OL274505W",
				Title:            "Cien años de soledad",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Gabriel García Márquez"},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "One Hundred Years of Solitude",
		Author: &models.Author{Name: "Gabriel Garcia Marquez"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL274505W" {
		t.Fatalf("got %+v, want canonical Cien años work", got)
	}
}

func TestAggregator_CanonicalPrimaryBookRejectsCompilationBeforeSwappedAuthorQuery(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"The Three-Body Problem Liu Cixin": {{
				ForeignID: "OL44576333W",
				Title:     "Cixin Liu Bestselling Collecting Books Series, Set of 4 Books. the Three-Body Problem, the Wandering Earth, the Dark Forest and Death's End",
				Author:    &models.Author{Name: "Cixin Liu"},
			}},
			"The Three-Body Problem Cixin Liu": {{
				ForeignID: "OL17267881W",
				Title:     "三体",
				Author: &models.Author{
					Name:           "刘慈欣",
					AlternateNames: []string{"Liu Cixin", "Cixin Liu"},
				},
				Editions: []models.Edition{{
					ForeignID: "OL25840917M",
					Title:     "The Three-Body Problem",
					Language:  "eng",
				}},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL17267881W": {
				ForeignID:        "OL17267881W",
				Title:            "三体",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author: &models.Author{
					Name:           "刘慈欣",
					AlternateNames: []string{"Liu Cixin", "Cixin Liu"},
				},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "The Three-Body Problem",
		Author: &models.Author{Name: "Liu Cixin"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL17267881W" {
		t.Fatalf("got %+v, want canonical Three-Body work", got)
	}
	if len(primary.searchBookQueries) < 3 || primary.searchBookQueries[2] != "The Three-Body Problem Cixin Liu" {
		t.Fatalf("search queries = %v, want swapped author query after rejecting compilation", primary.searchBookQueries)
	}
}

func TestAggregator_CanonicalPrimaryBookUsesSwappedEastAsianAuthorForCJKTitle(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"三体 Liu Cixin": {{
				ForeignID: "OL17267881W",
				Title:     "三体",
				Author: &models.Author{
					Name:           "刘慈欣",
					AlternateNames: []string{"Liu Cixin", "Cixin Liu"},
				},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL17267881W": {
				ForeignID:        "OL17267881W",
				Title:            "三体",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author: &models.Author{
					Name:           "刘慈欣",
					AlternateNames: []string{"Liu Cixin", "Cixin Liu"},
				},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "三体",
		Author: &models.Author{Name: "Cixin Liu"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL17267881W" {
		t.Fatalf("got %+v, want canonical Three-Body work", got)
	}
}

func TestAggregator_CanonicalPrimaryBookMatchesEditionTitleWithAuthorAlias(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"The Master and Margarita Mikhail Bulgakov": {{
				ForeignID: "OL676009W",
				Title:     "Мастер и Маргарита",
				Author: &models.Author{
					Name:           "Михаил Афанасьевич Булгаков",
					AlternateNames: []string{"Mikhail Bulgakov", "Bulgakov, Mikhail"},
				},
				Editions: []models.Edition{{
					ForeignID: "OL1413669M",
					Title:     "The Master & Margarita",
					Language:  "eng",
				}},
			}},
		},
		getBookByID: map[string]*models.Book{
			"OL676009W": {
				ForeignID:        "OL676009W",
				Title:            "Мастер и Маргарита",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author: &models.Author{
					Name:           "Михаил Афанасьевич Булгаков",
					AlternateNames: []string{"Mikhail Bulgakov", "Bulgakov, Mikhail"},
				},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "The Master and Margarita",
		Author: &models.Author{Name: "Mikhail Bulgakov"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL676009W" {
		t.Fatalf("got %+v, want canonical Master and Margarita work", got)
	}
}

func TestAggregator_CanonicalPrimaryBookUsesEditionArticleEquivalenceByRank(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Master and Margarita Mikhail Bulgakov": {
				{
					ForeignID: "OL676009W",
					Title:     "Мастер и Маргарита",
					Author: &models.Author{
						Name:           "Михаил Афанасьевич Булгаков",
						AlternateNames: []string{"Mikhail Bulgakov", "Bulgakov, Mikhail"},
					},
					Editions: []models.Edition{{
						ForeignID: "OL1413669M",
						Title:     "The Master & Margarita",
						Language:  "eng",
					}},
				},
				{
					ForeignID: "OL39795340W",
					Title:     "Master and Margarita",
					Author: &models.Author{
						Name:           "Михаил Афанасьевич Булгаков",
						AlternateNames: []string{"Mikhail Bulgakov", "Bulgakov, Mikhail"},
					},
				},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL676009W": {
				ForeignID:        "OL676009W",
				Title:            "Мастер и Маргарита",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author: &models.Author{
					Name:           "Михаил Афанасьевич Булгаков",
					AlternateNames: []string{"Mikhail Bulgakov", "Bulgakov, Mikhail"},
				},
			},
		},
	}
	agg := newTestAggregator(primary)
	source := models.Book{
		Title:  "Master and Margarita",
		Author: &models.Author{Name: "Mikhail Bulgakov"},
	}

	got, ok := agg.canonicalPrimaryBook(context.Background(), "", source)
	if !ok {
		t.Fatal("canonicalPrimaryBook ok = false, want true")
	}
	if got == nil || got.ForeignID != "OL676009W" {
		t.Fatalf("got %+v, want canonical Master and Margarita work", got)
	}
}

func TestAggregator_GetBookByISBN_PrefersRightSubtitleBeforeLeftFallback(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"The Hunger Games: Catching Fire Suzanne Collins": {
				{ForeignID: "OL-HUNGER-GAMES", Title: "The Hunger Games", Author: &models.Author{Name: "Suzanne Collins"}},
			},
			"Catching Fire Suzanne Collins": {
				{ForeignID: "OL-CATCHING-FIRE", Title: "Catching Fire", Author: &models.Author{Name: "Suzanne Collins"}},
			},
			"The Hunger Games Suzanne Collins": {
				{ForeignID: "OL-HUNGER-GAMES", Title: "The Hunger Games", Author: &models.Author{Name: "Suzanne Collins"}},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL-CATCHING-FIRE": {
				ForeignID:        "OL-CATCHING-FIRE",
				Title:            "Catching Fire",
				Description:      "OpenLibrary description long enough to avoid extra enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Suzanne Collins"},
			},
			"OL-HUNGER-GAMES": {
				ForeignID:        "OL-HUNGER-GAMES",
				Title:            "The Hunger Games",
				Description:      "OpenLibrary parent description long enough to avoid extra enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Suzanne Collins"},
			},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:catching-fire",
			Title:            "The Hunger Games: Catching Fire",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Suzanne Collins"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780439023498")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-CATCHING-FIRE" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want Catching Fire OpenLibrary work", got)
	}
	if !equalStringSlices(primary.gotBookIDs, []string{"OL-CATCHING-FIRE"}) {
		t.Fatalf("GetBook IDs = %v, want only OL-CATCHING-FIRE", primary.gotBookIDs)
	}
	for _, query := range primary.searchBookQueries {
		if query == "The Hunger Games Suzanne Collins" {
			t.Fatalf("searched left fallback before returning right subtitle match: %v", primary.searchBookQueries)
		}
	}
}

func TestAggregator_GetBookByISBN_PreservesExactFullTitleOverSegmentFallback(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Foo: Bar Jane Author": {
				{ForeignID: "OL-FOO-BAR", Title: "Foo: Bar", Author: &models.Author{Name: "Jane Author"}},
			},
			"Bar Jane Author": {
				{ForeignID: "OL-BAR", Title: "Bar", Author: &models.Author{Name: "Jane Author"}},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL-FOO-BAR": {
				ForeignID:        "OL-FOO-BAR",
				Title:            "Foo: Bar",
				Description:      "OpenLibrary exact full-title description long enough to avoid extra enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
			"OL-BAR": {
				ForeignID:        "OL-BAR",
				Title:            "Bar",
				Description:      "OpenLibrary segment-title description long enough to avoid extra enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Jane Author"},
			},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:foo-bar",
			Title:            "Foo: Bar",
			Description:      "Google Books description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Jane Author"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780000000011")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-FOO-BAR" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want exact full-title OpenLibrary work", got)
	}
	if !equalStringSlices(primary.gotBookIDs, []string{"OL-FOO-BAR"}) {
		t.Fatalf("GetBook IDs = %v, want only OL-FOO-BAR", primary.gotBookIDs)
	}
	for _, query := range primary.searchBookQueries {
		if query == "Bar Jane Author" {
			t.Fatalf("searched segment fallback before returning exact full-title match: %v", primary.searchBookQueries)
		}
	}
}

func TestAggregator_GetBookByISBN_FallsBackToLeftTitleWhenRightSubtitleMisses(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"The Body Keeps the Score: Brain, Mind, and Body in the Healing of Trauma Bessel van der Kolk": {
				{ForeignID: "OL-BODY", Title: "The Body Keeps the Score", Author: &models.Author{Name: "Bessel van der Kolk"}},
			},
			"The Body Keeps the Score Bessel van der Kolk": {
				{ForeignID: "OL-BODY", Title: "The Body Keeps the Score", Author: &models.Author{Name: "Bessel van der Kolk"}},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL-BODY": {
				ForeignID:        "OL-BODY",
				Title:            "The Body Keeps the Score",
				Description:      "OpenLibrary description long enough to avoid extra enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Bessel van der Kolk"},
			},
		},
	}
	hardcover := &mockProvider{
		name: "hardcover",
		getByISBN: &models.Book{
			ForeignID:        "hc:the-body-keeps-the-score",
			Title:            "The Body Keeps the Score: Brain, Mind, and Body in the Healing of Trauma",
			Description:      "Hardcover description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "hardcover",
			Author:           &models.Author{Name: "Bessel van der Kolk"},
		},
	}
	agg := newTestAggregator(primary, hardcover)

	got, err := agg.GetBookByISBN(context.Background(), "9780143127741")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-BODY" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want left-title OpenLibrary work", got)
	}
	if !equalStringSlices(primary.gotBookIDs, []string{"OL-BODY"}) {
		t.Fatalf("GetBook IDs = %v, want only OL-BODY", primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_DerivedPartialCandidateDoesNotBeatExactTitleCandidate(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Dune Frank Herbert": {
				{ForeignID: "OL-DUNE-MESSIAH", Title: "Dune Messiah", Author: &models.Author{Name: "Frank Herbert"}},
				{ForeignID: "OL-DUNE", Title: "Dune", Author: &models.Author{Name: "Frank Herbert"}},
			},
		},
		getBookByID: map[string]*models.Book{
			"OL-DUNE": {
				ForeignID:        "OL-DUNE",
				Title:            "Dune",
				Description:      "Canonical OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "Frank Herbert"},
			},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:dune-translated",
			Title:            "Dune – Der Wüstenplanet",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453317178")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-DUNE" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want exact title candidate over partial derived match", got)
	}
	if primary.getBookCalls != 1 || primary.gotBookIDs[0] != "OL-DUNE" {
		t.Fatalf("GetBook calls=%d ids=%v, want OL-DUNE", primary.getBookCalls, primary.gotBookIDs)
	}
}

func TestAggregator_GetBookByISBN_KeepsSecondaryWhenDerivedTitleHasOnlyPartialCandidates(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooksByQuery: map[string][]models.Book{
			"Dune Frank Herbert": {
				{ForeignID: "OL-DUNE-MESSIAH", Title: "Dune Messiah", Author: &models.Author{Name: "Frank Herbert"}},
				{ForeignID: "OL-HERETICS", Title: "Heretics of Dune", Author: &models.Author{Name: "Frank Herbert"}},
			},
		},
	}
	dnb := &mockProvider{
		name: "dnb",
		getByISBN: &models.Book{
			ForeignID:        "dnb:dune-translated",
			Title:            "Dune – Der Wüstenplanet",
			Description:      "DNB description long enough to avoid enrichment if canonicalization fails.",
			MetadataProvider: "dnb",
			Author:           &models.Author{Name: "Frank Herbert"},
		},
	}
	agg := newTestAggregator(primary, dnb)

	got, err := agg.GetBookByISBN(context.Background(), "9783453317178")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "dnb:dune-translated" || got.MetadataProvider != "dnb" {
		t.Fatalf("got %+v, want original DNB result when derived title has only partial primary matches", got)
	}
	if primary.getBookCalls != 0 {
		t.Fatalf("GetBook calls=%d ids=%v, want no canonical primary fetch", primary.getBookCalls, primary.gotBookIDs)
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

func TestAggregator_GetBookByISBN_CanonicalizesSecondaryHitWithSubtitleAndCaseVariant(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{{
			ForeignID: "OL-BODY",
			Title:     "The Body Keeps the Score",
			Author:    &models.Author{Name: "Bessel van der Kolk"},
		}},
		getBook: &models.Book{
			ForeignID:        "OL-BODY",
			Title:            "The Body Keeps the Score",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Bessel van der Kolk"},
		},
	}
	hardcover := &mockProvider{
		name: "hardcover",
		getByISBN: &models.Book{
			ForeignID:        "hc:the-body-keeps-the-score",
			Title:            "THE BODY KEEPS THE SCORE: Brain, Mind, and Body in the Healing of Trauma",
			MetadataProvider: "hardcover",
			Author:           &models.Author{Name: "Bessel van der Kolk"},
		},
	}
	agg := newTestAggregator(primary, hardcover)

	got, err := agg.GetBookByISBN(context.Background(), "9780143127741")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-BODY" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary book", got)
	}
}

func TestAggregator_GetBookByISBN_CanonicalizesSecondaryHitWithAuthorPunctuationVariant(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{{
			ForeignID: "OL-LHOD",
			Title:     "The Left Hand of Darkness",
			Author:    &models.Author{Name: "Ursula K. Le Guin"},
		}},
		getBook: &models.Book{
			ForeignID:        "OL-LHOD",
			Title:            "The Left Hand of Darkness",
			Description:      "OpenLibrary canonical description long enough to avoid extra enrichment.",
			MetadataProvider: "openlibrary",
			Author:           &models.Author{Name: "Ursula K. Le Guin"},
		},
	}
	google := &mockProvider{
		name: "googlebooks",
		getByISBN: &models.Book{
			ForeignID:        "gb:left-hand",
			Title:            "The Left Hand of Darkness",
			MetadataProvider: "googlebooks",
			Author:           &models.Author{Name: "Ursula Le Guin"},
		},
	}
	agg := newTestAggregator(primary, google)

	got, err := agg.GetBookByISBN(context.Background(), "9780441478125")
	if err != nil {
		t.Fatalf("GetBookByISBN: %v", err)
	}
	if got == nil || got.ForeignID != "OL-LHOD" || got.MetadataProvider != "openlibrary" {
		t.Fatalf("got %+v, want canonical OpenLibrary book", got)
	}
}

func TestAggregator_GetBookByISBN_UsesPrimarySearchRankForExactTitleAuthorDuplicates(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchBooks: []models.Book{
			{ForeignID: "OL1W", Title: "The Book", Author: &models.Author{Name: "A. Author"}},
			{ForeignID: "OL2W", Title: "The Book", Author: &models.Author{Name: "A. Author"}},
		},
		getBookByID: map[string]*models.Book{
			"OL1W": {
				ForeignID:        "OL1W",
				Title:            "The Book",
				Description:      "OpenLibrary description long enough to avoid enrichment.",
				MetadataProvider: "openlibrary",
				Author:           &models.Author{Name: "A. Author"},
			},
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
	if got == nil || got.ForeignID != "OL1W" {
		t.Fatalf("got %+v, want top-ranked primary result", got)
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
