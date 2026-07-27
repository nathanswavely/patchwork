package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// max_admins is not a cap (docs/adr/051). docs/adr/049 enforced it here on
// the strength of the governance overview rendering "3 of 7 seats filled",
// and that was wrong in the instance if not the principle: no surface can
// edit the field — RulesProposalEditor is the only writer of
// governance-rules.json and the structured editor preserves max_admins
// without offering a control — while migration 041 backfilled it into
// nearly every patch. Enforcing it capped live patches at three admins with
// no recourse.
//
// How many admins a patch has is a function of how it governs. This test
// exists so the guard is not re-added without reading 051 first.
func TestPromoteToAdmin_MaxAdminsIsNotACap(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "No Cap", "no-cap", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// A cap of one, already met by the founding admin.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"max_admins":1}`, nodeID)

	for _, name := range []string{"pastcap1", "pastcap2", "pastcap3"} {
		u, _ := createTestUser(t, db, name, "member")
		memID := createTestMembership(t, db, u.ID, nodeID, "member", "active")

		r := authedRequest("PATCH", "/api/v1/nodes/no-cap/members/"+u.ID,
			map[string]interface{}{"role": "admin"}, adminToken)
		w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: max_admins must not gate promotion, got %d: %s", name, w.Code, w.Body.String())
		}
		var role string
		db.QueryRow("SELECT role FROM memberships WHERE id = ?", memID).Scan(&role)
		if role != "admin" {
			t.Errorf("%s: expected promotion to land, got %q", name, role)
		}
	}

	var admins int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&admins)
	if admins != 4 {
		t.Errorf("expected 4 admins past a max_admins of 1, got %d", admins)
	}
}

// The last-admin floor is untouched by the retraction — it is a different
// guard with a different reason, and docs/adr/051's holdover model depends
// on it holding (a patch that cannot reach zero admins is why a lapsed term
// can safely remove nobody).
func TestDemoteLastAdmin_StillRefused(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "lonefloor", "member")
	nodeID := createTestNode(t, db, admin.ID, "Lone Floor", "lone-floor", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/lone-floor/members/"+admin.ID,
		map[string]interface{}{"role": "member"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 demoting the last admin, got %d: %s", w.Code, w.Body.String())
	}
	var role string
	db.QueryRow("SELECT role FROM memberships WHERE node_id = ? AND user_id = ?", nodeID, admin.ID).Scan(&role)
	if role != "admin" {
		t.Errorf("expected the last admin to keep the role, got %q", role)
	}
}
