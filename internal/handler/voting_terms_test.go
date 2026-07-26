package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// setNodeRules replaces a patch's live governance config — the thing an admin
// edit does, and the thing a running vote must stop listening to.
func setNodeRules(t *testing.T, db *database.DB, nodeID, rules string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, rules, nodeID); err != nil {
		t.Fatalf("set node rules: %v", err)
	}
}

// createProposalVia posts a proposal through the handler, so it photographs the
// rules in force the way a real one does.
func createProposalVia(t *testing.T, db *database.DB, slug, token, title string) string {
	t.Helper()
	body := map[string]interface{}{"title": title, "duration_hours": 72}
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/proposals", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create proposal: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)["id"].(string)
}

func expireProposal(t *testing.T, db *database.DB, proposalID string) {
	t.Helper()
	ended := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec("UPDATE proposals SET voting_ends_at = ? WHERE id = ?", ended, proposalID); err != nil {
		t.Fatalf("expire proposal: %v", err)
	}
}

func voteVia(t *testing.T, db *database.DB, proposalID, token, value string) int {
	t.Helper()
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote", map[string]string{"value": value}, token)
	return serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), r).Code
}

// A vote is judged by the rules it opened with (docs/adr/047). An admin editing
// the patch's rules must not redraw a contest people have already voted in.
func TestVotingTerms_RulesEditDoesNotMoveARunningVote(t *testing.T) {
	t.Run("someone eligible at open stays eligible when the bar rises", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "vt_admin1", "member")
		member, memberToken := createTestUser(t, db, "vt_member1", "member")
		nodeID := createTestNode(t, db, admin.ID, "Terms One", "terms-one", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0}`)

		proposalID := createProposalVia(t, db, "terms-one", adminToken, "Opened under no tenure rule")

		// The patch raises the bar after voting opened. The member joined
		// moments ago and would fail the new rule.
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`)

		if code := voteVia(t, db, proposalID, memberToken, "approve"); code != http.StatusOK {
			t.Errorf("vote returned %d, want 200 — this vote opened under a rule the member met", code)
		}
	})

	t.Run("someone eligible only under the new rules waits for the next vote", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "vt_admin2", "member")
		member, memberToken := createTestUser(t, db, "vt_member2", "member")
		nodeID := createTestNode(t, db, admin.ID, "Terms Two", "terms-two", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`)

		proposalID := createProposalVia(t, db, "terms-two", adminToken, "Opened under a 30-day rule")

		// The patch drops the requirement. The running vote keeps its terms.
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0}`)

		if code := voteVia(t, db, proposalID, memberToken, "approve"); code != http.StatusForbidden {
			t.Errorf("vote returned %d, want 403 — this vote opened under a rule the member did not meet", code)
		}

		// ...but a proposal opened after the change admits them.
		nextID := createProposalVia(t, db, "terms-two", adminToken, "Opened after the change")
		if code := voteVia(t, db, nextID, memberToken, "approve"); code != http.StatusOK {
			t.Errorf("vote on the later proposal returned %d, want 200", code)
		}
	})

	t.Run("quorum is measured against the terms the vote opened with", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "vt_admin3", "member")
		nodeID := createTestNode(t, db, admin.ID, "Terms Three", "terms-three", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		for _, name := range []string{"vt_m3a", "vt_m3b", "vt_m3c"} {
			u, _ := createTestUser(t, db, name, "member")
			createTestMembership(t, db, u.ID, nodeID, "member", "active")
		}
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":25,"min_voting_tenure_days":0}`)

		proposalID := createProposalVia(t, db, "terms-three", adminToken, "Opened at 25% quorum")
		if code := voteVia(t, db, proposalID, adminToken, "approve"); code != http.StatusOK {
			t.Fatalf("admin vote returned %d, want 200", code)
		}

		// One of four is 25% — enough under the terms this vote opened with,
		// and nowhere near the bar the patch raises now.
		setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":100,"min_voting_tenure_days":0}`)

		expireProposal(t, db, proposalID)
		if got := readProposalStatus(t, db, proposalID, adminToken); got != "approved" {
			t.Errorf("status = %q, want %q — quorum was met under the vote's own terms", got, "approved")
		}
	})
}

// The auto-apply switch is a safety valve, not a term of the contest: flipping
// it off stops in-flight amendments from applying themselves (docs/adr/047).
func TestVotingTerms_AutoApplyIsReadLive(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "vt_auto_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Terms Auto", "terms-auto", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	setupGovernanceForNode(t, nodeID)

	// Opens with auto-apply on, so the photograph says yes.
	setNodeRules(t, db, nodeID,
		`{"decision_method":"majority","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":true,"min_voting_tenure_days":0}`)

	body := map[string]interface{}{
		"title":          "Amend under a switch that moves",
		"proposal_type":  "amendment",
		"target_doc":     "community-standards.md",
		"proposed_body":  "# Revised\n\nNew text.",
		"duration_hours": 48,
	}
	r := authedRequest("POST", "/api/v1/nodes/terms-auto/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create amendment: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	proposalID := decodeJSON(t, w)["id"].(string)

	// The patch gets nervous and switches auto-apply off mid-vote.
	setNodeRules(t, db, nodeID,
		`{"decision_method":"majority","quorum_percent":0,"amendment_threshold":"majority","amendment_auto_apply":false,"min_voting_tenure_days":0}`)

	if code := voteVia(t, db, proposalID, adminToken, "approve"); code != http.StatusOK {
		t.Fatalf("vote returned %d, want 200", code)
	}
	expireProposal(t, db, proposalID)
	if got := readProposalStatus(t, db, proposalID, adminToken); got != "approved" {
		t.Fatalf("status = %q, want approved", got)
	}

	// Approved, but waiting for a person: the switch was off when it landed.
	var state string
	var appliedAt *string
	db.QueryRow("SELECT COALESCE(state,''), applied_at FROM proposals WHERE id = ?", proposalID).Scan(&state, &appliedAt)
	if state == "in_effect" || appliedAt != nil {
		t.Errorf("state=%q applied_at=%v — auto-apply was switched off before this resolved", state, appliedAt)
	}
}

// The governance hub's count spans proposals with different terms, so it can no
// longer ask one question about the patch (docs/adr/047).
func TestVotingTerms_NeedsVoteAsksPerProposal(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "vt_hub_admin", "member")
	member, memberToken := createTestUser(t, db, "vt_hub_member", "member")
	nodeID := createTestNode(t, db, admin.ID, "Terms Hub", "terms-hub", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	// One proposal opened with no tenure rule — the member may vote on it.
	setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0}`)
	createProposalVia(t, db, "terms-hub", adminToken, "Open to the newcomer")

	// A second opened after the bar rose — the same person is outside it.
	setNodeRules(t, db, nodeID, `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`)
	createProposalVia(t, db, "terms-hub", adminToken, "Closed to the newcomer")

	if got := overviewNeedsVote(t, db, "terms-hub", memberToken); got != 1 {
		t.Errorf("needs_vote = %d, want 1 — eligible for one of the two open votes", got)
	}
	// The admin, who predates neither rule but is an admin, is asked about the
	// one they can actually vote on too.
	if got := overviewNeedsVote(t, db, "terms-hub", adminToken); got != 1 {
		t.Errorf("admin needs_vote = %d, want 1", got)
	}
}
