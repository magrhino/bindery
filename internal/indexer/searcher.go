package indexer

import (
	"context"
	"log/slog"
	"math"
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

// MatchCriteria describes what we're searching for. Year and ISBN are
// optional and only used for ranking — they never cause a result to be
// rejected.
type MatchCriteria struct {
	Title  string
	Author string
	Year   int
	ISBN   string
}

// SearchBook queries all enabled indexers and returns deduplicated, filtered,
// ranked results.
func (s *Searcher) SearchBook(ctx context.Context, indexers []models.Indexer, c MatchCriteria) []newznab.SearchResult {
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
			hits, err := client.BookSearch(ctx, c.Title, c.Author, idx.Categories)
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

			slog.Debug("indexer returned results", "indexer", idx.Name, "count", len(hits))
		}(idx)
	}

	wg.Wait()

	results = dedupe(results)
	results = filterRelevant(results, c.Title, c.Author)
	rankResults(results, c)
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
	rankResults(results, MatchCriteria{Title: query})
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

// primaryTitle returns the portion of title before the first colon (used for
// subtitle handling — "Dune: Messiah" → "Dune"). If there's no colon the full
// title is returned.
func primaryTitle(title string) string {
	if i := strings.Index(title, ":"); i > 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

// titleMatchesResult returns true if the normalized result contains the
// significant words of the title either as a contiguous phrase or (for
// multi-word titles as a fallback) with every significant word appearing at
// a word boundary. A single-significant-word title additionally requires the
// author's surname to be present.
func titleMatchesResult(normResult string, titleKws []string, surname string, allowKwFallback bool) bool {
	switch len(titleKws) {
	case 0:
		return surname != "" && WordBoundaryRegex(surname).MatchString(normResult)
	case 1:
		if !WordBoundaryRegex(titleKws[0]).MatchString(normResult) {
			return false
		}
		if surname == "" {
			// No surname to anchor on — accept (can't do better).
			return true
		}
		return WordBoundaryRegex(surname).MatchString(normResult)
	default:
		if ContainsPhrase(normResult, titleKws) {
			return true
		}
		if !allowKwFallback {
			return false
		}
		for _, kw := range titleKws {
			if !WordBoundaryRegex(kw).MatchString(normResult) {
				return false
			}
		}
		return true
	}
}

// filterRelevant removes results that don't plausibly match the requested book.
// Strategy:
//   - Multi-significant-word titles: require a contiguous phrase match. If no
//     result in the batch matches as a phrase, relax to "every keyword at a
//     word boundary" so we still surface something for oddly-structured names.
//   - Single-significant-word titles: require the word AND the author surname
//     at word boundaries (prevents "sparrow" alone from matching noise).
//   - Titles with no significant words: fall back to the author surname alone.
//   - Subtitle handling: if the title has "primary: subtitle", results matching
//     either the primary-only or the full title form are accepted.
func filterRelevant(results []newznab.SearchResult, title, author string) []newznab.SearchResult {
	fullKws := sigWords(title)
	primaryKws := sigWords(primaryTitle(title))
	authorKws := sigWords(author)
	surname := AuthorSurname(author)

	if len(fullKws) == 0 && len(primaryKws) == 0 && len(authorKws) == 0 {
		return results
	}

	// Pre-normalize all result titles once.
	normTitles := make([]string, len(results))
	for i, r := range results {
		normTitles[i] = NormalizeRelease(r.Title)
	}

	// First pass: strict (no keyword-only fallback). This determines whether
	// any phrase match exists.
	anyPhraseMatch := false
	if len(fullKws) >= 2 {
		for _, n := range normTitles {
			if ContainsPhrase(n, fullKws) || (len(primaryKws) >= 2 && ContainsPhrase(n, primaryKws)) {
				anyPhraseMatch = true
				break
			}
		}
	}

	allowFallback := !anyPhraseMatch

	filtered := make([]newznab.SearchResult, 0, len(results))
	for i, r := range results {
		n := normTitles[i]

		fullOK := titleMatchesResult(n, fullKws, surname, allowFallback)
		primaryOK := false
		if !fullOK && len(primaryKws) > 0 && !sameKws(primaryKws, fullKws) {
			primaryOK = titleMatchesResult(n, primaryKws, surname, allowFallback)
		}
		if fullOK || primaryOK {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func sameKws(a, b []string) bool {
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

// rankResults sorts results in place by a composite score combining format
// quality, edition markers (RETAIL/UNABRIDGED/ABRIDGED), year match against
// the book's release year, grabs, size, and an ISBN exact-match boost.
func rankResults(results []newznab.SearchResult, c MatchCriteria) {
	scores := make([]float64, len(results))
	for i, r := range results {
		scores[i] = scoreResult(r, c)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return scores[i] > scores[j]
	})
}

// scoreResult computes the composite ranking score for a single result.
// Higher is better. Weights are hardcoded (no profile UI in v0.4.0).
func scoreResult(r newznab.SearchResult, c MatchCriteria) float64 {
	p := ParseRelease(r.Title)

	quality := "unknown"
	if p.Format != "" {
		quality = p.Format
	} else {
		quality = detectQuality(r.Title)
	}
	score := float64(models.QualityRank[quality]) * 100

	switch {
	case p.Retail:
		score += 50
	case p.Unabridged:
		score += 30
	case p.Abridged:
		score -= 50
	}

	if c.Year > 0 && p.Year > 0 {
		diff := c.Year - p.Year
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff == 0:
			score += 20
		case diff <= 2:
			score += 10
		case diff <= 5:
			score += 5
		default:
			score -= 5
		}
	}

	if r.Grabs > 0 {
		score += math.Log10(float64(r.Grabs+1)) * 10
	}

	if r.Size > 0 {
		mb := float64(r.Size) / (1024 * 1024)
		if mb > 1024 {
			mb = 1024
		}
		score += mb / 100
	}

	if c.ISBN != "" && p.ISBN != "" && strings.EqualFold(p.ISBN, c.ISBN) {
		score += 200
	}

	return score
}

// detectQuality scans a result title for known quality keywords and returns
// the best (highest-ranked) match found. Retained as a fallback for
// scoreResult when ParseRelease's word-boundary format detection misses
// (e.g. if a format token is jammed against other text without separators).
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

// knownForeignTags lists release-name markers indicating a non-English
// release. Matched at word boundaries against the normalized title — so
// "RUSSE" (French for "Russian") no longer falsely matches "RUSSELL".
var knownForeignTags = []string{
	"french", "francais",
	"vf", "vostfr", "vff",
	"german", "deutsch",
	"spanish", "espanol", "español",
	"dutch", "netherlands",
	"italian", "italiano",
	"portuguese", "portugues",
	"russian", "russe",
	"japanese", "japonais",
	"chinese", "mandarin",
	"korean",
	"arabic", "arabe",
	"swedish", "svenska", "norwegian", "danish",
	"polish", "polski",
	"czech",
	"turkish",
	"hindi",
}

// FilterByLanguage removes results whose titles contain known foreign-language
// markers when lang is "en". When lang is "any" (or empty), all results pass.
// Tag matching is word-boundary anchored to avoid false positives (e.g. the
// former "RUSSE" ⊂ "RUSSELL" bug).
func FilterByLanguage(results []newznab.SearchResult, lang string) []newznab.SearchResult {
	if lang == "" || lang == "any" {
		return results
	}
	if lang != "en" {
		return results
	}

	filtered := make([]newznab.SearchResult, 0, len(results))
	for _, r := range results {
		norm := NormalizeRelease(r.Title)
		foreign := false
		for _, tag := range knownForeignTags {
			if WordBoundaryRegex(tag).MatchString(norm) {
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
