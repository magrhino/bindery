package indexer

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/vavallee/bindery/internal/indexer/newznab"
	"github.com/vavallee/bindery/internal/models"
)

// Searcher coordinates searches across multiple Newznab indexers.
type Searcher struct{}

// NewSearcher creates a new multi-indexer searcher.
func NewSearcher() *Searcher {
	return &Searcher{}
}

// SearchBook queries all enabled indexers for a book and returns deduplicated, ranked results.
func (s *Searcher) SearchBook(ctx context.Context, indexers []models.Indexer, title, author string) []newznab.SearchResult {
	var (
		mu      sync.Mutex
		results []newznab.SearchResult
		wg      sync.WaitGroup
	)

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}
		wg.Add(1)
		go func(idx models.Indexer) {
			defer wg.Done()

			client := newznab.New(idx.URL, idx.APIKey)
			var hits []newznab.SearchResult
			var err error

			// Try book-specific search first, then generic
			hits, err = client.BookSearch(ctx, title, author, idx.Categories)
			if err != nil {
				slog.Warn("indexer search failed", "indexer", idx.Name, "error", err)
				return
			}

			// Tag results with indexer info and protocol
			protocol := protocolForType(idx.Type)
			for i := range hits {
				hits[i].IndexerID = idx.ID
				hits[i].IndexerName = idx.Name
				hits[i].Protocol = protocol
			}

			mu.Lock()
			results = append(results, hits...)
			mu.Unlock()

			slog.Debug("indexer returned results", "indexer", idx.Name, "count", len(hits))
		}(idx)
	}

	wg.Wait()

	results = dedupe(results)
	results = filterRelevant(results, title, author)
	rankResults(results)
	return results
}

// SearchQuery performs a generic text search across all enabled indexers.
func (s *Searcher) SearchQuery(ctx context.Context, indexers []models.Indexer, query string) []newznab.SearchResult {
	var (
		mu      sync.Mutex
		results []newznab.SearchResult
		wg      sync.WaitGroup
	)

	for _, idx := range indexers {
		if !idx.Enabled {
			continue
		}
		wg.Add(1)
		go func(idx models.Indexer) {
			defer wg.Done()

			client := newznab.New(idx.URL, idx.APIKey)
			hits, err := client.Search(ctx, query, idx.Categories)
			if err != nil {
				slog.Warn("indexer search failed", "indexer", idx.Name, "error", err)
				return
			}

			protocol := protocolForType(idx.Type)
			for i := range hits {
				hits[i].IndexerID = idx.ID
				hits[i].IndexerName = idx.Name
				hits[i].Protocol = protocol
			}

			mu.Lock()
			results = append(results, hits...)
			mu.Unlock()
		}(idx)
	}

	wg.Wait()

	results = dedupe(results)
	rankResults(results)
	return results
}

// stopWords are common English words excluded from keyword significance checks.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"of": true, "in": true, "to": true, "by": true, "for": true,
	"with": true, "at": true, "from": true, "is": true, "it": true,
	"as": true, "on": true, "be": true,
}

// sigWords returns the meaningful (non-stop, 3+ char) words from s.
func sigWords(s string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		if len(w) >= 3 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// normResultTitle replaces NZB filename separators (dots, underscores, dashes)
// with spaces so keyword matching works regardless of release naming style.
func normResultTitle(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '_' || r == '-' {
			return ' '
		}
		return r
	}, s)
}

// filterRelevant removes results that don't match the significant words of the title.
// All significant title words must appear in the result. For ambiguous titles with
// only one significant word (e.g. "The Sparrow" → ["sparrow"]), at least one author
// keyword must also match — otherwise every book with "sparrow" in the title passes.
// If the title has no significant words at all, author keywords are used instead.
func filterRelevant(results []newznab.SearchResult, title, author string) []newznab.SearchResult {
	titleKws := sigWords(title)
	authorKws := sigWords(author)

	required := titleKws
	if len(required) == 0 {
		required = authorKws
	}
	if len(required) == 0 {
		return results
	}

	// For short/ambiguous titles (one significant word), also require the author
	// to appear in the result title so unrelated books sharing a word are excluded.
	requireAuthor := len(titleKws) == 1 && len(authorKws) > 0

	var filtered []newznab.SearchResult
	for _, r := range results {
		lower := strings.ToLower(normResultTitle(r.Title))

		// All required title keywords must be present
		ok := true
		for _, kw := range required {
			if !strings.Contains(lower, kw) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}

		// Short-title guard: at least one author keyword must also appear
		if requireAuthor {
			authorMatch := false
			for _, kw := range authorKws {
				if strings.Contains(lower, kw) {
					authorMatch = true
					break
				}
			}
			if !authorMatch {
				continue
			}
		}

		filtered = append(filtered, r)
	}
	return filtered
}

func dedupe(results []newznab.SearchResult) []newznab.SearchResult {
	seen := make(map[string]bool)
	deduped := make([]newznab.SearchResult, 0, len(results))
	for _, r := range results {
		key := r.GUID
		if key == "" {
			key = r.Title + r.NZBURL
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}
	return deduped
}

func rankResults(results []newznab.SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		qi := models.QualityRank[detectQuality(results[i].Title)]
		qj := models.QualityRank[detectQuality(results[j].Title)]
		// Primary: quality rank descending
		if qi != qj {
			return qi > qj
		}
		// Secondary: more grabs (indicates healthier release)
		if results[i].Grabs != results[j].Grabs {
			return results[i].Grabs > results[j].Grabs
		}
		// Tertiary: size descending
		return results[i].Size > results[j].Size
	})
}

// detectQuality scans a result title for known quality keywords and returns
// the best (highest-ranked) match found.
func detectQuality(title string) string {
	lower := strings.ToLower(title)
	best := "unknown"
	bestRank := 0
	for q, rank := range models.QualityRank {
		if q == "unknown" {
			continue
		}
		if strings.Contains(lower, q) {
			if rank > bestRank {
				bestRank = rank
				best = q
			}
		}
	}
	return best
}

// protocolForType maps an indexer type string to its protocol name.
func protocolForType(t string) string {
	if t == "torznab" {
		return "torrent"
	}
	return "usenet"
}

// knownForeignTags contains uppercase markers commonly found in Usenet/torrent
// release titles that indicate a non-English release. Add more as needed.
var knownForeignTags = []string{
	// French
	"FRENCH", "FRANCAIS", ".VF.", ".VF ", "VF.", " VF ", "VOSTFR", ".VOSTFR.",
	// German
	"GERMAN", "DEUTSCH",
	// Spanish
	"SPANISH", "ESPANOL", "ESPAÑOL",
	// Dutch
	"DUTCH", "NETHERLANDS",
	// Italian
	"ITALIAN", "ITALIANO",
	// Portuguese
	"PORTUGUESE", "PORTUGUES",
	// Russian
	"RUSSIAN", "RUSSE",
	// Japanese
	"JAPANESE", "JAPONAIS",
	// Chinese
	"CHINESE", "MANDARIN",
	// Korean
	"KOREAN",
	// Arabic
	"ARABIC", "ARABE",
	// Swedish / Nordic
	"SWEDISH", "SVENSKA", "NORWEGIAN", "DANISH",
	// Polish
	"POLISH", "POLSKI",
	// Czech
	"CZECH",
	// Turkish
	"TURKISH",
	// Hindi
	"HINDI",
}

// FilterByLanguage removes results whose titles contain known foreign-language
// markers when lang is "en". When lang is "any" (or empty), all results pass.
func FilterByLanguage(results []newznab.SearchResult, lang string) []newznab.SearchResult {
	if lang == "" || lang == "any" {
		return results
	}
	// Only English filtering is implemented; other specific lang targets fall
	// through unfiltered (future work).
	if lang != "en" {
		return results
	}

	filtered := make([]newznab.SearchResult, 0, len(results))
	for _, r := range results {
		upper := strings.ToUpper(r.Title)
		foreign := false
		for _, tag := range knownForeignTags {
			if strings.Contains(upper, strings.ToUpper(tag)) {
				foreign = true
				break
			}
		}
		if !foreign {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
