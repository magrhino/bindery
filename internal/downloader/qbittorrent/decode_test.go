package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// webuiIndex is what qBittorrent's WebUI serves for any URL outside its API,
// which is what a Host field carrying a path or a fragment routes every
// request to (#2203).
const webuiIndex = `<!DOCTYPE html>
<html lang="en">
<head><title>qBittorrent Web UI</title></head>
<body><div id="desktop"></div></body>
</html>`

// serveWebUIIndex is a stub that logs in successfully and then answers every
// API path with the WebUI's own index page, HTTP 200 and all.
func serveWebUIIndex() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(webuiIndex))
	}))
}

// assertLegible checks that an error explains what came back instead of
// quoting the parser's byte-level complaint, and that it does not paste the
// page itself into the message.
func assertLegible(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid character '<'") {
		t.Errorf("error still surfaces the raw parser message: %q", msg)
	}
	if strings.Contains(msg, "<html") || strings.Contains(msg, "<!DOCTYPE") {
		t.Errorf("error pastes the HTML body into the message: %q", msg)
	}
	for _, want := range []string{"HTML page", "Host", "URL Base"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// TestGetTorrents_HTMLResponse is the poll-time symptom from #2203: the
// scanner logged "decode torrents: invalid character '<' looking for
// beginning of value" every 15 seconds and named nothing an operator could
// act on.
func TestGetTorrents_HTMLResponse(t *testing.T) {
	srv := serveWebUIIndex()
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_, err := c.GetTorrents(context.Background(), "books")
	assertLegible(t, err)
	if !strings.HasPrefix(err.Error(), "decode torrents: ") {
		t.Errorf("lost the operation prefix: %q", err)
	}
}

// TestGetCategories_HTMLResponse is the health-check symptom from the same
// report: "qBittorrent category path check failed: decode categories: invalid
// character '<' ...".
func TestGetCategories_HTMLResponse(t *testing.T) {
	srv := serveWebUIIndex()
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_, err := c.GetCategories(context.Background())
	assertLegible(t, err)
	if !strings.HasPrefix(err.Error(), "decode categories: ") {
		t.Errorf("lost the operation prefix: %q", err)
	}
}

func TestFiles_HTMLResponse(t *testing.T) {
	srv := serveWebUIIndex()
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	_, err := c.Files(context.Background(), "deadbeef")
	assertLegible(t, err)
}

// TestTest_HTMLResponse is the half of #2203 the reporter found hardest to
// believe: the Test button said the connection was verified while every poll
// failed. /api/v2/app/version answers in plain text, so an HTML page came
// back with HTTP 200 and nothing looked at it.
func TestTest_HTMLResponse(t *testing.T) {
	srv := serveWebUIIndex()
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	assertLegible(t, c.Test(context.Background()))
}

func TestTest_EmptyVersionResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("Ok."))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "admin", "pass")
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected Test to fail on an empty version response")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("error = %q, want it to mention an empty response", err)
	}
}

// TestDescribeDecodeFailure_ShapeMismatchKeepsParserMessage guards the other
// side of the rule: a body that really is JSON but does not fit the struct is
// a schema problem, and the parser's message is the informative one there.
func TestDescribeDecodeFailure_ShapeMismatchKeepsParserMessage(t *testing.T) {
	var torrents []Torrent
	err := decodeJSON("torrents", []byte(`{"error":"nope"}`), &torrents)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("error = %q, want the parser's type message", err)
	}
}

// TestDescribeDecodeFailure_PlainTextSnippet checks the fallback for a
// non-JSON, non-HTML body: a short quoted snippet, collapsed onto one line.
func TestDescribeDecodeFailure_PlainTextSnippet(t *testing.T) {
	var categories map[string]Category
	err := decodeJSON("categories", []byte("Forbidden\n\nyour IP is banned"), &categories)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Errorf("snippet was not collapsed onto one line: %q", msg)
	}
	if !strings.Contains(msg, "Forbidden your IP is banned") {
		t.Errorf("error = %q, want the body's opening text", err)
	}
}
