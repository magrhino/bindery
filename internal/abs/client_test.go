package abs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAuthorizeInjectsBearerHeader(t *testing.T) {
	t.Parallel()

	sawAuth := ""
	sawAgent := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawAgent = r.Header.Get("User-Agent")
		if r.URL.Path != "/api/authorize" {
			t.Fatalf("path = %s, want /api/authorize", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"root","username":"root","type":"root","librariesAccessible":[],"permissions":{"accessAllLibraries":true}},"userDefaultLibraryId":"lib_main","serverSettings":{"version":"2.33.1"},"Source":"docker"}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Authorize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer secret-key" {
		t.Fatalf("Authorization header = %q", sawAuth)
	}
	if sawAgent == "" {
		t.Fatal("User-Agent header should be set")
	}
	if resp.ServerSettings.Version != "2.33.1" {
		t.Fatalf("version = %q", resp.ServerSettings.Version)
	}
}

func TestClientAuthorizeReturnsAPIErrorOnUnauthorized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "bad-key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Authorize(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "invalid api key" {
		t.Fatalf("message = %q", apiErr.Message)
	}
}

func TestClientListLibraryItems_UsesPagingAndMinifiedQuery(t *testing.T) {
	t.Parallel()

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/libraries/lib-books/items" {
			t.Fatalf("path = %s, want /api/libraries/lib-books/items", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"total":0,"limit":50,"page":1,"mediaType":"book","minified":true}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListLibraryItems(context.Background(), "lib-books", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=50&minified=1&page=1" && gotQuery != "limit=50&page=1&minified=1" && gotQuery != "minified=1&limit=50&page=1" {
		t.Fatalf("query = %q", gotQuery)
	}
	if page.Page != 1 || page.Limit != 50 {
		t.Fatalf("page = %+v", page)
	}
}

func TestClientGetLibraryItem_UsesExpandedAuthorsQuery(t *testing.T) {
	t.Parallel()

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/items/li_test" {
			t.Fatalf("path = %s, want /api/items/li_test", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"li_test","libraryId":"lib-books","mediaType":"book","media":{"metadata":{"title":"Test"}}}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	item, err := client.GetLibraryItem(context.Background(), "li_test")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "expanded=1&include=authors" && gotQuery != "include=authors&expanded=1" {
		t.Fatalf("query = %q", gotQuery)
	}
	if item.ID != "li_test" {
		t.Fatalf("item = %+v", item)
	}
}
