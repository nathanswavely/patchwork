package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// openProposal inserts an open proposal directly, so the overview tests don't
// depend on who may author one.
func openProposal(t *testing.T, db *database.DB, nodeID, authorID, title string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, status) VALUES (?, ?, ?, ?, 'open')`,
		id, nodeID, authorID, title,
	)
	if err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	return id
}

func overviewNeedsVote(t *testing.T, db *database.DB, slug, token string) int {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/overview", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	n, ok := result["needs_vote"].(float64)
	if !ok {
		t.Fatalf("needs_vote missing or not a number in %v", result)
	}
	return int(n)
}

// The governance hub's "N proposals need your vote" banner must count only
// proposals the viewer may actually vote on. The electorate is one set
// (docs/adr/044): a nudge aimed at someone VoteOnProposal will answer with 403
// is a dead end, not a prompt.
func TestGovernanceOverview_NeedsVoteFollowsElectorate(t *testing.T) {
	t.Run("follower is not nudged", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "ov_admin_f", "member")
		follower, followerToken := createTestUser(t, db, "ov_follower", "member")
		nodeID := createTestNode(t, db, admin.ID, "Overview F", "overview-f", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
		openProposal(t, db, nodeID, admin.ID, "Follower sees this")

		if got := overviewNeedsVote(t, db, "overview-f", followerToken); got != 0 {
			t.Errorf("follower needs_vote = %d, want 0 (a follower may not vote)", got)
		}
	})

	t.Run("member with an unvoted open proposal is nudged", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "ov_admin_m", "member")
		member, memberToken := createTestUser(t, db, "ov_member", "member")
		nodeID := createTestNode(t, db, admin.ID, "Overview M", "overview-m", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		openProposal(t, db, nodeID, admin.ID, "Member should vote")

		if got := overviewNeedsVote(t, db, "overview-m", memberToken); got != 1 {
			t.Errorf("member needs_vote = %d, want 1", got)
		}
	})

	t.Run("member inside the tenure window is not nudged", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "ov_admin_t", "member")
		newMember, newMemberToken := createTestUser(t, db, "ov_new_member", "member")
		nodeID := createTestNode(t, db, admin.ID, "Overview T", "overview-t", "open")
		predateNode(t, db, nodeID) // the tenure rule applies in full (docs/adr/098)
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, newMember.ID, nodeID, "member", "active")
		if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
			`{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":30}`, nodeID); err != nil {
			t.Fatalf("set governance config: %v", err)
		}
		openProposal(t, db, nodeID, admin.ID, "Too soon to vote")

		// The member joined just now, so they are 30 days short.
		if got := overviewNeedsVote(t, db, "overview-t", newMemberToken); got != 0 {
			t.Errorf("new member needs_vote = %d, want 0 (tenure not met)", got)
		}

		// Backdate past the requirement and the same proposal starts counting.
		sixtyDaysAgo := time.Now().UTC().Add(-60 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
		if _, err := db.Exec("UPDATE memberships SET joined_at = ? WHERE user_id = ? AND node_id = ?",
			sixtyDaysAgo, newMember.ID, nodeID); err != nil {
			t.Fatalf("backdate membership: %v", err)
		}
		if got := overviewNeedsVote(t, db, "overview-t", newMemberToken); got != 1 {
			t.Errorf("tenured member needs_vote = %d, want 1", got)
		}
	})

	t.Run("a cast ballot still clears the nudge", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "ov_admin_v", "member")
		member, memberToken := createTestUser(t, db, "ov_voted_member", "member")
		nodeID := createTestNode(t, db, admin.ID, "Overview V", "overview-v", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		proposalID := openProposal(t, db, nodeID, admin.ID, "Already decided")
		if _, err := db.Exec(`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
			auth.NewUUIDv7(), proposalID, member.ID); err != nil {
			t.Fatalf("insert vote: %v", err)
		}

		if got := overviewNeedsVote(t, db, "overview-v", memberToken); got != 0 {
			t.Errorf("voted member needs_vote = %d, want 0", got)
		}
	})
}

// A proposal about you is never one the hub says needs your vote (F-042).
// A member who stood for a seat was told "2 proposals need your vote" with
// one of them their own candidacy. Where the patch recuses its subject the
// gate refuses that ballot, so the nudge must not name it; where it does not,
// the subject may vote, but the hub still does not chase them onto the
// council. Either way the proposal stays in the list, and the exception is
// the one recusal already makes: if nobody else could vote, it needs them.
func TestGovernanceOverview_NeedsVoteSkipsYourOwnCandidacy(t *testing.T) {
	setup := func(t *testing.T, slug string, recusal bool) (db *database.DB, nodeID, candidateID, candidateToken string) {
		t.Helper()
		db = setupTestDB(t)
		admin, _ := createTestUser(t, db, slug+"_admin", "member")
		candidate, candidateToken := createTestUser(t, db, slug+"_cand", "member")
		nodeID = createTestNode(t, db, admin.ID, "Overview "+slug, slug, "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, candidate.ID, nodeID, "member", "active")
		gc := `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0,"subject_recusal":false}`
		if recusal {
			gc = `{"decision_method":"majority","quorum_percent":0,"min_voting_tenure_days":0,"subject_recusal":true}`
		}
		if _, err := db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, gc, nodeID); err != nil {
			t.Fatalf("set governance config: %v", err)
		}
		openProposal(t, db, nodeID, admin.ID, "An ordinary question")
		candidacy := openProposal(t, db, nodeID, candidate.ID, "Standing for admin")
		if _, err := db.Exec(`UPDATE proposals SET proposal_type = 'membership', target_user_id = ? WHERE id = ?`,
			candidate.ID, candidacy); err != nil {
			t.Fatalf("make the candidacy: %v", err)
		}
		return db, nodeID, candidate.ID, candidateToken
	}

	for _, recusal := range []bool{true, false} {
		name := "recusal off"
		slug := "own-cand-off"
		if recusal {
			name, slug = "recusal on", "own-cand-on"
		}
		t.Run(name, func(t *testing.T) {
			db, _, _, tok := setup(t, slug, recusal)
			if got := overviewNeedsVote(t, db, slug, tok); got != 1 {
				t.Errorf("needs_vote = %d, want 1: the ordinary question, not the candidate's own candidacy", got)
			}
			if got, _ := awaitingOnList(t, db, slug, tok); got != 1 {
				t.Errorf("awaiting_your_vote = %v, want 1: the dashboard reads the same count", got)
			}
			// The candidacy is still there to read.
			r := authedRequest("GET", "/api/v1/nodes/"+slug+"/proposals?status=open", nil, tok)
			w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
			if w.Code != http.StatusOK {
				t.Fatalf("proposals: got %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "Standing for admin") {
				t.Errorf("the candidacy dropped out of the proposals list: %s", w.Body.String())
			}
		})
	}

	t.Run("the subject is the only one who could vote", func(t *testing.T) {
		db, nodeID, candidateID, tok := setup(t, "own-cand-sole", false)
		// Take the admin's ballot away: demoted to follower, the candidate is
		// the whole electorate, and the candidacy cannot pass without them.
		if _, err := db.Exec(`UPDATE memberships SET role = 'follower' WHERE node_id = ? AND user_id != ?`,
			nodeID, candidateID); err != nil {
			t.Fatalf("demote: %v", err)
		}
		if got := overviewNeedsVote(t, db, "own-cand-sole", tok); got != 2 {
			t.Errorf("needs_vote = %d, want 2: nobody else can vote on the candidacy", got)
		}
	})
}
