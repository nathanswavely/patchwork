package notifications

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
)

// You cannot miss a meeting that never happened
// (docs/adr/2026-10-19-you-cannot-miss-a-meeting-that-never-happened.md).
//
// A band keeps its calendar here and has never raised a proposal. Its admin
// was warned that the seat was inactive at 90 days, and at 180 every admin it
// had would have been vacated for not governing a patch with nothing to
// govern. Absence is now counted from the first proposal an admin let pass.

func countWarnings(seat string) string {
	return `SELECT COUNT(*) FROM notification_reminders_sent
	        WHERE entity_id = '` + seat + `' AND reminder_type = 'inactivity_warning'`
}

// The case that started this. A year of events, every member an admin, and no
// proposal ever: nobody is warned and nobody is vacated.
func TestSweep_APatchThatNeverProposesNeverWarns(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "a-band", "longest_tenure", 90)

	var seats []string
	for _, name := range []string{"bandsinger", "banddrummer", "bandbassist"} {
		uid := seedPerson(t, db, name)
		seats = append(seats, seedSeat(t, db, uid, nodeID, "admin", 400))
		seedEventBy(t, db, nodeID, uid, 30)
	}

	for day := 0; day < 365; day++ {
		SweepInactiveAdmins(n)
		advanceDays(t, db, 1)
	}

	for _, seat := range seats {
		if got := roleOfSeat(t, db, seat); got != "admin" {
			t.Errorf("an admin of a patch with nothing to govern lost the seat, got %q", got)
		}
		if got := countRows(t, db, countWarnings(seat)); got != 0 {
			t.Errorf("an admin of a patch with nothing to govern was warned %d times", got)
		}
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM notifications`); got != 0 {
		t.Errorf("a year with nothing to govern rang the bell %d times", got)
	}
}

// The clock starts at the proposal, not at the admin's last act. An admin
// last seen in governance 200 days ago, on a patch that raised its first
// proposal since then ten days ago, has been absent for ten days.
func TestSweep_TheClockStartsAtTheProposalNotTheLastAct(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "long-quiet", "longest_tenure", 30)
	admin := seedPerson(t, db, "longquietadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 200)
	seedOccasion(t, db, nodeID, 10)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("ten days after a proposal is not sixty, got %q", got)
	}
	if got := countRows(t, db, countWarnings(seat)); got != 0 {
		t.Errorf("ten days after a proposal is not thirty, warned %d times", got)
	}
}

// The stranded patch the sweep was built for still gets caught: a member asks
// something, the only admin never answers, and the clock runs from the asking.
func TestSweep_AnUnansweredProposalStillCostsTheSeat(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "stranded", "longest_tenure", 30)
	admin := seedPerson(t, db, "strandedadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	asker := seedPerson(t, db, "strandedasker")
	askerSeat := seedSeat(t, db, asker, nodeID, "member", 300)
	seedProposalBy(t, db, nodeID, asker, 61)
	seedEventBy(t, db, nodeID, asker, 3)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "member" {
		t.Errorf("an admin who left a member's proposal unanswered for 61 days keeps the seat, got %q", got)
	}
	// And the member who asked is still around, so the patch is not left with
	// nobody. Asking alone would not have been enough: the proposal is as old
	// as the absence it caused, which puts it just outside the presence
	// window (see the ADR's consequences).
	if got := roleOfSeat(t, db, askerSeat); got != "admin" {
		t.Errorf("expected the member who asked to become an interim admin, got %q", got)
	}
}

// A proposal taken back is no occasion: a proposal raised by mistake and
// pulled a minute later must not start a clock that runs for months.
func TestSweep_AWithdrawnProposalIsNoOccasion(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "withdrawn", "longest_tenure", 30)
	admin := seedPerson(t, db, "withdrawnadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	pid := seedOccasion(t, db, nodeID, 90)
	db.Exec(`UPDATE proposals SET status = 'withdrawn', state = 'withdrawn' WHERE id = ?`, pid)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("a withdrawn proposal cost an admin the seat, got %q", got)
	}
}

// Deciding a proposal on an admin-decides patch (docs/adr/092) is the act the
// proposal was waiting for. An admin who answers every member by deciding,
// without ever voting or commenting, is not absent from what they answered.
func TestSweep_DecidingAProposalIsTakingPart(t *testing.T) {
	for _, tc := range []struct {
		name, set string
	}{
		{"applied", `applied_by = ?, applied_at = ?, state = 'in_effect'`},
		{"declined", `declined_by = ?, updated_at = ?, state = 'rejected'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sweepDB(t)
			n := NewNotifier(db)
			nodeID := seedPatch(t, db, "decides-"+tc.name, "longest_tenure", 30)
			admin := seedPerson(t, db, "decider"+tc.name)
			seat := seedSeat(t, db, admin, nodeID, "admin", 400)
			pid := seedOccasion(t, db, nodeID, 50)
			if _, err := db.Exec(`UPDATE proposals SET `+tc.set+` WHERE id = ?`, admin, ago(49), pid); err != nil {
				t.Fatalf("decide: %v", err)
			}

			SweepInactiveAdmins(n)

			if got := countRows(t, db, countWarnings(seat)); got != 0 {
				t.Errorf("an admin who %s the proposal was warned for letting it pass", tc.name)
			}
		})
	}
}

// An election ballot is a vote. It lives in its own table (docs/adr/051), and
// an admin who voted in a contest and nothing else used to read as absent.
func TestSweep_AnElectionBallotIsTakingPart(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "ballot", "longest_tenure", 30)
	admin := seedPerson(t, db, "balloter")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	contest := seedOccasion(t, db, nodeID, 50)
	candidate := seedPerson(t, db, "ballotcandidate")
	candID := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO election_candidates (id, proposal_id, user_id) VALUES (?, ?, ?)`,
		candID, contest, candidate); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO election_ballots (id, proposal_id, voter_id, candidate_id, created_at)
	                      VALUES (?, ?, ?, ?, ?)`, auth.NewUUIDv7(), contest, admin, candID, ago(49)); err != nil {
		t.Fatalf("seed ballot: %v", err)
	}

	SweepInactiveAdmins(n)

	if got := countRows(t, db, countWarnings(seat)); got != 0 {
		t.Errorf("an admin who cast a ballot was warned for letting the contest pass")
	}
}

// Taking part in one proposal answers for the ones raised before it: the
// admin is back, and the next absence starts at the next proposal.
func TestSweep_OneActAnswersForEverythingBeforeIt(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "caught-up", "longest_tenure", 30)
	admin := seedPerson(t, db, "caughtupadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedOccasion(t, db, nodeID, 100)
	seedOccasion(t, db, nodeID, 80)
	seedProposalBy(t, db, nodeID, admin, 5)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("an admin who took part five days ago lost the seat, got %q", got)
	}
	if got := countRows(t, db, countWarnings(seat)); got != 0 {
		t.Errorf("an admin who took part five days ago was warned")
	}
}
