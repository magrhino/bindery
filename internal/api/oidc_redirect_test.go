package api

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("parse cidr %q: %v", cidr, err)
	}
	return n
}

func TestResolveOIDCRedirectBase_ConfiguredWins(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://internal.cluster.svc/api/v1/auth/oidc/x/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "bindery.example.com")
	r.RemoteAddr = "10.0.0.5:54321"

	got := ResolveOIDCRedirectBase(r, "https://override.example.com", []*net.IPNet{mustCIDR(t, "10.0.0.0/8")})
	if got != "https://override.example.com" {
		t.Fatalf("configured value should win, got %q", got)
	}
}

func TestResolveOIDCRedirectBase_TrustedProxyForwarded(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/x/login", nil)
	r.Host = "internal.cluster.svc"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "bindery.example.com")
	r.RemoteAddr = "10.0.0.5:54321"

	got := ResolveOIDCRedirectBase(r, "", []*net.IPNet{mustCIDR(t, "10.0.0.0/8")})
	if got != "https://bindery.example.com" {
		t.Fatalf("trusted-proxy forwarded headers should be used, got %q", got)
	}
}

func TestResolveOIDCRedirectBase_UntrustedPeerIgnoresForwarded(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/x/login", nil)
	r.Host = "internal.cluster.svc"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "evil.example.com")
	r.RemoteAddr = "203.0.113.7:443"

	got := ResolveOIDCRedirectBase(r, "", []*net.IPNet{mustCIDR(t, "10.0.0.0/8")})
	if got != "https://internal.cluster.svc" {
		t.Fatalf("forwarded headers from untrusted peer must be ignored, got %q", got)
	}
}

func TestResolveOIDCRedirectBase_NoForwardedHeadersFallsBackToHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/x/login", nil)
	r.Host = "bindery.example.com"
	r.RemoteAddr = "10.0.0.5:54321"

	got := ResolveOIDCRedirectBase(r, "", []*net.IPNet{mustCIDR(t, "10.0.0.0/8")})
	if got != "http://bindery.example.com" {
		t.Fatalf("trusted peer without XF headers should fall back to Host, got %q", got)
	}
}

func TestResolveOIDCRedirectBase_HTTPSDetection(t *testing.T) {
	cases := []struct {
		name string
		set  func(*http.Request)
		want string
	}{
		{
			name: "TLS connection",
			set: func(r *http.Request) {
				r.Host = "bindery.example.com"
				r.TLS = &tls.ConnectionState{}
			},
			want: "https://bindery.example.com",
		},
		{
			name: "X-Forwarded-Proto=https without trusted proxy",
			set: func(r *http.Request) {
				r.Host = "bindery.example.com"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			want: "https://bindery.example.com",
		},
		{
			name: "plain HTTP",
			set:  func(r *http.Request) { r.Host = "bindery.example.com" },
			want: "http://bindery.example.com",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			c.set(r)
			if got := ResolveOIDCRedirectBase(r, "", nil); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveOIDCRedirectBase_StripsTrailingSlashes(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "bindery.example.com"

	got := ResolveOIDCRedirectBase(r, "https://override.example.com/", nil)
	if got != "https://override.example.com" {
		t.Fatalf("trailing slash on configured value should be stripped, got %q", got)
	}

	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "bindery.example.com/")
	r.RemoteAddr = "10.0.0.5:54321"
	got = ResolveOIDCRedirectBase(r, "", []*net.IPNet{mustCIDR(t, "10.0.0.0/8")})
	if got != "https://bindery.example.com" {
		t.Fatalf("trailing slash on X-Forwarded-Host should be stripped, got %q", got)
	}
}
