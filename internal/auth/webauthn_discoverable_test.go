package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

func testWebAuthnService(t *testing.T, db *database.DB) *WebAuthnService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Instance.Name = "Test Quilt"
	cfg.Instance.Domain = "quilt.test"

	svc, err := NewWebAuthnService(db, cfg)
	if err != nil {
		t.Fatalf("NewWebAuthnService: %v", err)
	}
	return svc
}

func makeWebAuthnUser(t *testing.T, db *database.DB, username string) *model.User {
	t.Helper()
	id := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		id, username, username, now, now,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return &model.User{ID: id, Username: username, DisplayName: username}
}

// Sign-in is BeginDiscoverableLogin — an empty allowCredentials that only a
// client-side discoverable credential can answer. Registration therefore has
// to ask for one. It did not: with authenticatorSelection left unset, an
// absent residentKey means "discouraged", so an authenticator was free to
// hand back a credential the sign-in page could never find again. Syncing
// passkey providers store everything discoverably and hid it; a security key
// would have enrolled happily and then been useless.
func TestRegistrationRequiresDiscoverableCredential(t *testing.T) {
	db := setupTestDB(t)
	svc := testWebAuthnService(t, db)
	user := makeWebAuthnUser(t, db, "enroller")

	optJSON, err := svc.BeginRegistration(user)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}

	var opts struct {
		PublicKey struct {
			AuthenticatorSelection struct {
				ResidentKey        string `json:"residentKey"`
				RequireResidentKey *bool  `json:"requireResidentKey"`
			} `json:"authenticatorSelection"`
			RP struct {
				ID string `json:"id"`
			} `json:"rp"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(optJSON, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}

	sel := opts.PublicKey.AuthenticatorSelection
	if sel.ResidentKey != "required" {
		t.Errorf("residentKey = %q, want required — sign-in cannot find anything else", sel.ResidentKey)
	}
	if sel.RequireResidentKey == nil || !*sel.RequireResidentKey {
		t.Error("requireResidentKey not set — the L1 spelling is what older authenticators read")
	}
	if opts.PublicKey.RP.ID != "quilt.test" {
		t.Errorf("rp.id = %q, want quilt.test", opts.PublicKey.RP.ID)
	}
}

// The login challenge must stay discoverable: an allowCredentials list here
// would mean the server naming who is signing in before they have proved it.
func TestLoginChallengeIsDiscoverable(t *testing.T) {
	db := setupTestDB(t)
	svc := testWebAuthnService(t, db)

	optJSON, err := svc.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}

	var opts struct {
		PublicKey struct {
			RPID             string `json:"rpId"`
			Challenge        string `json:"challenge"`
			AllowCredentials []any  `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(optJSON, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}

	if len(opts.PublicKey.AllowCredentials) != 0 {
		t.Errorf("allowCredentials has %d entries, want none", len(opts.PublicKey.AllowCredentials))
	}
	if opts.PublicKey.RPID != "quilt.test" {
		t.Errorf("rpId = %q, want quilt.test", opts.PublicKey.RPID)
	}
	if opts.PublicKey.Challenge == "" {
		t.Error("no challenge issued")
	}
}
