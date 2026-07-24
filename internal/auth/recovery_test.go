package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func insertRecoveryUser(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		userID, username, username, now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return userID
}

func TestGenerateRecoveryCodes(t *testing.T) {
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, "mika")

	codes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), RecoveryCodeCount)
	}

	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate code in batch: %s", code)
		}
		seen[code] = true
		if len(code) != 14 || strings.Count(code, "-") != 2 {
			t.Errorf("code %q not in xxxx-xxxx-xxxx form", code)
		}
	}

	// Stored hashed, never raw.
	var rawCount int
	db.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE code = ?`, codes[0]).Scan(&rawCount)
	if rawCount != 0 {
		t.Error("raw code found in database — codes must be stored hashed")
	}

	total, remaining, err := CountRecoveryCodes(db, userID)
	if err != nil {
		t.Fatal(err)
	}
	if total != RecoveryCodeCount || remaining != RecoveryCodeCount {
		t.Errorf("counts = (%d, %d), want (%d, %d)", total, remaining, RecoveryCodeCount, RecoveryCodeCount)
	}
}

func TestRedeemRecoveryCode(t *testing.T) {
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, "mika")
	codes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatal(err)
	}

	user, err := RedeemRecoveryCode(db, "mika", codes[0])
	if err != nil {
		t.Fatalf("RedeemRecoveryCode: %v", err)
	}
	if user.ID != userID {
		t.Errorf("redeemed as user %s, want %s", user.ID, userID)
	}

	// Single use: the same code fails the second time.
	if _, err := RedeemRecoveryCode(db, "mika", codes[0]); err == nil {
		t.Fatal("expected reuse of a burned code to fail")
	}

	_, remaining, _ := CountRecoveryCodes(db, userID)
	if remaining != RecoveryCodeCount-1 {
		t.Errorf("remaining = %d, want %d", remaining, RecoveryCodeCount-1)
	}
}

func TestRedeemRecoveryCodeNormalization(t *testing.T) {
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, "noor")
	codes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatal(err)
	}

	// A code read off paper comes back with arbitrary case, spacing, and
	// hyphenation; username matching tolerates case and whitespace too.
	mangled := "  " + strings.ToUpper(strings.ReplaceAll(codes[1], "-", " ")) + " "
	if _, err := RedeemRecoveryCode(db, " Noor ", mangled); err != nil {
		t.Fatalf("normalized redeem failed: %v", err)
	}
}

func TestRedeemRecoveryCodeFailuresAreUniform(t *testing.T) {
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, "mika")
	if _, err := GenerateRecoveryCodes(db, userID); err != nil {
		t.Fatal(err)
	}

	_, errUnknownUser := RedeemRecoveryCode(db, "ghost", "aaaa-bbbb-cccc")
	_, errWrongCode := RedeemRecoveryCode(db, "mika", "aaaa-bbbb-cccc")
	if errUnknownUser == nil || errWrongCode == nil {
		t.Fatal("expected both failure shapes to error")
	}
	// Identical message, so redemption can't probe which usernames exist.
	if errUnknownUser.Error() != errWrongCode.Error() {
		t.Errorf("failure messages differ: %q vs %q", errUnknownUser, errWrongCode)
	}
}

func TestRegenerateReplacesOldBatch(t *testing.T) {
	db := setupTestDB(t)
	userID := insertRecoveryUser(t, db, "mika")

	oldCodes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatal(err)
	}
	newCodes, err := GenerateRecoveryCodes(db, userID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := RedeemRecoveryCode(db, "mika", oldCodes[0]); err == nil {
		t.Fatal("code from a replaced batch still redeems")
	}
	if _, err := RedeemRecoveryCode(db, "mika", newCodes[0]); err != nil {
		t.Fatalf("code from the current batch failed: %v", err)
	}

	total, _, _ := CountRecoveryCodes(db, userID)
	if total != RecoveryCodeCount {
		t.Errorf("total after regenerate = %d, want %d (old batch not replaced)", total, RecoveryCodeCount)
	}
}
