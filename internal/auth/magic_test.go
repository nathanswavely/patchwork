package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// insertMagicLink stores a hashed magic link directly (bypassing SMTP send)
// and returns the raw token.
func insertMagicLink(t *testing.T, db *database.DB, email string, expiresIn time.Duration) string {
	t.Helper()
	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(expiresIn).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, email, tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return rawToken
}

func TestVerifyMagicLinkNewEmailTwoPhase(t *testing.T) {
	db := setupTestDB(t)
	email := "test@example.com"
	rawToken := insertMagicLink(t, db, email, 15*time.Minute)

	// Phase 1: verifying an unknown email creates NO user — it returns a
	// signup token instead (docs/adr/013: usernames are chosen, never
	// derived from the email).
	user, signupToken, err := VerifyMagicLink(db, rawToken)
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if user != nil {
		t.Fatalf("expected no user for new email, got %q", user.Username)
	}
	if signupToken == "" {
		t.Fatal("expected a signup token for new email")
	}

	var userCount int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&userCount)
	if userCount != 0 {
		t.Fatalf("expected no user row before signup completes, got %d", userCount)
	}

	// The token certifies the email.
	gotEmail, err := ValidateSignupToken(db, signupToken)
	if err != nil {
		t.Fatalf("ValidateSignupToken: %v", err)
	}
	if gotEmail != email {
		t.Errorf("expected email %q, got %q", email, gotEmail)
	}

	// Phase 2: completing signup with a chosen username creates the account.
	created, err := CompleteSignup(db, signupToken, "Chosen-Name", "Chosen Person")
	if err != nil {
		t.Fatalf("CompleteSignup: %v", err)
	}
	if created.Username != "chosen-name" {
		t.Errorf("expected normalized username 'chosen-name', got %q", created.Username)
	}
	if created.Email != email {
		t.Errorf("expected email %q, got %q", email, created.Email)
	}
	if created.Role != "admin" {
		t.Errorf("first account should bootstrap as admin, got %q", created.Role)
	}

	// The signup token is single-use.
	if _, err := CompleteSignup(db, signupToken, "second-try", ""); err == nil {
		t.Fatal("expected error reusing a consumed signup token")
	}
}

func TestCompleteSignupRaceOnEmail(t *testing.T) {
	db := setupTestDB(t)
	email := "race@example.com"

	// Two magic links verified for the same address → two signup tokens.
	_, tokenA, err := VerifyMagicLink(db, insertMagicLink(t, db, email, 15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, tokenB, err := VerifyMagicLink(db, insertMagicLink(t, db, email, 15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := CompleteSignup(db, tokenA, "first-finisher", ""); err != nil {
		t.Fatalf("first CompleteSignup: %v", err)
	}
	if _, err := CompleteSignup(db, tokenB, "second-finisher", ""); err == nil {
		t.Fatal("expected error when an account with the email already exists")
	}
}

func TestCompleteSignupInvalidToken(t *testing.T) {
	db := setupTestDB(t)
	if _, err := CompleteSignup(db, "not-a-real-token", "someone", ""); err == nil {
		t.Fatal("expected error for invalid signup token")
	}
}

func TestVerifyMagicLinkExpired(t *testing.T) {
	db := setupTestDB(t)

	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, "expired@example.com", tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = VerifyMagicLink(db, rawToken)
	if err == nil {
		t.Fatal("expected error for expired magic link")
	}
	if err.Error() != "magic link has expired" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyMagicLinkAlreadyUsed(t *testing.T) {
	db := setupTestDB(t)

	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at, used) VALUES (?, ?, ?, ?, 1)`,
		id, "used@example.com", tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = VerifyMagicLink(db, rawToken)
	if err == nil {
		t.Fatal("expected error for already used magic link")
	}
	if err.Error() != "magic link already used" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyMagicLinkInvalidToken(t *testing.T) {
	db := setupTestDB(t)

	_, _, err := VerifyMagicLink(db, "not-a-real-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestVerifyMagicLinkExistingUser(t *testing.T) {
	db := setupTestDB(t)

	// Create an existing user with the email.
	email := "existing@example.com"
	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, email, "existing", "Existing User", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create a magic link for the same email.
	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, email, tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	user, signupToken, err := VerifyMagicLink(db, rawToken)
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if signupToken != "" {
		t.Errorf("expected no signup token for existing user, got one")
	}
	// Should return the existing user, not create a new one.
	if user.ID != userID {
		t.Errorf("expected existing user ID %q, got %q", userID, user.ID)
	}
	if user.Username != "existing" {
		t.Errorf("expected username 'existing', got %q", user.Username)
	}
}
