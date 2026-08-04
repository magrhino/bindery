package metadata

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestAggregator_SearchAuthorCandidates_IncludesEnrichersAndSkipsUnconfigured(t *testing.T) {
	primary := &mockProvider{
		name: "openlibrary",
		searchAuthors: []models.Author{
			{ForeignID: "OL13200512A", Name: "Emilia Jae", MetadataProvider: "openlibrary"},
		},
	}
	hardcover := &mockProvider{
		name: "hardcover",
		searchAuthors: []models.Author{
			{ForeignID: "hc:emilia-jae", Name: "Emilia Jae", Description: "Fantasy author."},
		},
	}
	unconfigured := &mockProvider{name: "googlebooks", searchAuthErr: ErrProviderNotConfigured}
	agg := newTestAggregator(primary, hardcover, unconfigured)

	got, err := agg.SearchAuthorCandidates(context.Background(), "emilia jae")
	if err != nil {
		t.Fatalf("SearchAuthorCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(got), got)
	}
	if got[0].ForeignID != "OL13200512A" || got[1].ForeignID != "hc:emilia-jae" {
		t.Fatalf("candidate order/ids = %+v", got)
	}
	if got[1].MetadataProvider != "hardcover" {
		t.Fatalf("hardcover provider default = %q, want hardcover", got[1].MetadataProvider)
	}
	if len(primary.searchAuthorQueries) != 1 || primary.searchAuthorQueries[0] != "emilia jae" {
		t.Fatalf("primary queries = %+v", primary.searchAuthorQueries)
	}
	if len(hardcover.searchAuthorQueries) != 1 || hardcover.searchAuthorQueries[0] != "emilia jae" {
		t.Fatalf("hardcover queries = %+v", hardcover.searchAuthorQueries)
	}
}

func TestAggregator_SearchAuthorCandidates_DeduplicatesForeignIDs(t *testing.T) {
	primary := &mockProvider{
		name:          "openlibrary",
		searchAuthors: []models.Author{{ForeignID: "OL1A", Name: "Author"}},
	}
	enricher := &mockProvider{
		name:          "openlibrary",
		searchAuthors: []models.Author{{ForeignID: "OL1A", Name: "Author"}},
	}
	agg := newTestAggregator(primary, enricher)

	got, err := agg.SearchAuthorCandidates(context.Background(), "author")
	if err != nil {
		t.Fatalf("SearchAuthorCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want 1: %+v", len(got), got)
	}
}

// TestAggregator_SearchAuthorCandidates_RanksRealRecordAboveAnthologyComposites
// reproduces the "Link metadata" ordering reported in #1754.
//
// OpenLibrary emits a composite author record per anthology — one row named
// after every contributor, carrying a single work. Those arrived ahead of the
// author's real record because candidates were returned in raw provider order
// with no ranking at all, so the modal led with one-book records while the
// record holding the whole catalogue sat further down.
//
// The exact-name match with the largest catalogue must rank first.
func TestAggregator_SearchAuthorCandidates_RanksRealRecordAboveAnthologyComposites(t *testing.T) {
	stats := func(n int) *models.AuthorStats { return &models.AuthorStats{BookCount: n} }
	primary := &mockProvider{
		name: "openlibrary",
		searchAuthors: []models.Author{
			// Provider order deliberately puts the junk first.
			{ForeignID: "OL7777A", Name: "David Annandale", Statistics: stats(3)},
			{ForeignID: "OL8888A", Name: "Dan Abnett, David Annandale, John French, Guy Haley", Statistics: stats(1)},
			{ForeignID: "OL9999A", Name: "Annandale, David, Haley, Guy, Sanders, Rob", Statistics: stats(1)},
			{ForeignID: "OL1529996A", Name: "David Annandale", Statistics: stats(142)},
		},
	}
	agg := newTestAggregator(primary)

	got, err := agg.SearchAuthorCandidates(context.Background(), "David Annandale")
	if err != nil {
		t.Fatalf("SearchAuthorCandidates: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("candidates = %d, want 4 (records must be ranked, never merged): %+v", len(got), got)
	}
	if got[0].ForeignID != "OL1529996A" {
		names := make([]string, len(got))
		for i, a := range got {
			names[i] = a.ForeignID
		}
		t.Fatalf("top candidate = %q, want OL1529996A (exact name, largest catalogue); order was %v", got[0].ForeignID, names)
	}
	// The two composite records name several people, so they must not
	// outrank either plain "David Annandale" record.
	for i, a := range got {
		if (a.ForeignID == "OL8888A" || a.ForeignID == "OL9999A") && i < 2 {
			t.Errorf("anthology composite %q ranked at position %d, above a real name match", a.ForeignID, i)
		}
	}
}

// TestAggregator_SearchAuthorCandidates_DoesNotMergeSamePersonRecords guards the
// property that makes this modal work: candidates from different providers for
// the same person must all remain selectable. Ranking them is intended;
// collapsing them (what SearchAuthors does) would defeat the modal's purpose.
func TestAggregator_SearchAuthorCandidates_DoesNotMergeSamePersonRecords(t *testing.T) {
	primary := &mockProvider{
		name:          "openlibrary",
		searchAuthors: []models.Author{{ForeignID: "OL1A", Name: "David Annandale"}},
	}
	other := &mockProvider{
		name:          "dnb",
		searchAuthors: []models.Author{{ForeignID: "dnb:123", Name: "David Annandale"}},
	}
	agg := newTestAggregator(primary, other)

	got, err := agg.SearchAuthorCandidates(context.Background(), "David Annandale")
	if err != nil {
		t.Fatalf("SearchAuthorCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want 2 — same-person records must stay separately selectable: %+v", len(got), got)
	}
}
