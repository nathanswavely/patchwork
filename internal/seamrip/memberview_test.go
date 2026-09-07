package seamrip

import (
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The member seamrip's boundary (docs/adr/089): a member carries what they
// can already see, other people travel as stubs, and nothing in the bundle
// is a key.

type fixture struct {
	viewer, insider, hiddenPerson, stranger, ghost string
	pubOwn, privOwn, pubOther, privOther           string
	membersDocOwn, membersDocOther                 string
	membersEventOwn, membersEventOther             string
	proposalOwn                                    string
}

// seedTwoRooms builds a quilt with two rooms the viewer is in and two they
// are not, each carrying the things that are supposed to stop at the door:
// a hidden membership, a members-only charter, a members-only event, a
// notice.
func seedTwoRooms(t *testing.T, db *database.DB) fixture {
	t.Helper()
	f := fixture{
		viewer: nextID(), insider: nextID(), hiddenPerson: nextID(),
		stranger: nextID(), ghost: nextID(),
		pubOwn: nextID(), privOwn: nextID(), pubOther: nextID(), privOther: nextID(),
	}
	now := "2026-01-01T00:00:00Z"

	people := []struct{ id, name string }{
		{f.viewer, "weaver"}, {f.insider, "binder"}, {f.hiddenPerson, "quiet"},
		{f.stranger, "stranger"}, {f.ghost, "ghost"},
	}
	for _, p := range people {
		mustExec(t, db,
			`INSERT INTO users (id, email, username, display_name, bio, avatar_url, links, role, contact_phone, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'Sews at night', 'https://cdn.example/a.png', '[{"url":"https://example.com","label":"Site"}]', 'member', '+1 717 555 0100', ?, ?)`,
			p.id, p.name+"@example.com", p.name, strings.ToUpper(p.name[:1])+p.name[1:], now, now)
	}
	// The ghost deleted their account (docs/adr/086): the row survives as a
	// tombstone, and the handle stays retired on it.
	mustExec(t, db,
		`UPDATE users SET email = NULL, display_name = '', bio = '', avatar_url = '', links = '[]',
		 contact_phone = '', deleted_at = ? WHERE id = ?`, now, f.ghost)

	rooms := []struct {
		id, name, slug, visibility string
	}{
		{f.pubOwn, "Gallery Row", "gallery-row", "public"},
		{f.privOwn, "The Selvage", "the-selvage", "private"},
		{f.pubOther, "Binns Park", "binns-park", "public"},
		{f.privOther, "Back Room", "back-room", "private"},
	}
	for _, n := range rooms {
		mustExec(t, db,
			`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', ?, 'open', 'active', ?, ?)`,
			n.id, f.insider, n.name, n.slug, n.visibility, now, now)
	}

	type mem struct {
		user, node, role string
		visible          int
	}
	for _, m := range []mem{
		{f.viewer, f.pubOwn, "member", 1},
		{f.viewer, f.privOwn, "member", 1},
		{f.insider, f.pubOwn, "admin", 1},
		{f.insider, f.privOwn, "admin", 1},
		{f.hiddenPerson, f.pubOwn, "member", 0},
		{f.stranger, f.pubOther, "admin", 1},
		{f.hiddenPerson, f.pubOther, "member", 0},
		{f.stranger, f.privOther, "admin", 1},
	} {
		mustExec(t, db,
			`INSERT INTO memberships (id, user_id, node_id, role, status, visible, joined_at) VALUES (?, ?, ?, ?, 'active', ?, ?)`,
			nextID(), m.user, m.node, m.role, m.visible, now)
	}

	// Charters: one public and one members-only in each room.
	f.membersDocOwn, f.membersDocOther = nextID(), nextID()
	for _, d := range []struct{ id, node, title, visibility string }{
		{nextID(), f.pubOwn, "Charter", "public"},
		{f.membersDocOwn, f.pubOwn, "House Rules", "members"},
		{nextID(), f.pubOther, "Charter", "public"},
		{f.membersDocOther, f.pubOther, "House Rules", "members"},
	} {
		mustExec(t, db,
			`INSERT INTO governance_docs (id, node_id, title, body, visibility, version, created_by) VALUES (?, ?, ?, 'The rule.', ?, 1, ?)`,
			d.id, d.node, d.title, d.visibility, f.insider)
	}

	// Events: one public and one members-only in each room.
	f.membersEventOwn, f.membersEventOther = nextID(), nextID()
	for _, e := range []struct{ id, node, title, visibility string }{
		{nextID(), f.pubOwn, "Open Show", "public"},
		{f.membersEventOwn, f.pubOwn, "Members Meeting", "private"},
		{nextID(), f.pubOther, "Park Show", "public"},
		{f.membersEventOther, f.pubOther, "Their Meeting", "private"},
	} {
		mustExec(t, db,
			`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility, created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', 'Venue', ?, '', ?, ?, ?)`,
			e.id, e.node, f.insider, e.title, now, e.visibility, now, now)
	}

	// A proposal the ghost wrote before deleting, so the bundle has to carry
	// a tombstone to keep the record whole.
	f.proposalOwn = nextID()
	mustExec(t, db,
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, target_doc, proposed_body, created_at, updated_at)
		 VALUES (?, ?, ?, 'Amend the house rules', '', 'open', 'voting', 'house-rules.md', 'SECRET AMENDMENT TEXT', ?, ?)`,
		f.proposalOwn, f.pubOwn, f.ghost, now, now)
	mustExec(t, db, `INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		nextID(), f.proposalOwn, f.viewer)

	// A proposal in the other room amending THEIR members-only charter.
	otherProposal := nextID()
	mustExec(t, db,
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, target_doc, proposed_body, created_at, updated_at)
		 VALUES (?, ?, ?, 'Amend their house rules', '', 'open', 'voting', 'house-rules.md', 'THEIR SECRET TEXT', ?, ?)`,
		otherProposal, f.pubOther, f.stranger, now, now)

	// Noticeboards in both rooms. Neither travels, not even the viewer's own
	// (docs/adr/081): the room is the members', and a person carries their
	// own notices out through the personal export.
	for _, n := range []struct{ node, author string }{{f.pubOwn, f.viewer}, {f.pubOther, f.stranger}} {
		notice := nextID()
		mustExec(t, db,
			`INSERT INTO notices (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'The PA is broken', 'Again.')`,
			notice, n.node, n.author)
		mustExec(t, db,
			`INSERT INTO notice_replies (id, notice_id, author_id, body) VALUES (?, ?, ?, 'On it.')`,
			nextID(), notice, n.author)
	}

	// A calendar feed on each room. Only an admin sees the URL, and the
	// viewer administers neither.
	for _, node := range []string{f.pubOwn, f.pubOther} {
		mustExec(t, db,
			`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', 'https://calendar.example/secret-token.ics', ?)`,
			nextID(), node, f.insider)
	}

	// Contact cards and a claim, neither of which is anybody's to carry out.
	mustExec(t, db, `UPDATE memberships SET share_contact = 1 WHERE user_id = ? AND node_id = ?`, f.hiddenPerson, f.pubOwn)
	mustExec(t, db,
		`INSERT INTO claim_requests (id, node_id, user_id, method, evidence, status) VALUES (?, ?, ?, 'email', 'me@venue.example', 'pending')`,
		nextID(), f.pubOther, f.stranger)

	return f
}

// memberFiles runs a member seamrip into an in-memory file set.
func memberFiles(t *testing.T, db *database.DB, viewerID string) map[string][]map[string]any {
	t.Helper()
	files := map[string][]map[string]any{}
	if err := MemberExport(db, viewerID, func(tab Table, items []map[string]any) error {
		files[tab.File] = items
		return nil
	}); err != nil {
		t.Fatalf("member export: %v", err)
	}
	return files
}

func has(items []map[string]any, key, want string) bool {
	for _, item := range items {
		if s, _ := item[key].(string); s == want {
			return true
		}
	}
	return false
}

func TestMemberExport_PrivatePatchNeedsARole(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)

	mine := memberFiles(t, db, f.viewer)
	if !has(mine["nodes.json"], "id", f.privOwn) {
		t.Error("a member of a private patch did not get it")
	}
	if has(mine["nodes.json"], "id", f.privOther) {
		t.Error("a private patch the viewer holds no role on travelled")
	}
	if !has(mine["nodes.json"], "id", f.pubOther) {
		t.Error("a public patch the viewer is not in should still travel")
	}

	theirs := memberFiles(t, db, f.stranger)
	if has(theirs["nodes.json"], "id", f.privOwn) {
		t.Error("a non-member carried out a private patch")
	}
	if !has(theirs["nodes.json"], "id", f.privOther) {
		t.Error("the stranger's own private patch did not travel for them")
	}
}

func TestMemberExport_HiddenMembershipStopsAtTheDoor(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)

	// docs/adr/006: a hidden membership stays visible inside the workspace,
	// so a member of that patch carries it. Outside, it does not exist.
	mine := memberFiles(t, db, f.viewer)
	if !hasMembership(mine, f.hiddenPerson, f.pubOwn) {
		t.Error("a hidden membership did not travel for a member of that patch")
	}
	if hasMembership(mine, f.hiddenPerson, f.pubOther) {
		t.Error("a hidden membership travelled to somebody outside that patch")
	}

	theirs := memberFiles(t, db, f.stranger)
	if hasMembership(theirs, f.hiddenPerson, f.pubOwn) {
		t.Error("an outsider carried out a hidden membership")
	}
}

func hasMembership(files map[string][]map[string]any, userID, nodeID string) bool {
	for _, m := range files["memberships.json"] {
		u, _ := m["user_id"].(string)
		n, _ := m["node_id"].(string)
		if u == userID && n == nodeID {
			return true
		}
	}
	return false
}

func TestMemberExport_MembersOnlyCharterAndEvent(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)

	mine := memberFiles(t, db, f.viewer)
	if !has(mine["governance.json"], "id", f.membersDocOwn) {
		t.Error("a members-only charter did not travel for a member of that patch")
	}
	if has(mine["governance.json"], "id", f.membersDocOther) {
		t.Error("another patch's members-only charter travelled")
	}
	if !has(mine["events.json"], "id", f.membersEventOwn) {
		t.Error("a members-only event did not travel for a member of that patch")
	}
	if has(mine["events.json"], "id", f.membersEventOther) {
		t.Error("another patch's members-only event travelled")
	}

	// The amendment text mirrored into a proposal follows the charter, or a
	// members-only charter is one proposal away from being world-readable
	// (docs/adr/036).
	for _, p := range mine["proposals.json"] {
		body, _ := p["proposed_body"].(string)
		if strings.Contains(body, "THEIR SECRET TEXT") {
			t.Error("mirrored charter text leaked out of a patch the viewer is not in")
		}
	}
	found := false
	for _, p := range mine["proposals.json"] {
		if id, _ := p["id"].(string); id == f.proposalOwn {
			found = true
			if body, _ := p["proposed_body"].(string); body != "SECRET AMENDMENT TEXT" {
				t.Errorf("a member lost the amendment text of their own patch: %q", body)
			}
		}
	}
	if !found {
		t.Error("the viewer's own patch's proposal did not travel")
	}
}

func TestMemberExport_NeverTravels(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)
	mine := memberFiles(t, db, f.viewer)

	for _, file := range []string{
		"notices.json", "notice_replies.json", "claim_requests.json",
		"notification_preferences.json", "aggregators.json",
	} {
		if len(mine[file]) != 0 {
			t.Errorf("%s travelled in a member seamrip: %d rows", file, len(mine[file]))
		}
	}

	// A feed URL can carry a token, so it travels only for an admin of that
	// patch — and the viewer administers neither room.
	if len(mine["event_sources.json"]) != 0 {
		t.Errorf("a calendar feed travelled to a non-admin: %d rows", len(mine["event_sources.json"]))
	}
}

func TestMemberExport_PeopleAreStubs(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)
	mine := memberFiles(t, db, f.viewer)

	if len(mine["users.json"]) == 0 {
		t.Fatal("nobody travelled")
	}
	for _, u := range mine["users.json"] {
		id, _ := u["id"].(string)
		if u["email"] != nil {
			t.Errorf("user %s carried an email address", id)
		}
		if bio, _ := u["bio"].(string); bio != "" {
			t.Errorf("user %s carried a bio", id)
		}
		if phone, _ := u["contact_phone"].(string); phone != "" {
			t.Errorf("user %s carried a contact card", id)
		}
		if links, _ := u["links"].(string); links != "[]" {
			t.Errorf("user %s carried profile links: %q", id, links)
		}
		for _, forbidden := range []string{"private_key", "public_key", "ap_id", "feed_secret_hash"} {
			if _, present := u[forbidden]; present {
				t.Errorf("users.json leaks %s", forbidden)
			}
		}
	}

	// The stranger has no visible relationship to anything the viewer can
	// see except their own public patch's admin row, so they DO travel —
	// through that membership. The tombstone travels as a tombstone.
	var ghostSeen bool
	for _, u := range mine["users.json"] {
		if id, _ := u["id"].(string); id == f.ghost {
			ghostSeen = true
			if u["deleted_at"] == nil {
				t.Error("a deleted account arrived on the fork as a live one")
			}
			if name, _ := u["display_name"].(string); name != "" {
				t.Errorf("a tombstone carried a display name: %q", name)
			}
			if handle, _ := u["username"].(string); handle != "ghost" {
				t.Errorf("a tombstone lost the handle that keeps it retired: %q", handle)
			}
		}
	}
	if !ghostSeen {
		t.Error("the author of a travelling proposal did not travel, so the fork drops the proposal")
	}
}

// The closure property, stated as the failure it prevents: every user id any
// travelling row names has a users row in the bundle. Without it the import
// mints an id for a person who is not there and SQLite refuses the insert.
func TestMemberExport_EveryReferencedPersonTravels(t *testing.T) {
	db := testDB(t)
	f := seedTwoRooms(t, db)
	mine := memberFiles(t, db, f.viewer)

	present := map[string]bool{SentinelUserID: true}
	for _, u := range mine["users.json"] {
		if id, _ := u["id"].(string); id != "" {
			present[id] = true
		}
	}

	for _, tab := range Tables() {
		if tab.Name == "users" {
			continue
		}
		cols, err := userRefColumns(db, tab.Name)
		if err != nil {
			t.Fatal(err)
		}
		for _, col := range cols {
			for _, row := range mine[tab.File] {
				id, _ := row[col].(string)
				if id != "" && !present[id] {
					t.Errorf("%s.%s names %s, who is not in the bundle", tab.Name, col, id)
				}
			}
		}
	}
}

// The round trip: a member's bundle imports into a fresh database and the
// fork arrives with the patches they could see, the people as stubs, and the
// memberships that the threads are inferred from.
func TestMemberExport_RoundTrip(t *testing.T) {
	src := testDB(t)
	f := seedTwoRooms(t, src)
	files := memberFiles(t, src, f.viewer)

	dst := testDB(t)
	_, results, err := Import(dst,
		func(file string) ([]map[string]any, error) { return files[file], nil },
		nextID)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, r := range results {
		if r.Skipped > 0 {
			t.Errorf("table %s skipped %d rows importing a member seamrip", r.Table, r.Skipped)
		}
	}

	if n := count(t, dst, `SELECT COUNT(*) FROM nodes WHERE slug IN ('gallery-row','the-selvage','binns-park')`); n != 3 {
		t.Errorf("the fork arrived with %d of 3 visible patches", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM nodes WHERE slug = 'back-room'`); n != 0 {
		t.Error("a private patch the member holds no role on reached the fork")
	}

	// Nobody can be signed in from this file.
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE email IS NOT NULL AND username != '_system'`); n != 0 {
		t.Errorf("%d email addresses reached the fork", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE private_key IS NOT NULL`); n != 0 {
		t.Errorf("%d keys reached the fork", n)
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM notices`); n != 0 {
		t.Errorf("%d notices reached the fork", n)
	}

	// The stubs are enough to name people.
	if n := count(t, dst, `SELECT COUNT(*) FROM users WHERE username = 'weaver' AND display_name = 'Weaver'`); n != 1 {
		t.Error("the person who took the bundle is not in it under their own name")
	}

	// THE property the seamrip exists for: the shared-member overlap the
	// threads and the quilt are inferred from survives the move.
	overlap := count(t, dst, `
		SELECT COUNT(*) FROM memberships m1
		JOIN memberships m2 ON m1.user_id = m2.user_id AND m1.node_id != m2.node_id
		WHERE m1.role IN ('admin','member') AND m2.role IN ('admin','member')`)
	if overlap < 4 {
		t.Errorf("member overlap lost: got %d directed overlap rows, want at least 4", overlap)
	}

	// The record stays a record: the proposal keeps its proposer, even
	// though the proposer is a tombstone.
	if n := count(t, dst, `SELECT COUNT(*) FROM proposals p JOIN users u ON u.id = p.author_id
	                        WHERE p.title = 'Amend the house rules' AND u.deleted_at IS NOT NULL`); n != 1 {
		t.Error("the proposal lost its proposer across the fork")
	}
	if n := count(t, dst, `SELECT COUNT(*) FROM votes`); n != 1 {
		t.Errorf("the tally lost its votes: %d", n)
	}

	var violations int
	dst.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations)
	if violations != 0 {
		t.Errorf("the fork has %d foreign key violations", violations)
	}
}
