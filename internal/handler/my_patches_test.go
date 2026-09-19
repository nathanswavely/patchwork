package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// unclaimNode drops a node into the state a community-submitted listing
// lives in: on the quilt, followable, run by nobody (docs/adr/030).
func unclaimNode(t *testing.T, db *database.DB, nodeID string) {
	t.Helper()
	if _, err := db.Exec("UPDATE nodes SET status = 'unclaimed' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("unclaim node: %v", err)
	}
}

func myNodes(t *testing.T, db *database.DB, token string) []map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/me/nodes", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/me/nodes", handler.ListMyMemberships(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Items
}

// Whether a member rung exists at all depends on the patch's own status,
// and My Patches could not see it: every follower row drew a "Become a
// member" that answered with a 403 (docs/adr/042).
func TestMyMembershipsCarriesNodeStatus(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner-status", "member")
	follower, followerToken := createTestUser(t, db, "follower-status", "member")

	openID := createTestNode(t, db, owner.ID, "Open House", "open-house", "open")
	unclaimedID := createTestNode(t, db, owner.ID, "Community Listing", "community-listing", "open")
	unclaimNode(t, db, unclaimedID)
	createTestMembership(t, db, follower.ID, openID, "follower", "active")
	createTestMembership(t, db, follower.ID, unclaimedID, "follower", "active")

	got := map[string]string{}
	for _, item := range myNodes(t, db, followerToken) {
		slug, _ := item["node_slug"].(string)
		status, ok := item["node_status"].(string)
		if !ok {
			t.Fatalf("membership for %q carries no node_status: %v", slug, item)
		}
		got[slug] = status
	}

	if got["open-house"] != "active" {
		t.Errorf("expected open-house active, got %q", got["open-house"])
	}
	if got["community-listing"] != "unclaimed" {
		t.Errorf("expected community-listing unclaimed, got %q", got["community-listing"])
	}
}

// The message the follower used to be handed told them to do the thing
// they had already done ("you can follow it"). It now names the patch's
// state and what would change it.
func TestJoinUnclaimedTellsTheFollowerSomethingNew(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner-unclaimed", "member")
	follower, followerToken := createTestUser(t, db, "follower-unclaimed", "member")
	nodeID := createTestNode(t, db, owner.ID, "Community Listing", "listing-two", "open")
	unclaimNode(t, db, nodeID)
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	r := authedRequest("POST", "/api/v1/nodes/listing-two/join", nil, followerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "must be claimed") {
		t.Errorf("message should state the patch's state and what would change it, got %s", body)
	}
	if strings.Contains(body, "follow") {
		t.Errorf("message tells a follower to follow, which they already have: %s", body)
	}
}

// Withdrawing a request lived here as a relaxed LeaveNode, on the reading
// that a request was a relationship a person could enter and not get out
// of. docs/adr/088 rejected that reading — a requester holds a request,
// not a relationship — and moved the act to its own route. The tests
// moved with it, to membership_withdraw_test.go, which also guards the
// boundary this file's version could not: that leave keeps refusing a
// pending row.

// The only-admin floor (docs/adr/012, docs/adr/051) is about the person
// running the patch.
func TestOnlyAdminStillCannotLeave(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin-floor", "member")
	nodeID := createTestNode(t, db, admin.ID, "Solo House", "solo-house", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/solo-house/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
