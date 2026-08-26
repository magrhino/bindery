package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// hardcover.app routes series on their slug, not on the numeric provider id, so
// the slug is the only thing the linked-series "View on Hardcover" link can be
// built from (#1708, migration 080).
func TestSeriesHardcoverLinkSlugRoundTrips(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "Stormlight Archive"}
	if err := repo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}

	link := &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   "hc-series:1",
		HardcoverProviderID: "1",
		HardcoverSlug:       "the-stormlight-archive",
		HardcoverTitle:      "The Stormlight Archive",
	}
	if err := repo.UpsertHardcoverLink(ctx, link); err != nil {
		t.Fatalf("upsert link: %v", err)
	}

	got, err := repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got == nil || got.HardcoverSlug != "the-stormlight-archive" {
		t.Fatalf("slug = %+v, want the-stormlight-archive", got)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list series: %v", err)
	}
	if list[0].HardcoverLink == nil || list[0].HardcoverLink.HardcoverSlug != "the-stormlight-archive" {
		t.Fatalf("expected slug on the hydrated list link, got %+v", list[0].HardcoverLink)
	}
}

// A link stored before migration 080 has no slug, and every read path has to
// cope with that rather than assume one is present.
func TestSeriesHardcoverLinkWithoutSlug(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)
	series := &models.Series{ForeignID: "ol-series:mistborn", Title: "Mistborn"}
	if err := repo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertHardcoverLink(ctx, &models.SeriesHardcoverLink{
		SeriesID:          series.ID,
		HardcoverSeriesID: "hc-series:2",
		HardcoverTitle:    "Mistborn",
	}); err != nil {
		t.Fatalf("upsert link: %v", err)
	}

	got, err := repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got == nil || got.HardcoverSlug != "" {
		t.Fatalf("slug = %+v, want empty", got)
	}
}

// An upsert from a path that could not resolve a slug must not erase one that
// is already recorded: the link would silently lose its way back to Hardcover.
func TestSeriesHardcoverLinkUpsertKeepsKnownSlug(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	repo := NewSeriesRepo(database)
	series := &models.Series{ForeignID: "ol-series:stormlight", Title: "Stormlight Archive"}
	if err := repo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertHardcoverLink(ctx, &models.SeriesHardcoverLink{
		SeriesID:          series.ID,
		HardcoverSeriesID: "hc-series:1",
		HardcoverSlug:     "the-stormlight-archive",
		HardcoverTitle:    "The Stormlight Archive",
	}); err != nil {
		t.Fatalf("upsert link: %v", err)
	}

	if err := repo.UpsertHardcoverLink(ctx, &models.SeriesHardcoverLink{
		SeriesID:          series.ID,
		HardcoverSeriesID: "hc-series:1",
		HardcoverTitle:    "The Stormlight Archive (revised)",
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got.HardcoverSlug != "the-stormlight-archive" {
		t.Fatalf("slug = %q, want the slug to survive a slugless upsert", got.HardcoverSlug)
	}
	if got.HardcoverTitle != "The Stormlight Archive (revised)" {
		t.Fatalf("title = %q, want the upsert to still apply", got.HardcoverTitle)
	}
}
