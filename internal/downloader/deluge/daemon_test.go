package deluge_test

import (
	"context"
	"strings"
	"testing"
)

// The tests below cover #2204: deluge-web and deluged are separate processes,
// and a Web UI session that has not been attached to a daemon answers
// auth.login happily while failing every core.* method. Bindery never called
// web.connect, so Test reported success against a session that could not grab
// anything.

// TestConnectDaemon_AlreadyConnected_SkipsConnect pins the no-op path: when
// web.connected already reports a daemon, nothing extra goes over the wire.
func TestConnectDaemon_AlreadyConnected_SkipsConnect(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	c := clientFromServer(srv, "pw")

	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test() against a connected session: %v", err)
	}
	if len(ds.connectCalls) != 0 {
		t.Errorf("web.connect must not fire when a daemon is already attached; got %v", ds.connectCalls)
	}
	if ds.connectedCalls == 0 {
		t.Error("web.connected was never asked")
	}
}

// TestConnectDaemon_SingleHost_AttachesThenSucceeds is the reported shape: the
// session has no daemon, exactly one host is configured, so Bindery attaches it
// and the grab goes through.
func TestConnectDaemon_SingleHost_AttachesThenSucceeds(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	ds.daemonConnected = false
	c := clientFromServer(srv, "pw")

	const magnet = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=Test+Book"
	hash, err := c.AddTorrent(context.Background(), magnet, "", nil)
	if err != nil {
		t.Fatalf("AddTorrent after attaching a daemon: %v", err)
	}
	if hash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("unexpected hash %q", hash)
	}
	if len(ds.connectCalls) != 1 || ds.connectCalls[0] != "hostid-local" {
		t.Errorf("web.connect calls = %v, want exactly [hostid-local]", ds.connectCalls)
	}
}

// TestConnectDaemon_MultipleHosts_RefusesToGuess: attaching an arbitrary daemon
// would send grabs somewhere the importer never looks, so Bindery reports the
// ambiguity instead of picking one.
func TestConnectDaemon_MultipleHosts_RefusesToGuess(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	ds.daemonConnected = false
	ds.hosts = []any{
		[]any{"hostid-local", "127.0.0.1", 58846, "localclient"},
		[]any{"hostid-seedbox", "10.0.0.9", 58846, "seedbox"},
	}
	c := clientFromServer(srv, "pw")

	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected an error when several daemon hosts are configured")
	}
	msg := err.Error()
	for _, want := range []string{"daemon", "127.0.0.1:58846", "10.0.0.9:58846"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q, got %q", want, msg)
		}
	}
	if len(ds.connectCalls) != 0 {
		t.Errorf("no host may be picked when the choice is ambiguous; got %v", ds.connectCalls)
	}
}

// TestConnectDaemon_ConnectFailureIsSurfaced: when web.connect itself fails the
// grab must fail with that reason attached, not swallow it and go on to a
// core.* call that cannot work.
func TestConnectDaemon_ConnectFailureIsSurfaced(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	ds.daemonConnected = false
	ds.connectErr = true
	c := clientFromServer(srv, "pw")

	const magnet = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd"
	_, err := c.AddTorrent(context.Background(), magnet, "", nil)
	if err == nil {
		t.Fatal("expected AddTorrent to fail when web.connect fails")
	}
	if !strings.Contains(err.Error(), "Failed to connect to daemon") {
		t.Errorf("the daemon's own reason should survive, got %q", err)
	}
}

// TestTest_NoDaemonHost_Fails is the defect underneath the report: auth.login
// succeeds, so the old Test passed, while every grab failed. With no host
// configured there is nothing to attach and Test must say so.
func TestTest_NoDaemonHost_Fails(t *testing.T) {
	srv, ds := newTestServer(t, "pw")
	ds.daemonConnected = false
	ds.hosts = nil
	c := clientFromServer(srv, "pw")

	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("Test must fail when the Web UI has no daemon and none can be attached")
	}
	msg := err.Error()
	if !strings.Contains(msg, "daemon") {
		t.Errorf("error should name the missing daemon, got %q", msg)
	}
	// The Web UI answered, so the network hints would be a false trail.
	for _, hint := range []string{"Docker network", "check the port", "firewall or proxy"} {
		if strings.Contains(msg, hint) {
			t.Errorf("a missing daemon must not produce the transport hint %q; got %q", hint, msg)
		}
	}
}
