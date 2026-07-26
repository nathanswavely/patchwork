package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// backdateMembership ages a membership so it clears min_voting_tenure_days.
func backdateMembership(t *testing.T, db *database.DB, userID, nodeID string, days int) {
	t.Helper()
	when := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec("UPDATE memberships SET joined_at = ? WHERE user_id = ? AND node_id = ?", when, userID, nodeID); err != nil {
		t.Fatalf("backdate membership: %v", err)
	}
}

// closedProposal inserts an open proposal whose voting window has already
// expired, so the next read resolves it.
func closedProposal(t *testing.T, db *database.DB, nodeID, authorID, title string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	ended := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, status, voting_ends_at) VALUES (?, ?, ?, ?, 'open', ?)`,
		id, nodeID, authorID, title, ended,
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	return id
}

func castBallot(t *testing.T, db *database.DB, proposalID, userID, value string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), proposalID, userID, value,
	); err != nil {
		t.Fatalf("insert vote: %v", err)
	}
}

// readProposalStatus fetches a proposal, which resolves it if its window has
// expired.
func readProposalStatus(t *testing.T, db *database.DB, proposalID, token string) string {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, token)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("get proposal: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	status, _ := result["status"].(string)
	return status
}

// Quorum divides by the electorate, not by everyone wearing a member role.
// Counting people who may not cast a ballot made quorum unreachable: on the
// shipped Formal defaults (quorum 50%, tenure 30 days), a patch where most
// members joined inside the tenure window could never resolve a proposal.
func TestResolveProposal_QuorumDividesByElectorate(t *testing.T) {
	t.Run("under-tenure members do not inflate the denominator", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "quorum_admin", "member")
		nodeID := createTestNode(t, db, admin.ID, "Quorum Node", "quorum-node", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		backdateMembership(t, db, admin.ID, nodeID, 60)

		// Three members who joined just now — role-eligible, tenure-short.
		for _, name := range []string{"quorum_new1", "quorum_new2", "quorum_new3"} {
			u, _ := createTestUser(t, db, name, "member")
			createTestMembership(t, db, u.ID, nodeID, "member", "active")
		}

		// Formal-style rules: half the electorate must turn out.
		if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
			`{"decision_method":"majority","quorum_percent":50,"min_voting_tenure_days":30,"amendment_threshold":"majority","amendment_auto_apply":true}`,
			nodeID); err != nil {
			t.Fatalf("set governance config: %v", err)
		}

		// The one person who may vote votes. That is a turnout of 1 out of an
		// electorate of 1 — but of 1 out of 4 if the denominator counts people
		// the gate refuses.
		proposalID := closedProposal(t, db, nodeID, admin.ID, "Needs quorum")
		castBallot(t, db, proposalID, admin.ID, "approve")

		if got := readProposalStatus(t, db, proposalID, adminToken); got != "approved" {
			t.Errorf("status = %q, want %q — the whole electorate voted, so quorum is met", got, "approved")
		}
	})

	t.Run("genuine turnout shortfall still fails quorum", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "quorum2_admin", "member")
		nodeID := createTestNode(t, db, admin.ID, "Quorum2 Node", "quorum2-node", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		backdateMembership(t, db, admin.ID, nodeID, 60)

		// Three members who are all past the tenure requirement, so the
		// electorate really is four and one ballot really is 25%.
		for _, name := range []string{"quorum2_old1", "quorum2_old2", "quorum2_old3"} {
			u, _ := createTestUser(t, db, name, "member")
			createTestMembership(t, db, u.ID, nodeID, "member", "active")
			backdateMembership(t, db, u.ID, nodeID, 60)
		}

		if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
			`{"decision_method":"majority","quorum_percent":50,"min_voting_tenure_days":30,"amendment_threshold":"majority","amendment_auto_apply":true}`,
			nodeID); err != nil {
			t.Fatalf("set governance config: %v", err)
		}

		proposalID := closedProposal(t, db, nodeID, admin.ID, "Short of quorum")
		castBallot(t, db, proposalID, admin.ID, "approve")

		if got := readProposalStatus(t, db, proposalID, adminToken); got != "open" {
			t.Errorf("status = %q, want %q — one of four is below a 50%% quorum", got, "open")
		}
	})
}

// The proposal page must not offer a vote the server will refuse. `can_vote`
// carries the electorate's own answer rather than leaving the client to infer
// one from membership_role, which cannot see min_voting_tenure_days.
func TestGetProposal_CanVoteFollowsElectorate(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "cv_admin", "member")
	member, memberToken := createTestUser(t, db, "cv_member", "member")
	newMember, newMemberToken := createTestUser(t, db, "cv_new_member", "member")
	follower, followerToken := createTestUser(t, db, "cv_follower", "member")
	nodeID := createTestNode(t, db, admin.ID, "CanVote Node", "canvote-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, newMember.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	backdateMembership(t, db, admin.ID, nodeID, 60)
	backdateMembership(t, db, member.ID, nodeID, 60)

	if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`, nodeID); err != nil {
		t.Fatalf("set governance config: %v", err)
	}
	proposalID := openProposal(t, db, nodeID, admin.ID, "Who may vote")

	canVote := func(token string) interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, token)
		w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("get proposal: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)["can_vote"]
	}

	cases := []struct {
		who   string
		token string
		want  bool
	}{
		{"tenured member", memberToken, true},
		{"admin", adminToken, true},
		{"member inside the tenure window", newMemberToken, false},
		{"follower", followerToken, false},
		{"signed-out visitor", "", false},
	}
	for _, c := range cases {
		if got := canVote(c.token); got != c.want {
			t.Errorf("can_vote for %s = %v, want %v", c.who, got, c.want)
		}
	}
}
