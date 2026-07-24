package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// clientIP extracts the client IP address from the request. See
// middleware.ClientIP for the trust model around X-Forwarded-For.
func clientIP(r *http.Request) string {
	return middleware.ClientIP(r)
}

// GenerateInviteLink handles POST /api/v1/auth/invite-link (admin-only).
func GenerateInviteLink(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			MaxUses      int `json:"max_uses"`
			ExpiresInHrs int `json:"expires_in_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Body is optional; defaults are fine.
		}

		if err := middleware.CheckInviteGenerationRate(user.ID); err != nil {
			w.Header().Set("Retry-After", "180")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		maxUses := req.MaxUses
		if maxUses <= 0 {
			maxUses = 1
		}

		var expiresAt *time.Time
		if req.ExpiresInHrs > 0 {
			t := time.Now().Add(time.Duration(req.ExpiresInHrs) * time.Hour)
			expiresAt = &t
		}

		rawToken, err := auth.GenerateInviteLink(db, user.ID, maxUses, expiresAt)
		if err != nil {
			http.Error(w, `{"error":"failed to generate invite link"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "invite.generate", "invite_link", "", "{}", clientIP(r))

		// Must match the SPA route (/invite/:token in App.svelte).
		url := "https://" + cfg.Instance.Domain + "/invite/" + rawToken

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token": rawToken,
			"url":   url,
		})
	}
}

// ValidateInviteLink handles GET /api/v1/auth/invite/{token}/validate.
// Lets the invite landing page distinguish a live invite from an expired or
// used-up one before the visitor fills in the signup form. Returns no
// invite metadata beyond validity.
func ValidateInviteLink(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := r.PathValue("token")
		if rawToken == "" {
			http.Error(w, `{"error":"token required"}`, http.StatusBadRequest)
			return
		}

		if err := auth.ValidateInviteLink(db, rawToken); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"valid": true})
	}
}

// RedeemInviteLink handles POST /api/v1/auth/invite with {token, username} body.
func RedeemInviteLink(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token       string `json:"token"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Token == "" || req.Username == "" {
			http.Error(w, `{"error":"token and username are required"}`, http.StatusBadRequest)
			return
		}

		user, err := auth.RedeemInviteLink(db, req.Token, req.Username, req.DisplayName)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		ip := clientIP(r)

		// Create session.
		token, err := auth.CreateSession(db, user.ID, ip, r.UserAgent())
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, token)
		auth.LogAuditEvent(db, user.ID, "invite.redeem", "user", user.ID, "{}", ip)
		auth.LogAuditEvent(db, user.ID, "user.create", "user", user.ID, "{}", ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// magicLinkURL builds the verify link — both the one emailed and the one
// printed to the server log when SMTP is not configured. It must point at the
// API route (/api/v1/auth/verify/{token}), not an SPA path: the SPA has no
// verify route and would silently fall back to the home page. With
// instance.domain set it must be clickable from outside the box
// (DEPLOYMENT.md tells deployers to grep the log for it), so it uses https on
// the public domain like the other outward-facing URLs. Only a domainless
// config (local dev) falls back to localhost.
func magicLinkURL(domain, port, token string) string {
	if domain != "" {
		return fmt.Sprintf("https://%s/api/v1/auth/verify/%s", domain, token)
	}
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://localhost:%s/api/v1/auth/verify/%s", port, token)
}

// RequestMagicLink handles POST /api/v1/auth/magic-link.
func RequestMagicLink(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			// Always return 200 to not leak info.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		ip := clientIP(r)

		// Rate limit.
		if err := middleware.CheckMagicLinkRate(req.Email, ip); err != nil {
			// Still return 200 to not leak info.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		if cfg.SMTP.Configured() {
			// Best-effort send. The response never exposes errors, but the log
			// must — a broken SMTP config is otherwise invisible to operators.
			linkFor := func(token string) string {
				return magicLinkURL(cfg.Instance.Domain, cfg.Server.Port, token)
			}
			if err := auth.GenerateMagicLink(db, req.Email, cfg.SMTP, linkFor); err != nil {
				log.Printf("magic link: send to %s failed: %v", req.Email, err)
			}
		} else {
			// No SMTP — generate the link and print to the server log.
			token, err := auth.GenerateMagicLinkLocal(db, req.Email)
			if err == nil {
				link := magicLinkURL(cfg.Instance.Domain, cfg.Server.Port, token)
				log.Printf("\n\033[1;36m✉  Magic link for %s:\033[0m\n   \033[4m%s\033[0m\n", req.Email, link)
			} else {
				log.Printf("magic link: generate for %s failed: %v", req.Email, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// VerifyMagicLink handles GET /api/v1/auth/verify/{token}.
// Existing accounts are logged straight in. An unknown email gets a signup
// token instead of an account — the username is chosen by the person, never
// derived from the email (docs/adr/013) — and is sent to /signup/complete.
func VerifyMagicLink(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := r.PathValue("token")
		if rawToken == "" {
			http.Error(w, `{"error":"token required"}`, http.StatusBadRequest)
			return
		}

		user, signupToken, err := auth.VerifyMagicLink(db, rawToken)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		if user == nil {
			// New email: no account yet. Hand the browser to the
			// username-selection page; API clients get the token as JSON.
			accept := r.Header.Get("Accept")
			if accept == "" || accept == "*/*" || len(accept) > 20 {
				dest := "/signup/complete?token=" + signupToken
				if rd := r.URL.Query().Get("redirect"); rd != "" && len(rd) < 256 && rd[0] == '/' {
					dest += "&redirect=" + rd
				}
				http.Redirect(w, r, dest, http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status":       "username_required",
				"signup_token": signupToken,
			})
			return
		}

		ip := clientIP(r)

		sessionToken, err := auth.CreateSession(db, user.ID, ip, r.UserAgent())
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, sessionToken)
		auth.LogAuditEvent(db, user.ID, "user.login", "user", user.ID, `{"method":"magic_link"}`, ip)

		// If request is from a browser (not API client), redirect.
		accept := r.Header.Get("Accept")
		if accept == "" || accept == "*/*" || len(accept) > 20 {
			dest := "/dashboard"
			if rd := r.URL.Query().Get("redirect"); rd != "" && len(rd) < 256 && rd[0] == '/' {
				dest = rd
			}
			http.Redirect(w, r, dest, http.StatusFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// ValidateSignupToken handles GET /api/v1/auth/signup/{token}/validate.
// Lets the signup-completion page show which email is being signed up
// before the form is submitted. Possession of the token is proof of
// control of that email (it arrived there), so echoing it back to the
// holder leaks nothing.
func ValidateSignupToken(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := r.PathValue("token")
		if rawToken == "" {
			http.Error(w, `{"error":"token required"}`, http.StatusBadRequest)
			return
		}

		email, err := auth.ValidateSignupToken(db, rawToken)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": true,
			"email": email,
		})
	}
}

// CompleteSignup handles POST /api/v1/auth/signup with
// {token, username, display_name}. Consumes the signup token issued by
// magic-link verification and creates the account with the chosen
// username (docs/adr/013).
func CompleteSignup(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token       string `json:"token"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Token == "" || req.Username == "" {
			http.Error(w, `{"error":"token and username are required"}`, http.StatusBadRequest)
			return
		}

		user, err := auth.CompleteSignup(db, req.Token, req.Username, req.DisplayName)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		ip := clientIP(r)

		token, err := auth.CreateSession(db, user.ID, ip, r.UserAgent())
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, token)
		auth.LogAuditEvent(db, user.ID, "user.create", "user", user.ID, `{"method":"magic_link"}`, ip)
		auth.LogAuditEvent(db, user.ID, "user.login", "user", user.ID, `{"method":"magic_link"}`, ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// maxCredentialPayloadBytes bounds the attestation body we buffer in order to
// read the passkey name out of it. Attestation objects are a few KB at most;
// this leaves generous headroom without letting an authenticated request hand
// us an unbounded read.
const maxCredentialPayloadBytes = 1 << 20

// WebAuthnRegisterBegin handles POST /api/v1/auth/webauthn/register/begin.
func WebAuthnRegisterBegin(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		optJSON, err := wa.BeginRegistration(user)
		if err != nil {
			http.Error(w, `{"error":"failed to begin registration"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(optJSON)
	}
}

// WebAuthnRegisterFinish handles POST /api/v1/auth/webauthn/register/finish.
func WebAuthnRegisterFinish(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		// The name rides alongside the credential as a sibling field rather than
		// wrapping it, so a client that sends only the raw credential still
		// enrolls successfully and just gets the default name.
		body, err := io.ReadAll(io.LimitReader(r.Body, maxCredentialPayloadBytes))
		if err != nil {
			http.Error(w, `{"error":"invalid credential response"}`, http.StatusBadRequest)
			return
		}

		var named struct {
			Name string `json:"name"`
		}
		json.Unmarshal(body, &named)

		parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
		if err != nil {
			http.Error(w, `{"error":"invalid credential response"}`, http.StatusBadRequest)
			return
		}

		cred, err := wa.FinishRegistration(user, parsedResponse, named.Name)
		if err != nil {
			http.Error(w, `{"error":"registration failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		ip := clientIP(r)
		auth.LogAuditEvent(db, user.ID, "credential.enroll", "credential", cred.ID, "{}", ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cred)
	}
}

// StepUpStatus handles GET /api/v1/auth/step-up — what the current session
// can do without further ceremony, and whether the person even has a passkey
// to step up with.
//
// The admin UI reads this to warn about a missing passkey *before* someone
// reaches for the export button, which ADR 017 asks for specifically: the
// requirement should not be discovered at the moment it blocks you.
func StepUpStatus(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		resp := map[string]interface{}{
			"has_passkey": auth.HasCredential(db, user.ID),
			"active":      middleware.SudoSatisfied(db, r),
			"window_secs": int(auth.SudoWindow.Seconds()),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// StepUpBegin handles POST /api/v1/auth/step-up/begin — starts a WebAuthn
// assertion for the signed-in user (docs/adr/017).
func StepUpBegin(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		optJSON, err := wa.BeginStepUp(user)
		if errors.Is(err, auth.ErrNoCredentials) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "This action needs a passkey. Enroll one in Security settings first.",
				"code":  "passkey_required",
			})
			return
		}
		if err != nil {
			http.Error(w, `{"error":"failed to begin step-up"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(optJSON)
	}
}

// StepUpFinish handles POST /api/v1/auth/step-up/finish — verifies the
// assertion and opens the five-minute window on this session.
func StepUpFinish(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		parsedResponse, err := protocol.ParseCredentialRequestResponseBody(r.Body)
		if err != nil {
			http.Error(w, `{"error":"invalid assertion response"}`, http.StatusBadRequest)
			return
		}

		if err := wa.FinishStepUp(user, parsedResponse); err != nil {
			http.Error(w, `{"error":"step-up failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		// The window is written to the row of the session presenting the
		// cookie, so it is confined to this browser and dies with logout.
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			http.Error(w, `{"error":"no session"}`, http.StatusUnauthorized)
			return
		}
		until, err := auth.GrantSudo(db, cookie.Value)
		if err != nil {
			http.Error(w, `{"error":"failed to open confirmation window"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "auth.step_up", "user", user.ID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":     true,
			"expires_at": until.Format(time.RFC3339),
		})
	}
}

// WebAuthnLoginBegin handles POST /api/v1/auth/webauthn/login/begin.
func WebAuthnLoginBegin(wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		optJSON, err := wa.BeginLogin()
		if err != nil {
			http.Error(w, `{"error":"failed to begin login"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(optJSON)
	}
}

// WebAuthnLoginFinish handles POST /api/v1/auth/webauthn/login/finish.
func WebAuthnLoginFinish(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parsedResponse, err := protocol.ParseCredentialRequestResponseBody(r.Body)
		if err != nil {
			http.Error(w, `{"error":"invalid assertion response"}`, http.StatusBadRequest)
			return
		}

		user, err := wa.FinishLogin(parsedResponse)
		if err != nil {
			http.Error(w, `{"error":"login failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		ip := clientIP(r)

		sessionToken, err := auth.CreateSession(db, user.ID, ip, r.UserAgent())
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, sessionToken)
		auth.LogAuditEvent(db, user.ID, "user.login", "user", user.ID, `{"method":"webauthn"}`, ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// loadUserLinks populates user.Links from the users.links JSON column.
func loadUserLinks(db *database.DB, user *model.User) {
	var linksJSON string
	db.QueryRow("SELECT COALESCE(links,'[]') FROM users WHERE id = ?", user.ID).Scan(&linksJSON)
	user.Links = []model.NodeLink{}
	json.Unmarshal([]byte(linksJSON), &user.Links)
}

// Me handles GET /api/v1/auth/me.
func Me(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		loadUserLinks(db, user)
		var hide int
		db.QueryRow("SELECT hide_amended_linings FROM users WHERE id = ?", user.ID).Scan(&hide)
		user.HideAmendedLinings = hide == 1
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// UpdateMe handles PATCH /api/v1/auth/me — update current user's profile.
func UpdateMe(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			DisplayName        *string           `json:"display_name"`
			Bio                *string           `json:"bio"`
			Links              *[]model.NodeLink `json:"links"`
			StartOnMyQuilt     *bool             `json:"start_on_my_quilt"`
			HideAmendedLinings *bool             `json:"hide_amended_linings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.DisplayName != nil {
			_, err := db.Exec("UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?",
				*req.DisplayName, time.Now().UTC().Format(time.RFC3339), user.ID)
			if err != nil {
				http.Error(w, `{"error":"failed to update display name"}`, http.StatusInternalServerError)
				return
			}
			user.DisplayName = *req.DisplayName
		}

		if req.Bio != nil {
			_, err := db.Exec("UPDATE users SET bio = ?, updated_at = ? WHERE id = ?",
				*req.Bio, time.Now().UTC().Format(time.RFC3339), user.ID)
			if err != nil {
				http.Error(w, `{"error":"failed to update bio"}`, http.StatusInternalServerError)
				return
			}
			user.Bio = *req.Bio
		}

		if req.Links != nil {
			lb, _ := json.Marshal(*req.Links)
			_, err := db.Exec("UPDATE users SET links = ?, updated_at = ? WHERE id = ?",
				string(lb), time.Now().UTC().Format(time.RFC3339), user.ID)
			if err != nil {
				http.Error(w, `{"error":"failed to update links"}`, http.StatusInternalServerError)
				return
			}
		}
		if req.StartOnMyQuilt != nil {
			_, err := db.Exec("UPDATE users SET start_on_my_quilt = ?, updated_at = ? WHERE id = ?",
				*req.StartOnMyQuilt, time.Now().UTC().Format(time.RFC3339), user.ID)
			if err != nil {
				http.Error(w, `{"error":"failed to update landing preference"}`, http.StatusInternalServerError)
				return
			}
			user.StartOnMyQuilt = *req.StartOnMyQuilt
		}

		if req.HideAmendedLinings != nil {
			v := 0
			if *req.HideAmendedLinings {
				v = 1
			}
			_, err := db.Exec("UPDATE users SET hide_amended_linings = ?, updated_at = ? WHERE id = ?",
				v, time.Now().UTC().Format(time.RFC3339), user.ID)
			if err != nil {
				http.Error(w, `{"error":"failed to update setting"}`, http.StatusInternalServerError)
				return
			}
		}
		loadUserLinks(db, user)
		var hide int
		db.QueryRow("SELECT hide_amended_linings FROM users WHERE id = ?", user.ID).Scan(&hide)
		user.HideAmendedLinings = hide == 1

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// Logout handles POST /api/v1/auth/logout.
func Logout(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err == nil {
			user := middleware.UserFromContext(r.Context())
			auth.DestroySession(db, cookie.Value)
			if user != nil {
				auth.LogAuditEvent(db, user.ID, "user.logout", "user", user.ID, "{}", clientIP(r))
			}
		}

		auth.ClearSessionCookie(w)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListCredentials handles GET /api/v1/auth/credentials.
func ListCredentials(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		rows, err := db.Query(
			`SELECT id, user_id, name, created_at FROM credentials WHERE user_id = ? ORDER BY created_at`,
			user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list credentials"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type credResponse struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
		}
		var creds []credResponse
		for rows.Next() {
			var c credResponse
			var userID string
			if err := rows.Scan(&c.ID, &userID, &c.Name, &c.CreatedAt); err != nil {
				continue
			}
			creds = append(creds, c)
		}
		if creds == nil {
			creds = []credResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(creds)
	}
}

// RenameCredential handles PATCH /api/v1/auth/credentials/{id}. Passkeys are
// only distinguishable by their names, so someone who enrolled several before
// naming was possible needs a way to tell them apart after the fact.
func RenameCredential(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		credID := r.PathValue("id")
		if credID == "" {
			http.Error(w, `{"error":"credential id required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		name := auth.SanitizeCredentialName(req.Name)

		// Scoped by user_id: a credential id is not a capability to rename
		// someone else's passkey.
		result, err := db.Exec(
			`UPDATE credentials SET name = ? WHERE id = ? AND user_id = ?`,
			name, credID, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to rename credential"}`, http.StatusInternalServerError)
			return
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			http.Error(w, `{"error":"credential not found"}`, http.StatusNotFound)
			return
		}

		auth.LogAuditEvent(db, user.ID, "credential.rename", "credential", credID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": credID, "name": name})
	}
}

// DeleteCredential handles DELETE /api/v1/auth/credentials/{id}.
func DeleteCredential(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		credID := r.PathValue("id")
		if credID == "" {
			http.Error(w, `{"error":"credential id required"}`, http.StatusBadRequest)
			return
		}

		result, err := db.Exec(
			`DELETE FROM credentials WHERE id = ? AND user_id = ?`,
			credID, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to delete credential"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"credential not found"}`, http.StatusNotFound)
			return
		}

		// Revoking a passkey is how someone responds to a device being lost or
		// stolen, so cut every other live session with it — removing the
		// credential alone would leave any session it created still valid.
		// The requesting session survives: the person doing this is present
		// and authenticated, and signing them out mid-flow helps no one.
		if cookie, err := r.Cookie(auth.CookieName); err == nil {
			if err := auth.DestroyOtherUserSessions(db, user.ID, cookie.Value); err != nil {
				log.Printf("auth: revoke other sessions for %s: %v", user.ID, err)
			}
		}

		auth.LogAuditEvent(db, user.ID, "credential.revoke", "credential", credID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
