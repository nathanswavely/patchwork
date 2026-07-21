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
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// External lookups used by claim verification, swappable in tests.
var (
	ClaimLookupTXT  func(domain string) ([]string, error) = net.LookupTXT
	ClaimHTTPClient                                       = &http.Client{Timeout: 10 * time.Second}
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

// claimMethodsFor reports which claim methods a patch currently supports.
func claimMethodsFor(verificationDomain string, cfg *config.Config) map[string]bool {
	hasDomain := verificationDomain != ""
	return map[string]bool{
		"dns":      hasDomain,
		"meta_tag": hasDomain,
		"email":    hasDomain && cfg.SMTP.Configured(),
		"admin":    true,
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

		validMethods := map[string]bool{"dns": true, "meta_tag": true, "email": true, "admin": true}
		if !validMethods[req.Method] {
			http.Error(w, `{"error":"method must be dns, meta_tag, email, or admin"}`, http.StatusBadRequest)
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
			claimEmail = strings.TrimSpace(strings.ToLower(req.Email))
			at := strings.LastIndex(claimEmail, "@")
			if at <= 0 || at == len(claimEmail)-1 {
				http.Error(w, `{"error":"a valid email address is required for email verification"}`, http.StatusBadRequest)
				return
			}
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

		resp := map[string]interface{}{
			"claim":               nil,
			"methods":             claimMethodsFor(verificationDomain, cfg),
			"verification_domain": verificationDomain,
			"node_status":         nodeStatus,
		}

		var c model.ClaimRequest
		err = db.QueryRow(
			`SELECT id, method, evidence, status, verification_token, COALESCE(email,''), created_at
			 FROM claim_requests WHERE node_id = ? AND user_id = ? AND status = 'pending'
			 ORDER BY created_at DESC LIMIT 1`, nodeID, user.ID,
		).Scan(&c.ID, &c.Method, &c.Evidence, &c.Status, &c.VerificationToken, &c.Email, &c.CreatedAt)
		if err == nil {
			claim := map[string]interface{}{
				"id":         c.ID,
				"method":     c.Method,
				"evidence":   c.Evidence,
				"status":     c.Status,
				"email":      c.Email,
				"created_at": c.CreatedAt,
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
		claimID, nodeID, userID, _, slug, expiresAt, ok := lookupEmailClaim(db, req.Token)
		if !ok {
			http.Error(w, `{"error":"invalid or already-used verification link"}`, http.StatusNotFound)
			return
		}
		if emailClaimExpired(expiresAt) {
			http.Error(w, `{"error":"this verification link has expired — request a new email from the claim page"}`, http.StatusBadRequest)
			return
		}

		if err := transferOwnership(db, nodeID, userID, claimID, r.RemoteAddr); err != nil {
			http.Error(w, `{"error":"failed to transfer ownership"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "approved",
			"slug":   slug,
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

		case "admin":
			verifyError = "admin claims are reviewed manually — you'll be notified"

		case "email":
			verifyError = "check your inbox — email claims are completed via the emailed link"
		}

		if verified {
			if err := transferOwnership(db, claim.NodeID, user.ID, claimID, r.RemoteAddr); err != nil {
				http.Error(w, `{"error":"failed to transfer ownership"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":   "approved",
				"verified": true,
				"slug":     nodeSlug,
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

		switch req.Action {
		case "approve":
			if err := transferOwnership(db, claim.NodeID, claim.UserID, claimID, r.RemoteAddr); err != nil {
				http.Error(w, `{"error":"failed to transfer ownership"}`, http.StatusInternalServerError)
				return
			}
			db.Exec("UPDATE claim_requests SET reviewed_by = ?, review_note = ?, updated_at = ? WHERE id = ?",
				admin.ID, req.Note, now, claimID)
			auth.LogAuditEvent(db, admin.ID, "node.claim_approved", "node", claim.NodeID, r.RemoteAddr, "")

		case "reject":
			db.Exec("UPDATE claim_requests SET status = 'rejected', reviewed_by = ?, review_note = ?, updated_at = ? WHERE id = ?",
				admin.ID, req.Note, now, claimID)
			auth.LogAuditEvent(db, admin.ID, "node.claim_rejected", "node", claim.NodeID, r.RemoteAddr, "")

		default:
			http.Error(w, `{"error":"action must be 'approve' or 'reject'"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
// Admin directly assigns a user as owner of an unclaimed patch.
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
		var nodeID, nodeStatus string
		err := db.QueryRow("SELECT id, status FROM nodes WHERE slug = ? AND removed_at IS NULL", slug).Scan(&nodeID, &nodeStatus)
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

		if err := transferOwnership(db, nodeID, req.UserID, "", r.RemoteAddr); err != nil {
			http.Error(w, `{"error":"failed to assign owner"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, admin.ID, "node.owner_assigned", "node", nodeID, r.RemoteAddr, fmt.Sprintf(`{"assigned_to":"%s"}`, req.UserID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "slug": slug})
	}
}

// transferOwnership moves an unclaimed patch to active status with a new owner.
func transferOwnership(db *database.DB, nodeID, newOwnerID, claimID, remoteAddr string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Update node ownership and status.
	_, err := db.Exec(
		"UPDATE nodes SET owner_id = ?, status = 'active', updated_at = ? WHERE id = ?",
		newOwnerID, now, nodeID,
	)
	if err != nil {
		return err
	}

	// Create admin membership for new owner (if not already a member). A
	// failure here must fail the transfer: otherwise the patch goes active
	// with an owner who holds no admin membership.
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

	// If there's a claim, mark it approved and reject all others.
	if claimID != "" {
		db.Exec("UPDATE claim_requests SET status = 'approved', updated_at = ? WHERE id = ?", now, claimID)
		db.Exec("UPDATE claim_requests SET status = 'rejected', review_note = 'Another claim was approved', updated_at = ? WHERE node_id = ? AND status = 'pending' AND id != ?",
			now, nodeID, claimID)
	}

	auth.LogAuditEvent(db, newOwnerID, "node.claimed", "node", nodeID, remoteAddr, "")
	return nil
}
