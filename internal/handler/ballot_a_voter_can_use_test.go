package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Two things a ballot could not do.
//
// F-095: a candidate had nowhere to say why they were standing, so the slate
// was a list of bare names and four voters decided on the strength of a
// comment behind a Discussion tab.
//
// F-092: approving nobody left no rows, so it could not be told from not
// voting — on a patch that had failed six contests for turnout, and where a
// member was asking everyone to turn up and tick nothing to make quorum.

// standWithStatement stands the caller with a statement, through the route.
func standWithStatement(t *testing.T, db *database.DB, proposalID, token, statement string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]interface{}{"statement": statement}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/candidates", body, token)
	return serveMux(t, db, "POST", "/api/v1/proposals/{id}/candidates", handler.AddCandidate(db), r)
}

func candidatesOf(t *testing.T, db *database.DB, proposalID, token string) []map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, token)
	w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	raw, _ := decodeJSON(t, w)["candidates"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, c := range raw {
		out = append(out, c.(map[string]interface{}))
	}
	return out
}

func TestBallot_ACandidateSaysWhyTheyAreStanding(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "why-stand", "member")
	nodeID := electedNode(t, db, admin.ID, "Why Stand", "why-stand", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	if w := standWithStatement(t, db, id, adminToken, "  I have run the bar rota for three years.  "); w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("stand: got %d: %s", w.Code, w.Body.String())
	}

	cands := candidatesOf(t, db, id, adminToken)
	if len(cands) != 1 {
		t.Fatalf("expected one candidate, got %d", len(cands))
	}
	// Trimmed, and on the ballot rather than behind another tab.
	if cands[0]["statement"] != "I have run the bar rota for three years." {
		t.Errorf("statement = %q", cands[0]["statement"])
	}
}

// Standing is optional prose. A patch where everybody knows everybody should
// not be made to write an essay.
func TestBallot_AStatementIsOptional(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "no-words", "member")
	nodeID := electedNode(t, db, admin.ID, "No Words", "no-words", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")

	cands := candidatesOf(t, db, id, adminToken)
	if len(cands) != 1 {
		t.Fatalf("expected one candidate, got %d", len(cands))
	}
	if s, present := cands[0]["statement"]; present && s != "" {
		t.Errorf("a candidate who wrote nothing carries %q", s)
	}
}

// Putting somebody forward is a real act; speaking for them is not.
func TestBallot_YouCannotWriteSomebodyElsesStatement(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "not-yours", "member")
	nodeID := electedNode(t, db, admin.ID, "Not Yours", "not-yours", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "not-yours-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	body := map[string]interface{}{"user_id": other.ID, "statement": "They would be great."}
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/candidates", body, adminToken)
	serveMux(t, db, "POST", "/api/v1/proposals/{id}/candidates", handler.AddCandidate(db), r)

	var stored string
	db.QueryRow(`SELECT statement FROM election_candidates WHERE proposal_id = ? AND user_id = ?`,
		id, other.ID).Scan(&stored)
	if stored != "" {
		t.Errorf("a nominator wrote words into somebody else's mouth: %q", stored)
	}
}

func TestBallot_AStatementHasALimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "too-long", "member")
	nodeID := electedNode(t, db, admin.ID, "Too Long", "too-long", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	w := standWithStatement(t, db, id, adminToken, strings.Repeat("x", 501))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for an over-long statement, got %d", w.Code)
	}
}

// F-092. Taking part without approving anybody counts toward the turnout
// that decides the contest.
func TestBallot_ApprovingNobodyStillCounts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "abst", "member")
	nodeID := electedNode(t, db, admin.ID, "Abst", "abst", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, otherToken := createTestUser(t, db, "abst-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)

	before := turnoutOfProposal(t, db, id, adminToken)
	if before["voted"] != float64(0) {
		t.Fatalf("fixture: expected nobody to have voted, got %v", before["voted"])
	}

	// The member turns up and approves nobody.
	body := map[string]interface{}{"candidate_ids": []string{}, "abstain": true}
	r := authedRequest("PUT", "/api/v1/proposals/"+id+"/ballot", body, otherToken)
	if w := serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r); w.Code != http.StatusOK {
		t.Fatalf("abstain: got %d: %s", w.Code, w.Body.String())
	}

	after := turnoutOfProposal(t, db, id, adminToken)
	if after["voted"] != float64(1) {
		t.Errorf("an abstention did not count toward turnout: voted = %v", after["voted"])
	}
}

// An empty ballot with no abstain flag still means "take mine back", because
// a member needs both acts and they are not the same.
func TestBallot_AnEmptyBallotIsStillAWithdrawal(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "withdrawb", "member")
	nodeID := electedNode(t, db, admin.ID, "Withdraw B", "withdraw-b", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, admin.ID)})
	if turnoutOfProposal(t, db, id, adminToken)["voted"] != float64(1) {
		t.Fatal("fixture: the approval did not register")
	}

	body := map[string]interface{}{"candidate_ids": []string{}}
	r := authedRequest("PUT", "/api/v1/proposals/"+id+"/ballot", body, adminToken)
	serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r)

	if got := turnoutOfProposal(t, db, id, adminToken)["voted"]; got != float64(0) {
		t.Errorf("an empty ballot left the voter counted: voted = %v", got)
	}
}

// Approving somebody replaces an abstention, and the two never both stand.
func TestBallot_AnApprovalReplacesAnAbstention(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "swap", "member")
	nodeID := electedNode(t, db, admin.ID, "Swap", "swap", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)

	body := map[string]interface{}{"candidate_ids": []string{}, "abstain": true}
	r := authedRequest("PUT", "/api/v1/proposals/"+id+"/ballot", body, adminToken)
	serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r)

	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, admin.ID)})

	var abstentions int
	db.QueryRow(`SELECT COUNT(*) FROM election_abstentions WHERE proposal_id = ?`, id).Scan(&abstentions)
	if abstentions != 0 {
		t.Errorf("approving somebody left the abstention standing: %d rows", abstentions)
	}
	// And the voter is counted once, not twice.
	if got := turnoutOfProposal(t, db, id, adminToken)["voted"]; got != float64(1) {
		t.Errorf("voted = %v, want 1", got)
	}
}

// Both at once is not a thing a person can mean.
func TestBallot_RefusesApprovalsAndAnAbstentionTogether(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "both", "member")
	nodeID := electedNode(t, db, admin.ID, "Both", "both", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)

	body := map[string]interface{}{
		"candidate_ids": []string{candidateIDFor(t, db, id, admin.ID)},
		"abstain":       true,
	}
	r := authedRequest("PUT", "/api/v1/proposals/"+id+"/ballot", body, adminToken)
	w := serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
