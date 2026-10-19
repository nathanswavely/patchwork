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

// The warning is counted from a proposal, so it names the proposal and links
// to it (docs/adr/2026-10-19-you-cannot-miss-a-meeting-that-never-happened.md).
// "You have not taken part for 90 days" told a band's admin nothing he could
// act on, because there was nothing to take part in; a named proposal is
// something to open and answer.
func TestWarning_NamesTheProposalAndTheDayTheSeatGoes(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "names-the-day", "longest_tenure", 30)
	admin := seedPerson(t, db, "namesadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 50)
	missed := seedTitledOccasion(t, db, nodeID, "Buy a new practice amp", 40) // past 30, short of 60

	SweepInactiveAdmins(n)

	var body, link string
	if err := db.QueryRow(`SELECT body, COALESCE(link,'') FROM notifications WHERE user_id = ? AND type = ?`,
		admin, string(GovernanceInactivityWarning)).Scan(&body, &link); err != nil {
		t.Fatalf("no warning reached the admin (seat %s): %v", seat, err)
	}
	if !strings.Contains(body, "Buy a new practice amp") {
		t.Errorf("the warning does not name the proposal it counts from: %q", body)
	}
	if !strings.HasSuffix(link, "/governance/"+missed) {
		t.Errorf("the warning should open the proposal, links to %q", link)
	}
	// How long it has waited, and how long is left: both counted from the
	// proposal, so neither asks the reader to multiply (F-115).
	if !strings.Contains(body, "40 days ago") {
		t.Errorf("the warning does not say how long the proposal has waited: %q", body)
	}
	if !strings.Contains(body, "vacant in 20 days") {
		t.Errorf("the warning does not say when the seat goes: %q", body)
	}
	if strings.Contains(body, "twice that long") {
		t.Errorf("the warning still asks the reader to multiply: %q", body)
	}
}

// A patch with a different rule gets its own numbers, not the default's.
func TestWarning_TheDaysComeFromThePatchsOwnRule(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "own-rule", "longest_tenure", 45)
	admin := seedPerson(t, db, "ownruleadmin")
	seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 70)
	seedOccasion(t, db, nodeID, 60) // past 45, short of 90

	SweepInactiveAdmins(n)

	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		admin, string(GovernanceInactivityWarning)).Scan(&body)
	if !strings.Contains(body, "vacant in 30 days") {
		t.Errorf("expected this patch's own vacate day in %q", body)
	}
}
