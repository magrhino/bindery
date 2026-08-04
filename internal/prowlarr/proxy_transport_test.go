package prowlarr

import (
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/httpsec"
)

// The Prowlarr client must route through the shared outbound-proxy transport
// like the newznab search client that talks to the same hosts, so sync/test
// calls honor BINDERY_OUTBOUND_PROXY instead of dialing Prowlarr directly.
func TestNewWithTimeout_UsesProxyTransport(t *testing.T) {
	c := NewWithTimeout("http://prowlarr:9696", "key", 30*time.Second)
	if c.http.Transport != httpsec.DefaultProxyTransport() {
		t.Errorf("NewWithTimeout must use the shared proxy transport; got %#v", c.http.Transport)
	}
	if New("http://prowlarr:9696", "key").http.Transport != httpsec.DefaultProxyTransport() {
		t.Error("New must use the shared proxy transport")
	}
}
