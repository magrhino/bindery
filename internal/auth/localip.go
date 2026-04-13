package auth

import (
	"net"
	"net/http"
	"strings"
)

// privateCIDRs: RFC1918 + loopback + link-local + IPv6 unique-local + IPv6 loopback.
// Matches what Sonarr considers "local" for the "Disabled for Local Addresses" mode.
var privateCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // link-local v4
		"::1/128",
		"fc00::/7",  // ULA v6
		"fe80::/10", // link-local v6
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// IsLocalRequest returns true when the request's client IP falls in a private
// range. Respects a cleaned RemoteAddr set by chi's RealIP middleware upstream.
func IsLocalRequest(r *http.Request) bool {
	return IsLocalIP(clientIP(r))
}

// IsLocalIP returns true for RFC1918, loopback, and link-local addresses.
func IsLocalIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range privateCIDRs {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	// chi's middleware.RealIP (applied in main.go) normalises RemoteAddr based on
	// X-Forwarded-For / X-Real-IP. Accept RemoteAddr as host:port or bare host.
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host
}
