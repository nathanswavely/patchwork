package seamrip

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
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

	// A patch's image is a URL it owns, so the reference travels even though
	// the bytes were never the instance's to move (docs/adr/007).
	mustExec(t, db,
		`UPDATE nodes SET image_url = 'https://cdn.example/patch-1.jpg', image_alt = 'The storefront' WHERE id = ?`, n1)

	// Choices a fork must not silently reverse: a member who hid a membership
	// (docs/adr/006), a patch that closed its door to event suggestions
	// (docs/adr/026), and a person's profile links.
	mustExec(t, db, `UPDATE memberships SET visible = 0 WHERE user_id = ? AND node_id = ?`, u2, n1)
	mustExec(t, db, `UPDATE nodes SET accept_event_suggestions = 0 WHERE id = ?`, n2)
	mustExec(t, db, `UPDATE users SET links = '[{"url":"https://example.com","label":"Site"}]' WHERE id = ?`, u1)

	tag := nextID()
	mustExec(t, db, `INSERT INTO tags (id, name) VALUES (?, 'music')`, tag)
	mustExec(t, db, `INSERT INTO node_tags (node_id, tag_id) VALUES (?, ?)`, n1, tag)

	ev := nextID()
	mustExec(t, db,
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility, created_at, updated_at) VALUES (?, ?, ?, 'Show', '', 'Venue', ?, '', 'public', ?, ?)`,
		ev, n1, u1, now, now, now)

	// The show is also n2's (docs/adr/032): a confirmed link, and a doorway to
	// a band on another quilt.
	mustExec(t, db,
		`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by, confirmed_at)
		 VALUES (?, ?, ?, 'confirmed', 'owner', ?, ?)`,
		nextID(), ev, n2, u1, now)
	mustExec(t, db,
		`INSERT INTO event_mentions (id, event_id, host, slug, name) VALUES (?, ?, 'other.example', 'the-band', 'The Band')`,
		nextID(), ev)

	// A second event carrying a link nobody confirmed. A fork cannot carry a
	// handshake half-finished, and a pending link is invisible everywhere.
	ev2 := nextID()
	mustExec(t, db,
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility, created_at, updated_at) VALUES (?, ?, ?, 'Workshop', '', 'Shop', ?, '', 'public', ?, ?)`,
		ev2, n2, u1, now, now, now)
	mustExec(t, db,
		`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by)
		 VALUES (?, ?, ?, 'pending', 'linked', ?)`,
		nextID(), ev2, n1, u2)

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

	// An elected council on n2: two seats with a term end, and the election
	// that filled them — the proposal, who stood, and who approved whom
	// (docs/adr/051).
	seatA, seatB := nextID(), nextID()
	mustExec(t, db,
		`INSERT INTO seats (id, node_id, holder_id, term_ends_at, created_at) VALUES (?, ?, ?, '2027-03-01', ?)`,
		seatA, n2, u1, now)
	// A vacant chair: it exists, it will be contested, nobody holds it.
	mustExec(t, db,
		`INSERT INTO seats (id, node_id, holder_id, term_ends_at, created_at) VALUES (?, ?, NULL, '2027-03-01', ?)`,
		seatB, n2, now)

	election := nextID()
	mustExec(t, db,
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		 seats_contested, nominations_close_at, voting_terms, created_at, updated_at)
		 VALUES (?, ?, ?, 'Council election', '', 'open', 'voting', 'membership', 2, ?,
		 '{"decision_method":"majority","quorum_percent":40}', ?, ?)`,
		election, n2, u1, now, now, now)
	candA, candB := nextID(), nextID()
	mustExec(t, db,
		`INSERT INTO election_candidates (id, proposal_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		candA, election, u1, now)
	mustExec(t, db,
		`INSERT INTO election_candidates (id, proposal_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		candB, election, u2, now)
	// u1 approves both; u2 approves only u1. An approval ballot is rows.
	for _, b := range []struct{ voter, cand string }{{u1, candA}, {u1, candB}, {u2, candA}} {
		mustExec(t, db,
			`INSERT INTO election_ballots (id, proposal_id, voter_id, candidate_id, created_at) VALUES (?, ?, ?, ?, ?)`,
			nextID(), election, b.voter, b.cand, now)
	}

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
		"events": 2, "proposals": 2, "votes": 2, "proposal_comments": 2,
		"comment_reactions": 1, "governance_docs": 1, "notification_preferences": 1,
		"seats": 2, "election_candidates": 2, "election_ballots": 3,
		"event_links": 1, "event_mentions": 1,
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

	// The fork can still hold an election. Dueness is derived from
	// `seats.term_ends_at` rather than stored (docs/adr/051), so a fork with no
	// seats never schedules another contest — the safety valve would have
	// stripped the machinery that rotates leadership.
	if n := count(t, dst, `SELECT COUNT(*) FROM seats WHERE term_ends_at = '2027-03-01'`); n != 2 {
		t.Errorf("election calendar lost: got %d seats with a term end, want 2", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM seats WHERE holder_id IS NULL`); n != 1 {
		t.Errorf("the vacant chair did not travel: got %d, want 1", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM seats s JOIN nodes n ON n.id = s.node_id WHERE n.slug = 'patch-2'`); n != 2 {
		t.Errorf("seats landed on the wrong patch: got %d on patch-2, want 2", n)
	}

	// And it is still an election. `seats_contested` is what every read path
	// branches on; a proposal carrying candidates with zero seats renders as an
	// ordinary proposal and the slate below it is orphaned rows.
	if n := count(t, dst, `SELECT COUNT(*) FROM proposals WHERE seats_contested = 2`); n != 1 {
		t.Errorf("the election stopped being one: got %d proposals with seats contested, want 1", n)
	}

	// The tally survives, joined through both remapped ends. u1 approved by two
	// people, u2 by one — a record that says a contest happened must be able to
	// say what it decided.
	winner := count(t, dst, `
		SELECT COUNT(*) FROM election_ballots b
		JOIN election_candidates c ON c.id = b.candidate_id
		JOIN users u ON u.id = c.user_id
		WHERE u.username = 'user1' AND b.proposal_id = c.proposal_id`)
	if winner != 2 {
		t.Errorf("approval tally lost: user1 has %d approvals, want 2", winner)
	}

	// A vote keeps the terms it opened with (docs/adr/047), across a fork too.
	var terms string
	dst.QueryRow(`SELECT COALESCE(voting_terms,'') FROM proposals WHERE seats_contested = 2`).Scan(&terms)
	if terms == "" {
		t.Error("voting terms lost: the fork would judge an in-flight vote by its own live rules")
	}

	// An event's other patch survives, joined through both remapped ends
	// (docs/adr/032) — and only the confirmed one. A pending link is a
	// handshake nobody finished; a fork cannot carry it.
	linked := count(t, dst, `
		SELECT COUNT(*) FROM event_links l
		JOIN events e ON e.id = l.event_id
		JOIN nodes n ON n.id = l.node_id
		WHERE e.title = 'Show' AND n.slug = 'patch-2'`)
	if linked != 1 {
		t.Errorf("event link lost: got %d, want 1", linked)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM event_links WHERE status = 'pending'`); n != 0 {
		t.Errorf("a pending link traveled: %d", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM event_mentions WHERE host = 'other.example' AND slug = 'the-band'`); n != 1 {
		t.Errorf("cross-quilt mention lost: got %d, want 1", n)
	}

	// The image reference travels with its description. Both, or the fork
	// arrives with a picture nobody can read (docs/adr/007).
	var imgURL, imgAlt string
	dst.QueryRow(`SELECT image_url, image_alt FROM nodes WHERE slug = 'patch-1'`).Scan(&imgURL, &imgAlt)
	if imgURL != "https://cdn.example/patch-1.jpg" || imgAlt != "The storefront" {
		t.Errorf("image reference lost: url=%q alt=%q", imgURL, imgAlt)
	}

	// A hidden membership stays hidden. `visible` defaults to 1, so leaving it
	// behind re-exposed it — on the profile and in the patch's public member
	// list at once, since one switch drives both (docs/adr/006). A seamrip is
	// when that choice matters most: it is what a community does when its
	// leadership goes sideways.
	var hidden int
	dst.QueryRow(`SELECT COUNT(*) FROM memberships m JOIN users u ON u.id = m.user_id
	              WHERE u.username = 'user2' AND m.visible = 0`).Scan(&hidden)
	if hidden != 1 {
		t.Errorf("a hidden membership was re-exposed by the fork: got %d hidden, want 1", hidden)
	}

	// A patch that closed its door to event suggestions keeps it closed.
	var accepts int
	dst.QueryRow(`SELECT accept_event_suggestions FROM nodes WHERE slug = 'patch-2'`).Scan(&accepts)
	if accepts != 0 {
		t.Error("a patch that refused event suggestions found the door open again")
	}

	// Profile links travel, the way a patch's already did.
	var links string
	dst.QueryRow(`SELECT COALESCE(links,'') FROM users WHERE username = 'user1'`).Scan(&links)
	if !strings.Contains(links, "example.com") {
		t.Errorf("profile links lost: %q", links)
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

// An archive written before migration 033 has no provenance keys on its
// event rows. The INSERT names every column, so the table DEFAULT can't
// apply — the Column.Default fallback must, or every event is skipped
// and "older exports stay importable" is broken (docs/adr/031).
func TestImportPre033Archive(t *testing.T) {
	src := testDB(t)
	seedSource(t, src)

	files := map[string][]map[string]any{}
	if err := Export(src, func(tab Table, items []map[string]any) error {
		files[tab.File] = items
		return nil
	}); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Rewind the archive to its pre-033 shape: no provenance keys on
	// events, and no event-source files at all.
	for _, e := range files["events.json"] {
		delete(e, "source_id")
		delete(e, "source_uid")
		delete(e, "source_occurrence")
	}
	delete(files, "event_sources.json")
	delete(files, "event_source_skips.json")

	dst := testDB(t)
	_, results, err := Import(dst,
		func(file string) ([]map[string]any, error) { return files[file], nil },
		nextID)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, r := range results {
		if r.Skipped > 0 {
			t.Errorf("table %s skipped %d rows importing an old archive", r.Table, r.Skipped)
		}
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM events`); n == 0 {
		t.Fatal("old archive imported zero events")
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM events WHERE source_occurrence != ''`); n != 0 {
		t.Errorf("defaulted source_occurrence rows should be '': %d weren't", n)
	}
}

// Every table is either exported or deliberately left behind.
//
// This is the test that would have caught the elections gap when it was made
// rather than three PRs later. Migration 050 added `seats`,
// `election_candidates` and `election_ballots`; nothing failed, because a
// table absent from Tables() is simply never asked about. The consequence was
// invisible and severe: dueness is derived from `seats.term_ends_at`, so a
// forked elected patch silently stopped holding elections — the safety valve
// stripping the machinery that rotates leadership.
//
// A new table now forces a choice. Add it to Tables(), or name it here with
// the reason it stays behind. Both are fine; not deciding is not.
func TestEveryTableHasABoundaryDecision(t *testing.T) {
	db := testDB(t)

	// Instance identity, secrets, and derived state (docs/adr/002). A fresh
	// instance mints its own on first boot or rebuilds it from what travels.
	staysBehind := map[string]string{
		"aggregator_holds":            "undecided duplicate questions; they re-arise on the fork's first routing pass (docs/adr/056)",
		"aggregator_listings":         "the cache of one fetch, rebuilt when the fork's steward resumes the aggregator",
		"ap_followers":                "remote followers belong to the old instance's identity",
		"ap_following":                "same, outbound",
		"ap_outbox_queue":             "delivery state, not a record of anything",
		"audit_log":                   "instance operations, not community data",
		"content_reports":             "moderation history is about the old instance's handling",
		"credentials":                 "passkeys are bound to the old domain",
		"edges":                       "retired concept: connections are inferred (CLAUDE.md)",
		"instance_actor":              "the fork is a different actor",
		"instance_icon":               "retired by migration 044",
		"instance_settings":           "the fork sets its own name, policy, and icon",
		"invite_links":                "single-use URLs pointing at the old domain",
		"label":                       "instance-level cost transparency, not a patch's data",
		"label_cost_items":            "same",
		"label_stewards":              "same",
		"magic_links":                 "short-lived auth tokens",
		"neighbor_quilts":             "the fork curates its own neighbors",
		"notifications":               "in-app rows are read state, regenerated by what travels",
		"notification_reminders_sent": "dedup state for reminders already sent from the old instance",
		"recovery_codes":              "secrets",
		"remote_follows":              "a person's cross-quilt relationships, not the community's",
		"sessions":                    "secrets",
		"signup_tokens":               "single-use, domain-bound",
		"user_quilts":                 "a person's own connected quilts",
	}

	exported := map[string]bool{}
	for _, tab := range Tables() {
		exported[tab.Name] = true
	}

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'
	                       AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var undecided []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		if exported[name] {
			if reason, both := staysBehind[name]; both {
				t.Errorf("%s is both exported and listed as staying behind (%q)", name, reason)
			}
			continue
		}
		if _, known := staysBehind[name]; !known {
			undecided = append(undecided, name)
		}
	}

	for _, name := range undecided {
		t.Errorf("table %q has no portability decision: add it to Tables() so it "+
			"travels with the community, or to staysBehind here with the reason", name)
	}
}

// Every column of an exported table is either exported or deliberately left
// behind.
//
// TestEveryTableHasABoundaryDecision catches a whole table nobody decided
// about. It does not catch a column, and a column is how this has actually
// gone wrong twice: `seats_contested` reached the schema without reaching
// Tables(), so a forked election arrived carrying candidates and no seats to
// contest; and a careless edit dropped `website` and `links` out of the nodes
// column list, which no test noticed because the table was still there.
//
// Adding a column to an exported table now fails here until someone says which
// side of the boundary it falls on.
func TestEveryColumnHasABoundaryDecision(t *testing.T) {
	db := testDB(t)

	// Per table, the columns that deliberately do not travel: instance
	// identity, secrets, fetch state, retired columns, and rows the export
	// already filters out (docs/adr/002).
	staysBehind := map[string]map[string]string{
		"users": {
			"private_key":          "ActivityPub keypair; a fork mints its own on first boot",
			"public_key":           "same",
			"ap_id":                "names an actor on the old domain",
			"ap_type":              "same",
			"feed_secret_hash":     "a personal calendar URL secret",
			"trusted_contributor":  "one instance's judgement about a person, not the community's",
			"hide_amended_linings": "a per-user discovery filter, not community data",
			"start_on_my_quilt":    "a per-user landing preference",
			"last_seen_at":         "derived, and rebuilt by using the fork",
		},
		"nodes": {
			"ap_id":               "names an actor on the old domain",
			"ap_type":             "same",
			"private_key":         "ActivityPub keypair",
			"public_key":          "same",
			"removed_at":          "the export filters these rows out",
			"verification_domain": "vetted by the old instance's claim review (docs/adr/030)",
			"theme":               "retired: replaced by appearance in migration 018",
			"parent_id":           "retired: patches are flat (docs/adr/009)",
			"node_type":           "retired with the container/leaf split (docs/adr/009)",
		},
		"events": {
			"ap_id":      "names an object on the old domain",
			"ap_type":    "same",
			"removed_at": "the export filters these rows out",
			"status":     "the export takes active events only",
		},
		"proposals": {
			"ap_id":           "names an object on the old domain",
			"ap_type":         "same",
			"proposed_branch": "a git branch in a repo that does not travel (docs/adr/002 known gap)",
			"git_sha":         "a commit in that repo",
			"base_sha":        "same",
		},
		"votes":             {"ap_id": "names an object on the old domain"},
		"proposal_comments": {"ap_id": "names an object on the old domain"},
		"aggregators": {
			"status":          "fetch state; the fork re-syncs from scratch",
			"last_fetch_at":   "same",
			"last_success_at": "same",
			"last_error":      "same",
			"etag":            "an HTTP cache validator for the old instance's last fetch",
			"last_modified":   "same",
		},
		"event_sources": {
			"status":          "fetch state; the fork re-syncs every feed from scratch",
			"last_fetch_at":   "same",
			"last_success_at": "same",
			"last_error":      "same",
			"etag":            "an HTTP cache validator for the old instance's last fetch",
			"last_modified":   "same",
		},
		"claim_requests": {
			"verification_token":     "a secret the old instance issued (docs/adr/002 says claims travel minus these)",
			"email":                  "contact for that instance's review, not the patch's data",
			"email_token_expires_at": "expiry of a token that did not travel",
			"email_send_count":       "rate-limit state for the old instance",
			"email_window_start":     "same",
			"setup_expires_at":       "a deadline set by the old instance's review",
		},
		"memberships": {"join_message": "written to that instance's admins during review, not to the fork's"},
	}

	for _, tab := range Tables() {
		exported := map[string]bool{}
		for _, col := range tab.Columns {
			exported[col.Name] = true
		}

		rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, tab.Name)
		if err != nil {
			t.Errorf("%s: %v", tab.Name, err)
			continue
		}
		var undecided []string
		for rows.Next() {
			var col string
			if rows.Scan(&col) != nil {
				continue
			}
			if exported[col] {
				continue
			}
			if _, known := staysBehind[tab.Name][col]; known {
				continue
			}
			undecided = append(undecided, col)
		}
		rows.Close()

		for _, col := range undecided {
			t.Errorf("%s.%s has no portability decision: add it to that table's Columns "+
				"so it travels, or to staysBehind here with the reason", tab.Name, col)
		}
	}
}
