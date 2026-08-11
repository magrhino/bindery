package newznab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// emptyRSS is a well-formed feed with zero items, so every tier of the
// BookSearch cascade falls through to the next one. That is the shape of the
// #1814 report: nothing matched, so each book paid for the full cascade.
const emptyRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel><title>Empty</title><newznab:response offset="0" total="0"/></channel>
</rss>`

// queryRecorder is an httptest handler that records the query parameters of
// every request an indexer receives.
type queryRecorder struct {
	mu     sync.Mutex
	got    []url.Values
	delay  time.Duration
	server *httptest.Server
}

func newQueryRecorder(t *testing.T) *queryRecorder {
	t.Helper()
	rec := &queryRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.got = append(rec.got, r.URL.Query())
		delay := rec.delay
		rec.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(emptyRSS))
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (q *queryRecorder) requests() []url.Values {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]url.Values, len(q.got))
	copy(out, q.got)
	return out
}

func (q *queryRecorder) count() int { return len(q.requests()) }

// TestBookSearch_NeverIssuesEmptyTerm covers the highest-value part of #1814:
// 76 of the reporter's 294 indexer queries carried no search term at all. A
// title that normalises away leaves nothing to search for, and every tier of
// the cascade would otherwise degenerate into `q=` or a bare author name.
func TestBookSearch_NeverIssuesEmptyTerm(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		author string
	}{
		{"blank title", "", "Lemony Snicket"},
		{"whitespace title", "   ", "Lemony Snicket"},
		{"title is only an edition qualifier", "(German Edition)", "Lemony Snicket"},
		{"title is only a qualifier, no author", "(Unabridged)", ""},
		{"blank title and blank author", "", ""},
		{"title reduces to nothing after the colon split", ":", "Lemony Snicket"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newQueryRecorder(t)
			c := testNew(rec.server.URL, "key")

			results, err := c.BookSearch(context.Background(), tc.title, tc.author, []int{7020})
			if err != nil {
				t.Fatalf("BookSearch: %v", err)
			}
			if len(results) != 0 {
				t.Fatalf("expected no results, got %d", len(results))
			}
			if n := rec.count(); n != 0 {
				t.Fatalf("issued %d indexer queries for an unsearchable book, want 0: %v", n, rec.requests())
			}
		})
	}
}

// TestBookSearch_MissingAuthorStillSearchesTitleOnly is the control for the
// guard above: a book with no author is still searchable by title, and the one
// query it issues must carry a real term.
func TestBookSearch_MissingAuthorStillSearchesTitleOnly(t *testing.T) {
	rec := newQueryRecorder(t)
	c := testNew(rec.server.URL, "key")

	if _, err := c.BookSearch(context.Background(), "The Carnivorous Carnival", "", []int{7020}); err != nil {
		t.Fatalf("BookSearch: %v", err)
	}
	reqs := rec.requests()
	if len(reqs) != 1 {
		t.Fatalf("issued %d queries for an author-less book, want 1: %v", len(reqs), reqs)
	}
	if got := reqs[0].Get("q"); got != "The Carnivorous Carnival" {
		t.Fatalf("q = %q, want the book title", got)
	}
}

// TestSearch_EmptyQueryIssuesNoRequest is the floor under every caller: no code
// path may put `q=` on the wire with nothing after it.
func TestSearch_EmptyQueryIssuesNoRequest(t *testing.T) {
	rec := newQueryRecorder(t)
	c := testNew(rec.server.URL, "key")

	for _, q := range []string{"", " ", "\t\n"} {
		results, err := c.Search(context.Background(), q, []int{7020})
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if len(results) != 0 {
			t.Fatalf("Search(%q) returned %d results", q, len(results))
		}
	}
	if n := rec.count(); n != 0 {
		t.Fatalf("empty-term searches reached the indexer %d times: %v", n, rec.requests())
	}
}

// TestBookSearch_DedupesIdenticalQueries covers part 2 of #1814. Two catalogue
// rows for the same work — here an edition row whose parenthesised qualifier
// normalises away — produce a byte-identical cascade, and the reporter's log
// showed those repeats going out to the indexer twice.
func TestBookSearch_DedupesIdenticalQueries(t *testing.T) {
	rec := newQueryRecorder(t)
	c := testNew(rec.server.URL, "key")

	for _, title := range []string{
		"The Carnivorous Carnival",
		"The Carnivorous Carnival (Deluxe Edition)",
		"The Carnivorous Carnival: A Series of Unfortunate Events",
	} {
		if _, err := c.BookSearch(context.Background(), title, "Lemony Snicket", []int{7020}); err != nil {
			t.Fatalf("BookSearch(%q): %v", title, err)
		}
	}

	// One cascade: t=book, then surname+title, author+title, title-only.
	if n := rec.count(); n != 4 {
		t.Fatalf("three rows for one work issued %d queries, want 4: %v", n, rec.requests())
	}
}

// TestBookSearch_CollapsesConcurrentDuplicates proves the dedupe holds when the
// duplicates race rather than arrive in sequence — which is what a bounded,
// paced fan-out actually produces.
func TestBookSearch_CollapsesConcurrentDuplicates(t *testing.T) {
	rec := newQueryRecorder(t)
	rec.delay = 50 * time.Millisecond
	c := testNew(rec.server.URL, "key")

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.BookSearch(context.Background(), "Hurry Up and Wait", "Lemony Snicket", []int{7020})
		}()
	}
	wg.Wait()

	if n := rec.count(); n != 4 {
		t.Fatalf("six concurrent identical cascades issued %d queries, want 4: %v", n, rec.requests())
	}
}

// TestQueryCache_DoesNotCacheFailures guards the one thing the dedupe must not
// do: turn a transient indexer failure into a 90-second outage.
func TestQueryCache_DoesNotCacheFailures(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	c := testNew(srv.URL, "key")
	if _, err := c.Search(context.Background(), "Dune", []int{7020}); err == nil {
		t.Fatal("expected the first search to fail")
	}
	results, err := c.Search(context.Background(), "Dune", []int{7020})
	if err != nil {
		t.Fatalf("retry after a failure: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("retry after a failure returned no results — the failure was cached")
	}
}

// TestQueryCache_FailedEntryStillInMapIsNotServedAsSuccess covers the window
// between close(e.done) and the delete of a failed entry: those are two
// separate critical sections, so a caller can find the failed entry still in
// the map with done closed and e.at just set. Reading it as a hit would return
// (nil, nil) and swallow the error.
//
// That matters beyond the lost error: BookSearch aborts the whole cascade on a
// tier-1 error, so a swallowed one looks like "no results" and falls through
// into the remaining tiers — issuing more indexer queries, which is the
// opposite of what #1814 is for.
//
// The window is not reachable by racing two real requests, so the entry is
// planted directly in the state the write side leaves behind.
func TestQueryCache_FailedEntryStillInMapIsNotServedAsSuccess(t *testing.T) {
	q := newQueryCache()
	fetchErr := errors.New("indexer exploded")

	planted := &queryCacheEntry{done: make(chan struct{}), err: fetchErr, at: time.Now()}
	close(planted.done)
	q.entries["k"] = planted

	var refetched bool
	body, err := q.do(context.Background(), "k", func() ([]byte, error) {
		refetched = true
		return []byte("fresh"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !refetched {
		t.Fatal("a failed entry must not be served from the cache; fetch was never retried")
	}
	if string(body) != "fresh" {
		t.Fatalf("body = %q, want the refetched body — a nil body here is the swallowed error", body)
	}
}

// TestCaps_NotCached keeps the admin "Test indexer" button honest: a
// configuration fix must be visible on the next click, so caps responses stay
// off the search cache.
func TestCaps_NotCached(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testCaps))
	}))
	defer srv.Close()

	c := testNew(srv.URL, "key")
	for i := 0; i < 3; i++ {
		if _, err := c.Caps(context.Background()); err != nil {
			t.Fatalf("Caps: %v", err)
		}
	}
	if calls != 3 {
		t.Fatalf("caps requests = %d, want 3 (caps must not be cached)", calls)
	}
}
