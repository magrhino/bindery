package abs

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	fuzzy "github.com/creditx/go-fuzzywuzzy"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/textutil"
)

type hardcoverSeriesQuery struct {
	Title    string
	Sequence string
	FromABS  bool
}

type hardcoverSeriesCandidate struct {
	Result      metadata.SeriesSearchResult
	Catalog     *metadata.SeriesCatalog
	Book        metadata.SeriesCatalogBook
	Score       int
	MatchedBy   string
	SeriesScore int
	TitleScore  int
}

func (i *Importer) matchHardcoverSeries(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, book *models.Book, item NormalizedLibraryItem, stats *ImportStats) (metadataMergeResult, int) {
	if i.meta == nil || i.series == nil || i.books == nil || book == nil {
		return metadataMergeResult{}, 0
	}
	queries := hardcoverSeriesQueries(item)
	if len(queries) == 0 {
		return metadataMergeResult{}, 0
	}
	authorName := primaryAuthorName(item)
	if author != nil && strings.TrimSpace(author.Name) != "" {
		authorName = strings.TrimSpace(author.Name)
	}
	candidates := make(map[string]hardcoverSeriesCandidate)
	for _, query := range queries {
		results, err := i.meta.SearchSeries(ctx, query.Title, 5)
		if err != nil {
			slog.Warn("abs import: hardcover series search failed", "itemID", item.ItemID, "query", query.Title, "error", err)
			continue
		}
		for _, result := range results {
			catalog, err := i.meta.GetSeriesCatalog(ctx, result.ForeignID)
			if err != nil {
				slog.Warn("abs import: hardcover series catalog failed", "itemID", item.ItemID, "series", result.ForeignID, "error", err)
				continue
			}
			if catalog == nil {
				continue
			}
			candidate, ok := evaluateHardcoverSeriesCandidate(query, authorName, item, result, catalog)
			if !ok {
				continue
			}
			key := catalog.ForeignID
			if existing, exists := candidates[key]; !exists || candidate.Score > existing.Score {
				candidates[key] = candidate
			}
		}
	}
	selected, ambiguous := selectHardcoverSeriesCandidate(candidates)
	if ambiguous {
		return metadataMergeResult{Messages: []string{"hardcover series link skipped: match was ambiguous"}}, 0
	}
	if selected == nil {
		return metadataMergeResult{}, 0
	}

	created, matchedBy, err := i.upsertHardcoverSeries(ctx, cfg, runID, book.ID, selected.Catalog, selected.Book)
	if err != nil {
		slog.Warn("abs import: hardcover series link failed", "itemID", item.ItemID, "series", selected.Catalog.ForeignID, "error", err)
		return metadataMergeResult{}, 0
	}
	if stats != nil {
		if created {
			stats.SeriesCreated++
		} else if matchedBy != "" {
			stats.SeriesLinked++
		}
	}

	metaResult := metadataMergeResult{}
	if !cfg.DryRun && selected.Book.Book.ForeignID != "" {
		if mergeResult, err := i.mergeUpstreamBook(ctx, cfg, item, book, &selected.Book.Book, "hardcover_series"); err != nil {
			slog.Warn("abs import: hardcover series metadata merge failed", "itemID", item.ItemID, "book", selected.Book.Book.ForeignID, "error", err)
		} else {
			metaResult = mergeResult
		}
	}
	if !cfg.DryRun {
		extra, err := i.linkExistingHardcoverCatalogBooks(ctx, cfg, runID, author, selected.Catalog, book.ID, created)
		if err != nil {
			slog.Warn("abs import: hardcover catalog existing-book linking failed", "itemID", item.ItemID, "series", selected.Catalog.ForeignID, "error", err)
		} else if stats != nil {
			stats.SeriesLinked += extra
		}
	}
	if matchedBy != "" {
		metaResult.Messages = append(metaResult.Messages, fmt.Sprintf("hardcover series linked by %s", selected.MatchedBy))
	}
	return metaResult, 1
}

func hardcoverSeriesQueries(item NormalizedLibraryItem) []hardcoverSeriesQuery {
	queries := make([]hardcoverSeriesQuery, 0, len(item.Series)+1)
	seen := map[string]struct{}{}
	for _, series := range item.Series {
		title := strings.TrimSpace(series.Name)
		if title == "" {
			continue
		}
		key := normalizeTitle(title)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		queries = append(queries, hardcoverSeriesQuery{
			Title:    title,
			Sequence: strings.TrimSpace(series.Sequence),
			FromABS:  true,
		})
	}
	if len(queries) > 0 {
		return queries
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		queries = append(queries, hardcoverSeriesQuery{Title: title})
	}
	return queries
}

func evaluateHardcoverSeriesCandidate(query hardcoverSeriesQuery, authorName string, item NormalizedLibraryItem, result metadata.SeriesSearchResult, catalog *metadata.SeriesCatalog) (hardcoverSeriesCandidate, bool) {
	if catalog == nil || catalog.ForeignID == "" || len(catalog.Books) == 0 {
		return hardcoverSeriesCandidate{}, false
	}
	hcAuthor := firstNonEmpty(result.AuthorName, catalog.AuthorName)
	authorScore := hardcoverAuthorScore(authorName, hcAuthor)
	if strings.TrimSpace(authorName) != "" && strings.TrimSpace(hcAuthor) != "" && authorScore == 0 {
		return hardcoverSeriesCandidate{}, false
	}
	matchedBook, titleScore, positionMatched, ok := matchHardcoverCatalogBook(item, query.Sequence, catalog.Books)
	if !ok {
		return hardcoverSeriesCandidate{}, false
	}
	seriesScore := 0
	if query.FromABS {
		seriesScore = shelfarrTitleScore(query.Title, firstNonEmpty(result.Title, catalog.Title))
		if normalizeSeriesName(query.Title) == normalizeSeriesName(firstNonEmpty(result.Title, catalog.Title)) {
			seriesScore = 100
		}
		if seriesScore < 80 {
			return hardcoverSeriesCandidate{}, false
		}
	}
	score := titleScore
	matchedBy := "hardcover_title"
	if positionMatched {
		score += 25
		matchedBy = "hardcover_position_title"
	}
	if query.FromABS {
		score += seriesScore / 2
		matchedBy = "hardcover_series_title"
		if positionMatched {
			matchedBy = "hardcover_series_position"
		}
	}
	score += authorScore
	if score < 105 {
		return hardcoverSeriesCandidate{}, false
	}
	return hardcoverSeriesCandidate{
		Result:      result,
		Catalog:     catalog,
		Book:        matchedBook,
		Score:       score,
		MatchedBy:   matchedBy,
		SeriesScore: seriesScore,
		TitleScore:  titleScore,
	}, true
}

func matchHardcoverCatalogBook(item NormalizedLibraryItem, sequence string, books []metadata.SeriesCatalogBook) (metadata.SeriesCatalogBook, int, bool, bool) {
	sequence = strings.TrimSpace(sequence)
	bestScore := 0
	var best metadata.SeriesCatalogBook
	matches := 0
	for _, book := range books {
		positionMatched := sequence != "" && sameSeriesPosition(sequence, book.Position)
		if sequence != "" && !positionMatched {
			continue
		}
		score := shelfarrTitleScore(item.Title, firstNonEmpty(book.Title, book.Book.Title))
		threshold := 88
		if positionMatched {
			threshold = 70
		}
		if score < threshold {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = book
			matches = 1
			continue
		}
		if score == bestScore {
			matches++
		}
	}
	if matches == 1 {
		return best, bestScore, sequence != "" && sameSeriesPosition(sequence, best.Position), true
	}
	return metadata.SeriesCatalogBook{}, 0, false, false
}

func selectHardcoverSeriesCandidate(candidates map[string]hardcoverSeriesCandidate) (*hardcoverSeriesCandidate, bool) {
	if len(candidates) == 0 {
		return nil, false
	}
	ordered := make([]hardcoverSeriesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ordered = append(ordered, candidate)
	}
	sort.SliceStable(ordered, func(a, b int) bool {
		if ordered[a].Score == ordered[b].Score {
			return ordered[a].Catalog.ForeignID < ordered[b].Catalog.ForeignID
		}
		return ordered[a].Score > ordered[b].Score
	})
	if len(ordered) > 1 && ordered[0].Score-ordered[1].Score < 10 {
		return nil, true
	}
	return &ordered[0], false
}

func (i *Importer) upsertHardcoverSeries(ctx context.Context, cfg ImportConfig, runID, bookID int64, catalog *metadata.SeriesCatalog, matchedBook metadata.SeriesCatalogBook) (bool, string, error) {
	if catalog == nil || strings.TrimSpace(catalog.Title) == "" || strings.TrimSpace(catalog.ForeignID) == "" {
		return false, "", nil
	}
	existing, err := i.series.GetByForeignID(ctx, catalog.ForeignID)
	if err != nil {
		return false, "", err
	}
	matchedBy := ""
	if existing == nil {
		match, ambiguous, err := i.findSeriesByTitle(ctx, catalog.Title)
		if err != nil {
			return false, "", err
		}
		if ambiguous {
			return false, "", fmt.Errorf("ambiguous existing series match for %q", catalog.Title)
		}
		if match != nil {
			existing = match
			matchedBy = "normalized_title"
			if shouldPromoteSeriesToHardcover(match, catalog) {
				if prior, err := i.series.GetByForeignID(ctx, catalog.ForeignID); err != nil {
					return false, "", err
				} else if prior == nil {
					if !cfg.DryRun {
						if err := i.series.UpdateForeignID(ctx, existing.ID, catalog.ForeignID); err != nil {
							return false, "", err
						}
					}
					existing.ForeignID = catalog.ForeignID
					matchedBy = "hardcover_promotion"
				}
			}
		}
	}
	created := false
	if existing == nil {
		existing = &models.Series{
			ForeignID:   catalog.ForeignID,
			Title:       strings.TrimSpace(catalog.Title),
			Description: "",
		}
		if !cfg.DryRun {
			if err := i.series.CreateOrGet(ctx, existing); err != nil {
				return false, "", err
			}
		}
		created = true
		matchedBy = "created"
	}
	localID := existing.ID
	if cfg.DryRun && created {
		localID = 0
	}
	metadata := map[string]any{
		"bookId":          bookID,
		"sequence":        strings.TrimSpace(matchedBook.Position),
		"matchedBy":       "hardcover_series",
		"hardcoverBookId": strings.TrimSpace(matchedBook.ForeignID),
	}
	linkExternalID := hardcoverSeriesLinkExternalID(catalog.ForeignID, bookID)
	linkCreated := cfg.DryRun
	if !cfg.DryRun {
		var err error
		linkCreated, err = i.series.LinkBookIfMissing(ctx, existing.ID, bookID, strings.TrimSpace(matchedBook.Position), true)
		if err != nil {
			return false, "", err
		}
		if linkCreated {
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   cfg.LibraryID,
				EntityType:  entityTypeSeries,
				ExternalID:  linkExternalID,
				LocalID:     existing.ID,
				ItemID:      "",
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return false, "", err
			}
		}
	}
	outcome := itemOutcomeLinked
	if created {
		outcome = itemOutcomeCreated
	}
	if linkCreated {
		_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, linkExternalID, localID, outcome, metadata)
	}
	return created, matchedBy, nil
}

func (i *Importer) linkExistingHardcoverCatalogBooks(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, catalog *metadata.SeriesCatalog, importedBookID int64, createdSeries bool) (int, error) {
	if catalog == nil || author == nil {
		return 0, nil
	}
	seriesRow, err := i.series.GetByForeignID(ctx, catalog.ForeignID)
	if err != nil {
		return 0, err
	}
	if seriesRow == nil {
		match, ambiguous, err := i.findSeriesByTitle(ctx, catalog.Title)
		if err != nil || ambiguous {
			return 0, err
		}
		seriesRow = match
	}
	if seriesRow == nil {
		return 0, nil
	}
	linked := 0
	for _, catalogBook := range catalog.Books {
		localBook, err := i.findExistingHardcoverCatalogBook(ctx, author.ID, catalogBook)
		if err != nil {
			return linked, err
		}
		if localBook == nil || localBook.ID == importedBookID {
			continue
		}
		if catalogBook.Book.Author != nil && textutil.MatchAuthorName(author.Name, catalogBook.Book.Author.Name).Kind == textutil.AuthorMatchNone {
			continue
		}
		linkCreated, err := i.series.LinkBookIfMissing(ctx, seriesRow.ID, localBook.ID, strings.TrimSpace(catalogBook.Position), false)
		if err != nil {
			return linked, err
		}
		if !linkCreated {
			continue
		}
		linkExternalID := hardcoverSeriesLinkExternalID(catalog.ForeignID, localBook.ID)
		if err := i.upsertProvenance(ctx, &models.ABSProvenance{
			SourceID:    cfg.SourceID,
			LibraryID:   cfg.LibraryID,
			EntityType:  entityTypeSeries,
			ExternalID:  linkExternalID,
			LocalID:     seriesRow.ID,
			ItemID:      "",
			ImportRunID: ptrInt64(runID),
		}); err != nil {
			return linked, err
		}
		outcome := itemOutcomeLinked
		if createdSeries {
			outcome = itemOutcomeCreated
		}
		_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, linkExternalID, seriesRow.ID, outcome, map[string]any{
			"bookId":          localBook.ID,
			"sequence":        strings.TrimSpace(catalogBook.Position),
			"matchedBy":       "hardcover_catalog_existing_book",
			"hardcoverBookId": strings.TrimSpace(catalogBook.ForeignID),
		})
		linked++
	}
	return linked, nil
}

func (i *Importer) findExistingHardcoverCatalogBook(ctx context.Context, authorID int64, catalogBook metadata.SeriesCatalogBook) (*models.Book, error) {
	if strings.TrimSpace(catalogBook.ForeignID) != "" {
		existing, err := i.books.GetByForeignID(ctx, catalogBook.ForeignID)
		if err != nil || existing != nil {
			return existing, err
		}
	}
	title := firstNonEmpty(catalogBook.Title, catalogBook.Book.Title)
	match, ambiguous, err := i.findBookByNormalizedTitle(ctx, authorID, title)
	if err != nil || ambiguous || match == nil {
		return nil, err
	}
	if shelfarrTitleScore(match.Title, title) < 92 {
		return nil, nil
	}
	return match, nil
}

func shouldPromoteSeriesToHardcover(existing *models.Series, catalog *metadata.SeriesCatalog) bool {
	if existing == nil || catalog == nil {
		return false
	}
	if !strings.HasPrefix(existing.ForeignID, "abs:series:") {
		return false
	}
	return normalizeSeriesName(existing.Title) == normalizeSeriesName(catalog.Title)
}

func hardcoverSeriesLinkExternalID(seriesForeignID string, bookID int64) string {
	seriesForeignID = strings.TrimSpace(seriesForeignID)
	if bookID <= 0 {
		return seriesForeignID + ":book:planned"
	}
	return fmt.Sprintf("%s:book:%d", seriesForeignID, bookID)
}

func hardcoverAuthorScore(absAuthor, hcAuthor string) int {
	absAuthor = strings.TrimSpace(absAuthor)
	hcAuthor = strings.TrimSpace(hcAuthor)
	if absAuthor == "" || hcAuthor == "" {
		return 0
	}
	match := textutil.MatchAuthorName(absAuthor, hcAuthor)
	switch match.Kind {
	case textutil.AuthorMatchExact:
		return 30
	case textutil.AuthorMatchFuzzyAuto:
		return 20
	default:
		score := shelfarrTitleScore(absAuthor, hcAuthor)
		if score >= 90 {
			return 15
		}
		return 0
	}
}

func sameSeriesPosition(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	return aerr == nil && berr == nil && math.Abs(af-bf) < 0.001
}

func normalizeSeriesName(name string) string {
	normalized := normalizeTitle(name)
	if normalized == "" {
		return ""
	}
	suffixes := map[string]struct{}{
		"series":     {},
		"trilogy":    {},
		"saga":       {},
		"chronicles": {},
		"cycle":      {},
		"books":      {},
		"novels":     {},
	}
	words := strings.Fields(normalized)
	if len(words) > 1 {
		if _, ok := suffixes[words[len(words)-1]]; ok {
			words = words[:len(words)-1]
		}
	}
	return strings.Join(words, " ")
}

func shelfarrTitleScore(a, b string) int {
	cleanA := shelfarrCleanTitle(a)
	cleanB := shelfarrCleanTitle(b)
	if cleanA == "" || cleanB == "" {
		return 0
	}
	return maxInt(
		fuzzy.TokenSetRatio(cleanA, cleanB),
		fuzzy.TokenSortRatio(cleanA, cleanB),
		fuzzy.Ratio(cleanA, cleanB),
		fuzzy.PartialRatio(cleanA, cleanB),
	)
}

func shelfarrCleanTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(' ')
		}
	}
	noise := map[string]struct{}{
		"a":     {},
		"an":    {},
		"the":   {},
		"novel": {},
		"book":  {},
	}
	words := strings.Fields(b.String())
	out := words[:0]
	for _, word := range words {
		if _, ok := noise[word]; ok {
			continue
		}
		out = append(out, word)
	}
	return strings.Join(out, " ")
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
