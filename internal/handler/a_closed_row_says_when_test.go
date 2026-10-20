package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A closed proposal's row said its outcome twice, once in the meta line and
// once in the badge, so a contest that seated nobody read "nobody elected
// [nobody elected]" (F-136). The meta line now says when it closed, which is
// the question it answers on every open row, and that date comes from the
// list endpoint as decided_at.
//
// decided_at is COALESCE(applied_at, updated_at), the expression the Record
// files a settled proposal under (governance_record.go settledProposals). A
// list naming one day and the Record another for the same decision is the
// thing this guards: an amendment that carried on the 1st and was made
// official on the 4th is dated the 4th in both places.
func TestListProposals_AClosedRowSaysWhenItClosed(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "closedadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Closed", "closed-patch", "open")
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seed := func(title, status, state, updatedAt, appliedAt string) {
		var applied interface{}
		if appliedAt != "" {
			applied = appliedAt
		}
		if _, err := db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		         duration_hours, voting_ends_at, created_at, updated_at, applied_at)
		         VALUES (?, ?, ?, ?, '', ?, ?, 'other', 72, NULL, '2026-08-01T10:00:00.000Z', ?, ?)`,
			auth.NewUUIDv7(), nodeID, admin.ID, title, status, state, updatedAt, applied); err != nil {
			t.Fatalf("seed %s: %v", title, err)
		}
	}
	seed("Ran out of time", "rejected", "lapsed", "2026-09-01T12:00:00.000Z", "")
	seed("Carried then made official", "approved", "in_effect", "2026-09-01T12:00:00.000Z", "2026-09-04T09:30:00.000Z")
	seed("Still running", "open", "voting", "2026-09-02T08:00:00.000Z", "")

	r := authedRequest("GET", "/api/v1/nodes/closed-patch/proposals?status=all", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got := map[string]interface{}{}
	items, _ := decodeJSON(t, w)["items"].([]interface{})
	for _, it := range items {
		m, _ := it.(map[string]interface{})
		title, _ := m["title"].(string)
		got[title] = m["decided_at"]
	}

	if got["Ran out of time"] != "2026-09-01T12:00:00.000Z" {
		t.Errorf("a lapse closes when its window does, got decided_at %v", got["Ran out of time"])
	}
	if got["Carried then made official"] != "2026-09-04T09:30:00.000Z" {
		t.Errorf("applied_at wins over updated_at, as on the Record, got decided_at %v", got["Carried then made official"])
	}
	if v, present := got["Still running"]; present && v != nil {
		t.Errorf("an open proposal has not been decided and must carry no date, got %v", v)
	}
}
