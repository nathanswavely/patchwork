package ap_test

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

func TestPrivateKeyForActor(t *testing.T) {
	db := setupTestDB(t)

	// Insert a user with an ap_id and a generated keypair.
	userID := "user-key-1"
	userAPID := "https://test.example.com/ap/users/" + userID
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, ap_id, created_at, updated_at)
		 VALUES (?, 'k@example.com', 'keyuser', 'Key User', 'member', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		userID, userAPID,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, _, err := ap.EnsureUserKeypair(db, userID); err != nil {
		t.Fatalf("ensure keypair: %v", err)
	}

	keyID, priv, err := ap.PrivateKeyForActor(db, userAPID)
	if err != nil {
		t.Fatalf("PrivateKeyForActor: %v", err)
	}
	if keyID != userAPID+"#main-key" {
		t.Errorf("expected keyID %q, got %q", userAPID+"#main-key", keyID)
	}
	if priv == "" {
		t.Error("expected non-empty private key")
	}
}

func TestPrivateKeyForActor_NotLocal(t *testing.T) {
	db := setupTestDB(t)

	_, _, err := ap.PrivateKeyForActor(db, "https://remote.example/ap/users/nobody")
	if err == nil {
		t.Fatal("expected error for unknown remote actor, got nil")
	}
}

func TestPrivateKeyForActor_NoKey(t *testing.T) {
	db := setupTestDB(t)

	userID := "user-nokey-1"
	userAPID := "https://test.example.com/ap/users/" + userID
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, ap_id, created_at, updated_at)
		 VALUES (?, 'nk@example.com', 'nokeyuser', 'No Key', 'member', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		userID, userAPID,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, _, err := ap.PrivateKeyForActor(db, userAPID)
	if err == nil {
		t.Fatal("expected error for actor without a private key, got nil")
	}
}
