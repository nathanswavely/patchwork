package middleware

import (
	"net/http"
	"strings"
	"testing"
)

// A peer inside DefaultTrustedProxies (the Docker bridge network Caddy uses).
const trustedPeer = "172.18.0.5:41234"

// A peer outside it — what a directly-exposed app sees from the internet.
const untrustedPeer = "198.51.100.22:41234"

func req(remoteAddr, xff string) *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"trusted, no header", trustedPeer, "", "172.18.0.5"},
		{"trusted, proxy-written single entry", trustedPeer, "203.0.113.7", "203.0.113.7"},
		{"trusted, appended: rightmost wins", trustedPeer, "1.2.3.4, 203.0.113.7", "203.0.113.7"},
		{"trusted, junk rightmost rejected", trustedPeer, "203.0.113.7, evil", "172.18.0.5"},
		{"trusted, all junk rejected", trustedPeer, "evil", "172.18.0.5"},
		{"trusted, empty entries skipped", trustedPeer, ",  , 203.0.113.7", "203.0.113.7"},
		{"trusted, ipv6 forwarded", trustedPeer, "2001:db8::2", "2001:db8::2"},
		{"trusted, ipv6 normalised", trustedPeer, "2001:0db8:0000::0002", "2001:db8::2"},

		{"untrusted peer ignores header", untrustedPeer, "203.0.113.7", "198.51.100.22"},
		{"untrusted peer ignores appended header", untrustedPeer, "1.2.3.4, 5.6.7.8", "198.51.100.22"},

		{"loopback is trusted", "127.0.0.1:9999", "203.0.113.7", "203.0.113.7"},
		{"ipv6 peer, no header", "[2001:db8::1]:41234", "", "2001:db8::1"},
		{"remoteaddr without port", "172.18.0.5", "", "172.18.0.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClientIP(req(tt.remoteAddr, tt.xff)); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

var spoofPayloads = []string{
	"1.2.3.4",
	"9.9.9.9, 8.8.8.8",
	"not-an-ip",
	"'; DROP TABLE sessions; --",
	strings.Repeat("a", 8000),
	strings.Repeat("1.2.3.4, ", 500) + "5.6.7.8",
	"203.0.113.7\n203.0.113.8",
}

// The core regression test for a directly-exposed deployment: a forged
// X-Forwarded-For must not change the rate-limiter key, or every distinct
// forgery pins its own limiter for an hour.
func TestSpoofedXFFFromUntrustedPeerDoesNotChangeKey(t *testing.T) {
	baseline := ClientIP(req(untrustedPeer, ""))
	if baseline != "198.51.100.22" {
		t.Fatalf("baseline = %q", baseline)
	}

	for _, spoof := range spoofPayloads {
		if got := ClientIP(req(untrustedPeer, spoof)); got != baseline {
			t.Errorf("spoof %.40q changed key: got %q, want %q", spoof, got, baseline)
		}
	}
}

// And behind the real proxy: whatever the client pre-seeds, the value Caddy
// appends is rightmost and therefore wins. The client cannot mint keys.
func TestSpoofedXFFBehindProxyDoesNotChangeKey(t *testing.T) {
	const realIP = "203.0.113.7"

	for _, spoof := range spoofPayloads {
		// Simulate Caddy's default append behaviour.
		appended := spoof + ", " + realIP
		if got := ClientIP(req(trustedPeer, appended)); got != realIP {
			t.Errorf("spoof %.40q changed key: got %q, want %q", spoof, got, realIP)
		}

		// And with the Caddyfile's header_up overwrite, the client's value
		// never reaches the app at all.
		if got := ClientIP(req(trustedPeer, realIP)); got != realIP {
			t.Errorf("overwrite case: got %q, want %q", got, realIP)
		}
	}
}

// A key derived from client input must never be unbounded in length, since it
// is retained as a rate-limiter map key for up to an hour.
func TestClientIPKeyIsBounded(t *testing.T) {
	huge := strings.Repeat("x", 100_000)
	for _, r := range []*http.Request{
		req(trustedPeer, huge),
		req(untrustedPeer, huge),
		req(huge, huge),
	} {
		if got := ClientIP(r); len(got) > maxIPKeyLen {
			t.Errorf("key length %d exceeds cap %d", len(got), maxIPKeyLen)
		}
	}
}

func TestSetTrustedProxies(t *testing.T) {
	t.Cleanup(func() { SetTrustedProxies(nil) })

	if err := SetTrustedProxies([]string{"198.51.100.22"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	// Bare address is treated as a single host, and is now trusted.
	if got := ClientIP(req(untrustedPeer, "203.0.113.7")); got != "203.0.113.7" {
		t.Errorf("newly trusted peer: got %q", got)
	}
	// The formerly-trusted Docker range no longer is.
	if got := ClientIP(req(trustedPeer, "203.0.113.7")); got != "172.18.0.5" {
		t.Errorf("no-longer-trusted peer: got %q", got)
	}

	// Empty restores defaults.
	if err := SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies(nil): %v", err)
	}
	if got := ClientIP(req(trustedPeer, "203.0.113.7")); got != "203.0.113.7" {
		t.Errorf("after restore: got %q", got)
	}

	if err := SetTrustedProxies([]string{"garbage"}); err == nil {
		t.Error("expected error for invalid CIDR")
	}
}
