package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

func insertMagicLinkDB(t *testing.T, db *database.DB, email string) string {
	t.Helper()
	rawToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		NewUUIDv7(), email, tokenHash, expiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return rawToken
}

func TestFirstMagicLinkUserBecomesAdmin(t *testing.T) {
	db := setupTestDB(t)

	// Two-phase flow (docs/adr/013): verify issues a signup token; the
	// account (and the first-admin bootstrap) happens at CompleteSignup.
	_, signupA, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "founder@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink (first): %v", err)
	}
	first, err := CompleteSignup(db, signupA, "founder", "")
	if err != nil {
		t.Fatalf("CompleteSignup (first): %v", err)
	}
	if first.Role != "admin" {
		t.Errorf("first user role = %q, want admin", first.Role)
	}

	// Role must be persisted, not just on the returned struct.
	var stored string
	if err := db.QueryRow(`SELECT role FROM users WHERE id = ?`, first.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "admin" {
		t.Errorf("stored role = %q, want admin", stored)
	}

	_, signupB, err := VerifyMagicLink(db, insertMagicLinkDB(t, db, "second@example.com"))
	if err != nil {
		t.Fatalf("VerifyMagicLink (second): %v", err)
	}
	second, err := CompleteSignup(db, signupB, "second-person", "")
	if err != nil {
		t.Fatalf("CompleteSignup (second): %v", err)
	}
	if second.Role != "member" {
		t.Errorf("second user role = %q, want member", second.Role)
	}
}

func TestFirstInviteUserBecomesAdmin(t *testing.T) {
	db := setupTestDB(t)

	// A seeded invite on a fresh instance (e.g. created out-of-band).
	token, err := GenerateInviteLink(db, model.SystemUserID, 5, nil)
	if err != nil {
		t.Fatal(err)
	}

	first, err := RedeemInviteLink(db, token, "founder", "")
	if err != nil {
		t.Fatalf("RedeemInviteLink (first): %v", err)
	}
	if first.Role != "admin" {
		t.Errorf("first user role = %q, want admin", first.Role)
	}

	var stored string
	if err := db.QueryRow(`SELECT role FROM users WHERE id = ?`, first.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "admin" {
		t.Errorf("stored role = %q, want admin", stored)
	}

	second, err := RedeemInviteLink(db, token, "second", "")
	if err != nil {
		t.Fatalf("RedeemInviteLink (second): %v", err)
	}
	if second.Role != "member" {
		t.Errorf("second user role = %q, want member", second.Role)
	}
}

func TestNoUsersExist(t *testing.T) {
	db := setupTestDB(t)

	if !NoUsersExist(db) {
		t.Error("NoUsersExist = false on fresh DB, want true")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, username, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		NewUUIDv7(), "someone", "Someone", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	if NoUsersExist(db) {
		t.Error("NoUsersExist = true with a user present, want false")
	}
}
