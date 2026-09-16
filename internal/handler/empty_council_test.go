package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The route back into a patch inactivity emptied (docs/adr/051).
//
// Inactivity may leave a patch with no admins — that is the one path that can,
// and docs/adr/051 says so deliberately. Every mechanism that makes an admin
// afterwards is started by an admin: an attestation is recorded by one, a
// nomination is raised by one, a seat is added by one. So on an empty council
// the refusals in UpdateMember stop protecting a mechanism and start sealing
// the door, and the only person who can be standing at it is an instance
// admin.

func roleSinceOf(t *testing.T, db *database.DB, userID, nodeID string) string {
	t.Helper()
	var since string
	db.QueryRow(`SELECT COALESCE(role_since,'') FROM memberships WHERE user_id = ? AND node_id = ?`,
		userID, nodeID).Scan(&since)
	return since
}

// With a council still standing, the elected gate holds for everybody
// including an instance admin — this is docs/adr/100's rule and it is not
// being weakened.
func TestUpdateMember_ElectedGateHoldsWhileAdminsRemain(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "gateowner", "member")
	nodeID := electedNode(t, db, owner.ID, "Gate Hall", "gate-hall", 0, 12)
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	member, _ := createTestUser(t, db, "gatemember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	_, siteToken := createTestUser(t, db, "gatesiteadmin", "admin")

	code, body := promoteVia(t, db, "gate-hall", member.ID, siteToken)
	if code != http.StatusConflict {
		t.Fatalf("expected the elected gate to hold, got %d: %s", code, body)
	}
	if !strings.Contains(body, "elects its council") {
		t.Errorf("expected the refusal to say which mechanism runs, got %s", body)
	}
}

// With no admins left, an instance admin can put one back — and on an elected
// patch they land in a chair the record can name, keeping that chair's term.
func TestUpdateMember_InstanceAdminRestoresAnEmptyElectedCouncil(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "emptyowner", "member")
	nodeID := electedNode(t, db, owner.ID, "Empty Hall", "empty-hall", 0, 12)
	// The council went quiet and was vacated: the person is a member again
	// and the chair is empty but still there.
	createTestMembership(t, db, owner.ID, nodeID, "member", "active")
	seatID := makeVacantSeat(t, db, nodeID, "2027-09-01")

	member, _ := createTestUser(t, db, "emptymember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	_, siteToken := createTestUser(t, db, "emptysiteadmin", "admin")

	code, body := promoteVia(t, db, "empty-hall", member.ID, siteToken)
	if code != http.StatusOK {
		t.Fatalf("expected an empty council to be repairable, got %d: %s", code, body)
	}

	holder, termEnds := seatRow(t, db, seatID)
	if holder != member.ID {
		t.Errorf("expected the restored admin seated in the empty chair, got %q", holder)
	}
	// The clock belongs to the seat (docs/adr/051): a repair is the last act
	// that should hand out a fresh mandate.
	if termEnds != "2027-09-01" {
		t.Errorf("expected the chair to keep its own term end, got %q", termEnds)
	}
	// And their own inactivity clock starts now (migration 072), so the sweep
	// does not judge them absent for the years before they held anything.
	if roleSinceOf(t, db, member.ID, nodeID) == "" {
		t.Error("a promotion must record when the role was taken")
	}

	// The door closes behind them: with an admin in place the gate is back.
	other, _ := createTestUser(t, db, "emptyother", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")
	code, body = promoteVia(t, db, "empty-hall", other.ID, siteToken)
	if code != http.StatusConflict {
		t.Errorf("expected the elected gate to close again, got %d: %s", code, body)
	}
}

// The same door on a patch that decides its leadership elsewhere
// (docs/adr/052): recording an attestation is an admin's act too, so an
// emptied one cannot record its way back either.
func TestUpdateMember_InstanceAdminRestoresAnEmptyElsewhereCouncil(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "elsewhereowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Elsewhere Hall", "elsewhere-hall", "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","leadership_model":"elected","leadership_venue":"elsewhere"}`, nodeID)
	createTestMembership(t, db, owner.ID, nodeID, "member", "active")

	member, _ := createTestUser(t, db, "elsewheremember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	_, siteToken := createTestUser(t, db, "elsewheresiteadmin", "admin")

	code, body := promoteVia(t, db, "elsewhere-hall", member.ID, siteToken)
	if code != http.StatusOK {
		t.Fatalf("expected an empty council to be repairable, got %d: %s", code, body)
	}
	var role string
	db.QueryRow(`SELECT role FROM memberships WHERE user_id = ? AND node_id = ?`, member.ID, nodeID).Scan(&role)
	if role != "admin" {
		t.Errorf("expected the patch to have an admin again, got %q", role)
	}
	// The record says this was a repair rather than a promotion the patch's
	// own rules allowed.
	var entries int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log
	             WHERE action = 'membership.role_change' AND metadata LIKE '%council_empty%'`).Scan(&entries)
	if entries != 1 {
		t.Errorf("expected the repair recorded with its reason, got %d entries", entries)
	}
}

// Migration 072 in the ordinary case: the dropdown on a maintainer patch
// records when the role was taken, so the inactivity sweep measures that
// person's absence from their promotion and not from their joining.
func TestUpdateMember_RoleChangeRecordsWhenTheRoleWasTaken(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "sinceowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Since Hall", "since-hall", "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","leadership_model":"maintainer"}`, nodeID)
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	member, _ := createTestUser(t, db, "sincemember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE user_id = ? AND node_id = ?`,
		member.ID, nodeID)

	if code, body := promoteVia(t, db, "since-hall", member.ID, ownerToken); code != http.StatusOK {
		t.Fatalf("expected a maintainer's dropdown to work, got %d: %s", code, body)
	}
	since := roleSinceOf(t, db, member.ID, nodeID)
	if since == "" || since < "2021" {
		t.Errorf("expected role_since stamped at the promotion, got %q", since)
	}
}
