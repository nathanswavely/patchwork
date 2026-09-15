package handler_test

import (
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// An archived patch holds no more elections.
//
// `ScheduleDueElections` has always skipped patches that are not active;
// `SweepElections` never did, so a contest already open when the patch was
// archived kept moving — nominations closed on the calendar, a ballot
// opened, members were notified, and a council would have been seated on a
// patch nobody can open. Found in the governance simulation: a collective
// created a duplicate patch by accident and archived it, and a fortnight
// later the ghost was running a vote.
func TestSweepElections_SkipsArchivedPatches(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "ghost-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Ghost Hall", "ghost-hall", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	id := openElection(t, db, nodeID)

	// The contest is mid-flight: nominations have run out, so the next sweep
	// would open its ballot.
	closeNominations(t, db, id)
	db.Exec(`UPDATE nodes SET status = 'archived' WHERE id = ?`, nodeID)

	handler.SweepElections(db)

	var votingEnds, status string
	db.QueryRow(`SELECT COALESCE(voting_ends_at,''), status FROM proposals WHERE id = ?`, id).Scan(&votingEnds, &status)
	if votingEnds != "" {
		t.Errorf("an archived patch opened a ballot: voting_ends_at = %q", votingEnds)
	}
	if status != "open" {
		t.Errorf("status = %q; the contest should be left exactly as the archive found it", status)
	}

	// Restored, it picks up where it left off — the archive paused the
	// calendar rather than cancelling it (docs/adr/034: archive keeps its
	// gravity, and restore is a real act).
	db.Exec(`UPDATE nodes SET status = 'active' WHERE id = ?`, nodeID)
	handler.SweepElections(db)
	db.QueryRow(`SELECT COALESCE(voting_ends_at,'') FROM proposals WHERE id = ?`, id).Scan(&votingEnds)
	if votingEnds == "" {
		t.Error("a restored patch did not resume its contest")
	}
}

// The same for a resolution: a ballot that ran out while the patch was
// archived seats nobody until somebody brings the patch back.
func TestSweepElections_ArchivedBallotSeatsNobody(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "ghost2-admin", "member")
	nodeID := electedNode(t, db, admin.ID, "Ghost Two", "ghost-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	id := openElection(t, db, nodeID)

	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET nominations_close_at = ?, voting_ends_at = ? WHERE id = ?`, past, past, id)
	db.Exec(`UPDATE nodes SET status = 'archived' WHERE id = ?`, nodeID)

	// Adoption seated the sitting admin before it contested them
	// (docs/adr/098), so the council is not empty to begin with. What must
	// not move is the seating, so measure the change rather than the count.
	seatsBefore := councilSnapshot(t, db, nodeID)

	handler.SweepElections(db)

	var status string
	db.QueryRow(`SELECT status FROM proposals WHERE id = ?`, id).Scan(&status)
	if status != "open" {
		t.Errorf("an archived patch resolved its election: status = %q", status)
	}
	if got := councilSnapshot(t, db, nodeID); got != seatsBefore {
		t.Errorf("an archived patch reseated its council:\n before %q\n after  %q", seatsBefore, got)
	}
}

// councilSnapshot renders who holds which seat, so a test can assert that a
// sweep changed nothing rather than counting rows that were always there.
func councilSnapshot(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	rows, err := db.Query(`SELECT id, COALESCE(holder_id,''), COALESCE(term_ends_at,'')
	                       FROM seats WHERE node_id = ? ORDER BY created_at, id`, nodeID)
	if err != nil {
		t.Fatalf("seats: %v", err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var id, holder, term string
		if rows.Scan(&id, &holder, &term) == nil {
			out += id + "=" + holder + "@" + term + ";"
		}
	}
	return out
}
