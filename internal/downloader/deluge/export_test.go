package deluge

import (
	"net/http"
	"time"
)

// HashPollTimeout exposes the package-level poll timeout for test overrides.
var HashPollTimeout = &hashPollTimeout

// SetHashPollTimeout temporarily overrides hashPollTimeout for a test and
// returns a function that restores the original value.
func SetHashPollTimeout(d time.Duration) func() {
	orig := hashPollTimeout
	hashPollTimeout = d
	return func() { hashPollTimeout = orig }
}

// SetHTTPTransport replaces the underlying http.Client transport for testing.
func (c *Client) SetHTTPTransport(rt http.RoundTripper) {
	c.http.Transport = rt
}
