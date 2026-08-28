package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// setEmail drives the endpoint through its real gate chain: admin, then
// step-up (docs/adr/017).
func setEmail(t *testing.T, db *database.DB, targetID, email, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PUT", "/api/v1/admin/users/"+targetID+"/email",
		map[string]string{"email": email}, token)
	return serveSudoAdmin(db, "PUT", "/api/v1/admin/users/{id}/email",
		handler.SetUserEmail(db, testConfig()), r)
}

// storedEmail reads users.email back, treating NULL as "".
func storedEmail(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var email sql.NullString
	if err := db.QueryRow("SELECT email FROM users WHERE id = ?", id).Scan(&email); err != nil {
		t.Fatalf("read email: %v", err)
	}
	return email.String
}

// An address is a way into the account: whoever holds the mailbox can
// magic-link in. An admin cookie alone must not be enough to point one.
func TestSetUserEmailRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin", "admin")
	target, _ := createTestUser(t, db, "lockedout", "member")

	w := setEmail(t, db, target.ID, "rescue@example.com", token)

	if w.Code != http.StatusForbidden {
		t.Fatalf("set email without step-up returned %d, want 403: %s", w.Code, w.Body.String())
	}
	if got := storedEmail(t, db, target.ID); got != "" {
		t.Fatalf("email is %q — the write happened despite the 403", got)
	}
}

// The gate has to be passable, not merely present. This is the lockout
// repair: an account with no address at all gets one.
func TestSetUserEmailSucceedsWithStepUp(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin2", "admin")
	target, _ := createTestUser(t, db, "lockedout2", "member")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := setEmail(t, db, target.ID, "rescue@example.com", token)

	if w.Code != http.StatusOK {
		t.Fatalf("set email returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := storedEmail(t, db, target.ID); got != "rescue@example.com" {
		t.Fatalf("email is %q, want rescue@example.com", got)
	}
}

// Sign-in looks the account up with an exact `WHERE email = ?`, so an address
// stored as the admin happened to type it is an address the person cannot
// sign in with. Normalize on the way in.
func TestSetUserEmailNormalizesBeforeStoring(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin3", "admin")
	target, _ := createTestUser(t, db, "typo", "member")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := setEmail(t, db, target.ID, "  HSwavely@Example.COM \n", token)

	if w.Code != http.StatusOK {
		t.Fatalf("set email returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := storedEmail(t, db, target.ID); got != "hswavely@example.com" {
		t.Fatalf("stored %q, want hswavely@example.com", got)
	}

	// And the caller is told the canonical form, so the admin can tell the
	// person what to type rather than guessing from what they submitted.
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if body["email"] != "hswavely@example.com" {
		t.Fatalf("response email is %q, want the normalized form", body["email"])
	}
}

// users.email is UNIQUE. The person on the other end of this should read a
// sentence, not "UNIQUE constraint failed: users.email".
func TestSetUserEmailRejectsAddressHeldByAnotherAccount(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin4", "admin")
	holder, _ := createTestUser(t, db, "alreadyhere", "member")
	target, _ := createTestUser(t, db, "hopeful", "member")

	if _, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", "taken@example.com", holder.ID); err != nil {
		t.Fatalf("seed holder email: %v", err)
	}
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := setEmail(t, db, target.ID, "taken@example.com", token)

	if w.Code != http.StatusConflict {
		t.Fatalf("collision returned %d, want 409: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "UNIQUE") {
		t.Fatalf("error leaked the constraint violation: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alreadyhere") {
		t.Fatalf("error does not say which account holds it: %s", w.Body.String())
	}
	if got := storedEmail(t, db, target.ID); got != "" {
		t.Fatalf("target email is %q — the rejected write happened anyway", got)
	}
}

// Rows written before normalization existed can hold a mixed-case address.
// The UNIQUE index would let a lowercase twin sit beside it, which is the
// collision the check exists to prevent, so the check is case-insensitive.
func TestSetUserEmailRejectsAddressHeldInDifferentCase(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin5", "admin")
	holder, _ := createTestUser(t, db, "oldrow", "member")
	target, _ := createTestUser(t, db, "hopeful2", "member")

	if _, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", "Taken@Example.com", holder.ID); err != nil {
		t.Fatalf("seed holder email: %v", err)
	}
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := setEmail(t, db, target.ID, "taken@example.com", token)

	if w.Code != http.StatusConflict {
		t.Fatalf("case-differing collision returned %d, want 409: %s", w.Code, w.Body.String())
	}
	if got := storedEmail(t, db, target.ID); got != "" {
		t.Fatalf("target email is %q — two accounts now share an address", got)
	}
}

// Re-setting the address a user already has is a no-op, not a change: it
// must not write an audit entry claiming one happened.
func TestSetUserEmailUnchangedIsNotAudited(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin6", "admin")
	target, _ := createTestUser(t, db, "settled", "member")

	if _, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", "same@example.com", target.ID); err != nil {
		t.Fatalf("seed email: %v", err)
	}
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	if w := setEmail(t, db, target.ID, "SAME@example.com", token); w.Code != http.StatusOK {
		t.Fatalf("no-op set returned %d, want 200: %s", w.Code, w.Body.String())
	}

	var n int
	db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action = 'admin.user_email_set'").Scan(&n)
	if n != 0 {
		t.Fatalf("%d audit entries for a no-op", n)
	}
}

func TestSetUserEmailRejectsMalformedAddress(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin7", "admin")
	target, _ := createTestUser(t, db, "victim", "member")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	for _, bad := range []string{"", "   ", "hswavely", "hswavely@", "@example.com", "Sam <sam@example.com>", "one@example.com, two@example.com"} {
		w := setEmail(t, db, target.ID, bad, token)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%q returned %d, want 400: %s", bad, w.Code, w.Body.String())
		}
	}
	if got := storedEmail(t, db, target.ID); got != "" {
		t.Fatalf("email is %q after only malformed submissions", got)
	}
}

func TestSetUserEmailUnknownUserIsNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin8", "admin")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	w := setEmail(t, db, auth.NewUUIDv7(), "nobody@example.com", token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown user returned %d, want 404: %s", w.Code, w.Body.String())
	}
}

// The audit entry is the record someone consults when asking whether an
// admin pointed an account at a mailbox they control, so it needs its own
// action name and both addresses.
func TestSetUserEmailIsAudited(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "email-admin9", "admin")
	target, _ := createTestUser(t, db, "moved", "member")

	if _, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", "old@example.com", target.ID); err != nil {
		t.Fatalf("seed email: %v", err)
	}
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	if w := setEmail(t, db, target.ID, "new@example.com", token); w.Code != http.StatusOK {
		t.Fatalf("set email returned %d: %s", w.Code, w.Body.String())
	}

	var actorID, entityID, metadata string
	err := db.QueryRow(
		`SELECT user_id, entity_id, metadata FROM audit_log WHERE action = 'admin.user_email_set'`,
	).Scan(&actorID, &entityID, &metadata)
	if err != nil {
		t.Fatalf("no admin.user_email_set audit entry: %v", err)
	}
	if actorID != admin.ID {
		t.Fatalf("audit actor is %q, want the admin who acted", actorID)
	}
	if entityID != target.ID {
		t.Fatalf("audit entity is %q, want the target user", entityID)
	}

	var meta map[string]string
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatalf("audit metadata %q is not JSON: %v", metadata, err)
	}
	if meta["old_email"] != "old@example.com" || meta["new_email"] != "new@example.com" {
		t.Fatalf("audit metadata does not record both addresses: %v", meta)
	}
}

// The account's owner learns their address changed from inside the product,
// not only from whatever mail did or did not send.
func TestSetUserEmailNotifiesTheAccountHolder(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "email-admin10", "admin")
	target, _ := createTestUser(t, db, "notified", "member")

	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	if w := setEmail(t, db, target.ID, "told@example.com", token); w.Code != http.StatusOK {
		t.Fatalf("set email returned %d: %s", w.Code, w.Body.String())
	}

	var n int
	db.QueryRow(
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'account.email_changed'`,
		target.ID,
	).Scan(&n)
	if n != 1 {
		t.Fatalf("%d account.email_changed notifications for the target, want 1", n)
	}
}
