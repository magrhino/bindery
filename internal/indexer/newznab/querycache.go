package newznab

import (
	"context"
	"sync"
	"time"
)

// A bulk "search all wanted" sweep issues the same indexer query many times
// over. Each book runs a multi-tier query cascade, a dual-format book runs that
// cascade once per format, two catalogue rows for the same work (edition
// duplicates, or titles that differ only by a parenthesised qualifier) normalise
// to a byte-identical query, and an overlapping sweep — a catalogue refresh's
// auto-search racing a manual "Search N wanted" click — repeats the whole set.
// In #1814 one 26-book author produced ~294 queries against a single indexer and
// every search past the first minute failed with "context deadline exceeded".
//
// queryCache collapses those repeats. It is keyed on the fully-built request URL
// with the api key redacted, so only an exactly identical query — same t=, same
// term, same category set — is ever served from it, and because the searcher
// pools one *Client per (indexer URL, api key) the cache is already scoped per
// indexer. Concurrent duplicates wait on the first request instead of issuing
// their own; a completed response is reused for queryCacheTTL.
//
// Only search responses go through the cache (see getXMLCached). Caps/Test/Probe
// deliberately do not: an admin who has just fixed an indexer's configuration
// must see the result of the retry, not a cached failure.
const (
	// queryCacheTTL is how long a completed search response is reused. It has
	// to outlive a single fan-out sweep to be useful — the bulk search paces
	// launches 3 s apart, so 26 books run well over a minute — while staying
	// short enough that a user re-running a search a few minutes later gets
	// fresh results from the indexer. An interactive search repeated inside
	// the window is served from the cache too, which is the intended trade:
	// an indexer's answer to the same question does not change in 90 seconds,
	// and re-asking it is exactly the behaviour that flooded #1814.
	queryCacheTTL = 90 * time.Second

	// queryCacheMaxBytes caps the total size of the cached response bodies. A
	// newznab feed at limit=100 runs to tens of kilobytes and a large sweep
	// produces hundreds of distinct queries, so the bound is on bytes rather
	// than entry count. Once the budget is exhausted expired entries are
	// dropped and, if that is not enough, every completed entry is — a cold
	// cache costs one extra query, never correctness.
	queryCacheMaxBytes = 4 << 20
)

// queryCacheEntry is one in-flight or completed indexer query. done is closed
// once body/err/at are written, so late arrivals can either wait on the
// original request or read its result without holding the cache lock.
type queryCacheEntry struct {
	done chan struct{}
	body []byte
	err  error
	at   time.Time
}

// queryCache deduplicates indexer queries for a single client. The zero value
// is not usable; a nil *queryCache is, and passes every call straight through
// (clients built by hand in tests keep their uncached behaviour).
type queryCache struct {
	mu      sync.Mutex
	entries map[string]*queryCacheEntry
	bytes   int
}

func newQueryCache() *queryCache {
	return &queryCache{entries: make(map[string]*queryCacheEntry)}
}

// do returns the response body for key, invoking fetch at most once for any set
// of callers that either overlap in time or arrive within queryCacheTTL of a
// successful fetch.
//
// Failures are never cached: a transient timeout must not suppress the next
// attempt. Callers that arrive while a request is in flight do share its error,
// which is the point — piling four more requests onto an indexer that is
// already timing out is what #1814 was about. Those callers also inherit the
// first caller's context, so a fetch cancelled by the originator surfaces as a
// cancellation for everyone waiting on it.
func (q *queryCache) do(ctx context.Context, key string, fetch func() ([]byte, error)) ([]byte, error) {
	if q == nil {
		return fetch()
	}

	q.mu.Lock()
	if e, ok := q.entries[key]; ok {
		select {
		case <-e.done:
			// e.err must be checked here, not just at the write side below.
			// close(e.done) and the delete of a failed entry are two separate
			// critical sections, so a caller arriving in between finds the
			// failed entry still in the map with e.at just set. Without this
			// guard it would return (nil, nil) and swallow the error — and
			// BookSearch depends on a tier-1 error aborting the cascade
			// rather than falling through into the remaining tiers.
			if e.err == nil && time.Since(e.at) < queryCacheTTL {
				body := e.body
				q.mu.Unlock()
				return body, nil
			}
			q.dropLocked(key, e)
		default:
			// In flight: wait for the original request rather than issuing a
			// second one.
			q.mu.Unlock()
			select {
			case <-e.done:
				return e.body, e.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	e := &queryCacheEntry{done: make(chan struct{})}
	q.entries[key] = e
	q.mu.Unlock()

	e.body, e.err = fetch()
	e.at = time.Now()
	close(e.done)

	q.mu.Lock()
	if cur, ok := q.entries[key]; ok && cur == e {
		if e.err != nil || len(e.body) > queryCacheMaxBytes {
			delete(q.entries, key)
		} else {
			q.bytes += len(e.body)
			q.evictLocked()
		}
	}
	q.mu.Unlock()

	return e.body, e.err
}

// dropLocked removes entry e under key, keeping the byte accounting in step.
// The identity check makes a double drop (two callers racing on the same
// expired entry) a no-op.
func (q *queryCache) dropLocked(key string, e *queryCacheEntry) {
	cur, ok := q.entries[key]
	if !ok || cur != e {
		return
	}
	delete(q.entries, key)
	q.bytes -= len(e.body)
}

// evictLocked brings the cache back under queryCacheMaxBytes: expired entries
// first, then — if the live set alone still exceeds the budget — every
// completed entry. In-flight entries are always kept so their waiters are
// still served.
func (q *queryCache) evictLocked() {
	if q.bytes <= queryCacheMaxBytes {
		return
	}
	now := time.Now()
	for k, e := range q.entries {
		select {
		case <-e.done:
			if now.Sub(e.at) >= queryCacheTTL {
				delete(q.entries, k)
				q.bytes -= len(e.body)
			}
		default:
		}
	}
	if q.bytes <= queryCacheMaxBytes {
		return
	}
	for k, e := range q.entries {
		select {
		case <-e.done:
			delete(q.entries, k)
			q.bytes -= len(e.body)
		default:
		}
	}
}
