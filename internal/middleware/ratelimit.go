package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiterEntry holds a limiter and its last access time for cleanup.
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiterStore manages per-key rate limiters with periodic cleanup.
type RateLimiterStore struct {
	mu       sync.Mutex
	limiters sync.Map
	r        rate.Limit
	burst    int
}

// NewRateLimiterStore creates a store with the given rate (events/second) and burst.
func NewRateLimiterStore(r rate.Limit, burst int) *RateLimiterStore {
	s := &RateLimiterStore{r: r, burst: burst}
	go s.cleanup()
	return s
}

func (s *RateLimiterStore) getLimiter(key string) *rate.Limiter {
	val, ok := s.limiters.Load(key)
	if ok {
		entry := val.(*rateLimiterEntry)
		entry.lastSeen = time.Now()
		return entry.limiter
	}
	entry := &rateLimiterEntry{
		limiter:  rate.NewLimiter(s.r, s.burst),
		lastSeen: time.Now(),
	}
	actual, _ := s.limiters.LoadOrStore(key, entry)
	return actual.(*rateLimiterEntry).limiter
}

func (s *RateLimiterStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-1 * time.Hour)
		s.limiters.Range(func(key, value any) bool {
			entry := value.(*rateLimiterEntry)
			if entry.lastSeen.Before(cutoff) {
				s.limiters.Delete(key)
			}
			return true
		})
	}
}

// Allow returns true if the request for the given key is allowed.
func (s *RateLimiterStore) Allow(key string) bool {
	return s.getLimiter(key).Allow()
}

// Per-endpoint rate limiter factories.
var (
	// MagicLink: 3 per email/hour + 10 per IP/hour.
	magicLinkEmailLimiter = NewRateLimiterStore(rate.Every(20*time.Minute), 3)
	magicLinkIPLimiter    = NewRateLimiterStore(rate.Every(6*time.Minute), 10)

	// InviteGeneration: 20 per admin/hour.
	inviteGenerationLimiter = NewRateLimiterStore(rate.Every(3*time.Minute), 20)

	// UnauthedAuth guards the unauthenticated auth endpoints (invite redeem
	// and validate, signup and validate, WebAuthn login begin and finish).
	// These are not brute-force paths — every token is crypto/rand-backed —
	// but each request converts into retained server memory, most sharply in
	// WebAuthn login begin, which stores a challenge for five minutes. The
	// budget is generous enough that a real person fumbling a passkey never
	// notices: 30 requests up front, refilling at one every two seconds.
	unauthedAuthIPLimiter = NewRateLimiterStore(rate.Every(2*time.Second), 30)

	// A coarse instance-wide ceiling, so a distributed source cannot bypass
	// the per-IP budget by spreading across addresses. 25/sec sustained with
	// a 250 burst is far above organic use of these six routes.
	unauthedAuthGlobalLimiter = NewRateLimiterStore(rate.Every(40*time.Millisecond), 250)

	// RecoveryRedeem is tighter than the other unauthed limits because
	// recovery codes carry ~59 bits, not 256: a real person mistyping a
	// code off paper gets 5 tries then one every 2 minutes per account,
	// while a guesser gets nowhere near the keyspace.
	recoveryRedeemLimiter = NewRateLimiterStore(rate.Every(2*time.Minute), 5)
)


// CheckMagicLinkRate checks rate limits for magic link. Returns error message if limited.
func CheckMagicLinkRate(email, ip string) error {
	if !magicLinkEmailLimiter.Allow("email:" + email) {
		return fmt.Errorf("rate limit exceeded for email")
	}
	if !magicLinkIPLimiter.Allow("ip:" + ip) {
		return fmt.Errorf("rate limit exceeded for IP")
	}
	return nil
}

// UnauthedAuthRateLimit throttles an unauthenticated auth route per client IP
// and instance-wide. The client IP comes from ClientIP, so it cannot be forged
// into a fresh bucket by way of X-Forwarded-For.
func UnauthedAuthRateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !unauthedAuthIPLimiter.Allow("auth:" + ClientIP(r)) {
			tooManyRequests(w)
			return
		}
		if !unauthedAuthGlobalLimiter.Allow("auth:global") {
			tooManyRequests(w)
			return
		}
		next(w, r)
	}
}

func tooManyRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"error":"rate limit exceeded"}`))
}

// CheckRecoveryRedeemRate limits recovery-code redemption per target account
// and per source IP, so neither one account nor one address can be ground
// against the code space.
func CheckRecoveryRedeemRate(username, ip string) error {
	if !recoveryRedeemLimiter.Allow("user:" + username) {
		return fmt.Errorf("rate limit exceeded")
	}
	if !recoveryRedeemLimiter.Allow("ip:" + ip) {
		return fmt.Errorf("rate limit exceeded")
	}
	return nil
}

// CheckInviteGenerationRate checks rate limits for invite generation. Returns error if limited.
func CheckInviteGenerationRate(adminID string) error {
	if !inviteGenerationLimiter.Allow("admin:" + adminID) {
		return fmt.Errorf("rate limit exceeded")
	}
	return nil
}
