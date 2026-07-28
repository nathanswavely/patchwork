package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Meritocratic ratification (docs/adr/051), phase 2. "Admins earn their role
// through sustained contribution. When a seat opens, existing admins nominate
// from active members and the community ratifies." Nominating was not a thing
// anyone could do, and ratifying decided nothing: approving a `membership`
// proposal changed no roles.

func meritocraticNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	// Zero quorum and a majority method so a single ballot resolves the vote.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"leadership_model":"meritocratic","min_voting_tenure_days":0}`,
		nodeID)
	return nodeID
}

func roleOf(t *testing.T, db *database.DB, userID, nodeID string) string {
	t.Helper()
	var role string
	db.QueryRow("SELECT COALESCE(role,'') FROM memberships WHERE user_id = ? AND node_id = ?", userID, nodeID).Scan(&role)
	return role
}

// The gate that makes ratification real rather than optional. An optional vote
// is theatre — the "rules on screen are not the rules in force" failure
// docs/adr/041 named.
func TestPromoteToAdmin_RefusedOnMeritocratic(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit One", "merit-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	hopeful, _ := createTestUser(t, db, "merithopeful", "member")
	createTestMembership(t, db, hopeful.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/merit-one/members/"+hopeful.ID,
		map[string]interface{}{"role": "admin"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if got := roleOf(t, db, hopeful.ID, nodeID); got != "member" {
		t.Errorf("expected the role untouched, got %q", got)
	}
}

// Demotion is untouched: nothing about earning a role says the community must
// vote to end it.
func TestDemoteAdmin_AllowedOnMeritocratic(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin2", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Two", "merit-two")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "meritother2", "member")
	createTestMembership(t, db, other.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/merit-two/members/"+other.ID,
		map[string]interface{}{"role": "member"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := roleOf(t, db, other.ID, nodeID); got != "member" {
		t.Errorf("expected demotion to land, got %q", got)
	}
}

func TestCreateNomination(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin3", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Three", "merit-three")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "meritnominee3", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	body := map[string]interface{}{
		"title":          "Nominate Nominee for admin",
		"proposal_type":  "membership",
		"target_user_id": nominee.ID,
	}
	r := authedRequest("POST", "/api/v1/nodes/merit-three/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var stored string
	db.QueryRow("SELECT COALESCE(target_user_id,'') FROM proposals WHERE node_id = ?", nodeID).Scan(&stored)
	if stored != nominee.ID {
		t.Errorf("expected the nominee recorded, got %q", stored)
	}
	// The nomination is a vote, not a promotion wearing a proposal's clothes.
	if got := roleOf(t, db, nominee.ID, nodeID); got != "member" {
		t.Errorf("nominating must not promote, got %q", got)
	}
}

// The three conditions are the template's own sentence read literally.
func TestCreateNomination_Conditions(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin4", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Four", "merit-four")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	follower, _ := createTestUser(t, db, "meritfollower4", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	outsider, _ := createTestUser(t, db, "meritoutsider4", "member")
	coAdmin, _ := createTestUser(t, db, "meritcoadmin4", "member")
	createTestMembership(t, db, coAdmin.ID, nodeID, "admin", "active")

	for _, tc := range []struct{ name, userID string }{
		{"follower", follower.ID},
		{"outsider", outsider.ID},
		{"already an admin", coAdmin.ID},
	} {
		body := map[string]interface{}{
			"title": "Nominate", "proposal_type": "membership", "target_user_id": tc.userID,
		}
		r := authedRequest("POST", "/api/v1/nodes/merit-four/proposals", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
		if w.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", tc.name, w.Code, w.Body.String())
		}
	}

	// A member cannot nominate — the template says existing admins do.
	member, memberToken := createTestUser(t, db, "meritmember4", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	nominee, _ := createTestUser(t, db, "meritnominee4", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/merit-four/proposals", body, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusConflict {
		t.Errorf("member nominating: expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ?", nodeID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no proposal written by a refused nomination, got %d", count)
	}
}

// Nomination is the meritocratic mechanic and nobody else's.
func TestCreateNomination_RefusedOnOtherLeadershipModels(t *testing.T) {
	for _, lm := range []string{"maintainer", "elected"} {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "nom_"+lm, "member")
		nodeID := createTestNode(t, db, admin.ID, "Node "+lm, "nom-"+lm, "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
			`{"decision_method":"majority","quorum_percent":0,"leadership_model":"`+lm+`"}`, nodeID)
		nominee, _ := createTestUser(t, db, "nominee_"+lm, "member")
		createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

		body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
		r := authedRequest("POST", "/api/v1/nodes/nom-"+lm+"/proposals", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
		if w.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", lm, w.Code, w.Body.String())
		}
	}
}

// A proposal about a person has to say so in its type, or the nomination
// conditions would be skippable by mislabelling it.
func TestCreateNomination_RequiresMembershipType(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin5", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Five", "merit-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "meritnominee5", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	body := map[string]interface{}{"title": "Sneaky", "proposal_type": "action", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/merit-five/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// The whole point: ratification promotes, on approval, with no admin left to
// veto it afterwards.
func TestRatifiedNominationPromotes(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin6", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Six", "merit-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "meritnominee6", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	body := map[string]interface{}{
		"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID,
	}
	r := authedRequest("POST", "/api/v1/nodes/merit-six/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create nomination: got %d: %s", w.Code, w.Body.String())
	}
	proposalID := decodeJSON(t, w)["id"].(string)

	// The electorate here is two — the admin and the nominee, who is a member
	// — so docs/adr/041's sole-voter early close does not apply. One approval
	// carries it on majority with zero quorum; the window then has to close
	// for the tally to be read, which GetProposal does on expiry.
	vr := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, adminToken)
	vw := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), vr)
	if vw.Code != http.StatusOK && vw.Code != http.StatusCreated {
		t.Fatalf("vote: got %d: %s", vw.Code, vw.Body.String())
	}

	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	var status, state string
	db.QueryRow("SELECT status, COALESCE(state,'') FROM proposals WHERE id = ?", proposalID).Scan(&status, &state)
	if status != "approved" {
		t.Fatalf("expected the nomination approved, got status=%q state=%q", status, state)
	}
	if got := roleOf(t, db, nominee.ID, nodeID); got != "admin" {
		t.Errorf("expected the ratified nominee promoted, got %q", got)
	}
	// Nothing is left for an admin to apply.
	if state != "in_effect" {
		t.Errorf("expected state in_effect, got %q", state)
	}
}

// A nominee who leaves between nomination and ratification is not dragged
// back into the patch they left.
func TestRatifiedNomination_SkipsDepartedNominee(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "meritadmin7", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Merit Seven", "merit-seven")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "meritnominee7", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/merit-seven/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	proposalID := decodeJSON(t, w)["id"].(string)

	db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", nominee.ID, nodeID)

	vr := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, adminToken)
	serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), vr)

	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	var role, status string
	db.QueryRow("SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?", nominee.ID, nodeID).Scan(&role, &status)
	if role == "admin" {
		t.Error("a departed nominee must not be promoted into the patch they left")
	}
	if status != "left" {
		t.Errorf("expected the departure to stand, got %q", status)
	}
}
