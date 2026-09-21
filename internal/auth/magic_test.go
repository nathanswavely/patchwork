package auth

import (
	"crypto/sha256"
	"database/sql"
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
	created, err := CompleteSignup(db, signupToken, "Chosen-Name", "Chosen Person", useBootstrapToken(t))
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
	if _, err := CompleteSignup(db, signupToken, "second-try", "", ""); err == nil {
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

	if _, err := CompleteSignup(db, tokenA, "first-finisher", "", useBootstrapToken(t)); err != nil {
		t.Fatalf("first CompleteSignup: %v", err)
	}
	if _, err := CompleteSignup(db, tokenB, "second-finisher", "", ""); err == nil {
		t.Fatal("expected error when an account with the email already exists")
	}
}

func TestCompleteSignupInvalidToken(t *testing.T) {
	db := setupTestDB(t)
	if _, err := CompleteSignup(db, "not-a-real-token", "someone", "", ""); err == nil {
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

// --- Sign-in code -------------------------------------------------------
//
// The code is the same sign-in as the link, for a client that cannot be
// handed the link in its own session. These tests hold the two properties
// that make six digits safe to accept: the row stops answering after five
// wrong guesses (spending the link with it), and every refusal says one
// thing.

// insertCodedMagicLink stores a magic link with a known code, at a chosen
// expiry, and returns the raw token and the raw code.
func insertCodedMagicLink(t *testing.T, db *database.DB, email, code string, expiresIn time.Duration) string {
	t.Helper()
	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, code_hash, expires_at) VALUES (?, ?, ?, ?, ?)`,
		NewUUIDv7(), email, tokenHash, hashMagicCode(code),
		time.Now().Add(expiresIn).UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	return rawToken
}

func TestVerifyMagicCodeSignsInExistingUser(t *testing.T) {
	db := setupTestDB(t)
	email := "coded@example.com"
	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, email, "coded", "Coded Person", now, now,
	); err != nil {
		t.Fatal(err)
	}

	_, code, err := GenerateMagicLinkLocal(db, email)
	if err != nil {
		t.Fatalf("GenerateMagicLinkLocal: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code = %q, want six digits", code)
	}

	user, signupToken, err := VerifyMagicCode(db, email, code)
	if err != nil {
		t.Fatalf("VerifyMagicCode: %v", err)
	}
	if signupToken != "" {
		t.Errorf("expected no signup token for an existing account")
	}
	if user == nil || user.ID != userID {
		t.Fatalf("expected the existing account %s, got %+v", userID, user)
	}

	// One row, one use: the code just spent it, so it answers nothing else.
	if _, _, err := VerifyMagicCode(db, email, code); err != ErrInvalidMagicCode {
		t.Errorf("reusing a spent code: err = %v, want %v", err, ErrInvalidMagicCode)
	}
}

func TestVerifyMagicCodeAcceptsSpacedCode(t *testing.T) {
	db := setupTestDB(t)
	email := "spaces@example.com"

	_, code, err := GenerateMagicLinkLocal(db, email)
	if err != nil {
		t.Fatal(err)
	}

	// The email groups the digits for reading; the person types what they
	// see, spaces and all.
	spaced := formatMagicCode(code)
	if spaced == code {
		t.Fatalf("formatMagicCode did not group %q", code)
	}
	if _, signupToken, err := VerifyMagicCode(db, email, " "+spaced+" "); err != nil {
		t.Fatalf("VerifyMagicCode with a spaced code: %v", err)
	} else if signupToken == "" {
		t.Error("expected a signup token for an address with no account")
	}
}

func TestVerifyMagicCodeUnknownAddressAsksForAUsername(t *testing.T) {
	db := setupTestDB(t)
	email := "stranger@example.com"

	_, code, err := GenerateMagicLinkLocal(db, email)
	if err != nil {
		t.Fatal(err)
	}

	user, signupToken, err := VerifyMagicCode(db, email, code)
	if err != nil {
		t.Fatalf("VerifyMagicCode: %v", err)
	}
	// docs/adr/013: an unknown address gets a signup token, never an
	// account with a username derived from the email.
	if user != nil {
		t.Fatalf("expected no user, got %q", user.Username)
	}
	if signupToken == "" {
		t.Fatal("expected a signup token")
	}
	certified, err := ValidateSignupToken(db, signupToken)
	if err != nil {
		t.Fatalf("ValidateSignupToken: %v", err)
	}
	if certified != email {
		t.Errorf("signup token certifies %q, want %q", certified, email)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d accounts created before a username was chosen", count)
	}
}

// Five wrong codes spend the row, and the row is the link as well: both
// credentials prove one thing, so a guessed-at request stops answering
// altogether rather than leaving the emailed link live.
func TestVerifyMagicCodeFiveWrongGuessesBurnTheLink(t *testing.T) {
	db := setupTestDB(t)
	email := "guessed@example.com"
	rawToken := insertCodedMagicLink(t, db, email, "123456", 15*time.Minute)

	for i := 0; i < 5; i++ {
		if _, _, err := VerifyMagicCode(db, email, "000000"); err != ErrInvalidMagicCode {
			t.Fatalf("guess %d: err = %v, want %v", i+1, err, ErrInvalidMagicCode)
		}
	}

	var attempts, used int
	if err := db.QueryRow(`SELECT attempts, used FROM magic_links WHERE email = ?`, email).Scan(&attempts, &used); err != nil {
		t.Fatal(err)
	}
	if attempts != 5 {
		t.Errorf("attempts = %d, want 5", attempts)
	}
	if used != 1 {
		t.Errorf("used = %d, want the row spent after five wrong codes", used)
	}

	// The right code no longer works...
	if _, _, err := VerifyMagicCode(db, email, "123456"); err != ErrInvalidMagicCode {
		t.Errorf("after five guesses: err = %v, want %v", err, ErrInvalidMagicCode)
	}
	// ...and neither does the link that was mailed with it.
	if _, _, err := VerifyMagicLink(db, rawToken); err == nil || err.Error() != "magic link already used" {
		t.Errorf("link after five wrong codes: err = %v, want \"magic link already used\"", err)
	}
}

func TestVerifyMagicCodeExpired(t *testing.T) {
	db := setupTestDB(t)
	email := "stale@example.com"
	insertCodedMagicLink(t, db, email, "246813", -1*time.Hour)

	if _, _, err := VerifyMagicCode(db, email, "246813"); err != ErrInvalidMagicCode {
		t.Errorf("expired code: err = %v, want %v", err, ErrInvalidMagicCode)
	}
}

// A row minted before the codes migration has no code_hash, so no string is
// the code it was never sent. The link on that row still works.
func TestVerifyMagicCodeRejectsPreMigrationRow(t *testing.T) {
	db := setupTestDB(t)
	email := "legacy@example.com"
	rawToken := insertMagicLink(t, db, email, 15*time.Minute)

	var codeHash sql.NullString
	if err := db.QueryRow(`SELECT code_hash FROM magic_links WHERE email = ?`, email).Scan(&codeHash); err != nil {
		t.Fatal(err)
	}
	if codeHash.Valid {
		t.Fatalf("expected a NULL code_hash on a link minted without a code, got %q", codeHash.String)
	}

	for _, guess := range []string{"", "000000", "123456"} {
		if _, _, err := VerifyMagicCode(db, email, guess); err != ErrInvalidMagicCode {
			t.Errorf("guess %q against a codeless row: err = %v, want %v", guess, err, ErrInvalidMagicCode)
		}
	}
	// Nothing was counted against it, and the link is untouched.
	var attempts, used int
	if err := db.QueryRow(`SELECT attempts, used FROM magic_links WHERE email = ?`, email).Scan(&attempts, &used); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || used != 0 {
		t.Errorf("codeless row changed: attempts = %d, used = %d, want 0 and 0", attempts, used)
	}
	if _, _, err := VerifyMagicLink(db, rawToken); err != nil {
		t.Errorf("the link on a codeless row should still work: %v", err)
	}
}

// One answer for every way this can fail, so the endpoint cannot be asked
// whether an address has ever been used here.
func TestVerifyMagicCodeFailuresAreIndistinguishable(t *testing.T) {
	db := setupTestDB(t)
	insertCodedMagicLink(t, db, "known@example.com", "135790", 15*time.Minute)
	insertCodedMagicLink(t, db, "expired@example.com", "135790", -1*time.Hour)

	cases := []struct {
		name, email, code string
	}{
		{"wrong code for a real request", "known@example.com", "999999"},
		{"address nobody ever asked about", "never-asked@example.com", "135790"},
		{"expired row", "expired@example.com", "135790"},
		{"malformed address", "not-an-address", "135790"},
		{"empty code", "known@example.com", ""},
		{"non-numeric code", "known@example.com", "abcdef"},
	}
	for _, tc := range cases {
		_, _, err := VerifyMagicCode(db, tc.email, tc.code)
		if err != ErrInvalidMagicCode {
			t.Errorf("%s: err = %v, want the one generic %v", tc.name, err, ErrInvalidMagicCode)
		}
	}
}

// Uniform over the whole six-digit range, and zero-padded: "004271" is a
// code and "4271" is a different string to read back.
func TestGenerateMagicCodeShape(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 500; i++ {
		code, err := generateMagicCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 {
			t.Fatalf("code %q is not six characters", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("code %q is not all digits", code)
			}
		}
		seen[code]++
	}
	if len(seen) < 450 {
		t.Errorf("only %d distinct codes in 500 draws; the source is not random enough", len(seen))
	}
}
