package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Putting somebody forward, and taking your own name back (docs/adr/107).
//
// The bylaws have always said "any member may nominate themselves or another
// member" and the server has always accepted both; only the first had a
// control. A simulated member came to nominate a colleague, pressed the one
// button there was, and put herself on a three-seat ballot with no way off.

func standingIn(t *testing.T, db *database.DB, proposalID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT u.username FROM election_candidates ec
	                       JOIN users u ON u.id = ec.user_id
	                       WHERE ec.proposal_id = ? ORDER BY u.username`, proposalID)
	if err != nil {
		t.Fatalf("read slate: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			out = append(out, n)
		}
	}
	return out
}

func withdrawVia(t *testing.T, db *database.DB, proposalID, token string) int {
	t.Helper()
	r := authedRequest("DELETE", "/api/v1/proposals/"+proposalID+"/candidates/me", nil, token)
	return serveMux(t, db, "DELETE", "/api/v1/proposals/{id}/candidates/me",
		handler.WithdrawCandidacy(db), r).Code
}

// A member puts a colleague forward, and it is the colleague who goes on the
// slate.
func TestNomination_PutsTheNamedMemberForward(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "nom-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Nominating", "nominating", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	quiet, _ := createTestUser(t, db, "nom-quiet", "member")
	createTestMembership(t, db, quiet.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)

	if code := standFor(t, db, id, adminToken, quiet.ID); code != http.StatusCreated {
		t.Fatalf("putting somebody forward: expected 201, got %d", code)
	}
	slate := standingIn(t, db, id)
	if len(slate) != 1 || slate[0] != "nom-quiet" {
		t.Errorf("expected the person who was named on the slate, got %v", slate)
	}
}

// And the way back off, which is what makes the above safe to offer.
func TestNomination_YouCanTakeYourOwnNameBack(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "wd-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Withdrawing", "withdrawing", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, otherToken := createTestUser(t, db, "wd-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)

	// Ana's accident: she meant to nominate somebody else and stood herself.
	if code := standFor(t, db, id, adminToken, ""); code != http.StatusCreated {
		t.Fatalf("standing: expected 201, got %d", code)
	}
	// And the colleague she meant to name.
	if code := standFor(t, db, id, adminToken, other.ID); code != http.StatusCreated {
		t.Fatalf("nominating: expected 201, got %d", code)
	}
	if got := len(standingIn(t, db, id)); got != 2 {
		t.Fatalf("expected two on the slate, got %d", got)
	}

	if code := withdrawVia(t, db, id, adminToken); code != http.StatusNoContent {
		t.Fatalf("withdrawing your own candidacy: expected 204, got %d", code)
	}
	slate := standingIn(t, db, id)
	if len(slate) != 1 || slate[0] != "wd-other" {
		t.Errorf("expected only the colleague left standing, got %v", slate)
	}

	// Withdrawing again is nothing to withdraw.
	if code := withdrawVia(t, db, id, adminToken); code != http.StatusNotFound {
		t.Errorf("expected 404 for somebody who is not standing, got %d", code)
	}

	// The route is /me, so the only candidacy anybody can reach is their own:
	// the colleague takes their own name back, and nobody else could have.
	if code := withdrawVia(t, db, id, otherToken); code != http.StatusNoContent {
		t.Errorf("a nominee must be able to withdraw themselves, got %d", code)
	}
	if got := standingIn(t, db, id); len(got) != 0 {
		t.Errorf("expected an empty slate, got %v", got)
	}
}

// Once people are voting on the slate, the slate stops moving.
func TestNomination_CannotWithdrawOnceTheBallotIsRunning(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "wd2-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Too Late", "too-late", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)
	if code := standFor(t, db, id, adminToken, ""); code != http.StatusCreated {
		t.Fatalf("standing: expected 201, got %d", code)
	}
	closeNominations(t, db, id)
	if !handler.OpenElectionVoting(db, id) {
		t.Fatal("expected the ballot to open")
	}

	if code := withdrawVia(t, db, id, adminToken); code != http.StatusConflict {
		t.Errorf("expected 409 once people are voting on the slate, got %d", code)
	}
	if got := len(standingIn(t, db, id)); got != 1 {
		t.Errorf("the slate must not move mid-ballot, got %d standing", got)
	}
}
