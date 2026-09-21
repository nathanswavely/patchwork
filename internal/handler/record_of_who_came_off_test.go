package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-100. The governance record held everything a patch had settled and
// nothing about who ran it.
//
// A seat vacated for inactivity and the interim promotion that answers it
// are written to the audit log, which is instance-admin only. So the single
// largest governance event a patch can have — its council emptying — reached
// no member-facing surface at all. A founder came back for exactly that,
// twice: "Nothing on this site tells me when Devon and Ana came off, or why.
// The Record is headed 'Everything this patch has settled' and it does not
// contain the single most important event in the co-op's year."

func recordEntriesFor(t *testing.T, db *database.DB, slug, token string) []map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/record", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("record: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	raw, _ := decodeJSON(t, w)["items"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, e := range raw {
		out = append(out, e.(map[string]interface{}))
	}
	return out
}

func seatEntries(entries []map[string]interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, e := range entries {
		if e["kind"] == "seat" {
			out = append(out, e)
		}
	}
	return out
}

// membershipIDFor is the row the audit entries point at.
func membershipIDFor(t *testing.T, db *database.DB, nodeID, userID string) string {
	t.Helper()
	var id string
	db.QueryRow(`SELECT id FROM memberships WHERE node_id = ? AND user_id = ?`, nodeID, userID).Scan(&id)
	if id == "" {
		t.Fatal("no membership row to hang an audit entry on")
	}
	return id
}

func TestRecord_SaysWhoCameOffTheCouncilAndWhy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "cameoff", "member")
	nodeID := createTestNode(t, db, owner.ID, "Came Off", "came-off", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	devon, _ := createTestUser(t, db, "cameoff-devon", "member")
	createTestMembership(t, db, devon.ID, nodeID, "member", "active")

	// What the sweep writes when a council empties, and what it writes when
	// somebody is handed a seat to answer it.
	auth.LogAuditEvent(db, "", "membership.seat_vacated", "membership",
		membershipIDFor(t, db, nodeID, devon.ID),
		`{"node_id":"`+nodeID+`","reason":"inactivity"}`, "")
	auth.LogAuditEvent(db, "", "membership.succession", "membership",
		membershipIDFor(t, db, nodeID, owner.ID),
		`{"node_id":"`+nodeID+`","policy":"longest_tenure"}`, "")

	seats := seatEntries(recordEntriesFor(t, db, "came-off", token))
	if len(seats) != 2 {
		t.Fatalf("expected the two council changes on the record, got %d", len(seats))
	}

	byOutcome := map[string]map[string]interface{}{}
	for _, e := range seats {
		byOutcome[e["outcome"].(string)] = e
	}
	// The "why" half of her question is carried by the outcome, so the page
	// can say it in a sentence rather than printing the reason as a word.
	vacated, ok := byOutcome["vacated_inactivity"]
	if !ok {
		t.Fatalf("the record does not say a seat was vacated for inactivity: %v", byOutcome)
	}
	if vacated["title"] != "cameoff-devon" {
		t.Errorf("the vacated entry names %v, want the person who came off", vacated["title"])
	}
	if vacated["summary"] != nil {
		t.Errorf("the reason leaked into the generic summary line: %v", vacated["summary"])
	}
	if _, ok := byOutcome["stepped_in"]; !ok {
		t.Error("the record does not say who stepped in")
	}
}

// A person promoting or demoting another is a council change too, and the
// record names who did it.
func TestRecord_NamesWhoMadeAnAdminAndWhoStoodOneDown(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "madeadmin", "member")
	nodeID := createTestNode(t, db, owner.ID, "Made Admin", "made-admin", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "madeadmin-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")
	memID := membershipIDFor(t, db, nodeID, other.ID)

	auth.LogAuditEvent(db, owner.ID, "membership.role_change", "membership", memID,
		`{"target_user_id":"`+other.ID+`","old_role":"member","new_role":"admin"}`, "")
	auth.LogAuditEvent(db, owner.ID, "membership.role_change", "membership", memID,
		`{"target_user_id":"`+other.ID+`","old_role":"admin","new_role":"member"}`, "")

	seats := seatEntries(recordEntriesFor(t, db, "made-admin", token))
	if len(seats) != 2 {
		t.Fatalf("expected both role changes, got %d", len(seats))
	}
	for _, e := range seats {
		if e["actor"] != "madeadmin" {
			t.Errorf("entry does not name who did it: actor = %v", e["actor"])
		}
	}
}

// A change that never touched the council is not a council event.
func TestRecord_IgnoresARoleChangeThatMissedTheCouncil(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "notcouncil", "member")
	nodeID := createTestNode(t, db, owner.ID, "Not Council", "not-council", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "notcouncil-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	auth.LogAuditEvent(db, owner.ID, "membership.role_change", "membership",
		membershipIDFor(t, db, nodeID, other.ID),
		`{"target_user_id":"`+other.ID+`","old_role":"follower","new_role":"member"}`, "")

	if seats := seatEntries(recordEntriesFor(t, db, "not-council", token)); len(seats) != 0 {
		t.Errorf("a follower becoming a member is a relationship, not a seat: got %d entries", len(seats))
	}
}

// Another patch's council changes are not this patch's record.
func TestRecord_DoesNotBorrowAnotherPatchsCouncil(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "mine", "member")
	mineID := createTestNode(t, db, owner.ID, "Mine", "mine", "open")
	createTestMembership(t, db, owner.ID, mineID, "admin", "active")

	theirsOwner, _ := createTestUser(t, db, "theirs", "member")
	theirsID := createTestNode(t, db, theirsOwner.ID, "Theirs", "theirs", "open")
	createTestMembership(t, db, theirsOwner.ID, theirsID, "admin", "active")

	auth.LogAuditEvent(db, "", "membership.seat_vacated", "membership",
		membershipIDFor(t, db, theirsID, theirsOwner.ID),
		`{"node_id":"`+theirsID+`","reason":"inactivity"}`, "")

	if seats := seatEntries(recordEntriesFor(t, db, "mine", token)); len(seats) != 0 {
		t.Errorf("another patch's seat change appeared here: got %d", len(seats))
	}
}

// The record can be public (docs/adr/095's sibling control), and a public
// one must not become the roster by another route.
func TestRecord_WithholdsTheNamesFromOutsideARoomThatPublishes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, _ := createTestUser(t, db, "pubrec", "member")
	nodeID := createTestNode(t, db, owner.ID, "Pub Rec", "pub-rec", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	if _, err := db.Exec(`UPDATE nodes SET public_governance_record = 'everyone' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("publish the record: %v", err)
	}
	auth.LogAuditEvent(db, "", "membership.seat_vacated", "membership",
		membershipIDFor(t, db, nodeID, owner.ID),
		`{"node_id":"`+nodeID+`","reason":"inactivity"}`, "")

	// Anonymous: the record is readable, the person is not named.
	r := authedRequest("GET", "/api/v1/nodes/pub-rec/governance/record", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(db), r)
	raw, _ := decodeJSON(t, w)["items"].([]interface{})
	var found bool
	for _, it := range raw {
		e := it.(map[string]interface{})
		if e["kind"] != "seat" {
			continue
		}
		found = true
		if e["title"] != handler.HiddenMemberName {
			t.Errorf("a public record named the holder to an outsider: %v", e["title"])
		}
	}
	if !found {
		t.Error("the seat change is missing from a record the patch publishes")
	}
}

// And inside the room it reads normally, which is the whole point.
func TestRecord_NamesThemInsideTheRoom(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, _ := createTestUser(t, db, "inroom", "member")
	nodeID := createTestNode(t, db, owner.ID, "In Room", "in-room", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "inroom-member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	auth.LogAuditEvent(db, "", "membership.seat_vacated", "membership",
		membershipIDFor(t, db, nodeID, owner.ID),
		`{"node_id":"`+nodeID+`","reason":"inactivity"}`, "")

	seats := seatEntries(recordEntriesFor(t, db, "in-room", memberToken))
	if len(seats) != 1 {
		t.Fatalf("expected the seat change, got %d", len(seats))
	}
	if seats[0]["title"] != "inroom" {
		t.Errorf("a member of the patch was not shown who came off: %v", seats[0]["title"])
	}
}
