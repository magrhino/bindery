package metadata

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

// Aggregator fans out requests to multiple providers and merges results.
// OpenLibrary is always the primary source. Other providers enrich the data.
type Aggregator struct {
	primary   Provider
	enrichers []Provider
	cache     *ttlCache
}

// NewAggregator creates an aggregator with OpenLibrary as primary and optional enrichers.
func NewAggregator(primary Provider, enrichers ...Provider) *Aggregator {
	return &Aggregator{
		primary:   primary,
		enrichers: enrichers,
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

	author, err := a.primary.GetAuthor(ctx, foreignID)
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

	book, err := a.primary.GetBook(ctx, foreignID)
	if err != nil {
		return nil, err
	}

	// Enrich from secondary providers if description is sparse
	if len(book.Description) < 50 {
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

	editions, err := a.primary.GetEditions(ctx, bookForeignID)
	if err != nil {
		return nil, err
	}
	a.cache.set(key, editions)
	return editions, nil
}

func (a *Aggregator) GetBookByISBN(ctx context.Context, isbn string) (*models.Book, error) {
	key := "isbn:" + isbn
	if cached, ok := a.cache.get(key); ok {
		return cached.(*models.Book), nil
	}

	book, err := a.primary.GetBookByISBN(ctx, isbn)
	if err != nil {
		return nil, err
	}

	if book != nil && len(book.Description) < 50 {
		a.enrichBook(ctx, book)
	}

	a.cache.set(key, book)
	return book, nil
}

// enrichBook tries to fill in missing data from secondary providers.
func (a *Aggregator) enrichBook(ctx context.Context, book *models.Book) {
	for _, enricher := range a.enrichers {
		// Try ISBN search for better match
		enriched, err := enricher.SearchBooks(ctx, book.Title)
		if err != nil {
			slog.Debug("enrichment failed", "provider", enricher.Name(), "error", err)
			continue
		}
		for _, e := range enriched {
			if len(e.Description) > len(book.Description) {
				book.Description = e.Description
				slog.Debug("enriched description", "provider", enricher.Name(), "book", book.Title)
			}
			if book.AverageRating == 0 && e.AverageRating > 0 {
				book.AverageRating = e.AverageRating
				book.RatingsCount = e.RatingsCount
			}
			break // Use first match
		}
	}
}

// ttlCache is a simple in-process cache with TTL expiry.
type ttlCache struct {
	mu      sync.RWMutex
	items   map[string]cacheItem
	ttl     time.Duration
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

func newTTLCache(ttl time.Duration) *ttlCache {
	c := &ttlCache{
		items: make(map[string]cacheItem),
		ttl:   ttl,
	}
	// Background cleanup every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanup()
		}
	}()
	return c
}

func (c *ttlCache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *ttlCache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *ttlCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, v := range c.items {
		if now.After(v.expiresAt) {
			delete(c.items, k)
		}
	}
}
