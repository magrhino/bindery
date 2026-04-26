package metadata

import (
	"context"

	"github.com/vavallee/bindery/internal/models"
)

// Provider defines the interface that all metadata sources must implement.
type Provider interface {
	// Name returns the provider identifier (e.g. "openlibrary", "googlebooks").
	Name() string

	// SearchAuthors searches for authors by name.
	SearchAuthors(ctx context.Context, query string) ([]models.Author, error)

	// SearchBooks searches for books by title, author, or ISBN.
	SearchBooks(ctx context.Context, query string) ([]models.Book, error)

	// GetAuthor fetches a single author by their provider-specific foreign ID.
	GetAuthor(ctx context.Context, foreignID string) (*models.Author, error)

	// GetBook fetches a single book/work by its provider-specific foreign ID.
	GetBook(ctx context.Context, foreignID string) (*models.Book, error)

	// GetEditions fetches all editions for a book/work by its foreign ID.
	GetEditions(ctx context.Context, bookForeignID string) ([]models.Edition, error)

	// GetBookByISBN looks up a book by ISBN-13 or ISBN-10.
	GetBookByISBN(ctx context.Context, isbn string) (*models.Book, error)
}

// SeriesSearchResult is a provider-neutral series discovery result.
type SeriesSearchResult struct {
	ForeignID    string
	ProviderID   string
	Title        string
	AuthorName   string
	BookCount    int
	ReadersCount int
	Books        []string
}

// SeriesCatalog is an ordered provider catalog for a single series.
type SeriesCatalog struct {
	ForeignID  string
	ProviderID string
	Title      string
	AuthorName string
	BookCount  int
	Books      []SeriesCatalogBook
}

// SeriesCatalogBook is a provider book entry with its position in a series.
type SeriesCatalogBook struct {
	ForeignID  string
	ProviderID string
	Title      string
	Subtitle   string
	Position   string
	UsersCount int
	Book       models.Book
}

// SeriesCatalogProvider is an optional metadata provider capability used by
// importers that can safely link provider series without widening Provider.
type SeriesCatalogProvider interface {
	SearchSeries(ctx context.Context, query string, limit int) ([]SeriesSearchResult, error)
	GetSeriesCatalog(ctx context.Context, foreignID string) (*SeriesCatalog, error)
}
