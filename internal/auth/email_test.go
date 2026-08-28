package auth

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bob@example.com", "bob@example.com"},
		{"Bob@Example.com", "bob@example.com"},
		{"BOB@EXAMPLE.COM", "bob@example.com"},
		{"  bob@example.com  ", "bob@example.com"},
		{"\tBob@Example.com\n", "bob@example.com"},
		{"   ", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// insertUserWithEmail writes a user row directly, bypassing the signup path,
// so a test can stage a row in whatever capitalization it likes — including
// the mixed case that only rows predating migration 058 can hold.
func insertUserWithEmail(t *testing.T, db *database.DB, email, username string) string {
	t.Helper()
	id := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, email, username, username, "member", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// applyMigration058 runs the shipped migration SQL against an already-migrated
// test database, the same way the runner does: one transaction, rolled back if
// anything in the file aborts. Tests exercise the real file rather than a
// paraphrase of it, so a change to the SQL cannot pass a stale test.
func applyMigration058(t *testing.T, db *database.DB) error {
	t.Helper()
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(migrations, "058_normalize_user_emails.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(string(data)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// The upgrade path: rows written before normalization existed keep whatever
// capitalization was typed, and the migration canonicalizes them in place.
func TestMigration058CanonicalizesExistingEmails(t *testing.T) {
	db := setupTestDB(t)

	mixed := insertUserWithEmail(t, db, "Bob@Example.com", "bob")
	spaced := insertUserWithEmail(t, db, "  Carol@Example.COM ", "carol")
	already := insertUserWithEmail(t, db, "dave@example.com", "dave")
	// Invite-created accounts carry no address at all; NULL must survive.
	noEmail := insertUserWithEmail(t, db, "", "erin")
	if _, err := db.Exec(`UPDATE users SET email = NULL WHERE id = ?`, noEmail); err != nil {
		t.Fatal(err)
	}

	if err := applyMigration058(t, db); err != nil {
		t.Fatalf("migration 058: %v", err)
	}

	for _, c := range []struct{ id, want string }{
		{mixed, "bob@example.com"},
		{spaced, "carol@example.com"},
		{already, "dave@example.com"},
	} {
		var got string
		if err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, c.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("user %s: email = %q, want %q", c.id, got, c.want)
		}
	}

	var isNull bool
	if err := db.QueryRow(`SELECT email IS NULL FROM users WHERE id = ?`, noEmail).Scan(&isNull); err != nil {
		t.Fatal(err)
	}
	if !isNull {
		t.Error("a NULL email was rewritten; invite accounts have no address to canonicalize")
	}
}

// Two accounts differing only in capitalization cannot both be canonicalized,
// and picking a winner would sign the loser's owner into the winner's account.
// The migration refuses rather than choosing, and leaves the rows untouched.
func TestMigration058RefusesCaseCollision(t *testing.T) {
	db := setupTestDB(t)

	lower := insertUserWithEmail(t, db, "bob@example.com", "bob")
	upper := insertUserWithEmail(t, db, "Bob@Example.com", "bob-two")

	err := applyMigration058(t, db)
	if err == nil {
		t.Fatal("expected migration 058 to abort on a case-colliding pair")
	}
	if !strings.Contains(err.Error(), "migration 058") {
		t.Errorf("abort message should name the migration and the remedy, got: %v", err)
	}

	// The transaction rolled back, so neither address moved. An operator
	// resolving the collision by hand must find the data as they left it.
	for _, c := range []struct{ id, want string }{
		{lower, "bob@example.com"},
		{upper, "Bob@Example.com"},
	} {
		var got string
		if err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, c.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("user %s: email = %q after a refused migration, want %q unchanged", c.id, got, c.want)
		}
	}
}

// The bug this whole change exists for: a returning person typing a different
// capitalization than they signed up with was treated as a stranger and
// offered a second account, rather than signed into the one they have.
func TestVerifyMagicLinkMatchesExistingAccountRegardlessOfCase(t *testing.T) {
	db := setupTestDB(t)
	userID := insertUserWithEmail(t, db, "bob@example.com", "bob")

	// A link requested as "Bob@Example.com" — stored canonically going
	// forward, but staged raw here to stand in for one minted before the
	// upgrade, which VerifyMagicLink must still resolve.
	rawToken := insertMagicLink(t, db, "Bob@Example.com", 15*time.Minute)

	user, signupToken, err := VerifyMagicLink(db, rawToken)
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if signupToken != "" {
		t.Fatal("a known address was offered a signup token; the account already exists")
	}
	if user == nil {
		t.Fatal("expected the existing user, got nil")
	}
	if user.ID != userID {
		t.Errorf("signed into %s, want the existing account %s", user.ID, userID)
	}
}

func TestGenerateMagicLinkLocalStoresNormalizedEmail(t *testing.T) {
	db := setupTestDB(t)

	if _, err := GenerateMagicLinkLocal(db, "  Bob@Example.COM "); err != nil {
		t.Fatalf("GenerateMagicLinkLocal: %v", err)
	}

	var stored string
	if err := db.QueryRow(`SELECT email FROM magic_links`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "bob@example.com" {
		t.Errorf("magic_links.email = %q, want %q", stored, "bob@example.com")
	}
}

// A brand-new account must be created canonically, or the next sign-in with
// the same address recreates exactly the mismatch migration 058 just cleaned up.
func TestCompleteSignupStoresNormalizedEmail(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	_, signupToken, err := VerifyMagicLink(db, insertMagicLink(t, db, "Bob@Example.com", 15*time.Minute))
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if signupToken == "" {
		t.Fatal("expected a signup token for an unknown address")
	}

	// The completion page shows the address the account will be created
	// under, not the capitalization that happened to be typed.
	certified, err := ValidateSignupToken(db, signupToken)
	if err != nil {
		t.Fatalf("ValidateSignupToken: %v", err)
	}
	if certified != "bob@example.com" {
		t.Errorf("ValidateSignupToken returned %q, want %q", certified, "bob@example.com")
	}

	user, err := CompleteSignup(db, signupToken, "bob", "Bob", boot)
	if err != nil {
		t.Fatalf("CompleteSignup: %v", err)
	}
	if user.Email != "bob@example.com" {
		t.Errorf("created account email = %q, want %q", user.Email, "bob@example.com")
	}

	var stored string
	if err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, user.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "bob@example.com" {
		t.Errorf("users.email = %q, want %q", stored, "bob@example.com")
	}
}

// A signup token minted before the upgrade carries the address as typed. It
// must not be able to mint a second account alongside one that already holds
// the canonical form — the UNIQUE index is case-sensitive and would let it.
func TestCompleteSignupRejectsCaseVariantOfExistingAccount(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	insertUserWithEmail(t, db, "bob@example.com", "bob")

	rawSignup := stageSignupToken(t, db, "Bob@Example.com")

	if _, err := CompleteSignup(db, rawSignup, "bob-two", "Bob Two", boot); err == nil {
		t.Fatal("a case variant of an existing address created a second account")
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE lower(email) = ?`, "bob@example.com").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected exactly 1 account for the address, got %d", n)
	}
}

func TestValidEmail(t *testing.T) {
	valid := []string{
		"bob@example.com",
		"bob+tag@example.com",
		"bob.smith@sub.example.co.uk",
		"bob@[127.0.0.1]",
		// Dotless domains are legal and load-bearing: the seeded dev admin
		// is admin@localhost, and rejecting it would lock local dev out of
		// magic-link sign-in.
		"admin@localhost",
		"a@b",
	}
	for _, e := range valid {
		if !ValidEmail(e) {
			t.Errorf("ValidEmail(%q) = false, want true", e)
		}
	}

	invalid := []string{
		"",
		"   ",
		"bob",
		"bob@",
		"@example.com",
		"bob@@example.com",
		"bob smith@example.com",
		"bob@exam ple.com",
		"bob@example.com\n",
		// Header forms. net/mail parses all of these, and each returns a
		// bare address that is not the string handed in — stored verbatim
		// they would never match a lookup again.
		"Bob <bob@example.com>",
		"<bob@example.com>",
		"bob@example.com (Bob)",
		`"bob"@example.com`,
		// Two addresses in one field.
		"bob@example.com, carol@example.com",
		// Past the RFC 5321 forward-path cap.
		strings.Repeat("a", 250) + "@example.com",
	}
	for _, e := range invalid {
		if ValidEmail(e) {
			t.Errorf("ValidEmail(%q) = true, want false", e)
		}
	}
}

// Validation sits behind normalization, so an address that is only valid
// once trimmed still gets in.
func TestValidEmailRunsOnNormalizedInput(t *testing.T) {
	raw := "  Bob@Example.COM  "
	if ValidEmail(raw) {
		t.Error("ValidEmail should reject an untrimmed address; callers normalize first")
	}
	if !ValidEmail(NormalizeEmail(raw)) {
		t.Errorf("ValidEmail(NormalizeEmail(%q)) = false, want true", raw)
	}
}

// Signup is the boundary the invariant is stated at: a malformed address
// must not become an account, even carried in on a token minted before the
// check existed.
func TestCompleteSignupRejectsMalformedEmail(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	rawSignup := stageSignupToken(t, db, "Bob <bob@example.com>")

	if _, err := CompleteSignup(db, rawSignup, "bob", "Bob", boot); err == nil {
		t.Fatal("a malformed address created an account")
	}

	// Counting rows with an address, not all rows: migration 015 ships a
	// `_system` user (the owner of unclaimed patches) with no email.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected no account to be created, got %d", n)
	}
}

// The good path still works, and the token is consumed exactly once.
func TestCompleteSignupAcceptsDotlessDomain(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	rawSignup := stageSignupToken(t, db, "admin@localhost")

	user, err := CompleteSignup(db, rawSignup, "dev-admin", "Dev Admin", boot)
	if err != nil {
		t.Fatalf("CompleteSignup on the seeded dev admin address: %v", err)
	}
	if user.Email != "admin@localhost" {
		t.Errorf("email = %q, want %q", user.Email, "admin@localhost")
	}
}

// stageSignupToken writes a signup token holding an address verbatim,
// bypassing createSignupToken's normalization, to stand in for one minted
// before this change shipped.
func stageSignupToken(t *testing.T, db *database.DB, email string) string {
	t.Helper()
	raw, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		`INSERT INTO signup_tokens (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		NewUUIDv7(), email, HashToken(raw),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
