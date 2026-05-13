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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestAddTorrent_ConcurrentUniqueHashes is a regression test for Bug 2.
// When AddTorrent is called concurrently (e.g. during a bulk grab), each call
// must return its own unique torrent hash. Without serialisation, both
// goroutines snapshot beforeSet while it is empty, both torrents are submitted,
// and both goroutines resolve to the same "newest" torrent (highest AddedOn) —
// leaving one download record permanently mapped to the wrong hash.
func TestAddTorrent_ConcurrentUniqueHashes(t *testing.T) {
	const (
		hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	var mu sync.Mutex
	addedCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			mu.Lock()
			count := addedCount
			mu.Unlock()

			var torrents []Torrent
			if count >= 1 {
				torrents = append(torrents, Torrent{Hash: hashA, Name: "Book A", AddedOn: 1000})
			}
			if count >= 2 {
				torrents = append(torrents, Torrent{Hash: hashB, Name: "Book B", AddedOn: 2000})
			}
			body, _ := json.Marshal(torrents)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case "/api/v2/torrents/add":
			// Sleep long enough to guarantee both goroutines complete their
			// initial "before" GetTorrents snapshots before any add is
			// acknowledged, reliably opening the race window.
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			addedCount++
			mu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	c.loggedIn = true

	var wg sync.WaitGroup
	results := make([]string, 2)
	errs := make([]error, 2)

	for i := range results {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = c.AddTorrent(
				context.Background(),
				"torrent-source://book",
				"", "",
			)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	if results[0] == results[1] {
		t.Errorf("bug 2 race: both concurrent AddTorrent calls returned %q; each must return a unique hash", results[0])
	}
	got := map[string]bool{results[0]: true, results[1]: true}
	if !got[hashA] || !got[hashB] {
		t.Errorf("want one goroutine to get %q and the other %q, got %q and %q", hashA, hashB, results[0], results[1])
	}
}

// logCatcher captures slog records for test assertions.
type logCatcher struct {
	mu      sync.Mutex
	records []slog.Record
}

func (lc *logCatcher) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (lc *logCatcher) Handle(_ context.Context, r slog.Record) error {
	lc.mu.Lock()
	lc.records = append(lc.records, r.Clone())
	lc.mu.Unlock()
	return nil
}
func (lc *logCatcher) WithAttrs(_ []slog.Attr) slog.Handler { return lc }
func (lc *logCatcher) WithGroup(_ string) slog.Handler      { return lc }

func (lc *logCatcher) Records() []slog.Record {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.records
}

// newTestClient creates a Client pointing at the given test server URL.
func newTestClient(serverURL, username, password string) *Client {
	c := New("localhost", 8080, username, password, "", false)
	c.baseURL = serverURL
	return c
}

func allowTorrentFetch(c *Client) {
	c.validateTorrentURL = func(string) error { return nil }
}

func torrentFixture() ([]byte, string) {
	info := []byte("d6:lengthi123e4:name8:book.txt12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae")
	sum := sha1.Sum(info)
	data := []byte("d8:announce14:http://tracker4:info" + string(info) + "e")
	return data, hex.EncodeToString(sum[:])
}

func TestNew(t *testing.T) {
	c := New("myhost", 8080, "admin", "secret", "", false)
	if c.baseURL != "http://myhost:8080" {
		t.Errorf("baseURL: want %q, got %q", "http://myhost:8080", c.baseURL)
	}
	if c.username != "admin" || c.password != "secret" {
		t.Error("credentials not stored correctly")
	}
	if c.loggedIn {
		t.Error("should not be logged in on construction")
	}

	cs := New("securehost", 443, "u", "p", "", true)
	if cs.baseURL != "https://securehost:443" {
		t.Errorf("SSL baseURL: got %q", cs.baseURL)
	}
}

func TestLogin_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth/login" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("username") != "admin" || r.FormValue("password") != "pass" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Fails."))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !c.loggedIn {
		t.Error("loggedIn should be true after successful login")
	}
}

func TestLogin_Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "bad", "creds")
	if err := c.Login(context.Background()); err == nil {
		t.Fatal("expected Login to return error on 'Fails.' body")
	}
	if c.loggedIn {
		t.Error("loggedIn should remain false after failed login")
	}
}

func TestLogin_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Login(context.Background()); err == nil {
		t.Fatal("expected error on 500 response")
	}
}

// TestLogin_V5_NoContent verifies that qBittorrent v5.x's `204 No Content`
// login response is treated as a success (v4.x returned `200 OK` + "Ok.").
func TestLogin_V5_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth/login" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !c.loggedIn {
		t.Error("loggedIn should be true after 204 response")
	}
}

// TestLogin_SendsCSRFHeaders verifies that Origin and Referer headers are
// sent on every login request, as required by qBittorrent v5.x CSRF checks.
// v4.x ignores these headers, so setting them is safe across versions.
func TestLogin_SendsCSRFHeaders(t *testing.T) {
	var gotOrigin, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			gotOrigin = r.Header.Get("Origin")
			gotReferer = r.Header.Get("Referer")
			_, _ = w.Write([]byte("Ok."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if gotOrigin != srv.URL {
		t.Errorf("Origin: want %q, got %q", srv.URL, gotOrigin)
	}
	if gotReferer != srv.URL {
		t.Errorf("Referer: want %q, got %q", srv.URL, gotReferer)
	}
}

// TestLogin_AuthError_BadCreds covers the "200 + Fails." path. Bindery
// should return an *AuthError that surfaces the credentials hint.
func TestLogin_AuthError_BadCreds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "bad", "creds")
	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *AuthError, got %T: %v", err, err)
	}
	if authErr.Status != http.StatusOK || authErr.Body != "Fails." {
		t.Errorf("AuthError fields: got status=%d body=%q", authErr.Status, authErr.Body)
	}
	if !strings.Contains(err.Error(), "credentials rejected") {
		t.Errorf("expected credentials hint, got %q", err.Error())
	}
}

// TestLogin_AuthError_BanEmpty403 covers the qBit IP-ban shape: HTTP 403
// with an empty body. Pre-fix this surfaced as "qBittorrent login failed: "
// (nothing useful). Now should explain IP-ban + how to clear it.
func TestLogin_AuthError_BanEmpty403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *AuthError, got %T: %v", err, err)
	}
	if authErr.Status != http.StatusForbidden || authErr.Body != "" {
		t.Errorf("AuthError fields: got status=%d body=%q", authErr.Status, authErr.Body)
	}
	if !strings.Contains(err.Error(), "IP is most likely banned") {
		t.Errorf("expected IP-ban hint, got %q", err.Error())
	}
}

// TestTest_AuthErrorDoesNotMisdirect proves Test() no longer wraps auth
// failures with the "could not reach + use container name" hint that only
// makes sense for actual transport failures.
func TestTest_AuthErrorDoesNotMisdirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusForbidden) // simulate IP ban
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "could not reach") {
		t.Errorf("auth failure must not be reported as 'could not reach': %q", msg)
	}
	if !strings.Contains(msg, "connected to qBittorrent") {
		t.Errorf("expected 'connected to qBittorrent at ... but ...' wording: %q", msg)
	}
	if !strings.Contains(msg, "IP is most likely banned") {
		t.Errorf("expected the underlying AuthError hint to propagate: %q", msg)
	}
}

func TestTest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("5.0.0"))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestTest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.Test(context.Background()); err == nil {
		t.Fatal("expected Test to fail on 500")
	}
}

func TestAddTorrent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("urls") == "" {
				t.Error("expected urls in form body")
			}
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if _, err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc123", "", ""); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
}

func TestTest_TorrentListFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("5.0.0"))
		case "/api/v2/torrents/info":
			http.Error(w, "broken list", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected Test to fail when torrent listing fails")
	}
	if !strings.Contains(err.Error(), "could not list torrents") {
		t.Errorf("expected torrent-list error, got %q", err.Error())
	}
}

func TestAddTorrent_WithCategoryAndSavePath(t *testing.T) {
	var gotCategory, gotSavePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			_ = r.ParseForm()
			gotCategory = r.FormValue("category")
			gotSavePath = r.FormValue("savepath")
			_, _ = w.Write([]byte("Ok."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if _, err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc", "books", "/downloads"); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if gotCategory != "books" {
		t.Errorf("category: want 'books', got %q", gotCategory)
	}
	if gotSavePath != "/downloads" {
		t.Errorf("savepath: want '/downloads', got %q", gotSavePath)
	}
}

func TestGetCategories_NormalizesSavePathKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/categories":
			_, _ = w.Write([]byte(`{
				"books":{"name":"books","savePath":"/media/books"},
				"audiobooks":{"name":"audiobooks","save_path":"/media/audio"}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	cats, err := c.GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if cats["books"].SavePath != "/media/books" {
		t.Errorf("books save path = %q", cats["books"].SavePath)
	}
	if cats["audiobooks"].SavePath != "/media/audio" {
		t.Errorf("audiobooks save path = %q", cats["audiobooks"].SavePath)
	}
}

func TestAddTorrent_FailsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Fails."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if _, err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc", "", ""); err == nil {
		t.Fatal("expected error on 'Fails.' body")
	}
}

func TestAddTorrent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad request"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if _, err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc", "", ""); err == nil {
		t.Fatal("expected error on 400")
	}
}

func TestAddTorrent_HTTPURLFetchesTorrentAndUploadsFile(t *testing.T) {
	torrentData, wantHash := torrentFixture()
	var gotAccept string
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(torrentData)
	}))
	defer source.Close()

	var addHit bool
	var gotCategory, gotSavePath string
	var gotUploaded []byte
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			addHit = true
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
				t.Fatalf("expected multipart upload, got %q", r.Header.Get("Content-Type"))
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			if urls := r.FormValue("urls"); urls != "" {
				t.Fatalf("torrent URL must not be sent to qBit, got urls=%q", urls)
			}
			gotCategory = r.FormValue("category")
			gotSavePath = r.FormValue("savepath")
			file, _, err := r.FormFile("torrents")
			if err != nil {
				t.Fatalf("expected torrents file upload: %v", err)
			}
			defer file.Close()
			gotUploaded, _ = io.ReadAll(file)
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	allowTorrentFetch(c)
	hash, err := c.AddTorrent(context.Background(), source.URL+"/12/download?id=abc", "books", "/downloads")
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if hash != wantHash {
		t.Errorf("hash: want %q, got %q", wantHash, hash)
	}
	if !addHit {
		t.Fatal("expected qBit add endpoint to be called")
	}
	if gotAccept != "application/x-bittorrent" {
		t.Errorf("Accept header: want application/x-bittorrent, got %q", gotAccept)
	}
	if gotCategory != "books" {
		t.Errorf("category: want books, got %q", gotCategory)
	}
	if gotSavePath != "/downloads" {
		t.Errorf("savepath: want /downloads, got %q", gotSavePath)
	}
	if !bytes.Equal(gotUploaded, torrentData) {
		t.Errorf("uploaded torrent bytes differ from fetched bytes")
	}
}

func TestAddTorrent_HTTPURLFetchFailureDoesNotCallQbitAdd(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer source.Close()

	addHit := false
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			addHit = true
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	allowTorrentFetch(c)
	_, err := c.AddTorrent(context.Background(), source.URL+"/missing.torrent", "books", "")
	if err == nil {
		t.Fatal("expected fetch failure")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 error, got %q", err.Error())
	}
	if addHit {
		t.Fatal("qBit add endpoint should not be called when torrent fetch fails")
	}
}

func TestAddTorrent_HTTPURLInvalidTorrentDoesNotCallQbitAdd(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not a torrent</html>"))
	}))
	defer source.Close()

	addHit := false
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			addHit = true
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	allowTorrentFetch(c)
	_, err := c.AddTorrent(context.Background(), source.URL+"/bad.torrent", "books", "")
	if err == nil {
		t.Fatal("expected invalid torrent error")
	}
	if !strings.Contains(err.Error(), "invalid torrent file") {
		t.Errorf("expected invalid torrent error, got %q", err.Error())
	}
	if addHit {
		t.Fatal("qBit add endpoint should not be called for invalid torrent data")
	}
}

func TestInfoHashFromTorrentFile_RejectsOversizedStringLength(t *testing.T) {
	_, err := infoHashFromTorrentFile([]byte("d4:info9223372036854775807:xee"))
	if err == nil {
		t.Fatal("expected invalid torrent error")
	}
	if !strings.Contains(err.Error(), "string exceeds input") {
		t.Errorf("expected string exceeds input error, got %q", err.Error())
	}
}

func TestAddTorrent_HTTPURLOversizedTorrentDoesNotCallQbitAdd(t *testing.T) {
	orig := maxTorrentFileBytes
	maxTorrentFileBytes = 8
	t.Cleanup(func() { maxTorrentFileBytes = orig })

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("123456789"))
	}))
	defer source.Close()

	addHit := false
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			addHit = true
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	allowTorrentFetch(c)
	_, err := c.AddTorrent(context.Background(), source.URL+"/too-large.torrent", "books", "")
	if err == nil {
		t.Fatal("expected oversized torrent error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected oversized error, got %q", err.Error())
	}
	if addHit {
		t.Fatal("qBit add endpoint should not be called for oversized torrent data")
	}
}

func TestAddTorrent_HTTPRedirectToMagnetUsesURLField(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:ABCDEF123&dn=Book"
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, magnet, http.StatusFound)
	}))
	defer source.Close()

	var gotURL string
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			gotURL = r.FormValue("urls")
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	allowTorrentFetch(c)
	hash, err := c.AddTorrent(context.Background(), source.URL+"/redirect", "books", "")
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if hash != "abcdef123" {
		t.Errorf("hash: want abcdef123, got %q", hash)
	}
	if gotURL != magnet {
		t.Errorf("urls field: want %q, got %q", magnet, gotURL)
	}
}

func TestAddTorrent_HTTPURLDefaultValidationRejectsLoopbackBeforeFetch(t *testing.T) {
	sourceHit := false
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceHit = true
		_, _ = w.Write([]byte("should not be fetched"))
	}))
	defer source.Close()

	addHit := false
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			addHit = true
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	_, err := c.AddTorrent(context.Background(), source.URL+"/blocked.torrent", "books", "")
	if err == nil {
		t.Fatal("expected loopback URL validation failure")
	}
	if !strings.Contains(err.Error(), "url not allowed") {
		t.Errorf("expected url validation error, got %q", err.Error())
	}
	if sourceHit {
		t.Fatal("source URL must not be fetched after validation failure")
	}
	if addHit {
		t.Fatal("qBit add endpoint should not be called after validation failure")
	}
}

func TestAddTorrent_HTTPRedirectTargetValidationRejectsBeforeFetch(t *testing.T) {
	targetHit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit = true
		_, _ = w.Write([]byte("should not be fetched"))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/blocked.torrent", http.StatusFound)
	}))
	defer source.Close()

	addHit := false
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			addHit = true
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qbit.Close()

	c := newTestClient(qbit.URL, "admin", "pass")
	c.validateTorrentURL = func(raw string) error {
		if strings.HasPrefix(raw, target.URL) {
			return fmt.Errorf("blocked redirect target")
		}
		return nil
	}

	_, err := c.AddTorrent(context.Background(), source.URL+"/redirect", "books", "")
	if err == nil {
		t.Fatal("expected redirect target validation failure")
	}
	if !strings.Contains(err.Error(), "blocked redirect target") {
		t.Errorf("expected redirect target validation error, got %q", err.Error())
	}
	if targetHit {
		t.Fatal("redirect target must not be fetched after validation failure")
	}
	if addHit {
		t.Fatal("qBit add endpoint should not be called after redirect validation failure")
	}
}

func TestGetTorrents_Success(t *testing.T) {
	want := []Torrent{
		{Hash: "abc123", Name: "My Book", Size: 1024, Progress: 0.5, State: "downloading", Category: "books"},
		{Hash: "def456", Name: "Another Book", Size: 2048, Progress: 1.0, State: "seeding"},
	}
	body, _ := json.Marshal(want)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	torrents, err := c.GetTorrents(context.Background(), "")
	if err != nil {
		t.Fatalf("GetTorrents: %v", err)
	}
	if len(torrents) != 2 {
		t.Fatalf("expected 2 torrents, got %d", len(torrents))
	}
	if torrents[0].Hash != "abc123" {
		t.Errorf("first hash: want 'abc123', got %q", torrents[0].Hash)
	}
	if torrents[1].Name != "Another Book" {
		t.Errorf("second name: want 'Another Book', got %q", torrents[1].Name)
	}
}

func TestGetTorrents_WithCategory(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			gotRawQuery = r.URL.RawQuery
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_, err := c.GetTorrents(context.Background(), "audiobooks")
	if err != nil {
		t.Fatalf("GetTorrents: %v", err)
	}
	if !strings.Contains(gotRawQuery, "category=audiobooks") {
		t.Errorf("expected category in query string, got: %q", gotRawQuery)
	}
}

func TestGetTorrents_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("not valid json"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if _, err := c.GetTorrents(context.Background(), ""); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestDeleteTorrent_Success(t *testing.T) {
	var gotHash, gotDeleteFiles string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/delete":
			_ = r.ParseForm()
			gotHash = r.FormValue("hashes")
			gotDeleteFiles = r.FormValue("deleteFiles")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.DeleteTorrent(context.Background(), "abc123", true); err != nil {
		t.Fatalf("DeleteTorrent: %v", err)
	}
	if gotHash != "abc123" {
		t.Errorf("hashes: want 'abc123', got %q", gotHash)
	}
	if gotDeleteFiles != "true" {
		t.Errorf("deleteFiles: want 'true', got %q", gotDeleteFiles)
	}
}

func TestDeleteTorrent_KeepFiles(t *testing.T) {
	var gotDeleteFiles string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/delete":
			_ = r.ParseForm()
			gotDeleteFiles = r.FormValue("deleteFiles")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_ = c.DeleteTorrent(context.Background(), "abc", false)
	if gotDeleteFiles != "false" {
		t.Errorf("deleteFiles: want 'false', got %q", gotDeleteFiles)
	}
}

func TestDeleteTorrent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/delete":
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.DeleteTorrent(context.Background(), "abc", false); err == nil {
		t.Fatal("expected error on 500")
	}
}

// TestGet_403Retry verifies that a 403 triggers re-login and a single retry.
func TestGet_403Retry(t *testing.T) {
	loginCount := 0
	versionHits := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			loginCount++
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			versionHits++
			if versionHits == 1 {
				// Simulate expired session
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("5.0.0"))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	// Mark as already logged in so the first call skips the initial login.
	c.loggedIn = true

	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if loginCount != 1 {
		t.Errorf("expected 1 re-login on 403, got %d", loginCount)
	}
	if versionHits != 2 {
		t.Errorf("expected 2 version requests (403 + retry), got %d", versionHits)
	}
}

// TestEnsureLoggedIn_AlreadyLoggedIn verifies that Login is not called again.
func TestEnsureLoggedIn_AlreadyLoggedIn(t *testing.T) {
	loginCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			loginCount++
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	c.loggedIn = true
	if err := c.ensureLoggedIn(context.Background()); err != nil {
		t.Fatalf("ensureLoggedIn: %v", err)
	}
	if loginCount != 0 {
		t.Errorf("Login should not be called when already logged in; called %d times", loginCount)
	}
}

// TestEnsureLoggedIn_NotLoggedIn verifies that Login is called when loggedIn=false.
func TestEnsureLoggedIn_NotLoggedIn(t *testing.T) {
	loginCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			loginCount++
			_, _ = w.Write([]byte("Ok."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	if err := c.ensureLoggedIn(context.Background()); err != nil {
		t.Fatalf("ensureLoggedIn: %v", err)
	}
	if loginCount != 1 {
		t.Errorf("Login should be called once when not logged in; called %d times", loginCount)
	}
}

// TestAddTorrent_HashFoundUnderDifferentCategory verifies that the unfiltered
// poll finds a torrent even when qBittorrent has initially placed it under a
// different category than the one requested.
func TestAddTorrent_HashFoundUnderDifferentCategory(t *testing.T) {
	const wantHash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	var setCategoryHash, setCategoryValue string
	var mu sync.Mutex
	added := false // becomes true after /torrents/add is called
	infoQueries := []string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			mu.Lock()
			added = true
			mu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			mu.Lock()
			infoQueries = append(infoQueries, r.URL.RawQuery)
			mu.Unlock()
			if r.URL.Query().Get("category") != "" {
				// A category-filtered lookup would reproduce the race from #418:
				// qBittorrent can expose the hash before category metadata lands.
				_, _ = w.Write([]byte("[]"))
				return
			}
			mu.Lock()
			isAdded := added
			mu.Unlock()
			if !isAdded {
				// Before add: no torrents yet.
				_, _ = w.Write([]byte("[]"))
				return
			}
			// After add: torrent appears under "uncategorized" (different from requested "books").
			torrents := []Torrent{{
				Hash:     wantHash,
				Name:     "Test Book",
				Category: "uncategorized",
				AddedOn:  1000,
			}}
			body, _ := json.Marshal(torrents)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case "/api/v2/torrents/setCategory":
			_ = r.ParseForm()
			mu.Lock()
			setCategoryHash = r.FormValue("hashes")
			setCategoryValue = r.FormValue("category")
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	hash, err := c.AddTorrent(context.Background(), "torrent-source://book", "books", "")
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if hash != wantHash {
		t.Errorf("hash: want %q, got %q", wantHash, hash)
	}
	mu.Lock()
	gotSetCatHash := setCategoryHash
	gotSetCatVal := setCategoryValue
	mu.Unlock()
	if gotSetCatHash != wantHash {
		t.Errorf("setCategory hashes: want %q, got %q", wantHash, gotSetCatHash)
	}
	if gotSetCatVal != "books" {
		t.Errorf("setCategory category: want %q, got %q", "books", gotSetCatVal)
	}
	mu.Lock()
	gotInfoQueries := append([]string(nil), infoQueries...)
	mu.Unlock()
	if len(gotInfoQueries) == 0 {
		t.Fatal("expected torrent info to be polled")
	}
	for _, rawQuery := range gotInfoQueries {
		if rawQuery != "" {
			t.Errorf("torrent info poll should be unfiltered, got query %q", rawQuery)
		}
	}
}

// TestAddTorrent_HashLookupTimeout verifies that when the torrent never appears
// within the deadline, an ERROR is logged with before/after hash lists and the
// appropriate error is returned.
func TestAddTorrent_HashLookupTimeout(t *testing.T) {
	orig := hashPollTimeout
	hashPollTimeout = 50 * time.Millisecond
	t.Cleanup(func() { hashPollTimeout = orig })

	catcher := &logCatcher{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(catcher))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			// Never return the new torrent — list stays empty.
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_, err := c.AddTorrent(context.Background(), "torrent-source://book", "books", "")
	if err == nil {
		t.Fatal("expected error on timeout, got nil")
	}
	if !strings.Contains(err.Error(), "hash could not be determined") {
		t.Errorf("unexpected error: %v", err)
	}

	records := catcher.Records()
	if len(records) == 0 {
		t.Fatal("expected slog.Error to be called on timeout")
	}
	found := false
	for _, r := range records {
		if r.Level == slog.LevelError && strings.Contains(r.Message, "timed out") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ERROR log with 'timed out' message, got %d records", len(records))
	}
}

// roundTripFunc is a test helper that implements http.RoundTripper via a function.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// qbNetTimeoutErr is a minimal net.Error that signals a timeout.
type qbNetTimeoutErr struct{}

func (e *qbNetTimeoutErr) Error() string   { return "i/o timeout" }
func (e *qbNetTimeoutErr) Timeout() bool   { return true }
func (e *qbNetTimeoutErr) Temporary() bool { return true }

// newTransportClient creates a qBittorrent Client with a custom HTTP transport.
func newTransportClient(transport http.RoundTripper) *Client {
	c := New("fake-host", 8080, "admin", "pass", "", false)
	c.http = &http.Client{Transport: transport, Jar: c.http.Jar}
	return c
}

// TestTest_DNSNotFound verifies that a DNS lookup failure appends the Docker
// network hint and does NOT misclassify it as an auth error.
func TestTest_DNSNotFound(t *testing.T) {
	dnsErr := &net.DNSError{Name: "qbittorrent-container", IsNotFound: true}
	c := newTransportClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial: %w", dnsErr)
	}))

	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "connected to qBittorrent") {
		t.Errorf("DNS failure must not be reported as auth error: %q", msg)
	}
	if !strings.Contains(msg, "same Docker network") {
		t.Errorf("expected Docker network hint, got: %q", msg)
	}
}

// TestTest_ConnectionRefused verifies that ECONNREFUSED appends the port hint.
func TestTest_ConnectionRefused(t *testing.T) {
	c := newTransportClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial tcp: %w", syscall.ECONNREFUSED)
	}))

	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "service may not be running") {
		t.Errorf("expected port hint, got: %q", err.Error())
	}
}

// TestTest_Timeout_QBit verifies that a timeout error appends the firewall hint.
func TestTest_Timeout_QBit(t *testing.T) {
	c := newTransportClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &qbNetTimeoutErr{}
	}))

	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "firewall or proxy") {
		t.Errorf("expected firewall hint, got: %q", err.Error())
	}
}

// TestTest_ServerError_NoHint_QBit verifies that an HTTP 500 from the server
// does NOT produce a network hint (qBittorrent responded, transport worked).
func TestTest_ServerError_NoHint_QBit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		default:
			http.Error(w, "server error", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, hint := range []string{"Docker network", "service may not be running", "firewall or proxy"} {
		if strings.Contains(msg, hint) {
			t.Errorf("server error must not produce hint %q; got: %q", hint, msg)
		}
	}
}
