package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// A recovery code proves presence too (docs/adr/099) — but only one the
// session could not have minted itself.

// stepUpFixture makes a user, a batch of codes, and a session, with the
// session's and the codes' ages under the test's control. `codeAge` and
// `sessionAge` are how long ago each was created.
func stepUpFixture(t *testing.T, username string, codeAge, sessionAge time.Duration) (*database.DB, string, string, []string) {
	t.Helper()
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, username)

	codes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatalf("generate codes: %v", err)
	}
	stamp := func(d time.Duration) string {
		return time.Now().UTC().Add(-d).Format("2006-01-02T15:04:05.000Z")
	}
	if _, err := db.Exec(`UPDATE recovery_codes SET created_at = ? WHERE user_id = ?`, stamp(codeAge), userID); err != nil {
		t.Fatal(err)
	}

	token, err := CreateSession(db, userID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET created_at = ? WHERE token = ?`, stamp(sessionAge), HashToken(token)); err != nil {
		t.Fatal(err)
	}
	return db, userID, token, codes
}

// The ordinary case: codes made last month, session opened this morning.
func TestStepUpWithRecoveryCode_CodeOlderThanTheSession(t *testing.T) {
	db, userID, token, codes := stepUpFixture(t, "olive", 30*24*time.Hour, time.Hour)

	remaining, err := StepUpWithRecoveryCode(db, userID, token, codes[0])
	if err != nil {
		t.Fatalf("a code from before this sign-in should work: %v", err)
	}
	if remaining != RecoveryCodeCount-1 {
		t.Errorf("remaining = %d, want %d", remaining, RecoveryCodeCount-1)
	}

	// Single use, here as everywhere else.
	if _, err := StepUpWithRecoveryCode(db, userID, token, codes[0]); !errors.Is(err, ErrInvalidRecoveryCode) {
		t.Errorf("a burnt code was accepted again: %v", err)
	}
	if n := UsableStepUpCodes(db, userID, token); n != RecoveryCodeCount-1 {
		t.Errorf("UsableStepUpCodes = %d, want %d", n, RecoveryCodeCount-1)
	}
}

// The security property. A batch minted inside this session is not a second
// factor: whoever holds the cookie could have minted it, so accepting it
// would let a stolen session reach the irreversible.
func TestStepUpWithRecoveryCode_RefusesCodesMintedInThisSession(t *testing.T) {
	db, userID, token, codes := stepUpFixture(t, "wendell", time.Hour, 24*time.Hour)

	_, err := StepUpWithRecoveryCode(db, userID, token, codes[0])
	if !errors.Is(err, ErrRecoveryCodesTooNew) {
		t.Fatalf("a code made during this session must be refused, got %v", err)
	}
	if n := UsableStepUpCodes(db, userID, token); n != 0 {
		t.Errorf("UsableStepUpCodes = %d, want 0 — none of these predate the session", n)
	}
	// And it is still unspent: a refusal must not burn the code.
	var used int
	db.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 1`, userID).Scan(&used)
	if used != 0 {
		t.Errorf("%d codes were burnt by a refusal", used)
	}
}

// The three failures are told apart, because the caller is already
// authenticated as this account and each one asks for a different move.
func TestStepUpWithRecoveryCode_DistinguishesItsRefusals(t *testing.T) {
	db, userID, token, codes := stepUpFixture(t, "bram", 30*24*time.Hour, time.Hour)

	if _, err := StepUpWithRecoveryCode(db, userID, token, "zzzz-zzzz-zzzz"); !errors.Is(err, ErrInvalidRecoveryCode) {
		t.Errorf("wrong code: got %v, want ErrInvalidRecoveryCode", err)
	}

	db.Exec(`DELETE FROM recovery_codes WHERE user_id = ?`, userID)
	if _, err := StepUpWithRecoveryCode(db, userID, token, codes[0]); !errors.Is(err, ErrNoRecoveryCodes) {
		t.Errorf("no codes at all: got %v, want ErrNoRecoveryCodes", err)
	}
}

// Formatting survives the round trip to paper and back, exactly as it does
// for sign-in redemption.
func TestStepUpWithRecoveryCode_AcceptsAReTypedCode(t *testing.T) {
	db, userID, token, codes := stepUpFixture(t, "pearl", 30*24*time.Hour, time.Hour)

	if _, err := StepUpWithRecoveryCode(db, userID, token, "  "+codes[0]+"  "); err != nil {
		t.Errorf("a code typed back with stray spaces was refused: %v", err)
	}
}

// One person's codes never open another person's window.
func TestStepUpWithRecoveryCode_IsPerAccount(t *testing.T) {
	db, _, token, codes := stepUpFixture(t, "iris", 30*24*time.Hour, time.Hour)
	stranger := insertRecoveryUser(t, db, "mallory")

	if _, err := StepUpWithRecoveryCode(db, stranger, token, codes[0]); !errors.Is(err, ErrNoRecoveryCodes) {
		t.Errorf("another account's code was considered: %v", err)
	}
}
