package abs

import (
	"context"
	"errors"
	"testing"

	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

// bindingStubProvider is a minimal provider whose author search can be made to
// fail, which stubABSMetadataProvider cannot do.
type bindingStubProvider struct {
	name    string
	authors []models.Author
	err     error
	full    map[string]*models.Author
}

func (p *bindingStubProvider) Name() string { return p.name }
func (p *bindingStubProvider) SearchAuthors(context.Context, string) ([]models.Author, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]models.Author(nil), p.authors...), nil
}
func (p *bindingStubProvider) SearchBooks(context.Context, string) ([]models.Book, error) {
	return nil, nil
}
func (p *bindingStubProvider) GetAuthor(_ context.Context, foreignID string) (*models.Author, error) {
	return p.full[foreignID], nil
}
func (p *bindingStubProvider) GetBook(context.Context, string) (*models.Book, error) {
	return nil, nil
}
func (p *bindingStubProvider) GetEditions(context.Context, string) ([]models.Edition, error) {
	return nil, nil
}
func (p *bindingStubProvider) GetBookByISBN(context.Context, string) (*models.Book, error) {
	return nil, nil
}

const rateLimit429 = "HTTP 429: API rate limit exceeded for tier 'Free'. Try again in 1 seconds."

// TestABSImportRefusesToBindOnPrimaryFailure is the exact sequence reported in
// #2271: primary_provider = hardcover, a valid token, Hardcover 429s during
// one import, OpenLibrary answers, and the author is bound to OpenLibrary
// forever because providerForForeignID reads the provider back off that id.
func TestABSImportRefusesToBindOnPrimaryFailure(t *testing.T) {
	hc := &bindingStubProvider{name: "hardcover", err: errors.New(rateLimit429)}
	ol := &bindingStubProvider{
		name:    "openlibrary",
		authors: []models.Author{{Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
		full:    map[string]*models.Author{"OL7468980A": {Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
	}
	imp := (&Importer{}).WithMetadata(metadata.NewAggregator(hc, ol))

	author, ambiguous, err := imp.lookupUpstreamAuthor(context.Background(), "Adrian Tchaikovsky")
	if author != nil {
		t.Errorf("no author may be returned for binding while the primary is throttled, got %+v", author)
	}
	if ambiguous {
		t.Error("this is not an ambiguous match, it is an unavailable provider")
	}
	var unavailable *PrimaryProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("want PrimaryProviderUnavailableError so enrichAuthor can report it, got %v", err)
	}
	if unavailable.Failed != "hardcover" {
		t.Errorf("the failing provider should be named for the operator, got %q", unavailable.Failed)
	}
	if !errors.Is(err, unavailable.Err) {
		t.Error("the upstream error should stay in the chain for the log")
	}
}

// TestABSImportStillBindsWhenPrimaryMerelyMisses guards the other half. #2237
// is the case where Hardcover answers and simply does not have the record;
// there the OpenLibrary link is the right answer and refusing it would stop
// imports working on any author Hardcover has never heard of.
func TestABSImportStillBindsWhenPrimaryMerelyMisses(t *testing.T) {
	hc := &bindingStubProvider{name: "hardcover"} // answers, empty
	ol := &bindingStubProvider{
		name:    "openlibrary",
		authors: []models.Author{{Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
		full:    map[string]*models.Author{"OL7468980A": {Name: "Adrian Tchaikovsky", ForeignID: "OL7468980A"}},
	}
	imp := (&Importer{}).WithMetadata(metadata.NewAggregator(hc, ol))

	author, _, err := imp.lookupUpstreamAuthor(context.Background(), "Adrian Tchaikovsky")
	if err != nil {
		t.Fatalf("a primary that answers with nothing is not a failure: %v", err)
	}
	if author == nil || author.ForeignID != "OL7468980A" {
		t.Fatalf("expected the fallback match to be usable, got %+v", author)
	}
}

// TestABSImportBindsPrimaryRecordEvenWhenAnotherProviderFails: only the
// PRIMARY failing matters. An enricher dropping out cannot cause a downgrade,
// because the record being bound is the primary's own.
func TestABSImportBindsPrimaryRecordEvenWhenAnotherProviderFails(t *testing.T) {
	hc := &bindingStubProvider{
		name:    "hardcover",
		authors: []models.Author{{Name: "Adrian Tchaikovsky", ForeignID: "hc:adrian-tchaikovsky"}},
		full:    map[string]*models.Author{"hc:adrian-tchaikovsky": {Name: "Adrian Tchaikovsky", ForeignID: "hc:adrian-tchaikovsky"}},
	}
	ol := &bindingStubProvider{name: "openlibrary", err: errors.New("HTTP 503")}
	imp := (&Importer{}).WithMetadata(metadata.NewAggregator(hc, ol))

	author, _, err := imp.lookupUpstreamAuthor(context.Background(), "Adrian Tchaikovsky")
	if err != nil {
		t.Fatalf("an enricher failing must not block a primary match: %v", err)
	}
	if author == nil || author.ForeignID != "hc:adrian-tchaikovsky" {
		t.Fatalf("expected the primary's record, got %+v", author)
	}
}
