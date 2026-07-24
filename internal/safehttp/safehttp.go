// Package safehttp provides the SSRF-guarded HTTP client used for every
// outbound fetch of a URL that someone outside the instance can influence:
// ActivityPub actor documents (inbound keyIds, user-supplied remote
// follows) and event source feeds (admin-supplied calendar URLs). Fetching
// such a URL must never become a probe of the host's own network.
package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// allowPrivateAddresses disables the guard below. Off in production; a
// seam for tests and for deliberately federating loopback dev instances.
var allowPrivateAddresses = false

// SetAllowPrivateAddresses toggles the private-address guard and returns
// the previous value so callers can restore it.
func SetAllowPrivateAddresses(v bool) bool {
	prev := allowPrivateAddresses
	allowPrivateAddresses = v
	return prev
}

// isPublicIP reports whether ip is a plausible public unicast address.
// Loopback, RFC1918/ULA, link-local, multicast, and unspecified addresses
// are all refused.
func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified())
}

// GuardedDialContext dials like net.Dialer but refuses non-public
// addresses at connect time — after DNS resolution, so DNS-rebinding to a
// private IP is caught, and inside the transport, so redirects are
// covered by the same check.
func GuardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("ssrf guard: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("ssrf guard: unparseable address %q", host)
			}
			if !allowPrivateAddresses && !isPublicIP(ip) {
				return fmt.Errorf("ssrf guard: refusing non-public address %s", ip)
			}
			return nil
		},
	}
	return d.DialContext(ctx, network, addr)
}

// NewClient returns an HTTP client whose every connection — including
// ones reached via redirect — passes the private-address guard.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: GuardedDialContext},
	}
}
