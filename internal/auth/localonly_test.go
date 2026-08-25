package auth

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureWarn installs a temporary slog handler for the duration of the test
// and returns the buffer it writes to.
func captureWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestLocalOnlyWithoutTrustedProxy covers the whole mode/CIDR matrix. Only
// local-only with an empty trusted-proxy list is the reported combination:
// that is the shape where a reverse proxy in front of Bindery makes every
// forwarded request resolve as a trusted local admin.
func TestLocalOnlyWithoutTrustedProxy(t *testing.T) {
	withProxy := ParseTrustedProxyCIDRs("172.20.0.0/16")
	if len(withProxy) != 1 {
		t.Fatalf("fixture parse failed: got %d CIDRs", len(withProxy))
	}

	cases := []struct {
		name       string
		mode       Mode
		hasTrusted bool
		want       bool
	}{
		{"local-only without trusted proxy", ModeLocalOnly, false, true},
		{"local-only with trusted proxy", ModeLocalOnly, true, false},
		{"enabled without trusted proxy", ModeEnabled, false, false},
		{"enabled with trusted proxy", ModeEnabled, true, false},
		{"disabled without trusted proxy", ModeDisabled, false, false},
		{"disabled with trusted proxy", ModeDisabled, true, false},
		{"proxy without trusted proxy", ModeProxy, false, false},
		{"proxy with trusted proxy", ModeProxy, true, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			trusted := withProxy
			if !c.hasTrusted {
				trusted = nil
			}
			if got := LocalOnlyWithoutTrustedProxy(c.mode, trusted); got != c.want {
				t.Errorf("LocalOnlyWithoutTrustedProxy(%q, %v) = %v, want %v", c.mode, trusted, got, c.want)
			}

			buf := captureWarn(t)
			if got := WarnIfLocalOnlyWithoutTrustedProxy(c.mode, trusted); got != c.want {
				t.Errorf("WarnIfLocalOnlyWithoutTrustedProxy(%q, %v) = %v, want %v", c.mode, trusted, got, c.want)
			}
			logged := strings.Contains(buf.String(), "BINDERY_TRUSTED_PROXY is empty")
			if logged != c.want {
				t.Errorf("warning logged = %v, want %v (log: %q)", logged, c.want, buf.String())
			}
		})
	}
}

// TestLocalOnlyNoTrustedProxyWarningText pins the parts of the message an
// operator needs: what is misconfigured, what the consequence is, and which
// environment variable fixes it.
func TestLocalOnlyNoTrustedProxyWarningText(t *testing.T) {
	for _, want := range []string{
		"local-only",
		"BINDERY_TRUSTED_PROXY",
		"trusted local admin",
	} {
		if !strings.Contains(LocalOnlyNoTrustedProxyWarning, want) {
			t.Errorf("warning text is missing %q: %s", want, LocalOnlyNoTrustedProxyWarning)
		}
	}
}
