package hardcover

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/vavallee/bindery/internal/metadata"
)

// hardcoverQueryCalls names every exported Client method that issues a GraphQL
// query, so the guard tests below stay in step with the API surface.
func hardcoverQueryCalls() []struct {
	name string
	call func(context.Context, *Client) error
} {
	return []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{"SearchAuthors", func(ctx context.Context, c *Client) error {
			authors, err := c.SearchAuthors(ctx, "Frank Herbert")
			if authors != nil {
				return fmt.Errorf("authors = %+v, want nil", authors)
			}
			return err
		}},
		{"SearchBooks", func(ctx context.Context, c *Client) error {
			books, err := c.SearchBooks(ctx, "Dune")
			if books != nil {
				return fmt.Errorf("books = %+v, want nil", books)
			}
			return err
		}},
		{"GetAuthorWorksByName", func(ctx context.Context, c *Client) error {
			books, err := c.GetAuthorWorksByName(ctx, "Frank Herbert")
			if books != nil {
				return fmt.Errorf("books = %+v, want nil", books)
			}
			return err
		}},
		{"GetAuthor", func(ctx context.Context, c *Client) error {
			author, err := c.GetAuthor(ctx, "hc:frank-herbert")
			if author != nil {
				return fmt.Errorf("author = %+v, want nil", author)
			}
			return err
		}},
		{"GetBook", func(ctx context.Context, c *Client) error {
			book, err := c.GetBook(ctx, "hc:dune")
			if book != nil {
				return fmt.Errorf("book = %+v, want nil", book)
			}
			return err
		}},
		{"GetEditions", func(ctx context.Context, c *Client) error {
			editions, err := c.GetEditions(ctx, "hc:dune")
			if editions != nil {
				return fmt.Errorf("editions = %+v, want nil", editions)
			}
			return err
		}},
		{"GetBookByISBN", func(ctx context.Context, c *Client) error {
			book, err := c.GetBookByISBN(ctx, "9780441013593")
			if book != nil {
				return fmt.Errorf("book = %+v, want nil", book)
			}
			return err
		}},
		{"SearchSeries", func(ctx context.Context, c *Client) error {
			results, err := c.SearchSeries(ctx, "Dune", 5)
			if results != nil {
				return fmt.Errorf("results = %+v, want nil", results)
			}
			return err
		}},
		{"GetSeriesCatalog", func(ctx context.Context, c *Client) error {
			catalog, err := c.GetSeriesCatalog(ctx, "hc-series:42")
			if catalog != nil {
				return fmt.Errorf("catalog = %+v, want nil", catalog)
			}
			return err
		}},
		{"GetUserLists", func(ctx context.Context, c *Client) error {
			lists, err := c.GetUserLists(ctx)
			if lists != nil {
				return fmt.Errorf("lists = %+v, want nil", lists)
			}
			return err
		}},
		{"GetUsername", func(ctx context.Context, c *Client) error {
			username, err := c.GetUsername(ctx)
			if username != "" {
				return fmt.Errorf("username = %q, want empty", username)
			}
			return err
		}},
		{"GetListBooks", func(ctx context.Context, c *Client) error {
			books, err := c.GetListBooks(ctx, 7)
			if books != nil {
				return fmt.Errorf("books = %+v, want nil", books)
			}
			return err
		}},
	}
}

// countingServerTransport points the client at a local test server, since the
// GraphQL endpoint is a package constant, and counts the requests that reach it.
type countingServerTransport struct {
	base  *url.URL
	inner http.RoundTripper
	calls atomic.Int64
}

func (t *countingServerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	clone := r.Clone(r.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	return t.inner.RoundTrip(clone)
}

func newCountingServer(t *testing.T) *countingServerTransport {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return &countingServerTransport{base: base, inner: srv.Client().Transport}
}

// TestUnconfiguredClient_MakesNoRequests is the #2075 regression test. A bulk
// author import on a config with no Hardcover token fired hundreds of requests
// that could only come back 401 and then 429, because the token guard existed
// on GetAuthorWorksByName alone.
func TestUnconfiguredClient_MakesNoRequests(t *testing.T) {
	for _, tc := range hardcoverQueryCalls() {
		t.Run(tc.name, func(t *testing.T) {
			transport := newCountingServer(t)
			c := &Client{http: &http.Client{Transport: transport}}

			err := tc.call(context.Background(), c)
			if !errors.Is(err, metadata.ErrProviderNotConfigured) {
				t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
			}
			if got := transport.calls.Load(); got != 0 {
				t.Fatalf("HTTP requests = %d, want 0", got)
			}
		})
	}
}

// TestConfiguredClient_StillReachesServer is the other half of the guard: with
// a token every one of those calls is a real request again.
func TestConfiguredClient_StillReachesServer(t *testing.T) {
	for _, tc := range hardcoverQueryCalls() {
		t.Run(tc.name, func(t *testing.T) {
			transport := newCountingServer(t)
			c := (&Client{http: &http.Client{Transport: transport}}).WithToken("hc-token")

			_ = tc.call(context.Background(), c)
			if got := transport.calls.Load(); got == 0 {
				t.Fatal("HTTP requests = 0, want at least 1 once a token is configured")
			}
		})
	}
}

// TestUnconfiguredClient_TokenSourceCanArriveLater covers why the enricher
// stays registered at startup: the token is resolved per request, so setting
// one in the UI takes effect without a restart.
func TestUnconfiguredClient_TokenSourceCanArriveLater(t *testing.T) {
	transport := newCountingServer(t)
	token := ""
	c := (&Client{http: &http.Client{Transport: transport}}).WithTokenSource(func(context.Context) string {
		return token
	})

	if _, err := c.SearchBooks(context.Background(), "Dune"); !errors.Is(err, metadata.ErrProviderNotConfigured) {
		t.Fatalf("SearchBooks without token: error = %v, want ErrProviderNotConfigured", err)
	}
	if got := transport.calls.Load(); got != 0 {
		t.Fatalf("HTTP requests before a token = %d, want 0", got)
	}

	token = "hc-token"
	if _, err := c.SearchBooks(context.Background(), "Dune"); err != nil {
		t.Fatalf("SearchBooks with token: %v", err)
	}
	if got := transport.calls.Load(); got != 1 {
		t.Fatalf("HTTP requests after a token = %d, want 1", got)
	}
}
