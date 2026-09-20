package notifications

import (
	"strings"
	"testing"
)

// F-115. Three sentences about losing a seat, and a co-op's only admin could
// not tell which of them was about him.
//
// The bell, four days before he sat down: "You have not taken part in
// governance here for 30 days. Vote, propose, or comment to keep the seat;
// otherwise it is declared vacant after twice that long." The council page:
// "Nobody loses their seat when a term ends." And in between, the hub's own
// description of his leadership model: "Admins inactive for 30 days may be
// asked to step down." Three mechanisms' worth of wording for two mechanisms,
// and every one of them true of something.
//
// Sam: "I read both twice. I still cannot tell you which one is true, and it
// is the single thing I most wanted to know when I sat down. If the page is
// right, I can relax. If the notice is right, our only admin disappears in
// November and the co-op has nobody."
//
// What this holds is the half the page cannot: the warning states the day it
// is warning about, rather than asking a reader to multiply the one number it
// does give. The page's half is in
// web/src/test/when-two-pages-disagree.test.js.

func TestWarning_NamesTheDayTheSeatGoes(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "names-the-day", "longest_tenure", 30)
	admin := seedPerson(t, db, "namesadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 40) // past 30, short of 60
	other := seedPerson(t, db, "namesactive")
	seedSeat(t, db, other, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, other, 1)

	SweepInactiveAdmins(n)

	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		admin, string(GovernanceInactivityWarning)).Scan(&body)
	if body == "" {
		t.Fatalf("no warning reached the admin (seat %s)", seat)
	}
	if !strings.Contains(body, "60 days") {
		t.Errorf("the warning does not say when the seat goes: %q", body)
	}
	if strings.Contains(body, "twice that long") {
		t.Errorf("the warning still asks the reader to multiply: %q", body)
	}
	// The number it is warning from stays, since "you have been quiet for
	// 30 days" is what makes the 60 mean anything.
	if !strings.Contains(body, "30 days") {
		t.Errorf("the warning dropped how long they have been away: %q", body)
	}
}

// A patch with a different rule gets its own two numbers, not the default's.
func TestWarning_TheDaysComeFromThePatchsOwnRule(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "own-rule", "longest_tenure", 45)
	admin := seedPerson(t, db, "ownruleadmin")
	seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 60) // past 45, short of 90
	other := seedPerson(t, db, "ownruleactive")
	seedSeat(t, db, other, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, other, 1)

	SweepInactiveAdmins(n)

	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		admin, string(GovernanceInactivityWarning)).Scan(&body)
	if !strings.Contains(body, "90 days") {
		t.Errorf("expected this patch's own vacate day in %q", body)
	}
}
