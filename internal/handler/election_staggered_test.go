package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A contest decides the chairs it was opened for, and nothing else
// (docs/adr/103).
//
// Staggering is not an exotic setting: docs/adr/051 put the term end on the
// seat so a council could spread its cohort, and the term-end box
// (docs/adr/100) is how a founder does it. Until this, a contest carried only
// a *count*, so resolving one refilled the council's chairs in created order,
// emptied every chair past the winner count, and stepped down every admin it
// had not seated. On a council with one chair up and two running on, that
// removed two admins the electorate was never asked about.

// seatIDOf is the chair a person holds on this patch, or empty.
func seatIDOf(t *testing.T, db *database.DB, nodeID, holderID string) string {
	t.Helper()
	var id string
	db.QueryRow(`SELECT id FROM seats WHERE node_id = ? AND holder_id = ?`, nodeID, holderID).Scan(&id)
	return id
}

func termEndOf(t *testing.T, db *database.DB, seatID string) string {
	t.Helper()
	var end string
	db.QueryRow(`SELECT COALESCE(term_ends_at,'') FROM seats WHERE id = ?`, seatID).Scan(&end)
	return end
}

func chairsClaimed(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ? AND contested_in IS NOT NULL`, nodeID).Scan(&n)
	return n
}

func chairsOn(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ?`, nodeID).Scan(&n)
	return n
}

// staggeredCouncil is three admins on one patch: one whose term ran out two
// months ago, and two whose terms run for another two years.
func staggeredCouncil(t *testing.T, db *database.DB, prefix string) (nodeID, slug, dueUser, dueToken, safeA, safeB string) {
	t.Helper()
	due, tok := createTestUser(t, db, prefix+"-due", "member")
	slug = "stagger-" + prefix
	nodeID = electedNode(t, db, due.ID, "Stagger "+prefix, slug, 0, 12)
	createTestMembership(t, db, due.ID, nodeID, "admin", "active")

	a, _ := createTestUser(t, db, prefix+"-a", "member")
	createTestMembership(t, db, a.ID, nodeID, "admin", "active")
	b, _ := createTestUser(t, db, prefix+"-b", "member")
	createTestMembership(t, db, b.ID, nodeID, "admin", "active")

	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	far := time.Now().UTC().AddDate(2, 0, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, due.ID, past)
	seedSeatFor(t, db, nodeID, a.ID, far)
	seedSeatFor(t, db, nodeID, b.ID, far)

	return nodeID, slug, due.ID, tok, a.ID, b.ID
}

// The finding itself (F-086): one chair up, a challenger wins it, and the two
// colleagues nobody voted about keep their seats and their role.
func TestElection_StaggeredContestLeavesTheOtherChairsAlone(t *testing.T) {
	db := setupTestDB(t)
	nodeID, _, dueUser, dueToken, safeA, safeB := staggeredCouncil(t, db, "one")

	challenger, challengerToken := createTestUser(t, db, "one-challenger", "member")
	createTestMembership(t, db, challenger.ID, nodeID, "member", "active")

	seatA := seatIDOf(t, db, nodeID, safeA)
	seatB := seatIDOf(t, db, nodeID, safeB)
	seatDue := seatIDOf(t, db, nodeID, dueUser)
	farEnd := termEndOf(t, db, seatA)

	handler.ScheduleDueElections(db)

	var contestID string
	var contested int
	db.QueryRow(`SELECT id, seats_contested FROM proposals
	             WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&contestID, &contested)
	if contestID == "" {
		t.Fatal("expected the overdue chair to open a contest")
	}
	if contested != 1 {
		t.Fatalf("expected one chair contested, got %d", contested)
	}
	if got := chairsClaimed(t, db, nodeID); got != 1 {
		t.Fatalf("expected the contest to claim exactly the overdue chair, got %d claimed", got)
	}
	var claimed string
	db.QueryRow(`SELECT id FROM seats WHERE contested_in = ?`, contestID).Scan(&claimed)
	if claimed != seatDue {
		t.Error("expected the overdue chair contested, got a different one")
	}

	if code := standFor(t, db, contestID, challengerToken, ""); code != http.StatusCreated {
		t.Fatalf("standing: expected 201, got %d", code)
	}
	closeNominations(t, db, contestID)
	handler.OpenElectionVoting(db, contestID)
	cand := candidateIDFor(t, db, contestID, challenger.ID)
	if code := castApprovals(t, db, contestID, dueToken, []string{cand}); code != http.StatusOK {
		t.Fatalf("ballot: expected 200, got %d", code)
	}
	expireProposal(t, db, contestID)
	handler.SweepElections(db)

	// The contested chair changed hands.
	if got := roleOf(t, db, challenger.ID, nodeID); got != "admin" {
		t.Errorf("the winner takes the contested chair: expected admin, got %q", got)
	}
	if got := seatIDOf(t, db, nodeID, challenger.ID); got != seatDue {
		t.Error("the winner sits in the chair that was contested, not in the oldest one")
	}
	if got := roleOf(t, db, dueUser, nodeID); got != "member" {
		t.Errorf("the incumbent the electorate did not return steps down, got %q", got)
	}

	// And the two nobody voted about are untouched — role, chair and calendar.
	for _, who := range []struct{ id, seat, label string }{
		{safeA, seatA, "A"}, {safeB, seatB, "B"},
	} {
		if got := roleOf(t, db, who.id, nodeID); got != "admin" {
			t.Errorf("colleague %s was not on this ballot and must keep the role, got %q", who.label, got)
		}
		if got := seatIDOf(t, db, nodeID, who.id); got != who.seat {
			t.Errorf("colleague %s must keep the chair they were elected to", who.label)
		}
		if got := termEndOf(t, db, who.seat); got != farEnd {
			t.Errorf("colleague %s's term end moved: want %s, got %s", who.label, farEnd, got)
		}
	}

	if got := adminCount(t, db, nodeID); got != 3 {
		t.Errorf("expected a council of three after a one-seat contest, got %d", got)
	}
	if got := chairsOn(t, db, nodeID); got != 3 {
		t.Errorf("expected the council to keep its three chairs, got %d", got)
	}
	if got := chairsClaimed(t, db, nodeID); got != 0 {
		t.Errorf("a settled contest releases its chairs, got %d still claimed", got)
	}
}

// Holdover on a partial contest: nobody stands, and the overdue incumbent
// stays in the chair along with everyone else (docs/adr/051).
func TestElection_UnsettledStaggeredContestReleasesItsChairs(t *testing.T) {
	db := setupTestDB(t)
	nodeID, _, dueUser, _, safeA, _ := staggeredCouncil(t, db, "two")

	handler.ScheduleDueElections(db)
	var contestID string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&contestID)
	if contestID == "" {
		t.Fatal("expected a contest for the overdue chair")
	}

	closeNominations(t, db, contestID)
	handler.OpenElectionVoting(db, contestID)
	expireProposal(t, db, contestID)
	handler.SweepElections(db)

	if got := roleOf(t, db, dueUser, nodeID); got != "admin" {
		t.Errorf("holdover: an unsettled contest leaves the incumbent serving, got %q", got)
	}
	if got := roleOf(t, db, safeA, nodeID); got != "admin" {
		t.Errorf("a colleague not on the ballot keeps the role, got %q", got)
	}
	if got := adminCount(t, db, nodeID); got != 3 {
		t.Errorf("expected the whole council held over, got %d admins", got)
	}
	if got := chairsClaimed(t, db, nodeID); got != 0 {
		t.Errorf("a contest that settled nothing releases its chairs, got %d still claimed", got)
	}
}

// The calendar box stays usable on the chairs the contest is not about. A
// contested chair's date is frozen (docs/adr/047); an uncontested one is not,
// because a founder setting next year's dates should not have to wait out a
// contest for a seat she is not touching.
func TestSeatTerm_FrozenOnlyOnTheContestedChair(t *testing.T) {
	db := setupTestDB(t)
	nodeID, slug, dueUser, dueToken, safeA, _ := staggeredCouncil(t, db, "three")

	handler.ScheduleDueElections(db)
	if openElectionCount(t, db, nodeID) != 1 {
		t.Fatal("expected a contest for the overdue chair")
	}

	sooner := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	if code, body := setTermVia(t, db, slug, seatIDOf(t, db, nodeID, safeA), sooner, dueToken); code != http.StatusOK {
		t.Errorf("a chair outside the contest can still be scheduled, got %d: %s", code, body)
	}
	if code, _ := setTermVia(t, db, slug, seatIDOf(t, db, nodeID, dueUser), sooner, dueToken); code != http.StatusConflict {
		t.Errorf("the contested chair's calendar is frozen while people vote on it, got %d", code)
	}
}
