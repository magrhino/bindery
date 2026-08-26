package hardcover

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The public Hardcover series page is hardcover.app/series/<slug> and does not
// route on the numeric id Bindery stores, so the slug is the only thing a link
// out can be built from (#1708). Hardcover ships it in both the Typesense
// search document and series_by_pk; these assert we actually read it.

func TestSearchSeries_CarriesSlug(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		return gqlResponse(t, http.StatusOK, map[string]interface{}{
			"search": map[string]interface{}{
				"ids": []interface{}{123},
				"results": map[string]interface{}{
					"found": 2,
					"hits": []map[string]interface{}{
						{
							"document": map[string]interface{}{
								"id":                  123,
								"name":                "Foundation",
								"slug":                "foundation",
								"author_name":         "Isaac Asimov",
								"primary_books_count": 7,
							},
						},
						{
							// A document with no slug still has to map, it just
							// gets no link in the UI.
							"document": map[string]interface{}{
								"id":   124,
								"name": "Foundation (manga)",
							},
						},
					},
				},
			},
		}), nil
	})

	results, err := c.SearchSeries(context.Background(), "Foundation", 5)
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if results[0].Slug != "foundation" {
		t.Fatalf("slug = %q, want foundation", results[0].Slug)
	}
	if results[1].Slug != "" {
		t.Fatalf("slug = %q, want empty for a document without one", results[1].Slug)
	}
}

func TestGetSeriesCatalog_RequestsAndCarriesSlug(t *testing.T) {
	var query string
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		var req gqlRequest
		_ = json.Unmarshal(body, &req)
		query = req.Query
		return gqlResponse(t, http.StatusOK, map[string]interface{}{
			"series_by_pk": map[string]interface{}{
				"id":          123,
				"name":        "Foundation",
				"slug":        "foundation",
				"books_count": 1,
				"book_series": []map[string]interface{}{},
			},
		}), nil
	})

	catalog, err := c.GetSeriesCatalog(context.Background(), "hc-series:123")
	if err != nil {
		t.Fatalf("GetSeriesCatalog: %v", err)
	}
	if catalog == nil {
		t.Fatal("expected catalog")
		return
	}
	if catalog.Slug != "foundation" {
		t.Fatalf("catalog slug = %q, want foundation", catalog.Slug)
	}
	// A field we never select is a field the API never returns, so assert the
	// selection itself rather than only the parse.
	if !strings.Contains(query, "slug") {
		t.Fatalf("series_by_pk query does not select slug: %s", query)
	}
}
