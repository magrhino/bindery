// Package hardcover provides a read-only GraphQL client for hardcover.app,
// used as a metadata enricher for community ratings and series data.
package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

const (
	graphqlURL = "https://api.hardcover.app/v1/graphql"
	idPrefix   = "hc:"
)

// Client implements metadata.Provider for Hardcover.app using its public GraphQL API.
type Client struct {
	http *http.Client
}

// New creates a new Hardcover client.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Name() string { return "hardcover" }

func (c *Client) SearchAuthors(ctx context.Context, query string) ([]models.Author, error) {
	gql := `query SearchAuthors($query: String!) {
		authors(where: {name: {_ilike: $query}}, limit: 20) {
			id
			name
			slug
			bio
			image { url }
		}
	}`
	var resp struct {
		Data struct {
			Authors []hcAuthor `json:"authors"`
		} `json:"data"`
	}
	if err := c.query(ctx, gql, map[string]any{"query": "%" + query + "%"}, &resp); err != nil {
		return nil, fmt.Errorf("hardcover search authors: %w", err)
	}
	authors := make([]models.Author, 0, len(resp.Data.Authors))
	for _, a := range resp.Data.Authors {
		authors = append(authors, c.toAuthor(a))
	}
	return authors, nil
}

func (c *Client) SearchBooks(ctx context.Context, query string) ([]models.Book, error) {
	gql := `query SearchBooks($query: String!) {
		books(where: {title: {_ilike: $query}}, limit: 20) {
			id
			title
			slug
			description
			image { url }
			release_year
			ratings_count
			rating
			contributions {
				author { id name slug }
			}
		}
	}`
	var resp struct {
		Data struct {
			Books []hcBook `json:"books"`
		} `json:"data"`
	}
	if err := c.query(ctx, gql, map[string]any{"query": "%" + query + "%"}, &resp); err != nil {
		return nil, fmt.Errorf("hardcover search books: %w", err)
	}
	books := make([]models.Book, 0, len(resp.Data.Books))
	for _, b := range resp.Data.Books {
		books = append(books, c.toBook(b))
	}
	return books, nil
}

func (c *Client) GetAuthor(ctx context.Context, foreignID string) (*models.Author, error) {
	slug := strings.TrimPrefix(foreignID, idPrefix)
	gql := `query GetAuthor($slug: String!) {
		authors(where: {slug: {_eq: $slug}}, limit: 1) {
			id
			name
			slug
			bio
			image { url }
		}
	}`
	var resp struct {
		Data struct {
			Authors []hcAuthor `json:"authors"`
		} `json:"data"`
	}
	if err := c.query(ctx, gql, map[string]any{"slug": slug}, &resp); err != nil {
		return nil, fmt.Errorf("hardcover get author: %w", err)
	}
	if len(resp.Data.Authors) == 0 {
		return nil, nil
	}
	a := c.toAuthor(resp.Data.Authors[0])
	return &a, nil
}

func (c *Client) GetBook(ctx context.Context, foreignID string) (*models.Book, error) {
	slug := strings.TrimPrefix(foreignID, idPrefix)
	gql := `query GetBook($slug: String!) {
		books(where: {slug: {_eq: $slug}}, limit: 1) {
			id
			title
			slug
			description
			image { url }
			release_year
			ratings_count
			rating
			contributions {
				author { id name slug }
			}
		}
	}`
	var resp struct {
		Data struct {
			Books []hcBook `json:"books"`
		} `json:"data"`
	}
	if err := c.query(ctx, gql, map[string]any{"slug": slug}, &resp); err != nil {
		return nil, fmt.Errorf("hardcover get book: %w", err)
	}
	if len(resp.Data.Books) == 0 {
		return nil, nil
	}
	b := c.toBook(resp.Data.Books[0])
	return &b, nil
}

// GetEditions is not supported by Hardcover.
func (c *Client) GetEditions(_ context.Context, _ string) ([]models.Edition, error) {
	return nil, nil
}

func (c *Client) GetBookByISBN(ctx context.Context, isbn string) (*models.Book, error) {
	gql := `query GetBookByISBN($isbn: String!) {
		editions(where: {_or: [{isbn_10: {_eq: $isbn}}, {isbn_13: {_eq: $isbn}}]}, limit: 1) {
			book {
				id
				title
				slug
				description
				image { url }
				release_year
				ratings_count
				rating
				contributions {
					author { id name slug }
				}
			}
		}
	}`
	var resp struct {
		Data struct {
			Editions []struct {
				Book hcBook `json:"book"`
			} `json:"editions"`
		} `json:"data"`
	}
	if err := c.query(ctx, gql, map[string]any{"isbn": isbn}, &resp); err != nil {
		return nil, fmt.Errorf("hardcover get book by isbn: %w", err)
	}
	if len(resp.Data.Editions) == 0 {
		return nil, nil
	}
	b := c.toBook(resp.Data.Editions[0].Book)
	return &b, nil
}

// --- GraphQL transport ---

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

func (c *Client) query(ctx context.Context, q string, vars map[string]any, out interface{}) error {
	body, err := json.Marshal(gqlRequest{Query: q, Variables: vars})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Bindery/0.1 (https://github.com/vavallee/bindery)")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// --- Internal types for JSON mapping ---

type hcImage struct {
	URL string `json:"url"`
}

type hcAuthor struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Slug  string   `json:"slug"`
	Bio   string   `json:"bio"`
	Image *hcImage `json:"image"`
}

type hcContribution struct {
	Author hcAuthor `json:"author"`
}

type hcBook struct {
	ID            int              `json:"id"`
	Title         string           `json:"title"`
	Slug          string           `json:"slug"`
	Description   string           `json:"description"`
	Image         *hcImage         `json:"image"`
	ReleaseYear   *int             `json:"release_year"`
	RatingsCount  int              `json:"ratings_count"`
	Rating        float64          `json:"rating"`
	Contributions []hcContribution `json:"contributions"`
}

// --- Converters ---

func (c *Client) toAuthor(a hcAuthor) models.Author {
	slug := a.Slug
	if slug == "" {
		slug = fmt.Sprintf("%d", a.ID)
	}
	au := models.Author{
		ForeignID:        idPrefix + slug,
		Name:             a.Name,
		SortName:         sortName(a.Name),
		Description:      a.Bio,
		MetadataProvider: "hardcover",
	}
	if a.Image != nil {
		au.ImageURL = a.Image.URL
	}
	return au
}

func (c *Client) toBook(b hcBook) models.Book {
	slug := b.Slug
	if slug == "" {
		slug = fmt.Sprintf("%d", b.ID)
	}
	bk := models.Book{
		ForeignID:        idPrefix + slug,
		Title:            b.Title,
		SortTitle:        b.Title,
		Description:      b.Description,
		AverageRating:    b.Rating,
		RatingsCount:     b.RatingsCount,
		MetadataProvider: "hardcover",
		Monitored:        true,
		Status:           models.BookStatusWanted,
		Genres:           []string{},
	}
	if b.Image != nil {
		bk.ImageURL = b.Image.URL
	}
	if b.ReleaseYear != nil && *b.ReleaseYear > 0 {
		t := time.Date(*b.ReleaseYear, 1, 1, 0, 0, 0, 0, time.UTC)
		bk.ReleaseDate = &t
	}
	if len(b.Contributions) > 0 {
		a := c.toAuthor(b.Contributions[0].Author)
		bk.Author = &a
	}
	return bk
}

func sortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	last := parts[len(parts)-1]
	rest := strings.Join(parts[:len(parts)-1], " ")
	return last + ", " + rest
}
