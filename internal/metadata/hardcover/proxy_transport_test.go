package hardcover

import (
	"testing"

	"github.com/vavallee/bindery/internal/httpsec"
)

// NewAuthenticated must route through the shared outbound-proxy transport like
// New() does, so the list syncer and import-list browse (its callers) honor
// BINDERY_OUTBOUND_PROXY instead of dialing hardcover.app directly.
func TestNewAuthenticated_UsesProxyTransport(t *testing.T) {
	c := NewAuthenticated("tok")
	if c.http.Transport != httpsec.DefaultProxyTransport() {
		t.Errorf("NewAuthenticated must use the shared proxy transport; got %#v", c.http.Transport)
	}
}
