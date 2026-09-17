package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// docs/adr/117. A follower is a different relationship from a member, not a
// quieter kind of one. Three surfaces had it as the same thing, and together
// they let an admin hand a follower the admin role without that person ever
// asking to join.

// The ladder, at the one door that did not check it. attestations.go and
// succession.go both refuse a follower by name; this control checked only that
// the word was one of three.
func TestAFollowerIsNotPromotedFromTheRoleControl(t *testing.T) {
	for _, to := range []string{"member", "admin"} {
		db := setupTestDB(t)
		admin, adminTok := createTestUser(t, db, "fp-admin", "member")
		fan, _ := createTestUser(t, db, "fp-fan", "member")
		nodeID := createTestNode(t, db, admin.ID, "FP", "fp", "invite_only")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, fan.ID, nodeID, "follower", "active")

		r := authedRequest("PATCH", "/api/v1/nodes/fp/members/"+fan.ID, map[string]string{"role": to}, adminTok)
		w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
		if w.Code != http.StatusConflict {
			t.Errorf("promoting a follower to %s: expected 409, got %d: %s", to, w.Code, w.Body.String())
		}
		var role string
		db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ?", fan.ID, nodeID).Scan(&role)
		if role != "follower" {
			t.Errorf("promoting a follower to %s left them %q", to, role)
		}
	}
}

// Demotion is the other direction and stays open: ending a relationship needs
// nobody's consent the way starting one does.
func TestAMemberMayStillBeDroppedToFollower(t *testing.T) {
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, "fd-admin", "member")
	mem, _ := createTestUser(t, db, "fd-mem", "member")
	nodeID := createTestNode(t, db, admin.ID, "FD", "fd", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, mem.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/fd/members/"+mem.ID, map[string]string{"role": "follower"}, adminTok)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("demoting a member to follower: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var role string
	db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ?", mem.ID, nodeID).Scan(&role)
	if role != "follower" {
		t.Errorf("demotion did not land: role is %q", role)
	}
}

// "/members" means the membership. It used to mean every active row, so an
// insider's roster listed followers under a count that excluded them.
func TestTheMemberListingIsTheMembership(t *testing.T) {
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, "ml-admin", "member")
	mem, _ := createTestUser(t, db, "ml-mem", "member")
	fan, _ := createTestUser(t, db, "ml-fan", "member")
	nodeID := createTestNode(t, db, admin.ID, "ML", "ml", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, mem.ID, nodeID, "member", "active")
	createTestMembership(t, db, fan.ID, nodeID, "follower", "active")

	read := func(q string) ([]string, float64) {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/ml/members"+q, nil, adminTok)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
		var body struct {
			Items []struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"items"`
			MemberCount float64 `json:"member_count"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		var names []string
		for _, it := range body.Items {
			names = append(names, it.Username+"/"+it.Role)
		}
		return names, body.MemberCount
	}

	names, count := read("")
	if len(names) != 2 {
		t.Errorf("default listing should be the membership, got %v", names)
	}
	// The roster and the count are the same set. This is the disagreement
	// that made the bug visible: 2 rows under a count of 1.
	if int(count) != len(names) {
		t.Errorf("member_count %v disagrees with the listing %v", count, names)
	}

	followers, _ := read("?role=follower")
	if len(followers) != 1 || followers[0] != "ml-fan/follower" {
		t.Errorf("?role=follower should list the followers, got %v", followers)
	}
}

// A member cannot change their own role. Two gates already say so; neither had
// a test saying it was deliberate.
func TestAMemberCannotChangeTheirOwnRole(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "sr-admin", "member")
	mem, memTok := createTestUser(t, db, "sr-mem", "member")
	nodeID := createTestNode(t, db, admin.ID, "SR", "sr", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, mem.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/sr/members/"+mem.ID, map[string]string{"role": "admin"}, memTok)
	if w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r); w.Code != http.StatusForbidden {
		t.Errorf("a member promoting themselves: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	// And the endpoint that is theirs takes no role at all.
	r2 := authedRequest("PATCH", "/api/v1/users/me/memberships/"+nodeID,
		map[string]interface{}{"visible": true, "role": "admin"}, memTok)
	serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r2)

	var role string
	db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ?", mem.ID, nodeID).Scan(&role)
	if role != "member" {
		t.Errorf("a member changed their own role to %q", role)
	}
}

// is_member means the membership (docs/adr/117). Six components had written
// their own guard against it meaning otherwise.
func TestIsMemberMeansTheMembership(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "im-owner", "member")
	fan, fanTok := createTestUser(t, db, "im-fan", "member")
	nodeID := createTestNode(t, db, owner.ID, "IM", "im", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, fan.ID, nodeID, "follower", "active")

	r := authedRequest("GET", "/api/v1/nodes/im", nil, fanTok)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)

	if body["is_member"] == true {
		t.Error("a follower is reported as a member")
	}
	// The wider question still has an answer, so callers that wanted it have
	// somewhere to go.
	if body["membership_role"] != "follower" {
		t.Errorf("membership_role should still report standing, got %v", body["membership_role"])
	}
}
