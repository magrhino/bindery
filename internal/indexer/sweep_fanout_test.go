package indexer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// emptyFeed is a well-formed feed with no items, so every tier of the per-book
// query cascade falls through to the next. That is the shape of #1814: nothing
// matched, so each book paid the full cascade against the one configured
// indexer.
const emptyFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel><title>Empty</title><newznab:response offset="0" total="0"/></channel>
</rss>`

// sweepIndexer is a fake indexer that records the search parameters of every
// request it receives, so a test can count and fingerprint the queries a bulk
// sweep actually puts on the wire.
type sweepIndexer struct {
	mu   sync.Mutex
	reqs []url.Values
	srv  *httptest.Server
}

func newSweepIndexer(t *testing.T) *sweepIndexer {
	t.Helper()
	s := &sweepIndexer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.reqs = append(s.reqs, r.URL.Query())
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(emptyFeed))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sweepIndexer) snapshot() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]url.Values, len(s.reqs))
	copy(out, s.reqs)
	return out
}

// fingerprints renders each recorded request as the (term, categories) pair the
// indexer sees. A t=book request has no term of its own, so its title/author
// pair stands in for one.
func (s *sweepIndexer) fingerprints() []string {
	reqs := s.snapshot()
	out := make([]string, 0, len(reqs))
	for _, q := range reqs {
		term := q.Get("q")
		if q.Get("t") == "book" {
			term = "book:" + q.Get("author") + "|" + q.Get("title")
		}
		out = append(out, fmt.Sprintf("%s @ %s", term, q.Get("cat")))
	}
	return out
}

func (s *sweepIndexer) config() models.Indexer {
	return models.Indexer{
		ID: 1, Name: "Fake", URL: s.srv.URL, APIKey: "key",
		Enabled: true, Type: "newznab", Categories: []int{7020, 3030},
	}
}

// sweepBook mirrors the fields searchAndGrabFormat feeds into MatchCriteria.
type sweepBook struct {
	title  string
	author string
}

// runSweep drives the same fan-out shape the bulk "search all wanted" action
// produces: for every book, one SearchBook per format that still needs a file.
func runSweep(t *testing.T, s *Searcher, idx models.Indexer, books []sweepBook) {
	t.Helper()
	for _, b := range books {
		for _, mt := range []string{models.MediaTypeEbook, models.MediaTypeAudiobook} {
			s.SearchBook(context.Background(), []models.Indexer{idx}, MatchCriteria{
				Title: b.title, Author: b.author, MediaType: mt,
			})
		}
	}
}

// snicketSweep is a scaled-down version of the reporter's catalogue in #1814:
// distinct wanted books, plus the two shapes an Open Library catalogue reliably
// contributes — an edition row that normalises onto a title already in the list,
// and a row whose title carries nothing searchable.
var snicketSweep = []sweepBook{
	{"The Carnivorous Carnival", "Lemony Snicket"},
	{"Hurry Up and Wait", "Lemony Snicket"},
	{"The Basic Eight", "Lemony Snicket"},
	{"The Carnivorous Carnival (Deluxe Edition)", "Lemony Snicket"}, // same work, same query
	{"(Unabridged)", "Lemony Snicket"},                              // nothing to search for
}

// TestSweep_QueryCountIsBounded is the headline regression guard for #1814. A
// 26-book author sent ~294 queries to one indexer; the multiplication is
// books × formats × cascade tiers, with duplicate and empty-term queries on top.
// This pins the number a sweep may issue so any future regression that
// reintroduces the multiplication fails loudly rather than quietly costing a
// user their indexer's API budget.
func TestSweep_QueryCountIsBounded(t *testing.T) {
	fake := newSweepIndexer(t)
	s := newTestSearcher()

	runSweep(t, s, fake.config(), snicketSweep)

	// 3 distinct searchable works × 2 formats × 4 cascade tiers. The edition
	// duplicate rides the cache, the unsearchable row issues nothing.
	const want = 3 * 2 * 4
	if got := len(fake.snapshot()); got != want {
		t.Fatalf("sweep issued %d indexer queries, want %d:\n  %s",
			got, want, strings.Join(fake.fingerprints(), "\n  "))
	}
}

// TestSweep_IssuesNoDuplicateQueries covers part 2: within one sweep the same
// (term, categories) pair must never go to the same indexer twice.
func TestSweep_IssuesNoDuplicateQueries(t *testing.T) {
	fake := newSweepIndexer(t)
	s := newTestSearcher()

	runSweep(t, s, fake.config(), snicketSweep)

	seen := map[string]int{}
	for _, fp := range fake.fingerprints() {
		seen[fp]++
	}
	var dupes []string
	for fp, n := range seen {
		if n > 1 {
			dupes = append(dupes, fmt.Sprintf("%s ×%d", fp, n))
		}
	}
	sort.Strings(dupes)
	if len(dupes) > 0 {
		t.Fatalf("sweep repeated %d (term, categories) pairs:\n  %s",
			len(dupes), strings.Join(dupes, "\n  "))
	}
}

// TestSweep_IssuesNoEmptyTerms covers part 1: no query may reach the indexer
// without something to search for. 76 of the reporter's 294 queries had none.
func TestSweep_IssuesNoEmptyTerms(t *testing.T) {
	fake := newSweepIndexer(t)
	s := newTestSearcher()

	runSweep(t, s, fake.config(), append(snicketSweep,
		sweepBook{"", "Lemony Snicket"},    // book row with no title at all
		sweepBook{"Hurry Up and Wait", ""}, // book row with no author
		sweepBook{"(German Edition)", ""},  // neither
	))

	for _, q := range fake.snapshot() {
		if q.Get("t") == "book" {
			if strings.TrimSpace(q.Get("title")) == "" {
				t.Errorf("structured book query sent with an empty title: %v", q)
			}
			continue
		}
		if strings.TrimSpace(q.Get("q")) == "" {
			t.Errorf("text search sent with an empty term: %v", q)
		}
	}
}

// TestSweep_ConcurrentBooksShareQueries drives the sweep the way the bulk
// fan-out does — several books in flight at once — and asserts the dedupe holds
// under that race, not just when duplicates arrive in sequence.
func TestSweep_ConcurrentBooksShareQueries(t *testing.T) {
	fake := newSweepIndexer(t)
	s := newTestSearcher()
	idx := fake.config()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runSweep(t, s, idx, snicketSweep)
		}()
	}
	wg.Wait()

	const want = 3 * 2 * 4
	if got := len(fake.snapshot()); got != want {
		t.Fatalf("6 overlapping sweeps issued %d queries, want %d", got, want)
	}
}

// TestSweep_UsesOneClientPerIndexer pins the property the query dedupe depends
// on: the searcher pools one newznab client per indexer, so the cache that
// collapses repeats is shared across every book in the sweep rather than
// rebuilt per call.
func TestSweep_UsesOneClientPerIndexer(t *testing.T) {
	fake := newSweepIndexer(t)
	s := newTestSearcher()

	runSweep(t, s, fake.config(), snicketSweep)

	if got := s.cache.ConstructorCount(); got != 1 {
		t.Fatalf("built %d clients for one indexer, want 1", got)
	}
}
