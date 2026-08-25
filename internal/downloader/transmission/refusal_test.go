package transmission

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRemoveTorrent_RefusedIsAnError covers transmission's half of #2192.
// Every Transmission RPC reply carries a "result" string that is either
// "success" or the failure reason, always under HTTP 200. RemoveTorrent threw
// the body away with `_`, so a refused torrent-remove was reported as a
// removal. AddTorrent has always checked the same field.
func TestRemoveTorrent_RefusedIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"torrent-remove failed","arguments":{}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "user", "pass")
	err := c.RemoveTorrent(context.Background(), 42, false)
	if err == nil {
		t.Fatal("expected an error when Transmission refuses the removal, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "transmission") {
		t.Errorf("error should name the client, got %q", err)
	}
	if !strings.Contains(err.Error(), "torrent-remove failed") {
		t.Errorf("error should carry Transmission's reason, got %q", err)
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error should name the torrent id, got %q", err)
	}
}

// TestRemoveTorrent_RefusedWithoutReason verifies the message never degrades to
// a trailing colon when "result" comes back empty.
func TestRemoveTorrent_RefusedWithoutReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"arguments":{}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "user", "pass")
	err := c.RemoveTorrent(context.Background(), 42, false)
	if err == nil {
		t.Fatal("expected an error when result is missing, got nil")
	}
	if !strings.Contains(err.Error(), "Transmission gave no reason") {
		t.Errorf("expected the no-reason fallback, got %q", err)
	}
}
