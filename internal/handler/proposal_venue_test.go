package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Declaring the venue elsewhere removes the ballot and keeps the discussion
// (docs/adr/053).
//
// This is what makes the attestation gate mean anything. A patch with both a
// tally and an attestation would let an admin who disliked where the tally was
// heading record a meeting result instead — the bypass restored one field
// further along. So these tests are really about the absence: no clock, no
// ballot, and no `can_vote` saying otherwise.

func createProposal(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/proposals", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		return w.Code, map[string]interface{}{"error": w.Body.String()}
	}
	return w.Code, decodeJSON(t, w)
}

func getProposal(t *testing.T, db *database.DB, id, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+id, nil, token)
	w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 reading the proposal, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

func castVote(t *testing.T, db *database.DB, id, token, value string) int {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/vote", map[string]interface{}{"value": value}, token)
	return serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r).Code
}

// A proposal on such a patch is open and has no clock. `voting_ends_at` stays
// NULL on purpose: resolveProposal only runs where there is an end to have
// passed, so the absence is what keeps an undecidable proposal from being
// decided by a timer.
func TestProposalVenue_BornWithoutABallot(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv1", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "PV One", "pv-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	code, p := createProposal(t, db, "pv-one", adminToken, map[string]interface{}{
		"title": "Buy the kiln", "body": "It is time.", "proposal_type": "action",
	})
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %v", code, p)
	}

	id := p["id"].(string)
	if got := stateOf(t, db, id); got != "elsewhere" {
		t.Errorf("expected state=elsewhere, got %q", got)
	}
	var endsAt *string
	db.QueryRow("SELECT voting_ends_at FROM proposals WHERE id = ?", id).Scan(&endsAt)
	if endsAt != nil {
		t.Errorf("expected no voting window, got %q", *endsAt)
	}
	// Still open: it can be discussed, revised and withdrawn.
	var status string
	db.QueryRow("SELECT status FROM proposals WHERE id = ?", id).Scan(&status)
	if status != "open" {
		t.Errorf("expected the proposal to stay open, got %q", status)
	}
}

// The gate and the page have to agree. `can_vote` is the server's answer
// (docs/adr/044), and a payload saying you may vote on something nothing will
// accept a vote for is the contradiction that rule exists to end.
func TestProposalVenue_NoBallotAndNoOfferOfOne(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv2", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "PV Two", "pv-two")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "pv2m", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	_, p := createProposal(t, db, "pv-two", adminToken, map[string]interface{}{
		"title": "Buy the kiln", "proposal_type": "action",
	})
	id := p["id"].(string)

	if got := getProposal(t, db, id, memberToken)["can_vote"]; got != false {
		t.Errorf("expected can_vote=false, got %v", got)
	}
	if code := castVote(t, db, id, memberToken, "approve"); code != http.StatusConflict {
		t.Errorf("expected the vote refused with 409, got %d", code)
	}
	var votes int
	db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ?", id).Scan(&votes)
	if votes != 0 {
		t.Errorf("expected no ballot written, got %d", votes)
	}
}

// A patch that flips the venue mid-vote does not kill the vote (docs/adr/047:
// a vote keeps the terms it opened with). The venue governs new proposals, and
// the refusal reads off the row rather than the patch's current config.
func TestProposalVenue_FlipDoesNotReachOpenVotes(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv3", "member")
	nodeID := createTestNode(t, db, admin.ID, "PV Three", "pv-three", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"min_voting_tenure_days":0}`,
		nodeID)
	member, memberToken := createTestUser(t, db, "pv3m", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	_, p := createProposal(t, db, "pv-three", adminToken, map[string]interface{}{
		"title": "Raised while we still voted here", "proposal_type": "action",
	})
	id := p["id"].(string)

	// The patch moves its venue after the proposal was raised.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,`+
			`"proposal_venue":"elsewhere","min_voting_tenure_days":0}`, nodeID)

	if code := castVote(t, db, id, memberToken, "approve"); code != http.StatusOK {
		t.Errorf("an open vote finishes under the terms it opened with, got %d", code)
	}
}

// The rules file is machine configuration, not a text a meeting adopts, so it
// never comes back as an attestation. A member's rules proposal on such a
// patch would sit open forever; saying so beats accepting it.
func TestProposalVenue_RulesChangeIsAnAdminsDirectChange(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv4", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "PV Four", "pv-four")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "pv4m", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	setupGovernanceForNode(t, nodeID)

	rulesBody := func() map[string]interface{} {
		return map[string]interface{}{
			"title": "Raise the quorum", "proposal_type": "amendment",
			"target_doc": "governance-rules.json", "proposed_title": "Governance Rules",
			"proposed_body": `{"decision_method":"majority","quorum_percent":50,"proposal_venue":"elsewhere"}`,
		}
	}

	if code, p := createProposal(t, db, "pv-four", memberToken, rulesBody()); code != http.StatusConflict {
		t.Errorf("a member's rules proposal should be refused with 409, got %d: %v", code, p)
	}

	code, p := createProposal(t, db, "pv-four", adminToken, rulesBody())
	if code != http.StatusCreated {
		t.Fatalf("an admin's rules change should apply, got %d: %v", code, p)
	}
	if got := stateOf(t, db, p["id"].(string)); got != "in_effect" {
		t.Errorf("expected a direct change born applied, got %q", got)
	}
}

// A prose amendment on such a patch is a discussion, not a direct change. The
// admin-decides bypass is a different rule and must not leak into this one:
// an admin here has no more say over the charter's text than any member,
// because the meeting decides it.
func TestProposalVenue_ProseAmendmentIsNotAnAdminsToApply(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv5", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "PV Five", "pv-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	setupGovernanceForNode(t, nodeID)

	_, p := createProposal(t, db, "pv-five", adminToken, map[string]interface{}{
		"title": "Reword Article II", "proposal_type": "amendment",
		"target_doc": "bylaws.md", "proposed_body": "New wording",
	})
	if got := stateOf(t, db, p["id"].(string)); got != "elsewhere" {
		t.Errorf("expected state=elsewhere, got %q", got)
	}
}

// The nudge names the same set the gate does (docs/adr/044), and two kinds of
// open proposal are outside it. Both were counted, because `votes` holds no
// row for either: a proposal with no ballot at all (docs/adr/053), and an
// election, whose ballot is rows in `election_ballots` (docs/adr/051) — so
// `NOT EXISTS (SELECT 1 FROM votes …)` was true for every election forever,
// including during nominations when no ballot may be cast.
func TestNeedsVote_CountsOnlyProposalsAVoteCanBeCastOn(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "nv1", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "NV One", "nv-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	needsVote := func() float64 {
		r := authedRequest("GET", "/api/v1/nodes/nv-one/governance/overview", nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview",
			handler.GovernanceOverview(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		n, _ := decodeJSON(t, w)["needs_vote"].(float64)
		return n
	}

	createProposal(t, db, "nv-one", adminToken, map[string]interface{}{
		"title": "Decided at the meeting", "proposal_type": "action",
	})
	if got := needsVote(); got != 0 {
		t.Errorf("a proposal with no ballot needs nobody's vote, got %v", got)
	}

	// An election, opened by the calendar rather than by a person, sitting in
	// its nomination window.
	db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state,
	         proposal_type, duration_hours, created_at, updated_at, seats_contested, nominations_close_at)
	         VALUES (?, ?, ?, 'Council election', '', 'open', 'voting', 'membership', 72,
	         '2026-07-01T00:00:00.000Z', '2026-07-01T00:00:00.000Z', 2, '2030-01-01T00:00:00.000Z')`,
		auth.NewUUIDv7(), nodeID, admin.ID)
	if got := needsVote(); got != 0 {
		t.Errorf("nominations are not a ballot, got %v", got)
	}
}

// An attestation can land on top of an in-flight amendment proposal, whose
// diff is then against something else. docs/adr/053 takes that trade — a
// community's own text should win over a draft — and owes the draft's readers
// the news.
func TestProposalVenue_OpenAmendmentIsToldTheGroundMoved(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "pv6", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "PV Six", "pv-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	setupGovernanceForNode(t, nodeID)
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws", "Old text", "charter")

	_, p := createProposal(t, db, "pv-six", adminToken, map[string]interface{}{
		"title": "Reword Article II", "proposal_type": "amendment",
		"target_doc": "bylaws.md", "proposed_body": "Drafted against the old text",
	})
	id := p["id"].(string)

	if got := getProposal(t, db, id, adminToken)["ground_moved"]; got != nil {
		t.Errorf("nothing has moved yet, got %v", got)
	}

	res := recordAdoption(t, db, "pv-six", adminToken, map[string]interface{}{
		"doc_id": docID, "decided_at": "2026-03-14", "adopted_body": "What the meeting actually adopted",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201 recording, got %d: %s", res.Code, res.Body)
	}

	after := getProposal(t, db, id, adminToken)
	if after["ground_moved"] != true {
		t.Errorf("expected the draft to be told the ground moved, got %v", after["ground_moved"])
	}
	if after["ground_moved_at"] != "2026-03-14" {
		t.Errorf("expected the decision's own date, got %v", after["ground_moved_at"])
	}
}
