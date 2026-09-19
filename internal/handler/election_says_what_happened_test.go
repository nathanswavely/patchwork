package handler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// What a contest says about itself (docs/adr/106).
//
// Two sentences the product was getting wrong, both found by people rather
// than by tests. "The council continues until a successor is elected" was
// printed to a co-op that had no council at all, five times over a year; and
// a contest nobody stood in opened a ballot anyway and called the whole
// electorate to an empty page.

// tellingPeople wires the real notifier for a test, since these assertions
// are about what a person is told.
func tellingPeople(t *testing.T, db *database.DB) {
	t.Helper()
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
}

// unsettledBodyFor is what the members were told when a contest settled
// nothing. Delivery is asynchronous, so this waits for it the way the other
// notification assertions in this package do.
func unsettledBodyFor(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	// Notifications carry no node id of their own — the patch's name is in
	// the title, which is what a person reads.
	var name string
	db.QueryRow(`SELECT name FROM nodes WHERE id = ?`, nodeID).Scan(&name)
	title := "The election in " + name + " settled nothing"
	deadline := time.Now().Add(2 * time.Second)
	for {
		var body string
		db.QueryRow(`SELECT COALESCE(body,'') FROM notifications WHERE title = ?
		             ORDER BY created_at DESC LIMIT 1`, title).Scan(&body)
		if body != "" || time.Now().After(deadline) {
			return body
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// A contest nobody stood in settles when nominations close, rather than
// opening a ballot and running a fortnight to reach the same answer.
func TestElection_EmptySlateSettlesWhenNominationsClose(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, _ := createTestUser(t, db, "emptyslate", "member")
	nodeID := electedNode(t, db, admin.ID, "Empty Slate", "empty-slate", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for i, name := range []string{"es-a", "es-b", "es-c"} {
		u, _ := createTestUser(t, db, name, "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
		_ = i
	}

	id := openElection(t, db, nodeID)
	closeNominations(t, db, id)

	if handler.OpenElectionVoting(db, id) {
		t.Error("a ballot must not open over a slate with nobody on it")
	}

	var status, state, votingEnds string
	db.QueryRow(`SELECT status, COALESCE(state,''), COALESCE(voting_ends_at,'') FROM proposals WHERE id = ?`, id).
		Scan(&status, &state, &votingEnds)
	if state != "unsettled" {
		t.Errorf("expected the contest settled as unsettled, got state %q (status %q)", state, status)
	}
	if votingEnds != "" {
		t.Errorf("no ballot ran, so nothing should have a closing time: %q", votingEnds)
	}

	// Nobody was invited to vote. This is the notification that reached a
	// simulated candidate over a contest with no names on it, and the one he
	// said would have sent him away for good.
	time.Sleep(250 * time.Millisecond)
	var called int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE type = ?`,
		string(notifications.ProposalVoting)).Scan(&called)
	if called != 0 {
		t.Errorf("expected nobody called to an empty ballot, got %d notified", called)
	}
}

// The sentence about the council has to be true of the council.
func TestElection_UnsettledSaysWhetherAnybodyIsStillHoldingTheSeats(t *testing.T) {
	// A council still sitting: holdover is the reassuring, true thing.
	t.Run("with a sitting council", func(t *testing.T) {
		db := setupTestDB(t)
		tellingPeople(t, db)
		admin, _ := createTestUser(t, db, "holdover-sitting", "member")
		nodeID := electedNode(t, db, admin.ID, "Sitting", "sitting", 0, 12)
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		seedSeatFor(t, db, nodeID, admin.ID, time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02"))

		id := openElection(t, db, nodeID)
		closeNominations(t, db, id)
		handler.SweepElections(db)

		body := unsettledBodyFor(t, db, nodeID)
		if !strings.Contains(body, "The council continues until a successor is elected.") {
			t.Errorf("a sitting council holds over and should be told so, got %q", body)
		}
	})

	// No council at all: the same sentence is a comfortable lie, and a
	// simulated member stopped reading the page over it.
	t.Run("with every chair empty", func(t *testing.T) {
		db := setupTestDB(t)
		tellingPeople(t, db)
		admin, _ := createTestUser(t, db, "holdover-empty", "member")
		nodeID := electedNode(t, db, admin.ID, "Empty", "empty", 0, 12)
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
		makeVacantSeat(t, db, nodeID, past)
		makeVacantSeat(t, db, nodeID, past)
		db.Exec(`UPDATE memberships SET role = 'member' WHERE node_id = ?`, nodeID)

		id := openElection(t, db, nodeID)
		closeNominations(t, db, id)
		handler.SweepElections(db)

		body := unsettledBodyFor(t, db, nodeID)
		if strings.Contains(body, "The council continues") {
			t.Errorf("there is no council to continue, and the notice said there was: %q", body)
		}
		if !strings.Contains(body, "Nobody was elected") || !strings.Contains(body, "no admins") {
			t.Errorf("expected the empty council said out loud, got %q", body)
		}
		if !strings.Contains(body, "all 2 seats") {
			t.Errorf("expected the empty chairs counted, got %q", body)
		}
	})
}
