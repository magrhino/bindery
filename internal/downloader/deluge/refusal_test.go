package deluge_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/downloader/deluge"
)

// TestRemoveTorrent_RefusedIsAnError covers deluge's half of #2192.
// core.remove_torrent can answer a plain `false` with no RPC error object;
// doCall only inspects the error member, so RemoveTorrent discarded the
// boolean and reported the refusal as a removal.
func TestRemoveTorrent_RefusedIsAnError(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	c := clientFromServer(srv, "pw")
	ds.removeRefused = true

	ds.torrents["deadbeef"] = deluge.TorrentStatus{Hash: "deadbeef", State: "Seeding"}

	err := c.RemoveTorrent(context.Background(), "deadbeef", false)
	if err == nil {
		t.Fatal("expected an error when Deluge answers false, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deluge") {
		t.Errorf("error should name the client, got %q", err)
	}
	if !strings.Contains(err.Error(), "deadbeef") {
		t.Errorf("error should name the torrent, got %q", err)
	}
}

// TestRemoveTorrent_NullResultIsNotARefusal pins the deliberate tolerance: a
// daemon that answers null rather than a boolean has not refused anything, so
// it must not produce a spurious error.
func TestRemoveTorrent_NullResultIsNotARefusal(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	c := clientFromServer(srv, "pw")
	ds.nullResult = true

	ds.torrents["deadbeef"] = deluge.TorrentStatus{Hash: "deadbeef", State: "Seeding"}

	if err := c.RemoveTorrent(context.Background(), "deadbeef", false); err != nil {
		t.Fatalf("null result should not be treated as a refusal: %v", err)
	}
}
