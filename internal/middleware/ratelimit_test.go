package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestUnauthedAuthRateLimitBlocksBurst(t *testing.T) {
	// Isolate from the package-level limiters so ordering with other tests
	// cannot affect the budget.
	origIP, origGlobal := unauthedAuthIPLimiter, unauthedAuthGlobalLimiter
	t.Cleanup(func() { unauthedAuthIPLimiter, unauthedAuthGlobalLimiter = origIP, origGlobal })
	unauthedAuthIPLimiter = NewRateLimiterStore(rate.Every(time.Hour), 5)
	unauthedAuthGlobalLimiter = NewRateLimiterStore(rate.Every(time.Hour), 1000)

	var served int
	h := UnauthedAuthRateLimit(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})

	var limited int
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		h(rec, req(untrustedPeer, ""))
		if rec.Code == http.StatusTooManyRequests {
			limited++
			if rec.Header().Get("Retry-After") == "" {
				t.Error("429 response missing Retry-After")
			}
		}
	}

	if served != 5 {
		t.Errorf("served %d requests, want 5 (the burst)", served)
	}
	if limited != 15 {
		t.Errorf("limited %d requests, want 15", limited)
	}
}

// A separate address gets its own budget — the limit is per client, not global
// at this tier.
func TestUnauthedAuthRateLimitIsPerIP(t *testing.T) {
	origIP, origGlobal := unauthedAuthIPLimiter, unauthedAuthGlobalLimiter
	t.Cleanup(func() { unauthedAuthIPLimiter, unauthedAuthGlobalLimiter = origIP, origGlobal })
	unauthedAuthIPLimiter = NewRateLimiterStore(rate.Every(time.Hour), 2)
	unauthedAuthGlobalLimiter = NewRateLimiterStore(rate.Every(time.Hour), 1000)

	h := UnauthedAuthRateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	drain := func(peer string) {
		for i := 0; i < 2; i++ {
			h(httptest.NewRecorder(), req(peer, ""))
		}
	}

	drain("198.51.100.1:1000")
	rec := httptest.NewRecorder()
	h(rec, req("198.51.100.1:1000", ""))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("exhausted IP: code = %d, want 429", rec.Code)
	}

	rec = httptest.NewRecorder()
	h(rec, req("198.51.100.2:1000", ""))
	if rec.Code != http.StatusOK {
		t.Errorf("fresh IP: code = %d, want 200", rec.Code)
	}
}

// This is the link between the two fixes: without the Part 1 clientIP fix, a
// forged X-Forwarded-For would mint a fresh limiter bucket per request and
// make this throttle bypassable.
func TestUnauthedAuthRateLimitNotBypassableViaXFF(t *testing.T) {
	origIP, origGlobal := unauthedAuthIPLimiter, unauthedAuthGlobalLimiter
	t.Cleanup(func() { unauthedAuthIPLimiter, unauthedAuthGlobalLimiter = origIP, origGlobal })
	unauthedAuthIPLimiter = NewRateLimiterStore(rate.Every(time.Hour), 3)
	unauthedAuthGlobalLimiter = NewRateLimiterStore(rate.Every(time.Hour), 1000)

	var served int
	h := UnauthedAuthRateLimit(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})

	// Every request carries a distinct forged header from the same peer.
	for i := 0; i < 50; i++ {
		spoof := strings.Repeat("9", i%5+1) + ".1.1.1"
		h(httptest.NewRecorder(), req(untrustedPeer, spoof))
	}

	if served != 3 {
		t.Errorf("served %d requests despite forged XFF, want 3 (the burst)", served)
	}
}

// The global ceiling catches a source distributed across many addresses.
func TestUnauthedAuthRateLimitGlobalCeiling(t *testing.T) {
	origIP, origGlobal := unauthedAuthIPLimiter, unauthedAuthGlobalLimiter
	t.Cleanup(func() { unauthedAuthIPLimiter, unauthedAuthGlobalLimiter = origIP, origGlobal })
	unauthedAuthIPLimiter = NewRateLimiterStore(rate.Every(time.Hour), 1000)
	unauthedAuthGlobalLimiter = NewRateLimiterStore(rate.Every(time.Hour), 10)

	var served int
	h := UnauthedAuthRateLimit(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})

	// 100 distinct addresses, one request each: per-IP never trips.
	for i := 0; i < 100; i++ {
		peer := fmt.Sprintf("198.51.100.%d:1000", i%254+1)
		h(httptest.NewRecorder(), req(peer, ""))
	}

	if served != 10 {
		t.Errorf("served %d requests across many IPs, want 10 (global burst)", served)
	}
}
