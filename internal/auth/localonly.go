package auth

import (
	"log/slog"
	"net"
)

// LocalOnlyNoTrustedProxyWarning is the operator-facing message emitted when
// local-only auth mode is active with no trusted proxy configured.
//
// In local-only mode the middleware grants admin to any request whose resolved
// client IP is private (see IsLocalRequestTrusted). ResolveClientIP only
// consults X-Forwarded-For for peers listed in BINDERY_TRUSTED_PROXY, so with
// that list empty the trust decision is made on the raw TCP peer. Directly on a
// LAN that is exactly right. Behind a reverse proxy or a Kubernetes ingress the
// TCP peer is the proxy itself, which sits on a private container network, so
// every request forwarded by the proxy looks local no matter where it came
// from.
const LocalOnlyNoTrustedProxyWarning = "local-only auth mode is active but BINDERY_TRUSTED_PROXY is empty: " +
	"if Bindery is behind a reverse proxy it cannot tell a proxied public request from a genuine LAN client, " +
	"so every proxied request is treated as a trusted local admin. " +
	"Set BINDERY_TRUSTED_PROXY to your proxy's IP or CIDR, or use auth mode 'enabled' when Bindery is reachable from the internet."

// LocalOnlyWithoutTrustedProxy reports whether the effective auth mode and the
// parsed trusted-proxy CIDR list form the combination described by
// LocalOnlyNoTrustedProxyWarning. It is the single source of truth for that
// check so the boot path and the runtime mode-change path stay in agreement.
//
// It deliberately does not try to detect whether a proxy is actually in front
// of Bindery: nothing observable at startup answers that. A direct-to-LAN
// install is a legitimate deployment, which is why this is a warning and not a
// startup gate like the one proxy mode carries.
func LocalOnlyWithoutTrustedProxy(mode Mode, trusted []*net.IPNet) bool {
	return mode == ModeLocalOnly && len(trusted) == 0
}

// WarnIfLocalOnlyWithoutTrustedProxy logs LocalOnlyNoTrustedProxyWarning when
// LocalOnlyWithoutTrustedProxy holds, and reports whether it warned so callers
// can pass the same text on to an operator through another channel.
func WarnIfLocalOnlyWithoutTrustedProxy(mode Mode, trusted []*net.IPNet) bool {
	if !LocalOnlyWithoutTrustedProxy(mode, trusted) {
		return false
	}
	slog.Warn(LocalOnlyNoTrustedProxyWarning, "mode", string(mode), "env", "BINDERY_TRUSTED_PROXY")
	return true
}
