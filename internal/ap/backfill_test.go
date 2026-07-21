package ap_test

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

func TestBackfillKeypairs(t *testing.T) {
	db := setupTestDB(t)

	// Two users and one node with no keypair (mimics seeded/pre-federation data).
	for _, u := range []struct{ id, email, uname string }{
		{"bf-user-1", "a@example.com", "bfuser1"},
		{"bf-user-2", "b@example.com", "bfuser2"},
	} {
		if _, err := db.Exec(
			`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'member', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			u.id, u.email, u.uname, u.uname,
		); err != nil {
			t.Fatalf("insert user %s: %v", u.id, err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, visibility, membership_policy, status, created_at, updated_at)
		 VALUES ('bf-node-1', 'bf-user-1', 'BF Node', 'bf-node', 'public', 'open', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	nu, nn, err := ap.BackfillKeypairs(db)
	if err != nil {
		t.Fatalf("BackfillKeypairs: %v", err)
	}
	// The migrated schema may seed a system user, so don't assume an empty DB —
	// assert that at least the entities we created were backfilled.
	if nu < 2 {
		t.Errorf("expected at least 2 users backfilled, got %d", nu)
	}
	if nn < 1 {
		t.Errorf("expected at least 1 node backfilled, got %d", nn)
	}

	// Every row should now have both keys.
	var missing int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE public_key IS NULL OR public_key = '' OR private_key IS NULL OR private_key = ''").Scan(&missing)
	if missing != 0 {
		t.Errorf("expected 0 users missing keys, got %d", missing)
	}
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE public_key IS NULL OR public_key = '' OR private_key IS NULL OR private_key = ''").Scan(&missing)
	if missing != 0 {
		t.Errorf("expected 0 nodes missing keys, got %d", missing)
	}

	// A second run is a no-op (idempotent).
	nu2, nn2, err := ap.BackfillKeypairs(db)
	if err != nil {
		t.Fatalf("second BackfillKeypairs: %v", err)
	}
	if nu2 != 0 || nn2 != 0 {
		t.Errorf("expected second run to backfill nothing, got %d users, %d nodes", nu2, nn2)
	}
}

func TestBackfillKeypairs_PreservesExisting(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at)
		 VALUES ('bf-keep-1', 'k@example.com', 'bfkeep', 'bfkeep', 'member', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// Give it a key up front.
	pub, _, err := ap.EnsureUserKeypair(db, "bf-keep-1")
	if err != nil {
		t.Fatalf("ensure keypair: %v", err)
	}

	if _, _, err := ap.BackfillKeypairs(db); err != nil {
		t.Fatalf("BackfillKeypairs: %v", err)
	}

	var got string
	db.QueryRow("SELECT public_key FROM users WHERE id = 'bf-keep-1'").Scan(&got)
	if got != pub {
		t.Error("backfill overwrote an existing keypair")
	}
}

func TestBackfillAPIDs_HealsStaleDomain(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at, ap_id)
		 VALUES ('heal-user-1', 'heal@example.com', 'healuser', 'healuser', 'member',
		         '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'https://localhost/ap/users/heal-user-1')`,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, visibility, membership_policy, status, created_at, updated_at, ap_id)
		 VALUES ('heal-node-1', 'heal-user-1', 'Heal Node', 'heal-node', 'public', 'open', 'active',
		         '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'https://localhost/ap/nodes/heal-node-1')`,
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	// A row already on the right domain must not be counted or changed.
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at, ap_id)
		 VALUES ('heal-user-2', 'heal2@example.com', 'healuser2', 'healuser2', 'member',
		         '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'https://right.test/ap/users/heal-user-2')`,
	); err != nil {
		t.Fatalf("insert user 2: %v", err)
	}
	// A remote-shaped URI (no local /ap/<kind>/<row id> tail) must be untouched.
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at, ap_id)
		 VALUES ('heal-user-3', 'heal3@example.com', 'healuser3', 'healuser3', 'member',
		         '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'https://mastodon.example/users/someone')`,
	); err != nil {
		t.Fatalf("insert user 3: %v", err)
	}

	n, err := ap.BackfillAPIDs(db, "right.test")
	if err != nil {
		t.Fatalf("BackfillAPIDs: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows rewritten, got %d", n)
	}

	var got string
	db.QueryRow("SELECT ap_id FROM users WHERE id = 'heal-user-1'").Scan(&got)
	if got != "https://right.test/ap/users/heal-user-1" {
		t.Errorf("user ap_id = %q, want healed domain", got)
	}
	db.QueryRow("SELECT ap_id FROM nodes WHERE id = 'heal-node-1'").Scan(&got)
	if got != "https://right.test/ap/nodes/heal-node-1" {
		t.Errorf("node ap_id = %q, want healed domain", got)
	}
	db.QueryRow("SELECT ap_id FROM users WHERE id = 'heal-user-3'").Scan(&got)
	if got != "https://mastodon.example/users/someone" {
		t.Errorf("remote-shaped ap_id was rewritten to %q", got)
	}

	// Idempotent: second run touches nothing.
	n2, err := ap.BackfillAPIDs(db, "right.test")
	if err != nil {
		t.Fatalf("second BackfillAPIDs: %v", err)
	}
	if n2 != 0 {
		t.Errorf("expected second run to rewrite nothing, got %d", n2)
	}
}
