package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// max_admins is rendered to members as "N of M seats filled" and was never
// enforced, so "9 of 7" was reachable (docs/adr/049). The guard sits on the
// role-change path because that is the only one that promotes an existing
// member; patch creation and claim completion each mint the first admin,
// which no positive cap can exclude.

// Promotion is refused once every seat is taken.
func TestPromoteToAdmin_RefusedWhenSeatsFull(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Seats Full", "seats-full", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// One seat, already filled by the founding admin.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"max_admins":1}`, nodeID)

	hopeful, _ := createTestUser(t, db, "seathopeful", "member")
	memID := createTestMembership(t, db, hopeful.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/seats-full/members/"+hopeful.ID,
		map[string]interface{}{"role": "admin"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when seats are full, got %d: %s", w.Code, w.Body.String())
	}
	var role string
	db.QueryRow("SELECT role FROM memberships WHERE id = ?", memID).Scan(&role)
	if role != "member" {
		t.Errorf("expected the refusal to leave the role alone, got %q", role)
	}
}

// A free seat still promotes. The guard must not turn max_admins into a
// blanket refusal.
func TestPromoteToAdmin_AllowedWhenSeatFree(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Seat Free", "seat-free", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"max_admins":3}`, nodeID)

	hopeful, _ := createTestUser(t, db, "seathopeful2", "member")
	memID := createTestMembership(t, db, hopeful.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/seat-free/members/"+hopeful.ID,
		map[string]interface{}{"role": "admin"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with a seat free, got %d: %s", w.Code, w.Body.String())
	}
	var role string
	db.QueryRow("SELECT role FROM memberships WHERE id = ?", memID).Scan(&role)
	if role != "admin" {
		t.Errorf("expected promotion to land, got role %q", role)
	}
}

// No cap configured means no cap enforced — the read surfaces all gate on
// max_admins > 0, and the guard has to agree with them.
func TestPromoteToAdmin_NoCapWhenUnset(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin3", "member")
	nodeID := createTestNode(t, db, admin.ID, "No Cap", "no-cap", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// governance_config with no max_admins at all.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25}`, nodeID)

	for _, name := range []string{"uncapped1", "uncapped2", "uncapped3"} {
		u, _ := createTestUser(t, db, name, "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")

		r := authedRequest("PATCH", "/api/v1/nodes/no-cap/members/"+u.ID,
			map[string]interface{}{"role": "admin"}, adminToken)
		w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200 with no cap set, got %d: %s", name, w.Code, w.Body.String())
		}
	}
}

// A patch already over its cap is left alone: the guard refuses to add a
// seat, and never takes one away. Lowering max_admins below the sitting
// admin count must not demote anyone, and must not error on the way past.
func TestPromoteToAdmin_OverCapPatchIsNotPruned(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin4", "member")
	nodeID := createTestNode(t, db, admin.ID, "Over Cap", "over-cap", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	second, _ := createTestUser(t, db, "seatsecond", "member")
	createTestMembership(t, db, second.ID, nodeID, "admin", "active")
	third, _ := createTestUser(t, db, "seatthird", "member")
	createTestMembership(t, db, third.ID, nodeID, "admin", "active")

	// Three sitting admins, cap lowered to two after the fact.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"max_admins":2}`, nodeID)

	// Nobody is demoted by the setting alone.
	var sitting int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&sitting)
	if sitting != 3 {
		t.Fatalf("expected the three sitting admins to be untouched, got %d", sitting)
	}

	// But a fourth cannot be added.
	hopeful, _ := createTestUser(t, db, "seatfourth", "member")
	createTestMembership(t, db, hopeful.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/over-cap/members/"+hopeful.ID,
		map[string]interface{}{"role": "admin"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 over cap, got %d: %s", w.Code, w.Body.String())
	}
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&sitting)
	if sitting != 3 {
		t.Errorf("expected admin count unchanged at 3, got %d", sitting)
	}
}

// Demotion is unaffected: the last-admin floor still holds, and the seat
// guard must not fire on a role change that frees a seat.
func TestDemoteAdmin_UnaffectedBySeatGuard(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Demote Seats", "demote-seats", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	second, _ := createTestUser(t, db, "seatsecond5", "member")
	memID := createTestMembership(t, db, second.ID, nodeID, "admin", "active")

	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"max_admins":2}`, nodeID)

	r := authedRequest("PATCH", "/api/v1/nodes/demote-seats/members/"+second.ID,
		map[string]interface{}{"role": "member"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected demotion to succeed at a full cap, got %d: %s", w.Code, w.Body.String())
	}
	var role string
	db.QueryRow("SELECT role FROM memberships WHERE id = ?", memID).Scan(&role)
	if role != "member" {
		t.Errorf("expected demotion to land, got %q", role)
	}
}
