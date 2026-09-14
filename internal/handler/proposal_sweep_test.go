package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// A window that closes settles something (docs/adr/097).

// sweepRig is a majority-vote patch with an admin and three members, all
// past any tenure window, and one open proposal with a running clock.
type sweepRig struct {
	db         *database.DB
	nodeID     string
	proposalID string
	tokens     []string // admin first
	userIDs    []string
}

func newSweepRig(t *testing.T, slug string, quorum int) sweepRig {
	t.Helper()
	db := setupTestDB(t)
	rig := sweepRig{db: db}
	admin, adminTok := createTestUser(t, db, slug+"_admin", "member")
	rig.nodeID = createTestNode(t, db, admin.ID, "Sweep "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, rig.nodeID, "admin", "active")
	rig.tokens = append(rig.tokens, adminTok)
	rig.userIDs = append(rig.userIDs, admin.ID)
	for _, name := range []string{"m1", "m2", "m3"} {
		u, tok := createTestUser(t, db, slug+"_"+name, "member")
		createTestMembership(t, db, u.ID, rig.nodeID, "member", "active")
		rig.tokens = append(rig.tokens, tok)
		rig.userIDs = append(rig.userIDs, u.ID)
	}
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, rig.nodeID)
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":`+itoaT(quorum)+`,"default_vote_duration_hours":72,"amendment_threshold":"majority","min_voting_tenure_days":0}`,
		rig.nodeID)
	rig.proposalID = createTestProposal(t, db, rig.nodeID, admin.ID)
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET state = 'voting', voting_ends_at = ? WHERE id = ?`, future, rig.proposalID)
	return rig
}

// closeWindow drags the voting window into the past, the way the clock would.
func (r sweepRig) closeWindow(t *testing.T) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	r.db.Exec(`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, past, r.proposalID)
}

func (r sweepRig) statusAndState(t *testing.T) (string, string) {
	t.Helper()
	var status, state string
	r.db.QueryRow(`SELECT status, COALESCE(state,'') FROM proposals WHERE id = ?`, r.proposalID).Scan(&status, &state)
	return status, state
}

// Before this sweep existed, a vote with a majority and quorum sat "open"
// until somebody opened the page. Nobody opens anything here.
func TestSweepProposals_TheClockEndsAVote(t *testing.T) {
	rig := newSweepRig(t, "swp-clock", 25)
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[1], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[2], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	rig.closeWindow(t)

	handler.SweepProposals(rig.db)

	if status, state := rig.statusAndState(t); status != "approved" || state != "approved" {
		t.Errorf("after the sweep: status %q state %q, want approved/approved", status, state)
	}
}

// Under quorum at close, the proposal lapses: a terminal state that says
// "not decided", tells the members, and is audited to the clock rather
// than a person. It used to stay open forever.
func TestSweepProposals_LapsesUnderQuorum(t *testing.T) {
	rig := newSweepRig(t, "swp-lapse", 50)
	handler.SetNotifier(notifications.NewNotifier(rig.db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	// One of four eligible: 25 %, under the 50 % quorum.
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[1], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	rig.closeWindow(t)

	handler.SweepProposals(rig.db)

	if status, state := rig.statusAndState(t); status != "rejected" || state != "lapsed" {
		t.Fatalf("after the sweep: status %q state %q, want rejected/lapsed", status, state)
	}

	var audited int
	rig.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'proposal.lapsed' AND entity_id = ? AND user_id IS NULL`,
		rig.proposalID).Scan(&audited)
	if audited != 1 {
		t.Errorf("expected one actorless proposal.lapsed audit entry, got %d", audited)
	}

	// Notifications land on a goroutine.
	deadline := time.Now().Add(3 * time.Second)
	var notified int
	for time.Now().Before(deadline) {
		rig.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE type = 'proposal.rejected' AND title LIKE 'Not decided:%' AND user_id = ?`,
			rig.userIDs[3]).Scan(&notified)
		if notified > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if notified == 0 {
		t.Error("a member who never voted was not told the proposal lapsed")
	}

	// Idempotent: a second pass moves nothing and tells nobody twice.
	handler.SweepProposals(rig.db)
	rig.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'proposal.lapsed' AND entity_id = ?`, rig.proposalID).Scan(&audited)
	if audited != 1 {
		t.Errorf("second sweep audited the lapse again: %d entries", audited)
	}

	// And the window stays closed to late enthusiasm.
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[2], "approve"); code != http.StatusBadRequest {
		t.Errorf("vote after lapse: got %d, want 400", code)
	}
}

// Under quorum while the window runs, votes may still come: the sweep
// leaves it exactly as it found it.
func TestSweepProposals_LeavesARunningWindowAlone(t *testing.T) {
	rig := newSweepRig(t, "swp-running", 50)
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[1], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}

	handler.SweepProposals(rig.db)

	if status, state := rig.statusAndState(t); status != "open" || state != "voting" {
		t.Errorf("sweep touched a running window: status %q state %q", status, state)
	}
	// A second vote inside the window meets quorum and the sweep, once the
	// window closes, resolves it rather than lapsing it.
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[2], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	rig.closeWindow(t)
	handler.SweepProposals(rig.db)
	if status, state := rig.statusAndState(t); status != "approved" || state != "approved" {
		t.Errorf("after quorum and close: status %q state %q, want approved/approved", status, state)
	}
}

// Elections are the other sweep's. This one must not touch them, or a
// contest still taking nominations would be judged as a vote nobody cast.
func TestSweepProposals_IgnoresElections(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "swp-el", "member")
	nodeID := electedNode(t, db, admin.ID, "Swp Elec", "swp-elec", 50, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	id := openElection(t, db, nodeID)
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, past, id)

	handler.SweepProposals(db)

	var status string
	db.QueryRow(`SELECT status FROM proposals WHERE id = ?`, id).Scan(&status)
	if status != "open" {
		t.Errorf("the proposal sweep touched an election: status %q", status)
	}
}

// A patch born under an elected template adopts elected leadership at
// birth, and adoption starts an election (docs/adr/051). Creation never
// fired that trigger, so a Formal co-op's founder held no seat and its
// calendar never ran.
func TestCreateNode_ElectedTemplateOpensFirstElection(t *testing.T) {
	db := setupTestDB(t)
	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })
	if err := governance.InitInstanceRepo(tmp); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}
	_, token := createTestUser(t, db, "born-elected", "member")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))

	create := func(name, template string) string {
		t.Helper()
		r := authedRequest("POST", "/api/v1/nodes", map[string]string{
			"name": name, "template": template, "membership_policy": "open",
		}, token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, w.Code, w.Body.String())
		}
		var node map[string]interface{}
		json.NewDecoder(w.Body).Decode(&node)
		return node["id"].(string)
	}

	formal := create("Born Elected", "formal")
	var elections, seats int
	db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(seats_contested),0) FROM proposals WHERE node_id = ? AND seats_contested > 0 AND status = 'open'`,
		formal).Scan(&elections, &seats)
	if elections != 1 || seats != 1 {
		t.Errorf("formal template: want one open election for the founder's seat, got %d election(s) contesting %d", elections, seats)
	}
	var nominations string
	db.QueryRow(`SELECT COALESCE(nominations_close_at,'') FROM proposals WHERE node_id = ? AND seats_contested > 0`, formal).Scan(&nominations)
	if nominations == "" {
		t.Error("the first election should open with a nomination window")
	}

	casual := create("Born Casual", "casual")
	db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE node_id = ? AND seats_contested > 0`, casual).Scan(&elections)
	if elections != 0 {
		t.Errorf("casual template (maintainer): expected no election, got %d", elections)
	}
}
