// Package qbittorrent provides a client for the qBittorrent WebUI API v2,
// used to submit magnet/torrent URLs and poll status for torrent downloads.
package qbittorrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/vavallee/bindery/internal/downloader/nethint"
	"github.com/vavallee/bindery/internal/downloader/urlbase"
	"github.com/vavallee/bindery/internal/httpsec"
)

// AuthError signals that qBittorrent responded but rejected the login.
// Test() inspects this type via errors.As so it can avoid wrapping auth
// failures with the misleading "could not reach + use container name"
// hint that only applies to actual transport failures.
type AuthError struct {
	Status int
	Body   string
}

func (e *AuthError) Error() string {
	switch {
	case e.Status == http.StatusForbidden && e.Body == "":
		return "qBittorrent auth failed (HTTP 403, empty body): your IP is most likely banned after repeated failed logins. " +
			"Clear it in qBit (Tools → Options → Web UI → IP filtering, or the banlist in qBittorrent.conf — restart of qBit may not clear it because the banlist is persisted)."
	case e.Status == http.StatusOK && e.Body == "Fails.":
		return "qBittorrent auth failed: credentials rejected (check the WebUI username/password matches what's saved in bindery)."
	case e.Status == http.StatusForbidden:
		return fmt.Sprintf("qBittorrent auth failed (HTTP 403): %s — host-header validation may be rejecting bindery; disable it in Tools → Options → Web UI, or whitelist the bindery container's hostname.", e.Body)
	case e.Status != http.StatusOK:
		return fmt.Sprintf("qBittorrent auth failed (HTTP %d): %s", e.Status, e.Body)
	default:
		return fmt.Sprintf("qBittorrent auth failed: %s", e.Body)
	}
}

// hashPollTimeout is the maximum time to wait for a newly-added torrent's hash
// to appear in the unfiltered torrent list.
var hashPollTimeout = 30 * time.Second

// maxTorrentFileBytes caps the torrent payload Bindery will fetch before
// uploading it to qBittorrent.
var maxTorrentFileBytes int64 = 32 << 20

// Client interacts with the qBittorrent WebUI API v2.
// Authentication is cookie-based: Login() obtains a SID cookie which is
// stored in the embedded http.Client's cookie jar and sent automatically on
// subsequent requests.
//
// Field mapping for DownloadClient storage:
//   - APIKey  → password  (qBittorrent uses username/password, not an API key)
//   - URLBase → reverse-proxy subpath, appended to baseURL (#369)
type Client struct {
	baseURL            string
	username           string
	password           string
	http               *http.Client
	validateTorrentURL func(string) error
	mu                 sync.Mutex // guards loggedIn
	addMu              sync.Mutex // serialises AddTorrent: keeps before/after hash diff atomic
	loggedIn           bool
}

// New creates a qBittorrent client. urlBase is the optional reverse-proxy
// subpath (e.g. "/qbit") that will be appended between the host:port and
// the standard /api/v2 endpoints; leave it empty for a direct connection.
func New(host string, port int, username, password, urlBase string, useSSL bool) *Client {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}

	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  fmt.Sprintf("%s://%s:%d%s", scheme, host, port, urlbase.Normalize(urlBase)),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 15 * time.Second, Jar: jar},
		validateTorrentURL: func(raw string) error {
			return httpsec.ValidateOutboundURL(raw, httpsec.PolicyLAN)
		},
	}
}

// Login authenticates with qBittorrent and stores the SID cookie.
func (c *Client) Login(ctx context.Context) error {
	form := url.Values{
		"username": {c.username},
		"password": {c.password},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/auth/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// qBittorrent v5.x enforces CSRF protection on /auth/login and rejects
	// requests without matching Origin and Referer headers (often silently —
	// the empty-body 403 that motivated AuthError above). v4.x ignores these
	// headers, so setting them is safe across versions.
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	text := strings.TrimSpace(string(body))

	// qBittorrent v4.x returns `200 OK` + body "Ok." on a successful login;
	// v5.x returns `204 No Content` with an empty body. Accept both.
	if resp.StatusCode == http.StatusNoContent {
		c.mu.Lock()
		c.loggedIn = true
		c.mu.Unlock()
		return nil
	}

	if resp.StatusCode != http.StatusOK || text == "Fails." {
		return &AuthError{Status: resp.StatusCode, Body: text}
	}

	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// Test verifies non-mutating connectivity by fetching the application version
// and listing torrents. The error wording adapts to the failure mode:
// auth/config issues (the server responded but rejected us) get a targeted
// hint; transport failures (the server didn't respond at all) get a hint based
// on the error class.
func (c *Client) Test(ctx context.Context) error {
	if _, err := c.get(ctx, "/api/v2/app/version"); err != nil {
		return c.testError(err)
	}
	if _, err := c.GetTorrents(ctx, ""); err != nil {
		return c.testError(fmt.Errorf("could not list torrents: %w", err))
	}
	return nil
}

func (c *Client) testError(err error) error {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		// Server responded — this is an auth/config issue, not unreachable.
		return fmt.Errorf("connected to qBittorrent at %s but %w", c.baseURL, err)
	}
	return fmt.Errorf("could not reach qBittorrent at %s — %w%s", c.baseURL, err, nethint.ForErr(err))
}

// AddTorrent submits a magnet link or torrent file to qBittorrent for download
// and returns the torrent hash when it can be determined.
func (c *Client) AddTorrent(ctx context.Context, magnetOrURL, category, savePath string) (string, error) {
	// Serialise concurrent AddTorrent calls so that each goroutine's
	// before-snapshot → submit → poll sequence is atomic. Without this, two
	// concurrent calls both snapshot an identical beforeSet, both submit their
	// torrents, and then both resolve to the same "newest" torrent hash — the
	// root cause of Bug 2.
	c.addMu.Lock()
	defer c.addMu.Unlock()

	if infoHash := infoHashFromMagnet(magnetOrURL); infoHash != "" {
		if err := c.addTorrentURL(ctx, magnetOrURL, category, savePath); err != nil {
			return "", err
		}
		return infoHash, nil
	}

	if isHTTPURL(magnetOrURL) {
		torrent, err := c.fetchTorrentFile(ctx, magnetOrURL)
		if err != nil {
			return "", err
		}
		if torrent.magnetURL != "" {
			infoHash := infoHashFromMagnet(torrent.magnetURL)
			beforeSet := map[string]struct{}{}
			if infoHash == "" {
				beforeSet = c.snapshotHashes(ctx)
			}
			if err := c.addTorrentURL(ctx, torrent.magnetURL, category, savePath); err != nil {
				return "", err
			}
			if infoHash != "" {
				return infoHash, nil
			}
			return c.waitForNewTorrent(ctx, beforeSet, category)
		}

		infoHash, err := infoHashFromTorrentFile(torrent.data)
		if err != nil {
			return "", err
		}
		beforeSet := map[string]struct{}{}
		if infoHash == "" {
			beforeSet = c.snapshotHashes(ctx)
		}
		if err := c.addTorrentFile(ctx, torrent.filename, torrent.data, category, savePath); err != nil {
			return "", err
		}
		if infoHash != "" {
			return infoHash, nil
		}
		return c.waitForNewTorrent(ctx, beforeSet, category)
	}

	beforeSet := c.snapshotHashes(ctx)
	if err := c.addTorrentURL(ctx, magnetOrURL, category, savePath); err != nil {
		return "", err
	}
	return c.waitForNewTorrent(ctx, beforeSet, category)
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func (c *Client) snapshotHashes(ctx context.Context) map[string]struct{} {
	beforeSet := map[string]struct{}{}
	if before, err := c.GetTorrents(ctx, ""); err == nil {
		for _, t := range before {
			beforeSet[strings.ToLower(t.Hash)] = struct{}{}
		}
	}
	return beforeSet
}

func (c *Client) waitForNewTorrent(ctx context.Context, beforeSet map[string]struct{}, category string) (string, error) {
	deadline := time.Now().Add(hashPollTimeout)
	var lastAfter []Torrent
	for {
		after, err := c.GetTorrents(ctx, "")
		if err != nil {
			return "", fmt.Errorf("add torrent accepted but hash lookup failed: %w", err)
		}
		lastAfter = after
		var newest *Torrent
		for i := range after {
			t := &after[i]
			h := strings.ToLower(t.Hash)
			if _, seen := beforeSet[h]; seen {
				continue
			}
			if newest == nil || t.AddedOn > newest.AddedOn {
				newest = t
			}
		}
		if newest != nil {
			hash := strings.ToLower(newest.Hash)
			if category != "" {
				_ = c.setCategory(ctx, hash, category)
			}
			return hash, nil
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	beforeKeys := make([]string, 0, len(beforeSet))
	for h := range beforeSet {
		beforeKeys = append(beforeKeys, h)
	}
	afterKeys := make([]string, 0, len(lastAfter))
	for i := range lastAfter {
		afterKeys = append(afterKeys, strings.ToLower(lastAfter[i].Hash))
	}
	slog.Error("add torrent hash lookup timed out",
		"category", category,
		"before_hashes", beforeKeys,
		"after_hashes", afterKeys,
	)
	return "", fmt.Errorf("add torrent accepted but hash could not be determined")
}

func (c *Client) addTorrentURL(ctx context.Context, magnetOrURL, category, savePath string) error {
	form := url.Values{"urls": {magnetOrURL}}
	return c.addTorrentForm(ctx, form, category, savePath)
}

func (c *Client) addTorrentForm(ctx context.Context, form url.Values, category, savePath string) error {
	if category != "" {
		form.Set("category", category)
	}
	if savePath != "" {
		form.Set("savepath", savePath)
	}

	if err := c.ensureLoggedIn(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/torrents/add",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build add request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.doAddRequest(req)
}

func (c *Client) addTorrentFile(ctx context.Context, filename string, data []byte, category, savePath string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("torrents", filename)
	if err != nil {
		return fmt.Errorf("build torrent upload: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write torrent upload: %w", err)
	}
	if category != "" {
		if err := writer.WriteField("category", category); err != nil {
			return fmt.Errorf("write torrent category: %w", err)
		}
	}
	if savePath != "" {
		if err := writer.WriteField("savepath", savePath); err != nil {
			return fmt.Errorf("write torrent savepath: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close torrent upload: %w", err)
	}

	if err := c.ensureLoggedIn(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/torrents/add", &body)
	if err != nil {
		return fmt.Errorf("build add request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doAddRequest(req)
}

func (c *Client) doAddRequest(req *http.Request) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add torrent: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	text := strings.TrimSpace(string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("add torrent HTTP %d: %s", resp.StatusCode, text)
	}
	if text != "Ok." {
		return fmt.Errorf("add torrent failed: %s", text)
	}
	return nil
}

type fetchedTorrent struct {
	filename  string
	data      []byte
	magnetURL string
}

func (c *Client) fetchTorrentFile(ctx context.Context, rawURL string) (*fetchedTorrent, error) {
	current := rawURL
	fetchClient := &http.Client{
		Transport: c.http.Transport,
		Timeout:   c.http.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for redirects := 0; redirects <= 5; redirects++ {
		if err := c.validateTorrentFetchURL(current); err != nil {
			return nil, fmt.Errorf("download torrent file: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return nil, fmt.Errorf("build torrent download request: %w", err)
		}
		req.Header.Set("Accept", "application/x-bittorrent")

		resp, err := fetchClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download torrent file: %w", err)
		}

		if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
			location := resp.Header.Get("Location")
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if location == "" {
				return nil, fmt.Errorf("download torrent file: redirect without location")
			}
			if strings.HasPrefix(strings.ToLower(location), "magnet:") {
				return &fetchedTorrent{magnetURL: location}, nil
			}
			next, err := req.URL.Parse(location)
			if err != nil {
				return nil, fmt.Errorf("download torrent file: invalid redirect location: %w", err)
			}
			if next.Scheme != "http" && next.Scheme != "https" {
				return nil, fmt.Errorf("download torrent file: unsupported redirect scheme %q", next.Scheme)
			}
			current = next.String()
			continue
		}

		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("download torrent file HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		data, err := readLimited(resp.Body, maxTorrentFileBytes)
		if err != nil {
			return nil, err
		}
		data = append(body, data...)
		if int64(len(data)) > maxTorrentFileBytes {
			return nil, fmt.Errorf("download torrent file: response exceeds %d bytes", maxTorrentFileBytes)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("download torrent file: empty response")
		}

		filename := filenameFromURL(current)
		return &fetchedTorrent{filename: filename, data: data}, nil
	}

	return nil, fmt.Errorf("download torrent file: too many redirects")
}

func (c *Client) validateTorrentFetchURL(raw string) error {
	if c.validateTorrentURL == nil {
		return httpsec.ValidateOutboundURL(raw, httpsec.PolicyLAN)
	}
	return c.validateTorrentURL(raw)
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download torrent file: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("download torrent file: response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func filenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "download.torrent"
	}
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		return "download.torrent"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return name
}

func infoHashFromTorrentFile(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("download torrent file: empty response")
	}
	if data[0] != 'd' {
		return "", fmt.Errorf("download torrent file: invalid torrent file")
	}

	pos := 1
	for pos < len(data) && data[pos] != 'e' {
		key, next, err := readBencodeString(data, pos)
		if err != nil {
			return "", fmt.Errorf("download torrent file: invalid torrent file: %w", err)
		}
		pos = next
		valueStart := pos
		valueEnd, err := skipBencode(data, pos, 0)
		if err != nil {
			return "", fmt.Errorf("download torrent file: invalid torrent file: %w", err)
		}
		if key == "info" {
			if data[valueStart] != 'd' {
				return "", fmt.Errorf("download torrent file: invalid torrent file: info is not a dictionary")
			}
			hasPieces, err := bencodeDictHasKey(data[valueStart:valueEnd], "pieces")
			if err != nil {
				return "", fmt.Errorf("download torrent file: invalid torrent file: %w", err)
			}
			if !hasPieces {
				return "", nil
			}
			sum := sha1.Sum(data[valueStart:valueEnd])
			return hex.EncodeToString(sum[:]), nil
		}
		pos = valueEnd
	}
	if pos >= len(data) || data[pos] != 'e' {
		return "", fmt.Errorf("download torrent file: invalid torrent file")
	}
	if pos != len(data)-1 {
		return "", fmt.Errorf("download torrent file: invalid torrent file")
	}
	return "", fmt.Errorf("download torrent file: invalid torrent file: missing info dictionary")
}

func bencodeDictHasKey(data []byte, want string) (bool, error) {
	if len(data) == 0 || data[0] != 'd' {
		return false, fmt.Errorf("dictionary expected")
	}
	pos := 1
	for pos < len(data) && data[pos] != 'e' {
		key, next, err := readBencodeString(data, pos)
		if err != nil {
			return false, err
		}
		pos = next
		valueEnd, err := skipBencode(data, pos, 0)
		if err != nil {
			return false, err
		}
		if key == want {
			return true, nil
		}
		pos = valueEnd
	}
	return pos == len(data)-1 && data[pos] == 'e', nil
}

func readBencodeString(data []byte, pos int) (string, int, error) {
	if pos >= len(data) || data[pos] < '0' || data[pos] > '9' {
		return "", pos, fmt.Errorf("string expected")
	}
	length := 0
	for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
		digit := int(data[pos] - '0')
		if length > (len(data)-digit)/10 {
			return "", pos, fmt.Errorf("string exceeds input")
		}
		length = length*10 + digit
		pos++
	}
	if pos >= len(data) || data[pos] != ':' {
		return "", pos, fmt.Errorf("string length missing colon")
	}
	pos++
	if length > len(data)-pos {
		return "", pos, fmt.Errorf("string exceeds input")
	}
	end := pos + length
	return string(data[pos:end]), end, nil
}

func skipBencode(data []byte, pos, depth int) (int, error) {
	if depth > 128 {
		return pos, fmt.Errorf("bencode nesting too deep")
	}
	if pos >= len(data) {
		return pos, fmt.Errorf("unexpected end of input")
	}
	switch data[pos] {
	case 'i':
		end := strings.IndexByte(string(data[pos+1:]), 'e')
		if end < 0 {
			return pos, fmt.Errorf("unterminated integer")
		}
		return pos + 1 + end + 1, nil
	case 'l', 'd':
		pos++
		for pos < len(data) && data[pos] != 'e' {
			next, err := skipBencode(data, pos, depth+1)
			if err != nil {
				return pos, err
			}
			pos = next
		}
		if pos >= len(data) {
			return pos, fmt.Errorf("unterminated list or dictionary")
		}
		return pos + 1, nil
	default:
		if data[pos] >= '0' && data[pos] <= '9' {
			_, next, err := readBencodeString(data, pos)
			return next, err
		}
		return pos, fmt.Errorf("invalid bencode token %q", data[pos])
	}
}

func infoHashFromMagnet(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	xt := u.Query().Get("xt")
	if !strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
		return ""
	}
	h := strings.TrimSpace(xt[len("urn:btih:"):])
	if h == "" {
		return ""
	}
	return strings.ToLower(h)
}

// GetTorrents returns all torrents in the given category (empty = all).
func (c *Client) GetTorrents(ctx context.Context, category string) ([]Torrent, error) {
	endpoint := "/api/v2/torrents/info"
	if category != "" {
		endpoint += "?category=" + url.QueryEscape(category)
	}

	data, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var torrents []Torrent
	if err := json.Unmarshal(data, &torrents); err != nil {
		return nil, fmt.Errorf("decode torrents: %w", err)
	}
	return torrents, nil
}

// DeleteTorrent removes a torrent by hash, optionally deleting its files.
func (c *Client) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	deleteFilesStr := "false"
	if deleteFiles {
		deleteFilesStr = "true"
	}

	form := url.Values{
		"hashes":      {hash},
		"deleteFiles": {deleteFilesStr},
	}

	if err := c.ensureLoggedIn(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/torrents/delete",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete torrent: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete torrent HTTP %d", resp.StatusCode)
	}
	return nil
}

// setCategory assigns a category to a torrent by hash.
func (c *Client) setCategory(ctx context.Context, hash, category string) error {
	form := url.Values{
		"hashes":   {hash},
		"category": {category},
	}
	if err := c.ensureLoggedIn(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/torrents/setCategory",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build setCategory request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("setCategory: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// ensureLoggedIn logs in if not already authenticated.
func (c *Client) ensureLoggedIn(ctx context.Context) error {
	c.mu.Lock()
	loggedIn := c.loggedIn
	c.mu.Unlock()
	if loggedIn {
		return nil
	}
	return c.Login(ctx)
}

// get performs an authenticated GET request and returns the response body.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	if err := c.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		// Session expired — re-login once and retry.
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return nil, fmt.Errorf("build retry request for %s: %w", path, err)
		}
		resp2, err := c.http.Do(req2)
		if err != nil {
			return nil, fmt.Errorf("GET %s (retry): %w", path, err)
		}
		defer resp2.Body.Close()
		return io.ReadAll(resp2.Body)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
