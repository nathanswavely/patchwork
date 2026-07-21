package auth

import (
	"testing"
	"time"
)

// The raw token must never be recoverable from the database file. A read of
// patchwork.db, or of a backup tarball, should yield nothing directly replayable.
func TestSessionTokenIsHashedAtRest(t *testing.T) {
	db := setupTestDB(t)

	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		userID, "session-test", "Session Test", now, now,
	); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rawToken, err := CreateSession(db, userID, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var stored string
	if err := db.QueryRow("SELECT token FROM sessions WHERE user_id = ?", userID).Scan(&stored); err != nil {
		t.Fatalf("read session: %v", err)
	}

	if stored == rawToken {
		t.Fatal("session token stored in plaintext")
	}
	if stored != HashToken(rawToken) {
		t.Fatalf("stored token is not the SHA-256 of the raw token")
	}

	// The raw token still authenticates.
	user, err := ValidateSession(db, rawToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if user == nil || user.ID != userID {
		t.Fatal("expected raw token to validate")
	}

	// The stored hash must not be usable as a cookie value.
	if u, _ := ValidateSession(db, stored); u != nil {
		t.Fatal("stored hash validated as a token — hashing gains nothing")
	}
}

func TestDestroyUserSessionsRevokesAll(t *testing.T) {
	db := setupTestDB(t)

	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		userID, "revoke-test", "Revoke Test", now, now,
	)

	first, _ := CreateSession(db, userID, "127.0.0.1")
	second, _ := CreateSession(db, userID, "127.0.0.2")

	if err := DestroyUserSessions(db, userID); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	for name, token := range map[string]string{"first": first, "second": second} {
		if u, _ := ValidateSession(db, token); u != nil {
			t.Fatalf("%s session survived bulk revocation", name)
		}
	}
}

// Revoking a credential cuts every session but the one making the request.
func TestDestroyOtherUserSessionsKeepsCurrent(t *testing.T) {
	db := setupTestDB(t)

	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		userID, "keep-test", "Keep Test", now, now,
	)

	current, _ := CreateSession(db, userID, "127.0.0.1")
	other, _ := CreateSession(db, userID, "127.0.0.2")

	if err := DestroyOtherUserSessions(db, userID, current); err != nil {
		t.Fatalf("destroy others: %v", err)
	}

	if u, _ := ValidateSession(db, current); u == nil {
		t.Fatal("current session was revoked")
	}
	if u, _ := ValidateSession(db, other); u != nil {
		t.Fatal("other session survived")
	}
}
