package handler_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Opening an election wrote nothing to the audit log (F-015). Resolution,
// nominations and ballots were all recorded, and the one governance act the
// calendar takes on its own, deciding a contest exists, was not. Each opening
// now leaves a row naming the seats it puts up and what opened it.

type openedRow struct {
	NodeID             string   `json:"node_id"`
	Seats              int      `json:"seats"`
	SeatIDs            []string `json:"seat_ids"`
	NominationsCloseAt string   `json:"nominations_close_at"`
	Trigger            string   `json:"trigger"`
}

func electionOpenedRows(t *testing.T, db *database.DB, proposalID string) []openedRow {
	t.Helper()
	rows, err := db.Query(`SELECT COALESCE(metadata,'{}') FROM audit_log
	                       WHERE action = 'election.opened' AND entity_type = 'proposal' AND entity_id = ?`, proposalID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []openedRow
	for rows.Next() {
		var meta string
		if err := rows.Scan(&meta); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var r openedRow
		if err := json.Unmarshal([]byte(meta), &r); err != nil {
			t.Fatalf("metadata %q: %v", meta, err)
		}
		out = append(out, r)
	}
	return out
}

func TestElectionOpened_AdoptionIsOnTheRecord(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "opened1", "member")
	nodeID := electedNode(t, db, admin.ID, "Opened One", "opened-one", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	second, _ := createTestUser(t, db, "opened1b", "member")
	createTestMembership(t, db, second.ID, nodeID, "admin", "active")

	id := openElection(t, db, nodeID)

	got := electionOpenedRows(t, db, id)
	if len(got) != 1 {
		t.Fatalf("expected one election.opened row for the contest, got %d", len(got))
	}
	r := got[0]
	if r.Trigger != "adoption" {
		t.Errorf("trigger = %q, want adoption", r.Trigger)
	}
	if r.NodeID != nodeID || r.Seats != 2 || len(r.SeatIDs) != 2 || r.NominationsCloseAt == "" {
		t.Errorf("the row should name the patch, both seats and the nomination close, got %+v", r)
	}

	// A second adoption opens no rival contest, so it writes no second row.
	handler.StartElectionOnAdoption(db, nodeID)
	if n := len(electionOpenedRows(t, db, id)); n != 1 {
		t.Errorf("expected still one row, got %d", n)
	}
}

func TestElectionOpened_TheCalendarIsOnTheRecord(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "opened2", "member")
	nodeID := electedNode(t, db, admin.ID, "Opened Two", "opened-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	soon := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, soon)

	handler.ScheduleDueElections(db)

	var id string
	if err := db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND seats_contested > 0`, nodeID).Scan(&id); err != nil {
		t.Fatalf("expected the calendar to open a contest: %v", err)
	}
	got := electionOpenedRows(t, db, id)
	if len(got) != 1 {
		t.Fatalf("expected one election.opened row, got %d", len(got))
	}
	if got[0].Trigger != "calendar" || got[0].Seats != 1 {
		t.Errorf("expected a calendar opening for the one due seat, got %+v", got[0])
	}
}
