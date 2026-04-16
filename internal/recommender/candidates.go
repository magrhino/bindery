package recommender

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// GenerateSeries produces candidates for the next unowned book in each series
// the user has started.
func GenerateSeries(
	ctx context.Context,
	books *db.BookRepo,
	series *db.SeriesRepo,
	profile *UserProfile,
) []models.RecommendationCandidate {
	var candidates []models.RecommendationCandidate

	for seriesID, state := range profile.SeriesState {
		full, err := series.GetByID(ctx, seriesID)
		if err != nil || full == nil {
			continue
		}

		// Find next unowned book after the user's max position.
		nextPos := state.MaxPosition + 1
		for _, sb := range full.Books {
			pos := parsePosition(sb.PositionInSeries)
			if pos != nextPos || sb.Book == nil {
				continue
			}
			if profile.OwnedForeignIDs[sb.Book.ForeignID] {
				continue
			}

			c := bookToCandidate(sb.Book)
			c.RecType = models.RecTypeSeries
			c.Reason = fmt.Sprintf("Next in %s", state.SeriesTitle)
			c.SeriesID = &seriesID
			c.SeriesPos = sb.PositionInSeries
			candidates = append(candidates, c)
			break
		}

		// Also surface books that fill gaps.
		for _, missingPos := range state.MissingPositions {
			for _, sb := range full.Books {
				pos := parsePosition(sb.PositionInSeries)
				if pos != missingPos || sb.Book == nil {
					continue
				}
				if profile.OwnedForeignIDs[sb.Book.ForeignID] {
					continue
				}

				sid := seriesID
				c := bookToCandidate(sb.Book)
				c.RecType = models.RecTypeSeries
				c.Reason = fmt.Sprintf("Missing from %s", state.SeriesTitle)
				c.SeriesID = &sid
				c.SeriesPos = sb.PositionInSeries
				candidates = append(candidates, c)
				break
			}
		}
	}

	return candidates
}

// GenerateAuthorNew produces candidates from monitored authors' works that
// the user doesn't own yet.
func GenerateAuthorNew(
	ctx context.Context,
	books *db.BookRepo,
	authors *db.AuthorRepo,
	profile *UserProfile,
) []models.RecommendationCandidate {
	var candidates []models.RecommendationCandidate

	for authorID := range profile.MonitoredAuthors {
		author, err := authors.GetByID(ctx, authorID)
		if err != nil || author == nil {
			continue
		}

		authorBooks, err := books.ListByAuthor(ctx, authorID)
		if err != nil {
			slog.Warn("recommender: failed to list books for author", "authorId", authorID, "error", err)
			continue
		}

		for i := range authorBooks {
			b := &authorBooks[i]
			if profile.OwnedForeignIDs[b.ForeignID] {
				continue
			}
			// Only recommend books with wanted or skipped status (they're in
			// the DB but the user hasn't grabbed them yet).
			if b.Status != models.BookStatusWanted && b.Status != models.BookStatusSkipped {
				continue
			}

			c := bookToCandidate(b)
			c.RecType = models.RecTypeAuthorNew
			c.Reason = fmt.Sprintf("New from %s", author.Name)
			c.AuthorID = &authorID
			c.AuthorName = author.Name
			candidates = append(candidates, c)
		}
	}

	return candidates
}

// GenerateGenreSimilar produces candidates from series books that match the
// user's genre profile but aren't owned yet.
func GenerateGenreSimilar(
	ctx context.Context,
	books *db.BookRepo,
	series *db.SeriesRepo,
	profile *UserProfile,
) []models.RecommendationCandidate {
	var candidates []models.RecommendationCandidate

	// Find the user's top genre for the reason string.
	topGenre := topGenre(profile)

	// Walk all series the user has started and find unowned books.
	allSeries, err := series.List(ctx)
	if err != nil {
		return nil
	}

	for _, s := range allSeries {
		full, err := series.GetByID(ctx, s.ID)
		if err != nil || full == nil {
			continue
		}

		for _, sb := range full.Books {
			if sb.Book == nil || profile.OwnedForeignIDs[sb.Book.ForeignID] {
				continue
			}
			// Skip books already covered by series candidates.
			if _, inSeries := profile.SeriesState[s.ID]; inSeries {
				continue
			}

			c := bookToCandidate(sb.Book)
			c.RecType = models.RecTypeGenreSimilar
			if topGenre != "" {
				c.Reason = fmt.Sprintf("Because of your %s library", topGenre)
			} else {
				c.Reason = "Similar to books in your library"
			}
			candidates = append(candidates, c)
		}
	}

	return candidates
}

// GenerateSerendipity samples random candidates from the available pool that
// are outside the user's top-5 genres. This breaks filter bubbles.
func GenerateSerendipity(
	ctx context.Context,
	books *db.BookRepo,
	series *db.SeriesRepo,
	profile *UserProfile,
	count int,
) []models.RecommendationCandidate {
	top5 := topNGenres(profile, 5)

	// Collect eligible books from series not started by the user.
	var pool []models.RecommendationCandidate
	allSeries, err := series.List(ctx)
	if err != nil {
		return nil
	}

	for _, s := range allSeries {
		full, err := series.GetByID(ctx, s.ID)
		if err != nil || full == nil {
			continue
		}

		for _, sb := range full.Books {
			if sb.Book == nil || profile.OwnedForeignIDs[sb.Book.ForeignID] {
				continue
			}

			// Must be outside the user's top-5 genres.
			if hasTopGenre(sb.Book.Genres, top5) {
				continue
			}

			c := bookToCandidate(sb.Book)
			c.RecType = models.RecTypeSerendipity
			c.Reason = "Something different"
			pool = append(pool, c)
		}
	}

	// Shuffle and take up to count.
	rand.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})
	if len(pool) > count {
		pool = pool[:count]
	}
	return pool
}

// bookToCandidate converts a models.Book into a RecommendationCandidate.
func bookToCandidate(b *models.Book) models.RecommendationCandidate {
	c := models.RecommendationCandidate{
		ForeignID:    b.ForeignID,
		Title:        b.Title,
		AuthorID:     &b.AuthorID,
		ImageURL:     b.ImageURL,
		Description:  b.Description,
		Genres:       b.Genres,
		Rating:       b.AverageRating,
		RatingsCount: b.RatingsCount,
		ReleaseDate:  b.ReleaseDate,
		Language:     b.Language,
		MediaType:    b.MediaType,
	}
	if c.MediaType == "" {
		c.MediaType = models.MediaTypeEbook
	}
	return c
}

// topGenre returns the highest-weighted genre in the user's profile.
func topGenre(p *UserProfile) string {
	var best string
	var bestW float64
	for g, w := range p.GenreWeights {
		if w > bestW {
			bestW = w
			best = g
		}
	}
	return best
}

// topNGenres returns the top N genres by weight.
func topNGenres(p *UserProfile, n int) map[string]bool {
	type gw struct {
		genre  string
		weight float64
	}
	var all []gw
	for g, w := range p.GenreWeights {
		all = append(all, gw{g, w})
	}
	// Simple selection: find top N by iterating N times.
	result := make(map[string]bool, n)
	for range n {
		var best int
		var bestW float64 = -1
		for i, g := range all {
			if !result[g.genre] && g.weight > bestW {
				bestW = g.weight
				best = i
			}
		}
		if bestW >= 0 {
			result[all[best].genre] = true
		}
	}
	return result
}

// hasTopGenre checks if any of the book's genres are in the top set.
func hasTopGenre(genres []string, top map[string]bool) bool {
	for _, g := range genres {
		if top[strings.ToLower(strings.TrimSpace(g))] {
			return true
		}
	}
	return false
}
