package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-137: the proposals list sent nothing that told an election apart, so a
// contest that settled on no ballots matched the list's test for a direct
// change and read "applied by" the founder. The list now sends the same
// election_phase the detail page reads.
func TestListProposals_AnElectionSaysItIsOne(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	admin, _ := createTestUser(t, db, "listelect", "member")
	nodeID := createTestNode(t, db, admin.ID, "List Elect", "list-elect", "open")
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seed := func(title string, seats int) {
		if _, err := db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		         duration_hours, voting_ends_at, created_at, updated_at, seats_contested)
		         VALUES (?, ?, ?, ?, '', 'approved', 'in_effect', 'other', 72, NULL, '2026-08-01T10:00:00.000Z', '2026-08-04T10:00:00.000Z', ?)`,
			auth.NewUUIDv7(), nodeID, admin.ID, title, seats); err != nil {
			t.Fatalf("seed %s: %v", title, err)
		}
	}
	seed("Council election", 3)
	seed("Changed the hours", 0)

	r := authedRequest("GET", "/api/v1/nodes/list-elect/proposals?status=all", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	rows := map[string]map[string]interface{}{}
	items, _ := decodeJSON(t, w)["items"].([]interface{})
	for _, it := range items {
		m, _ := it.(map[string]interface{})
		title, _ := m["title"].(string)
		rows[title] = m
	}
	if got := rows["Council election"]["election_phase"]; got != "closed" {
		t.Errorf("a settled election should say so, got election_phase %v", got)
	}
	if got := rows["Council election"]["seats_contested"]; got != float64(3) {
		t.Errorf("seats_contested = %v, want 3", got)
	}
	if got, present := rows["Changed the hours"]["election_phase"]; present {
		t.Errorf("a proposal that is not an election carries no phase, got %v", got)
	}
}

// F-070: a settled vote box says how the viewer voted, and says "You did not
// vote" only to somebody who could have. could_have_voted is that second
// half: an admin or member whose tenure had run by the time voting closed,
// and never somebody who joined afterwards.
func TestGetProposal_CouldHaveVotedOnlyWhereTrue(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	admin, adminToken := createTestUser(t, db, "settledadmin", "member")
	early, earlyToken := createTestUser(t, db, "settledearly", "member")
	late, lateToken := createTestUser(t, db, "settledlate", "member")
	_, strangerToken := createTestUser(t, db, "settledstranger", "member")
	nodeID := createTestNode(t, db, admin.ID, "Settled", "settled-patch", "open")
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, early.ID, nodeID, "member", "active")
	createTestMembership(t, db, late.ID, nodeID, "member", "active")
	setJoined(t, db, nodeID, admin.ID, "2026-01-01T00:00:00.000Z")
	setJoined(t, db, nodeID, early.ID, "2026-01-01T00:00:00.000Z")
	setJoined(t, db, nodeID, late.ID, "2026-09-10T00:00:00.000Z")

	id := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
	         duration_hours, voting_ends_at, created_at, updated_at, applied_at)
	         VALUES (?, ?, ?, 'Settled one', '', 'approved', 'in_effect', 'other', 72, '2026-09-04T10:00:00.000Z',
	                 '2026-09-01T10:00:00.000Z', '2026-09-04T10:00:00.000Z', '2026-09-04T10:00:00.000Z')`,
		id, nodeID, admin.ID); err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		auth.NewUUIDv7(), id, admin.ID); err != nil {
		t.Fatalf("seed vote: %v", err)
	}

	read := func(token string) map[string]interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/proposals/"+id, nil, token)
		w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)
	}

	if got := read(adminToken); got["my_vote"] != "approve" || got["could_have_voted"] != false {
		t.Errorf("the voter: my_vote %v, could_have_voted %v; want approve and false", got["my_vote"], got["could_have_voted"])
	}
	if got := read(earlyToken)["could_have_voted"]; got != true {
		t.Errorf("a member here before it closed could have voted, got %v", got)
	}
	if got := read(lateToken)["could_have_voted"]; got != false {
		t.Errorf("a member who joined after it closed could not have, got %v", got)
	}
	if got := read(strangerToken)["could_have_voted"]; got != false {
		t.Errorf("somebody holding no role here could not have, got %v", got)
	}
	if got := read("")["could_have_voted"]; got != false {
		t.Errorf("a signed-out reader could not have, got %v", got)
	}
}

func setJoined(t *testing.T, db *database.DB, nodeID, userID, at string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE memberships SET joined_at = ? WHERE node_id = ? AND user_id = ?`, at, nodeID, userID); err != nil {
		t.Fatalf("set joined_at: %v", err)
	}
}
