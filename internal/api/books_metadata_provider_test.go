package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// #1707: the book detail page could not say which provider a record came from,
// which made "is this the right book, should I re-bind" unanswerable with
// OpenLibrary, Hardcover, Google Books and DNB all in play. metadata_provider
// already shipped in the DTO; the identity map from #1705 did not.
func TestBookGet_ServesProviderAndIdentityMap(t *testing.T) {
	h, books, _, author, ctx := bookFixture(t)
	book := &models.Book{
		ForeignID:        "OL27448W",
		AuthorID:         author.ID,
		Title:            "The Way of Kings",
		SortTitle:        "way of kings",
		MetadataProvider: "openlibrary",
	}
	if err := books.Create(ctx, book); err != nil {
		t.Fatal(err)
	}
	if err := books.UpsertBookIdentifier(ctx, book.ID, "hc:the-way-of-kings"); err != nil {
		t.Fatal(err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/book/1", nil), "id", strconv.FormatInt(book.ID, 10))
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		ForeignBookID    string                  `json:"foreignBookId"`
		MetadataProvider string                  `json:"metadataProvider"`
		Identifiers      []models.BookIdentifier `json:"identifiers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.MetadataProvider != "openlibrary" {
		t.Fatalf("metadataProvider = %q, want openlibrary", got.MetadataProvider)
	}
	if got.ForeignBookID != "OL27448W" {
		t.Fatalf("foreignBookId = %q, want OL27448W", got.ForeignBookID)
	}
	providers := map[string]string{}
	for _, identifier := range got.Identifiers {
		providers[identifier.Provider] = identifier.ForeignID
	}
	if providers["openlibrary"] != "OL27448W" {
		t.Fatalf("identifiers = %+v, want the primary id recorded", got.Identifiers)
	}
	if providers["hardcover"] != "hc:the-way-of-kings" {
		t.Fatalf("identifiers = %+v, want the Hardcover id recorded", got.Identifiers)
	}
}
