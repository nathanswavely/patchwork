package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// An account made through an invite used to be able to leave signup holding
// nothing — no passkey (the next screen offered "Skip for now") and no address
// to mail a link to. That is a permanent lockout no admin can undo, and it
// happened in production. Where mail can be sent, the address is now required
// (docs/adr/071).

func redeemInvite(t *testing.T, db *database.DB, cfg *config.Config, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/api/v1/auth/invite", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.RedeemInviteLink(db, cfg)(w, r)

	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func inviteToken(t *testing.T, db *database.DB) string {
	t.Helper()
	admin, _ := createTestUser(t, db, "inviter", "admin")
	token, err := auth.GenerateInviteLink(db, admin.ID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}
	return token
}

func TestInviteRequiresEmailWhenSMTPConfigured(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{SMTP: config.SMTP{Host: "smtp.test", From: "quilt@test"}}

	code, out := redeemInvite(t, db, cfg, map[string]any{
		"token": inviteToken(t, db), "username": "noaddress",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — an emailless account here is a lockout", code)
	}
	if msg, _ := out["error"].(string); msg == "" {
		t.Error("no error message explaining why")
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = 'noaddress'`).Scan(&n)
	if n != 0 {
		t.Error("account was created despite the rejection")
	}
}

func TestInviteStoresNormalizedEmail(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{SMTP: config.SMTP{Host: "smtp.test", From: "quilt@test"}}

	code, _ := redeemInvite(t, db, cfg, map[string]any{
		"token": inviteToken(t, db), "username": "shouty", "email": "  SHOUTY@Example.COM ",
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	// Stored folded, because the sign-in lookup is an exact match on whatever
	// they type next time.
	var email sql.NullString
	db.QueryRow(`SELECT email FROM users WHERE username = 'shouty'`).Scan(&email)
	if email.String != "shouty@example.com" {
		t.Errorf("stored email = %q, want shouty@example.com", email.String)
	}
}

func TestInviteRejectsMalformedEmail(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{SMTP: config.SMTP{Host: "smtp.test", From: "quilt@test"}}

	code, _ := redeemInvite(t, db, cfg, map[string]any{
		"token": inviteToken(t, db), "username": "typo", "email": "not-an-address",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}

// Without SMTP there is nothing to send, so demanding an address would only
// collect one that could never be used — and would block the offline-invite
// case the invite link exists for. Recovery codes are the floor there instead.
func TestInviteAllowsNoEmailWithoutSMTP(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}

	code, out := redeemInvite(t, db, cfg, map[string]any{
		"token": inviteToken(t, db), "username": "offline",
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %v)", code, out)
	}

	var email sql.NullString
	db.QueryRow(`SELECT email FROM users WHERE username = 'offline'`).Scan(&email)
	if email.Valid {
		t.Errorf("stored email = %q, want NULL", email.String)
	}
}
