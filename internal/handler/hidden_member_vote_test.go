package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// A hidden membership is not named outside the room (docs/adr/006), and the
// public voter list was naming it. Only members vote, so every row in that
// list published the one fact the member's switch took down — and across a
// patch's proposals the list reassembles the roster from the other end.
//
// The fix substitutes rather than filters. These tests pin both halves: the
// name is gone, and the row is not, because ProposalDetail.svelte reads an
// empty voter list as "no vote ever happened".

type hiddenVoteFixture struct {
	nodeID, proposalID          string
	hiddenID, visibleID         string
	memberToken, followerToken  string
	strangerToken, siteToken    string
	hiddenName, visibleUsername string
}

func seedHiddenVote(t *testing.T, db *database.DB, slug string) hiddenVoteFixture {
	t.Helper()
	owner, _ := createTestUser(t, db, slug+"-owner", "member")
	hidden, _ := createTestUser(t, db, slug+"-hidden", "member")
	visible, _ := createTestUser(t, db, slug+"-visible", "member")
	roomMate, memberToken := createTestUser(t, db, slug+"-roommate", "member")
	follower, followerToken := createTestUser(t, db, slug+"-follower", "member")
	_, strangerToken := createTestUser(t, db, slug+"-stranger", "member")
	_, siteToken := createTestUser(t, db, slug+"-site", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Hidden Vote "+slug, slug, "open")
	// The subject here is what a *public* voter list does and does not name,
	// so the patch has to be one that publishes its record at all
	// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md). On a
	// closed patch every assertion below would pass for the wrong reason:
	// nobody is named because nobody outside the room reads anything.
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, hidden.ID, nodeID, "member", "active")
	createTestMembership(t, db, visible.ID, nodeID, "member", "active")
	createTestMembership(t, db, roomMate.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	// The one switch ADR 006 gives the member, on their own membership.
	if _, err := db.Exec(
		`UPDATE memberships SET visible = 0 WHERE node_id = ? AND user_id = ?`, nodeID, hidden.ID,
	); err != nil {
		t.Fatalf("hide membership: %v", err)
	}

	proposalID := createTestProposal(t, db, nodeID, owner.ID)
	for _, v := range []struct{ user, value string }{
		{hidden.ID, "approve"},
		{visible.ID, "reject"},
	} {
		if _, err := db.Exec(
			`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, ?)`,
			auth.NewUUIDv7(), proposalID, v.user, v.value,
		); err != nil {
			t.Fatalf("cast vote: %v", err)
		}
	}

	return hiddenVoteFixture{
		nodeID: nodeID, proposalID: proposalID,
		hiddenID: hidden.ID, visibleID: visible.ID,
		memberToken: memberToken, followerToken: followerToken,
		strangerToken: strangerToken, siteToken: siteToken,
		hiddenName: slug + "-hidden", visibleUsername: slug + "-visible",
	}
}

type voterRow struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Value       string `json:"value"`
	Counted     bool   `json:"counted"`
}

// readVoters fetches the proposal as the holder of token ("" signed out) and
// returns its voter list. AuthOptional, exactly as main.go mounts the route.
func readVoters(t *testing.T, db *database.DB, proposalID, token string) []voterRow {
	t.Helper()
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, token)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/proposals/{id}", middleware.AuthOptional(db, handler.GetProposal(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Voters []voterRow `json:"voters"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Voters
}

func findVote(voters []voterRow, value string) (voterRow, bool) {
	for _, v := range voters {
		if v.Value == value {
			return v, true
		}
	}
	return voterRow{}, false
}

func TestHiddenMemberIsNotNamedInThePublicVoterList(t *testing.T) {
	db := setupTestDB(t)
	f := seedHiddenVote(t, db, "hvpublic")

	voters := readVoters(t, db, f.proposalID, "")

	// The record keeps its shape: both ballots, both values.
	if len(voters) != 2 {
		t.Fatalf("expected both ballots to survive, got %d: %+v", len(voters), voters)
	}

	hiddenVote, ok := findVote(voters, "approve")
	if !ok {
		t.Fatal("the hidden member's ballot vanished; it should be present but unnamed")
	}
	if hiddenVote.DisplayName != handler.HiddenMemberName {
		t.Errorf("expected %q, got %q", handler.HiddenMemberName, hiddenVote.DisplayName)
	}
	if hiddenVote.Username != "" {
		t.Errorf("username leaked: %q — it is what a profile link is built from", hiddenVote.Username)
	}
	// The id goes with the name, or one anonymous ballot links to another
	// across every proposal this patch has run.
	if hiddenVote.UserID != "" {
		t.Errorf("user_id leaked: %q — it correlates ballots across proposals", hiddenVote.UserID)
	}

	// A member who did not hide is unaffected.
	shownVote, ok := findVote(voters, "reject")
	if !ok {
		t.Fatal("the visible member's ballot vanished")
	}
	if shownVote.Username != f.visibleUsername {
		t.Errorf("expected the visible member named %q, got %q", f.visibleUsername, shownVote.Username)
	}
	if shownVote.UserID != f.visibleID {
		t.Error("a visible member lost their user_id; only a hidden one should")
	}
}

// ADR 006: hidden means hidden from outside, not from the patch.
func TestHiddenMemberIsStillNamedInsideTheRoom(t *testing.T) {
	db := setupTestDB(t)
	f := seedHiddenVote(t, db, "hvroom")

	for _, c := range []struct{ who, token string }{
		{"a member of the patch", f.memberToken},
		{"an instance admin", f.siteToken},
	} {
		hiddenVote, ok := findVote(readVoters(t, db, f.proposalID, c.token), "approve")
		if !ok {
			t.Fatalf("%s: ballot missing", c.who)
		}
		if hiddenVote.DisplayName == handler.HiddenMemberName {
			t.Errorf("%s should see the real name, got the substitute", c.who)
		}
		if hiddenVote.UserID != f.hiddenID {
			t.Errorf("%s should see the real user_id", c.who)
		}
	}
}

// A follower is an observer, not a member — the people in a patch are not
// theirs to enumerate. Same for any other signed-in stranger.
func TestHiddenMemberIsNotNamedToFollowersOrStrangers(t *testing.T) {
	db := setupTestDB(t)
	f := seedHiddenVote(t, db, "hvoutside")

	for _, c := range []struct{ who, token string }{
		{"a follower", f.followerToken},
		{"a signed-in stranger", f.strangerToken},
	} {
		hiddenVote, ok := findVote(readVoters(t, db, f.proposalID, c.token), "approve")
		if !ok {
			t.Fatalf("%s: ballot missing", c.who)
		}
		if hiddenVote.DisplayName != handler.HiddenMemberName {
			t.Errorf("%s saw a hidden member named: %q", c.who, hiddenVote.DisplayName)
		}
	}
}

// The reason this substitutes instead of filtering. ProposalDetail.svelte
// reads an empty voter list as "no vote ever happened" to recognise a direct
// change (docs/adr/041); drop the row and a proposal that was voted on and
// passed would claim it never was.
func TestWithholdingANameDoesNotEraseTheVote(t *testing.T) {
	db := setupTestDB(t)
	f := seedHiddenVote(t, db, "hvrecord")

	// Hide every voter, which is the worst case for the inference.
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE node_id = ?`, f.nodeID); err != nil {
		t.Fatalf("hide all: %v", err)
	}

	voters := readVoters(t, db, f.proposalID, "")
	if len(voters) != 2 {
		t.Fatalf("a vote that happened must not read as a vote that did not: got %d rows", len(voters))
	}
	for _, v := range voters {
		if v.DisplayName != handler.HiddenMemberName {
			t.Errorf("expected every row substituted, got %q", v.DisplayName)
		}
		if v.Value == "" {
			t.Error("the ballot's value is the record and must survive the substitution")
		}
	}
}

// A voter with no membership row has no membership to hide: they left, and
// ADR 006 governs a switch on a row that exists. Pinned because the
// COALESCE that produces it would read as an oversight otherwise.
func TestADepartedVoterIsNotTreatedAsHidden(t *testing.T) {
	db := setupTestDB(t)
	f := seedHiddenVote(t, db, "hvdeparted")

	if _, err := db.Exec(
		`DELETE FROM memberships WHERE node_id = ? AND user_id = ?`, f.nodeID, f.visibleID,
	); err != nil {
		t.Fatalf("remove membership: %v", err)
	}

	shownVote, ok := findVote(readVoters(t, db, f.proposalID, ""), "reject")
	if !ok {
		t.Fatal("the departed voter's ballot vanished")
	}
	if shownVote.Username != f.visibleUsername {
		t.Errorf("a departed voter should stay named, got %q", shownVote.Username)
	}
}
