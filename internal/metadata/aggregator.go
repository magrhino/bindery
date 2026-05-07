// Package metadata aggregates book and author data from multiple public
// sources (OpenLibrary, Google Books, Hardcover) behind a unified interface.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vavallee/bindery/internal/isbnutil"
	"github.com/vavallee/bindery/internal/metadata/audible"
	"github.com/vavallee/bindery/internal/metadata/audnex"
	"github.com/vavallee/bindery/internal/models"
)

// Aggregator fans out requests to multiple providers and merges results.
// OpenLibrary is always the primary source. Other providers enrich the data.
type Aggregator struct {
	primary   Provider
	enrichers []Provider
	audnex    *audnex.Client
	audible   *audible.Client
	cache     *ttlCache
}

// NewAggregator creates an aggregator with OpenLibrary as primary and optional enrichers.
func NewAggregator(primary Provider, enrichers ...Provider) *Aggregator {
	return &Aggregator{
		primary:   primary,
		enrichers: enrichers,
		audnex:    audnex.New(""),
		audible:   audible.New(),
		cache:     newTTLCache(24 * time.Hour),
	}
}

func (a *Aggregator) SearchAuthors(ctx context.Context, query string) ([]models.Author, error) {
	return a.primary.SearchAuthors(ctx, query)
}

func (a *Aggregator) SearchBooks(ctx context.Context, query string) ([]models.Book, error) {
	return a.primary.SearchBooks(ctx, query)
}

func (a *Aggregator) GetAuthor(ctx context.Context, foreignID string) (*models.Author, error) {
	key := "author:" + foreignID
	if cached, ok := a.cache.get(key); ok {
		return cached.(*models.Author), nil
	}

	provider := a.providerForForeignID(foreignID)
	if provider == nil {
		return nil, nil
	}
	author, err := provider.GetAuthor(ctx, foreignID)
	if err != nil {
		return nil, err
	}
	a.cache.set(key, author)
	return author, nil
}

func (a *Aggregator) GetBook(ctx context.Context, foreignID string) (*models.Book, error) {
	key := "book:" + foreignID
	if cached, ok := a.cache.get(key); ok {
		return cached.(*models.Book), nil
	}

	provider := a.providerForForeignID(foreignID)
	if provider == nil {
		return nil, nil
	}
	book, err := provider.GetBook(ctx, foreignID)
	if err != nil {
		return nil, err
	}
	if book == nil {
		a.cache.set(key, book)
		return nil, nil
	}

	// Enrich from secondary providers if description is sparse or cover is missing.
	if len(book.Description) < 50 || book.ImageURL == "" {
		a.enrichBook(ctx, book)
	}

	a.cache.set(key, book)
	return book, nil
}

func (a *Aggregator) GetEditions(ctx context.Context, bookForeignID string) ([]models.Edition, error) {
	key := "editions:" + bookForeignID
	if cached, ok := a.cache.get(key); ok {
		return cached.([]models.Edition), nil
	}

	provider := a.providerForForeignID(bookForeignID)
	if provider == nil {
		return nil, nil
	}
	editions, err := provider.GetEditions(ctx, bookForeignID)
	if err != nil {
		return nil, err
	}
	a.cache.set(key, editions)
	return editions, nil
}

func (a *Aggregator) GetBookByISBN(ctx context.Context, isbn string) (*models.Book, error) {
	isbn = isbnutil.Normalize(isbn)
	key := "isbn:" + isbn
	if cached, ok := a.cache.get(key); ok {
		return cached.(*models.Book), nil
	}

	var errs []error
	skippedUnconfigured := false
	providers := a.providers()
	var primaryFallback *models.Book
	var firstFallback *models.Book
	for idx, provider := range providers {
		if provider == nil {
			continue
		}
		book, err := provider.GetBookByISBN(ctx, isbn)
		if err != nil {
			if errors.Is(err, ErrProviderNotConfigured) {
				skippedUnconfigured = true
				slog.Debug("isbn provider not configured", "provider", provider.Name())
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", provider.Name(), err))
			slog.Debug("isbn lookup provider failed", "provider", provider.Name(), "error", err)
			continue
		}
		if book == nil {
			continue
		}
		if canonical, status := a.lookupCanonicalPrimaryBook(ctx, isbn, *book); status != canonicalPrimaryBookNoMatch {
			if status == canonicalPrimaryBookMatched {
				book = canonical
				return a.cacheISBNBook(ctx, key, book), nil
			}
			if idx > 0 || len(providers) == 1 {
				return a.cacheISBNBook(ctx, key, book), nil
			}
		}
		if firstFallback == nil {
			firstFallback = book
		}
		if idx == 0 && len(providers) > 1 {
			primaryFallback = book
			continue
		}
		if primaryFallback != nil {
			continue
		}
		return a.cacheISBNBook(ctx, key, book), nil
	}

	if primaryFallback != nil {
		return a.cacheISBNBook(ctx, key, primaryFallback), nil
	}
	if firstFallback != nil {
		return a.cacheISBNBook(ctx, key, firstFallback), nil
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	var noBook *models.Book
	if !skippedUnconfigured {
		a.cache.set(key, noBook)
	}
	return nil, nil
}

func (a *Aggregator) cacheISBNBook(ctx context.Context, key string, book *models.Book) *models.Book {
	if book != nil && len(book.Description) < 50 {
		a.enrichBook(ctx, book)
	}
	a.cache.set(key, book)
	return book
}
