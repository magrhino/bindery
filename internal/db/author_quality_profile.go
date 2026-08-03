package db

import (
	"context"
	"log/slog"

	"github.com/vavallee/bindery/internal/models"
)

// ResolveAuthorQualityProfile returns the quality profile assigned to an
// author, or nil when there is nothing to enforce.
//
// It lives here rather than in either caller because the auto-grab path
// (internal/scheduler) and the interactive search path (internal/api) must
// agree on which profile governs a book: disagreeing would mean a release the
// user sees marked "rejected" in the search dialog gets grabbed anyway by the
// next scheduled sweep. Both feed the result to decision.QualityAllowed.
//
// Every failure mode returns nil — no profile assigned, no repo wired, or a
// lookup error. nil means "no format filter", which is the pre-#1693 behaviour
// and the only safe default: a profile we cannot read must not silently block
// every grab for that author.
func ResolveAuthorQualityProfile(ctx context.Context, repo *QualityProfileRepo, a *models.Author) *models.QualityProfile {
	if repo == nil || a == nil || a.QualityProfileID == nil {
		return nil
	}
	p, err := repo.GetByID(ctx, *a.QualityProfileID)
	if err != nil {
		slog.Warn("could not load author quality profile; format filter not applied",
			"author_id", a.ID, "quality_profile_id", *a.QualityProfileID, "error", err)
		return nil
	}
	return p
}
