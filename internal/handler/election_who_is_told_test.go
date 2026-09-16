package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Whose contest it is, and who hears how it ended (docs/adr/109).
//
// A contest the calendar opened wore the longest-standing admin's name, and
// the comments members left on it rang that person's bell. And a contest that
// seated a council told the winner and nobody else — not the other candidate,
// not the people who voted.

func notifiedTitles(t *testing.T, db *database.DB, userID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT title FROM notifications WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		t.Fatalf("read notifications: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

// heardAbout waits for a notification to reach somebody. Delivery is
// asynchronous, so a bare read after the act finds an empty table and proves
// nothing either way.
func heardAbout(t *testing.T, db *database.DB, userID, prefix string) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		for _, title := range notifiedTitles(t, db, userID) {
			if strings.HasPrefix(title, prefix) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// quietFor is the same wait spent proving a notification never arrives.
func quietFor(t *testing.T, db *database.DB, userID, prefix string) bool {
	t.Helper()
	time.Sleep(750 * time.Millisecond)
	for _, title := range notifiedTitles(t, db, userID) {
		if strings.HasPrefix(title, prefix) {
			return false
		}
	}
	return true
}

// The calendar signs its own contests, and a comment on one rings nobody's
// bell for being its author.
func TestElectionAuthor_IsTheCalendarAndNotAMember(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, _ := createTestUser(t, db, "auth-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Authored", "authored", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	talker, talkerToken := createTestUser(t, db, "auth-talker", "member")
	createTestMembership(t, db, talker.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)

	var authorID string
	db.QueryRow(`SELECT author_id FROM proposals WHERE id = ?`, id).Scan(&authorID)
	if authorID != model.SystemUserID {
		t.Errorf("a contest nobody raised is signed by nobody, got %q", authorID)
	}

	// Somebody comments on it. The admin is not its author any more, so this
	// is not their business to be told about.
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/comments",
		map[string]interface{}{"body": "Who is standing?"}, talkerToken)
	if code := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments",
		handler.CreateComment(db), r).Code; code != http.StatusCreated {
		t.Fatalf("comment: expected 201, got %d", code)
	}

	if !quietFor(t, db, admin.ID, "New comment on:") {
		t.Error("the longest-standing admin is not the author of the calendar's contest and should not be told about comments on it")
	}
	if got := notifiedTitles(t, db, model.SystemUserID); len(got) != 0 {
		t.Errorf("the sentinel user reads nothing, so nothing should be written to it: %v", got)
	}
}

// Somebody who voted in a contest is a participant in it.
func TestElection_ABallotMakesYouAParticipant(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "part-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Participants", "participants", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	voter, voterToken := createTestUser(t, db, "part-voter", "member")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, adminToken, "")
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	cand := candidateIDFor(t, db, id, admin.ID)
	if code := castApprovals(t, db, id, voterToken, []string{cand}); code != http.StatusOK {
		t.Fatalf("ballot: expected 200, got %d", code)
	}

	// An election's ballots are their own table, so this voter used to be
	// invisible to the participants audience.
	r := authedRequest("POST", "/api/v1/proposals/"+id+"/comments",
		map[string]interface{}{"body": "A word about the slate."}, adminToken)
	if code := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments",
		handler.CreateComment(db), r).Code; code != http.StatusCreated {
		t.Fatalf("comment: expected 201, got %d", code)
	}

	if !heardAbout(t, db, voter.ID, "New comment on:") {
		t.Error("somebody who cast a ballot is in the conversation about it")
	}
}

// A contest that settles something says so to the patch, not only to whoever
// won.
func TestElection_SeatingTellsThePatchAndNotOnlyTheWinner(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	// The sitting admin, who holds the one chair and is not standing again.
	founder, _ := createTestUser(t, db, "told-founder", "member")
	nodeID := electedNode(t, db, founder.ID, "Told", "told", 0, 12)
	createTestMembership(t, db, founder.ID, nodeID, "admin", "active")

	challenger, challengerToken := createTestUser(t, db, "told-challenger", "member")
	createTestMembership(t, db, challenger.ID, nodeID, "member", "active")
	loser, _ := createTestUser(t, db, "told-loser", "member")
	createTestMembership(t, db, loser.ID, nodeID, "member", "active")
	voter, voterToken := createTestUser(t, db, "told-voter", "member")
	createTestMembership(t, db, voter.ID, nodeID, "member", "active")

	id := openElection(t, db, nodeID)
	standFor(t, db, id, challengerToken, "")
	standFor(t, db, id, challengerToken, loser.ID)
	closeNominations(t, db, id)
	handler.OpenElectionVoting(db, id)
	castApprovals(t, db, id, voterToken, []string{candidateIDFor(t, db, id, challenger.ID)})
	expireProposal(t, db, id)
	handler.SweepElections(db)

	closed := "The election in Told has closed"
	for who, userID := range map[string]string{
		"the candidate who was not seated": loser.ID,
		"somebody who voted":               voter.ID,
	} {
		if !heardAbout(t, db, userID, closed) {
			t.Errorf("%s was told nothing about the result: %v", who, notifiedTitles(t, db, userID))
		}
	}

	// The personal notices were always right and stay: the winner is told they
	// hold a seat, and the admin the electorate did not return is told theirs
	// has ended.
	if !heardAbout(t, db, challenger.ID, "You were elected to the council") {
		t.Errorf("the winner should be told they hold a seat: %v", notifiedTitles(t, db, challenger.ID))
	}
	if !heardAbout(t, db, founder.ID, "Your seat on the council") {
		t.Errorf("an admin who was not returned should be told: %v", notifiedTitles(t, db, founder.ID))
	}
	if n := countNotifications(t, db, challenger.ID, notifications.MembershipRoleChanged, 1); n != 1 {
		t.Errorf("expected exactly one role notice for the winner, got %d", n)
	}
}
