package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// listSessions drives GET /api/v1/auth/sessions for the session presenting token.
func listSessions(t *testing.T, db *database.DB, token string) []map[string]any {
	t.Helper()
	r := authedRequest("GET", "/api/v1/auth/sessions", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/auth/sessions", handler.ListSessions(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list sessions: status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode session list: %v", err)
	}
	return got
}

func TestListSessionsShowsOwnAndMarksCurrent(t *testing.T) {
	db := setupTestDB(t)
	// createTestUser makes one session; add a second on a different device.
	user, currentToken := createTestUser(t, db, "seer", "member")
	if _, err := auth.CreateSession(db, user.ID, "10.0.0.9",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit/605.1.15 Version/17.0 Mobile Safari/604.1"); err != nil {
		t.Fatalf("second session: %v", err)
	}

	got := listSessions(t, db, currentToken)
	if len(got) != 2 {
		t.Fatalf("session count = %d, want 2", len(got))
	}

	currentCount := 0
	for _, s := range got {
		if s["current"] == true {
			currentCount++
		}
		// No token or token-hash material may leak into the response.
		for k := range s {
			if k == "token" || k == "token_hash" {
				t.Errorf("session response leaked field %q", k)
			}
		}
		if _, ok := s["label"]; !ok {
			t.Errorf("session missing label: %v", s)
		}
	}
	if currentCount != 1 {
		t.Errorf("exactly one session should be current, got %d", currentCount)
	}
}

func TestListSessionsNeverShowsAnotherUsersSessions(t *testing.T) {
	db := setupTestDB(t)
	alice, aliceToken := createTestUser(t, db, "alice", "member")
	bob, _ := createTestUser(t, db, "bob", "member")
	// Bob has extra sessions; none of them may appear in alice's list.
	auth.CreateSession(db, bob.ID, "10.0.0.2", "bob-agent")
	auth.CreateSession(db, bob.ID, "10.0.0.3", "bob-agent")

	got := listSessions(t, db, aliceToken)
	if len(got) != 1 {
		t.Fatalf("alice sees %d sessions, want only her own (1)", len(got))
	}
	_ = alice
}

// revokeSession drives DELETE /api/v1/auth/sessions/{id}.
func revokeSession(t *testing.T, db *database.DB, token, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("DELETE", "/api/v1/auth/sessions/"+sessionID, nil, token)
	return serveMux(t, db, "DELETE", "/api/v1/auth/sessions/{id}", handler.RevokeSession(db), r)
}

func sessionIDForToken(t *testing.T, db *database.DB, rawToken string) string {
	t.Helper()
	id := auth.SessionIDForToken(db, rawToken)
	if id == "" {
		t.Fatalf("no session id for token")
	}
	return id
}

func TestRevokeSessionRemovesTargetSession(t *testing.T) {
	db := setupTestDB(t)
	user, currentToken := createTestUser(t, db, "cutter", "member")
	otherToken, err := auth.CreateSession(db, user.ID, "10.0.0.5", "other-agent")
	if err != nil {
		t.Fatalf("other session: %v", err)
	}
	otherID := sessionIDForToken(t, db, otherToken)

	w := revokeSession(t, db, currentToken, otherID)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	// The revoked session's token no longer validates.
	if u, _ := auth.ValidateSession(db, otherToken); u != nil {
		t.Errorf("revoked session still validates")
	}
	// The current session survives.
	if u, _ := auth.ValidateSession(db, currentToken); u == nil {
		t.Errorf("current session was destroyed by revoking another")
	}
}

func TestRevokeAnotherUsersSessionReturns404(t *testing.T) {
	db := setupTestDB(t)
	_, aliceToken := createTestUser(t, db, "alice2", "member")
	bob, bobToken := createTestUser(t, db, "bob2", "member")
	bobID := sessionIDForToken(t, db, bobToken)

	// Alice tries to revoke Bob's session by id: 404, not 403 — his id must be
	// indistinguishable from one that never existed.
	w := revokeSession(t, db, aliceToken, bobID)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user revoke: status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
	// Bob's session is untouched.
	if u, _ := auth.ValidateSession(db, bobToken); u == nil {
		t.Errorf("bob's session was destroyed by another user's revoke")
	}
	_ = bob
}

func TestRevokeCurrentSessionBehavesAsLogout(t *testing.T) {
	db := setupTestDB(t)
	user, currentToken := createTestUser(t, db, "selfcut", "member")
	currentID := sessionIDForToken(t, db, currentToken)

	w := revokeSession(t, db, currentToken, currentID)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke current: status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["was_current"] != true {
		t.Errorf("was_current = %v, want true", resp["was_current"])
	}
	// The session cookie is cleared (logout).
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("current-session revoke did not clear the session cookie")
	}
	if u, _ := auth.ValidateSession(db, currentToken); u != nil {
		t.Errorf("current session still validates after self-revoke")
	}
	_ = user
}

func TestRevokeOthersKeepsCurrentSession(t *testing.T) {
	db := setupTestDB(t)
	user, currentToken := createTestUser(t, db, "purger", "member")
	tokenB, _ := auth.CreateSession(db, user.ID, "10.0.0.6", "agent-b")
	tokenC, _ := auth.CreateSession(db, user.ID, "10.0.0.7", "agent-c")

	r := authedRequest("POST", "/api/v1/auth/sessions/revoke-others", nil, currentToken)
	w := serveMux(t, db, "POST", "/api/v1/auth/sessions/revoke-others", handler.RevokeOtherSessions(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke-others: status = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	if u, _ := auth.ValidateSession(db, currentToken); u == nil {
		t.Errorf("current session was cut by revoke-others")
	}
	if u, _ := auth.ValidateSession(db, tokenB); u != nil {
		t.Errorf("session B survived revoke-others")
	}
	if u, _ := auth.ValidateSession(db, tokenC); u != nil {
		t.Errorf("session C survived revoke-others")
	}

	// Only the current session remains in the list.
	got := listSessions(t, db, currentToken)
	if len(got) != 1 || got[0]["current"] != true {
		t.Errorf("after revoke-others, list = %v, want only the current session", got)
	}
}

// Sanity: the list route is reachable only with a session (AuthRequired), the
// same gate every other authed route uses.
func TestListSessionsRequiresAuth(t *testing.T) {
	db := setupTestDB(t)
	r := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/sessions", middleware.AuthRequired(db, handler.ListSessions(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: status = %d, want 401", w.Code)
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Errorf("unexpected token material in unauth response")
	}
}
