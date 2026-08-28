package handler

// Claiming an unclaimed patch (docs/adr/030). A claim is an assertion of
// ownership pending proof, never a reservation: claims on the same patch run
// concurrently, one open claim per user per patch, first proof wins. All
// self-service verification (dns, meta_tag, email) anchors on the node's
// verification_domain — a domain vetted through admin/trusted paths — never
// on the cosmetic website field.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/atproto"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// setupWindow is how long a verified or approved claim remains a valid,
// unused right to enter patch setup before it lapses and the patch becomes
// claimable again (docs/adr/039).
const setupWindow = 14 * 24 * time.Hour

// External lookups used by claim verification, swappable in tests.
// ClaimHTTPClient is SSRF-guarded: meta_tag verification fetches a page on
// the claimed domain, a URL someone outside the instance influences.
var (
	ClaimLookupTXT  func(domain string) ([]string, error)                = net.LookupTXT
	ClaimHTTPClient                                                      = safehttp.NewClient(10 * time.Second)
	ClaimSendMail   func(cfg config.SMTP, to []string, msg []byte) error = mail.Send
)

const (
	claimEmailTokenTTL   = 24 * time.Hour
	claimEmailSendLimit  = 3
	claimEmailSendWindow = 24 * time.Hour
)

// sharedPlatformDomains can never anchor ownership proof: controlling a
// mailbox or page on a shared platform proves nothing about the org. A
// small org's "website" is often a Facebook page — auto-derivation must
// refuse these. Matched with subdomains (myband.bandcamp.com counts).
var sharedPlatformDomains = []string{
	// mail providers
	"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com",
	"msn.com", "yahoo.com", "aol.com", "icloud.com", "me.com", "mac.com",
	"proton.me", "protonmail.com", "pm.me", "zoho.com", "mail.com",
	"gmx.com", "gmx.net", "yandex.com", "fastmail.com",
	// social + link-in-bio
	"facebook.com", "fb.com", "instagram.com", "threads.net", "twitter.com",
	"x.com", "tiktok.com", "youtube.com", "youtu.be", "linkedin.com",
	"linktr.ee", "beacons.ai", "carrd.co", "bio.link", "tumblr.com",
	"discord.gg", "discord.com", "t.me", "bsky.app",
	// music/creator platforms
	"bandcamp.com", "soundcloud.com", "spotify.com", "patreon.com",
	"substack.com", "medium.com", "eventbrite.com", "meetup.com",
	// site builders' shared hosts
	"wordpress.com", "blogspot.com", "wixsite.com", "weebly.com",
	"squarespace.com", "godaddysites.com", "github.io", "gitlab.io",
	"netlify.app", "vercel.app", "pages.dev", "neocities.org",
	// shorteners
	"bit.ly", "tinyurl.com",
}

// normalizeDomain reduces a URL or bare host to a lowercase hostname:
// scheme, path, port, and a leading www. are stripped.
func normalizeDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	host = strings.TrimPrefix(host, "www.")
	if !strings.Contains(host, ".") {
		return "" // "localhost" and friends can't anchor anything
	}
	return host
}

func isSharedPlatformDomain(domain string) bool {
	for _, blocked := range sharedPlatformDomains {
		if domain == blocked || strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
}

// deriveVerificationDomain derives a trust anchor from a website URL supplied
// through an admin/trusted path. Shared platforms yield "" — no anchor.
func deriveVerificationDomain(website string) string {
	domain := normalizeDomain(website)
	if domain == "" || isSharedPlatformDomain(domain) {
		return ""
	}
	return domain
}

// validateExplicitDomain checks a domain an admin typed in directly.
// Empty is valid (it clears the anchor).
func validateExplicitDomain(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	domain := normalizeDomain(raw)
	if domain == "" {
		return "", fmt.Errorf("not a valid domain")
	}
	if isSharedPlatformDomain(domain) {
		return "", fmt.Errorf("shared platforms like %s cannot anchor ownership verification", domain)
	}
	return domain, nil
}

// BackfillVerificationDomains runs once at startup: unclaimed patches created
// through admin paths before migration 031 get their verification_domain
// derived from their website. NULL means "never processed" — after this pass
// the row holds either a domain or '' and is never touched again, so an
// admin clearing the field later sticks.
func BackfillVerificationDomains(db *database.DB) {
	rows, err := db.Query(
		`SELECT id, COALESCE(website,'') FROM nodes
		 WHERE verification_domain IS NULL AND status = 'unclaimed'
		   AND submission_source IN ('admin','agent')`,
	)
	if err != nil {
		log.Printf("claims: verification domain backfill query: %v", err)
		return
	}
	type row struct{ id, website string }
	var pending []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.website) == nil {
			pending = append(pending, r)
		}
	}
	rows.Close()

	for _, r := range pending {
		db.Exec("UPDATE nodes SET verification_domain = ? WHERE id = ?", deriveVerificationDomain(r.website), r.id)
	}
	if len(pending) > 0 {
		log.Printf("claims: backfilled verification domains for %d unclaimed patches", len(pending))
	}
}

// claimResolver wires the atproto resolver to the same swappable seams the
// other claim methods use, so a test that stubs DNS stubs this too.
func claimResolver() atproto.Resolver {
	return atproto.Resolver{
		LookupTXT: func(domain string) ([]string, error) { return ClaimLookupTXT(domain) },
		Get:       ClaimHTTPClient.Get,
	}
}

// claimMethodsFor reports which claim methods a patch currently supports.
func claimMethodsFor(verificationDomain string, cfg *config.Config) map[string]bool {
	hasDomain := verificationDomain != ""
	return map[string]bool{
		"dns":      hasDomain,
		"meta_tag": hasDomain,
		// docs/adr/062: the handle IS the vetted domain, so this needs no
		// anchor the other domain methods don't already have.
		"did":   hasDomain,
		"email": hasDomain && cfg.SMTP.Configured(),
		"admin": true,
	}
}

// claimInstructions builds the method-specific instruction fields shared by
// RequestClaim and MyClaim responses.
func claimInstructions(method, token, verificationDomain, email string, resp map[string]interface{}) {
	switch method {
	case "dns":
		resp["instructions"] = fmt.Sprintf("Add a TXT record on %s with the value: patchwork-verify=%s", verificationDomain, token)
		resp["record_value"] = "patchwork-verify=" + token
	case "meta_tag":
		resp["instructions"] = fmt.Sprintf(`Add this tag to the <head> of https://%s: <meta name="patchwork-verify" content="%s">`, verificationDomain, token)
		resp["meta_content"] = token
	case "did":
		resp["instructions"] = fmt.Sprintf("Point the atproto handle %s at a did:web identity, and have that DID document list at://%s in alsoKnownAs.", verificationDomain, verificationDomain)
	case "email":
		resp["instructions"] = fmt.Sprintf("We sent a verification link to %s. It expires in 24 hours.", email)
	case "admin":
		resp["instructions"] = "Your claim has been submitted for admin review. You'll be notified when it's resolved."
	}
}

// claimEmailURL builds the SPA link mailed (or logged) for email claims.
// Same shape as magicLinkURL: public domain when configured, localhost in dev.
func claimEmailURL(domain, port, token string) string {
	if domain != "" {
		return fmt.Sprintf("https://%s/claims/verify-email?token=%s", domain, token)
	}
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://localhost:%s/claims/verify-email?token=%s", port, token)
}

// sendClaimEmail delivers the verification link. Without SMTP the link is
// printed to the server log, mirroring magic links (the UI hides the email
// method on SMTP-less instances, so this path is dev and operators only).
func sendClaimEmail(cfg *config.Config, to, nodeName, token string) {
	link := claimEmailURL(cfg.Instance.Domain, cfg.Server.Port, token)

	if !cfg.SMTP.Configured() {
		log.Printf("\n\033[1;36m✉  Claim verification link for %s (%s):\033[0m\n   \033[4m%s\033[0m\n", to, nodeName, link)
		return
	}

	subject := fmt.Sprintf("Verify your claim of %s", nodeName)
	body := fmt.Sprintf(
		`<!DOCTYPE html><html><body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 560px; margin: 0 auto; padding: 20px;">`+
			`<h2 style="margin: 0 0 12px; font-size: 18px; color: #1a1a1a;">Verify your claim of %s</h2>`+
			`<p style="color: #444; font-size: 14px; line-height: 1.5;">Someone (hopefully you) is claiming this listing on %s. If that's you, confirm with the button below. The link expires in 24 hours.</p>`+
			`<p><a href="%s" style="display: inline-block; padding: 10px 20px; background: #5B21B6; color: #fff; text-decoration: none; border-radius: 4px; font-size: 14px;">Confirm claim</a></p>`+
			`<p style="font-size: 12px; color: #999;">If you didn't expect this email, you can ignore it — nothing happens without the confirmation.</p>`+
			`</body></html>`,
		escapeHTMLClaims(nodeName), escapeHTMLClaims(cfg.Instance.Name), link,
	)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\nMIME-Version: 1.0\r\n\r\n%s",
		cfg.SMTP.From, to, subject, body,
	)
	if err := ClaimSendMail(cfg.SMTP, []string{to}, []byte(msg)); err != nil {
		log.Printf("claims: verification email to %s failed: %v", to, err)
	}
}

func escapeHTMLClaims(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// expirePastDueApprovedClaims lazily moves any approved claim on a node
// whose setup window has passed to 'expired' (docs/adr/039). There is no
// standing worker for this — a claim only needs to be honest at the moments
// something reads or acts on it, so this runs inline wherever that happens
// (RequestClaim, MyClaim; SetupClaim does its own check so it can respond
// with the specific 410).
func expirePastDueApprovedClaims(db *database.DB, nodeID string) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec(
		`UPDATE claim_requests SET status = 'expired', updated_at = ?
		 WHERE node_id = ? AND status = 'approved' AND setup_expires_at IS NOT NULL AND setup_expires_at < ?`,
		now, nodeID, now,
	)
}

// RequestClaim handles POST /api/v1/nodes/{slug}/claim.
func RequestClaim(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		var nodeID, nodeStatus, nodeName, verificationDomain string
		err := db.QueryRow(
			"SELECT id, status, name, COALESCE(verification_domain,'') FROM nodes WHERE slug = ? AND removed_at IS NULL", slug,
		).Scan(&nodeID, &nodeStatus, &nodeName, &verificationDomain)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if nodeStatus != "unclaimed" {
			http.Error(w, `{"error":"this patch is not available for claiming"}`, http.StatusBadRequest)
			return
		}

		// An approved claim is a single-use right to enter setup, not a
		// reservation held forever — expire it first so a lapsed claim never
		// blocks a fresh one (docs/adr/039).
		expirePastDueApprovedClaims(db, nodeID)
		var approvedOpen int
		db.QueryRow("SELECT COUNT(*) FROM claim_requests WHERE node_id = ? AND status = 'approved'", nodeID).Scan(&approvedOpen)
		if approvedOpen > 0 {
			http.Error(w, `{"error":"this patch has an approved claim awaiting setup"}`, http.StatusConflict)
			return
		}

		// One open claim per user per patch — other people's claims never
		// block yours (docs/adr/030).
		var mine int
		db.QueryRow("SELECT COUNT(*) FROM claim_requests WHERE node_id = ? AND user_id = ? AND status = 'pending'", nodeID, user.ID).Scan(&mine)
		if mine > 0 {
			http.Error(w, `{"error":"you already have an open claim for this patch"}`, http.StatusConflict)
			return
		}

		var req struct {
			Method   string `json:"method"`   // dns, meta_tag, email, admin
			Evidence string `json:"evidence"` // for admin method
			Email    string `json:"email"`    // for email method
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		validMethods := map[string]bool{"dns": true, "meta_tag": true, "did": true, "email": true, "admin": true}
		if !validMethods[req.Method] {
			http.Error(w, `{"error":"method must be dns, meta_tag, did, email, or admin"}`, http.StatusBadRequest)
			return
		}

		// Self-service methods prove control of the vetted domain; without
		// one there is nothing to prove against.
		if req.Method != "admin" && verificationDomain == "" {
			http.Error(w, `{"error":"this patch has no verified domain — choose admin review"}`, http.StatusBadRequest)
			return
		}

		claimEmail := ""
		var emailExpiry interface{}
		if req.Method == "email" {
			// Same canonicalization and grammar as the sign-in path
			// (internal/auth/email.go), rather than a second inline
			// lowercase and an "@ is somewhere in the middle" test. The old
			// check leaned entirely on the domain comparison below to throw
			// out malformed input, which it did — but only by accident of
			// where the last '@' happened to fall.
			claimEmail = auth.NormalizeEmail(req.Email)
			if !auth.ValidEmail(claimEmail) {
				http.Error(w, `{"error":"a valid email address is required for email verification"}`, http.StatusBadRequest)
				return
			}
			// ValidEmail guarantees a bare parseable address, so there is an
			// '@' with something on both sides. The domain is what follows
			// the last one — the ownership anchor, and the only part that
			// proves anything.
			at := strings.LastIndex(claimEmail, "@")
			if claimEmail[at+1:] != verificationDomain {
				http.Error(w, fmt.Sprintf(`{"error":"the email must be at @%s"}`, verificationDomain), http.StatusBadRequest)
				return
			}
			emailExpiry = time.Now().Add(claimEmailTokenTTL).UTC().Format("2006-01-02T15:04:05.000Z")
		}

		tokenBytes := make([]byte, 16)
		rand.Read(tokenBytes)
		token := hex.EncodeToString(tokenBytes)

		id := auth.NewUUIDv7()
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

		sendCount := 0
		var windowStart interface{}
		if req.Method == "email" {
			sendCount = 1
			windowStart = now
		}

		_, err = db.Exec(
			`INSERT INTO claim_requests (id, node_id, user_id, method, evidence, verification_token, email, email_token_expires_at, email_send_count, email_window_start, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, user.ID, req.Method, req.Evidence, token, claimEmail, emailExpiry, sendCount, windowStart, now, now,
		)
		if err != nil {
			// The partial unique index catches a concurrent duplicate.
			if strings.Contains(err.Error(), "UNIQUE") {
				http.Error(w, `{"error":"you already have an open claim for this patch"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"failed to create claim"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.claim_requested", "node", nodeID, r.RemoteAddr, fmt.Sprintf(`{"method":"%s"}`, req.Method))

		notify(notifications.Event{
			Type:     notifications.AdminClaimRequest,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: id,
			Title:    "New claim request for: " + nodeName,
			Link:     "/admin/claims",
		})

		if req.Method == "email" {
			sendClaimEmail(cfg, claimEmail, nodeName, token)
		}

		resp := map[string]interface{}{
			"id":     id,
			"method": req.Method,
			"status": "pending",
		}
		claimInstructions(req.Method, token, verificationDomain, claimEmail, resp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// MyClaim handles GET /api/v1/nodes/{slug}/claims/mine.
// Returns the caller's open claim on this patch (with its verification
// instructions) plus which methods the patch currently supports — everything
// the claim page needs to survive a reload.
func MyClaim(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		var nodeID, nodeStatus, verificationDomain string
		err := db.QueryRow(
			"SELECT id, status, COALESCE(verification_domain,'') FROM nodes WHERE slug = ? AND removed_at IS NULL", slug,
		).Scan(&nodeID, &nodeStatus, &verificationDomain)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// A reload must never show a stale approved claim as still actionable.
		expirePastDueApprovedClaims(db, nodeID)

		resp := map[string]interface{}{
			"claim":               nil,
			"methods":             claimMethodsFor(verificationDomain, cfg),
			"verification_domain": verificationDomain,
			"node_status":         nodeStatus,
		}

		var c model.ClaimRequest
		var setupExpiresAt sql.NullString
		err = db.QueryRow(
			`SELECT id, method, evidence, status, verification_token, COALESCE(email,''), created_at, setup_expires_at
			 FROM claim_requests WHERE node_id = ? AND user_id = ? AND status IN ('pending','approved')
			 ORDER BY created_at DESC LIMIT 1`, nodeID, user.ID,
		).Scan(&c.ID, &c.Method, &c.Evidence, &c.Status, &c.VerificationToken, &c.Email, &c.CreatedAt, &setupExpiresAt)
		if err == nil {
			claim := map[string]interface{}{
				"id":         c.ID,
				"method":     c.Method,
				"evidence":   c.Evidence,
				"status":     c.Status,
				"email":      c.Email,
				"created_at": c.CreatedAt,
			}
			if setupExpiresAt.Valid {
				claim["setup_expires_at"] = setupExpiresAt.String
			}
			claimInstructions(c.Method, c.VerificationToken, verificationDomain, c.Email, claim)
			resp["claim"] = claim
		} else if err != sql.ErrNoRows {
			http.Error(w, `{"error":"failed to load claim"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// WithdrawClaim handles POST /api/v1/claims/{id}/withdraw.
// A claimant rescinds their own pending claim. Distinct from rejection:
// nobody reviewed anything (docs/adr/030).
func WithdrawClaim(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		claimID := r.PathValue("id")

		var claimUserID, claimStatus, nodeID string
		err := db.QueryRow(
			"SELECT user_id, status, node_id FROM claim_requests WHERE id = ?", claimID,
		).Scan(&claimUserID, &claimStatus, &nodeID)
		if err != nil {
			http.Error(w, `{"error":"claim not found"}`, http.StatusNotFound)
			return
		}
		if claimUserID != user.ID {
			http.Error(w, `{"error":"not your claim"}`, http.StatusForbidden)
			return
		}
		if claimStatus != "pending" {
			http.Error(w, fmt.Sprintf(`{"error":"claim is already %s"}`, claimStatus), http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		db.Exec("UPDATE claim_requests SET status = 'withdrawn', updated_at = ? WHERE id = ?", now, claimID)
		auth.LogAuditEvent(db, user.ID, "node.claim_withdrawn", "node", nodeID, r.RemoteAddr, "")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "withdrawn"})
	}
}

// ResendClaimEmail handles POST /api/v1/claims/{id}/resend-email.
// Re-sends the verification link with a fresh 24h expiry, limited to 3 sends
// per rolling 24h window per claim.
func ResendClaimEmail(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		claimID := r.PathValue("id")

		var claim struct {
			userID, status, method, token, email, nodeName string
			sendCount                                      int
			windowStart                                    sql.NullString
		}
		err := db.QueryRow(
			`SELECT cr.user_id, cr.status, cr.method, cr.verification_token, COALESCE(cr.email,''), cr.email_send_count, cr.email_window_start, n.name
			 FROM claim_requests cr JOIN nodes n ON cr.node_id = n.id
			 WHERE cr.id = ?`, claimID,
		).Scan(&claim.userID, &claim.status, &claim.method, &claim.token, &claim.email, &claim.sendCount, &claim.windowStart, &claim.nodeName)
		if err != nil {
			http.Error(w, `{"error":"claim not found"}`, http.StatusNotFound)
			return
		}
		if claim.userID != user.ID {
			http.Error(w, `{"error":"not your claim"}`, http.StatusForbidden)
			return
		}
		if claim.status != "pending" || claim.method != "email" {
			http.Error(w, `{"error":"this claim has no email verification to resend"}`, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		sendCount := claim.sendCount
		windowStart := now
		if claim.windowStart.Valid {
			if ws, err := time.Parse("2006-01-02T15:04:05.000Z", claim.windowStart.String); err == nil && now.Sub(ws) < claimEmailSendWindow {
				windowStart = ws
			} else {
				sendCount = 0
			}
		} else {
			sendCount = 0
		}
		if sendCount >= claimEmailSendLimit {
			http.Error(w, `{"error":"resend limit reached — try again tomorrow"}`, http.StatusTooManyRequests)
			return
		}

		nowStr := now.Format("2006-01-02T15:04:05.000Z")
		expiry := now.Add(claimEmailTokenTTL).Format("2006-01-02T15:04:05.000Z")
		db.Exec(
			`UPDATE claim_requests SET email_token_expires_at = ?, email_send_count = ?, email_window_start = ?, updated_at = ? WHERE id = ?`,
			expiry, sendCount+1, windowStart.Format("2006-01-02T15:04:05.000Z"), nowStr, claimID,
		)

		sendClaimEmail(cfg, claim.email, claim.nodeName, claim.token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
	}
}

// lookupEmailClaim finds a pending email claim by its token.
func lookupEmailClaim(db *database.DB, token string) (claimID, nodeID, userID, nodeName, slug, expiresAt string, ok bool) {
	if token == "" {
		return "", "", "", "", "", "", false
	}
	err := db.QueryRow(
		`SELECT cr.id, cr.node_id, cr.user_id, n.name, n.slug, COALESCE(cr.email_token_expires_at,'')
		 FROM claim_requests cr JOIN nodes n ON cr.node_id = n.id
		 WHERE cr.verification_token = ? AND cr.method = 'email' AND cr.status = 'pending'`, token,
	).Scan(&claimID, &nodeID, &userID, &nodeName, &slug, &expiresAt)
	return claimID, nodeID, userID, nodeName, slug, expiresAt, err == nil
}

func emailClaimExpired(expiresAt string) bool {
	exp, err := time.Parse("2006-01-02T15:04:05.000Z", expiresAt)
	return err != nil || time.Now().UTC().After(exp)
}

// EmailClaimInfo handles GET /api/v1/claims/verify-email?token=...
// The SPA link page uses this to show what's being confirmed. Read-only —
// mail scanners prefetch GETs, so nothing may change here.
func EmailClaimInfo(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, _, nodeName, slug, expiresAt, ok := lookupEmailClaim(db, r.URL.Query().Get("token"))
		if !ok {
			http.Error(w, `{"error":"invalid or already-used verification link"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node_name": nodeName,
			"slug":      slug,
			"expired":   emailClaimExpired(expiresAt),
		})
	}
}

// CompleteEmailClaim handles POST /api/v1/claims/verify-email with {token}.
// Possessing the link is the proof: no login required, and ownership
// transfers to the claimant's account regardless of who clicks.
func CompleteEmailClaim(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		claimID, _, userID, _, slug, expiresAt, ok := lookupEmailClaim(db, req.Token)
		if !ok {
			http.Error(w, `{"error":"invalid or already-used verification link"}`, http.StatusNotFound)
			return
		}
		if emailClaimExpired(expiresAt) {
			http.Error(w, `{"error":"this verification link has expired — request a new email from the claim page"}`, http.StatusBadRequest)
			return
		}

		if err := approveClaim(db, claimID, userID, r.RemoteAddr); err != nil {
			http.Error(w, `{"error":"failed to approve claim"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "approved",
			"slug":           slug,
			"setup_required": true,
		})
	}
}

// VerifyClaim handles POST /api/v1/claims/{id}/verify — the "check now"
// button for dns and meta_tag. Both prove control of the verification
// domain, never of the cosmetic website field.
func VerifyClaim(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		claimID := r.PathValue("id")

		var claim model.ClaimRequest
		var nodeSlug, verificationDomain string
		err := db.QueryRow(
			`SELECT cr.id, cr.node_id, cr.user_id, cr.method, cr.verification_token, cr.status, n.slug, COALESCE(n.verification_domain,'')
			 FROM claim_requests cr JOIN nodes n ON cr.node_id = n.id
			 WHERE cr.id = ?`, claimID,
		).Scan(&claim.ID, &claim.NodeID, &claim.UserID, &claim.Method, &claim.VerificationToken, &claim.Status, &nodeSlug, &verificationDomain)
		if err != nil {
			http.Error(w, `{"error":"claim not found"}`, http.StatusNotFound)
			return
		}

		if claim.UserID != user.ID {
			http.Error(w, `{"error":"not your claim"}`, http.StatusForbidden)
			return
		}
		if claim.Status != "pending" {
			http.Error(w, fmt.Sprintf(`{"error":"claim is already %s"}`, claim.Status), http.StatusBadRequest)
			return
		}

		verified := false
		var verifyError string
		var verifiedDID string

		switch claim.Method {
		case "dns":
			if verificationDomain == "" {
				verifyError = "this patch no longer has a verified domain"
				break
			}
			records, err := ClaimLookupTXT(verificationDomain)
			if err != nil {
				verifyError = "DNS lookup failed — make sure the TXT record is published"
				break
			}
			target := "patchwork-verify=" + claim.VerificationToken
			for _, rec := range records {
				if strings.TrimSpace(rec) == target {
					verified = true
					break
				}
			}
			if !verified {
				verifyError = "TXT record not found — it may take a few minutes to propagate"
			}

		case "meta_tag":
			if verificationDomain == "" {
				verifyError = "this patch no longer has a verified domain"
				break
			}
			body, err := fetchClaimPage("https://" + verificationDomain)
			if err != nil {
				body, err = fetchClaimPage("http://" + verificationDomain)
			}
			if err != nil {
				verifyError = "could not fetch https://" + verificationDomain
				break
			}
			if strings.Contains(body, claim.VerificationToken) {
				verified = true
			} else {
				verifyError = "verification tag not found on the site"
			}

		// docs/adr/062. Unlike the three above, this proves a binding rather
		// than possession of a token we issued: the handle must name the DID
		// and the DID document must name the handle back. Either direction
		// alone is forgeable.
		case "did":
			if verificationDomain == "" {
				verifyError = "this patch no longer has a verified domain"
				break
			}
			did, err := claimResolver().Verify(verificationDomain)
			switch {
			case errors.Is(err, atproto.ErrNotDIDWeb):
				// Named explicitly. "Verification failed" would send someone
				// hunting a DNS typo that isn't there.
				verifyError = "that handle resolves to a DID this instance does not accept — only did:web is, because it is served from your own domain"
			case err != nil:
				verifyError = "could not verify the handle: " + err.Error()
			default:
				verifiedDID = did
				verified = true
			}

		case "admin":
			verifyError = "admin claims are reviewed manually — you'll be notified"

		case "email":
			verifyError = "check your inbox — email claims are completed via the emailed link"
		}

		if verified {
			if err := approveClaim(db, claimID, user.ID, r.RemoteAddr); err != nil {
				http.Error(w, `{"error":"failed to approve claim"}`, http.StatusInternalServerError)
				return
			}
			// Recorded only on the path that proved it (docs/adr/062). A
			// failure here must not un-approve a claim that verified, so it
			// is logged rather than surfaced.
			if verifiedDID != "" {
				if _, err := db.Exec(`UPDATE nodes SET did = ? WHERE id = ?`, verifiedDID, claim.NodeID); err != nil {
					log.Printf("claims: verified %s for node %s but could not store it: %v", verifiedDID, claim.NodeID, err)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":         "approved",
				"verified":       true,
				"slug":           nodeSlug,
				"setup_required": true,
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":   "pending",
				"verified": false,
				"error":    verifyError,
			})
		}
	}
}

// fetchClaimPage fetches a page for meta_tag verification, reading at most
// 256KB of the response.
func fetchClaimPage(pageURL string) (string, error) {
	resp, err := ClaimHTTPClient.Get(pageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ListClaims handles GET /api/v1/admin/claims.
func ListClaims(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}
		after, limit := parsePaginationParams(r)

		query := `SELECT cr.id, cr.node_id, cr.user_id, cr.method, cr.evidence, cr.status, cr.created_at, COALESCE(cr.email,''),
			n.name, n.slug, COALESCE(n.verification_domain,''), COALESCE(u.username,''), COALESCE(u.display_name,'')
			FROM claim_requests cr
			JOIN nodes n ON cr.node_id = n.id
			JOIN users u ON cr.user_id = u.id
			WHERE cr.status = ?`
		args := []interface{}{status}

		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			query += " AND " + keysetCondition("cr.created_at", "cr.id", true)
			args = append(args, sortKey, sortKey, id)
		}
		query += " ORDER BY cr.created_at DESC, cr.id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to query claims"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type claimItem struct {
			ID                 string `json:"id"`
			NodeID             string `json:"node_id"`
			UserID             string `json:"user_id"`
			Method             string `json:"method"`
			Evidence           string `json:"evidence"`
			Status             string `json:"status"`
			CreatedAt          string `json:"created_at"`
			Email              string `json:"email"`
			NodeName           string `json:"node_name"`
			NodeSlug           string `json:"node_slug"`
			VerificationDomain string `json:"verification_domain"`
			ClaimantName       string `json:"claimant_username"`
			ClaimantDisplay    string `json:"claimant_display_name"`
		}

		var items []claimItem
		for rows.Next() {
			var c claimItem
			if err := rows.Scan(&c.ID, &c.NodeID, &c.UserID, &c.Method, &c.Evidence, &c.Status, &c.CreatedAt, &c.Email,
				&c.NodeName, &c.NodeSlug, &c.VerificationDomain, &c.ClaimantName, &c.ClaimantDisplay); err != nil {
				continue
			}
			items = append(items, c)
		}

		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}
		if items == nil {
			items = []claimItem{}
		}

		resp := map[string]interface{}{"items": items}
		if hasMore && len(items) > 0 {
			last := items[len(items)-1]
			resp["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ReviewClaim handles PATCH /api/v1/admin/claims/{id}.
func ReviewClaim(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		claimID := r.PathValue("id")

		var req struct {
			Action string `json:"action"` // "approve" or "reject"
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var claim model.ClaimRequest
		err := db.QueryRow(
			"SELECT id, node_id, user_id, status FROM claim_requests WHERE id = ?", claimID,
		).Scan(&claim.ID, &claim.NodeID, &claim.UserID, &claim.Status)
		if err != nil {
			http.Error(w, `{"error":"claim not found"}`, http.StatusNotFound)
			return
		}
		if claim.Status != "pending" {
			http.Error(w, `{"error":"claim is not pending"}`, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		resp := map[string]interface{}{"status": "ok"}

		switch req.Action {
		case "approve":
			if err := approveClaim(db, claimID, admin.ID, r.RemoteAddr); err != nil {
				http.Error(w, `{"error":"failed to approve claim"}`, http.StatusInternalServerError)
				return
			}
			db.Exec("UPDATE claim_requests SET reviewed_by = ?, review_note = ?, updated_at = ? WHERE id = ?",
				admin.ID, req.Note, now, claimID)
			resp["setup_required"] = true

		case "reject":
			db.Exec("UPDATE claim_requests SET status = 'rejected', reviewed_by = ?, review_note = ?, updated_at = ? WHERE id = ?",
				admin.ID, req.Note, now, claimID)
			auth.LogAuditEvent(db, admin.ID, "node.claim_rejected", "node", claim.NodeID, r.RemoteAddr, "")

		default:
			http.Error(w, `{"error":"action must be 'approve' or 'reject'"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// AdminSetVerificationDomain handles PATCH /api/v1/admin/nodes/{slug}/verification-domain.
// Instance admins set or clear the trust anchor for an unclaimed patch.
func AdminSetVerificationDomain(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		var req struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		domain, err := validateExplicitDomain(req.Domain)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		var nodeID, nodeStatus string
		if err := db.QueryRow("SELECT id, status FROM nodes WHERE slug = ? AND removed_at IS NULL", slug).Scan(&nodeID, &nodeStatus); err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if nodeStatus != "unclaimed" {
			http.Error(w, `{"error":"verification domains only apply to unclaimed patches"}`, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		db.Exec("UPDATE nodes SET verification_domain = ?, updated_at = ? WHERE id = ?", domain, now, nodeID)
		auth.LogAuditEvent(db, admin.ID, "node.verification_domain_set", "node", nodeID, r.RemoteAddr, fmt.Sprintf(`{"domain":"%s"}`, domain))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "verification_domain": domain})
	}
}

// AdminAssignOwner handles POST /api/v1/admin/nodes/{slug}/assign.
// An admin directly names who setup is reserved for — consent still can't
// be assigned, so this opens the same 14-day setup window a self-service
// claim would (docs/adr/039), it just skips proof.
func AdminAssignOwner(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
			return
		}

		// Verify node is unclaimed.
		var nodeID, nodeStatus, nodeName string
		err := db.QueryRow("SELECT id, status, name FROM nodes WHERE slug = ? AND removed_at IS NULL", slug).Scan(&nodeID, &nodeStatus, &nodeName)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if nodeStatus != "unclaimed" {
			http.Error(w, `{"error":"patch is not unclaimed"}`, http.StatusBadRequest)
			return
		}

		// Verify target user exists.
		var userExists int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", req.UserID).Scan(&userExists)
		if userExists == 0 {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		claimID := auth.NewUUIDv7()
		now := time.Now().UTC()
		nowStr := now.Format("2006-01-02T15:04:05.000Z")
		expiresAt := now.Add(setupWindow).Format("2006-01-02T15:04:05.000Z")
		_, err = db.Exec(
			// verification_token is unused for the admin method but given an
			// empty string rather than left NULL: model.ClaimRequest scans it
			// into a plain string everywhere claims are read back.
			`INSERT INTO claim_requests (id, node_id, user_id, method, evidence, status, verification_token, setup_expires_at, created_at, updated_at)
			 VALUES (?, ?, ?, 'admin', 'Assigned by instance admin', 'approved', '', ?, ?, ?)`,
			claimID, nodeID, req.UserID, expiresAt, nowStr, nowStr,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to assign owner"}`, http.StatusInternalServerError)
			return
		}
		finalizeClaimApproval(db, claimID, nodeID, slug, nodeName, req.UserID, expiresAt)

		auth.LogAuditEvent(db, admin.ID, "node.owner_assigned", "node", nodeID, r.RemoteAddr, fmt.Sprintf(`{"assigned_to":"%s"}`, req.UserID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "slug": slug, "setup_required": true})
	}
}

// finalizeClaimApproval rejects any other pending claims on the node and
// notifies the claimant that setup is open (docs/adr/039). Shared by every
// path that lands a claim in 'approved' status — self-service verification,
// admin review, and admin assignment — after each has done its own status
// update however it needed to.
func finalizeClaimApproval(db *database.DB, claimID, nodeID, nodeSlug, nodeName, claimantID, expiresAt string) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec(
		"UPDATE claim_requests SET status = 'rejected', review_note = 'Another claim was approved', updated_at = ? WHERE node_id = ? AND status = 'pending' AND id != ?",
		now, nodeID, claimID,
	)

	notify(notifications.Event{
		Type:     notifications.ClaimApproved,
		NodeID:   nodeID,
		NodeSlug: nodeSlug,
		NodeName: nodeName,
		TargetID: claimantID,
		EntityID: claimID,
		Title:    "Your claim on " + nodeName + " was approved",
		Body:     "Finish setting up the patch to make it yours. This approval expires " + formatClaimDate(expiresAt) + ".",
		Link:     weblink.PatchSetup(nodeSlug),
	})
}

// formatClaimDate renders an ISO timestamp for claimant-facing copy
// ("expires August 7, 2026"). Falls back to the raw string if parsing ever
// fails — never worth failing a notification over.
func formatClaimDate(iso string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", iso)
	if err != nil {
		return iso
	}
	return t.Format("January 2, 2006")
}

// markClaimApproved transitions a pending claim to 'approved' and opens its
// 14-day setup window, then runs the shared finalize step (docs/adr/039). It
// never touches the nodes row — approval is no longer activation.
func markClaimApproved(db *database.DB, claimID string) (nodeID string, err error) {
	var nodeSlug, nodeName, claimantID string
	err = db.QueryRow(
		`SELECT cr.node_id, n.slug, n.name, cr.user_id FROM claim_requests cr JOIN nodes n ON cr.node_id = n.id WHERE cr.id = ?`,
		claimID,
	).Scan(&nodeID, &nodeSlug, &nodeName, &claimantID)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(setupWindow).Format("2006-01-02T15:04:05.000Z")
	if _, err = db.Exec(
		"UPDATE claim_requests SET status = 'approved', setup_expires_at = ?, updated_at = ? WHERE id = ?",
		expiresAt, now.Format("2006-01-02T15:04:05.000Z"), claimID,
	); err != nil {
		return "", err
	}

	finalizeClaimApproval(db, claimID, nodeID, nodeSlug, nodeName, claimantID, expiresAt)
	return nodeID, nil
}

// approveClaim marks a claim approved (docs/adr/039) and logs the audit
// event under actorID — the claimant for self-service verification, the
// admin for a manual review. Used by every path where an existing pending
// claim is the thing being approved; AdminAssignOwner has no pending claim
// to approve, so it builds one directly and calls finalizeClaimApproval
// itself under its own "node.owner_assigned" audit event instead.
func approveClaim(db *database.DB, claimID, actorID, remoteAddr string) error {
	nodeID, err := markClaimApproved(db, claimID)
	if err != nil {
		return err
	}
	auth.LogAuditEvent(db, actorID, "node.claim_approved", "node", nodeID, remoteAddr, "")
	return nil
}

// errNodeNotUnclaimed reports that activation lost the race: the node
// stopped being unclaimed between the handler's check and the update.
var errNodeNotUnclaimed = fmt.Errorf("node is not unclaimed")

// activateClaimedNode is the activation core of patch setup (docs/adr/039):
// flips an unclaimed patch to active under its new owner and grants them
// admin membership. Used only by SetupClaim — approval no longer touches
// the nodes row, so this is the one place a node actually goes live.
func activateClaimedNode(db *database.DB, nodeID, newOwnerID, now string) error {
	// Conditional on still-unclaimed so two racing setups can't both
	// activate: the status check in SetupClaim is advisory, this is the gate.
	res, err := db.Exec(
		"UPDATE nodes SET owner_id = ?, status = 'active', updated_at = ? WHERE id = ? AND status = 'unclaimed'",
		newOwnerID, now, nodeID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNodeNotUnclaimed
	}

	// Create admin membership for new owner (if not already a member). A
	// failure here must fail setup: otherwise the patch goes active with an
	// owner who holds no admin membership.
	var existingMem int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ?", newOwnerID, nodeID).Scan(&existingMem)
	if existingMem == 0 {
		memID := auth.NewUUIDv7()
		_, err = db.Exec(
			"INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, 'admin', 'active', ?)",
			memID, newOwnerID, nodeID, now,
		)
	} else {
		_, err = db.Exec("UPDATE memberships SET role = 'admin', status = 'active' WHERE user_id = ? AND node_id = ?", newOwnerID, nodeID)
	}
	if err != nil {
		return fmt.Errorf("grant admin membership: %w", err)
	}
	return nil
}

// SetupClaim handles POST /api/v1/claims/{id}/setup — the creation moment
// (docs/adr/039). An approved claim is a single-use, expiring right to enter
// the patch creation flow prepopulated with the listing's data: this
// activates the node, grants the claimant admin, and adopts the
// then-current lining out loud, exactly as at ordinary patch creation.
func SetupClaim(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		claimID := r.PathValue("id")

		// The setup form reuses the creation form, template picker included —
		// setup allows everything creation allows (docs/adr/039). The body is
		// optional; an empty one keeps the default template, same as before.
		var req struct {
			Template string `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Template != "" {
			valid := false
			for _, t := range governance.ValidTemplates {
				if t == req.Template {
					valid = true
					break
				}
			}
			if !valid {
				http.Error(w, `{"error":"unknown governance template"}`, http.StatusBadRequest)
				return
			}
		}

		var claimUserID, claimStatus, nodeID, nodeSlug, nodeStatus string
		var setupExpiresAt sql.NullString
		err := db.QueryRow(
			`SELECT cr.user_id, cr.status, cr.setup_expires_at, n.id, n.slug, n.status
			 FROM claim_requests cr JOIN nodes n ON cr.node_id = n.id
			 WHERE cr.id = ?`, claimID,
		).Scan(&claimUserID, &claimStatus, &setupExpiresAt, &nodeID, &nodeSlug, &nodeStatus)
		if err != nil {
			http.Error(w, `{"error":"claim not found"}`, http.StatusNotFound)
			return
		}
		if claimUserID != user.ID {
			http.Error(w, `{"error":"not your claim"}`, http.StatusForbidden)
			return
		}
		if claimStatus != "approved" {
			http.Error(w, fmt.Sprintf(`{"error":"claim is %s, not approved"}`, claimStatus), http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		nowStr := now.Format("2006-01-02T15:04:05.000Z")
		if setupExpiresAt.Valid && setupExpiresAt.String != "" {
			if exp, perr := time.Parse("2006-01-02T15:04:05.000Z", setupExpiresAt.String); perr == nil && now.After(exp) {
				db.Exec("UPDATE claim_requests SET status = 'expired', updated_at = ? WHERE id = ?", nowStr, claimID)
				http.Error(w, `{"error":"this claim's setup window has expired — the patch is claimable again"}`, http.StatusGone)
				return
			}
		}
		// No "awaiting setup" state exists — a claim that hasn't finished
		// setup leaves the patch unclaimed to everyone (docs/adr/039). If it
		// isn't unclaimed anymore, either this claim already consumed the
		// window or someone else's did.
		if nodeStatus != "unclaimed" {
			http.Error(w, `{"error":"this patch is no longer unclaimed"}`, http.StatusConflict)
			return
		}

		if err := activateClaimedNode(db, nodeID, user.ID, nowStr); err != nil {
			if err == errNodeNotUnclaimed {
				http.Error(w, `{"error":"this patch is no longer unclaimed"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"failed to complete setup"}`, http.StatusInternalServerError)
			return
		}

		// Governance is created here, not at claim approval — an unclaimed
		// patch carries none (docs/adr/039). Best-effort like every other
		// governance write; a missing data dir (gitless test/dev runs) is
		// tolerated, same as the startup backfill.
		forked := false
		if dataDir := governance.GetDataDir(); dataDir != "" {
			if err := governance.ForkForNode(dataDir, nodeID, req.Template); err != nil {
				log.Printf("claims: governance fork for node %s: %v", nodeID, err)
			} else {
				forked = true
			}
		}
		if forked {
			// Absorb the unclaimed row's live membership settings into the
			// template's rules file, then sync the rules into the DB cache —
			// the same treatment as ordinary creation (docs/adr/041). Without
			// the absorb, the rules file holds the template's membership
			// policy, and a later amendment sync would clobber the enforced
			// value.
			dataDir := governance.GetDataDir()
			var membershipPolicy, fpJSON string
			db.QueryRow(`SELECT membership_policy, COALESCE(follower_permissions,'') FROM nodes WHERE id = ?`, nodeID).
				Scan(&membershipPolicy, &fpJSON)
			if rules, err := governance.ReadRules(dataDir, nodeID); err == nil {
				if membershipPolicy != "" {
					rules.MembershipPolicy = membershipPolicy
				}
				if fpJSON != "" && fpJSON != "{}" {
					json.Unmarshal([]byte(fpJSON), &rules.FollowerPermissions)
				}
				if _, err := governance.WriteRules(dataDir, nodeID, rules, "Membership settings from claim setup"); err != nil {
					log.Printf("claims: absorb setup settings for node %s: %v", nodeID, err)
				}
			}
			if err := governance.SyncRulesToDB(db, dataDir, nodeID); err != nil {
				log.Printf("claims: sync governance rules for node %s: %v", nodeID, err)
			}
		} else {
			// Cache the rules on the row anyway — the unclaimed row carries
			// the pre-leadership column default (migration 041); gitless runs
			// get complete defaults.
			if err := governance.SyncConfigToDB(db, governance.GetDataDir(), nodeID); err != nil {
				log.Printf("claims: governance config sync for node %s: %v", nodeID, err)
			}
		}
		CreateDefaultLining(db, nodeID, user.ID)

		// The claim row stays 'approved' — a second setup attempt is refused
		// because the node is no longer unclaimed, not because the claim
		// changed state (docs/adr/039).
		auth.LogAuditEvent(db, user.ID, "node.claimed", "node", nodeID, r.RemoteAddr, "")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "slug": nodeSlug})
	}
}
