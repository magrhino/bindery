package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

const providerRateLimit429 = "HTTP 429: API rate limit exceeded for tier 'Free'. Try again in 1 seconds."

// TestRelinkUpstreamRefusesFallbackWhilePrimaryUnavailable: the relink
// endpoint resolves a name against every provider and writes the winner's id
// into the author row, which is what decides the provider every later
// catalogue sync uses. With the primary throttled, the fallback wins by
// walkover and the write is permanent (#2271), so the endpoint declines and
// says to retry instead.
func TestRelinkUpstreamRefusesFallbackWhilePrimaryUnavailable(t *testing.T) {
	primary := &searchableAuthorProvider{
		stubMetaProvider: stubMetaProvider{name: "hardcover"},
		searchAuthorsErr: errors.New(providerRateLimit429),
	}
	fallback := &searchableAuthorProvider{
		stubMetaProvider:     stubMetaProvider{name: "openlibrary"},
		searchAuthorsByQuery: map[string][]models.Author{"Adrian Tchaikovsky": {{Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}}},
		authors:              map[string]*models.Author{"OL7468980A": {Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
	}
	fixture := newRelinkUpstreamFixture(t, primary, fallback)
	author := fixture.createAuthor(t, &models.Author{Name: "Adrian Tchaikovsky", ForeignID: "abs:tchaikovsky"})

	rec := fixture.relink(t, author.ID)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (retry later), body %s", rec.Code, rec.Body.String())
	}

	stored, err := fixture.authors.GetByID(fixture.ctx, author.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ForeignID != "abs:tchaikovsky" {
		t.Errorf("the author must keep its existing identity rather than be bound to the fallback, got %q", stored.ForeignID)
	}
}

// TestRelinkUpstreamStillLinksWhenPrimaryMerelyMisses is the counterweight: a
// primary that answers and has no such record is #2237, where the fallback
// link is the correct outcome. Refusing here would break relinking for every
// author the primary has never heard of.
func TestRelinkUpstreamStillLinksWhenPrimaryMerelyMisses(t *testing.T) {
	primary := &searchableAuthorProvider{stubMetaProvider: stubMetaProvider{name: "hardcover"}}
	fallback := &searchableAuthorProvider{
		stubMetaProvider:     stubMetaProvider{name: "openlibrary"},
		searchAuthorsByQuery: map[string][]models.Author{"Adrian Tchaikovsky": {{Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}}},
		authors:              map[string]*models.Author{"OL7468980A": {Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
	}
	fixture := newRelinkUpstreamFixture(t, primary, fallback)
	author := fixture.createAuthor(t, &models.Author{Name: "Adrian Tchaikovsky", ForeignID: "abs:tchaikovsky"})

	rec := fixture.relink(t, author.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", rec.Code, rec.Body.String())
	}
	stored, err := fixture.authors.GetByID(fixture.ctx, author.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ForeignID != "OL7468980A" {
		t.Errorf("foreignID = %q, want the fallback link to have been written", stored.ForeignID)
	}
}
