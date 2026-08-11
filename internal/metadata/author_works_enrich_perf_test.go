package metadata

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

// concurrencyProbeEnricher is an enricher whose SearchBooks records how many
// calls were in flight at once, so a test can tell a serial loop from a
// bounded fan-out without asserting on wall clock.
type concurrencyProbeEnricher struct {
	mu       sync.Mutex
	calls    int
	inFlight int
	peak     int
	delay    time.Duration
}

func (e *concurrencyProbeEnricher) Name() string { return "hardcover" }
func (e *concurrencyProbeEnricher) SearchAuthors(context.Context, string) ([]models.Author, error) {
	return nil, nil
}
func (e *concurrencyProbeEnricher) SearchBooks(context.Context, string) ([]models.Book, error) {
	e.mu.Lock()
	e.calls++
	e.inFlight++
	if e.inFlight > e.peak {
		e.peak = e.inFlight
	}
	e.mu.Unlock()

	time.Sleep(e.delay)

	e.mu.Lock()
	e.inFlight--
	e.mu.Unlock()
	return nil, nil
}
func (e *concurrencyProbeEnricher) GetAuthor(context.Context, string) (*models.Author, error) {
	return nil, nil
}
func (e *concurrencyProbeEnricher) GetBook(context.Context, string) (*models.Book, error) {
	return nil, nil
}
func (e *concurrencyProbeEnricher) GetEditions(context.Context, string) ([]models.Edition, error) {
	return nil, nil
}
func (e *concurrencyProbeEnricher) GetBookByISBN(context.Context, string) (*models.Book, error) {
	return nil, nil
}

func (e *concurrencyProbeEnricher) stats() (calls, peak int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls, e.peak
}

// TestAuthorWorkCoverEnrichmentIsNotSerial pins the fan-out on the cover
// enrichment that runs over every cover-less author work before the sync loop
// starts. It used to walk the list one work at a time, so a 65-work author
// (Terry Bisson, #1888) spent 65 enricher round trips strictly in sequence
// before a single book row was written.
func TestAuthorWorkCoverEnrichmentIsNotSerial(t *testing.T) {
	const works = 65

	enricher := &concurrencyProbeEnricher{delay: 20 * time.Millisecond}
	// The primary contributes the works; none of them carries a cover, which
	// is the normal shape for OpenLibrary author works.
	books := make([]models.Book, works)
	for i := range books {
		books[i] = models.Book{
			ForeignID:        fmt.Sprintf("OL%dW", 1000+i),
			Title:            fmt.Sprintf("Work %d", i),
			MetadataProvider: "openlibrary",
		}
	}
	primary := &mockWorksProvider{mockProvider: mockProvider{name: "openlibrary", authorWorks: books}}
	agg := NewAggregator(primary, enricher)

	got, err := agg.GetAuthorWorks(context.Background(), "OL999A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != works {
		t.Fatalf("got %d works, want %d", len(got), works)
	}

	calls, peak := enricher.stats()
	if calls != works {
		t.Fatalf("enricher called %d times, want one per cover-less work (%d)", calls, works)
	}
	if peak < 2 {
		t.Fatalf("author-work cover enrichment ran serially (peak in-flight enricher calls %d); "+
			"a %d-work author pays every round trip in sequence before the sync loop starts",
			peak, works)
	}
}
