package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Subject recusal (docs/adr/051): a patch can bar the person a proposal is
// *about* from voting on it. The hard part is not the gate — it is ADR 044's
// rule that the gate, the quorum denominator, and the nudge all name one set.
// Barring someone from casting while still dividing quorum by them would make
// a nomination unpassable.

func recusingNode(t *testing.T, db *database.DB, ownerID, name, slug string, quorum int) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":`+itoa(quorum)+`,"default_vote_duration_hours":72,"leadership_model":"meritocratic","min_voting_tenure_days":0,"subject_recusal":true}`,
		nodeID)
	return nodeID
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func nominate(t *testing.T, db *database.DB, slug, token, nomineeID string) string {
	t.Helper()
	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nomineeID}
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/proposals", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("nominate: got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)["id"].(string)
}

// The gate: the nominee cannot vote on their own nomination, and is told why
// in words that are true — they are not "not a member".
func TestRecusal_SubjectCannotVoteOnOwnNomination(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin1", "member")
	nodeID := recusingNode(t, db, admin.ID, "Rec One", "rec-one", 0)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, nomineeToken := createTestUser(t, db, "recnominee1", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	proposalID := nominate(t, db, "rec-one", adminToken, nominee.ID)

	vr := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, nomineeToken)
	vw := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), vr)

	if vw.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for the subject, got %d: %s", vw.Code, vw.Body.String())
	}
	if !strings.Contains(vw.Body.String(), "about themselves") {
		t.Errorf("expected the refusal to name the reason, got %s", vw.Body.String())
	}
	// And no ballot was written.
	var votes int
	db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ?", proposalID).Scan(&votes)
	if votes != 0 {
		t.Errorf("expected no ballot recorded, got %d", votes)
	}
}

// Everyone else votes normally, and the recused subject is out of the
// denominator too — with quorum at 100% a single ballot from the only other
// eligible voter has to carry it. If the subject were still counted, quorum
// would be 1 of 2 and the nomination could never pass.
func TestRecusal_SubjectLeavesTheQuorumDenominator(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin2", "member")
	nodeID := recusingNode(t, db, admin.ID, "Rec Two", "rec-two", 100)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "recnominee2", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	proposalID := nominate(t, db, "rec-two", adminToken, nominee.ID)

	if code := voteVia(t, db, proposalID, adminToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("admin vote: got %d", code)
	}
	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	var status string
	db.QueryRow("SELECT status FROM proposals WHERE id = ?", proposalID).Scan(&status)
	if status != "approved" {
		t.Fatalf("expected quorum met with the subject recused, got %q", status)
	}
	if got := roleOf(t, db, nominee.ID, nodeID); got != "admin" {
		t.Errorf("expected the ratified nominee promoted, got %q", got)
	}
}

// The subject is in the electorate for every other vote — recusal is about one
// proposal, not about the person.
func TestRecusal_SubjectStillVotesOnOtherProposals(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin3", "member")
	nodeID := recusingNode(t, db, admin.ID, "Rec Three", "rec-three", 0)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, nomineeToken := createTestUser(t, db, "recnominee3", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	nominate(t, db, "rec-three", adminToken, nominee.ID)

	// An ordinary proposal, about nobody.
	other := authedRequest("POST", "/api/v1/nodes/rec-three/proposals",
		map[string]interface{}{"title": "Buy a kiln", "proposal_type": "action"}, adminToken)
	ow := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), other)
	otherID := decodeJSON(t, ow)["id"].(string)

	if code := voteVia(t, db, otherID, nomineeToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Errorf("the recused subject must still vote on other proposals, got %d", code)
	}
}

// Recusal must not silence the whole electorate: with nobody left eligible and
// a quorum above zero, quorumMet could never be true and the proposal would
// sit open past its window forever.
func TestRecusal_DoesNotEmptyTheElectorate(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin4", "member")
	nodeID := recusingNode(t, db, admin.ID, "Rec Four", "rec-four", 50)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, nomineeToken := createTestUser(t, db, "recnominee4", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	proposalID := nominate(t, db, "rec-four", adminToken, nominee.ID)

	// The only other eligible voter leaves, so recusing the nominee would
	// leave nobody at all. Recusal stands down rather than deadlocking.
	db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", admin.ID, nodeID)

	vr := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, nomineeToken)
	vw := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), vr)

	if vw.Code != http.StatusOK && vw.Code != http.StatusCreated {
		t.Fatalf("expected the last voter to be able to vote, got %d: %s", vw.Code, vw.Body.String())
	}
}

// Off by default, so existing patches keep the behaviour they had.
func TestRecusal_OffByDefault(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin5", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Rec Five", "rec-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, nomineeToken := createTestUser(t, db, "recnominee5", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	proposalID := nominate(t, db, "rec-five", adminToken, nominee.ID)

	if code := voteVia(t, db, proposalID, nomineeToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Errorf("without recusal the subject votes like anyone else, got %d", code)
	}
}

// Recusal is a term of the contest, so it freezes when voting opens
// (docs/adr/047). Switching it on mid-vote does not retroactively bar someone
// who was eligible when the vote opened.
func TestRecusal_FrozenWithTheRestOfTheTerms(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "recadmin6", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Rec Six", "rec-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, nomineeToken := createTestUser(t, db, "recnominee6", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	// Opened with recusal off.
	proposalID := nominate(t, db, "rec-six", adminToken, nominee.ID)

	// The patch turns recusal on afterwards.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"leadership_model":"meritocratic","min_voting_tenure_days":0,"subject_recusal":true}`,
		nodeID)

	if code := voteVia(t, db, proposalID, nomineeToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Errorf("a vote that opened without recusal keeps its terms, got %d", code)
	}
}
