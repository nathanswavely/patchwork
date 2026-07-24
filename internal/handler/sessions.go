package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// ListSessions handles GET /api/v1/auth/sessions — the caller's OWN active
// sessions only (issue #3). There is deliberately no path to another person's
// sessions, not even for instance admins: a session list is a map of where
// someone is signed in, and that is theirs alone. The response carries a coarse
// device label and timestamps, never token or token-hash material.
func ListSessions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		// The cookie identifies which session is "this one". A missing cookie
		// shouldn't happen behind AuthRequired, but if it does, nothing is
		// flagged current and the list still renders.
		currentToken := ""
		if cookie, err := r.Cookie(auth.CookieName); err == nil {
			currentToken = cookie.Value
		}

		sessions, err := auth.ListUserSessions(db, user.ID, currentToken)
		if err != nil {
			http.Error(w, `{"error":"failed to list sessions"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}

// RevokeSession handles DELETE /api/v1/auth/sessions/{id} — ends one of the
// caller's own sessions. The id must belong to the authenticated user; a
// non-owned or unknown id gets 404, never 403, so someone else's id is
// indistinguishable from a nonexistent one.
//
// Revoking the current session is logout: the row is gone, so its cookie is now
// dead weight, and we clear it so the browser stops presenting it.
func RevokeSession(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
			return
		}

		// Resolve whether this is the current session before deleting it.
		currentID := ""
		if cookie, err := r.Cookie(auth.CookieName); err == nil {
			currentID = auth.SessionIDForToken(db, cookie.Value)
		}

		deleted, err := auth.DestroyUserSession(db, user.ID, sessionID)
		if err != nil {
			http.Error(w, `{"error":"failed to revoke session"}`, http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			return
		}

		auth.LogAuditEvent(db, user.ID, "session.revoke", "session", sessionID, "{}", clientIP(r))

		isCurrent := sessionID == currentID
		if isCurrent {
			auth.ClearSessionCookie(w)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"was_current": isCurrent,
		})
	}
}

// RevokeOtherSessions handles POST /api/v1/auth/sessions/revoke-others —
// "sign out everywhere else". Every session except the one making the request
// is cut; the caller stays signed in. No step-up gate: requiring a passkey to
// kick out a possibly-stolen session would be backwards (issue #3).
func RevokeOtherSessions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			http.Error(w, `{"error":"no session"}`, http.StatusUnauthorized)
			return
		}

		if err := auth.DestroyOtherUserSessions(db, user.ID, cookie.Value); err != nil {
			log.Printf("auth: revoke other sessions for %s: %v", user.ID, err)
			http.Error(w, `{"error":"failed to sign out other sessions"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "session.revoke_others", "user", user.ID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
