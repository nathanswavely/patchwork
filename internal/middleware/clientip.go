package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// maxIPKeyLen bounds the length of any string derived from client-controlled
// input before it is used as a map key (rate limiter), stored on a session
// row, or written to the audit log. A valid IPv6 address with a zone is
// comfortably under this.
const maxIPKeyLen = 64

// DefaultTrustedProxies covers loopback plus the private ranges used by
// Docker bridge networks, matching the bundled compose topology where Caddy
// reaches the app over a private network. A deployment that terminates TLS
// somewhere else should set server.trusted_proxies explicitly.
var DefaultTrustedProxies = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

// trustedProxies holds the parsed CIDR set. Read on every request, written
// once at startup, so it is stored atomically rather than behind a mutex.
var trustedProxies atomic.Pointer[[]*net.IPNet]

func init() {
	nets, err := parseCIDRs(DefaultTrustedProxies)
	if err != nil {
		panic("middleware: bad DefaultTrustedProxies: " + err.Error())
	}
	trustedProxies.Store(&nets)
}

func parseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// Accept a bare address as a single-host range.
		if !strings.Contains(c, "/") {
			ip := net.ParseIP(c)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy %q", c)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			c = fmt.Sprintf("%s/%d", ip.String(), bits)
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", c, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// SetTrustedProxies replaces the trusted proxy set. An empty list restores
// DefaultTrustedProxies. Call once during startup, before serving.
func SetTrustedProxies(cidrs []string) error {
	if len(cidrs) == 0 {
		cidrs = DefaultTrustedProxies
	}
	nets, err := parseCIDRs(cidrs)
	if err != nil {
		return err
	}
	trustedProxies.Store(&nets)
	return nil
}

func isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range *trustedProxies.Load() {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the client IP address for a request.
//
// X-Forwarded-For is honoured only when the request's transport-level peer is
// a trusted proxy; otherwise the header is entirely client-controlled and is
// ignored. Within a trusted request we take the RIGHTMOST comma-separated
// entry, because Caddy APPENDS the peer address by default — so a client that
// pre-seeds the header cannot displace the value the proxy itself wrote. (The
// bundled Caddyfile additionally overwrites the header, making this belt and
// braces.) Any entry that fails net.ParseIP is discarded and we fall back to
// the peer address, which is not forgeable. The result is length-capped
// because it becomes a long-lived rate-limiter map key.
func ClientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	peerIP := net.ParseIP(peer)

	if isTrustedProxy(peerIP) {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				if candidate == "" {
					continue
				}
				if ip := net.ParseIP(candidate); ip != nil {
					return ip.String()
				}
				// The rightmost non-empty entry is not an IP: the header is
				// malformed or forged. Do not walk further left into
				// attacker-controlled territory.
				break
			}
		}
	}

	if peerIP != nil {
		return peerIP.String()
	}
	return truncateKey(peer)
}

func truncateKey(s string) string {
	if len(s) > maxIPKeyLen {
		return s[:maxIPKeyLen]
	}
	return s
}
