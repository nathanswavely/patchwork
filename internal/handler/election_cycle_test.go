package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The recurring cycle (docs/adr/051): "every cycle after is scheduled from
// when the last one seated the council."
//
// Nothing stores a next-election date. A seat carries its term end, so being
// due is derivable — which is also what leaves staggering free, since seats
// due at different dates come due at different times.

func seedSeatFor(t *testing.T, db *database.DB, nodeID, holderID, termEnds string) {
	t.Helper()
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (lower(hex(randomblob(16))), ?, ?, ?)`,
		nodeID, holderID, nullOrText(termEnds))
}

func nullOrText(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func openElectionCount(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'open' AND seats_contested > 0`, nodeID).Scan(&n)
	return n
}

// A council whose term ends inside the time a contest takes is due now.
func TestCycle_OpensWhenTheTermIsWithinReach(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc1", "member")
	nodeID := electedNode(t, db, admin.ID, "Cyc One", "cyc-one", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// nomination_days 14 + 72h voting is ~17 days of lead. A term ending in
	// three days is inside that.
	soon := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, soon)

	handler.ScheduleDueElections(db)

	if got := openElectionCount(t, db, nodeID); got != 1 {
		t.Fatalf("expected the cycle to open one election, got %d", got)
	}
	var seats int
	db.QueryRow(`SELECT seats_contested FROM proposals WHERE node_id = ? AND status = 'open'`, nodeID).Scan(&seats)
	if seats != 1 {
		t.Errorf("expected the one due seat contested, got %d", seats)
	}

	// Idempotent: a second sweep must not open a rival contest.
	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 1 {
		t.Errorf("expected still one election, got %d", got)
	}
}

// A term far off is not due, and nothing opens.
func TestCycle_QuietWhileTheTermRuns(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc2", "member")
	nodeID := electedNode(t, db, admin.ID, "Cyc Two", "cyc-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	far := time.Now().UTC().AddDate(0, 6, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, far)

	handler.ScheduleDueElections(db)

	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("a council mid-term is not due, got %d open", got)
	}
}

// An overdue council — its term already past, holdover carrying it — is due.
func TestCycle_OverdueCouncilIsDue(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc3", "member")
	nodeID := electedNode(t, db, admin.ID, "Cyc Three", "cyc-three", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, past)

	handler.ScheduleDueElections(db)

	if got := openElectionCount(t, db, nodeID); got != 1 {
		t.Errorf("an overdue council is due, got %d open", got)
	}
}

// A patch that sets no term length never schedules: elected once, then stable.
func TestCycle_NoTermLengthNeverSchedules(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc4", "member")
	nodeID := electedNode(t, db, admin.ID, "Cyc Four", "cyc-four", 0, 0)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	seedSeatFor(t, db, nodeID, admin.ID, "")

	handler.ScheduleDueElections(db)

	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("no term length means no cadence, got %d open", got)
	}
}

// Where admins are chosen elsewhere, the community keeps its own calendar.
func TestCycle_SilentWhereVenueIsElsewhere(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Cyc Five", "cyc-five", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","leadership_model":"elected","leadership_venue":"elsewhere","admin_term_months":12,"nomination_days":14,"default_vote_duration_hours":72}`, nodeID)
	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, past)

	handler.ScheduleDueElections(db)

	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("Patchwork must not run a calendar for a patch that elects elsewhere, got %d", got)
	}
}

// An election that settled nothing is not retried the same day. The council is
// overdue either way, and reopening immediately is the notice people learn to
// ignore.
func TestCycle_FailedElectionGetsABreather(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "cyc6", "member")
	nodeID := electedNode(t, db, admin.ID, "Cyc Six", "cyc-six", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	past := time.Now().UTC().AddDate(0, -2, 0).Format("2006-01-02")
	seedSeatFor(t, db, nodeID, admin.ID, past)

	// A contest that just failed.
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
	         duration_hours, created_at, updated_at, seats_contested)
	         VALUES (lower(hex(randomblob(16))), ?, ?, 'Council election', '', 'rejected', 'rejected',
	         'membership', 72, ?, ?, 1)`, nodeID, admin.ID, now, now)

	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 0 {
		t.Errorf("expected a breather after a failed election, got %d open", got)
	}

	// Once a full contest length has passed, it tries again.
	old := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET updated_at = ? WHERE node_id = ? AND status = 'rejected'`, old, nodeID)
	handler.ScheduleDueElections(db)
	if got := openElectionCount(t, db, nodeID); got != 1 {
		t.Errorf("expected a retry once the breather passed, got %d open", got)
	}
}

// The governance hub can see the contest the patch is running (docs/adr/051).
//
// It could not. The needs-a-vote banner deliberately stays quiet during
// nominations — nominations are not a ballot — so through the whole window,
// the only stretch when standing is possible, the governance page of a patch
// whose entire leadership story is elections said nothing about the election.
func TestGovernanceOverview_SurfacesTheLiveElection(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "hubelec", "member")
	nodeID := electedNode(t, db, admin.ID, "Hub Elec", "hub-elec", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	overview := func() map[string]interface{} {
		r := authedRequest("GET", "/api/v1/nodes/hub-elec/governance/overview", nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview",
			handler.GovernanceOverview(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)
	}

	// Before any contest: nothing to say, and the hub must not invent one.
	if got := overview()["election"]; got != nil {
		t.Errorf("expected no election, got %v", got)
	}

	id := openElection(t, db, nodeID)
	el, ok := overview()["election"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected the open contest in the payload, got %v", overview()["election"])
	}
	if el["id"] != id {
		t.Errorf("expected the running election, got %v", el["id"])
	}
	if el["phase"] != "nominating" {
		t.Errorf("expected phase=nominating, got %v", el["phase"])
	}
	if el["candidates"] != float64(0) {
		t.Errorf("expected nobody standing yet, got %v", el["candidates"])
	}

	// Once nominations close and the ballot opens, the phase moves with it.
	closeNominations(t, db, id)
	if !handler.OpenElectionVoting(db, id) {
		t.Fatal("expected voting to open")
	}
	el, _ = overview()["election"].(map[string]interface{})
	if el["phase"] != "voting" {
		t.Errorf("expected phase=voting, got %v", el["phase"])
	}

	// A resolved election is history; the hub is about what needs attention.
	db.Exec("UPDATE proposals SET status = 'approved' WHERE id = ?", id)
	if got := overview()["election"]; got != nil {
		t.Errorf("a closed election must not sit on the hub, got %v", got)
	}
}

// When the council next faces the electorate — the date behind "terms last 12
// months", which is policy rather than accountability.
func TestGovernanceOverview_ShowsWhenTheNextSeatComesUp(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "hubterm", "member")
	nodeID := electedNode(t, db, admin.ID, "Hub Term", "hub-term", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	read := func() interface{} {
		r := authedRequest("GET", "/api/v1/nodes/hub-term/governance/overview", nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview",
			handler.GovernanceOverview(db), r)
		return decodeJSON(t, w)["next_term_end"]
	}

	if got := read(); got != "" {
		t.Errorf("a patch with no seats has no term date, got %v", got)
	}

	// Staggered seats: the answer is the earliest, because that is the one a
	// member is asking about (docs/adr/051 put the date on the seat precisely
	// so they need not share it).
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, '2028-03-01')`,
		auth.NewUUIDv7(), nodeID, admin.ID)
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, NULL, '2027-03-01')`,
		auth.NewUUIDv7(), nodeID)

	if got := read(); got != "2027-03-01" {
		t.Errorf("expected the earliest term end, got %v", got)
	}
}
