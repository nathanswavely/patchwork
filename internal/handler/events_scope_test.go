package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// GET /api/v1/events had no scope parameter at all, so `/events/my` rendered
// the whole instance's calendar — the quilt and the map both narrowed, the
// events surface silently did not. These cover ?scope=my, and they hold it to
// the same definition of "My Quilt" as PersonalICSFeed, so the page and the
// calendar you subscribe to list the same events.

func listScopedEventIDs(t *testing.T, db *database.DB, r *http.Request) map[string]bool {
	t.Helper()
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list events: %d — %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ids := make(map[string]bool, len(resp.Items))
	for _, i := range resp.Items {
		ids[i.ID] = true
	}
	return ids
}

func soon(h int) time.Time { return time.Now().UTC().Add(time.Duration(h) * time.Hour) }

// The core of it: scope=my narrows to patches the caller has a relationship
// with, and every role counts — a follower's quilt is still their quilt.
func TestListEventsScopeMy(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-scope-user", "member")
	other, _ := createTestUser(t, db, "ev-scope-other", "member")

	adminNode := createTestNode(t, db, other.ID, "Admin Patch", "ev-admin", "open")
	memberNode := createTestNode(t, db, other.ID, "Member Patch", "ev-member", "open")
	followNode := createTestNode(t, db, other.ID, "Followed Patch", "ev-follow", "open")
	strangerNode := createTestNode(t, db, other.ID, "Stranger Patch", "ev-stranger", "open")

	createTestMembership(t, db, user.ID, adminNode, "admin", "active")
	createTestMembership(t, db, user.ID, memberNode, "member", "active")
	createTestMembership(t, db, user.ID, followNode, "follower", "active")
	createTestMembership(t, db, other.ID, strangerNode, "admin", "active")

	mine := insertEvent(t, db, adminNode, other.ID, "Admin Gig", soon(4))
	joined := insertEvent(t, db, memberNode, other.ID, "Member Gig", soon(5))
	followed := insertEvent(t, db, followNode, other.ID, "Followed Gig", soon(6))
	stranger := insertEvent(t, db, strangerNode, other.ID, "Stranger Gig", soon(7))

	r := authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token)
	ids := listScopedEventIDs(t, db, r)

	for name, id := range map[string]string{"admin": mine, "member": joined, "follower": followed} {
		if !ids[id] {
			t.Errorf("expected the %s patch's event on my quilt, missing", name)
		}
	}
	if ids[stranger] {
		t.Error("a stranger's event must not appear on my quilt")
	}
	if len(ids) != 3 {
		t.Errorf("expected exactly 3 events on my quilt, got %d", len(ids))
	}
}

// The regression itself, stated plainly: the unscoped list is the whole
// instance, and asking for my quilt must not return it.
func TestListEventsScopeMyIsNotTheWholeInstance(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-narrow-user", "member")
	owner, _ := createTestUser(t, db, "ev-narrow-owner", "member")

	minePatch := createTestNode(t, db, owner.ID, "Mine", "ev-narrow-mine", "open")
	createTestMembership(t, db, user.ID, minePatch, "member", "active")
	insertEvent(t, db, minePatch, owner.ID, "Mine", soon(3))

	for _, slug := range []string{"ev-narrow-a", "ev-narrow-b", "ev-narrow-c"} {
		n := createTestNode(t, db, owner.ID, slug, slug, "open")
		insertEvent(t, db, n, owner.ID, slug, soon(3))
	}

	unscoped := listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?limit=100", nil, token))
	scoped := listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token))

	if len(unscoped) != 4 {
		t.Fatalf("expected the unscoped list to hold all 4 events, got %d", len(unscoped))
	}
	if len(scoped) != 1 {
		t.Fatalf("expected 1 event on my quilt, got %d — scope=my returned the whole instance", len(scoped))
	}
}

// A confirmed event link travels with the relationship (docs/adr/032): a
// followed band's gig at a venue you don't follow still lands on your quilt,
// exactly as it does in the personal ICS feed.
func TestListEventsScopeMyFollowsConfirmedLinks(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-link-user", "member")
	owner, _ := createTestUser(t, db, "ev-link-owner", "member")

	venue := createTestNode(t, db, owner.ID, "Venue", "ev-link-venue", "open")
	band := createTestNode(t, db, owner.ID, "Band", "ev-link-band", "open")
	createTestMembership(t, db, user.ID, band, "follower", "active")

	gig := insertEvent(t, db, venue, owner.ID, "The Gig", soon(8))
	pendingGig := insertEvent(t, db, venue, owner.ID, "Maybe Gig", soon(9))
	linkEvent(t, db, gig, band, "confirmed", owner.ID)
	linkEvent(t, db, pendingGig, band, "pending", owner.ID)

	ids := listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token))

	if !ids[gig] {
		t.Error("a confirmed link must carry the event onto the follower's quilt")
	}
	if ids[pendingGig] {
		t.Error("an unconfirmed link must not put an event on anyone's quilt")
	}
}

// Members-only events belong to members of the event's OWN patch — a link
// never widens visibility, which is why the visibility test has to read the
// same membership row that matched the relationship.
func TestListEventsScopeMyMembersOnlyVisibility(t *testing.T) {
	db := setupTestDB(t)
	member, memberToken := createTestUser(t, db, "ev-vis-member", "member")
	follower, followerToken := createTestUser(t, db, "ev-vis-follower", "member")
	linked, linkedToken := createTestUser(t, db, "ev-vis-linked", "member")
	owner, _ := createTestUser(t, db, "ev-vis-owner", "member")

	host := createTestNode(t, db, owner.ID, "Host", "ev-vis-host", "open")
	band := createTestNode(t, db, owner.ID, "Band", "ev-vis-band", "open")
	createTestMembership(t, db, member.ID, host, "member", "active")
	createTestMembership(t, db, follower.ID, host, "follower", "active")
	createTestMembership(t, db, linked.ID, band, "member", "active")

	private := insertEvent(t, db, host, owner.ID, "Members Only", soon(10))
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, private); err != nil {
		t.Fatalf("set event private: %v", err)
	}
	linkEvent(t, db, private, band, "confirmed", owner.ID)

	if !listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, memberToken))[private] {
		t.Error("a member of the hosting patch should see its members-only event on their quilt")
	}
	if listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, followerToken))[private] {
		t.Error("a follower must not see a members-only event")
	}
	if listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, linkedToken))[private] {
		t.Error("a confirmed link must not widen a members-only event to the linked patch's members")
	}
}

// The unscoped, instance-wide feed stays public-only no matter who asks —
// wrapping the route in AuthOptional must not turn it into a personal feed.
func TestListEventsDefaultScopeStaysPublicOnly(t *testing.T) {
	db := setupTestDB(t)
	member, token := createTestUser(t, db, "ev-default-member", "member")
	owner, _ := createTestUser(t, db, "ev-default-owner", "member")

	host := createTestNode(t, db, owner.ID, "Host", "ev-default-host", "open")
	createTestMembership(t, db, member.ID, host, "member", "active")

	pub := insertEvent(t, db, host, owner.ID, "Open Night", soon(3))
	private := insertEvent(t, db, host, owner.ID, "Members Only", soon(4))
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, private); err != nil {
		t.Fatalf("set event private: %v", err)
	}

	ids := listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?limit=100", nil, token))
	if !ids[pub] {
		t.Error("expected the public event in the default listing")
	}
	if ids[private] {
		t.Error("the default listing must stay public-only even for a member")
	}
}

// My Quilt shows a private patch's events to the people who belong to it —
// the events surface has to agree with the quilt and the map (see
// TestListNodesScopeMyIncludesOwnPrivatePatches).
func TestListEventsScopeMyIncludesOwnPrivatePatches(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-priv-user", "member")
	owner, _ := createTestUser(t, db, "ev-priv-owner", "member")

	priv := createTestNode(t, db, owner.ID, "Private", "ev-priv-patch", "invite_only")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, priv); err != nil {
		t.Fatalf("set private: %v", err)
	}
	createTestMembership(t, db, user.ID, priv, "member", "active")
	ev := insertEvent(t, db, priv, owner.ID, "Private Patch Show", soon(5))

	if !listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token))[ev] {
		t.Error("expected my own private patch's event on my quilt")
	}
	// And it stays out of the instance-wide feed.
	if listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?limit=100", nil, token))[ev] {
		t.Error("a private patch's event must not reach the instance-wide feed")
	}
}

// Inactive memberships (pending, removed) are not belonging.
func TestListEventsScopeMyIgnoresInactiveMembership(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-inactive-user", "member")
	owner, _ := createTestUser(t, db, "ev-inactive-owner", "member")

	node := createTestNode(t, db, owner.ID, "Pending", "ev-inactive-node", "approval_required")
	createTestMembership(t, db, user.ID, node, "member", "pending")
	insertEvent(t, db, node, owner.ID, "Not Mine Yet", soon(3))

	ids := listScopedEventIDs(t, db, authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token))
	if len(ids) != 0 {
		t.Fatalf("a pending membership is not belonging, got %d events", len(ids))
	}
}

// scope=my without a session returns nothing — never the whole instance. The
// switcher only offers My Quilt to signed-in people, but `/events/my` is a
// plain URL that survives a sign-out, and other quilts read this endpoint
// cross-origin where no cookie ever rides along.
func TestListEventsScopeMyAnonymousReturnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "ev-anon-owner", "member")
	node := createTestNode(t, db, owner.ID, "Public", "ev-anon-node", "open")
	insertEvent(t, db, node, owner.ID, "Public Show", soon(3))

	r := httptest.NewRequest("GET", "/api/v1/events?scope=my&limit=100", nil)
	ids := listScopedEventIDs(t, db, r)
	if len(ids) != 0 {
		t.Fatalf("anonymous scope=my must return nothing, got %d events", len(ids))
	}
}

// scope=my composes with the date window rather than replacing it — the date
// filter and the scope switcher are independent controls on the same page.
func TestListEventsScopeMyComposesWithDateRange(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-date-user", "member")
	owner, _ := createTestUser(t, db, "ev-date-owner", "member")

	node := createTestNode(t, db, owner.ID, "Mine", "ev-date-node", "open")
	createTestMembership(t, db, user.ID, node, "member", "active")
	tonight := insertEvent(t, db, node, owner.ID, "Tonight", soon(3))
	later := insertEvent(t, db, node, owner.ID, "Next Month", soon(24*40))

	to := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	r := authedRequest("GET", "/api/v1/events?scope=my&limit=100&to="+to, nil, token)
	ids := listScopedEventIDs(t, db, r)

	if !ids[tonight] {
		t.Error("expected the near event within the window")
	}
	if ids[later] {
		t.Error("the date window must still apply under scope=my")
	}
}

// A person in both the hosting patch and a linked patch matches the
// relationship twice. The row must still appear once: a duplicate would
// double the list and corrupt the keyset cursor.
func TestListEventsScopeMyNoDuplicateRows(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "ev-dup-user", "member")
	owner, _ := createTestUser(t, db, "ev-dup-owner", "member")

	venue := createTestNode(t, db, owner.ID, "Venue", "ev-dup-venue", "open")
	band := createTestNode(t, db, owner.ID, "Band", "ev-dup-band", "open")
	createTestMembership(t, db, user.ID, venue, "member", "active")
	createTestMembership(t, db, user.ID, band, "follower", "active")

	gig := insertEvent(t, db, venue, owner.ID, "The Gig", soon(6))
	linkEvent(t, db, gig, band, "confirmed", owner.ID)

	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/events",
		handler.ListEvents(db), authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, token))
	var resp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected the event once, got %d rows", len(resp.Items))
	}
	if resp.Items[0].ID != gig {
		t.Fatalf("unexpected event %s", resp.Items[0].ID)
	}
}

func linkEvent(t *testing.T, db *database.DB, eventID, nodeID, status, requestedBy string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by)
		 VALUES (?, ?, ?, ?, 'owner', ?)`,
		auth.NewUUIDv7(), eventID, nodeID, status, requestedBy,
	)
	if err != nil {
		t.Fatalf("link event: %v", err)
	}
}
