package eventsource

import (
	"context"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// veventAt builds a VEVENT with a LOCATION, which is what a crosswalk
// entry keys on (docs/adr/056).
func veventAt(uid, summary, dtstart, location string) string {
	return "BEGIN:VEVENT\nUID:" + uid + "\nSUMMARY:" + summary +
		"\nDTSTART:" + dtstart + "\nLOCATION:" + location + "\nEND:VEVENT\n"
}

func seedUser(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role) VALUES (?, ?, ?, 'member')`,
		id, username, username,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status)
		 VALUES (?, ?, ?, ?, '', 'leaf', 'public', 'open', 'active')`,
		id, ownerID, name, slug,
	); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	return id
}

func seedAggregator(t *testing.T, db *database.DB, userID, feedURL string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO aggregators (id, name, type, url, added_by) VALUES (?, 'City Calendar', 'ics', ?, ?)`,
		id, feedURL, userID,
	); err != nil {
		t.Fatalf("seed aggregator: %v", err)
	}
	return id
}

func mapName(t *testing.T, db *database.DB, aggregatorID, nodeID, userID, nameKey string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key)
		 VALUES (?, ?, 'aggregator', ?, ?, ?, ?)`,
		id, nodeID, "https://city.example/cal.ics#"+nameKey, userID, aggregatorID, nameKey,
	); err != nil {
		t.Fatalf("map name: %v", err)
	}
	return id
}

func TestNameKey(t *testing.T) {
	// The four spellings Binns Park reaches Lancaster's city calendar by
	// must collapse to one key, and the two typo'd Gardner Theatres must
	// not: closing "Scool" against "School" would mean guessing.
	cases := []struct{ in, want string }{
		{"Binns Park\\,100 N Queen St\\, Lancaster\\, PA 17603\\, USA", "binns park"},
		{"Binns Park, 120 N Queen St, Lanacaster, Pennsylvania", "binns park"},
		{"  binns   park  ", "binns park"},
		{"West Art\\, 800 Buchanan Ave", "west art"},
		{"South County Brewing Co.\\, 26 W King St", "south county brewing co"},
		{"The Conway Room\\,28-30 E King Street", "the conway room"},
		{"Lancaster City", "lancaster city"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NameKey(c.in); got != c.want {
			t.Errorf("NameKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if NameKey("Gardner Theatre at Lancaster Country Day School") ==
		NameKey("Gardner Theatre at Lancaster Country Day Scool") {
		t.Error("a typo must not collapse into its neighbour — that would be guessing")
	}
}

func TestAggregator_CreatesNothingUntilMapped(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park")+
			veventAt("b@city", "Show B", future(72*time.Hour), "The Conway Room")))
	userID := seedUser(t, db, "steward")
	seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)

	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("sync aggregator: %v", err)
	}

	var listings, events int
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_listings WHERE aggregator_id = ?`, aggID).Scan(&listings)
	db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events)
	if listings != 2 {
		t.Errorf("expected 2 cached listings, got %d", listings)
	}
	if events != 0 {
		t.Errorf("an aggregator owns nothing and creates nothing until a crosswalk entry addresses a name; got %d events", events)
	}
}

func TestAggregator_RoutesOnlyTheMappedName(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park\\, 100 N Queen St")+
			veventAt("b@city", "Show B", future(72*time.Hour), "Binns Park\\, 120 N Queen St")+
			veventAt("c@city", "Elsewhere", future(96*time.Hour), "The Conway Room")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("sync aggregator: %v", err)
	}

	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	// Both address spellings normalize to one key, so both events land.
	if n := countEvents(t, db, entryID); n != 2 {
		t.Errorf("expected both Binns Park spellings to route, got %d events", n)
	}
	var others int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id IS NULL OR source_id != ?`, entryID).Scan(&others)
	if others != 0 {
		t.Errorf("an unmapped name must create nothing; got %d stray events", others)
	}
}

func TestAggregator_BackfillIsSilentThenAnnounces(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("sync aggregator: %v", err)
	}

	// The aggregator has already succeeded once. Announcement must still
	// anchor on the crosswalk entry, or a venue opting in months later
	// fires a notification for every back-listing (docs/adr/056).
	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	var lastSuccess *string
	db.QueryRow(`SELECT last_success_at FROM event_sources WHERE id = ?`, entryID).Scan(&lastSuccess)
	if lastSuccess != nil {
		t.Fatal("a fresh crosswalk entry must have no last_success_at")
	}
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("route: %v", err)
	}
	db.QueryRow(`SELECT last_success_at FROM event_sources WHERE id = ?`, entryID).Scan(&lastSuccess)
	if lastSuccess == nil {
		t.Fatal("the entry should have recorded its first routing pass")
	}

	// Whatever arrives after that first pass is news.
	feed.set(wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park")+
			veventAt("b@city", "Show B", future(96*time.Hour), "Binns Park")), 200)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-route: %v", err)
	}
	if n := countEvents(t, db, entryID); n != 2 {
		t.Errorf("expected the new listing to route, got %d events", n)
	}
}

func TestAggregator_HoldsDuplicateAtSameStart(t *testing.T) {
	db := setupTestDB(t)
	start := future(48 * time.Hour)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Music Friday hosted by Music For Everyone", start, "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("sync aggregator: %v", err)
	}

	// The patch already posted its own version of the same night. Titles
	// differ; the start instant is the signal.
	var listingStart string
	db.QueryRow(`SELECT starts_at FROM aggregator_listings WHERE aggregator_id = ?`, aggID).Scan(&listingStart)
	rivalID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location,
		 starts_at, recurrence, visibility, status)
		 VALUES (?, ?, ?, 'Music Friday', '', '', ?, '', 'public', 'active')`,
		rivalID, nodeID, userID, listingStart,
	); err != nil {
		t.Fatalf("seed rival event: %v", err)
	}

	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("route: %v", err)
	}

	if n := countEvents(t, db, entryID); n != 0 {
		t.Errorf("a colliding listing must be held, not created; got %d events", n)
	}
	var holds int
	var heldRival string
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_holds WHERE source_id = ?`, entryID).Scan(&holds)
	if holds != 1 {
		t.Fatalf("expected 1 hold, got %d", holds)
	}
	db.QueryRow(`SELECT rival_event_id FROM aggregator_holds WHERE source_id = ?`, entryID).Scan(&heldRival)
	if heldRival != rivalID {
		t.Errorf("the hold should name the patch's own event as the rival")
	}

	// A hold is a pending question, not a fresh judgement every hour.
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-route: %v", err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_holds WHERE source_id = ?`, entryID).Scan(&holds)
	if holds != 1 {
		t.Errorf("re-syncing must not multiply holds; got %d", holds)
	}
}

func TestAggregator_HoldClearsWhenTheListingLeavesTheFeed(t *testing.T) {
	db := setupTestDB(t)
	start := future(48 * time.Hour)
	feed := newFeedServer(t, wrap(veventAt("a@city", "Show A", start, "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)

	var listingStart string
	db.QueryRow(`SELECT starts_at FROM aggregator_listings WHERE aggregator_id = ?`, aggID).Scan(&listingStart)
	db.Exec(`INSERT INTO events (id, node_id, created_by, title, description, location,
	         starts_at, recurrence, visibility, status)
	         VALUES (?, ?, ?, 'Ours', '', '', ?, '', 'public', 'active')`,
		auth.NewUUIDv7(), nodeID, userID, listingStart)
	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	SyncAggregator(context.Background(), db, nil, aggID)

	feed.set(wrap(veventAt("z@city", "Something Else", future(96*time.Hour), "Elsewhere")), 200)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	var holds int
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_holds WHERE source_id = ?`, entryID).Scan(&holds)
	if holds != 0 {
		t.Errorf("a held listing the feed dropped is no longer a question; got %d holds", holds)
	}
}

func suggestName(t *testing.T, db *database.DB, aggregatorID, nodeID, userID, nameKey string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key, suggests)
		 VALUES (?, ?, 'aggregator', ?, ?, ?, ?, 1)`,
		id, nodeID, "https://city.example/cal.ics#"+nameKey, userID, aggregatorID, nameKey,
	); err != nil {
		t.Fatalf("suggest name: %v", err)
	}
	return id
}

// A patch that opened its door to suggestions gets suggestions, not a
// published calendar: nothing appears publicly until its own admins say
// so (docs/adr/056).
func TestAggregator_SuggestingEntryRoutesToReview(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Chanticleer", future(48*time.Hour), "The Trust")+
			veventAt("b@city", "Pianist Jiarui Li", future(72*time.Hour), "The Trust")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "The Trust", "the-trust")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)

	entryID := suggestName(t, db, aggID, nodeID, userID, "the trust")
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("route: %v", err)
	}

	var pending, active int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ? AND status = 'pending_review'`, entryID).Scan(&pending)
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ? AND status = 'active'`, entryID).Scan(&active)
	if pending != 2 {
		t.Errorf("expected 2 suggestions awaiting review, got %d", pending)
	}
	if active != 0 {
		t.Errorf("a suggesting entry must publish nothing; got %d active", active)
	}

	// Re-syncing must not duplicate what is already waiting.
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-route: %v", err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ?`, entryID).Scan(&pending)
	if pending != 2 {
		t.Errorf("re-sync should match pending events, not re-insert; got %d", pending)
	}
}

// A rejected suggestion must not come back on the next sync, or the same
// rejection is owed forever.
func TestAggregator_RejectedSuggestionStaysGone(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Not ours", future(48*time.Hour), "The Trust")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "The Trust", "the-trust")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	entryID := suggestName(t, db, aggID, nodeID, userID, "the trust")
	SyncAggregator(context.Background(), db, nil, aggID)

	var eventID, uid string
	if err := db.QueryRow(
		`SELECT id, source_uid FROM events WHERE source_id = ?`, entryID,
	).Scan(&eventID, &uid); err != nil {
		t.Fatalf("load suggestion: %v", err)
	}

	// What the reject branch of ReviewEvent does.
	db.Exec(`INSERT OR IGNORE INTO event_source_skips (source_id, uid, occurrence) VALUES (?, ?, '')`, entryID, uid)
	db.Exec(`DELETE FROM events WHERE id = ?`, eventID)

	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	var back int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ?`, entryID).Scan(&back)
	if back != 0 {
		t.Errorf("a rejected suggestion must stay rejected; %d came back", back)
	}
}

func TestUnmap_DropsPendingKeepsAccepted(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Approved one", future(48*time.Hour), "The Trust")+
			veventAt("b@city", "Still waiting", future(72*time.Hour), "The Trust")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "The Trust", "the-trust")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	entryID := suggestName(t, db, aggID, nodeID, userID, "the trust")
	SyncAggregator(context.Background(), db, nil, aggID)

	// The patch's admins approved one of the two.
	db.Exec(`UPDATE events SET status = 'active' WHERE source_id = ? AND title = 'Approved one'`, entryID)

	if err := Unmap(db, entryID); err != nil {
		t.Fatalf("unmap: %v", err)
	}

	var kept, pending int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ? AND status = 'active'`, nodeID).Scan(&kept)
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ? AND status = 'pending_review'`, nodeID).Scan(&pending)
	if kept != 1 {
		t.Errorf("an approved suggestion is the patch's own event and must survive; got %d", kept)
	}
	if pending != 0 {
		t.Errorf("suggestions nobody acted on go with the entry; %d stranded in the queue", pending)
	}
}

func TestUnmap_DetachesRatherThanDeletes(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Past Show", "20200101T190000Z", "Binns Park")+
			veventAt("b@city", "Future Show", future(72*time.Hour), "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	SyncAggregator(context.Background(), db, nil, aggID)

	before := countEvents(t, db, entryID)
	if before == 0 {
		t.Fatal("expected the mapped name to route at least one event")
	}

	if err := Unmap(db, entryID); err != nil {
		t.Fatalf("unmap: %v", err)
	}

	// The departure from ADR 031's Remove: the patch consented to this
	// name individually, so unmapping must not empty its calendar.
	var kept int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ? AND source_id IS NULL`, nodeID).Scan(&kept)
	if kept != before {
		t.Errorf("unmapping must detach all %d routed events, not delete them; %d survived", before, kept)
	}
	var entries int
	db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE id = ?`, entryID).Scan(&entries)
	if entries != 0 {
		t.Errorf("the crosswalk entry should be gone")
	}
}

func TestRemoveAggregator_LeavesEveryPatchItsCalendar(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "At the Park", future(48*time.Hour), "Binns Park")+
			veventAt("b@city", "At the Room", future(72*time.Hour), "The Conway Room")))
	userID := seedUser(t, db, "steward")
	parkID := seedNode(t, db, userID, "Binns Park", "binns-park")
	roomID := seedNode(t, db, userID, "The Conway Room", "conway-room")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	mapName(t, db, aggID, parkID, userID, "binns park")
	mapName(t, db, aggID, roomID, userID, "the conway room")
	SyncAggregator(context.Background(), db, nil, aggID)

	if err := RemoveAggregator(db, aggID); err != nil {
		t.Fatalf("remove aggregator: %v", err)
	}

	for _, id := range []string{parkID, roomID} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ? AND source_id IS NULL`, id).Scan(&n)
		if n != 1 {
			t.Errorf("no patch should lose a calendar because the instance admin unplugged a feed; node %s kept %d", id, n)
		}
	}
	var aggs int
	db.QueryRow(`SELECT COUNT(*) FROM aggregators WHERE id = ?`, aggID).Scan(&aggs)
	if aggs != 0 {
		t.Error("the aggregator should be gone")
	}
}

func TestAggregator_PausedStillRoutesFromCache(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")

	// Pausing stops the fetch, not the patches that already consented.
	db.Exec(`UPDATE aggregators SET paused = 1 WHERE id = ?`, aggID)
	feed.set("", 500)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err != nil {
		t.Fatalf("paused sync should route without fetching: %v", err)
	}
	if n := countEvents(t, db, entryID); n != 1 {
		t.Errorf("expected routing from cached listings while paused, got %d events", n)
	}
	status, _ := aggregatorState(t, db, aggID)
	if status == "error" {
		t.Error("a paused aggregator must not record a fetch error — it never fetched")
	}
}

func TestAggregator_UnreachableFeedKeepsCachedListings(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		veventAt("a@city", "Show A", future(48*time.Hour), "Binns Park")))
	userID := seedUser(t, db, "steward")
	nodeID := seedNode(t, db, userID, "Binns Park", "binns-park")
	aggID := seedAggregator(t, db, userID, feed.srv.URL)
	SyncAggregator(context.Background(), db, nil, aggID)
	entryID := mapName(t, db, aggID, nodeID, userID, "binns park")
	SyncAggregator(context.Background(), db, nil, aggID)

	feed.set("", 500)
	if err := SyncAggregator(context.Background(), db, nil, aggID); err == nil {
		t.Fatal("expected the failed fetch to surface")
	}
	// ADR 031's rule, inherited: an expired cert must not empty forty
	// patches at once.
	if n := countEvents(t, db, entryID); n != 1 {
		t.Errorf("a failed fetch must touch no events; got %d", n)
	}
	var listings int
	db.QueryRow(`SELECT COUNT(*) FROM aggregator_listings WHERE aggregator_id = ?`, aggID).Scan(&listings)
	if listings != 1 {
		t.Errorf("cached listings must survive a failed fetch; got %d", listings)
	}
	status, lastError := aggregatorState(t, db, aggID)
	if status != "error" || lastError == nil {
		t.Errorf("the failure belongs on the aggregator row: %s / %v", status, lastError)
	}
}

func aggregatorState(t *testing.T, db *database.DB, id string) (string, *string) {
	t.Helper()
	var status string
	var lastError *string
	if err := db.QueryRow(`SELECT status, last_error FROM aggregators WHERE id = ?`, id).Scan(&status, &lastError); err != nil {
		t.Fatalf("aggregator state: %v", err)
	}
	return status, lastError
}
