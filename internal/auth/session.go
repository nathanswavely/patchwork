package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

const (
	CookieName   = "patchwork_session"
	tokenByteLen = 32

	// SessionExpiry is the default absolute session lifetime, and the value
	// used when nothing configures otherwise. See config.Session.
	SessionExpiry = 30 * 24 * time.Hour

	// SessionIdleTimeout is the default idle timeout (docs/adr/017).
	SessionIdleTimeout = 14 * 24 * time.Hour

	// stampInterval throttles the last_used_at write. Validation runs on
	// every authenticated request; the target hardware is a Pi 4 on an SD
	// card, so a session is re-stamped at most once an hour. The cost is
	// that the effective idle timeout is fuzzy by up to an hour, which is
	// irrelevant next to a 14-day window.
	stampInterval = time.Hour

	// SudoWindow is how long a fresh WebAuthn assertion authorizes the three
	// irreversible instance actions.
	SudoWindow = 5 * time.Minute
)

// Lifetimes are process-globals set once at startup from patchwork.yaml, in
// the same shape as ap.SetDomain. They default to the values that were
// hardcoded before ADR 017, so an instance whose config says nothing about
// sessions behaves exactly as it did.
var (
	sessionMaxLifetime = SessionExpiry
	sessionIdleTimeout = SessionIdleTimeout
)

// ConfigureSessions sets the session lifetimes. Call once at startup, before
// serving. Non-positive values leave the corresponding default in place.
func ConfigureSessions(maxLifetime, idleTimeout time.Duration) {
	if maxLifetime > 0 {
		sessionMaxLifetime = maxLifetime
	}
	if idleTimeout > 0 {
		sessionIdleTimeout = idleTimeout
	}
}

// generateToken creates a crypto/rand 32-byte hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, tokenByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the hex-encoded SHA-256 of a raw token. Session tokens are
// stored hashed, matching invite links, magic links, and signup tokens: a read
// of the database file yields no directly replayable cookie.
func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// CreateSession inserts a new session and returns the raw token.
// Only the hash is persisted; the raw token exists solely in the cookie.
//
// The raw userAgent is stored so the session manager (issue #3) can render a
// coarse device label; see DeviceLabel. Callers pass r.UserAgent() from the
// login handler; "" is fine and renders as an unknown device.
func CreateSession(db *database.DB, userID, ip, userAgent string) (string, error) {
	rawToken, err := generateToken()
	if err != nil {
		return "", err
	}

	id := NewUUIDv7()
	now := time.Now().UTC()
	expiresAt := now.Add(sessionMaxLifetime).Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO sessions (id, user_id, token, expires_at, ip_address, user_agent, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, userID, HashToken(rawToken), expiresAt, ip, userAgent, now.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return rawToken, nil
}

// ValidateSession looks up a session by raw token and returns the associated
// user. It returns nil if the session is expired or not found.
//
// Expiry is two-sided (docs/adr/017): a session dies at whichever comes
// first, its absolute expires_at or last_used_at plus the idle timeout. On a
// successful validation the session's last_used_at is stamped, throttled to
// at most once per stampInterval so this does not write on every request.
//
// The role comes from the users join on every call, so authorization stays
// fresh even for a long-lived session — demoting an admin takes effect on
// their next request without rotating anything.
func ValidateSession(db *database.DB, rawToken string) (*model.User, error) {
	var user model.User
	var expiresAt, lastUsedAt string

	tokenHash := HashToken(rawToken)

	err := db.QueryRow(`
		SELECT u.id, COALESCE(u.email,''), u.username, u.display_name, u.bio, u.avatar_url, u.role, u.trusted_contributor, u.suspended_at, u.created_at, u.updated_at, s.expires_at, COALESCE(s.last_used_at,'')
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ?
	`, tokenHash).Scan(
		&user.ID, &user.Email, &user.Username, &user.DisplayName,
		&user.Bio, &user.AvatarURL, &user.Role, &user.TrustedContributor, &user.SuspendedAt, &user.CreatedAt, &user.UpdatedAt,
		&expiresAt, &lastUsedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validate session: %w", err)
	}

	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse session expiry: %w", err)
	}

	now := time.Now()
	if now.After(exp) {
		// Absolute ceiling reached — clean up.
		db.Exec("DELETE FROM sessions WHERE token = ?", tokenHash)
		return nil, nil
	}

	// A row with no parseable last_used_at (pre-migration, or written by
	// something that skipped the stamp) is treated as used right now rather
	// than as infinitely idle: failing open here only costs one idle window,
	// while failing closed would sign out every existing session on upgrade.
	lastUsed, lastUsedOK := parseTimestamp(lastUsedAt)
	if lastUsedOK && now.After(lastUsed.Add(sessionIdleTimeout)) {
		db.Exec("DELETE FROM sessions WHERE token = ?", tokenHash)
		return nil, nil
	}

	if !lastUsedOK || now.Sub(lastUsed) >= stampInterval {
		if _, err := db.Exec(
			"UPDATE sessions SET last_used_at = ? WHERE token = ?",
			now.UTC().Format(time.RFC3339), tokenHash,
		); err != nil {
			// A failed stamp costs freshness, not access. The session is
			// valid; do not fail the request over it.
			log.Printf("auth: stamp session last_used_at: %v", err)
		}
	}

	return &user, nil
}

// parseTimestamp reads a stored timestamp, tolerating both the RFC3339 form
// Go writes and the fractional-second form SQLite's strftime default writes.
func parseTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// GrantSudo opens a step-up window on the session presenting rawToken, after
// a fresh WebAuthn assertion has been verified. The window lives on the
// session row, so logging out ends it.
func GrantSudo(db *database.DB, rawToken string) (time.Time, error) {
	until := time.Now().Add(SudoWindow).UTC()
	res, err := db.Exec(
		"UPDATE sessions SET sudo_until = ? WHERE token = ?",
		until.Format(time.RFC3339), HashToken(rawToken),
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("grant sudo: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return time.Time{}, fmt.Errorf("no such session")
	}
	return until, nil
}

// SudoActive reports whether the session presenting rawToken is inside a live
// step-up window. A missing session, an unstamped column, or an elapsed
// window all read as false.
func SudoActive(db *database.DB, rawToken string) bool {
	var sudoUntil string
	err := db.QueryRow(
		"SELECT COALESCE(sudo_until,'') FROM sessions WHERE token = ?",
		HashToken(rawToken),
	).Scan(&sudoUntil)
	if err != nil {
		return false
	}
	until, ok := parseTimestamp(sudoUntil)
	return ok && time.Now().Before(until)
}

// HasCredential reports whether a user has at least one passkey enrolled.
// Step-up auth is a WebAuthn assertion, so an admin with no passkey cannot
// perform the gated actions until they enroll one — the admin UI uses this
// to say so before they hit that wall.
func HasCredential(db *database.DB, userID string) bool {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM credentials WHERE user_id = ?", userID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// DestroySession deletes a single session by its raw token.
func DestroySession(db *database.DB, rawToken string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE token = ?", HashToken(rawToken))
	return err
}

// DestroyUserSessions deletes every session belonging to a user, cutting off
// all live cookies immediately. Used when suspending an account: without this,
// a suspended admin keeps read access until their 30-day session lapses.
func DestroyUserSessions(db *database.DB, userID string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// DestroyOtherUserSessions deletes every session belonging to a user except the
// one presenting keepRawToken. Used when a credential is revoked: the actor doing
// the revoking stays signed in, every other session is cut.
func DestroyOtherUserSessions(db *database.DB, userID, keepRawToken string) error {
	_, err := db.Exec(
		"DELETE FROM sessions WHERE user_id = ? AND token != ?",
		userID, HashToken(keepRawToken),
	)
	return err
}

// SessionInfo is a single active session as shown in the session manager
// (issue #3). It carries no token or token-hash material — only what a person
// needs to recognize and decide about their own sessions.
type SessionInfo struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	Current    bool   `json:"current"`
}

// ListUserSessions returns the non-expired sessions belonging to userID, newest
// activity first, with the one presenting currentRawToken flagged Current.
//
// Expiry is evaluated the same two ways ValidateSession uses (docs/adr/017):
// a session is dropped from the list once it passes its absolute expires_at or
// its last_used_at plus the idle timeout, so the manager never offers to revoke
// something already dead. Filtering happens in Go because the idle timeout is a
// runtime config value, not a column. The token hash never leaves this function.
func ListUserSessions(db *database.DB, userID, currentRawToken string) ([]SessionInfo, error) {
	rows, err := db.Query(
		`SELECT id, COALESCE(user_agent,''), expires_at, COALESCE(last_used_at,''), COALESCE(created_at,''), token
		 FROM sessions WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	currentHash := HashToken(currentRawToken)
	now := time.Now()

	sessions := []SessionInfo{}
	for rows.Next() {
		var id, userAgent, expiresAt, lastUsedAt, createdAt, tokenHash string
		if err := rows.Scan(&id, &userAgent, &expiresAt, &lastUsedAt, &createdAt, &tokenHash); err != nil {
			continue
		}

		if exp, err := time.Parse(time.RFC3339, expiresAt); err == nil && now.After(exp) {
			continue
		}
		if lastUsed, ok := parseTimestamp(lastUsedAt); ok && now.After(lastUsed.Add(sessionIdleTimeout)) {
			continue
		}

		sessions = append(sessions, SessionInfo{
			ID:         id,
			Label:      DeviceLabel(userAgent),
			CreatedAt:  createdAt,
			LastUsedAt: lastUsedAt,
			Current:    tokenHash == currentHash,
		})
	}

	// Newest activity first; the current session tends to sort to the top,
	// which is where a person looks for it.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastUsedAt > sessions[j].LastUsedAt
	})

	return sessions, nil
}

// DestroyUserSession deletes one session, but only if it belongs to userID. It
// reports whether a row was actually deleted, so the handler can answer a
// non-owned or unknown id with 404 — a session id is not a capability to
// revoke someone else's session, and 404 (not 403) keeps another user's id
// indistinguishable from a nonexistent one.
func DestroyUserSession(db *database.DB, userID, sessionID string) (bool, error) {
	res, err := db.Exec(
		"DELETE FROM sessions WHERE id = ? AND user_id = ?",
		sessionID, userID,
	)
	if err != nil {
		return false, fmt.Errorf("destroy session: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SessionIDForToken returns the id of the session presenting rawToken, or ""
// if none matches. Used to tell whether a revoke targets the current session
// (which then behaves as logout).
func SessionIDForToken(db *database.DB, rawToken string) string {
	var id string
	db.QueryRow("SELECT id FROM sessions WHERE token = ?", HashToken(rawToken)).Scan(&id)
	return id
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
