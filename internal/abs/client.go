// Package abs provides Audiobookshelf client, normalization, and import logic.
package abs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout   = 10 * time.Second
	defaultUserAgent = "bindery-abs-phase1/1"
	maxAttempts      = 3
)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("abs api error (%d)", e.StatusCode)
	}
	return e.Message
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("base_url %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base_url %q must use http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base_url %q is missing a host", raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	} else {
		u.Path = ""
	}
	return u.String(), nil
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

func NewClient(baseURL, apiKey string) (*Client, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api_key is required")
	}
	return &Client{
		baseURL: normalized,
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: defaultUserAgent,
	}, nil
}

func (c *Client) Authorize(ctx context.Context) (*AuthorizeResponse, error) {
	var out AuthorizeResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/authorize", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListLibraries(ctx context.Context) ([]Library, error) {
	var out librariesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/libraries", nil, &out); err != nil {
		return nil, err
	}
	return out.Libraries, nil
}

func (c *Client) GetLibrary(ctx context.Context, id string) (*Library, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("library_id is required")
	}
	var out Library
	if err := c.doJSON(ctx, http.MethodGet, "/api/libraries/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListLibraryItems(ctx context.Context, libraryID string, page, limit int) (*LibraryItemsPage, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, errors.New("library_id is required")
	}
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	if page >= 0 {
		values.Set("page", fmt.Sprintf("%d", page))
	}
	values.Set("minified", "1")
	var out LibraryItemsPage
	if err := c.doJSON(ctx, http.MethodGet, "/api/libraries/"+url.PathEscape(libraryID)+"/items?"+values.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLibraryItem(ctx context.Context, id string) (*LibraryItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("item_id is required")
	}
	values := url.Values{}
	values.Set("expanded", "1")
	values.Set("include", "authors")
	var out LibraryItem
	if err := c.doJSON(ctx, http.MethodGet, "/api/items/"+url.PathEscape(id)+"?"+values.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, out any) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts-1 && shouldRetry(err) {
				sleepBackoff(ctx, attempt)
				continue
			}
			return err
		}

		if resp.StatusCode >= http.StatusInternalServerError && attempt < maxAttempts-1 {
			drainAndClose(resp.Body)
			sleepBackoff(ctx, attempt)
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return decodeAPIError(resp)
		}
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
		return nil
	}
	return lastErr
}

func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	message := strings.TrimSpace(string(body))
	if message != "" {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			if s, ok := payload["error"].(string); ok && s != "" {
				message = s
			} else if s, ok := payload["message"].(string); ok && s != "" {
				message = s
			}
		}
	}
	if message == "" {
		message = resp.Status
	}
	return &APIError{StatusCode: resp.StatusCode, Message: message}
}

func sleepBackoff(ctx context.Context, attempt int) {
	delay := 150 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func shouldRetry(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return true
}

func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}
