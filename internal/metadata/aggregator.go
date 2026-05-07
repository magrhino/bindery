// Package metadata aggregates book and author data from multiple public
// sources (OpenLibrary, Google Books, Hardcover) behind a unified interface.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/isbnutil"
	"github.com/vavallee/bindery/internal/metadata/audible"
	"github.com/vavallee/bindery/internal/metadata/audnex"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/seriesmatch"
	"github.com/vavallee/bindery/internal/textutil"
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

// EnrichAudiobook fills narrator, duration, and cover from audnex when a
// book has audiobook audio (MediaType=audiobook or both) and a known ASIN.
// No-op otherwise.
func (a *Aggregator) EnrichAudiobook(ctx context.Context, book *models.Book) error {
	if book == nil || book.ASIN == "" {
		return nil
	}
	if book.MediaType != models.MediaTypeAudiobook && book.MediaType != models.MediaTypeBoth {
		return nil
	}
	b, err := a.audnex.GetBook(ctx, book.ASIN)
	if err != nil || b == nil {
		return err
	}
	if narr := b.NarratorList(); narr != "" {
		book.Narrator = narr
	}
	if dur := b.DurationSeconds(); dur > 0 {
		book.DurationSeconds = dur
	}
	if book.ImageURL == "" && b.Image != "" {
		book.ImageURL = b.Image
	}
	if book.Description == "" && b.Summary != "" {
		book.Description = b.Summary
	}
	return nil
}

// GetAuthorAudiobooks queries the Audible catalogue directly for the given
// author name. Returned books carry MediaType=audiobook, an ASIN, and a
// normalized language code; the caller applies the active metadata
// profile's allowed_languages filter alongside OpenLibrary-sourced books.
//
// Callers use this as a supplement to GetAuthorWorks — neither OpenLibrary
// nor Hardcover has full Audible ASIN cross-referencing, so prolific
// authors (Sanderson, King, etc.) are missing a large share of their
// narrated catalogue without a direct Audible source.
//
// Returns an empty slice when the audible client is unconfigured (test
// aggregators) rather than nil-derefing. Errors propagate so the caller
// can log them without failing the broader ingestion.
func (a *Aggregator) GetAuthorAudiobooks(ctx context.Context, authorName string) ([]models.Book, error) {
	if a.audible == nil {
		return nil, nil
	}
	authorName = strings.TrimSpace(authorName)
	if authorName == "" {
		return nil, nil
	}
	key := "audible-author:" + strings.ToLower(authorName)
	if cached, ok := a.cache.get(key); ok {
		return cached.([]models.Book), nil
	}
	books, err := a.audible.SearchBooksByAuthor(ctx, authorName)
	if err != nil {
		return nil, err
	}
	if books == nil {
		books = []models.Book{}
	}
	a.cache.set(key, books)
	return books, nil
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

type worksProvider interface {
	GetAuthorWorks(ctx context.Context, authorForeignID string) ([]models.Book, error)
}

type authorWorksByNameProvider interface {
	Name() string
	GetAuthorWorksByName(ctx context.Context, authorName string) ([]models.Book, error)
}

// GetAuthorWorks fetches all works by an author using the dedicated primary
// provider endpoint. It retains the legacy foreign-ID-only behavior for tests
// and existing callers; author ingestion should use GetAuthorWorksForAuthor so
// enrichers can run author-scoped supplemental queries.
func (a *Aggregator) GetAuthorWorks(ctx context.Context, authorForeignID string) ([]models.Book, error) {
	key := "authorworks:" + authorForeignID
	if cached, ok := a.cache.get(key); ok {
		return cached.([]models.Book), nil
	}

	books, err := a.rawPrimaryAuthorWorks(ctx, authorForeignID)
	if err != nil {
		return nil, err
	}
	a.enrichMissingAuthorWorkCovers(ctx, books)
	a.cache.set(key, books)
	return books, nil
}

// GetAuthorWorksForAuthor fetches the primary provider's author works and
// merges any author-scoped supplemental catalogs from enrichers before falling
// back to per-title cover enrichment for remaining gaps.
func (a *Aggregator) GetAuthorWorksForAuthor(ctx context.Context, author models.Author) ([]models.Book, error) {
	key := "authorworks-author:" + author.ForeignID + ":" + strings.ToLower(strings.TrimSpace(author.Name))
	if cached, ok := a.cache.get(key); ok {
		return cached.([]models.Book), nil
	}

	books, err := a.rawPrimaryAuthorWorks(ctx, author.ForeignID)
	if err != nil {
		return nil, err
	}

	authorName := strings.TrimSpace(author.Name)
	supplementsComplete := true
	if authorName != "" {
		for _, provider := range a.authorWorksByNameProviders() {
			supplemental, err := provider.GetAuthorWorksByName(ctx, authorName)
			if err != nil {
				supplementsComplete = false
				if errors.Is(err, ErrProviderNotConfigured) {
					continue
				}
				slog.Warn("author works supplement failed", "provider", provider.Name(), "author", authorName, "error", err)
				continue
			}
			if len(supplemental) == 0 {
				continue
			}
			books = mergeAuthorWorks(books, supplemental)
		}
	}

	a.enrichMissingAuthorWorkCovers(ctx, books)
	if supplementsComplete {
		a.cache.set(key, books)
	}
	return books, nil
}

func (a *Aggregator) rawPrimaryAuthorWorks(ctx context.Context, authorForeignID string) ([]models.Book, error) {
	key := "authorworks-raw:" + authorForeignID
	if cached, ok := a.cache.get(key); ok {
		return cloneBooks(cached.([]models.Book)), nil
	}

	books, err := a.primaryAuthorWorks(ctx, authorForeignID)
	if err != nil {
		return nil, err
	}
	a.cache.set(key, cloneBooks(books))
	return cloneBooks(books), nil
}

func (a *Aggregator) primaryAuthorWorks(ctx context.Context, authorForeignID string) ([]models.Book, error) {
	provider := a.providerForForeignID(authorForeignID)
	if provider == nil {
		return nil, nil
	}
	if wp, ok := provider.(worksProvider); ok {
		return wp.GetAuthorWorks(ctx, authorForeignID)
	}
	if !sameProvider(provider, a.primary) {
		return nil, nil
	}
	return a.primary.SearchBooks(ctx, authorForeignID)
}

func (a *Aggregator) authorWorksByNameProviders() []authorWorksByNameProvider {
	if a == nil {
		return nil
	}
	providers := make([]authorWorksByNameProvider, 0, len(a.enrichers))
	for _, enricher := range a.enrichers {
		if provider, ok := enricher.(authorWorksByNameProvider); ok {
			providers = append(providers, provider)
		}
	}
	return providers
}

func cloneBooks(books []models.Book) []models.Book {
	if books == nil {
		return nil
	}
	cloned := make([]models.Book, len(books))
	copy(cloned, books)
	return cloned
}

func (a *Aggregator) enrichMissingAuthorWorkCovers(ctx context.Context, books []models.Book) {
	for i := range books {
		if books[i].ImageURL == "" {
			a.enrichBook(ctx, &books[i])
		}
	}
}

func mergeAuthorWorks(primary, supplemental []models.Book) []models.Book {
	books := make([]models.Book, 0, len(primary)+len(supplemental))
	index := make(map[string]int, len(primary)+len(supplemental))
	for _, book := range primary {
		key := authorWorkMergeKey(book.Title)
		if key != "" {
			if _, exists := index[key]; !exists {
				index[key] = len(books)
			}
		}
		books = append(books, book)
	}
	for _, book := range supplemental {
		key := authorWorkMergeKey(book.Title)
		if key == "" {
			continue
		}
		if pos, ok := index[key]; ok {
			mergeAuthorWorkMetadata(&books[pos], book)
			continue
		}
		index[key] = len(books)
		books = append(books, book)
	}
	return books
}

func authorWorkMergeKey(title string) string {
	key := indexer.NormalizeTitleForDedup(title)
	if key != "" {
		return key
	}
	return strings.ToLower(strings.TrimSpace(title))
}

func mergeAuthorWorkMetadata(dst *models.Book, src models.Book) {
	if dst.ImageURL == "" {
		dst.ImageURL = src.ImageURL
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if dst.AverageRating == 0 {
		dst.AverageRating = src.AverageRating
	}
	if dst.RatingsCount == 0 {
		dst.RatingsCount = src.RatingsCount
	}
	if dst.ReleaseDate == nil {
		dst.ReleaseDate = src.ReleaseDate
	}
	if len(dst.Genres) == 0 {
		dst.Genres = src.Genres
	}
	if dst.DurationSeconds == 0 {
		dst.DurationSeconds = src.DurationSeconds
	}
	if dst.ASIN == "" {
		dst.ASIN = src.ASIN
	}
	if dst.MediaType == "" {
		dst.MediaType = src.MediaType
	}
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

type bookMatchCandidate struct {
	book         models.Book
	titleExact   bool
	workExact    bool
	titleScore   int
	authorKind   textutil.AuthorMatchKind
	authorScore  float64
	authorAlias  bool
	resultRank   int
	rankTie      bool
	editionExact bool
	variantKind  canonicalTitleVariantKind
	isISBN       bool
}

type canonicalTitleVariantKind int

const (
	canonicalTitleVariantSource canonicalTitleVariantKind = iota
	canonicalTitleVariantDescriptor
	canonicalTitleVariantRightSegment
	canonicalTitleVariantLeftSegment
)

type primaryBookCanonicalQuery struct {
	query                  string
	matchTitle             string
	exactTitleOnly         bool
	allowRankTieBreak      bool
	allowEditionTitleMatch bool
	hasAuthor              bool
	variantKind            canonicalTitleVariantKind
	isISBN                 bool
}

type canonicalTitleVariant struct {
	title             string
	exactTitleOnly    bool
	allowRankTieBreak bool
	kind              canonicalTitleVariantKind
}

type canonicalPrimaryBookMatch struct {
	candidate  bookMatchCandidate
	sameSource bool
}

type canonicalPrimaryBookStatus int

const (
	canonicalPrimaryBookNoMatch canonicalPrimaryBookStatus = iota
	canonicalPrimaryBookMatched
	canonicalPrimaryBookDirectISBNConfirmed
)

func (a *Aggregator) canonicalPrimaryBook(ctx context.Context, isbn string, source models.Book) (*models.Book, bool) {
	canonical, status := a.lookupCanonicalPrimaryBook(ctx, isbn, source)
	return canonical, status == canonicalPrimaryBookMatched
}

func (a *Aggregator) lookupCanonicalPrimaryBook(ctx context.Context, isbn string, source models.Book) (*models.Book, canonicalPrimaryBookStatus) {
	if a == nil || a.primary == nil || source.Title == "" {
		return nil, canonicalPrimaryBookNoMatch
	}
	sourceAuthor := bookAuthorName(source)
	if sourceAuthor == "" {
		return nil, canonicalPrimaryBookNoMatch
	}
	queries := primaryBookCanonicalQueries(isbn, source.Title, sourceAuthor, source.Language)
	if canonical, status := a.canonicalPrimaryBookFromQueries(ctx, queries, func(query primaryBookCanonicalQuery) (*canonicalPrimaryBookMatch, bool) {
		return a.canonicalPrimaryBookSearch(ctx, query, source, sourceAuthor)
	}); status == canonicalPrimaryBookMatched {
		return canonical, status
	} else if status == canonicalPrimaryBookDirectISBNConfirmed {
		if canonical, ok := a.canonicalPrimaryBookAuthorWorks(ctx, source, sourceAuthor); ok {
			return canonical, canonicalPrimaryBookMatched
		}
		return nil, status
	}
	if canonical, ok := a.canonicalPrimaryBookAuthorWorks(ctx, source, sourceAuthor); ok {
		return canonical, canonicalPrimaryBookMatched
	}
	return nil, canonicalPrimaryBookNoMatch
}

func primaryBookCanonicalQueries(isbn, title, author, language string) []primaryBookCanonicalQuery {
	queries := make([]primaryBookCanonicalQuery, 0, 4)
	allowEditionTitleMatch := true
	seen := make(map[string]struct{})
	addQuery := func(queryText, matchTitle string, exactTitleOnly, allowRankTieBreak, hasAuthor bool, kind canonicalTitleVariantKind, isISBN bool) {
		queryText = strings.TrimSpace(queryText)
		if queryText == "" {
			return
		}
		if _, ok := seen[queryText]; ok {
			return
		}
		seen[queryText] = struct{}{}
		queries = append(queries, primaryBookCanonicalQuery{
			query:                  queryText,
			matchTitle:             matchTitle,
			exactTitleOnly:         exactTitleOnly,
			allowRankTieBreak:      allowRankTieBreak,
			allowEditionTitleMatch: allowEditionTitleMatch,
			hasAuthor:              hasAuthor,
			variantKind:            kind,
			isISBN:                 isISBN,
		})
	}
	if isbn = isbnutil.Normalize(isbn); isbn != "" {
		addQuery("isbn:"+isbn, title, false, false, true, canonicalTitleVariantSource, true)
	}
	for _, variant := range canonicalTitleVariants(title, language) {
		for _, authorVariant := range canonicalAuthorQueryVariants(author) {
			addQuery(variant.title+" "+authorVariant, variant.title, variant.exactTitleOnly, variant.allowRankTieBreak, true, variant.kind, false)
			addQuery("title:"+variant.title+" author:"+authorVariant, variant.title, variant.exactTitleOnly, variant.allowRankTieBreak, true, variant.kind, false)
		}
		if canonicalAuthorNeedsTitleOnlyFallback(author) {
			addQuery(variant.title, variant.title, variant.exactTitleOnly, variant.allowRankTieBreak, false, variant.kind, false)
			addQuery("title:"+variant.title, variant.title, variant.exactTitleOnly, variant.allowRankTieBreak, false, variant.kind, false)
		}
	}
	return queries
}

func canonicalAuthorNeedsTitleOnlyFallback(author string) bool {
	for _, part := range strings.Fields(author) {
		if likelyEastAsianRomanizedNameToken(part) {
			return true
		}
	}
	for _, r := range author {
		if r > 127 {
			return true
		}
	}
	return false
}

func canonicalAuthorQueryVariants(author string) []string {
	author = strings.TrimSpace(author)
	if author == "" {
		return nil
	}
	variants := make([]string, 0, 2)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range variants {
			if strings.EqualFold(existing, candidate) {
				return
			}
		}
		variants = append(variants, candidate)
	}
	if before, after, ok := strings.Cut(author, ","); ok {
		add(strings.TrimSpace(after) + " " + strings.TrimSpace(before))
	}
	add(author)
	if concise, ok := canonicalConciseAuthorQueryVariant(author); ok {
		add(concise)
	}
	if swapped, ok := canonicalEastAsianAuthorOrderVariant(author); ok {
		add(swapped)
	}
	return variants
}

func canonicalConciseAuthorQueryVariant(author string) (string, bool) {
	if strings.Contains(author, ",") {
		return "", false
	}
	parts := strings.Fields(author)
	if len(parts) < 3 {
		return "", false
	}
	return parts[0] + " " + parts[len(parts)-1], true
}

func canonicalEastAsianAuthorOrderVariant(author string) (string, bool) {
	if strings.Contains(author, ",") {
		return "", false
	}
	parts := strings.Fields(author)
	if len(parts) != 2 {
		return "", false
	}
	if !likelyEastAsianRomanizedNameToken(parts[0]) && !likelyEastAsianRomanizedNameToken(parts[1]) {
		return "", false
	}
	return parts[1] + " " + parts[0], true
}

func likelyEastAsianRomanizedNameToken(token string) bool {
	switch strings.ToLower(strings.Trim(token, " .")) {
	case "cixin", "hua", "liu", "yu":
		return true
	default:
		return false
	}
}

func canonicalTitleVariants(title, language string) []canonicalTitleVariant {
	title = cleanCanonicalTitleVariant(title)
	if title == "" {
		return nil
	}
	isGerman := isGermanBookLanguage(language)
	sourceKey := indexer.NormalizeTitleForDedup(title)
	variants := make([]canonicalTitleVariant, 0, 8)
	seen := make(map[string]struct{})
	add := func(candidate string, requireUseful bool, kind canonicalTitleVariantKind) bool {
		candidate = cleanCanonicalTitleVariant(candidate)
		if candidate == "" {
			return false
		}
		if requireUseful && !usefulCanonicalTitleVariant(candidate) {
			return false
		}
		key := indexer.NormalizeTitleForDedup(candidate)
		if key == "" {
			key = strings.ToLower(candidate)
		}
		if _, ok := seen[key]; ok {
			return false
		}
		seen[key] = struct{}{}
		derived := sourceKey != key
		variants = append(variants, canonicalTitleVariant{
			title:             candidate,
			exactTitleOnly:    derived,
			allowRankTieBreak: derived,
			kind:              kind,
		})
		return true
	}

	stripped := stripCanonicalTitleDescriptor(title)

	if sourceKey != "" {
		add(title, false, canonicalTitleVariantSource)
	}
	add(stripped, true, canonicalTitleVariantDescriptor)

	if isGerman {
		for _, segment := range translatedOriginalTitleSegments(title, stripped) {
			add(segment, true, canonicalTitleVariantRightSegment)
		}
	}

	for _, segment := range rightCanonicalTitleSegments(title, stripped) {
		add(stripCanonicalTitleDescriptor(segment), true, canonicalTitleVariantRightSegment)
		add(segment, true, canonicalTitleVariantRightSegment)
	}

	for _, segment := range nonLatinCanonicalTitleSegments(title) {
		add(segment, true, canonicalTitleVariantRightSegment)
	}

	for _, segment := range leftCanonicalTitleSegments(title, stripped) {
		add(stripCanonicalTitleDescriptor(segment), true, canonicalTitleVariantLeftSegment)
		add(segment, true, canonicalTitleVariantLeftSegment)
	}
	return variants
}

func isGermanBookLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ger", "deu", "de":
		return true
	default:
		return false
	}
}

func cleanCanonicalTitleVariant(title string) string {
	return norm.NFC.String(strings.Trim(strings.TrimSpace(title), `"'“”‘’.,;`))
}

func stripCanonicalTitleDescriptor(title string) string {
	title = cleanCanonicalTitleVariant(title)
	for {
		next := title
		for _, sep := range []string{":", " - ", " – ", " — "} {
			idx := strings.LastIndex(next, sep)
			if idx <= 0 {
				continue
			}
			head := cleanCanonicalTitleVariant(next[:idx])
			tail := cleanCanonicalTitleVariant(next[idx+len(sep):])
			if usefulCanonicalTitleVariant(head) && canonicalTitleDescriptor(tail) {
				next = head
				break
			}
		}
		if next == title {
			return title
		}
		title = next
	}
}

func translatedOriginalTitleSegments(titles ...string) []string {
	var segments []string
	for _, title := range titles {
		for _, sep := range []string{" – ", " — ", " - "} {
			parts := strings.Split(cleanCanonicalTitleVariant(title), sep)
			if len(parts) < 2 {
				continue
			}
			left := cleanCanonicalTitleVariant(parts[0])
			right := cleanCanonicalTitleVariant(parts[1])
			if usefulCanonicalTitleVariant(left) && startsWithGermanArticle(right) {
				segments = append(segments, left)
			}
		}
		if segment := leadingGermanTranslatedTitleSegment(title); segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func rightCanonicalTitleSegments(titles ...string) []string {
	var segments []string
	for _, title := range titles {
		for _, sep := range []string{":", " / ", " – ", " — ", " - "} {
			parts := strings.Split(cleanCanonicalTitleVariant(title), sep)
			if len(parts) < 2 {
				continue
			}
			for _, segment := range parts[1:] {
				segment = cleanCanonicalTitleVariant(segment)
				if segment != "" {
					segments = append(segments, segment)
				}
			}
		}
	}
	return segments
}

func leftCanonicalTitleSegments(titles ...string) []string {
	var segments []string
	for _, title := range titles {
		for _, sep := range []string{":", " / ", " – ", " — ", " - "} {
			parts := strings.Split(cleanCanonicalTitleVariant(title), sep)
			if len(parts) < 2 {
				continue
			}
			segment := cleanCanonicalTitleVariant(parts[0])
			if segment != "" {
				segments = append(segments, segment)
			}
		}
	}
	return segments
}

func nonLatinCanonicalTitleSegments(title string) []string {
	var segments []string
	var b strings.Builder
	spacePending := false
	flush := func() {
		segment := cleanCanonicalTitleVariant(b.String())
		if usefulCanonicalTitleVariant(segment) {
			segments = append(segments, segment)
		}
		b.Reset()
		spacePending = false
	}
	for _, r := range cleanCanonicalTitleVariant(title) {
		switch {
		case isCanonicalNonLatinScript(r):
			if spacePending && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			spacePending = false
		case unicode.IsSpace(r):
			spacePending = b.Len() > 0
		default:
			if b.Len() > 0 {
				flush()
			}
		}
	}
	if b.Len() > 0 {
		flush()
	}
	return segments
}

func isCanonicalNonLatinScript(r rune) bool {
	return unicode.In(r,
		unicode.Arabic,
		unicode.Cyrillic,
		unicode.Han,
		unicode.Hangul,
		unicode.Hiragana,
		unicode.Katakana,
	)
}

func startsWithGermanArticle(title string) bool {
	words := strings.Fields(cleanCanonicalTitleVariant(title))
	if len(words) == 0 {
		return false
	}
	switch strings.ToLower(words[0]) {
	case "der", "die", "das":
		return true
	default:
		return false
	}
}

func leadingGermanTranslatedTitleSegment(title string) string {
	words := strings.Fields(cleanCanonicalTitleVariant(title))
	if len(words) < 4 {
		return ""
	}
	switch strings.ToLower(words[1]) {
	case "der", "die", "das":
		if canonicalTitleDescriptorSuffix(strings.Join(words[2:], " ")) {
			return words[0]
		}
	default:
	}
	return ""
}

func usefulCanonicalTitleVariant(title string) bool {
	clean := seriesmatch.CleanTitle(title)
	if clean == "" {
		return false
	}
	words := strings.Fields(clean)
	return len(words) > 0 && !canonicalTitleDescriptor(title)
}

func canonicalTitleDescriptor(title string) bool {
	clean := seriesmatch.CleanTitle(title)
	if clean == "" {
		return true
	}
	for _, word := range strings.Fields(clean) {
		if canonicalTitleDescriptorWord(word) {
			continue
		}
		return false
	}
	return true
}

func canonicalTitleDescriptorSuffix(title string) bool {
	words := strings.Fields(seriesmatch.CleanTitle(title))
	if len(words) == 0 {
		return false
	}
	return canonicalTitleDescriptorWord(words[len(words)-1])
}

func canonicalTitleDescriptorWord(word string) bool {
	switch word {
	case "roman", "science", "fiction", "sci", "fi", "translated", "translation", "new", "neu", "ubersetzt", "übersetzt":
		return true
	default:
		return false
	}
}

func (a *Aggregator) canonicalPrimaryBookSearch(ctx context.Context, query primaryBookCanonicalQuery, source models.Book, sourceAuthor string) (*canonicalPrimaryBookMatch, bool) {
	results, err := a.primary.SearchBooks(ctx, query.query)
	if err != nil {
		slog.Debug("primary canonical book search failed", "query", query.query, "title", source.Title, "author", sourceAuthor, "error", err)
		return nil, false
	}
	return a.canonicalPrimaryBookFromResults(query, source, sourceAuthor, results, false)
}

func (a *Aggregator) canonicalPrimaryBookFromQueries(ctx context.Context, queries []primaryBookCanonicalQuery, lookup func(primaryBookCanonicalQuery) (*canonicalPrimaryBookMatch, bool)) (*models.Book, canonicalPrimaryBookStatus) {
	hasSegmentVariant := canonicalQueriesHaveSegmentVariant(queries)
	var provisional *canonicalPrimaryBookMatch
	provisionalAmbiguous := false
	directISBNConfirmed := false
	for _, query := range queries {
		match, ok := lookup(query)
		if !ok {
			continue
		}
		if match.sameSource {
			if query.isISBN {
				directISBNConfirmed = true
				break
			}
			continue
		}
		if query.isISBN {
			return a.fetchCanonicalPrimaryBook(ctx, match), canonicalPrimaryBookMatched
		}
		if hasSegmentVariant || shouldDeferCanonicalPrimaryBookMatch(query, match, hasSegmentVariant) {
			provisional, provisionalAmbiguous = betterCanonicalProvisionalMatch(provisional, provisionalAmbiguous, match)
			continue
		}
		return a.fetchCanonicalPrimaryBook(ctx, match), canonicalPrimaryBookMatched
	}
	if provisional != nil && !provisionalAmbiguous {
		return a.fetchCanonicalPrimaryBook(ctx, provisional), canonicalPrimaryBookMatched
	}
	if directISBNConfirmed {
		return nil, canonicalPrimaryBookDirectISBNConfirmed
	}
	return nil, canonicalPrimaryBookNoMatch
}

func canonicalQueriesHaveSegmentVariant(queries []primaryBookCanonicalQuery) bool {
	for _, query := range queries {
		if query.variantKind == canonicalTitleVariantRightSegment || query.variantKind == canonicalTitleVariantLeftSegment {
			return true
		}
	}
	return false
}

func shouldDeferCanonicalPrimaryBookMatch(query primaryBookCanonicalQuery, match *canonicalPrimaryBookMatch, hasSegmentVariant bool) bool {
	if !hasSegmentVariant || match == nil {
		return false
	}
	if query.allowEditionTitleMatch && query.variantKind == canonicalTitleVariantDescriptor {
		return true
	}
	if query.allowEditionTitleMatch && query.variantKind == canonicalTitleVariantSource {
		return !match.candidate.workExact
	}
	if query.isISBN || query.variantKind != canonicalTitleVariantSource {
		return false
	}
	return !match.candidate.titleExact
}

func betterCanonicalProvisionalMatch(current *canonicalPrimaryBookMatch, ambiguous bool, next *canonicalPrimaryBookMatch) (*canonicalPrimaryBookMatch, bool) {
	if current == nil {
		return next, false
	}
	if next == nil {
		return current, ambiguous
	}
	switch compareBookCandidate(next.candidate, current.candidate) {
	case 1:
		return next, false
	case 0:
		if strings.TrimSpace(next.candidate.book.ForeignID) != strings.TrimSpace(current.candidate.book.ForeignID) {
			return current, true
		}
	}
	return current, ambiguous
}

func (a *Aggregator) canonicalPrimaryBookAuthorWorks(ctx context.Context, source models.Book, sourceAuthor string) (*models.Book, bool) {
	if source.Author == nil || source.Author.ForeignID == "" {
		return nil, false
	}
	authorID := strings.TrimSpace(source.Author.ForeignID)
	if source.Author.MetadataProvider != "" && normalizedProviderName(source.Author.MetadataProvider) != normalizedProviderName(providerName(a.primary)) {
		return nil, false
	}
	if source.Author.MetadataProvider == "" && providerNameForForeignID(authorID) != normalizedProviderName(providerName(a.primary)) {
		return nil, false
	}
	if _, ok := a.primary.(worksProvider); !ok {
		return nil, false
	}
	works, err := a.rawPrimaryAuthorWorks(ctx, authorID)
	if err != nil {
		slog.Debug("primary canonical author works failed", "author", authorID, "title", source.Title, "error", err)
		return nil, false
	}
	queries := make([]primaryBookCanonicalQuery, 0, len(canonicalTitleVariants(source.Title, source.Language)))
	for _, variant := range canonicalTitleVariants(source.Title, source.Language) {
		queries = append(queries, primaryBookCanonicalQuery{
			query:                  "authorworks:" + authorID,
			matchTitle:             variant.title,
			exactTitleOnly:         variant.exactTitleOnly,
			allowRankTieBreak:      variant.allowRankTieBreak,
			allowEditionTitleMatch: isGermanBookLanguage(source.Language),
			hasAuthor:              true,
			variantKind:            variant.kind,
		})
	}
	canonical, status := a.canonicalPrimaryBookFromQueries(ctx, queries, func(query primaryBookCanonicalQuery) (*canonicalPrimaryBookMatch, bool) {
		return a.canonicalPrimaryBookFromResults(query, source, sourceAuthor, works, true)
	})
	return canonical, status == canonicalPrimaryBookMatched
}

func (a *Aggregator) canonicalPrimaryBookFromResults(query primaryBookCanonicalQuery, source models.Book, sourceAuthor string, results []models.Book, assumeAuthorMatch bool) (*canonicalPrimaryBookMatch, bool) {
	matches := make([]bookMatchCandidate, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for rank, result := range results {
		if result.ForeignID == "" {
			continue
		}
		if _, ok := seen[result.ForeignID]; ok {
			continue
		}
		seen[result.ForeignID] = struct{}{}
		author := textutil.AuthorMatchResult{Kind: textutil.AuthorMatchExact, Score: 1}
		authorMatchedAlias := false
		if !assumeAuthorMatch || bookAuthorName(result) != "" {
			author, authorMatchedAlias = bookAuthorMatch(sourceAuthor, result)
			if author.Kind != textutil.AuthorMatchExact && author.Kind != textutil.AuthorMatchFuzzyAuto {
				continue
			}
		}
		candidate, ok := scoreBookCandidate(query, result, author, authorMatchedAlias, rank, query.allowEditionTitleMatch || authorMatchedAlias)
		if ok {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil, false
	}
	best := matches[0]
	ambiguous := false
	for _, candidate := range matches[1:] {
		switch compareBookCandidate(candidate, best) {
		case 1:
			best = candidate
			ambiguous = false
		case 0:
			ambiguous = true
		}
	}
	if ambiguous {
		slog.Debug("primary canonical book search ambiguous", "query", query.query, "title", source.Title, "author", sourceAuthor)
		return nil, false
	}
	if strings.TrimSpace(best.book.ForeignID) == strings.TrimSpace(source.ForeignID) {
		return &canonicalPrimaryBookMatch{sameSource: true}, true
	}
	return &canonicalPrimaryBookMatch{candidate: best}, true
}

func (a *Aggregator) fetchCanonicalPrimaryBook(ctx context.Context, match *canonicalPrimaryBookMatch) *models.Book {
	full, err := a.primary.GetBook(ctx, match.candidate.book.ForeignID)
	if err != nil {
		slog.Debug("primary canonical book fetch failed", "foreignID", match.candidate.book.ForeignID, "error", err)
		return &match.candidate.book
	}
	if full == nil {
		return &match.candidate.book
	}
	return full
}

func scoreBookCandidate(query primaryBookCanonicalQuery, candidate models.Book, author textutil.AuthorMatchResult, authorAlias bool, resultRank int, allowEditionTitleMatch bool) (bookMatchCandidate, bool) {
	workExact, titleExact, titleScore, editionExact := bestBookTitleMatch(query.matchTitle, candidate, allowEditionTitleMatch)
	if query.exactTitleOnly && !query.isISBN && !titleExact {
		return bookMatchCandidate{}, false
	}
	if !query.isISBN && titleScore < 88 {
		if !dominantCatalogCandidate(candidate, author, query) {
			return bookMatchCandidate{}, false
		}
	}
	if !query.isISBN && !query.hasAuthor && candidate.EditionCount < 10 {
		return bookMatchCandidate{}, false
	}
	if !query.isISBN && !titleExact && !editionExact && weakPartialTitleMatch(query.matchTitle, candidate.Title) {
		return bookMatchCandidate{}, false
	}
	return bookMatchCandidate{
		book:         candidate,
		titleExact:   titleExact,
		workExact:    workExact,
		titleScore:   titleScore,
		authorKind:   author.Kind,
		authorScore:  author.Score,
		authorAlias:  authorAlias,
		resultRank:   resultRank,
		rankTie:      query.allowRankTieBreak,
		editionExact: editionExact,
		variantKind:  query.variantKind,
		isISBN:       query.isISBN,
	}, true
}

func dominantCatalogCandidate(candidate models.Book, author textutil.AuthorMatchResult, query primaryBookCanonicalQuery) bool {
	if query.exactTitleOnly || query.isISBN || candidate.EditionCount < 10 {
		return false
	}
	return author.Kind == textutil.AuthorMatchExact
}

func bestBookTitleMatch(matchTitle string, candidate models.Book, allowEditionTitleMatch bool) (bool, bool, int, bool) {
	workExact := workTitleExactMatch(matchTitle, candidate.Title)
	bestExact := titleExactMatch(matchTitle, candidate.Title)
	bestScore := titleMatchScore(matchTitle, candidate.Title)
	bestEditionExact := false
	if !allowEditionTitleMatch {
		return workExact, bestExact, bestScore, bestEditionExact
	}
	for _, edition := range candidate.Editions {
		if strings.TrimSpace(edition.Title) == "" {
			continue
		}
		exact := titleExactMatch(matchTitle, edition.Title)
		score := titleMatchScore(matchTitle, edition.Title)
		if exact && !bestExact {
			bestExact = true
			bestEditionExact = true
		}
		if score > bestScore {
			bestScore = score
			bestEditionExact = exact
		} else if exact && score == bestScore {
			bestEditionExact = true
		}
	}
	return workExact, bestExact, bestScore, bestEditionExact
}

func weakPartialTitleMatch(matchTitle, candidateTitle string) bool {
	matchWords := strings.Fields(seriesmatch.CleanTitle(matchTitle))
	candidateWords := strings.Fields(seriesmatch.CleanTitle(candidateTitle))
	if len(matchWords) == 0 || len(candidateWords) == 0 || strings.Join(matchWords, " ") == strings.Join(candidateWords, " ") {
		return false
	}
	if containsWordSequence(candidateWords, matchWords) {
		return len(candidateWords) > len(matchWords)
	}
	if containsWordSequence(matchWords, candidateWords) {
		return len(matchWords)-len(candidateWords) >= 3
	}
	return false
}

func containsWordSequence(words, sequence []string) bool {
	if len(sequence) == 0 || len(sequence) > len(words) {
		return false
	}
	for start := 0; start <= len(words)-len(sequence); start++ {
		matches := true
		for idx := range sequence {
			if words[start+idx] != sequence[idx] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func titleExactMatch(a, b string) bool {
	left := canonicalExactTitleKey(a)
	right := canonicalExactTitleKey(b)
	return left != "" && left == right
}

func workTitleExactMatch(a, b string) bool {
	left := canonicalWorkTitleExactKey(a)
	right := canonicalWorkTitleExactKey(b)
	return left != "" && left == right
}

func canonicalWorkTitleExactKey(title string) string {
	title = strings.ReplaceAll(cleanCanonicalTitleVariant(title), "&", " and ")
	return indexer.NormalizeTitleForDedup(title)
}

func canonicalExactTitleKey(title string) string {
	title = strings.ReplaceAll(cleanCanonicalTitleVariant(title), "&", " and ")
	if clean := seriesmatch.CleanTitle(title); clean != "" {
		return clean
	}
	return indexer.NormalizeTitleForDedup(title)
}

func titleMatchScore(a, b string) int {
	if titleExactMatch(a, b) {
		return 100
	}
	return seriesmatch.TitleScore(cleanCanonicalTitleVariant(a), cleanCanonicalTitleVariant(b))
}

func bookAuthorName(book models.Book) string {
	if book.Author == nil {
		return ""
	}
	return strings.TrimSpace(book.Author.Name)
}

func bookAuthorMatch(sourceAuthor string, book models.Book) (textutil.AuthorMatchResult, bool) {
	best := textutil.MatchAuthorName(sourceAuthor, bookAuthorName(book))
	bestMatchedAlias := false
	if book.Author == nil {
		return best, bestMatchedAlias
	}
	for _, alias := range book.Author.AlternateNames {
		next := textutil.MatchAuthorName(sourceAuthor, alias)
		if compareAuthorMatch(next, best) == 1 {
			best = next
			bestMatchedAlias = true
		}
	}
	return best, bestMatchedAlias
}

func compareAuthorMatch(a, b textutil.AuthorMatchResult) int {
	if authorMatchRank(a.Kind) != authorMatchRank(b.Kind) {
		if authorMatchRank(a.Kind) > authorMatchRank(b.Kind) {
			return 1
		}
		return -1
	}
	if math.Abs(a.Score-b.Score) > 0.001 {
		if a.Score > b.Score {
			return 1
		}
		return -1
	}
	return 0
}

func compareBookCandidate(a, b bookMatchCandidate) int {
	if a.isISBN != b.isISBN {
		if a.isISBN {
			return 1
		}
		return -1
	}
	if a.titleExact != b.titleExact {
		if a.titleExact {
			return 1
		}
		return -1
	}
	if a.titleScore != b.titleScore {
		if a.titleScore > b.titleScore {
			return 1
		}
		return -1
	}
	if authorMatchRank(a.authorKind) != authorMatchRank(b.authorKind) {
		if authorMatchRank(a.authorKind) > authorMatchRank(b.authorKind) {
			return 1
		}
		return -1
	}
	if a.authorAlias != b.authorAlias {
		if !a.authorAlias {
			return 1
		}
		return -1
	}
	if math.Abs(a.authorScore-b.authorScore) > 0.001 {
		if a.authorScore > b.authorScore {
			return 1
		}
		return -1
	}
	if a.editionExact != b.editionExact {
		if a.editionExact {
			return 1
		}
		return -1
	}
	if catalogDominates(a.book.EditionCount, b.book.EditionCount) {
		return 1
	}
	if catalogDominates(b.book.EditionCount, a.book.EditionCount) {
		return -1
	}
	if a.variantKind != b.variantKind {
		if a.variantKind == canonicalTitleVariantSource && a.workExact {
			return 1
		}
		if b.variantKind == canonicalTitleVariantSource && b.workExact {
			return -1
		}
		if a.variantKind == canonicalTitleVariantRightSegment && b.variantKind == canonicalTitleVariantLeftSegment {
			return 1
		}
		if b.variantKind == canonicalTitleVariantRightSegment && a.variantKind == canonicalTitleVariantLeftSegment {
			return -1
		}
	}
	if a.workExact != b.workExact {
		if a.workExact {
			return 1
		}
		return -1
	}
	if (a.rankTie || b.rankTie) && a.resultRank != b.resultRank {
		if a.resultRank < b.resultRank {
			return 1
		}
		return -1
	}
	return 0
}

func catalogDominates(a, b int) bool {
	if a < 10 || a <= b {
		return false
	}
	if b <= 1 {
		return true
	}
	return a >= b*3
}

func authorMatchRank(kind textutil.AuthorMatchKind) int {
	switch kind {
	case textutil.AuthorMatchExact:
		return 2
	case textutil.AuthorMatchFuzzyAuto:
		return 1
	default:
		return 0
	}
}

func (a *Aggregator) providerForForeignID(foreignID string) Provider {
	if a == nil {
		return nil
	}
	want := providerNameForForeignID(foreignID)
	if want == "" {
		return a.primary
	}
	for _, provider := range a.providers() {
		if provider == nil {
			continue
		}
		if normalizedProviderName(provider.Name()) == want {
			return provider
		}
	}
	if want == "openlibrary" || want == normalizedProviderName(providerName(a.primary)) {
		return a.primary
	}
	return nil
}

func providerName(provider Provider) string {
	if provider == nil {
		return ""
	}
	return provider.Name()
}

func sameProvider(a, b Provider) bool {
	return normalizedProviderName(providerName(a)) == normalizedProviderName(providerName(b))
}

func providerNameForForeignID(foreignID string) string {
	foreignID = strings.TrimSpace(foreignID)
	switch {
	case strings.HasPrefix(foreignID, "gb:"):
		return "googlebooks"
	case strings.HasPrefix(foreignID, "hc:"):
		return "hardcover"
	case strings.HasPrefix(foreignID, "dnb:"):
		return "dnb"
	default:
		return "openlibrary"
	}
}

func normalizedProviderName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ol", "openlibrary", "open_library":
		return "openlibrary"
	case "gb", "googlebooks", "google_books":
		return "googlebooks"
	case "hc", "hardcover":
		return "hardcover"
	case "dnb":
		return "dnb"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func (a *Aggregator) providers() []Provider {
	if a == nil {
		return nil
	}
	providers := make([]Provider, 0, len(a.enrichers)+1)
	if a.primary != nil {
		providers = append(providers, a.primary)
	}
	providers = append(providers, a.enrichers...)
	return providers
}

// SearchSeries queries metadata providers that expose series catalog search.
func (a *Aggregator) SearchSeries(ctx context.Context, query string, limit int) ([]SeriesSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	key := "series-search:" + strings.ToLower(query) + ":" + strconv.Itoa(limit)
	if cached, ok := a.cache.get(key); ok {
		return cached.([]SeriesSearchResult), nil
	}
	var lastErr error
	for _, provider := range a.seriesCatalogProviders() {
		results, err := provider.SearchSeries(ctx, query, limit)
		if err != nil {
			lastErr = err
			slog.Debug("series search failed", "error", err)
			continue
		}
		if results == nil {
			results = []SeriesSearchResult{}
		}
		a.cache.set(key, results)
		return results, nil
	}
	return nil, lastErr
}

// GetSeriesCatalog fetches the ordered book catalog for a provider series.
func (a *Aggregator) GetSeriesCatalog(ctx context.Context, foreignID string) (*SeriesCatalog, error) {
	foreignID = strings.TrimSpace(foreignID)
	if foreignID == "" {
		return nil, nil
	}
	key := "series-catalog:" + foreignID
	if cached, ok := a.cache.get(key); ok {
		return cached.(*SeriesCatalog), nil
	}
	var lastErr error
	for _, provider := range a.seriesCatalogProviders() {
		catalog, err := provider.GetSeriesCatalog(ctx, foreignID)
		if err != nil {
			lastErr = err
			slog.Debug("series catalog failed", "foreignID", foreignID, "error", err)
			continue
		}
		if catalog != nil {
			a.cache.set(key, catalog)
		}
		return catalog, nil
	}
	return nil, lastErr
}

func (a *Aggregator) seriesCatalogProviders() []SeriesCatalogProvider {
	if a == nil {
		return nil
	}
	providers := make([]SeriesCatalogProvider, 0, len(a.enrichers)+1)
	if provider, ok := a.primary.(SeriesCatalogProvider); ok {
		providers = append(providers, provider)
	}
	for _, enricher := range a.enrichers {
		if provider, ok := enricher.(SeriesCatalogProvider); ok {
			providers = append(providers, provider)
		}
	}
	return providers
}

// enrichBook tries to fill in missing data from secondary providers.
// It fills Description, AverageRating/RatingsCount, and ImageURL when
// the primary provider (OpenLibrary) left them empty or sparse.
func (a *Aggregator) enrichBook(ctx context.Context, book *models.Book) {
	for _, enricher := range a.enrichers {
		enriched, err := enricher.SearchBooks(ctx, book.Title)
		if err != nil {
			slog.Debug("enrichment failed", "provider", enricher.Name(), "error", err)
			continue
		}
		if len(enriched) == 0 {
			continue
		}
		e := enriched[0]
		if len(e.Description) > len(book.Description) {
			book.Description = e.Description
			slog.Debug("enriched description", "provider", enricher.Name(), "book", book.Title)
		}
		if book.AverageRating == 0 && e.AverageRating > 0 {
			book.AverageRating = e.AverageRating
			book.RatingsCount = e.RatingsCount
		}
		if book.ImageURL == "" && e.ImageURL != "" {
			book.ImageURL = e.ImageURL
			slog.Debug("enriched cover", "provider", enricher.Name(), "book", book.Title)
		}
	}
}

// ttlCache is a simple in-process cache with TTL expiry.
type ttlCache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
	ttl   time.Duration
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
