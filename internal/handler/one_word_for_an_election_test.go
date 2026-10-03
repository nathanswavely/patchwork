package handler_test

import (
	"strings"
	"testing"
)

// F-107. The product had four names for one event, and a new election's own
// body supplied one of them.
//
// "Nominations open first, then the ballot" named the voting phase after the
// thing a voter casts in it, beside the hub's "contest", the proposal's
// "Council election" and the panel's heading. Devon: "Four words for one
// event. It took me a few minutes of clicking between the pages to be sure
// they were all the same election and not four different ones I was somehow
// behind on."
//
// The body is written once, when the calendar opens the election, and stored
// on the proposal — so an election already on a patch keeps the words it was
// created with, which is right: the record is not rewritten behind a
// community's back. This holds the words the next one gets.
func TestElectionBody_NamesThePhasesTheBellNames(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, _ := createTestUser(t, db, "phases", "member")
	nodeID := electedNode(t, db, admin.ID, "Phases", "phases", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)

	var body string
	db.QueryRow(`SELECT body FROM proposals WHERE id = ?`, id).Scan(&body)
	if !strings.Contains(body, "Nominations open first, then voting.") {
		t.Errorf("a new election does not name its phases the way the bell does: %q", body)
	}
	if strings.Contains(body, "then the ballot") {
		t.Errorf("a new election still names the voting phase after the ballot: %q", body)
	}
}
