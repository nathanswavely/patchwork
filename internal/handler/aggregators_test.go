package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func seedAggregatorRow(t *testing.T, db *database.DB, addedBy string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO aggregators (id, name, type, url, added_by, status)
		 VALUES (?, 'City Calendar', 'ics', 'https://127.0.0.1:9/city.ics', ?, 'ok')`,
		id, addedBy,
	); err != nil {
		t.Fatalf("seed aggregator: %v", err)
	}
	return id
}

func seedListing(t *testing.T, db *database.DB, aggregatorID, uid, nameKey, displayName, title string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO aggregator_listings (aggregator_id, uid, occurrence, name_key, display_name,
		 title, title_key, description, location, starts_at)
		 VALUES (?, ?, '', ?, ?, ?, ?, '', ?, ?)`,
		aggregatorID, uid, nameKey, displayName, title, eventsource.TitleKey(title), displayName,
		time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed listing: %v", err)
	}
}

func seedUnclaimedNode(t *testing.T, db *database.DB, name, slug string) string {
	t.Helper()
	var sentinel string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = '_system'`).Scan(&sentinel); err != nil {
		t.Fatalf("sentinel user: %v", err)
	}
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility,
		 membership_policy, status) VALUES (?, ?, ?, ?, '', 'leaf', 'public', 'open', 'unclaimed')`,
		id, sentinel, name, slug,
	); err != nil {
		t.Fatalf("seed unclaimed node: %v", err)
	}
	return id
}

// The load-bearing constraint of docs/adr/056: instance role alone never
// writes onto an autonomous patch's calendar. A claimed patch that has
// not opened its door to suggestions cannot be mapped by anyone but its
// own admins.
func TestCrosswalk_InstanceAdminCannotMapAClosedActivePatch(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "venueowner", "member")
	instanceAdmin, adminToken := createTestUser(t, db, "steward", "admin")
	nodeID := createTestNode(t, db, owner.ID, "The Trust", "the-trust", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET accept_event_suggestions = 0 WHERE id = ?`, nodeID)
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "the trust", "The Trust", "A Show")

	r := authedRequest("POST", "/api/v1/nodes/the-trust/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "the trust"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a patch not accepting suggestions, got %d: %s", w.Code, w.Body.String())
	}
}

// With the switch on, the patch said "suggest to me" — so the instance
// admin may point a name at it, and what arrives waits for that patch's
// own admins.
func TestCrosswalk_InstanceAdminMaySuggestToAnOpenPatch(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "openowner", "member")
	instanceAdmin, adminToken := createTestUser(t, db, "steward12", "admin")
	nodeID := createTestNode(t, db, owner.ID, "The Trust", "trust-open", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET accept_event_suggestions = 1 WHERE id = ?`, nodeID)
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "the trust", "The Trust", "A Show")

	r := authedRequest("POST", "/api/v1/nodes/trust-open/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "the trust"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if entry := decodeJSON(t, w); entry["suggests"] != true {
		t.Errorf("an entry the instance admin pointed here must suggest, not publish: %v", entry)
	}

	// And the patch can see it and stop it — the whole of why suggesting
	// is allowed without a handshake.
	r = authedRequest("GET", "/api/v1/nodes/trust-open/crosswalk", nil, ownerToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/crosswalk", handler.ListCrosswalk(db), r)
	items, _ := decodeJSON(t, w)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("the patch must see entries pointed at it; got %d", len(items))
	}
	entry, _ := items[0].(map[string]any)
	if entry["suggests"] != true || entry["added_by_name"] == "" {
		t.Errorf("the patch should see that this suggests and who pointed it: %v", entry)
	}

	// The instance admin can see what they set up — they manage every
	// patch's event sources already, and the constraint that matters is
	// enforced at creation, not at read.
	r = authedRequest("GET", "/api/v1/nodes/trust-open/crosswalk", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/crosswalk", handler.ListCrosswalk(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("an instance admin should see the entry they set up, got %d", w.Code)
	}
}

// A patch's own admin mapping their own patch still publishes directly —
// the switch changes who else may reach them, not what their own consent
// means (docs/adr/056).
func TestCrosswalk_SelfMappingStillPublishesDirectly(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "selfmapper", "member")
	nodeID := createTestNode(t, db, owner.ID, "The Trust", "trust-self", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET accept_event_suggestions = 1 WHERE id = ?`, nodeID)
	aggID := seedAggregatorRow(t, db, owner.ID)
	seedListing(t, db, aggID, "a@city", "the trust", "The Trust", "A Show")

	r := authedRequest("POST", "/api/v1/nodes/trust-self/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "the trust"}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if entry := decodeJSON(t, w); entry["suggests"] == true {
		t.Error("a patch mapping itself is standing consent and must publish, not queue")
	}
}

// Rejecting a fed suggestion must skip-list it, or the next sync brings
// it back and the same rejection is owed forever.
func TestReviewEvent_RejectingAFedSuggestionSkipListsIt(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "rejowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "The Trust", "trust-reject", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	aggID := seedAggregatorRow(t, db, owner.ID)
	entryID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key, suggests)
		 VALUES (?, ?, 'aggregator', 'https://c.example/x.ics#t', ?, ?, 'the trust', 1)`,
		entryID, nodeID, owner.ID, aggID,
	); err != nil {
		t.Fatalf("seed entry: %v", err)
	}
	eventID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at,
		 recurrence, visibility, status, source_id, source_uid, source_occurrence)
		 VALUES (?, ?, ?, 'Not ours', '', '', ?, '', 'public', 'pending_review', ?, 'a@city', '')`,
		eventID, nodeID, owner.ID, time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339), entryID,
	); err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}

	r := authedRequest("PATCH", "/api/v1/events/"+eventID+"/review",
		map[string]string{"action": "reject"}, ownerToken)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skips int
	db.QueryRow(
		`SELECT COUNT(*) FROM event_source_skips WHERE source_id = ? AND uid = 'a@city'`, entryID,
	).Scan(&skips)
	if skips != 1 {
		t.Error("rejecting a fed suggestion must skip-list its feed item, or it returns every hour")
	}
}

func TestCrosswalk_PatchAdminMapsTheirOwnPatch(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "trustadmin", "member")
	instanceAdmin, _ := createTestUser(t, db, "steward2", "admin")
	nodeID := createTestNode(t, db, owner.ID, "The Trust", "trust-two", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "the trust", "The Trust", "A Show")

	r := authedRequest("POST", "/api/v1/nodes/trust-two/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "the trust"}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	entry := decodeJSON(t, w)
	if entry["aggregator_id"] != aggID || entry["name_key"] != "the trust" {
		t.Errorf("created entry: %v", entry)
	}
	if entry["aggregator_name"] != "City Calendar" {
		t.Errorf("the entry should name its aggregator for display: %v", entry)
	}
}

func TestCrosswalk_InstanceAdminMapsUnclaimedPatch(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, adminToken := createTestUser(t, db, "steward3", "admin")
	seedUnclaimedNode(t, db, "Binns Park", "binns-park")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "binns park", "Binns Park", "Music Friday")

	r := authedRequest("POST", "/api/v1/nodes/binns-park/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "binns park"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for an unclaimed patch held in trust, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCrosswalk_MemberCannotMap(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner4", "member")
	member, memberToken := createTestUser(t, db, "member4", "member")
	nodeID := createTestNode(t, db, owner.ID, "Venue", "venue-four", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	aggID := seedAggregatorRow(t, db, owner.ID)
	seedListing(t, db, aggID, "a@city", "venue", "Venue", "A Show")

	r := authedRequest("POST", "/api/v1/nodes/venue-four/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "venue"}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for a member, got %d", w.Code)
	}
}

func TestCrosswalk_UnknownNameIsRejected(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "owner5", "member")
	nodeID := createTestNode(t, db, owner.ID, "Venue", "venue-five", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	aggID := seedAggregatorRow(t, db, owner.ID)

	r := authedRequest("POST", "/api/v1/nodes/venue-five/crosswalk",
		map[string]string{"aggregator_id": aggID, "name_key": "no such name"}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/crosswalk", handler.CreateCrosswalkEntry(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a name the aggregator isn't carrying, got %d", w.Code)
	}
}

// Unrouted is a resting state, not a queue: the names an admin should
// never map ("PA", "3rd floor atrium") sit here forever, and the list is
// ordered so a handful of entries covers most of a city calendar.
func TestUnroutedNames_CommonestFirstAndExcludesMapped(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, adminToken := createTestUser(t, db, "steward6", "admin")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "binns park", "Binns Park", "Show A")
	seedListing(t, db, aggID, "b@city", "binns park", "Binns Park", "Show B")
	seedListing(t, db, aggID, "c@city", "binns park", "Binns Park", "Show C")
	seedListing(t, db, aggID, "d@city", "the conway room", "The Conway Room", "Study Hall")
	seedListing(t, db, aggID, "e@city", "pa", "PA", "Somewhere")

	nodeID := seedUnclaimedNode(t, db, "The Conway Room", "conway")
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key)
		 VALUES (?, ?, 'aggregator', '#the-conway-room', ?, ?, 'the conway room')`,
		auth.NewUUIDv7(), nodeID, instanceAdmin.ID, aggID,
	); err != nil {
		t.Fatalf("seed crosswalk entry: %v", err)
	}

	r := authedRequest("GET", "/api/v1/admin/aggregator-names", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/aggregator-names", handler.AdminListUnroutedNames(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 unrouted names (the mapped one drops out), got %d: %v", len(items), items)
	}
	first, _ := items[0].(map[string]any)
	if first["name_key"] != "binns park" {
		t.Errorf("commonest name should lead, got %v", first["name_key"])
	}
	if first["display_name"] != "Binns Park" {
		t.Errorf("the admin reads what the feed wrote, not the key: %v", first["display_name"])
	}
	if count, _ := first["count"].(float64); count != 3 {
		t.Errorf("expected a count of 3, got %v", first["count"])
	}
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["name_key"] == "the conway room" {
			t.Error("a mapped name is no longer unrouted")
		}
	}
}

// Ignoring is the instance admin's view of their own screen. It must not
// reach a patch's picker: whether a patch answers to a name is that
// patch's judgement, and this would otherwise be a quiet way for
// instance authority to pre-empt it (docs/adr/056).
func TestIgnoredName_HiddenFromAdminOnlyNotFromPatches(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "owner9", "member")
	instanceAdmin, adminToken := createTestUser(t, db, "steward9", "admin")
	nodeID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "west art", "West Art", "Improv Night")
	seedListing(t, db, aggID, "b@city", "pa", "PA", "First Friday")

	r := authedRequest("POST", "/api/v1/admin/aggregator-names/ignore",
		map[string]string{"aggregator_id": aggID, "name_key": "pa"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/admin/aggregator-names/ignore",
		handler.AdminIgnoreName(db, true), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Gone from the admin's working list.
	r = authedRequest("GET", "/api/v1/admin/aggregator-names", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/aggregator-names", handler.AdminListUnroutedNames(db), r)
	working, _ := decodeJSON(t, w)["items"].([]any)
	for _, it := range working {
		if m, _ := it.(map[string]any); m["name_key"] == "pa" {
			t.Error("an ignored name must leave the admin's working list")
		}
	}
	if len(working) != 1 {
		t.Errorf("expected 1 name left, got %d", len(working))
	}

	// Present in the ignored view, so the judgement is revisitable.
	r = authedRequest("GET", "/api/v1/admin/aggregator-names?ignored=true", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/aggregator-names", handler.AdminListUnroutedNames(db), r)
	setAside, _ := decodeJSON(t, w)["items"].([]any)
	if len(setAside) != 1 {
		t.Fatalf("expected 1 ignored name, got %d", len(setAside))
	}
	if m, _ := setAside[0].(map[string]any); m["ignored"] != true {
		t.Errorf("the ignored view should mark its rows: %v", setAside[0])
	}

	// Still offered to the patch's own admins.
	r = authedRequest("GET", "/api/v1/nodes/west-art/aggregator-names", nil, ownerToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/aggregator-names", handler.ListAggregatorNames(db), r)
	picker, _ := decodeJSON(t, w)["items"].([]any)
	if len(picker) != 2 {
		t.Errorf("a patch's picker must not be filtered by the instance admin's ignore list; got %d of 2", len(picker))
	}
}

func TestIgnoredName_Reversible(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, adminToken := createTestUser(t, db, "steward10", "admin")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	seedListing(t, db, aggID, "a@city", "pa", "PA", "First Friday")

	r := authedRequest("POST", "/api/v1/admin/aggregator-names/ignore",
		map[string]string{"aggregator_id": aggID, "name_key": "pa"}, adminToken)
	serveMux(t, db, "POST", "/api/v1/admin/aggregator-names/ignore", handler.AdminIgnoreName(db, true), r)

	r = authedRequest("POST", "/api/v1/admin/aggregator-names/unignore",
		map[string]string{"aggregator_id": aggID, "name_key": "pa"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/admin/aggregator-names/unignore", handler.AdminIgnoreName(db, false), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	r = authedRequest("GET", "/api/v1/admin/aggregator-names", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/aggregator-names", handler.AdminListUnroutedNames(db), r)
	items, _ := decodeJSON(t, w)["items"].([]any)
	if len(items) != 1 {
		t.Errorf("un-ignoring should return the name to the working list; got %d", len(items))
	}

	// The listings themselves were never touched.
	var listings int
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_listings WHERE aggregator_id = ?`, aggID).Scan(&listings)
	if listings != 1 {
		t.Errorf("ignoring must not delete listings; got %d", listings)
	}
}

func TestNameListings_ReturnsWhatTheFeedPublished(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, adminToken := createTestUser(t, db, "steward11", "admin")
	aggID := seedAggregatorRow(t, db, instanceAdmin.ID)
	if _, err := db.Exec(
		`INSERT INTO aggregator_listings (aggregator_id, uid, occurrence, name_key, display_name,
		 title, description, location, starts_at, url)
		 VALUES (?, 'a@city', '', 'west art', 'West Art', 'Improv Night',
		 'Comedy every third Thursday.', 'West Art, 800 Buchanan Ave', ?, 'https://calendar.example/detail/1')`,
		aggID, time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed listing: %v", err)
	}

	r := authedRequest("GET",
		"/api/v1/admin/aggregator-listings?aggregator_id="+aggID+"&name_key=west+art", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/aggregator-listings", handler.AdminListNameListings(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	items, _ := decodeJSON(t, w)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 listing, got %d", len(items))
	}
	l, _ := items[0].(map[string]any)
	if l["title"] != "Improv Night" || l["description"] != "Comedy every third Thursday." {
		t.Errorf("listing should carry what the feed published: %v", l)
	}
	if l["url"] != "https://calendar.example/detail/1" {
		t.Errorf("the publisher's own page is the point of this view: %v", l["url"])
	}
}

func TestAggregator_AttachedFeedCreatesNoEvents(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "steward7", "admin")

	r := authedRequest("POST", "/api/v1/admin/aggregators",
		map[string]string{"name": "City Calendar", "url": "https://127.0.0.1:9/city.ics"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/admin/aggregators", handler.AdminCreateAggregator(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	agg := decodeJSON(t, w)
	if agg["name"] != "City Calendar" {
		t.Errorf("created aggregator: %v", agg)
	}
	// An aggregator owns nothing and has no tile: attaching one must not
	// create a patch anywhere on the quilt (docs/adr/056).
	var nodes int
	db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&nodes)
	if nodes != 0 {
		t.Errorf("attaching an aggregator must not mint a patch; found %d", nodes)
	}
}

func TestCrosswalkEntriesDoNotSpendTheFeedBudget(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "owner8", "member")
	nodeID := createTestNode(t, db, owner.ID, "Venue", "venue-eight", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	aggID := seedAggregatorRow(t, db, owner.ID)

	// Five crosswalk entries — the whole event-source cap — must leave
	// the patch's own feed budget untouched.
	for i, name := range []string{"one", "two", "three", "four", "five"} {
		seedListing(t, db, aggID, name+"@city", name, name, "Show")
		if _, err := db.Exec(
			`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key)
			 VALUES (?, ?, 'aggregator', ?, ?, ?, ?)`,
			auth.NewUUIDv7(), nodeID, "https://127.0.0.1:9/city.ics#"+name, owner.ID, aggID, name,
		); err != nil {
			t.Fatalf("seed crosswalk entry %d: %v", i, err)
		}
	}

	r := authedRequest("POST", "/api/v1/nodes/venue-eight/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/mine.ics"}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("crosswalk entries must not count against maxSourcesPerNode, got %d: %s", w.Code, w.Body.String())
	}

	// And the patch's own source list shows only its own feed.
	r = authedRequest("GET", "/api/v1/nodes/venue-eight/event-sources", nil, ownerToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-sources", handler.ListEventSources(db), r)
	body := decodeJSON(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Errorf("event sources and crosswalk entries are different things; got %d in the source list", len(items))
	}
}
