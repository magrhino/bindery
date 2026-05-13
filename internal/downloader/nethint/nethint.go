// Package nethint classifies network errors and returns a short contextual
// hint string to append to "could not reach …" messages shown to users.
// The hints target the most common failure modes in Docker deployments.
package nethint

import (
	"errors"
	"net"
	"syscall"
)

// ForErr inspects err and returns a hint string (with a leading space) that
// can be appended directly to an error message. Returns an empty string when
// the error does not match any known class.
//
// Classification priority (first match wins):
//  1. net.DNSError with IsNotFound==true → Docker DNS hint
//  2. syscall.ECONNREFUSED (or wrapped) → port-not-open hint
//  3. net.Error with Timeout()==true → firewall/proxy hint
func ForErr(err error) string {
	if err == nil {
		return ""
	}

	// 1. DNS lookup failure — most likely a container name that isn't on the
	//    same Docker network as Bindery.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return " (if using a container name, ensure both services are on the same Docker network)"
	}

	// 2. Connection refused — service is not listening on that port.
	if errors.Is(err, syscall.ECONNREFUSED) {
		return " (service may not be running on that port)"
	}

	// 3. Timeout — host is reachable but not responding (firewall, proxy, etc.).
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return " (host is reachable but not responding — check firewall or proxy)"
	}

	return ""
}
