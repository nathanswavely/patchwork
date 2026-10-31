package notifications

import (
	"errors"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// An interim admin is asked first
// (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md, F-085). These hold
// what an answer does; inactivity_test.go and inactivity_settles_test.go hold
// that the sweep asks rather than promotes.

// emptiedPatch is a longest-tenure patch whose only admin has just been
// vacated, with present members joined `ages` days ago, oldest first. It runs
// the sweep once and returns the node and the members' user ids.
func emptiedPatch(t *testing.T, db *database.DB, n *Notifier, slug string, ages ...int) (string, []string) {
	t.Helper()
	nodeID := seedPatch(t, db, slug, "longest_tenure", 30)
	gone := seedPerson(t, db, slug+"-gone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)
	var users []string
	for i, age := range ages {
		uid := seedPerson(t, db, slug+"-m"+itoa(i))
		seedSeat(t, db, uid, nodeID, "member", age)
		seedProposalBy(t, db, nodeID, uid, 3)
		users = append(users, uid)
	}
	SweepInactiveAdmins(n)
	return nodeID, users
}

func openOffer(t *testing.T, db *database.DB, nodeID, userID string) bool {
	t.Helper()
	return countRows(t, db, `SELECT COUNT(*) FROM interim_admin_offers
	                         WHERE node_id = ? AND user_id = ? AND answered_at IS NULL`, nodeID, userID) == 1
}

// While the patch still has no admins, a no passes the question to the next
// member who has been around, so it still looks for three.
func TestInterim_ADeclineWhileEmptyPassesItOn(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "pass-on", 400, 300, 200, 100)

	if openOffer(t, db, nodeID, u[3]) {
		t.Fatalf("the fourth should not be asked while three are")
	}
	if err := AnswerInterimOffer(n, nodeID, u[1], false, ""); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if !openOffer(t, db, nodeID, u[3]) {
		t.Errorf("a decline while the council is empty should ask the next present member")
	}
	if openOffer(t, db, nodeID, u[1]) {
		t.Errorf("the person who said no is not asked again")
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("a decline makes nobody an admin, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.interim_declined'`); got != 1 {
		t.Errorf("the decline is on the record once, got %d", got)
	}
}

// Once somebody has said yes the patch is governed, so a later no recruits
// nobody. The questions already out stay answerable.
func TestInterim_ADeclineAfterSomebodyAcceptedRecruitsNobody(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "governed", 400, 300, 200, 100)

	if err := AnswerInterimOffer(n, nodeID, u[0], true, ""); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := AnswerInterimOffer(n, nodeID, u[1], false, ""); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if openOffer(t, db, nodeID, u[3]) {
		t.Errorf("the patch has an admin again, so nobody new should be asked")
	}
	if !openOffer(t, db, nodeID, u[2]) {
		t.Errorf("the third question is still out and still answerable")
	}
	if err := AnswerInterimOffer(n, nodeID, u[2], true, ""); err != nil {
		t.Fatalf("a later accept: %v", err)
	}
	if got := adminsOf(t, db, nodeID); got != 2 {
		t.Errorf("two said yes, so two interim admins, got %d", got)
	}
}

// Saying yes is the act, so the record names the person as its actor.
func TestInterim_AcceptingIsTheirOwnAct(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "own-act", 400)

	if err := AnswerInterimOffer(n, nodeID, u[0], true, "198.51.100.7"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log
	                           WHERE action = 'membership.succession' AND user_id = ?`, u[0]); got != 1 {
		t.Errorf("the step in should be audited with the person as actor, got %d rows", got)
	}
	if err := AnswerInterimOffer(n, nodeID, u[0], true, ""); !errors.Is(err, ErrNoInterimOffer) {
		t.Errorf("an answered offer cannot be answered twice, got %v", err)
	}
}

// When everybody who has been around says no, an instance admin is told: the
// route back that already exists for a patch nobody here can run.
func TestInterim_EveryoneSaysNo(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	seedInstanceAdmin(t, db, "nobodysiteadmin")
	nodeID, u := emptiedPatch(t, db, n, "all-no", 400, 300)

	if err := AnswerInterimOffer(n, nodeID, u[0], false, ""); err != nil {
		t.Fatalf("first decline: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notifications WHERE type = 'governance.succession_needed'`); got != 0 {
		t.Errorf("somebody is still deciding, so no instance admin yet, got %d", got)
	}
	if err := AnswerInterimOffer(n, nodeID, u[1], false, ""); err != nil {
		t.Fatalf("second decline: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notifications WHERE type = 'governance.succession_needed'`); got != 1 {
		t.Errorf("everybody said no, so an instance admin should be told once, got %d", got)
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("nobody said yes, got %d admins", got)
	}
}

// A no stands until the council has been filled and emptied again. Then the
// answered offers are spent, and a later emptying may ask the same people.
func TestInterim_ANoIsSpentWhenTheCouncilFills(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "spent", 400, 300)

	if err := AnswerInterimOffer(n, nodeID, u[1], false, ""); err != nil {
		t.Fatalf("decline: %v", err)
	}
	for i := 0; i < 5; i++ {
		SweepInactiveAdmins(n)
	}
	if openOffer(t, db, nodeID, u[1]) {
		t.Fatalf("a no is not asked again while the same emptying lasts")
	}

	if err := AnswerInterimOffer(n, nodeID, u[0], true, ""); err != nil {
		t.Fatalf("accept: %v", err)
	}
	SweepInactiveAdmins(n)
	if got := countRows(t, db, `SELECT COUNT(*) FROM interim_admin_offers WHERE node_id = ?`, nodeID); got != 0 {
		t.Errorf("a filled council spends the answered offers, got %d left", got)
	}
}

// Somebody who left before answering cannot step in, so their offer closes and
// the question moves on.
func TestInterim_LeavingBeforeAnsweringPassesItOn(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "left", 400, 300, 200, 100)

	if _, err := db.Exec(`UPDATE memberships SET status = 'left' WHERE node_id = ? AND user_id = ?`, nodeID, u[0]); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := AnswerInterimOffer(n, nodeID, u[0], true, ""); !errors.Is(err, ErrNoLongerHere) {
		t.Fatalf("expected ErrNoLongerHere, got %v", err)
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("somebody who left cannot become an admin, got %d", got)
	}
	if !openOffer(t, db, nodeID, u[3]) {
		t.Errorf("their place in the question should pass to the next present member")
	}
}

// No clock answers for anybody. A year of sweeps leaves the question open and
// the patch without admins; nothing is withdrawn and nothing is assumed.
func TestInterim_NoClockAnswers(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "no-clock", 400)

	for day := 0; day < 365; day++ {
		advanceDays(t, db, 1)
		SweepInactiveAdmins(n)
	}
	if !openOffer(t, db, nodeID, u[0]) {
		t.Errorf("an unanswered offer stands, a year on")
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("silence is not a yes, got %d admins", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notifications WHERE type = 'membership.interim_offer'`); got != 1 {
		t.Errorf("the question is put once, not every day, got %d", got)
	}
}

// The people around do say yes on the day, and then go quiet themselves. They
// lose the seats on the same clock anybody would, the patch is told once per
// emptying, and nobody is pulled round in a loop: the property the quiet year
// was written to hold, on the path where somebody answers.
func TestSweep_AQuietYearWhereThePresentSayYes(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID, u := emptiedPatch(t, db, n, "say-yes", 400, 120)
	for _, id := range u {
		if err := AnswerInterimOffer(n, nodeID, id, true, ""); err != nil {
			t.Fatalf("accept: %v", err)
		}
	}
	// A year with nobody acting from here on.
	for day := 0; day < 320; day++ {
		advanceDays(t, db, 1)
		SweepInactiveAdmins(n)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.succession'`); got != 2 {
		t.Errorf("the role moves exactly twice, once per yes, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.seat_vacated'`); got != 3 {
		t.Errorf("the founder and then the two interims, nobody else, got %d", got)
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("expected the patch to settle with no admins, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notifications WHERE type = 'membership.interim_offer'`); got != 2 {
		t.Errorf("each was asked once, and nobody re-asked after lapsing, got %d questions", got)
	}
	// Told once per emptying. The council emptied twice, and the two notices
	// say different things: people were asked, then nobody was left to ask.
	// Grouped by body as well as title, because the same title about two
	// different emptyings is two messages, not one repeated.
	var worst int
	// No rows means nobody was told anything, which is a worst of zero.
	_ = db.QueryRow(`SELECT COUNT(*) AS c FROM notifications GROUP BY user_id, title, body ORDER BY c DESC LIMIT 1`).Scan(&worst)
	if worst > 1 {
		t.Errorf("somebody was told the same thing %d times", worst)
	}
}
