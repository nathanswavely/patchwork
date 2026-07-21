package seamrip

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func testDB(t *testing.T) *database.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "seamrip-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(tmp.Name(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

var idCounter int

func nextID() string {
	idCounter++
	return fmt.Sprintf("019f0000-0000-7000-8000-%012d", idCounter)
}

func mustExec(t *testing.T, db *database.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func count(t *testing.T, db *database.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// seedSource builds a small community: three users, two patches with
// overlapping members, an event, a proposal with votes and a threaded
// comment, a governance doc, tags, and notification preferences.
func seedSource(t *testing.T, db *database.DB) {
	u1, u2, u3 := nextID(), nextID(), nextID()
	n1, n2 := nextID(), nextID()
	now := "2026-01-01T00:00:00Z"

	for i, u := range []string{u1, u2, u3} {
		mustExec(t, db,
			`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'member', ?, ?)`,
			u, fmt.Sprintf("user%d@example.com", i+1), fmt.Sprintf("user%d", i+1), fmt.Sprintf("User %d", i+1), now, now)
	}

	for i, n := range []string{n1, n2} {
		mustExec(t, db,
			`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, created_at, updated_at) VALUES (?, ?, ?, ?, '', 'public', 'open', 'active', ?, ?)`,
			n, u1, fmt.Sprintf("Patch %d", i+1), fmt.Sprintf("patch-%d", i+1), now, now)
	}

	// u1 and u2 are members of BOTH patches (overlap = 2); u3 follows n1.
	for _, m := range []struct {
		user, node, role string
	}{
		{u1, n1, "admin"}, {u2, n1, "member"}, {u3, n1, "follower"},
		{u1, n2, "admin"}, {u2, n2, "member"},
	} {
		mustExec(t, db,
			`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, 'active', ?)`,
			nextID(), m.user, m.node, m.role, now)
	}

	tag := nextID()
	mustExec(t, db, `INSERT INTO tags (id, name) VALUES (?, 'music')`, tag)
	mustExec(t, db, `INSERT INTO node_tags (node_id, tag_id) VALUES (?, ?)`, n1, tag)

	mustExec(t, db,
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility, created_at, updated_at) VALUES (?, ?, ?, 'Show', '', 'Venue', ?, '', 'public', ?, ?)`,
		nextID(), n1, u1, now, now, now)

	prop := nextID()
	mustExec(t, db,
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, created_at, updated_at) VALUES (?, ?, ?, 'Prop', '', 'open', 'voting', ?, ?)`,
		prop, n1, u1, now, now)
	mustExec(t, db,
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		nextID(), prop, u1)
	mustExec(t, db,
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'reject')`,
		nextID(), prop, u2)

	parent := nextID()
	child := nextID()
	// Insert child-before-parent in export order is not controllable, but
	// exercise threading either way.
	mustExec(t, db,
		`INSERT INTO proposal_comments (id, proposal_id, author_id, body) VALUES (?, ?, ?, 'root')`,
		parent, prop, u1)
	mustExec(t, db,
		`INSERT INTO proposal_comments (id, proposal_id, parent_id, author_id, body) VALUES (?, ?, ?, ?, 'reply')`,
		child, prop, parent, u2)
	mustExec(t, db,
		`INSERT INTO comment_reactions (id, comment_id, user_id, emoji) VALUES (?, ?, ?, '+1')`,
		nextID(), parent, u2)

	mustExec(t, db,
		`INSERT INTO governance_docs (id, node_id, title, body, version, created_by) VALUES (?, ?, 'Lining', 'Be kind.', 1, ?)`,
		nextID(), n1, u1)

	mustExec(t, db,
		`INSERT INTO notification_preferences (id, user_id, notification_type, channel, enabled) VALUES (?, ?, 'proposal', 'email', 0)`,
		nextID(), u2)

	// Things that must NOT travel.
	mustExec(t, db,
		`INSERT INTO sessions (id, user_id, token, expires_at) VALUES (?, ?, 'tok', '2027-01-01T00:00:00Z')`,
		nextID(), u1)
	mustExec(t, db, `UPDATE users SET private_key = 'SECRET', public_key = 'PUB', ap_id = 'https://old.example/ap/users/x' WHERE id = ?`, u1)
}

func TestRoundTrip(t *testing.T) {
	src := testDB(t)
	seedSource(t, src)

	// Export to an in-memory file set.
	files := map[string][]map[string]any{}
	err := Export(src, func(tab Table, items []map[string]any) error {
		files[tab.File] = items
		return nil
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Nothing secret in the users export.
	for _, u := range files["users.json"] {
		for _, forbidden := range []string{"private_key", "public_key", "ap_id"} {
			if _, ok := u[forbidden]; ok {
				t.Errorf("users.json leaks %s", forbidden)
			}
		}
	}
	if len(files["memberships.json"]) != 5 {
		t.Fatalf("expected 5 memberships exported, got %d", len(files["memberships.json"]))
	}

	// Import into a fresh database.
	dst := testDB(t)
	idMap, results, err := Import(dst,
		func(file string) ([]map[string]any, error) { return files[file], nil },
		nextID)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, r := range results {
		if r.Skipped > 0 {
			t.Errorf("table %s skipped %d rows", r.Table, r.Skipped)
		}
	}

	// Every table round-trips by count.
	for table, want := range map[string]int{
		"users": 3, "nodes": 2, "memberships": 5, "tags": 1, "node_tags": 1,
		"events": 1, "proposals": 1, "votes": 2, "proposal_comments": 2,
		"comment_reactions": 1, "governance_docs": 1, "notification_preferences": 1,
	} {
		if got := count(t, dst, "SELECT COUNT(*) FROM "+table); got < want {
			t.Errorf("%s: got %d rows, want >= %d", table, got, want)
		}
	}

	// THE mission-critical property: member overlap between the two patches
	// survives the fork. Threads are inferred from shared admin/member rows.
	overlap := count(t, dst, `
		SELECT COUNT(*) FROM memberships m1
		JOIN memberships m2 ON m1.user_id = m2.user_id AND m1.node_id != m2.node_id
		WHERE m1.role IN ('admin','member') AND m2.role IN ('admin','member')`)
	if overlap != 4 { // 2 shared users x 2 directed pairs
		t.Errorf("member overlap lost in seamrip: got %d directed overlap rows, want 4", overlap)
	}

	// Comment threading survives with remapped IDs.
	threaded := count(t, dst, `
		SELECT COUNT(*) FROM proposal_comments c
		JOIN proposal_comments p ON c.parent_id = p.id`)
	if threaded != 1 {
		t.Errorf("threaded comment lost: got %d, want 1", threaded)
	}

	// No secrets or instance identity in the destination.
	if n := count(t, dst, `SELECT COUNT(*) FROM sessions`); n != 0 {
		t.Errorf("sessions traveled: %d", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE private_key IS NOT NULL`); n != 0 {
		t.Errorf("private keys traveled: %d", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE ap_id IS NOT NULL AND username != '_system'`); n != 0 {
		t.Errorf("ap_ids traveled: %d", n)
	}

	// Emails travel so people can re-auth by magic link.
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE email LIKE 'user%@example.com'`); n != 3 {
		t.Errorf("emails lost: got %d, want 3", n)
	}

	// All IDs were rewritten.
	for old, minted := range idMap {
		if old == SentinelUserID {
			continue
		}
		if old == minted {
			t.Errorf("ID %s not rewritten", old)
		}
	}
}

func TestImportUnclaimedSentinelOwner(t *testing.T) {
	src := testDB(t)
	now := "2026-01-01T00:00:00Z"
	// An unclaimed patch owned by the sentinel user.
	mustExec(t, src,
		`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, submission_source, created_at, updated_at) VALUES (?, ?, 'Unclaimed Venue', 'unclaimed-venue', '', 'public', 'open', 'active', 'community', ?, ?)`,
		nextID(), SentinelUserID, now, now)

	files := map[string][]map[string]any{}
	if err := Export(src, func(tab Table, items []map[string]any) error {
		files[tab.File] = items
		return nil
	}); err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(files["users.json"]) != 0 {
		t.Fatalf("sentinel user must not be exported, got %d users", len(files["users.json"]))
	}

	dst := testDB(t)
	_, results, err := Import(dst,
		func(file string) ([]map[string]any, error) { return files[file], nil },
		nextID)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, r := range results {
		if r.Skipped > 0 {
			t.Errorf("table %s skipped %d rows", r.Table, r.Skipped)
		}
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM nodes WHERE owner_id = ?`, SentinelUserID); n != 1 {
		t.Errorf("unclaimed node lost its sentinel owner: %d", n)
	}
}
