package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// When Patchwork changes a patch rather than a person changing it, the
// record has to say what moved and somebody who exists has to hear about it.
//
// The September rollout failed both. It closed follower access to
// members-only charters on two patches and delivered one notice, because
// the other was a four-member collective with an empty council and the
// notice was addressed to admins. And the migration beside it retracted
// `public_member_list` on every patch while `node.update` logged a literal
// "{}", so the operator it delegated the telling to could not produce the
// list of who to tell.

func TestNodeUpdate_AuditNamesTheFieldsThatChanged(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "auditfields", "member")
	nodeID := createTestNode(t, db, owner.ID, "Audit Fields", "audit-fields", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	body := map[string]interface{}{
		"description":        "a new description",
		"public_member_list": "nobody",
		// Ignored by the update, so it must not appear in the record either:
		// logging it would claim an edit that never happened.
		"not_a_field": "x",
	}
	r := authedRequest("PATCH", "/api/v1/nodes/audit-fields", body, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var detail string
	db.QueryRow(`SELECT metadata FROM audit_log WHERE action = 'node.update' AND entity_id = ?
	             ORDER BY created_at DESC LIMIT 1`, nodeID).Scan(&detail)

	var got struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(detail), &got); err != nil {
		t.Fatalf("audit detail is not the JSON this promises: %q", detail)
	}
	// Sorted, so two edits of the same settings read the same.
	want := []string{"description", "public_member_list"}
	if strings.Join(got.Fields, ",") != strings.Join(want, ",") {
		t.Errorf("fields = %v, want %v (detail: %s)", got.Fields, want, detail)
	}
}

// The question the migration could not answer: which patches had chosen
// `everyone` on purpose. It is answerable now.
func TestNodeUpdate_TheRosterSettingIsFindableInTheRecord(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "rosterfind", "member")
	nodeID := createTestNode(t, db, owner.ID, "Roster Find", "roster-find", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	// One edit that touches it, one that does not.
	for _, body := range []map[string]interface{}{
		{"public_member_list": "everyone"},
		{"description": "unrelated"},
	} {
		r := authedRequest("PATCH", "/api/v1/nodes/roster-find", body, token)
		if w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r); w.Code != http.StatusOK {
			t.Fatalf("update: got %d: %s", w.Code, w.Body.String())
		}
	}

	var hits int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log
	              WHERE action = 'node.update' AND entity_id = ?
	                AND metadata LIKE '%public_member_list%'`, nodeID).Scan(&hits)
	if hits != 1 {
		t.Errorf("expected exactly the one edit that touched the roster setting, found %d", hits)
	}
}

// An edit carrying nothing the update accepts records nothing at all.
//
// The handler refuses it with a 400 before the audit line is reached, which
// is better than the empty `{"fields":[]}` this was first written to expect:
// no row beats a row saying an edit happened and naming none of it. Asserted
// rather than assumed, because the audit detail is only trustworthy if the
// rows that exist correspond to edits that did.
func TestNodeUpdate_AnEditOfNothingIsRefusedAndRecordsNothing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	owner, token := createTestUser(t, db, "auditempty", "member")
	nodeID := createTestNode(t, db, owner.ID, "Audit Empty", "audit-empty", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/audit-empty", map[string]interface{}{"nope": 1}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for an edit naming no known field, got %d", w.Code)
	}

	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'node.update' AND entity_id = ?`,
		nodeID).Scan(&rows)
	if rows != 0 {
		t.Errorf("an edit that changed nothing left %d audit row(s)", rows)
	}
}

// F-110. A patch with no admins is a supported state (docs/adr/102), and it
// is the state in which a member most needs to hear that Patchwork edited
// the rules — they are the only people left who can start putting it back.
func TestFollowerChartersClosed_TellsMembersWhenThereAreNoAdmins(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	tellingPeople(t, db)

	owner, _ := createTestUser(t, db, "nc-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "No Council", "no-council", "open")
	// Members only. Nobody holds the admin role, exactly as an emptied
	// council leaves a patch.
	createTestMembership(t, db, owner.ID, nodeID, "member", "active")
	member, _ := createTestUser(t, db, "nc-member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	openFollowerCharters(t, db, nodeID)

	closed, err := handler.CloseFollowerChartersDefault(db)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed != 1 {
		t.Fatalf("expected the patch to be closed, closed = %d", closed)
	}

	if !heardAbout(t, db, owner.ID, "Followers can no longer read") {
		t.Error("a member of an admin-less patch was not told its rules were edited")
	}
	if !heardAbout(t, db, member.ID, "Followers can no longer read") {
		t.Error("the second member was not told either")
	}
}

// F-118. The notice named the one key that moved and left the reader to
// discover the other three for himself.
func TestFollowerChartersClosed_SaysWhatFollowersCanStillSee(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	tellingPeople(t, db)

	admin, _ := createTestUser(t, db, "shelf-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Whole Shelf", "whole-shelf", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	openFollowerCharters(t, db, nodeID)

	if _, err := handler.CloseFollowerChartersDefault(db); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !heardAbout(t, db, admin.ID, "Followers can no longer read") {
		t.Fatal("the admin was not told at all")
	}

	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND title LIKE 'Followers can no longer read%'
	             ORDER BY created_at DESC LIMIT 1`, admin.ID).Scan(&body)

	for _, want := range []string{"events", "proposals", "member list"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notice does not mention %q, so the reader still learns one tin and not the shelf: %q", want, body)
		}
	}
	// And it points at where the switch actually is.
	//
	// It used to say "in Governance" and link to the hub, where the switch
	// is not; then it named the change form, and an admin who followed it
	// found a Submit button he would not press to read a setting. Both are
	// answered by Rules being a page: the permissions are listed there, and
	// the link goes there (F-117).
	if !strings.Contains(body, "under Rules") {
		t.Errorf("the notice does not say where to go: %q", body)
	}

	var link string
	const linkQ = `SELECT COALESCE(link,'') FROM notifications WHERE user_id = ? AND title LIKE 'Followers can no longer read%' ORDER BY created_at DESC LIMIT 1`
	db.QueryRow(linkQ, admin.ID).Scan(&link)
	if want := weblink.PatchGovernanceRules("whole-shelf"); link != want {
		t.Errorf("the notice links to %q, not to the page that lists the switch (%q)", link, want)
	}
}

// openFollowerCharters puts a patch back into the pre-rollout state: the row
// grants followers the members-only shelf, which is what migration 012's
// column default did to every patch that already existed.
func openFollowerCharters(t *testing.T, db *database.DB, nodeID string) {
	t.Helper()
	if _, err := db.Exec(
		`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
		`{"events":true,"proposals":true,"charters":true,"members":true}`, nodeID,
	); err != nil {
		t.Fatalf("open follower charters: %v", err)
	}
}
