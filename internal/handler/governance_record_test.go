package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// What a patch has decided, in order (docs/adr/055).
//
// The record stores nothing. Every entry is a view of a row some other feature
// owns, so the tests that matter are about what it includes, what it leaves
// out, and whether it can tell four kinds of settling apart — a vote, a change
// an admin applied, an election, and a decision a meeting made.

func readRecord(t *testing.T, db *database.DB, slug string) []map[string]interface{} {
	t.Helper()
	// Signed out, behind AuthOptional, the way it is actually mounted — the
	// record being readable without an account is the point.
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/record", nil, "")
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	raw, _ := decodeJSON(t, w)["items"].([]interface{})
	out := []map[string]interface{}{}
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func seedSettled(t *testing.T, db *database.DB, nodeID, authorID, title, status, state, decidedAt string, seats int) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		 duration_hours, created_at, updated_at, applied_at, seats_contested)
		 VALUES (?, ?, ?, ?, '', ?, ?, 'other', 72, ?, ?, ?, ?)`,
		id, nodeID, authorID, title, status, state, decidedAt, decidedAt, decidedAt, seats,
	); err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
	return id
}

// An open proposal is an argument in progress. Putting it in the record would
// make the record a to-do list.
func TestGovernanceRecord_OnlySettledThings(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rec One", "rec-one", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seedSettled(t, db, nodeID, admin.ID, "Still arguing", "open", "voting", "2026-03-01T00:00:00.000Z", 0)
	seedSettled(t, db, nodeID, admin.ID, "Settled", "approved", "in_effect", "2026-03-02T00:00:00.000Z", 0)

	entries := readRecord(t, db, "rec-one")
	if len(entries) != 1 {
		t.Fatalf("expected only the settled proposal, got %d entries", len(entries))
	}
	if entries[0]["title"] != "Settled" {
		t.Errorf("expected the settled one, got %v", entries[0]["title"])
	}
}

// The four ways a thing gets settled have to be distinguishable, or the record
// says "decided" about acts that carry very different weight (docs/adr/041,
// docs/adr/051, docs/adr/052).
func TestGovernanceRecord_TellsTheKindsApart(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec2", "member")
	voter, _ := createTestUser(t, db, "rec2v", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rec Two", "rec-two", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	// A vote somebody actually cast.
	voted := seedSettled(t, db, nodeID, admin.ID, "Voted on", "approved", "in_effect", "2026-03-01T00:00:00.000Z", 0)
	db.Exec(`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		auth.NewUUIDv7(), voted, voter.ID)

	// A direct change: born applied, no ballot ever existed.
	seedSettled(t, db, nodeID, admin.ID, "Applied directly", "approved", "in_effect", "2026-03-02T00:00:00.000Z", 0)

	// An election that seated somebody.
	seedSettled(t, db, nodeID, admin.ID, "Council election", "approved", "in_effect", "2026-03-03T00:00:00.000Z", 2)

	// A council a meeting chose.
	att := auth.NewUUIDv7()
	db.Exec(`INSERT INTO attestations (id, node_id, kind, decided_at, summary, recorded_by)
	         VALUES (?, ?, 'leadership', '2026-03-04', 'Annual meeting', ?)`, att, nodeID, admin.ID)
	db.Exec(`INSERT INTO attestation_names (id, attestation_id, display_name, position)
	         VALUES (?, ?, 'Dolores Vega', 0)`, auth.NewUUIDv7(), att)

	// A text a meeting adopted.
	db.Exec(`INSERT INTO amendment_attestations (id, node_id, target_doc, doc_title, decided_at, summary, adopted_body, recorded_by)
	         VALUES (?, ?, 'bylaws.md', 'Bylaws', '2026-03-05', 'Amended at the AGM', 'text', ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID)

	kinds := map[string]map[string]interface{}{}
	for _, e := range readRecord(t, db, "rec-two") {
		kinds[e["kind"].(string)] = e
	}

	for _, want := range []string{"vote", "direct", "election", "council", "adoption"} {
		if _, ok := kinds[want]; !ok {
			t.Errorf("the record cannot show a %q", want)
		}
	}

	// The record never carries a tally: the outcome is stored and fixed, the
	// counts are recomputed and drift (docs/adr/044), and the two contradicted
	// each other on real data. The proposal page holds the arithmetic.
	if _, has := kinds["vote"]["approve"]; has {
		t.Error("the record must not carry a recomputed tally beside a stored outcome")
	}
	// A direct change names who applied it.
	if kinds["direct"]["actor"] == nil || kinds["direct"]["actor"] == "" {
		t.Error("a direct change must name the admin who applied it")
	}
	if got := kinds["council"]["names"]; got == nil {
		t.Error("a recorded council must say who it seated")
	}
}

// Newest first, across three sources with different date columns — and an
// attestation is dated by the meeting rather than by the row.
func TestGovernanceRecord_NewestFirstAcrossSources(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rec Three", "rec-three", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seedSettled(t, db, nodeID, admin.ID, "Older vote", "approved", "in_effect", "2026-01-05T00:00:00.000Z", 0)
	seedSettled(t, db, nodeID, admin.ID, "Newer vote", "approved", "in_effect", "2026-06-05T00:00:00.000Z", 0)
	// Typed in today, decided in March: the record orders by the meeting.
	db.Exec(`INSERT INTO amendment_attestations (id, node_id, target_doc, doc_title, decided_at, summary, adopted_body, recorded_by)
	         VALUES (?, ?, 'bylaws.md', 'Bylaws', '2026-03-01', '', 'text', ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID)

	entries := readRecord(t, db, "rec-three")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	titles := []string{}
	for _, e := range entries {
		titles = append(titles, e["title"].(string))
	}
	want := []string{"Newer vote", "Bylaws", "Older vote"}
	for i := range want {
		if titles[i] != want[i] {
			t.Errorf("position %d: expected %q, got %q (full order %v)", i, want[i], titles[i], titles)
		}
	}
}

// A correction supersedes rather than edits (docs/adr/052). The hub shows both,
// beside each other; the record shows the one in force, or a single meeting
// reads as two councils seated on one day.
func TestGovernanceRecord_SupersededCouncilsStayOut(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec4", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rec Four", "rec-four", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	first := auth.NewUUIDv7()
	db.Exec(`INSERT INTO attestations (id, node_id, kind, decided_at, summary, recorded_by)
	         VALUES (?, ?, 'leadership', '2026-03-04', 'First pass', ?)`, first, nodeID, admin.ID)
	db.Exec(`INSERT INTO attestations (id, node_id, kind, decided_at, summary, recorded_by, supersedes_id)
	         VALUES (?, ?, 'leadership', '2026-03-04', 'Corrected', ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, first)

	entries := readRecord(t, db, "rec-four")
	if len(entries) != 1 {
		t.Fatalf("expected only the record in force, got %d", len(entries))
	}
	if entries[0]["summary"] != "Corrected" {
		t.Errorf("expected the correction, got %v", entries[0]["summary"])
	}
}

// A patch that has decided nothing says so, rather than 404ing or erroring.
func TestGovernanceRecord_EmptyIsAnAnswer(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "rec5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rec Five", "rec-five", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	if entries := readRecord(t, db, "rec-five"); len(entries) != 0 {
		t.Errorf("expected an empty record, got %d entries", len(entries))
	}
}
