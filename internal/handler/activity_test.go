package handler_test

import (
	"fmt"
	"net/http"
	"net/url"
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

// activityPage fetches one page of the caller's feed.
func activityPage(t *testing.T, db *database.DB, token, query string) ([]map[string]interface{}, string) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/activity"+query, nil, token)
	w := serveMux(t, db, "GET", "/api/v1/activity", handler.UserActivityFeed(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	raw, _ := body["items"].([]interface{})
	var items []map[string]interface{}
	for _, it := range raw {
		items = append(items, it.(map[string]interface{}))
	}
	cursor, _ := body["next_cursor"].(string)
	return items, cursor
}

// TestUserActivityFeed_LinksAreTheCanonicalRoutes: each item points where
// internal/weblink says that thing lives. The event link was
// /patches/{slug}/events/{id}, a route the SPA keeps only to redirect and no
// other client knows (issue #398).
func TestUserActivityFeed_LinksAreTheCanonicalRoutes(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "link-member", "member")
	nodeID := createTestNode(t, db, user.ID, "Links", "activity-links", "open")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	propID := createTestProposal(t, db, nodeID, user.ID)
	eventID := seedEvent(t, db, nodeID, user.ID, "Linked event", "2026-09-01T18:00:00Z")
	docID := seedGovernanceDoc(t, db, nodeID, user.ID, "Linked charter")

	want := map[string]string{
		propID:  "/patches/activity-links/governance/" + propID,
		eventID: "/events/" + eventID,
		docID:   "/patches/activity-links/governance/docs/" + docID,
	}
	items, _ := activityPage(t, db, token, "?limit=50")
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %d", len(want), len(items))
	}
	for _, it := range items {
		id := it["id"].(string)
		if it["link"] != want[id] {
			t.Errorf("%s %s: link = %v, want %s", it["type"], id, it["link"], want[id])
		}
	}
}

// TestUserActivityFeed_ACharterIsDatedByItsCreation: an edit is not news of
// a new charter. Dated by updated_at, every edit put an old charter back at
// the top of the feed as if it had just been written (issue #398).
func TestUserActivityFeed_ACharterIsDatedByItsCreation(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "dating-member", "member")
	nodeID := createTestNode(t, db, user.ID, "Dating", "activity-dating", "open")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	docID := seedGovernanceDoc(t, db, nodeID, user.ID, "Old charter, new edit")
	if _, err := db.Exec(`UPDATE governance_docs SET created_at = '2026-01-01T00:00:00Z', updated_at = '2026-10-01T00:00:00Z' WHERE id = ?`, docID); err != nil {
		t.Fatalf("stamp doc: %v", err)
	}
	seedEvent(t, db, nodeID, user.ID, "Newer event", "2026-11-01T18:00:00Z")
	if _, err := db.Exec(`UPDATE events SET created_at = '2026-06-01T00:00:00Z'`); err != nil {
		t.Fatalf("stamp event: %v", err)
	}

	items, _ := activityPage(t, db, token, "?limit=50")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["title"] != "Newer event" {
		t.Fatalf("an edited charter outranked a newer event: first item is %v", items[0]["title"])
	}
	if items[1]["created_at"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("charter dated %v, want its created_at", items[1]["created_at"])
	}
}

// TestUserActivityFeed_PagesAcrossTheUnionWithoutLossOrRepeat: next_cursor is
// a (created_at, id) keyset over a three-way UNION ALL, so a timestamp shared
// by a proposal, an event and a charter must not drop or repeat any of them at
// a page boundary. Every row here shares one instant, the worst case: the id
// tiebreaker is the only thing ordering them.
func TestUserActivityFeed_PagesAcrossTheUnionWithoutLossOrRepeat(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "paging-member", "member")
	nodeID := createTestNode(t, db, user.ID, "Paging", "activity-paging", "open")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	for i := 0; i < 3; i++ {
		createTestProposal(t, db, nodeID, user.ID)
		seedEvent(t, db, nodeID, user.ID, fmt.Sprintf("Event %d", i), "2026-09-01T18:00:00Z")
		seedGovernanceDoc(t, db, nodeID, user.ID, fmt.Sprintf("Charter %d", i))
	}
	// Plus one a second later, so the boundary also crosses a timestamp.
	seedEvent(t, db, nodeID, user.ID, "Later event", "2026-09-01T18:00:00Z")
	const tied = "2026-09-01T10:00:00Z"
	for _, stmt := range []string{
		`UPDATE proposals SET created_at = '` + tied + `'`,
		`UPDATE governance_docs SET created_at = '` + tied + `'`,
		`UPDATE events SET created_at = '` + tied + `' WHERE title != 'Later event'`,
		`UPDATE events SET created_at = '2026-09-01T10:00:01Z' WHERE title = 'Later event'`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("stamp: %v", err)
		}
	}

	whole, cursor := activityPage(t, db, token, "?limit=50")
	if cursor != "" || len(whole) != 10 {
		t.Fatalf("one page of 50: got %d items, cursor %q", len(whole), cursor)
	}

	for _, size := range []int{1, 2, 3, 4} {
		var paged []string
		seen := map[string]bool{}
		after := ""
		for pages := 0; ; pages++ {
			if pages > 20 {
				t.Fatalf("limit=%d: cursor never ran out", size)
			}
			q := fmt.Sprintf("?limit=%d", size)
			if after != "" {
				q += "&after=" + url.QueryEscape(after)
			}
			items, next := activityPage(t, db, token, q)
			for _, it := range items {
				id := it["id"].(string)
				if seen[id] {
					t.Fatalf("limit=%d: %s served twice", size, id)
				}
				seen[id] = true
				paged = append(paged, id)
			}
			if next == "" {
				break
			}
			after = next
		}
		if len(paged) != len(whole) {
			t.Fatalf("limit=%d: paged %d items, one page held %d", size, len(paged), len(whole))
		}
		for i := range whole {
			if paged[i] != whole[i]["id"] {
				t.Fatalf("limit=%d: item %d is %s, one page had %v", size, i, paged[i], whole[i]["id"])
			}
		}
	}
}
