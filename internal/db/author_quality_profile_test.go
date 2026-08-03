package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// ResolveAuthorQualityProfile is the single point both search paths use to
// decide which profile governs a book (#1693). Every failure mode must return
// nil — "no format filter" — because a profile we cannot read must not silently
// block every grab for that author.
func TestResolveAuthorQualityProfile(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewQualityProfileRepo(database)
	profile := &models.QualityProfile{
		Name:  "EPUB only",
		Items: []models.QualityItem{{Quality: "epub", Allowed: true}},
	}
	if err := repo.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}

	t.Run("resolves the author's profile", func(t *testing.T) {
		got := ResolveAuthorQualityProfile(ctx, repo, &models.Author{ID: 1, QualityProfileID: &profile.ID})
		if got == nil {
			t.Fatal("expected the profile to resolve")
		}
		if got.Name != "EPUB only" {
			t.Errorf("resolved %q, want %q", got.Name, "EPUB only")
		}
		if len(got.Items) != 1 || !got.Items[0].Allowed {
			t.Errorf("items should round-trip, got %+v", got.Items)
		}
	})

	t.Run("nil when the author has no profile", func(t *testing.T) {
		if got := ResolveAuthorQualityProfile(ctx, repo, &models.Author{ID: 1}); got != nil {
			t.Errorf("expected nil for an author with no profile, got %+v", got)
		}
	})

	t.Run("nil when the repo is not wired", func(t *testing.T) {
		if got := ResolveAuthorQualityProfile(ctx, nil, &models.Author{ID: 1, QualityProfileID: &profile.ID}); got != nil {
			t.Errorf("expected nil with no repo, got %+v", got)
		}
	})

	t.Run("nil for a nil author", func(t *testing.T) {
		if got := ResolveAuthorQualityProfile(ctx, repo, nil); got != nil {
			t.Errorf("expected nil for a nil author, got %+v", got)
		}
	})

	// A dangling quality_profile_id must not become an accidental grab block:
	// the lookup fails, and the caller has to fall back to no filtering.
	t.Run("nil when the referenced profile is gone", func(t *testing.T) {
		missing := int64(999999)
		if got := ResolveAuthorQualityProfile(ctx, repo, &models.Author{ID: 1, QualityProfileID: &missing}); got != nil {
			t.Errorf("expected nil for a dangling profile id, got %+v", got)
		}
	})
}
