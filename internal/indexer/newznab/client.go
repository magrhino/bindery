package newznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client interacts with a single Newznab-compatible indexer.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New creates a Newznab client for a specific indexer.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Caps fetches the indexer capabilities.
func (c *Client) Caps(ctx context.Context) (*capsResponse, error) {
	u := fmt.Sprintf("%s/api?t=caps&apikey=%s", c.baseURL, url.QueryEscape(c.apiKey))
	var caps capsResponse
	if err := c.getXML(ctx, u, &caps); err != nil {
		return nil, fmt.Errorf("caps: %w", err)
	}
	return &caps, nil
}

// Search performs a general search with optional category filtering.
func (c *Client) Search(ctx context.Context, query string, categories []int) ([]SearchResult, error) {
	cats := intSliceToCSV(categories)
	u := fmt.Sprintf("%s/api?t=search&apikey=%s&q=%s&cat=%s&limit=100",
		c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(query), cats)

	var rss rssResponse
	if err := c.getXML(ctx, u, &rss); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return c.parseResults(rss.Channel.Items), nil
}

// BookSearch tries a series of query variants against a Newznab/Torznab
// indexer and returns the first variant that yields results. Query order:
//
//  1. Structured t=book with title+author (primary title only — subtitles
//     are dropped for the query; filterRelevant still matches on the full).
//  2. t=search "Lastname Title" — most disambiguating freeform tier for
//     short titles (e.g. "Russell The Sparrow" beats "The Sparrow" alone).
//  3. t=search "Author Title" — full author name + title.
//  4. t=search "Title" — last-resort fallback.
func (c *Client) BookSearch(ctx context.Context, title, author string, categories []int) ([]SearchResult, error) {
	queryTitle := primaryTitleForQuery(title)
	surname := authorSurname(author)
	cats := intSliceToCSV(categories)

	// Tier 1: structured t=book
	if author != "" {
		u := fmt.Sprintf("%s/api?t=book&apikey=%s&title=%s&author=%s&cat=%s&limit=100",
			c.baseURL, url.QueryEscape(c.apiKey),
			url.QueryEscape(queryTitle), url.QueryEscape(author), cats)

		var rss rssResponse
		if err := c.getXML(ctx, u, &rss); err == nil && len(rss.Channel.Items) > 0 && rss.Channel.Response.Total < 1000 {
			return c.parseResults(rss.Channel.Items), nil
		}
	}

	// Tier 2: surname + title (short, disambiguating)
	if surname != "" && !strings.EqualFold(surname, author) {
		results, err := c.Search(ctx, surname+" "+queryTitle, categories)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	// Tier 3: full author + title
	if author != "" {
		results, err := c.Search(ctx, author+" "+queryTitle, categories)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	// Tier 4: title only
	return c.Search(ctx, queryTitle, categories)
}

// primaryTitleForQuery returns the portion of a book title before a colon,
// so "Dune: Messiah" queries as "Dune". Indexers rarely have the subtitle
// in the release name and including it can cause all-keyword-match failures.
func primaryTitleForQuery(title string) string {
	if i := strings.Index(title, ":"); i > 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

func authorSurname(author string) string {
	fields := strings.Fields(author)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// Test verifies the indexer is reachable and the API key is valid.
func (c *Client) Test(ctx context.Context) error {
	_, err := c.Caps(ctx)
	return err
}

func (c *Client) parseResults(items []rssItem) []SearchResult {
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		r := SearchResult{
			GUID:    item.GUID.Value,
			Title:   item.Title,
			Size:    item.Enclosure.Length,
			NZBURL:  item.Enclosure.URL,
			PubDate: item.PubDate,
		}

		// Parse newznab attributes
		for _, attr := range item.Attrs {
			switch attr.Name {
			case "size":
				if s, err := strconv.ParseInt(attr.Value, 10, 64); err == nil {
					r.Size = s
				}
			case "grabs":
				if g, err := strconv.Atoi(attr.Value); err == nil {
					r.Grabs = g
				}
			case "category":
				r.Category = attr.Value
			case "author":
				r.Author = attr.Value
			case "title":
				r.BookTitle = attr.Value
			}
		}

		if r.NZBURL == "" {
			r.NZBURL = item.Link
		}

		results = append(results, r)
	}
	return results
}

func (c *Client) getXML(ctx context.Context, rawURL string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Bindery/0.1")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return xml.NewDecoder(resp.Body).Decode(target)
}

func intSliceToCSV(ints []int) string {
	if len(ints) == 0 {
		return "7000,7020"
	}
	parts := make([]string, len(ints))
	for i, v := range ints {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
