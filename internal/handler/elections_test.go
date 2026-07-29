package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Elections (docs/adr/051): the contest Patchwork runs itself.
//
// The correctness-sensitive parts are the failure modes, not the happy path.
// Holdover has to hold on every way an election can settle nothing, and the
// last-admin floor has to survive a council being replaced wholesale.

func electedNode(t *testing.T, db *database.DB, ownerID, name, slug string, quorum, termMonths int) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":`+itoaT(quorum)+
			`,"default_vote_duration_hours":72,"leadership_model":"elected","leadership_venue":"patchwork",`+
			`"nomination_days":14,"admin_term_months":`+itoaT(termMonths)+`,"min_voting_tenure_days":0}`,
		nodeID)
	return nodeID
}

func itoaT(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

func openElection(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	handler.StartElectionOnAdoption(db, nodeID)
	var id string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND seats_contested > 0`, nodeID).Scan(&id)
	if id == "" {
		t.Fatal("expected adoption to open an election")
	}
	return id
}

// closeNominations drags the nomination window into the past so the sweep can
// open voting, the way the clock would.
func closeNominations(t *testing.T, db *database.DB, proposalID string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET nominations_close_at = ? WHERE id = ?`, past, proposalID)
}

func standFor(t *testing.T, db *database.DB, proposalID, token, userID string) int {
	t.Helper()
	body := map[string]interface{}{}
	if userID != "" {
		body["user_id"] = userID
	}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/candidates", body, token)
	return serveMux(t, db, "POST", "/api/v1/proposals/{id}/candidates", handler.AddCandidate(db), r).Code
}

func candidateIDFor(t *testing.T, db *database.DB, proposalID, userID string) string {
	t.Helper()
	var id string
	db.QueryRow(`SELECT id FROM election_candidates WHERE proposal_id = ? AND user_id = ?`, proposalID, userID).Scan(&id)
	return id
}

func castApprovals(t *testing.T, db *database.DB, proposalID, token string, candidateIDs []string) int {
	t.Helper()
	r := authedRequest("PUT", "/api/v1/proposals/"+proposalID+"/ballot",
		map[string]interface{}{"candidate_ids": candidateIDs}, token)
	return serveMux(t, db, "PUT", "/api/v1/proposals/{id}/ballot", handler.CastElectionBallot(db), r).Code
}

func adminCount(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&n)
	return n
}

// Adopting elected leadership opens an election for the council the patch
// already has.
func TestElection_AdoptionOpensOne(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "elec1", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec One", "elec-one", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	second, _ := createTestUser(t, db, "elec1b", "member")
	createTestMembership(t, db, second.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)

	var seats int
	var votingEnds, nominationsClose string
	db.QueryRow(`SELECT seats_contested, COALESCE(voting_ends_at,''), COALESCE(nominations_close_at,'')
	             FROM proposals WHERE id = ?`, id).Scan(&seats, &votingEnds, &nominationsClose)
	if seats != 2 {
		t.Errorf("expected the sitting council's two seats contested, got %d", seats)
	}
	if nominationsClose == "" {
		t.Error("expected a nomination window")
	}
	// The one proposal that is not born voting: the clock starts when
	// nominations close (docs/adr/051's amendment to 048).
	if votingEnds != "" {
		t.Errorf("voting must not be open during nominations, got %q", votingEnds)
	}

	// Idempotent — a second rules edit must not open a rival contest.
	handler.StartElectionOnAdoption(db, nodeID)
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE node_id = ? AND seats_contested > 0`, nodeID).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly one election, got %d", count)
	}
}

// Where the venue is elsewhere, Patchwork conducts nothing (docs/adr/052).
func TestElection_NotOpenedWhereVenueIsElsewhere(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "elec2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Elec Two", "elec-two", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","leadership_model":"elected","leadership_venue":"elsewhere"}`, nodeID)

	handler.StartElectionOnAdoption(db, nodeID)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE node_id = ? AND seats_contested > 0`, nodeID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no election where admins are chosen elsewhere, got %d", count)
	}
}

// Nominating is a member act; a follower is not on the ladder. And no ballot
// may be cast while nominations are open.
func TestElection_NominationGates(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "elec3", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Three", "elec-three", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "elec3m", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	follower, followerToken := createTestUser(t, db, "elec3f", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	id := openElection(t, db, nodeID)

	if code := standFor(t, db, id, memberToken, ""); code != http.StatusCreated {
		t.Errorf("a member may stand, got %d", code)
	}
	if code := standFor(t, db, id, followerToken, ""); code != http.StatusForbidden {
		t.Errorf("a follower may not nominate, got %d", code)
	}
	// Voting has not opened yet.
	cid := candidateIDFor(t, db, id, member.ID)
	if code := castApprovals(t, db, id, adminToken, []string{cid}); code != http.StatusConflict {
		t.Errorf("no ballot during nominations, got %d", code)
	}
}

// The whole path: nominations close, voting opens with its terms photographed,
// approvals are counted, and the top candidates take the seats.
func TestElection_SeatsTheMostApproved(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "elec4", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Four", "elec-four", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	popular, popularToken := createTestUser(t, db, "elec4pop", "member")
	createTestMembership(t, db, popular.ID, nodeID, "member", "active")
	quiet, _ := createTestUser(t, db, "elec4quiet", "member")
	createTestMembership(t, db, quiet.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID) // one seat: the sitting admin
	standFor(t, db, id, popularToken, "")
	standFor(t, db, id, adminToken, quiet.ID)

	closeNominations(t, db, id)
	if !handler.OpenElectionVoting(db, id) {
		t.Fatal("expected voting to open once nominations closed")
	}
	var terms, votingEnds string
	db.QueryRow(`SELECT COALESCE(voting_terms,''), COALESCE(voting_ends_at,'') FROM proposals WHERE id = ?`, id).Scan(&terms, &votingEnds)
	if terms == "" || votingEnds == "" {
		t.Fatalf("expected the terms photographed and the clock started, got terms=%q ends=%q", terms, votingEnds)
	}

	popularCID := candidateIDFor(t, db, id, popular.ID)
	if code := castApprovals(t, db, id, adminToken, []string{popularCID}); code != http.StatusOK {
		t.Fatalf("ballot: got %d", code)
	}
	if code := castApprovals(t, db, id, popularToken, []string{popularCID}); code != http.StatusOK {
		t.Fatalf("ballot: got %d", code)
	}

	expireProposal(t, db, id)
	handler.SweepElections(db)

	if got := roleOf(t, db, popular.ID, nodeID); got != "admin" {
		t.Errorf("expected the most-approved candidate seated, got %q", got)
	}
	if got := roleOf(t, db, quiet.ID, nodeID); got != "member" {
		t.Errorf("an unapproved candidate must not be seated, got %q", got)
	}
	// A seat exists and carries the term.
	var seatCount int
	var termEnds string
	db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(term_ends_at),'') FROM seats WHERE node_id = ? AND holder_id = ?`,
		nodeID, popular.ID).Scan(&seatCount, &termEnds)
	if seatCount != 1 || termEnds == "" {
		t.Errorf("expected one seat with a term end, got %d / %q", seatCount, termEnds)
	}
	// The council the electorate did not return steps down.
	if got := roleOf(t, db, admin.ID, nodeID); got != "member" {
		t.Errorf("expected the unreturned admin to step down, got %q", got)
	}
}

// Holdover: nobody stood, so nothing changes.
func TestElection_NoCandidatesLeavesTheCouncil(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "elec5", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Five", "elec-five", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	expireProposal(t, db, id)
	handler.SweepElections(db)

	if got := roleOf(t, db, admin.ID, nodeID); got != "admin" {
		t.Errorf("holdover: the sitting admin must keep serving, got %q", got)
	}
	var status string
	db.QueryRow(`SELECT status FROM proposals WHERE id = ?`, id).Scan(&status)
	if status == "open" {
		t.Error("expected the election closed rather than left open forever")
	}
	if adminCount(t, db, nodeID) < 1 {
		t.Fatal("an election that settles nothing must never empty a patch")
	}
}

// Holdover: quorum unmet.
func TestElection_QuorumFailureLeavesTheCouncil(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "elec6", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Six", "elec-six", 100, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	challenger, challengerToken := createTestUser(t, db, "elec6c", "member")
	createTestMembership(t, db, challenger.ID, nodeID, "member", "active")
	// A third person who never votes, so one ballot cannot reach 100%.
	bystander, _ := createTestUser(t, db, "elec6b", "member")
	createTestMembership(t, db, bystander.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, challengerToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, challenger.ID)})
	expireProposal(t, db, id)
	handler.SweepElections(db)

	if got := roleOf(t, db, challenger.ID, nodeID); got == "admin" {
		t.Error("a council must not be seated on a vote that missed quorum")
	}
	if got := roleOf(t, db, admin.ID, nodeID); got != "admin" {
		t.Errorf("holdover: the sitting admin keeps serving, got %q", got)
	}
}

// Replacing a council wholesale must not trip the last-admin floor on the way
// through, and must not leave the patch empty at any point.
func TestElection_WholesaleReplacementKeepsAnAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "elec7", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Seven", "elec-seven", 0, 0)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	challenger, challengerToken := createTestUser(t, db, "elec7c", "member")
	createTestMembership(t, db, challenger.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, challengerToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, challengerToken, []string{candidateIDFor(t, db, id, challenger.ID)})
	expireProposal(t, db, id)
	handler.SweepElections(db)

	if got := roleOf(t, db, challenger.ID, nodeID); got != "admin" {
		t.Errorf("expected the elected challenger seated, got %q", got)
	}
	if adminCount(t, db, nodeID) < 1 {
		t.Fatal("the patch must never be left with no admin")
	}
	// No term length configured: a council that serves until the next election.
	var termEnds string
	db.QueryRow(`SELECT COALESCE(term_ends_at,'') FROM seats WHERE node_id = ? AND holder_id = ?`,
		nodeID, challenger.ID).Scan(&termEnds)
	if termEnds != "" {
		t.Errorf("expected no term end where the patch sets no term length, got %q", termEnds)
	}
}

// A ballot replaces wholesale: approval voting is the set you now hold, not an
// append-only pile.
func TestElection_BallotReplacesRatherThanAccumulates(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "elec8", "member")
	nodeID := electedNode(t, db, admin.ID, "Elec Eight", "elec-eight", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	a, aToken := createTestUser(t, db, "elec8a", "member")
	createTestMembership(t, db, a.ID, nodeID, "member", "active")
	b, _ := createTestUser(t, db, "elec8b", "member")
	createTestMembership(t, db, b.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, aToken, "")
	standFor(t, db, id, adminToken, b.ID)
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)

	aCID := candidateIDFor(t, db, id, a.ID)
	bCID := candidateIDFor(t, db, id, b.ID)
	castApprovals(t, db, id, adminToken, []string{aCID, bCID})
	castApprovals(t, db, id, adminToken, []string{aCID}) // changed their mind

	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM election_ballots WHERE proposal_id = ? AND voter_id = ?`, id, admin.ID).Scan(&rows)
	if rows != 1 {
		t.Errorf("expected the ballot replaced, got %d approvals", rows)
	}
}
