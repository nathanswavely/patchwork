package auth

import (
	"errors"
	"strings"
	"testing"
)

// The bootstrap token gates the first account and nothing else
// (docs/adr/070). The vulnerability it closes is a race: the first account
// becomes instance admin, and magic-link signup is open registration, so a
// publicly reachable instance is claimable by whoever finds it first.

func TestBootstrapToken_ClaimNeedsTheToken(t *testing.T) {
	db := setupTestDB(t)
	useBootstrapToken(t)

	_, signup, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "stranger@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}

	// A stranger who found the address first, with no token.
	if _, err := CompleteSignup(db, signup, "stranger", "", ""); !errors.Is(err, ErrBootstrapToken) {
		t.Fatalf("claim with no token: err = %v, want ErrBootstrapToken", err)
	}
	// And with the wrong one.
	if _, err := CompleteSignup(db, signup, "stranger", "", "not-the-token"); !errors.Is(err, ErrBootstrapToken) {
		t.Fatalf("claim with wrong token: err = %v, want ErrBootstrapToken", err)
	}

	// Nothing was created by either attempt.
	if !NoUsersExist(db) {
		t.Fatal("a refused claim created an account")
	}
}

func TestBootstrapToken_OperatorClaimsWithIt(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	_, signup, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "operator@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	user, err := CompleteSignup(db, signup, "operator", "", boot)
	if err != nil {
		t.Fatalf("claim with the token: %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("role = %q, want admin", user.Role)
	}
}

// It gates bootstrap, never registration: the ordinary signup path on a
// running instance must not start demanding a token.
func TestBootstrapToken_DiesWithTheFirstAccount(t *testing.T) {
	db := setupTestDB(t)
	boot := useBootstrapToken(t)

	_, first, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "operator@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink (first): %v", err)
	}
	if _, err := CompleteSignup(db, first, "operator", "", boot); err != nil {
		t.Fatalf("first account: %v", err)
	}

	_, second, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "member@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink (second): %v", err)
	}
	user, err := CompleteSignup(db, second, "member-person", "", "")
	if err != nil {
		t.Fatalf("ordinary signup after bootstrap should not need a token: %v", err)
	}
	if user.Role != "member" {
		t.Errorf("role = %q, want member", user.Role)
	}
}

// An entry point that never installs a token leaves the gate shut rather
// than open. Failing closed is recoverable — the operator reads the log —
// where failing open is the vulnerability itself.
func TestBootstrapToken_UnsetFailsClosed(t *testing.T) {
	db := setupTestDB(t)
	SetBootstrapToken("")

	_, signup, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "whoever@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if _, err := CompleteSignup(db, signup, "whoever", "", ""); !errors.Is(err, ErrBootstrapToken) {
		t.Fatalf("err = %v, want ErrBootstrapToken", err)
	}
}

// The message has to send the operator somewhere, since an unconfigured
// instance generated its token into the log.
func TestBootstrapToken_ErrorNamesTheLog(t *testing.T) {
	if !strings.Contains(ErrBootstrapToken.Error(), "log") {
		t.Errorf("ErrBootstrapToken does not mention the log: %q", ErrBootstrapToken)
	}
}

func TestGenerateBootstrapToken_IsRandomAndLong(t *testing.T) {
	a, err := GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == b {
		t.Error("two generated tokens are equal")
	}
	if len(a) < 32 {
		t.Errorf("token length %d, want >= 32", len(a))
	}
}
