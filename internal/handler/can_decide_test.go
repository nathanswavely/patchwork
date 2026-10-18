package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
)

// An election on an admin-decides patch is the electorate's (docs/adr/051).
// decide already refused it; the payload said otherwise, and apply was a
// door around the refusal.
func TestAdminDecidesElection_IsNotTheMaintainers(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "electadmin", "elected")
	terms := `{"decision_method":"admin","quorum_percent":0,"leadership_model":"elected","leadership_venue":"patchwork"}`
	now := time.Now().UTC()
	electionID := auth.NewUUIDv7()
	if _, err := rig.db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
	         duration_hours, voting_ends_at, created_at, updated_at, seats_contested, nominations_close_at, voting_terms)
	         VALUES (?, ?, ?, 'Council election', '', 'open', 'voting', 'membership', 72, ?, ?, ?, 1, ?, ?)`,
		electionID, rig.nodeID, rig.adminID,
		now.Add(72*time.Hour).Format("2006-01-02T15:04:05.000Z"),
		now.Format("2006-01-02T15:04:05.000Z"), now.Format("2006-01-02T15:04:05.000Z"),
		now.Add(-time.Hour).Format("2006-01-02T15:04:05.000Z"), terms); err != nil {
		t.Fatalf("insert election: %v", err)
	}

	got := getProposal(t, rig.db, electionID, rig.adminTok)
	if got["can_decide"] != false {
		t.Errorf("can_decide on an election: want false, got %v", got["can_decide"])
	}
	if got["advisory"] != false {
		t.Errorf("advisory on an election: want false, got %v", got["advisory"])
	}

	if code, _ := decideOn(t, rig.db, electionID, rig.adminTok, "approve"); code != http.StatusConflict {
		t.Errorf("decide on an election: want 409, got %d", code)
	}
	if code, body := applyOn(t, rig.db, electionID, rig.adminTok); code != http.StatusConflict {
		t.Errorf("apply on an election: want 409, got %d: %s", code, body)
	}
	var status, state string
	rig.db.QueryRow("SELECT status, state FROM proposals WHERE id = ?", electionID).Scan(&status, &state)
	if status != "open" || state != "voting" {
		t.Errorf("the election must be left running, got status=%q state=%q", status, state)
	}

	// The same patch's ordinary proposal is still the maintainer's to decide.
	_, op := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "Repaint the hall", "proposal_type": "action", "put_to_vote": true,
	})
	ordinary := op["id"].(string)
	if got := getProposal(t, rig.db, ordinary, rig.adminTok); got["can_decide"] != true || got["advisory"] != true {
		t.Errorf("an ordinary proposal: want can_decide and advisory true, got %v / %v", got["can_decide"], got["advisory"])
	}
}

// can_decide is decide's answer, so it is false wherever decide refuses: an
// open proposal in a state no admin decides from.
func TestCanDecide_FalseOutsideTheDecidableStates(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "candecide", "meritocratic")
	id := rig.nominate(t, true)["id"].(string)
	rig.db.Exec("UPDATE proposals SET state = 'elsewhere' WHERE id = ?", id)
	if got := getProposal(t, rig.db, id, rig.adminTok); got["can_decide"] != false {
		t.Errorf("can_decide in state elsewhere: want false, got %v", got["can_decide"])
	}
	if code, _ := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusConflict {
		t.Errorf("decide in state elsewhere: want 409, got %d", code)
	}
}
