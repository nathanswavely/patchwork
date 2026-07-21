package auth

import (
	"io/fs"
	"os"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "patchwork-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func createTestAdmin(t *testing.T, db *database.DB) string {
	t.Helper()
	id := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, "admin_"+id[:8], "Test Admin", "admin", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGenerateAndRedeemInviteLink(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty token")
	}

	user, err := RedeemInviteLink(db, rawToken, "testuser", "Test User")
	if err != nil {
		t.Fatalf("RedeemInviteLink: %v", err)
	}
	if user.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", user.Username)
	}
	if user.DisplayName != "Test User" {
		t.Errorf("expected display name 'Test User', got %q", user.DisplayName)
	}
	if user.Role != "member" {
		t.Errorf("expected role 'member', got %q", user.Role)
	}
}

func TestInviteLinkExpiry(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	// Expired 1 hour ago.
	expired := time.Now().Add(-1 * time.Hour)
	rawToken, err := GenerateInviteLink(db, adminID, 1, &expired)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}

	_, err = RedeemInviteLink(db, rawToken, "expired_user", "")
	if err == nil {
		t.Fatal("expected error for expired invite link")
	}
	if err.Error() != "invite link has expired" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInviteLinkMaxUses(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}

	// First use: should succeed.
	_, err = RedeemInviteLink(db, rawToken, "user1", "")
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	// Second use: should fail.
	_, err = RedeemInviteLink(db, rawToken, "user2", "")
	if err == nil {
		t.Fatal("expected error for max uses exceeded")
	}
	if err.Error() != "invite link has reached maximum uses" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInviteLink(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}

	// Fresh link validates without consuming a use.
	if err := ValidateInviteLink(db, rawToken); err != nil {
		t.Fatalf("ValidateInviteLink: %v", err)
	}
	if err := ValidateInviteLink(db, rawToken); err != nil {
		t.Fatalf("second ValidateInviteLink: %v", err)
	}

	// Still redeemable after validation.
	if _, err := RedeemInviteLink(db, rawToken, "validated-user", ""); err != nil {
		t.Fatalf("redeem after validate: %v", err)
	}

	// Now exhausted.
	err = ValidateInviteLink(db, rawToken)
	if err == nil {
		t.Fatal("expected error for exhausted invite link")
	}
	if err.Error() != "invite link has reached maximum uses" {
		t.Errorf("unexpected error: %v", err)
	}

	// Unknown token.
	if err := ValidateInviteLink(db, "not-a-real-token"); err == nil {
		t.Fatal("expected error for invalid token")
	}

	// Expired token.
	expired := time.Now().Add(-1 * time.Hour)
	expiredToken, err := GenerateInviteLink(db, adminID, 1, &expired)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}
	err = ValidateInviteLink(db, expiredToken)
	if err == nil {
		t.Fatal("expected error for expired invite link")
	}
	if err.Error() != "invite link has expired" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInviteLinkInvalidToken(t *testing.T) {
	db := setupTestDB(t)

	_, err := RedeemInviteLink(db, "not-a-real-token", "baduser", "")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestRedeemInviteLinkUsernameRules(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	redeem := func(username string) error {
		rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
		if err != nil {
			t.Fatalf("GenerateInviteLink: %v", err)
		}
		_, err = RedeemInviteLink(db, rawToken, username, "")
		return err
	}

	// Mixed case is normalized, not rejected.
	rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	user, err := RedeemInviteLink(db, rawToken, "  MixedCase  ", "")
	if err != nil {
		t.Fatalf("mixed case should normalize: %v", err)
	}
	if user.Username != "mixedcase" {
		t.Errorf("expected normalized 'mixedcase', got %q", user.Username)
	}

	// Case-insensitive collision with the user just created.
	if err := redeem("MIXEDCASE"); err == nil {
		t.Fatal("expected 'taken' error for case-insensitive duplicate")
	}

	// Invalid formats.
	for _, bad := range []string{"ab", "has_underscore", "-leading", "trailing-", "has space", "Ünïcode", ""} {
		if err := redeem(bad); err == nil {
			t.Errorf("expected validation error for %q", bad)
		}
	}

	// Reserved names.
	if err := redeem("admin"); err == nil {
		t.Fatal("expected error for reserved username 'admin'")
	}

	// Collides with an existing node slug (shared acct: namespace).
	nodeID := NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug) VALUES (?, ?, 'Gallery Row', 'gallery-row')`,
		nodeID, adminID,
	); err != nil {
		t.Fatal(err)
	}
	if err := redeem("gallery-row"); err == nil {
		t.Fatal("expected error for username colliding with node slug")
	}
}

func TestInviteLinkMultipleUses(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	rawToken, err := GenerateInviteLink(db, adminID, 3, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}

	for i := 0; i < 3; i++ {
		_, err = RedeemInviteLink(db, rawToken, "multi-user-"+string(rune('a'+i)), "")
		if err != nil {
			t.Fatalf("redeem %d: %v", i+1, err)
		}
	}

	// Fourth use should fail.
	_, err = RedeemInviteLink(db, rawToken, "multi-user-d", "")
	if err == nil {
		t.Fatal("expected error for max uses exceeded")
	}
}
