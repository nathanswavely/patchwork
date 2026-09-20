package handler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A contest says what it did and why.
//
// An ordinary proposal has always explained itself: "Quorum met (3 of 4
// voted, 50% needed)", the voters named, the terms it was judged by. A
// contest said "Settled nothing." A four-person collective failed six in ten
// months and its founder found the arithmetic on a settings form, phrased
// about a hypothetical proposal, and worked out from it that her own
// election had failed by one person not turning up.
//
// Underneath that was a second defect, found while deciding which number the
// page should print: quorum's numerator counted every ballot row and its
// denominator counted the current electorate, so the two came from different
// populations.

// closeVoting drags a contest's ballot window into the past, the way the
// clock would.
func closeVoting(t *testing.T, db *database.DB, proposalID string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, past, proposalID)
}

func turnoutOfProposal(t *testing.T, db *database.DB, proposalID, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, token)
	w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	body := decodeJSON(t, w)
	turnout, ok := body["election_turnout"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected election_turnout on a contest, got %v", body["election_turnout"])
	}
	return turnout
}

// Four members, a 50% quorum, one ballot cast: the page can say so.
func TestContest_ReportsTurnoutWhileVotingIsOpen(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "why1", "member")
	nodeID := electedNode(t, db, admin.ID, "Why One", "why-one", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for _, n := range []string{"why1a", "why1b", "why1c"} {
		u, _ := createTestUser(t, db, n, "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, admin.ID)})

	turnout := turnoutOfProposal(t, db, id, adminToken)
	if turnout["voted"] != float64(1) {
		t.Errorf("voted = %v, want 1", turnout["voted"])
	}
	if turnout["eligible"] != float64(4) {
		t.Errorf("eligible = %v, want 4", turnout["eligible"])
	}
	if turnout["needed"] != float64(2) {
		t.Errorf("needed = %v, want 2", turnout["needed"])
	}
	if turnout["met"] != false {
		t.Errorf("met = %v, want false on one of four against a 50%% quorum", turnout["met"])
	}
}

// An ordinary proposal carries no election_turnout, so the page can test one
// field rather than re-deriving whether it is looking at a contest.
func TestContest_TurnoutIsAbsentOnAnOrdinaryProposal(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "why2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Why Two", "why-two", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	pid := createTestProposal(t, db, nodeID, admin.ID)

	r := authedRequest("GET", "/api/v1/proposals/"+pid, nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	if got := decodeJSON(t, w)["election_turnout"]; got != nil {
		t.Errorf("election_turnout = %v on a proposal that is not a contest, want null", got)
	}
}

// The number on the page is the number that decided, and the notice says it.
func TestContest_SaysWhyItFailedInTheNumbersItFailedBy(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "why3", "member")
	nodeID := electedNode(t, db, admin.ID, "Why Three", "why-three", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for _, n := range []string{"why3a", "why3b", "why3c"} {
		u, _ := createTestUser(t, db, n, "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, admin.ID)})
	closeVoting(t, db, id)
	handler.SweepElections(db)

	var state string
	db.QueryRow(`SELECT COALESCE(state,'') FROM proposals WHERE id = ?`, id).Scan(&state)
	if state != "unsettled" {
		t.Fatalf("expected the contest to settle nothing under quorum, state = %q", state)
	}

	body := unsettledBodyFor(t, db, nodeID)
	for _, want := range []string{"1 of 4", "2 were needed"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notice does not say %q; it says %q", want, body)
		}
	}
}

// Quorum's numerator and denominator have to describe the same population. A
// ballot from somebody who has since left the patch counts toward nobody's
// approvals, and used to count toward quorum.
func TestContest_ADepartedVotersBallotCountsTowardNeither(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "why4", "member")
	nodeID := electedNode(t, db, admin.ID, "Why Four", "why-four", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	leaver, leaverToken := createTestUser(t, db, "why4a", "member")
	createTestMembership(t, db, leaver.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	cid := candidateIDFor(t, db, id, admin.ID)
	castApprovals(t, db, id, leaverToken, []string{cid})

	// And then they leave.
	db.Exec(`UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?`, leaver.ID, nodeID)

	turnout := turnoutOfProposal(t, db, id, adminToken)
	if turnout["voted"] != float64(0) {
		t.Errorf("voted = %v, want 0: the only ballot was cast by somebody who has left", turnout["voted"])
	}
	if turnout["eligible"] != float64(1) {
		t.Errorf("eligible = %v, want 1", turnout["eligible"])
	}
	// The bug in its plainest form: turnout could exceed the electorate.
	if turnout["voted"].(float64) > turnout["eligible"].(float64) {
		t.Error("more people voted than could vote, which is the two-populations bug")
	}
}

// A contest records who it seated, so the record can name them without
// re-deriving it from a tally that moves when somebody leaves.
func TestContest_RecordsWhoItSeated(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "why5", "member")
	nodeID := electedNode(t, db, admin.ID, "Why Five", "why-five", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	challenger, _ := createTestUser(t, db, "why5a", "member")
	createTestMembership(t, db, challenger.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, challenger.ID)
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, adminToken, []string{candidateIDFor(t, db, id, challenger.ID)})
	closeVoting(t, db, id)
	handler.SweepElections(db)

	var seated int
	db.QueryRow(`SELECT seated FROM election_candidates WHERE proposal_id = ? AND user_id = ?`,
		id, challenger.ID).Scan(&seated)
	if seated != 1 {
		t.Error("the contest did not record who it seated")
	}

	// And the record names them rather than saying a council was seated.
	r := authedRequest("GET", "/api/v1/nodes/why-five/governance/record", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(db), r)
	items, _ := decodeJSON(t, w)["items"].([]interface{})
	var found bool
	for _, it := range items {
		e := it.(map[string]interface{})
		if e["kind"] != "election" || e["outcome"] != "seated" {
			continue
		}
		found = true
		names, _ := e["names"].([]interface{})
		if len(names) != 1 {
			t.Fatalf("expected the record to name the one person seated, got %v", e["names"])
		}
	}
	if !found {
		t.Error("the seated contest is not on the record")
	}
}

// A council can be part full, and "the council continues" over one person on
// a bench of three is what a former chair called generous to the point of
// being untrue.
func TestContest_APartlyFilledCouncilIsNotToldItContinues(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "why6", "member")
	nodeID := electedNode(t, db, admin.ID, "Why Six", "why-six", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for _, n := range []string{"why6a", "why6b", "why6c"} {
		u, _ := createTestUser(t, db, n, "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}
	// One chair held, two empty.
	seedSeatFor(t, db, nodeID, admin.ID, "2027-08-15")
	makeVacantSeat(t, db, nodeID, "2027-08-15")
	makeVacantSeat(t, db, nodeID, "2027-08-15")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	closeVoting(t, db, id)
	handler.SweepElections(db)

	body := unsettledBodyFor(t, db, nodeID)
	if strings.Contains(body, "The council continues until a successor is elected.") {
		t.Errorf("a council of one on a bench of three was told it continues: %q", body)
	}
	if !strings.Contains(body, "still empty") {
		t.Errorf("the notice does not say the other chairs are empty: %q", body)
	}
}
