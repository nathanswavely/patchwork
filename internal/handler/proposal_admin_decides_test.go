package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Who decides, on each kind of patch (docs/adr/092).
//
// The scenario matrix. Every governance template times every role, run
// through create → vote → resolve, asserting where the decision lands. It
// exists because the answer drifted without anybody changing it on purpose:
// on the Minimal template ("the maintainer makes all decisions") a member's
// proposal was born voting and resolved by the members' majority, so the
// maintainer could be outvoted on the one patch that says they cannot be.
// Nothing asserted who held power on a patch, so nothing noticed.

// The four shipped templates' rules, as governance_config caches them. Kept
// here rather than read from the template files so a template edit that
// moves power has to come here and say so.
var templateRules = map[string]string{
	"minimal":       `{"decision_method":"admin","quorum_percent":0,"default_vote_duration_hours":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0,"leadership_model":"maintainer","max_admins":1}`,
	"casual":        `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0,"leadership_model":"maintainer","max_admins":3}`,
	"collaborative": `{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168,"amendment_threshold":"supermajority","amendment_auto_apply":true,"min_voting_tenure_days":7,"leadership_model":"meritocratic","max_admins":5}`,
	"formal":        `{"decision_method":"consensus","quorum_percent":50,"default_vote_duration_hours":336,"amendment_threshold":"consensus","amendment_auto_apply":false,"min_voting_tenure_days":30,"leadership_model":"elected","max_admins":7}`,
}

type adminDecidesRig struct {
	db          *database.DB
	nodeID      string
	slug        string
	adminID     string
	adminTok    string
	memberID    string
	memberTok   string
	member2ID   string
	member2Tok  string
	instanceTok string
}

// newTemplateRig builds a patch under one template's rules with one admin,
// two members, and an instance admin who holds no role there. Members are
// stamped as having joined long ago, so tenure never silently decides a
// scenario that is about something else.
func newTemplateRig(t *testing.T, template, slug string) adminDecidesRig {
	t.Helper()
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, slug+"_admin", "member")
	member, memberTok := createTestUser(t, db, slug+"_member", "member")
	member2, member2Tok := createTestUser(t, db, slug+"_member2", "member")
	_, instanceTok := createTestUser(t, db, slug+"_instance", "admin")
	nodeID := createTestNode(t, db, admin.ID, "Rig "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, member2.ID, nodeID, "member", "active")
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, nodeID)
	setupGovernanceForNode(t, nodeID)
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, templateRules[template], nodeID)
	return adminDecidesRig{
		db: db, nodeID: nodeID, slug: slug,
		adminID: admin.ID, adminTok: adminTok,
		memberID: member.ID, memberTok: memberTok,
		member2ID: member2.ID, member2Tok: member2Tok,
		instanceTok: instanceTok,
	}
}

func voteOn(t *testing.T, db *database.DB, id, token, value string) int {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/vote", map[string]string{"value": value}, token)
	return serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r).Code
}

func decideOn(t *testing.T, db *database.DB, id, token, decision string) (int, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/decide", map[string]string{"decision": decision}, token)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/decide", handler.DecideProposal(db), r)
	return w.Code, w.Body.String()
}

func openVoteOn(t *testing.T, db *database.DB, id, token string, hours int) (int, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/open-vote", map[string]int{"duration_hours": hours}, token)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/open-vote", handler.OpenAdvisoryVote(db), r)
	return w.Code, w.Body.String()
}

func applyOn(t *testing.T, db *database.DB, id, token string) (int, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/apply", nil, token)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/apply", handler.ApplyProposal(db), r)
	return w.Code, w.Body.String()
}

// closeWindow backdates the voting window so the next read resolves it.
func closeWindow(t *testing.T, db *database.DB, id string) {
	t.Helper()
	db.Exec(`UPDATE proposals SET voting_ends_at = '2000-01-01T00:00:00.000Z' WHERE id = ?`, id)
}

func TestWhoDecides_Matrix(t *testing.T) {
	type scenario struct {
		template  string
		role      string // who proposes
		bornState string // state at creation
		bornVote  bool   // is there a ballot at creation
		decider   string // "tally" or "admin"
	}
	scenarios := []scenario{
		// The maintainer's patch: admins apply, members ask, nobody tallies.
		{"minimal", "admin", "in_effect", false, "admin"},
		{"minimal", "member", "awaiting_admin", false, "admin"},
		// Every voting template: everybody votes, admins included, and the
		// tally decides (docs/adr/041).
		{"casual", "admin", "voting", true, "tally"},
		{"casual", "member", "voting", true, "tally"},
		{"collaborative", "admin", "voting", true, "tally"},
		{"collaborative", "member", "voting", true, "tally"},
		{"formal", "admin", "voting", true, "tally"},
		{"formal", "member", "voting", true, "tally"},
	}

	for _, sc := range scenarios {
		t.Run(sc.template+"/"+sc.role, func(t *testing.T) {
			rig := newTemplateRig(t, sc.template, "m-"+sc.template+"-"+sc.role)
			token := rig.adminTok
			if sc.role == "member" {
				token = rig.memberTok
			}
			code, p := createProposal(t, rig.db, rig.slug, token, map[string]interface{}{
				"title": "Paint the door", "proposal_type": "action",
			})
			if code != http.StatusCreated {
				t.Fatalf("create: %d %v", code, p)
			}
			id := p["id"].(string)
			if got := p["state"]; got != sc.bornState {
				t.Fatalf("born state = %v, want %s", got, sc.bornState)
			}
			hasWindow := p["voting_ends_at"] != nil
			if hasWindow != sc.bornVote {
				t.Fatalf("voting_ends_at present = %v, want %v", hasWindow, sc.bornVote)
			}

			// A member's ballot: accepted only where a vote is open.
			voteCode := voteOn(t, rig.db, id, rig.member2Tok, "approve")
			if sc.bornVote && voteCode != http.StatusOK {
				t.Fatalf("member vote on an open ballot: %d", voteCode)
			}
			if !sc.bornVote && voteCode == http.StatusOK {
				t.Fatalf("member vote accepted where no ballot is open (state %s)", sc.bornState)
			}

			switch sc.decider {
			case "tally":
				// The admin cannot apply an open vote (docs/adr/041), and the
				// maintainer's verbs do not exist here.
				if code, _ := applyOn(t, rig.db, id, rig.adminTok); code != http.StatusConflict {
					t.Errorf("apply mid-vote on a voting patch = %d, want 409", code)
				}
				if code, _ := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusConflict {
					t.Errorf("decide on a voting patch = %d, want 409", code)
				}
				// Everyone approves; the window closes; the tally carries it.
				voteOn(t, rig.db, id, rig.memberTok, "approve")
				voteOn(t, rig.db, id, rig.adminTok, "approve")
				closeWindow(t, rig.db, id)
				got := getProposal(t, rig.db, id, rig.adminTok)
				if got["status"] != "approved" {
					t.Errorf("after the window, status = %v, want approved by tally", got["status"])
				}
			case "admin":
				if sc.bornState == "in_effect" {
					got := getProposal(t, rig.db, id, rig.adminTok)
					if got["status"] != "approved" || got["can_vote"] != false {
						t.Errorf("a direct change: status=%v can_vote=%v, want approved/false", got["status"], got["can_vote"])
					}
					return
				}
				// Waiting on the maintainer: no clock can end it, and the
				// admin's decision does.
				closeWindow(t, rig.db, id) // a no-op: there is no window
				got := getProposal(t, rig.db, id, rig.adminTok)
				if got["status"] != "open" || got["state"] != "awaiting_admin" {
					t.Fatalf("waiting proposal moved on its own: %v/%v", got["status"], got["state"])
				}
				if got["can_decide"] != true {
					t.Errorf("admin can_decide = %v, want true", got["can_decide"])
				}
				if code, body := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusOK {
					t.Fatalf("admin approve: %d %s", code, body)
				}
				got = getProposal(t, rig.db, id, rig.adminTok)
				if got["status"] != "approved" || got["state"] != "in_effect" {
					t.Errorf("after approve: %v/%v, want approved/in_effect", got["status"], got["state"])
				}
			}
		})
	}
}

// A member's proposal on an admin-decides patch is decided by the admin, and
// the members' majority against the admin is advice, not a verdict.
func TestAdminDecides_MaintainerCannotBeOutvoted(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "outvote")
	_, p := createProposal(t, rig.db, rig.slug, rig.memberTok, map[string]interface{}{
		"title": "Members want this", "proposal_type": "action",
	})
	id := p["id"].(string)

	// The admin asks the members first.
	if code, body := openVoteOn(t, rig.db, id, rig.adminTok, 48); code != http.StatusOK {
		t.Fatalf("open advisory vote: %d %s", code, body)
	}
	got := getProposal(t, rig.db, id, rig.memberTok)
	if got["state"] != "voting" || got["advisory"] != true || got["can_vote"] != true {
		t.Fatalf("after open-vote: state=%v advisory=%v can_vote=%v", got["state"], got["advisory"], got["can_vote"])
	}
	if got["duration_hours"].(float64) != 48 {
		t.Errorf("duration_hours = %v, want 48", got["duration_hours"])
	}

	// Two members for, the admin against.
	if c := voteOn(t, rig.db, id, rig.memberTok, "approve"); c != http.StatusOK {
		t.Fatalf("member vote: %d", c)
	}
	if c := voteOn(t, rig.db, id, rig.member2Tok, "approve"); c != http.StatusOK {
		t.Fatalf("member2 vote: %d", c)
	}
	if c := voteOn(t, rig.db, id, rig.adminTok, "reject"); c != http.StatusOK {
		t.Fatalf("admin vote: %d", c)
	}

	// The window closes. A majority patch would carry this 2–1; here the
	// tally is handed back to the admin.
	closeWindow(t, rig.db, id)
	got = getProposal(t, rig.db, id, rig.memberTok)
	if got["status"] != "open" || got["state"] != "awaiting_admin" {
		t.Fatalf("advisory window closed: %v/%v, want open/awaiting_admin", got["status"], got["state"])
	}
	if got["approve_count"].(float64) != 2 || got["reject_count"].(float64) != 1 {
		t.Errorf("tally kept as advice: %v/%v", got["approve_count"], got["reject_count"])
	}
	if got["can_vote"] != false {
		t.Errorf("after the advisory window, can_vote = %v, want false", got["can_vote"])
	}
	if c := voteOn(t, rig.db, id, rig.memberTok, "approve"); c != http.StatusConflict {
		t.Errorf("vote after the window = %d, want 409", c)
	}

	// The members have been asked; they cannot be asked again.
	if code, _ := openVoteOn(t, rig.db, id, rig.adminTok, 24); code != http.StatusConflict {
		t.Errorf("second open-vote = %d, want 409", code)
	}

	// The admin declines, against the advice, and the record names them.
	if code, body := decideOn(t, rig.db, id, rig.adminTok, "decline"); code != http.StatusOK {
		t.Fatalf("decline: %d %s", code, body)
	}
	got = getProposal(t, rig.db, id, rig.memberTok)
	if got["status"] != "rejected" || got["state"] != "rejected" {
		t.Errorf("after decline: %v/%v", got["status"], got["state"])
	}
	if got["declined_by"] != "outvote_admin" {
		t.Errorf("declined_by = %v, want the admin's name", got["declined_by"])
	}
	if got["can_decide"] != false {
		t.Errorf("a decided proposal still offers a decision: can_decide=%v", got["can_decide"])
	}

	// The governance record calls it a decision, not a failed vote.
	r := authedRequest("GET", "/api/v1/nodes/"+rig.slug+"/governance/record", nil, rig.memberTok)
	w := serveMux(t, rig.db, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(rig.db), r)
	items := decodeJSON(t, w)["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("record entries = %d, want 1", len(items))
	}
	e := items[0].(map[string]interface{})
	if e["kind"] != "direct" || e["outcome"] != "declined" || e["actor"] != "outvote_admin" {
		t.Errorf("record entry = %v, want direct/declined by the admin", e)
	}
}

// The admin may decide while the advisory vote is still running.
func TestAdminDecides_DecisionMidVote(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "midvote")
	_, p := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "Ask first", "proposal_type": "action", "put_to_vote": true,
	})
	id := p["id"].(string)
	if p["state"] != "voting" || p["voting_ends_at"] == nil {
		t.Fatalf("put_to_vote: state=%v window=%v, want an open advisory vote", p["state"], p["voting_ends_at"])
	}
	voteOn(t, rig.db, id, rig.memberTok, "reject")

	if code, body := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusOK {
		t.Fatalf("approve mid-vote: %d %s", code, body)
	}
	got := getProposal(t, rig.db, id, rig.adminTok)
	if got["state"] != "in_effect" {
		t.Errorf("state = %v, want in_effect", got["state"])
	}
	// The ballot cast stays on the record as advice.
	if got["reject_count"].(float64) != 1 {
		t.Errorf("advice lost: reject_count=%v", got["reject_count"])
	}
	if c := voteOn(t, rig.db, id, rig.member2Tok, "approve"); c == http.StatusOK {
		t.Errorf("vote accepted on a decided proposal")
	}
}

// A sole eligible voter on an advisory vote settles nothing early
// (docs/adr/041's early close is for votes that decide).
func TestAdminDecides_NoSoleVoterEarlyClose(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "sole")
	// Shrink the electorate to the admin alone.
	rig.db.Exec(`UPDATE memberships SET status = 'left' WHERE node_id = ? AND user_id IN (?, ?)`, rig.nodeID, rig.memberID, rig.member2ID)
	_, p := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "Just me", "proposal_type": "action", "put_to_vote": true,
	})
	id := p["id"].(string)
	if c := voteOn(t, rig.db, id, rig.adminTok, "approve"); c != http.StatusOK {
		t.Fatalf("vote: %d", c)
	}
	got := getProposal(t, rig.db, id, rig.adminTok)
	if got["status"] != "open" || got["state"] != "voting" {
		t.Errorf("a sole advisory ballot closed the vote: %v/%v", got["status"], got["state"])
	}
}

// The maintainer's verbs belong to the patch's admins and to no one else.
func TestAdminDecides_WhoMayDecide(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "whomay")
	_, p := createProposal(t, rig.db, rig.slug, rig.memberTok, map[string]interface{}{
		"title": "Please", "proposal_type": "action",
	})
	id := p["id"].(string)

	if code, _ := decideOn(t, rig.db, id, rig.memberTok, "approve"); code != http.StatusForbidden {
		t.Errorf("member decide = %d, want 403", code)
	}
	if code, _ := decideOn(t, rig.db, id, rig.instanceTok, "approve"); code != http.StatusForbidden {
		t.Errorf("instance admin decide = %d, want 403", code)
	}
	if code, _ := openVoteOn(t, rig.db, id, rig.memberTok, 24); code != http.StatusForbidden {
		t.Errorf("member open-vote = %d, want 403", code)
	}
	if code, _ := applyOn(t, rig.db, id, rig.instanceTok); code != http.StatusForbidden {
		t.Errorf("instance admin apply = %d, want 403", code)
	}
	got := getProposal(t, rig.db, id, rig.memberTok)
	if got["can_decide"] != false {
		t.Errorf("member can_decide = %v", got["can_decide"])
	}
	got = getProposal(t, rig.db, id, rig.instanceTok)
	if got["can_decide"] != false {
		t.Errorf("instance admin can_decide = %v", got["can_decide"])
	}

	// Apply is the same decision by its older name, and the patch admin
	// holds it.
	if code, body := applyOn(t, rig.db, id, rig.adminTok); code != http.StatusOK {
		t.Errorf("patch admin apply on a waiting proposal: %d %s", code, body)
	}
}

// The terms are frozen at creation (docs/adr/047): a patch switching to
// majority does not turn the maintainer's open question into a members'
// verdict, and one switching to admin-decides does not hand an open vote to
// its admin.
func TestAdminDecides_FrozenTermsGovernTheDecision(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "frozen")
	_, p := createProposal(t, rig.db, rig.slug, rig.memberTok, map[string]interface{}{
		"title": "Before the switch", "proposal_type": "action",
	})
	id := p["id"].(string)
	rig.db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, templateRules["casual"], rig.nodeID)
	if code, _ := decideOn(t, rig.db, id, rig.adminTok, "decline"); code != http.StatusOK {
		t.Errorf("after the patch moved to majority, the waiting proposal is still the maintainer's: %d", code)
	}

	rig2 := newTemplateRig(t, "casual", "frozen2")
	_, p2 := createProposal(t, rig2.db, rig2.slug, rig2.memberTok, map[string]interface{}{
		"title": "Born voting", "proposal_type": "action",
	})
	id2 := p2["id"].(string)
	rig2.db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, templateRules["minimal"], rig2.nodeID)
	if code, _ := decideOn(t, rig2.db, id2, rig2.adminTok, "approve"); code != http.StatusConflict {
		t.Errorf("a vote born under majority is not the admin's to decide after a switch: %d", code)
	}
	got := getProposal(t, rig2.db, id2, rig2.adminTok)
	if got["advisory"] != false {
		t.Errorf("a majority vote reported as advisory")
	}
}

// The governance hub's "needs your vote" never points at a proposal with no
// ballot (docs/adr/044).
func TestAdminDecides_NeedsVoteSkipsTheWaiting(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "needsvote")
	needsVote := func(tok string) float64 {
		r := authedRequest("GET", "/api/v1/nodes/"+rig.slug+"/governance/overview", nil, tok)
		w := serveMux(t, rig.db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(rig.db), r)
		n, _ := decodeJSON(t, w)["needs_vote"].(float64)
		return n
	}
	_, p := createProposal(t, rig.db, rig.slug, rig.memberTok, map[string]interface{}{
		"title": "Waiting", "proposal_type": "action",
	})
	if got := needsVote(rig.member2Tok); got != 0 {
		t.Errorf("a proposal waiting on the maintainer needs nobody's vote, got %v", got)
	}
	openVoteOn(t, rig.db, p["id"].(string), rig.adminTok, 24)
	if got := needsVote(rig.member2Tok); got != 1 {
		t.Errorf("an open advisory vote is a vote to cast, got %v", got)
	}
}

// A direct change is born without a window; only a proposal that was ever
// open carries one.
func TestAdminDecides_DirectChangeHasNoWindow(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "nowindow")
	_, p := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "Done", "proposal_type": "action",
	})
	if p["voting_ends_at"] != nil {
		t.Errorf("a direct change carries a voting window: %v", p["voting_ends_at"])
	}
	var declined *string
	rig.db.QueryRow(`SELECT declined_by FROM proposals WHERE id = ?`, p["id"]).Scan(&declined)
	if declined != nil {
		t.Errorf("declined_by set on an applied change")
	}
	_ = auth.NewUUIDv7
}
