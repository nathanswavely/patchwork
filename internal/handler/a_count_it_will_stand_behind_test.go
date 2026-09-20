package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-130. A patch that publishes no deliberation published how much of it
// there was.
//
// `"open_proposals": 1` went to an anonymous caller, the hub rendered it as a
// link, and the link answered "This content is only visible to members.
// Become a member to access proposals." A founder signed out to see her own
// patch as a visitor and found the pair: "a link that counts something to you
// and then refuses to show you the nothing it counted... the page is willing
// to tell you a number it will not stand behind."
//
// This is not docs/adr/095's member_count, which is published on purpose so
// the quilt can size a tile. `public_governance_record = nobody` is a
// retraction, and a count walked straight through it. The document count
// beside it has always been asked through its own gate for the same reason.

func overviewAs(t *testing.T, db *database.DB, slug, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/overview", nil, token)
	var w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	if token == "" {
		r = authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/overview", nil, "")
		w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

// closedRecordFixture is a patch running one open proposal and publishing
// none of it, which is what a patch created through the product is.
func closedRecordFixture(t *testing.T, db *database.DB, slug string) (nodeID, memberToken string) {
	t.Helper()
	admin, memberToken := createTestUser(t, db, slug+"-admin", "member")
	nodeID = createTestNode(t, db, admin.ID, "Closed "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestProposal(t, db, nodeID, admin.ID)
	return nodeID, memberToken
}

func TestRecord_AClosedPatchPublishesNoProposalCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	closedRecordFixture(t, db, "no-count")

	body := overviewAs(t, db, "no-count", "")
	if body["proposals_withheld"] != true {
		t.Errorf("proposals_withheld = %v, want true", body["proposals_withheld"])
	}
	if got := body["open_proposals"]; got != float64(0) {
		t.Errorf("a closed patch published open_proposals = %v to a stranger", got)
	}
	for _, k := range []string{"passed_proposals", "rejected_proposals"} {
		if got := body[k]; got != float64(0) {
			t.Errorf("a closed patch published %s = %v to a stranger", k, got)
		}
	}
}

// The room still gets its own numbers. Withholding them from members would
// be a worse bug than the leak.
func TestRecord_TheRoomStillSeesItsOwnCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, memberToken := closedRecordFixture(t, db, "own-count")

	body := overviewAs(t, db, "own-count", memberToken)
	if body["proposals_withheld"] != false {
		t.Errorf("proposals_withheld = %v for an admin of the patch", body["proposals_withheld"])
	}
	if got := body["open_proposals"]; got != float64(1) {
		t.Errorf("open_proposals = %v for an admin of the patch, want 1", got)
	}
}

// And a patch that publishes its deliberation publishes the count, because
// the link works for that reader.
func TestRecord_AnOpenPatchStillCountsForEveryone(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _ := closedRecordFixture(t, db, "open-count")
	openGovernanceRecord(t, db, nodeID)

	body := overviewAs(t, db, "open-count", "")
	if body["proposals_withheld"] != false {
		t.Errorf("proposals_withheld = %v on a published record", body["proposals_withheld"])
	}
	if got := body["open_proposals"]; got != float64(1) {
		t.Errorf("open_proposals = %v on a published record, want 1", got)
	}
}

// An election is a proposal, and "See the candidates" lands on the same
// refusal the count did.
func TestRecord_AClosedPatchDoesNotPublishItsContest(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	admin, adminToken := createTestUser(t, db, "hidden-contest", "member")
	nodeID := electedNode(t, db, admin.ID, "Hidden Contest", "hidden-contest", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	openElection(t, db, nodeID)

	if got := overviewAs(t, db, "hidden-contest", "")["election"]; got != nil {
		t.Errorf("a closed patch published its live contest to a stranger: %v", got)
	}
	// Its own admin still sees it, or the patch cannot run the election.
	if overviewAs(t, db, "hidden-contest", adminToken)["election"] == nil {
		t.Error("the patch's own admin cannot see the contest it is running")
	}
}
