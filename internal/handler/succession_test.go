package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Maintainer succession (docs/adr/051). "One person maintains this patch...
// and can designate a successor" was copy with nothing behind it. These tests
// hold the two halves that make it true: naming is restricted to the model
// that uses it, and the name actually fires — a designation nobody can ever
// succeed to would be another of the fields docs/adr/049 catalogued.

func maintainerNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"admin","quorum_percent":0,"leadership_model":"maintainer","min_voting_tenure_days":0}`,
		nodeID)
	return nodeID
}

func successorOf(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	var sid string
	db.QueryRow("SELECT COALESCE(designated_successor_id,'') FROM nodes WHERE id = ?", nodeID).Scan(&sid)
	return sid
}

func TestSetSuccessor(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint1", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint One", "maint-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	heir, _ := createTestUser(t, db, "heir1", "member")
	createTestMembership(t, db, heir.ID, nodeID, "member", "active")

	r := authedRequest("PUT", "/api/v1/nodes/maint-one/successor",
		map[string]interface{}{"user_id": heir.ID}, adminToken)
	w := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/successor", handler.SetSuccessor(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := successorOf(t, db, nodeID); got != heir.ID {
		t.Errorf("expected successor %s, got %q", heir.ID, got)
	}
}

// Designation is the maintainer model's mechanic and nobody else's. Storing it
// on an elected patch would be writing a field nothing will act on, which is
// the failure the whole run of ADRs keeps finding.
func TestSetSuccessor_RefusedOnOtherLeadershipModels(t *testing.T) {
	for _, lm := range []string{"elected", "meritocratic"} {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "maint_"+lm, "member")
		nodeID := createTestNode(t, db, admin.ID, "Node "+lm, "node-"+lm, "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
			`{"decision_method":"majority","quorum_percent":25,"leadership_model":"`+lm+`"}`, nodeID)

		heir, _ := createTestUser(t, db, "heir_"+lm, "member")
		createTestMembership(t, db, heir.ID, nodeID, "member", "active")

		r := authedRequest("PUT", "/api/v1/nodes/node-"+lm+"/successor",
			map[string]interface{}{"user_id": heir.ID}, adminToken)
		w := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/successor", handler.SetSuccessor(db), r)

		if w.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", lm, w.Code, w.Body.String())
		}
		if got := successorOf(t, db, nodeID); got != "" {
			t.Errorf("%s: expected no successor stored, got %q", lm, got)
		}
	}
}

// A follower is never eligible — admin is a rung on the member ladder, not a
// role handed sideways to an observer. Nor is someone outside the patch.
func TestSetSuccessor_MustBeAnActiveMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint2", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Two", "maint-two")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	follower, _ := createTestUser(t, db, "follower2", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	outsider, _ := createTestUser(t, db, "outsider2", "member")

	for _, tc := range []struct {
		name   string
		userID string
	}{
		{"follower", follower.ID},
		{"outsider", outsider.ID},
		{"self", admin.ID},
	} {
		r := authedRequest("PUT", "/api/v1/nodes/maint-two/successor",
			map[string]interface{}{"user_id": tc.userID}, adminToken)
		w := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/successor", handler.SetSuccessor(db), r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", tc.name, w.Code, w.Body.String())
		}
		if got := successorOf(t, db, nodeID); got != "" {
			t.Errorf("%s: expected no successor stored, got %q", tc.name, got)
		}
	}
}

// The whole point. docs/adr/012 says leaving is a member right, and the
// last-admin floor has always made that untrue for the one person nobody can
// replace. Naming a successor is how a maintainer earns their own exit.
func TestLeave_SoleAdminSucceedsToDesignatedHeir(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint3", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Three", "maint-three")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	heir, _ := createTestUser(t, db, "heir3", "member")
	heirMemID := createTestMembership(t, db, heir.ID, nodeID, "member", "active")
	db.Exec("UPDATE nodes SET designated_successor_id = ? WHERE id = ?", heir.ID, nodeID)

	r := authedRequest("POST", "/api/v1/nodes/maint-three/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected the sole admin to be able to leave, got %d: %s", w.Code, w.Body.String())
	}

	var heirRole string
	db.QueryRow("SELECT role FROM memberships WHERE id = ?", heirMemID).Scan(&heirRole)
	if heirRole != "admin" {
		t.Errorf("expected the successor promoted to admin, got %q", heirRole)
	}
	var departedStatus string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", admin.ID, nodeID).Scan(&departedStatus)
	if departedStatus != "left" {
		t.Errorf("expected the departing admin to have left, got %q", departedStatus)
	}
	// The designation is spent — re-naming belongs to the new maintainer.
	if got := successorOf(t, db, nodeID); got != "" {
		t.Errorf("expected the designation cleared after it fired, got %q", got)
	}
	// The patch is never left without an administrator.
	var admins int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&admins)
	if admins != 1 {
		t.Errorf("expected exactly one admin after succession, got %d", admins)
	}
}

// No successor named, so the floor holds. The alternative is a patch nobody
// can administer.
func TestLeave_SoleAdminStillRefusedWithoutSuccessor(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint4", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Four", "maint-four")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "other4", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/nodes/maint-four/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 with no successor named, got %d: %s", w.Code, w.Body.String())
	}
	var role, status string
	db.QueryRow("SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?", admin.ID, nodeID).Scan(&role, &status)
	if role != "admin" || status != "active" {
		t.Errorf("expected the admin to still hold the patch, got role=%q status=%q", role, status)
	}
}

// A designation is voided by the successor leaving, without anything touching
// the row. The membership is re-checked at succession time for exactly this.
func TestLeave_DesignationVoidWhenHeirHasLeft(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint5", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Five", "maint-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	heir, _ := createTestUser(t, db, "heir5", "member")
	createTestMembership(t, db, heir.ID, nodeID, "member", "active")
	db.Exec("UPDATE nodes SET designated_successor_id = ? WHERE id = ?", heir.ID, nodeID)
	// The heir walks first, leaving a stale name on the row.
	db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", heir.ID, nodeID)

	r := authedRequest("POST", "/api/v1/nodes/maint-five/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when the named successor has left, got %d: %s", w.Code, w.Body.String())
	}
	var heirRole string
	db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ?", heir.ID, nodeID).Scan(&heirRole)
	if heirRole == "admin" {
		t.Error("a departed member must not be promoted into the patch they left")
	}
}

// Succession is the sole-admin path only. A patch with other admins loses
// nothing when one leaves, so the designation must stay put for the day it is
// actually needed.
func TestLeave_DesignationSurvivesWhenOtherAdminsRemain(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint6", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Six", "maint-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	coAdmin, _ := createTestUser(t, db, "coadmin6", "member")
	createTestMembership(t, db, coAdmin.ID, nodeID, "admin", "active")

	heir, _ := createTestUser(t, db, "heir6", "member")
	heirMemID := createTestMembership(t, db, heir.ID, nodeID, "member", "active")
	db.Exec("UPDATE nodes SET designated_successor_id = ? WHERE id = ?", heir.ID, nodeID)

	r := authedRequest("POST", "/api/v1/nodes/maint-six/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var heirRole string
	db.QueryRow("SELECT role FROM memberships WHERE id = ?", heirMemID).Scan(&heirRole)
	if heirRole != "member" {
		t.Errorf("the heir must not be promoted while other admins remain, got %q", heirRole)
	}
	if got := successorOf(t, db, nodeID); got != heir.ID {
		t.Errorf("expected the designation to survive, got %q", got)
	}
}

func TestClearSuccessor(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "maint7", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Seven", "maint-seven")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	heir, _ := createTestUser(t, db, "heir7", "member")
	createTestMembership(t, db, heir.ID, nodeID, "member", "active")
	db.Exec("UPDATE nodes SET designated_successor_id = ? WHERE id = ?", heir.ID, nodeID)

	r := authedRequest("DELETE", "/api/v1/nodes/maint-seven/successor", nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/successor", handler.ClearSuccessor(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := successorOf(t, db, nodeID); got != "" {
		t.Errorf("expected the successor cleared, got %q", got)
	}
}

// A member cannot name the patch's successor, and neither can a follower.
func TestSetSuccessor_NonAdminRefused(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "maint8", "member")
	nodeID := maintainerNode(t, db, admin.ID, "Maint Eight", "maint-eight")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	member, memberToken := createTestUser(t, db, "member8", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	heir, _ := createTestUser(t, db, "heir8", "member")
	createTestMembership(t, db, heir.ID, nodeID, "member", "active")

	r := authedRequest("PUT", "/api/v1/nodes/maint-eight/successor",
		map[string]interface{}{"user_id": heir.ID}, memberToken)
	w := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/successor", handler.SetSuccessor(db), r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if got := successorOf(t, db, nodeID); got != "" {
		t.Errorf("expected no successor stored, got %q", got)
	}
}
