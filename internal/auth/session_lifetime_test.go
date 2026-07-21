package auth

import (
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// newSessionUser creates a user to hang sessions off.
func newSessionUser(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	id := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		id, username, username, now, now,
	); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

// withLifetimes runs fn with the session lifetimes temporarily overridden,
// restoring the process globals afterwards.
func withLifetimes(t *testing.T, maxLifetime, idleTimeout time.Duration, fn func()) {
	t.Helper()
	oldMax, oldIdle := sessionMaxLifetime, sessionIdleTimeout
	t.Cleanup(func() { sessionMaxLifetime, sessionIdleTimeout = oldMax, oldIdle })
	sessionMaxLifetime, sessionIdleTimeout = maxLifetime, idleTimeout
	fn()
}

// setLastUsed backdates a session's last use, standing in for time passing.
func setLastUsed(t *testing.T, db *database.DB, rawToken string, at time.Time) {
	t.Helper()
	if _, err := db.Exec(
		"UPDATE sessions SET last_used_at = ? WHERE token = ?",
		at.UTC().Format(time.RFC3339), HashToken(rawToken),
	); err != nil {
		t.Fatalf("backdate last_used_at: %v", err)
	}
}

func readLastUsed(t *testing.T, db *database.DB, rawToken string) string {
	t.Helper()
	var v string
	if err := db.QueryRow(
		"SELECT COALESCE(last_used_at,'') FROM sessions WHERE token = ?", HashToken(rawToken),
	).Scan(&v); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	return v
}

// The configured lifetime, not the old hardcoded const, is what lands in the
// row — otherwise the config field would be decorative.
func TestCreateSessionUsesConfiguredLifetime(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "lifetime-user")

	withLifetimes(t, 2*time.Hour, time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		var expiresAt string
		if err := db.QueryRow("SELECT expires_at FROM sessions WHERE token = ?", HashToken(rawToken)).Scan(&expiresAt); err != nil {
			t.Fatalf("read expiry: %v", err)
		}
		exp, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			t.Fatalf("parse expiry: %v", err)
		}

		want := time.Now().Add(2 * time.Hour)
		if diff := exp.Sub(want); diff > time.Minute || diff < -time.Minute {
			t.Fatalf("expires_at is %s, want about %s", exp, want)
		}
	})
}

// Absent config must behave exactly as the hardcoded 30 days did.
func TestDefaultLifetimesMatchPreviousBehaviour(t *testing.T) {
	if sessionMaxLifetime != 30*24*time.Hour {
		t.Fatalf("default max lifetime is %s, want 30 days", sessionMaxLifetime)
	}
	if sessionIdleTimeout != 14*24*time.Hour {
		t.Fatalf("default idle timeout is %s, want 14 days", sessionIdleTimeout)
	}
}

// A session that has gone unused past the idle timeout is dead, even though
// its absolute expiry is comfortably in the future. This is the lost-phone
// case ADR 017 is built around.
func TestIdleSessionExpires(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "idle-user")

	withLifetimes(t, 30*24*time.Hour, time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		// Used 90 minutes ago, idle timeout is 60.
		setLastUsed(t, db, rawToken, time.Now().Add(-90*time.Minute))

		user, err := ValidateSession(db, rawToken)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if user != nil {
			t.Fatal("idle session still validates")
		}

		var n int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE token = ?", HashToken(rawToken)).Scan(&n)
		if n != 0 {
			t.Fatal("expired-by-idle session was not deleted")
		}
	})
}

// A session used recently survives, even if it has been alive a long time.
func TestActiveSessionSurvivesIdleWindow(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "active-user")

	withLifetimes(t, 30*24*time.Hour, time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		setLastUsed(t, db, rawToken, time.Now().Add(-30*time.Minute))

		user, err := ValidateSession(db, rawToken)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if user == nil {
			t.Fatal("recently-used session was rejected")
		}
	})
}

// The absolute ceiling still ends a session that is being used constantly.
// Without this, a sliding window would be a permanent credential.
func TestAbsoluteExpiryEndsAnActiveSession(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "absolute-user")

	withLifetimes(t, 30*24*time.Hour, 14*24*time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		// Past its absolute expiry, but used a second ago.
		if _, err := db.Exec(
			"UPDATE sessions SET expires_at = ? WHERE token = ?",
			time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), HashToken(rawToken),
		); err != nil {
			t.Fatalf("backdate expiry: %v", err)
		}
		setLastUsed(t, db, rawToken, time.Now())

		user, err := ValidateSession(db, rawToken)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if user != nil {
			t.Fatal("session past its absolute expiry still validates")
		}
	})
}

// The stamp is throttled: validation runs on every authenticated request and
// the target hardware is a Pi on an SD card, so a burst of requests must not
// become a burst of writes.
func TestLastUsedStampIsThrottled(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "throttle-user")

	withLifetimes(t, 30*24*time.Hour, 14*24*time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		// Freshly stamped by CreateSession. Several validations in a row
		// should leave it untouched.
		before := readLastUsed(t, db, rawToken)
		for i := 0; i < 5; i++ {
			if _, err := ValidateSession(db, rawToken); err != nil {
				t.Fatalf("validate: %v", err)
			}
		}
		if after := readLastUsed(t, db, rawToken); after != before {
			t.Fatalf("last_used_at was rewritten within the throttle window (%s -> %s)", before, after)
		}

		// Once the throttle interval has passed, the next validation stamps.
		setLastUsed(t, db, rawToken, time.Now().Add(-2*stampInterval))
		stale := readLastUsed(t, db, rawToken)
		if _, err := ValidateSession(db, rawToken); err != nil {
			t.Fatalf("validate: %v", err)
		}
		if readLastUsed(t, db, rawToken) == stale {
			t.Fatal("last_used_at was not stamped after the throttle window elapsed")
		}
	})
}

// Stamping keeps a session alive across repeated use, which is the whole
// point: an organizer who checks in weekly is never signed out.
func TestStampingKeepsARegularlyUsedSessionAlive(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "regular-user")

	withLifetimes(t, 30*24*time.Hour, 3*time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		// Check in every two hours against a three-hour idle timeout, ten
		// times over. The session should never lapse.
		for i := 0; i < 10; i++ {
			setLastUsed(t, db, rawToken, time.Now().Add(-2*time.Hour))
			user, err := ValidateSession(db, rawToken)
			if err != nil {
				t.Fatalf("validate at step %d: %v", i, err)
			}
			if user == nil {
				t.Fatalf("session lapsed at step %d despite regular use", i)
			}
		}
	})
}

// A session row from before migration 024 has no last_used_at. It must keep
// working rather than signing everyone out on upgrade.
func TestUnstampedSessionIsNotTreatedAsInfinitelyIdle(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "legacy-user")

	withLifetimes(t, 30*24*time.Hour, time.Hour, func() {
		rawToken, err := CreateSession(db, userID, "127.0.0.1")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if _, err := db.Exec("UPDATE sessions SET last_used_at = '' WHERE token = ?", HashToken(rawToken)); err != nil {
			t.Fatalf("clear last_used_at: %v", err)
		}

		user, err := ValidateSession(db, rawToken)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if user == nil {
			t.Fatal("session with no last_used_at was rejected")
		}
		if readLastUsed(t, db, rawToken) == "" {
			t.Fatal("validation did not stamp an unstamped session")
		}
	})
}

// --- step-up window ---

func TestSudoWindowGrantAndExpiry(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "sudo-user")

	rawToken, err := CreateSession(db, userID, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A plain session holds no window. This is the whole claim of ADR 017:
	// a valid cookie is not proof of presence.
	if SudoActive(db, rawToken) {
		t.Fatal("a fresh session already has a step-up window")
	}

	until, err := GrantSudo(db, rawToken)
	if err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	if !SudoActive(db, rawToken) {
		t.Fatal("step-up window is not active immediately after being granted")
	}
	if d := time.Until(until); d > SudoWindow+time.Minute || d < SudoWindow-time.Minute {
		t.Fatalf("window runs for %s, want about %s", d, SudoWindow)
	}

	// Once elapsed, it is closed.
	if _, err := db.Exec(
		"UPDATE sessions SET sudo_until = ? WHERE token = ?",
		time.Now().Add(-time.Second).UTC().Format(time.RFC3339), HashToken(rawToken),
	); err != nil {
		t.Fatalf("backdate sudo_until: %v", err)
	}
	if SudoActive(db, rawToken) {
		t.Fatal("elapsed step-up window still reads as active")
	}
}

// The window belongs to one session, not to the person. Another browser the
// same account is signed in on must not inherit it.
func TestSudoWindowIsPerSession(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "two-device-user")

	tokenA, err := CreateSession(db, userID, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session A: %v", err)
	}
	tokenB, err := CreateSession(db, userID, "127.0.0.2")
	if err != nil {
		t.Fatalf("create session B: %v", err)
	}

	if _, err := GrantSudo(db, tokenA); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	if !SudoActive(db, tokenA) {
		t.Fatal("session A has no window")
	}
	if SudoActive(db, tokenB) {
		t.Fatal("session B inherited session A's step-up window")
	}
}

// Logging out destroys the session row, and the window with it.
func TestSudoWindowDoesNotSurviveLogout(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "logout-user")

	rawToken, err := CreateSession(db, userID, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := GrantSudo(db, rawToken); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	if err := DestroySession(db, rawToken); err != nil {
		t.Fatalf("destroy session: %v", err)
	}
	if SudoActive(db, rawToken) {
		t.Fatal("step-up window survived logout")
	}
}

func TestGrantSudoRejectsUnknownSession(t *testing.T) {
	db := setupTestDB(t)
	if _, err := GrantSudo(db, "not-a-real-token"); err == nil {
		t.Fatal("granting a window on a nonexistent session succeeded")
	}
}

// HasCredential drives the "enroll a passkey first" notice. It must not
// claim a credential exists when none does.
func TestHasCredential(t *testing.T) {
	db := setupTestDB(t)
	userID := newSessionUser(t, db, "passkey-user")

	if HasCredential(db, userID) {
		t.Fatal("user with no credentials reports having one")
	}

	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count) VALUES (?, ?, ?, ?, 'none', ?, 0)`,
		NewUUIDv7(), userID, []byte("cred-id"), []byte("pubkey"), make([]byte, 16),
	); err != nil {
		t.Fatalf("insert credential: %v", err)
	}

	if !HasCredential(db, userID) {
		t.Fatal("user with a credential reports having none")
	}
}
