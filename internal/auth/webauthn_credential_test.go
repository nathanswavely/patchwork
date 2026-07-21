package auth

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func makeCredUser(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		userID, username, username, now, now,
	); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID
}

func testCredential(id string) *webauthn.Credential {
	return &webauthn.Credential{
		ID:              []byte(id),
		PublicKey:       []byte("public-key-bytes"),
		AttestationType: "none",
	}
}

// The regression that made issue #50. A synced passkey (iCloud Keychain,
// 1Password, hybrid/QR) reports BackupEligible = true at enrollment.
// go-webauthn hard-compares the stored BE flag against the assertion's BE bit
// at every login and rejects on mismatch, so if BE does not survive the round
// trip through SQLite the credential enrolls fine and can then never log in
// again. It failed silently for months because nothing asserted on it.
func TestBackupEligibleSurvivesRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "synced-passkey-person")

	cred := testCredential("synced-cred")
	cred.Flags.BackupEligible = true
	cred.Flags.BackupState = true

	if _, err := SaveCredential(db, userID, "iPhone", cred); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadCredentials(db, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d credentials, want 1", len(loaded))
	}

	if !loaded[0].Flags.BackupEligible {
		t.Error("BackupEligible was lost in storage — every synced passkey is locked out")
	}
	if !loaded[0].Flags.BackupState {
		t.Error("BackupState was lost in storage")
	}
}

// Flags must round-trip in every combination, not just the true one. A
// platform authenticator reporting BE=0 has to keep loading as BE=0, or the
// same comparison rejects it from the other direction.
func TestCredentialFlagsRoundTrip(t *testing.T) {
	cases := []struct {
		name                        string
		backupEligible, backupState bool
	}{
		{"platform authenticator", false, false},
		{"syncable but not yet synced", true, false},
		{"synced", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			userID := makeCredUser(t, db, "flags-"+tc.name)

			cred := testCredential("cred-" + tc.name)
			cred.Flags.BackupEligible = tc.backupEligible
			cred.Flags.BackupState = tc.backupState

			if _, err := SaveCredential(db, userID, "", cred); err != nil {
				t.Fatalf("save: %v", err)
			}

			loaded, err := LoadCredentials(db, userID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(loaded) != 1 {
				t.Fatalf("loaded %d credentials, want 1", len(loaded))
			}

			if loaded[0].Flags.BackupEligible != tc.backupEligible {
				t.Errorf("BackupEligible = %v, want %v", loaded[0].Flags.BackupEligible, tc.backupEligible)
			}
			if loaded[0].Flags.BackupState != tc.backupState {
				t.Errorf("BackupState = %v, want %v", loaded[0].Flags.BackupState, tc.backupState)
			}
		})
	}
}

// The rest of the credential record has to survive too — a correct BE flag on
// a credential whose public key got mangled helps nobody.
func TestCredentialCoreFieldsRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "core-fields-person")

	cred := testCredential("core-cred")
	cred.Authenticator.SignCount = 42
	cred.Authenticator.AAGUID = []byte("0123456789abcdef")
	cred.Transport = []protocol.AuthenticatorTransport{
		protocol.Internal,
		protocol.Hybrid,
	}

	if _, err := SaveCredential(db, userID, "Laptop", cred); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadCredentials(db, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d credentials, want 1", len(loaded))
	}
	got := loaded[0]

	if string(got.ID) != "core-cred" {
		t.Errorf("ID = %q, want %q", got.ID, "core-cred")
	}
	if string(got.PublicKey) != "public-key-bytes" {
		t.Errorf("PublicKey = %q", got.PublicKey)
	}
	if got.AttestationType != "none" {
		t.Errorf("AttestationType = %q, want %q", got.AttestationType, "none")
	}
	if got.Authenticator.SignCount != 42 {
		t.Errorf("SignCount = %d, want 42", got.Authenticator.SignCount)
	}
	// AAGUID is a []byte slice, not an array: copy(dst[:], src) into it is a
	// no-op that silently drops the value, which is how it used to be loaded.
	if string(got.Authenticator.AAGUID) != "0123456789abcdef" {
		t.Errorf("AAGUID = %q, want %q", got.Authenticator.AAGUID, "0123456789abcdef")
	}
	if len(got.Transport) != 2 || got.Transport[0] != protocol.Internal || got.Transport[1] != protocol.Hybrid {
		t.Errorf("Transport = %v, want [internal hybrid]", got.Transport)
	}
}

// Rows enrolled before migration 023 have NULL flags. They must still load —
// as false, which is how they behaved all along — rather than erroring out or
// being quietly healed from whatever a later assertion claims. A synced
// passkey in this state fails once and the person re-enrolls; that was the
// explicit call, because deleting the rows instead would lock passkey-only
// people out of instances that run without SMTP.
func TestLegacyNullFlagsLoadAsFalse(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "legacy-person")

	// Written the way the pre-fix INSERT did: no flag columns at all.
	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		NewUUIDv7(), userID, []byte("legacy-cred"), []byte("pk"), "none", make([]byte, 16), 7,
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	loaded, err := LoadCredentials(db, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d credentials, want 1", len(loaded))
	}
	if loaded[0].Flags.BackupEligible || loaded[0].Flags.BackupState {
		t.Error("NULL flags should load as false, not as something invented")
	}
	if loaded[0].Authenticator.SignCount != 7 {
		t.Errorf("SignCount = %d, want 7", loaded[0].Authenticator.SignCount)
	}

	// The columns must still read NULL, not have been backfilled on load.
	var be, bs any
	if err := db.QueryRow(
		`SELECT backup_eligible, backup_state FROM credentials WHERE user_id = ?`, userID,
	).Scan(&be, &bs); err != nil {
		t.Fatalf("read flags: %v", err)
	}
	if be != nil || bs != nil {
		t.Errorf("legacy flags were backfilled: be=%v bs=%v", be, bs)
	}
}

// Issue #44: the INSERT omitted name entirely, so every passkey in the list
// read "Passkey" no matter what the person called it.
func TestCredentialNamePersists(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "named-passkey-person")

	if _, err := SaveCredential(db, userID, "  Work laptop  ", testCredential("named-cred")); err != nil {
		t.Fatalf("save: %v", err)
	}

	var name string
	if err := db.QueryRow(
		`SELECT name FROM credentials WHERE user_id = ?`, userID,
	).Scan(&name); err != nil {
		t.Fatalf("read name: %v", err)
	}
	if name != "Work laptop" {
		t.Errorf("name = %q, want %q", name, "Work laptop")
	}
}

func TestCredentialNameDefaultsWhenEmpty(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "unnamed-passkey-person")

	if _, err := SaveCredential(db, userID, "   ", testCredential("unnamed-cred")); err != nil {
		t.Fatalf("save: %v", err)
	}

	var name string
	if err := db.QueryRow(
		`SELECT name FROM credentials WHERE user_id = ?`, userID,
	).Scan(&name); err != nil {
		t.Fatalf("read name: %v", err)
	}
	if name != DefaultCredentialName {
		t.Errorf("name = %q, want %q", name, DefaultCredentialName)
	}
}

func TestSanitizeCredentialName(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"trims", "  iPhone  ", "iPhone"},
		{"empty falls back", "", DefaultCredentialName},
		{"whitespace only falls back", " \t\n ", DefaultCredentialName},
		{"strips control characters", "iPh\x00o\x07ne", "iPhone"},
		{"newlines become spaces", "my\npasskey", "my passkey"},
		{"keeps unicode", "clé 🔑", "clé 🔑"},
		{"caps length", long, long[:maxCredentialNameLen]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeCredentialName(tc.in); got != tc.want {
				t.Errorf("SanitizeCredentialName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The cap counts runes, not bytes: a multi-byte name must not be truncated
// mid-character into mojibake.
func TestSanitizeCredentialNameCapsRunesNotBytes(t *testing.T) {
	in := ""
	for i := 0; i < 100; i++ {
		in += "é"
	}

	got := SanitizeCredentialName(in)
	if n := len([]rune(got)); n != maxCredentialNameLen {
		t.Errorf("got %d runes, want %d", n, maxCredentialNameLen)
	}
	for _, r := range got {
		if r != 'é' {
			t.Fatalf("name was cut mid-character: %q", got)
		}
	}
}

// Two passkeys for the same person must both come back, each with its own
// flags — loading is per-user, and a shared zero value between them was
// exactly the shape of the original bug.
func TestLoadCredentialsKeepsPerCredentialFlags(t *testing.T) {
	db := setupTestDB(t)
	userID := makeCredUser(t, db, "two-passkey-person")

	platform := testCredential("platform-cred")
	synced := testCredential("synced-cred")
	synced.Flags.BackupEligible = true
	synced.Flags.BackupState = true

	if _, err := SaveCredential(db, userID, "Windows PC", platform); err != nil {
		t.Fatalf("save platform: %v", err)
	}
	if _, err := SaveCredential(db, userID, "iPhone", synced); err != nil {
		t.Fatalf("save synced: %v", err)
	}

	loaded, err := LoadCredentials(db, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d credentials, want 2", len(loaded))
	}

	byID := map[string]webauthn.Credential{}
	for _, c := range loaded {
		byID[string(c.ID)] = c
	}
	if byID["platform-cred"].Flags.BackupEligible {
		t.Error("platform credential should not be backup eligible")
	}
	if !byID["synced-cred"].Flags.BackupEligible {
		t.Error("synced credential lost its BackupEligible flag")
	}
}

// Credentials belong to one person; loading must never reach across users.
func TestLoadCredentialsIsScopedToUser(t *testing.T) {
	db := setupTestDB(t)
	mine := makeCredUser(t, db, "mine")
	theirs := makeCredUser(t, db, "theirs")

	if _, err := SaveCredential(db, mine, "Mine", testCredential("my-cred")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := SaveCredential(db, theirs, "Theirs", testCredential("their-cred")); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadCredentials(db, mine)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 || string(loaded[0].ID) != "my-cred" {
		t.Fatalf("expected only my credential, got %d", len(loaded))
	}
}
