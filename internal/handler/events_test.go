package handler_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// seedEvent inserts a public event with an explicit UUIDv7, so callers control
// creation order (id order) independently of starts_at order.
func seedEvent(t *testing.T, db *database.DB, nodeID, creatorID, title, startsAt string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility)
		 VALUES (?, ?, ?, ?, '', '', ?, '', 'public')`,
		id, nodeID, creatorID, title, startsAt,
	)
	if err != nil {
		t.Fatalf("seed event %s: %v", title, err)
	}
	return id
}

// pageThroughEvents walks the whole list endpoint with the given page size and
// returns every title it was served, in order, following next_cursor each time.
func pageThroughEvents(t *testing.T, db *database.DB, query string, limit int) []string {
	t.Helper()
	var titles []string
	cursor := ""
	for page := 0; ; page++ {
		if page > 20 {
			t.Fatalf("pagination did not terminate after 20 pages (served %d rows) — cursor is looping", len(titles))
		}
		path := fmt.Sprintf("/api/v1/events?limit=%d&%s", limit, query)
		if cursor != "" {
			path += "&after=" + url.QueryEscape(cursor)
		}
		r := authedRequest("GET", path, nil, "")
		w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("page %d: expected 200, got %d: %s", page, w.Code, w.Body.String())
		}

		result := decodeJSON(t, w)
		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatalf("page %d: expected items array, got %T", page, result["items"])
		}
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			if !ok {
				t.Fatalf("page %d: expected item object, got %T", page, it)
			}
			titles = append(titles, m["title"].(string))
		}

		next, _ := result["next_cursor"].(string)
		if next == "" {
			return titles
		}
		if next == cursor {
			t.Fatalf("page %d: next_cursor did not advance (%q) — cursor is looping", page, next)
		}
		cursor = next
	}
}

// TestListEvents_PaginationCoversAllRows is the regression test for the keyset
// bug where the cursor filtered on e.id while the query ordered by e.starts_at.
// Because UUIDv7 ids sort by creation time, seeding events whose creation order
// is the reverse of their start-date order made page 2 drop nearly every row.
func TestListEvents_PaginationCoversAllRows(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-pager", "member")

	// Two patches, so this exercises the default cross-patch feed where the two
	// orderings are least likely to agree.
	nodeA := createTestNode(t, db, user.ID, "Patch A", "patch-a", "open")
	nodeB := createTestNode(t, db, user.ID, "Patch B", "patch-b", "open")

	// Insert in ascending id order but descending starts_at order: the event
	// created first starts last. Under the old predicate, the page-1 boundary id
	// excluded every event created before it — i.e. all the later pages.
	const total = 9
	want := make([]string, 0, total)
	for i := 0; i < total; i++ {
		node := nodeA
		if i%2 == 1 {
			node = nodeB
		}
		title := fmt.Sprintf("event-%02d", total-1-i)
		seedEvent(t, db, node, user.ID, title, fmt.Sprintf("2026-09-%02dT18:00:00Z", total-i))
	}
	// Expected order is by starts_at ascending, which is reverse insertion order.
	for i := 0; i < total; i++ {
		want = append(want, fmt.Sprintf("event-%02d", i))
	}

	got := pageThroughEvents(t, db, "", 3)

	if len(got) != total {
		t.Fatalf("expected %d events across all pages, got %d: %v", total, len(got), got)
	}
	seen := map[string]int{}
	for _, title := range got {
		seen[title]++
	}
	for _, title := range want {
		switch seen[title] {
		case 1:
			// served exactly once, as required
		case 0:
			t.Errorf("event %q was never served — pagination skipped it", title)
		default:
			t.Errorf("event %q was served %d times — pagination repeated it", title, seen[title])
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events out of starts_at order at index %d: got %v, want %v", i, got, want)
		}
	}
}

// TestListEvents_PaginationHandlesStartsAtTies covers the tiebreaker half of the
// composite cursor: when several events share a starts_at, the id component has
// to carry the boundary or the page break drops or repeats the tied rows.
func TestListEvents_PaginationHandlesStartsAtTies(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-tied", "member")
	nodeID := createTestNode(t, db, user.ID, "Tied Patch", "tied-patch", "open")

	// All six share one starts_at, so every page break lands inside the tie.
	const total = 6
	for i := 0; i < total; i++ {
		seedEvent(t, db, nodeID, user.ID, fmt.Sprintf("tied-%02d", i), "2026-10-01T20:00:00Z")
	}

	got := pageThroughEvents(t, db, "", 2)

	if len(got) != total {
		t.Fatalf("expected %d events across all pages, got %d: %v", total, len(got), got)
	}
	seen := map[string]bool{}
	for _, title := range got {
		if seen[title] {
			t.Errorf("event %q served more than once", title)
		}
		seen[title] = true
	}
	for i := 0; i < total; i++ {
		if title := fmt.Sprintf("tied-%02d", i); !seen[title] {
			t.Errorf("event %q was never served", title)
		}
	}
}

// listEventTitles fetches one page of the list endpoint with the given query
// string and returns the served titles.
func listEventTitles(t *testing.T, db *database.DB, query string) []string {
	t.Helper()
	path := "/api/v1/events"
	if query != "" {
		path += "?" + query
	}
	r := authedRequest("GET", path, nil, "")
	w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
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

// TestListEvents_PrivatePatchHiddenFromGlobalList: a private patch is unlisted,
// not locked — its public events stay off the instance-wide feed (and the map,
// which reads the same endpoint) but still serve on the patch's own page via
// the node_slug filter.
func TestListEvents_PrivatePatchHiddenFromGlobalList(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-private", "member")
	publicNode := createTestNode(t, db, user.ID, "Public Patch", "public-patch", "open")
	privateNode := createTestNode(t, db, user.ID, "Private Patch", "private-patch", "open")
	if _, err := db.Exec("UPDATE nodes SET visibility = 'private' WHERE id = ?", privateNode); err != nil {
		t.Fatalf("set private: %v", err)
	}

	seedEvent(t, db, publicNode, user.ID, "public-event", "2026-09-01T18:00:00Z")
	seedEvent(t, db, privateNode, user.ID, "private-patch-event", "2026-09-02T18:00:00Z")

	got := listEventTitles(t, db, "")
	if len(got) != 1 || got[0] != "public-event" {
		t.Fatalf("global list should carry only the public patch's event, got %v", got)
	}

	got = listEventTitles(t, db, "node_slug=private-patch")
	if len(got) != 1 || got[0] != "private-patch-event" {
		t.Fatalf("private patch's own page should still list its events, got %v", got)
	}
}

// TestListEvents_ArchivedPatchEventsGone: archiving a patch sets
// status='archived' but leaves its events active — the node-status gate is
// what keeps them out of every listing, global or node-filtered.
func TestListEvents_ArchivedPatchEventsGone(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-archived", "member")
	liveNode := createTestNode(t, db, user.ID, "Live Patch", "live-patch", "open")
	deadNode := createTestNode(t, db, user.ID, "Dead Patch", "dead-patch", "open")

	seedEvent(t, db, liveNode, user.ID, "live-event", "2026-09-01T18:00:00Z")
	seedEvent(t, db, deadNode, user.ID, "dead-event", "2026-09-02T18:00:00Z")

	if _, err := db.Exec("UPDATE nodes SET status = 'archived' WHERE id = ?", deadNode); err != nil {
		t.Fatalf("archive node: %v", err)
	}

	got := listEventTitles(t, db, "")
	if len(got) != 1 || got[0] != "live-event" {
		t.Fatalf("archived patch's event should vanish from the global list, got %v", got)
	}
	if got := listEventTitles(t, db, "node_id="+deadNode); len(got) != 0 {
		t.Fatalf("archived patch's event should vanish from the node-filtered list, got %v", got)
	}
	if got := listEventTitles(t, db, "node_slug=dead-patch"); len(got) != 0 {
		t.Fatalf("archived patch's slug should resolve to no events, got %v", got)
	}
}

// TestGetEvent_GoneWithArchivedPatch: an event link must not outlive its
// patch — GetNode 404s an archived patch, and the event detail agrees.
func TestGetEvent_GoneWithArchivedPatch(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-detail", "member")
	nodeID := createTestNode(t, db, user.ID, "Detail Patch", "detail-patch", "open")
	eventID := seedEvent(t, db, nodeID, user.ID, "detail-event", "2026-09-01T18:00:00Z")

	r := authedRequest("GET", "/api/v1/events/"+eventID, nil, "")
	w := servePublicMux(t, "GET", "/api/v1/events/{id}", handler.GetEvent(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 before archive, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := db.Exec("UPDATE nodes SET status = 'archived' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("archive node: %v", err)
	}

	r = authedRequest("GET", "/api/v1/events/"+eventID, nil, "")
	w = servePublicMux(t, "GET", "/api/v1/events/{id}", handler.GetEvent(db), r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after archive, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListEvents_MalformedCursor ensures a garbage cursor degrades to the first
// page instead of binding junk into the keyset predicate.
func TestListEvents_MalformedCursor(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "events-badcursor", "member")
	nodeID := createTestNode(t, db, user.ID, "Cursor Patch", "cursor-patch", "open")
	seedEvent(t, db, nodeID, user.ID, "only-event", "2026-11-01T12:00:00Z")

	r := authedRequest("GET", "/api/v1/events?after=not-a-valid-cursor", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	items, ok := decodeJSON(t, w)["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected malformed cursor to serve the first page (1 event), got %v", items)
	}
}
