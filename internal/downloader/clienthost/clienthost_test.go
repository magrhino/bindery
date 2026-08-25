package clienthost

import (
	"strings"
	"testing"
)

// TestNormalize_Accepted covers everything a working install may already hold
// in the Host field. Every one of these must survive untouched (or, for the
// two unambiguous normalisations, survive with only the noise removed) —
// rejecting any of them would break a connection that works today.
func TestNormalize_Accepted(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"bare hostname", "qbittorrent", "qbittorrent"},
		{"docker service name", "qbittorrent_vpn", "qbittorrent_vpn"},
		{"fqdn", "qbit.home.arpa", "qbit.home.arpa"},
		{"fqdn with root dot", "qbit.home.arpa.", "qbit.home.arpa."},
		{"ipv4", "192.168.1.50", "192.168.1.50"},
		{"ipv6 bracketed", "[::1]", "[::1]"},
		{"ipv6 bare loopback", "::1", "::1"},
		{"ipv6 bare global", "fd00::1", "fd00::1"},
		{"http scheme stripped", "http://192.168.1.50", "192.168.1.50"},
		{"https scheme stripped", "https://qbit.home.arpa", "qbit.home.arpa"},
		{"scheme case insensitive", "HTTP://192.168.1.50", "192.168.1.50"},
		{"trailing slash dropped", "192.168.1.50/", "192.168.1.50"},
		{"scheme and trailing slash", "http://192.168.1.50/", "192.168.1.50"},
		{"surrounding whitespace", "  192.168.1.50  ", "192.168.1.50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in)
			if err != nil {
				t.Fatalf("Normalize(%q) returned error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalize_Rejected covers the values that cannot work. Each case checks
// the message names what is wrong and, where there is one, the value to move
// into the Port field — a rejection that does not say which box to edit is
// only marginally better than the decode error it replaces.
func TestNormalize_Rejected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       string
		contains []string
	}{
		{
			// The exact string from #2203, copied out of a browser address bar.
			name:     "address bar paste",
			in:       "10.1.2.3:8080/#/",
			contains: []string{"a port and a path", `"10.1.2.3"`, "8080 as the port"},
		},
		{
			name:     "embedded port",
			in:       "192.168.1.50:8080",
			contains: []string{"a port", `"192.168.1.50"`, "8080 as the port"},
		},
		{
			name:     "scheme and embedded port",
			in:       "http://192.168.1.50:9091",
			contains: []string{"a port", "9091 as the port"},
		},
		{
			name:     "path",
			in:       "192.168.1.50/qbittorrent",
			contains: []string{"a path", `"192.168.1.50"`},
		},
		{
			name:     "fragment",
			in:       "192.168.1.50#/downloads",
			contains: []string{"a fragment"},
		},
		{
			name:     "query string",
			in:       "192.168.1.50?v=2",
			contains: []string{"a query string"},
		},
		{
			name:     "bracketed ipv6 with port",
			in:       "[fd00::1]:8080",
			contains: []string{"a port", `"[fd00::1]"`, "8080 as the port"},
		},
		{
			name:     "unsupported scheme",
			in:       "ftp://192.168.1.50",
			contains: []string{"not a URL"},
		},
		{
			name:     "credentials",
			in:       "admin:hunter2@192.168.1.50",
			contains: []string{"Username and Password"},
		},
		{
			name:     "space in hostname",
			in:       "my qbit box",
			contains: []string{"hostname or IP address only"},
		},
		{
			name:     "empty",
			in:       "   ",
			contains: []string{"host is required"},
		},
		{
			name:     "colon without a valid port",
			in:       "192.168.1.50:notaport",
			contains: []string{"hostname or IP address only"},
		},
		{
			name:     "port out of range",
			in:       "192.168.1.50:99999",
			contains: []string{"hostname or IP address only"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in)
			if err == nil {
				t.Fatalf("Normalize(%q) = %q, want an error", tc.in, got)
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Normalize(%q) error = %q, want it to contain %q", tc.in, err, want)
				}
			}
		})
	}
}

// TestAuthority checks that both spellings of an IPv6 literal render to the
// one form a URL accepts, and that nothing else grows brackets.
func TestAuthority(t *testing.T) {
	for _, tc := range []struct {
		host string
		port int
		want string
	}{
		{"qbittorrent", 8080, "qbittorrent:8080"},
		{"192.168.1.50", 9091, "192.168.1.50:9091"},
		{"::1", 8080, "[::1]:8080"},
		{"[::1]", 8080, "[::1]:8080"},
		{"fd00::1", 8112, "[fd00::1]:8112"},
		{"[fd00::1]", 8112, "[fd00::1]:8112"},
		{" 192.168.1.50 ", 8080, "192.168.1.50:8080"},
	} {
		if got := Authority(tc.host, tc.port); got != tc.want {
			t.Errorf("Authority(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}
