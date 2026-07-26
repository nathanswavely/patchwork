package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A proposal opens for voting when it is created (docs/adr/048). The
// migration-016 state machine lists `draft` and `discussion` ahead of
// `voting`, and the SPA carried a "Submit for voting" button for the author of
// a draft — but nothing ever wrote either state, and the button PATCHed a
// field `UpdateProposal` did not decode, so it answered 400 "no valid fields
// to update" on the one path that would have used it. Both halves are gone;
// these tests hold the invariant that replaced them.

// The voting path is born voting. This is the assertion the whole decision
// rests on: if a pre-voting state ever becomes reachable, the voting-terms
// photograph (docs/adr/047) and `voting_ends_at` — both stamped at INSERT —
// are taken at the wrong moment, and this test is where that shows up first.
func TestCreateProposal_IsBornVoting(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "bornvoting", "member")
	nodeID := createTestNode(t, db, admin.ID, "Born Voting", "born-voting", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// A voting patch, so docs/adr/041's direct-change path is not in play.
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":72,"amendment_threshold":"majority","min_voting_tenure_days":0}`,
		nodeID)

	for _, proposalType := range []string{"action", "other", "membership"} {
		body := map[string]interface{}{
			"title":         "Born voting",
			"body":          "Body",
			"proposal_type": proposalType,
		}
		r := authedRequest("POST", "/api/v1/nodes/born-voting/proposals", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)

		if w.Code != http.StatusCreated {
			t.Fatalf("proposal_type=%s: expected 201, got %d: %s", proposalType, w.Code, w.Body.String())
		}
		result := decodeJSON(t, w)
		if result["state"] != "voting" {
			t.Errorf("proposal_type=%s: expected state=voting, got %v", proposalType, result["state"])
		}
		// Born voting means the clock is already running.
		if result["voting_ends_at"] == nil || result["voting_ends_at"] == "" {
			t.Errorf("proposal_type=%s: expected voting_ends_at to be stamped at creation", proposalType)
		}
	}

	var preVoting int
	db.QueryRow("SELECT COUNT(*) FROM proposals WHERE state IN ('draft','discussion')").Scan(&preVoting)
	if preVoting != 0 {
		t.Errorf("expected no proposal in a pre-voting state, got %d", preVoting)
	}
}

// The dead control's request, answered on purpose. It used to be dropped
// silently and fall out as "no valid fields to update", which reads like a bug
// in the caller rather than a decision about the product.
func TestUpdateProposal_StateIsNotSettable(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "statesetter", "member")
	nodeID := createTestNode(t, db, admin.ID, "State Setter", "state-setter", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	proposalID := seedStaleProposal(t, db, nodeID, admin.ID, "open", "voting", nil)

	// 'voting' is what the removed "Submit for voting" button sent; the others
	// are the states it would have been leaving. None of them is settable, and
	// a no-op write of the current state is refused too — the field is not a
	// field, not a field whose value happens to be wrong.
	for _, state := range []string{"voting", "draft", "discussion", "approved", "in_effect"} {
		r := authedRequest("PATCH", "/api/v1/proposals/"+proposalID, map[string]interface{}{"state": state}, adminToken)
		w := serveMux(t, db, "PATCH", "/api/v1/proposals/{id}", handler.UpdateProposal(db), r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("state=%s: expected 400, got %d: %s", state, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "not settable") {
			t.Errorf("state=%s: expected the refusal to say state is not settable, got %s", state, w.Body.String())
		}
		if got := stateOf(t, db, proposalID); got != "voting" {
			t.Fatalf("state=%s: proposal state moved to %q; the refusal must not write", state, got)
		}
	}
}

// The guard sits ahead of the decode's other fields, so it has to refuse a
// request carrying `state` without taking the rest of it with it — a caller
// that sends state alongside a title gets a refusal, not a half-applied edit.
func TestUpdateProposal_StateRefusalDoesNotApplyOtherFields(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "statewithtitle", "member")
	nodeID := createTestNode(t, db, admin.ID, "State With Title", "state-with-title", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	proposalID := seedStaleProposal(t, db, nodeID, admin.ID, "open", "voting", nil)

	r := authedRequest("PATCH", "/api/v1/proposals/"+proposalID,
		map[string]interface{}{"title": "Renamed on the way through", "state": "voting"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/proposals/{id}", handler.UpdateProposal(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var title string
	db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&title)
	if title != "Stale Proposal" {
		t.Errorf("expected the title to be untouched, got %q", title)
	}
}

// The guard must not cost the endpoint its actual job.
func TestUpdateProposal_TitleAndBodyStillUpdate(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "titleupdater", "member")
	nodeID := createTestNode(t, db, admin.ID, "Title Updater", "title-updater", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	proposalID := seedStaleProposal(t, db, nodeID, admin.ID, "open", "voting", nil)

	r := authedRequest("PATCH", "/api/v1/proposals/"+proposalID,
		map[string]interface{}{"title": "Revised title", "body": "Revised body"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/proposals/{id}", handler.UpdateProposal(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var title, body string
	db.QueryRow("SELECT title, body FROM proposals WHERE id = ?", proposalID).Scan(&title, &body)
	if title != "Revised title" || body != "Revised body" {
		t.Errorf("expected the edit to land, got title=%q body=%q", title, body)
	}
	if got := stateOf(t, db, proposalID); got != "voting" {
		t.Errorf("expected state to stay voting, got %q", got)
	}
}
