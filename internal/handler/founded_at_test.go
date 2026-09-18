package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Tenure is capped at the patch's age, and the patch may state its age
// (docs/adr/098).
//
// On the Formal defaults a patch made this morning asked its founder for
// thirty days of membership before a vote counted, counted an electorate of
// nobody, and lapsed its own first rules vote with no ballots. The required
// tenure is now min(configured, days since the patch was founded), and
// `founded_at` is how an organisation older than its patch keeps its bar.

// predateNode states that the group behind a patch is years older than its
// row, so a min_voting_tenure_days rule applies in full. Every test written
// before docs/adr/098 that expects a member who "joined just now" to be
// outside a thirty-day window needs this: the patch in those fixtures was
// also created just now, and a patch asks for no more tenure than it has.
func predateNode(t *testing.T, db *database.DB, nodeID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE nodes SET founded_at = '2015-01-01' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("predate node: %v", err)
	}
}

// tenureRig is a patch created now with a thirty-day tenure rule, one admin,
// one member who also joined now, and an open vote.
type tenureRig struct {
	db          *database.DB
	nodeID      string
	proposalID  string
	memberToken string
}

func newTenureRig(t *testing.T, slug string) tenureRig {
	t.Helper()
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, slug+"_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Tenure "+slug, slug, "open")
	// The payload under test is read from a proposal by id, which a closed
	// record answers 404.
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, slug+"_member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":50,"default_vote_duration_hours":72,"amendment_threshold":"majority","min_voting_tenure_days":30}`,
		nodeID)
	proposalID := createTestProposal(t, db, nodeID, admin.ID)
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	db.Exec(`UPDATE proposals SET state = 'voting', voting_ends_at = ? WHERE id = ?`, future, proposalID)
	return tenureRig{db: db, nodeID: nodeID, proposalID: proposalID, memberToken: memberToken}
}

// A patch founded today asks for no tenure today: the member who joined
// with it votes.
func TestTenure_CappedAtThePatchAge(t *testing.T) {
	rig := newTenureRig(t, "ten-today")
	if code := castVote(t, rig.db, rig.proposalID, rig.memberToken, "approve"); code != http.StatusOK {
		t.Errorf("a founding-day member on a founding-day patch may vote, got %d", code)
	}
}

// A patch that says it was founded in 2015 keeps the full bar: its members
// have in fact been there that long, and a newcomer waits the thirty days.
func TestTenure_FullBarWhereThePatchPredatesItsRow(t *testing.T) {
	rig := newTenureRig(t, "ten-old")
	rig.db.Exec(`UPDATE nodes SET founded_at = '2015-01-01' WHERE id = ?`, rig.nodeID)

	r := authedRequest("POST", "/api/v1/proposals/"+rig.proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, rig.memberToken)
	w := serveMux(t, rig.db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(rig.db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("a day-old member of a decade-old patch waits out the tenure, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "30 days") {
		t.Errorf("the refusal states the full bar, got %s", w.Body.String())
	}
}

// PATCH founded_at: a date, never in the future, and "" clears it.
func TestUpdateNode_FoundedAt(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "founded-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Founded", "founded", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	patchFounded := func(v string) int {
		t.Helper()
		r := authedRequest("PATCH", "/api/v1/nodes/founded", map[string]interface{}{"founded_at": v}, token)
		return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code
	}
	readFounded := func() interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/founded", nil, token)
		w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("get node: %d", w.Code)
		}
		return decodeJSON(t, w)["node"].(map[string]interface{})["founded_at"]
	}

	if got := readFounded(); got != nil {
		t.Errorf("a new patch states no founding date, got %v", got)
	}

	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	if code := patchFounded(tomorrow); code != http.StatusBadRequest {
		t.Errorf("a future founding date: got %d, want 400", code)
	}
	for _, bad := range []string{"2015/01/01", "2015-13-01", "yesterday", "2015-1-1"} {
		if code := patchFounded(bad); code != http.StatusBadRequest {
			t.Errorf("malformed %q: got %d, want 400", bad, code)
		}
	}

	if code := patchFounded("2015-01-01"); code != http.StatusOK {
		t.Fatalf("a past date: got %d, want 200", code)
	}
	if got := readFounded(); got != "2015-01-01" {
		t.Errorf("GET returns the founding date, got %v", got)
	}

	if code := patchFounded(""); code != http.StatusOK {
		t.Fatalf("clearing: got %d, want 200", code)
	}
	var stored interface{}
	db.QueryRow(`SELECT founded_at FROM nodes WHERE id = ?`, nodeID).Scan(&stored)
	if stored != nil {
		t.Errorf(`"" clears the column to NULL, got %v`, stored)
	}
	if got := readFounded(); got != nil {
		t.Errorf("GET returns null once cleared, got %v", got)
	}
}
