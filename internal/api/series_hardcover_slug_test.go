package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

// #1708: hardcover.app routes series on their slug, so the slug has to reach
// the client for the candidate list and the linked-series display to link out.

func TestSeriesHardcoverSearchServesSlug(t *testing.T) {
	h, _, _, _ := seriesFixtureWithProvider(t, &stubSeriesProvider{
		searchResults: []metadata.SeriesSearchResult{
			{ForeignID: "hc-series:42", ProviderID: "42", Slug: "the-stormlight-archive", Title: "The Stormlight Archive"},
			{ForeignID: "hc-series:43", ProviderID: "43", Title: "The Stormlight Archive (manga)"},
		},
	}, nil)

	rec := httptest.NewRecorder()
	h.SearchHardcover(rec, httptest.NewRequest(http.MethodGet, "/api/v1/series/hardcover/search?term=stormlight", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("results len = %d, want 2", len(got))
	}
	if got[0]["slug"] != "the-stormlight-archive" {
		t.Fatalf("slug = %v, want the-stormlight-archive", got[0]["slug"])
	}
	// A candidate Hardcover returned without a slug must not carry an empty
	// one the client could mistake for a page.
	if _, ok := got[1]["slug"]; ok {
		t.Fatalf("expected no slug key on the slugless candidate, got %v", got[1])
	}
}

func TestSeriesPutHardcoverLinkStoresSlug(t *testing.T) {
	catalog := stormlightCatalog()
	catalog.Slug = "the-stormlight-archive"
	h, seriesRepo, _, _ := seriesFixtureWithProvider(t, &stubSeriesProvider{
		catalogs: map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
	}, nil)
	ctx := context.Background()
	series := &models.Series{ForeignID: "manual:series:1", Title: "Stormlight"}
	if err := seriesRepo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}

	body := `{"foreignId":"hc-series:42","providerId":"42","slug":"the-stormlight-archive","title":"The Stormlight Archive"}`
	rec := httptest.NewRecorder()
	h.PutHardcoverLink(rec, withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/series/1/hardcover-link", bytes.NewBufferString(body)),
		"id", strconv.FormatInt(series.ID, 10)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored, err := seriesRepo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.HardcoverSlug != "the-stormlight-archive" {
		t.Fatalf("stored link = %+v, want the-stormlight-archive slug", stored)
	}
}

// A link stored before migration 080 has no slug and would otherwise stay
// unlinkable until the user re-linked it by hand. The diff already holds the
// catalog, so the slug is recorded there without an extra upstream call.
func TestSeriesHardcoverDiffBackfillsSlug(t *testing.T) {
	catalog := stormlightCatalog()
	catalog.Slug = "the-stormlight-archive"
	h, seriesRepo, _, _ := seriesFixtureWithProvider(t, &stubSeriesProvider{
		catalogs: map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
	}, nil)
	ctx := context.Background()
	series := &models.Series{ForeignID: "manual:series:1", Title: "Stormlight"}
	if err := seriesRepo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.UpsertHardcoverLink(ctx, &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   catalog.ForeignID,
		HardcoverProviderID: catalog.ProviderID,
		HardcoverTitle:      catalog.Title,
		Confidence:          1,
		LinkedBy:            "manual",
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.HardcoverDiff(rec, withURLParam(
		httptest.NewRequest(http.MethodGet, "/api/v1/series/1/hardcover-diff", nil),
		"id", strconv.FormatInt(series.ID, 10)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored, err := seriesRepo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.HardcoverSlug != "the-stormlight-archive" {
		t.Fatalf("stored link = %+v, want the slug backfilled from the catalog", stored)
	}
}

// A catalog without a slug leaves the link alone: there is nothing to record
// and nothing the client could build a URL from.
func TestSeriesHardcoverDiffWithoutSlugLeavesLinkAlone(t *testing.T) {
	catalog := stormlightCatalog()
	h, seriesRepo, _, _ := seriesFixtureWithProvider(t, &stubSeriesProvider{
		catalogs: map[string]*metadata.SeriesCatalog{catalog.ForeignID: catalog},
	}, nil)
	ctx := context.Background()
	series := &models.Series{ForeignID: "manual:series:1", Title: "Stormlight"}
	if err := seriesRepo.Create(ctx, series); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.UpsertHardcoverLink(ctx, &models.SeriesHardcoverLink{
		SeriesID:            series.ID,
		HardcoverSeriesID:   catalog.ForeignID,
		HardcoverProviderID: catalog.ProviderID,
		HardcoverTitle:      catalog.Title,
		Confidence:          1,
		LinkedBy:            "manual",
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.HardcoverDiff(rec, withURLParam(
		httptest.NewRequest(http.MethodGet, "/api/v1/series/1/hardcover-diff", nil),
		"id", strconv.FormatInt(series.ID, 10)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored, err := seriesRepo.GetHardcoverLink(ctx, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.HardcoverSlug != "" {
		t.Fatalf("stored link = %+v, want no slug", stored)
	}
}
