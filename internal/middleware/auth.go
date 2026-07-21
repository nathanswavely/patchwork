package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

type contextKey string

// UserContextKey is the context key used to store the authenticated user.
const UserContextKey contextKey = "patchwork_user"

// For backwards compat with internal references
const userContextKey = UserContextKey

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userContextKey).(*model.User)
	return u
}

// AuthRequired is middleware that checks for a valid session cookie
// and injects the user into the request context. Returns 401 if no valid session.
func AuthRequired(db *database.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		user, err := auth.ValidateSession(db, cookie.Value)
		if err != nil || user == nil {
			http.Error(w, `{"error":"invalid or expired session"}`, http.StatusUnauthorized)
			return
		}

		// Suspension enforcement: suspended users can read but not mutate.
		if user.SuspendedAt != nil && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			http.Error(w, `{"error":"Your account has been suspended"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// AuthOptional injects the user into context if a valid session exists,
// but does NOT block the request if unauthenticated. Use for public
// endpoints that return extra data for logged-in users.
func AuthOptional(db *database.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err == nil {
			user, err := auth.ValidateSession(db, cookie.Value)
			if err == nil && user != nil {
				ctx := context.WithValue(r.Context(), userContextKey, user)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	}
}

// AdminRequired wraps AuthRequired and additionally checks that the user has the
// "admin" role and is not suspended.
//
// AuthRequired deliberately lets suspended users keep reading — a suspended
// member can still browse the patches they were part of. That carve-out is wrong
// for admin routes: the reads behind them are the instance export, the user list,
// and the audit log. Suspending a misbehaving admin has to cut those off at once,
// otherwise their live session outlives the suspension by up to 30 days.
func AdminRequired(db *database.DB, next http.HandlerFunc) http.HandlerFunc {
	return AuthRequired(db, func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
		if user.SuspendedAt != nil {
			http.Error(w, `{"error":"Your account has been suspended"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SudoRequired gates an action behind a live step-up window — a WebAuthn
// assertion completed in the last few minutes (docs/adr/017).
//
// It wraps exactly three actions: instance wipe, instance export, and
// promotion to instance admin. Deliberately not every admin route: re-
// prompting for routine moderation trains people to approve prompts without
// reading them, which is how step-up auth stops working. Holding a valid
// session is proof of *identity*; for the irreversible three it is not proof
// of *presence*, and that is what this asks for.
//
// Compose it inside AdminRequired, which has already established the session.
func SudoRequired(db *database.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !SudoSatisfied(db, r) {
			WriteSudoRequired(db, w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// SudoSatisfied reports whether the request's own session is inside a live
// step-up window. Exposed for handlers that gate conditionally — promoting to
// admin is one field of a general-purpose PATCH, so the check cannot live in
// the router.
func SudoSatisfied(db *database.DB, r *http.Request) bool {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return false
	}
	return auth.SudoActive(db, cookie.Value)
}

// WriteSudoRequired sends the 403 that tells the SPA to run a step-up
// ceremony. The body distinguishes "confirm with your passkey" from "you have
// no passkey, enroll one first", because those need different UI and the
// second is not something to discover mid-action.
func WriteSudoRequired(db *database.DB, w http.ResponseWriter, r *http.Request) {
	code := "sudo_required"
	message := "Confirm with your passkey to continue."

	if user := UserFromContext(r.Context()); user != nil && !auth.HasCredential(db, user.ID) {
		code = "passkey_required"
		message = "This action needs a passkey. Enroll one in Security settings first."
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": message, "code": code})
}
