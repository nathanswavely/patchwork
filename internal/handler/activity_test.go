package handler_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// TestUserActivityFeed_ArchivedPatchActivityGone: archiving a patch removes it
// from the member's patch list, so its activity has to leave the dashboard
// feed too — otherwise the feed serves links that 404.
func TestUserActivityFeed_ArchivedPatchActivityGone(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "activity-user", "member")

	liveNode := createTestNode(t, db, user.ID, "Live Patch", "activity-live", "open")
	deadNode := createTestNode(t, db, user.ID, "Dead Patch", "activity-dead", "open")
	createTestMembership(t, db, user.ID, liveNode, "member", "active")
	createTestMembership(t, db, user.ID, deadNode, "member", "active")

	seedEvent(t, db, liveNode, user.ID, "live-activity-event", "2026-09-01T18:00:00Z")
	seedEvent(t, db, deadNode, user.ID, "dead-activity-event", "2026-09-02T18:00:00Z")

	if _, err := db.Exec("UPDATE nodes SET status = 'archived' WHERE id = ?", deadNode); err != nil {
		t.Fatalf("archive node: %v", err)
	}

	r := authedRequest("GET", "/api/v1/activity", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/activity", handler.UserActivityFeed(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items, ok := decodeJSON(t, w)["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array")
	}
	var titles []string
	for _, it := range items {
		titles = append(titles, it.(map[string]interface{})["title"].(string))
	}
	if len(titles) != 1 || titles[0] != "live-activity-event" {
		t.Fatalf("feed should carry only the live patch's event, got %v", titles)
	}
}

// activityTitles fetches the caller's feed and returns every title on its
// first page, in order.
func activityTitles(t *testing.T, db *database.DB, token string) []string {
	t.Helper()
	r := authedRequest("GET", "/api/v1/activity?limit=50", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/activity", handler.UserActivityFeed(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	items, ok := decodeJSON(t, w)["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array")
	}
	var titles []string
	for _, it := range items {
		titles = append(titles, it.(map[string]interface{})["title"].(string))
	}
	return titles
}

// TestUserActivityFeed_GovernanceReadsAsTheRESTLayerDoes is issue #398. The
// proposal and charter branches built themselves from every active
// relationship, follower included, and asked nothing further — so on any
// patch, whatever its door, clicking Follow put the titles of its closed
// proposals and unpublished charters in your feed. Following needs nobody's
// approval (docs/adr/116), which makes invite_only the case that matters:
// a patch whose whole door is an invitation leaked to a stranger.
//
// Each seat must see exactly what the REST reads would hand it:
// ListProposals behind canReadGovernanceRecord, ListGovernanceDocs behind
// visibility or canReadPatchDocs.
func TestUserActivityFeed_GovernanceReadsAsTheRESTLayerDoes(t *testing.T) {
	const (
		proposal  = "Closed-record proposal"
		published = "Published charter"
		private   = "Members-only charter"
	)

	cases := []struct {
		name string
		role string
		// openRecord publishes the patch's deliberation (Patch Settings).
		openRecord bool
		// grantCharters is the follower_permissions.charters key, off unless
		// a patch says so (docs/adr/116).
		grantCharters bool
		want          []string
	}{
		{name: "follower", role: "follower", want: []string{published}},
		{name: "follower, open record", role: "follower", openRecord: true, want: []string{published, proposal}},
		{name: "follower granted charters", role: "follower", grantCharters: true, want: []string{private, published}},
		{name: "member", role: "member", want: []string{private, published, proposal}},
		{name: "admin", role: "admin", want: []string{private, published, proposal}},
	}

	for _, policy := range []string{"open", "approval_required", "invite_only"} {
		for i, tc := range cases {
			t.Run(policy+"/"+tc.name, func(t *testing.T) {
				db := setupTestDB(t)
				owner, _ := createTestUser(t, db, fmt.Sprintf("owner-%d", i), "member")
				viewer, token := createTestUser(t, db, fmt.Sprintf("viewer-%d", i), "member")

				nodeID := createTestNode(t, db, owner.ID, "Shelf", "shelf-"+policy, policy)
				createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
				createTestMembership(t, db, viewer.ID, nodeID, tc.role, "active")
				if tc.openRecord {
					openGovernanceRecord(t, db, nodeID)
				}
				if tc.grantCharters {
					if _, err := db.Exec(`UPDATE nodes SET follower_permissions = '{"charters":true}' WHERE id = ?`, nodeID); err != nil {
						t.Fatalf("grant charters: %v", err)
					}
				}

				// Stamped a second apart, so the feed's newest-first order is
				// fixed: private charter, published charter, proposal.
				propID := createTestProposal(t, db, nodeID, owner.ID)
				pubID := seedGovernanceDoc(t, db, nodeID, owner.ID, published)
				privID := seedGovernanceDoc(t, db, nodeID, owner.ID, private)
				if _, err := db.Exec(`UPDATE proposals SET title = ?, created_at = '2026-09-01T10:00:00Z' WHERE id = ?`, proposal, propID); err != nil {
					t.Fatalf("stamp proposal: %v", err)
				}
				if _, err := db.Exec(`UPDATE governance_docs SET visibility = 'public', created_at = '2026-09-01T10:00:01Z', updated_at = '2026-09-01T10:00:01Z' WHERE id = ?`, pubID); err != nil {
					t.Fatalf("publish doc: %v", err)
				}
				if _, err := db.Exec(`UPDATE governance_docs SET created_at = '2026-09-01T10:00:02Z', updated_at = '2026-09-01T10:00:02Z' WHERE id = ?`, privID); err != nil {
					t.Fatalf("stamp doc: %v", err)
				}

				got := activityTitles(t, db, token)
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("%s on a %s patch: feed = %v, want %v", tc.role, policy, got, tc.want)
				}
			})
		}
	}
}

// TestUserActivityFeed_MalformedRulesKeepTheShelfShut: canReadPatchDocs
// reads a rules blob it cannot parse as "not granted", and the feed's SQL
// twin must too. followerEventsAllowedSQL defaults the other way, on purpose,
// and copying it here is the mistake this pins.
func TestUserActivityFeed_MalformedRulesKeepTheShelfShut(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "shelf-owner", "member")
	follower, token := createTestUser(t, db, "shelf-follower", "member")
	nodeID := createTestNode(t, db, owner.ID, "Shelf", "shelf-malformed", "invite_only")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	seedGovernanceDoc(t, db, nodeID, owner.ID, "Members-only charter")
	if _, err := db.Exec(`UPDATE nodes SET follower_permissions = '{not json' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("break rules: %v", err)
	}

	if got := activityTitles(t, db, token); len(got) != 0 {
		t.Fatalf("a follower read a members-only charter through a malformed rules blob: %v", got)
	}
}
