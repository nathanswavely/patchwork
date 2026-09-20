package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-108. Two surfaces counting what a member owes, and only one of them
// subtracting what they have already done.
//
// The Governance hub's needs_vote counts the proposals you still owe a
// ballot on and disappears the moment you cast one. The Dashboard's
// attention block counted a patch's open proposals, so the front door went
// on saying "ATTENTION NEEDED, 1 open proposal" after the vote was in. Teo:
// "the page inside the co-op knows I've voted and the front door still
// doesn't. I went back and checked the ballot a second time to make sure I
// really had saved it." Kofi: "I voted in that proposal twenty minutes ago
// and it is still asking for my attention."
//
// One definition, computed once, in the one place that already had it right.

// abstainOn takes part in a contest without approving anybody, through the
// route, the way the ballot's own button does.
func abstainOn(t *testing.T, db *database.DB, proposalID, token string) {
	t.Helper()
	body := map[string]interface{}{"candidate_ids": []string{}, "abstain": true}
	r := authedRequest("PUT", "/api/v1/proposals/"+proposalID+"/ballot", body, token)
	if w := serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r); w.Code != http.StatusOK {
		t.Fatalf("abstain: got %d: %s", w.Code, w.Body.String())
	}
}

// Taking part without approving anybody is taking part, so the nudge has to
// stop nudging. `election_abstentions` arrived after this count was written
// (it counts an election as unanswered while no election_ballots row
// exists), so an abstention left the banner standing over a ballot its
// owner had already cast.
func TestAwaitingVote_AnAbstentionIsAnAnsweredBallot(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "abst-nudge", "member")
	nodeID := electedNode(t, db, admin.ID, "Abstain Nudge", "abst-nudge", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	voter, voterToken := createTestUser(t, db, "abst-nudge-voter", "member")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)

	if got := overviewNeedsVote(t, db, "abst-nudge", voterToken); got != 1 {
		t.Fatalf("fixture: expected the contest to want a ballot, got %d", got)
	}

	abstainOn(t, db, id, voterToken)

	if got := overviewNeedsVote(t, db, "abst-nudge", voterToken); got != 0 {
		t.Errorf("the banner still wants a ballot the voter has cast: needs_vote = %d", got)
	}
}

// The number the front door reads, from the endpoint it already calls.
func awaitingOnList(t *testing.T, db *database.DB, slug, token string) (float64, bool) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/proposals?status=open", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("proposals: got %d: %s", w.Code, w.Body.String())
	}
	v, ok := decodeJSON(t, w)["awaiting_your_vote"].(float64)
	return v, ok
}

func votingFixture(t *testing.T, db *database.DB, slug string) (nodeID, proposalID, voterToken string) {
	t.Helper()
	admin, _ := createTestUser(t, db, slug+"-admin", "member")
	nodeID = createTestNode(t, db, admin.ID, "Summary "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	voter, voterToken := createTestUser(t, db, slug+"-voter", "member")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")
	proposalID = createTestProposal(t, db, nodeID, admin.ID)
	return nodeID, proposalID, voterToken
}

func TestAwaitingVote_TheListStatesWhatTheViewerStillOwes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, proposalID, voterToken := votingFixture(t, db, "front-door")

	got, ok := awaitingOnList(t, db, "front-door", voterToken)
	if !ok {
		t.Fatal("the proposals list does not state how many await this viewer's vote")
	}
	if got != 1 {
		t.Fatalf("awaiting_your_vote = %v, want 1", got)
	}

	body := map[string]interface{}{"value": "approve"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", body, voterToken)
	if w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r); w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("vote: got %d: %s", w.Code, w.Body.String())
	}

	// The front door subtracts the vote, exactly as the hub already did.
	if got, _ := awaitingOnList(t, db, "front-door", voterToken); got != 0 {
		t.Errorf("awaiting_your_vote = %v after voting, want 0", got)
	}
}

// The two surfaces read one number. Where they disagree, somebody checks
// their ballot a second time.
func TestAwaitingVote_TheListAndTheHubAgree(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, proposalID, voterToken := votingFixture(t, db, "agree")

	for _, step := range []string{"before", "after"} {
		hub := overviewNeedsVote(t, db, "agree", voterToken)
		list, ok := awaitingOnList(t, db, "agree", voterToken)
		if !ok {
			t.Fatal("no awaiting_your_vote on the list")
		}
		if float64(hub) != list {
			t.Errorf("%s voting: hub says %d, list says %v", step, hub, list)
		}
		if step == "before" {
			body := map[string]interface{}{"value": "approve"}
			r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", body, voterToken)
			serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r)
		}
	}
}

// A caller with no standing owes nothing, and is told so rather than being
// handed the patch's open count.
func TestAwaitingVote_AnOutsiderOwesNothing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := votingFixture(t, db, "outsider")
	follower, followerToken := createTestUser(t, db, "outsider-follower", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	openGovernanceRecord(t, db, nodeID)

	if got, _ := awaitingOnList(t, db, "outsider", followerToken); got != 0 {
		t.Errorf("a follower was told they owe %v ballots", got)
	}
}

var _ = database.DB{}

// F-106. A glimpse that takes three rows has to say so.
//
// Bobbin Hall's landing page showed "Council election OPEN / Council
// election UNSETTLED / Council election UNSETTLED" out of eight, and the
// press's showed three of seven. Nell found the rest by opening her
// personal export: "The press's page shows three. The file has six, going
// back to December 2025. Three of them I have never seen mentioned
// anywhere on this site."
//
// The total is what the glimpse subtracts from, so it has to be counted
// under the same filter the page is, or the remainder is wrong in whichever
// direction the drawer happens to be filtered.
func proposalsPage(t *testing.T, db *database.DB, slug, query, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/proposals"+query, nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("proposals%s: got %d: %s", query, w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

func TestGlimpse_TheTotalOutrunsThePage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "seven", "member")
	nodeID := createTestNode(t, db, admin.ID, "Seven", "seven", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for i := 0; i < 7; i++ {
		createTestProposal(t, db, nodeID, admin.ID)
	}

	body := proposalsPage(t, db, "seven", "?limit=3", adminToken)
	items, _ := body["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("expected a page of 3, got %d", len(items))
	}
	if body["total"] != float64(7) {
		t.Errorf("total = %v, want 7 — the glimpse subtracts from this", body["total"])
	}
}

// Counted under the same filter the page runs, so a filtered drawer's
// remainder is about the drawer.
func TestGlimpse_TheTotalRespectsTheFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "filtered", "member")
	nodeID := createTestNode(t, db, admin.ID, "Filtered", "filtered", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	open1 := createTestProposal(t, db, nodeID, admin.ID)
	createTestProposal(t, db, nodeID, admin.ID)
	closed := createTestProposal(t, db, nodeID, admin.ID)
	if _, err := db.Exec(`UPDATE proposals SET status = 'approved' WHERE id = ?`, closed); err != nil {
		t.Fatalf("close one: %v", err)
	}
	_ = open1

	if got := proposalsPage(t, db, "filtered", "?status=open&limit=1", adminToken)["total"]; got != float64(2) {
		t.Errorf("open total = %v, want 2", got)
	}
	if got := proposalsPage(t, db, "filtered", "?limit=1", adminToken)["total"]; got != float64(3) {
		t.Errorf("unfiltered total = %v, want 3", got)
	}
}

// A withheld record publishes no total either, for the same reason it
// publishes no count anywhere else.
func TestGlimpse_AWithheldRecordPublishesNoTotal(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, _ := createTestUser(t, db, "withheld-total", "member")
	nodeID := createTestNode(t, db, admin.ID, "Withheld Total", "withheld-total", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestProposal(t, db, nodeID, admin.ID)

	r := authedRequest("GET", "/api/v1/nodes/withheld-total/proposals?limit=3", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	body := decodeJSON(t, w)
	if body["total"] != float64(0) {
		t.Errorf("a closed record published total = %v", body["total"])
	}
	if body["public_governance_record"] != "nobody" {
		t.Errorf("expected the setting stated back, got %v", body["public_governance_record"])
	}
}
