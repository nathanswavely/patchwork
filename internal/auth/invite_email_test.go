package auth

import (
	"database/sql"
	"strings"
	"testing"
)

// An invite-made account is the one that could end up with no way back in
// (docs/adr/071): no passkey if the next screen is skipped, and — before the
// email came with it — no address to send a link to either.

func TestRedeemInviteStoresEmail(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}

	user, err := RedeemInviteLink(db, rawToken, "withemail", "", "someone@example.com")
	if err != nil {
		t.Fatalf("RedeemInviteLink: %v", err)
	}
	if user.Email != "someone@example.com" {
		t.Errorf("returned email = %q, want someone@example.com", user.Email)
	}

	var stored sql.NullString
	if err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, user.ID).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !stored.Valid || stored.String != "someone@example.com" {
		t.Errorf("stored email = %v, want someone@example.com", stored)
	}
}

// email is UNIQUE and SQLite treats every empty string as equal, so storing the empty
// string would let exactly one address-less account exist and fail the second.
// NULL is what "no address" has to mean.
func TestRedeemInviteWithoutEmailStaysNull(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	for _, username := range []string{"noemail-one", "noemail-two"} {
		rawToken, err := GenerateInviteLink(db, adminID, 1, nil)
		if err != nil {
			t.Fatalf("GenerateInviteLink: %v", err)
		}
		user, err := RedeemInviteLink(db, rawToken, username, "", "")
		if err != nil {
			t.Fatalf("RedeemInviteLink(%s): %v", username, err)
		}

		var stored sql.NullString
		if err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, user.ID).Scan(&stored); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if stored.Valid {
			t.Errorf("%s: stored email = %q, want NULL", username, stored.String)
		}
	}
}

func TestRedeemInviteRejectsTakenEmail(t *testing.T) {
	db := setupTestDB(t)
	adminID := createTestAdmin(t, db)

	first, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}
	if _, err := RedeemInviteLink(db, first, "firstuser", "", "taken@example.com"); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	second, err := GenerateInviteLink(db, adminID, 1, nil)
	if err != nil {
		t.Fatalf("GenerateInviteLink: %v", err)
	}
	_, err = RedeemInviteLink(db, second, "seconduser", "", "taken@example.com")
	if err == nil {
		t.Fatal("second redeem with a taken address succeeded")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to mention an existing account", err.Error())
	}
}
