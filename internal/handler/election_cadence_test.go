package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A chair nobody wins keeps the council's clock, not its own (docs/adr/108).
//
// Nothing moved an empty chair's term end, so it was overdue on every pass
// and the calendar re-opened a contest for it after each breather, for ever.
// A simulated co-op ran six contests **56 days apart, five times running**,
// on a patch whose page said "Each term runs 12 months". Kofi counted them:
// "Eight weeks, exactly, like clockwork... A term cannot be a year if the
// chair is contested every eight weeks."

func termEndsOf(t *testing.T, db *database.DB, nodeID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT COALESCE(term_ends_at,'') FROM seats WHERE node_id = ?
	                       ORDER BY created_at ASC`, nodeID)
	if err != nil {
		t.Fatalf("read terms: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if rows.Scan(&e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// A contest that fills one chair of three puts the other two back on the
// council's calendar, and the calendar then leaves them alone.
func TestCadence_AChairNobodyWinsRejoinsTheCouncilsCalendar(t *testing.T) {
	db := setupTestDB(t)
	founder, founderToken := createTestUser(t, db, "cad-founder", "member")
	nodeID := electedNode(t, db, founder.ID, "Cadence", "cadence", 0, 12)
	createTestMembership(t, db, founder.ID, nodeID, "admin", "active")

	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, founder.ID, past)
	makeVacantSeat(t, db, nodeID, past)
	makeVacantSeat(t, db, nodeID, past)

	handler.ScheduleDueElections(db)
	var contestID string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&contestID)
	if contestID == "" {
		t.Fatal("expected the overdue council to open a contest")
	}

	// One person stands and is approved; two chairs go unfilled.
	if code := standFor(t, db, contestID, founderToken, ""); code != http.StatusCreated {
		t.Fatalf("standing: expected 201, got %d", code)
	}
	closeNominations(t, db, contestID)
	handler.OpenElectionVoting(db, contestID)
	cand := candidateIDFor(t, db, contestID, founder.ID)
	if code := castApprovals(t, db, contestID, founderToken, []string{cand}); code != http.StatusOK {
		t.Fatalf("ballot: expected 200, got %d", code)
	}
	expireProposal(t, db, contestID)
	handler.SweepElections(db)

	terms := termEndsOf(t, db, nodeID)
	if len(terms) != 3 {
		t.Fatalf("expected three chairs, got %d", len(terms))
	}
	seated := terms[0]
	if seated <= time.Now().UTC().Format("2006-01-02") {
		t.Fatalf("the winner's chair should carry a fresh term, got %q", seated)
	}
	for i, end := range terms[1:] {
		if end != seated {
			t.Errorf("unfilled chair %d should ride the council's calendar (%s), got %q", i+1, seated, end)
		}
	}

	// And the calendar leaves them alone: nothing is overdue any more, so no
	// second contest eight weeks from now. The vacancies are filled by
	// nomination in the meantime (docs/adr/100).
	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("expected no new contest while the council's term runs, got %d", got)
	}
	db.Exec(`UPDATE proposals SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05.000Z"), contestID)
	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("a chair nobody won must not bring the whole council round again, got %d open", got)
	}
}

// The exception, and the one the churn was standing in for: with every chair
// empty there is nobody to nominate and the contest is the only way back, so
// those chairs stay overdue and the calendar keeps trying.
func TestCadence_AnEmptyCouncilKeepsBeingContested(t *testing.T) {
	db := setupTestDB(t)
	founder, _ := createTestUser(t, db, "cad-empty", "member")
	nodeID := electedNode(t, db, founder.ID, "Empty Council", "empty-council", 0, 12)
	createTestMembership(t, db, founder.ID, nodeID, "member", "active")

	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	makeVacantSeat(t, db, nodeID, past)
	makeVacantSeat(t, db, nodeID, past)

	handler.ScheduleDueElections(db)
	var contestID string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&contestID)
	if contestID == "" {
		t.Fatal("expected a contest for the empty council")
	}

	// Nobody stands; it settles nothing (docs/adr/106) and the chairs stay
	// overdue, because there is no held term for them to borrow.
	closeNominations(t, db, contestID)
	handler.SweepElections(db)
	for i, end := range termEndsOf(t, db, nodeID) {
		if end != past {
			t.Errorf("chair %d: an empty council has no term to ride, want %s, got %q", i, past, end)
		}
	}

	// Inside the breather, nothing reopens.
	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("expected the breather to hold, got %d open", got)
	}

	// Once it has run out, the calendar tries again — this patch's only way
	// back to having admins at all.
	db.Exec(`UPDATE proposals SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05.000Z"), contestID)
	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 1 {
		t.Errorf("an empty council must keep being offered a contest, got %d open", got)
	}
}

// The date a member is given is the day something happens, breather included.
func TestCadence_TheNextContestDateCountsTheBreather(t *testing.T) {
	db := setupTestDB(t)
	founder, token := createTestUser(t, db, "cad-date", "member")
	nodeID := electedNode(t, db, founder.ID, "Breather", "breather", 0, 12)
	createTestMembership(t, db, founder.ID, nodeID, "admin", "active")
	seedSeatFor(t, db, nodeID, founder.ID, time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02"))

	handler.ScheduleDueElections(db)
	var contestID string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&contestID)
	closeNominations(t, db, contestID)
	handler.SweepElections(db) // nobody stood: settles nothing, breather starts

	r := authedRequest("GET", "/api/v1/nodes/breather/governance/overview", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	opens, _ := decodeJSON(t, w)["next_contest_opens"].(string)

	today := time.Now().UTC().Format("2006-01-02")
	if opens == "" {
		t.Fatal("expected a date for the next contest")
	}
	if opens <= today {
		// This is the sentence Devon could not use: the page said the contest
		// was due now, every day, while the breather ran, and then nothing
		// happened for four weeks.
		t.Errorf("a patch inside its breather is not due now: got %q, today is %s", opens, today)
	}
}
