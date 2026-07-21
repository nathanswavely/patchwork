package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// seedImportedEvent inserts an event carrying source provenance, as the
// sync reconciler would have written it.
func seedImportedEvent(t *testing.T, db *database.DB, nodeID, creatorID, sourceID, uid, title, startsAt string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at,
		 recurrence, visibility, status, source_id, source_uid, source_occurrence)
		 VALUES (?, ?, ?, ?, '', '', ?, '', 'public', 'active', ?, ?, '')`,
		id, nodeID, creatorID, title, startsAt, sourceID, uid,
	)
	if err != nil {
		t.Fatalf("seed imported event: %v", err)
	}
	return id
}

func seedEventSource(t *testing.T, db *database.DB, nodeID, addedBy, url string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
		id, nodeID, url, addedBy,
	); err != nil {
		t.Fatalf("seed event source: %v", err)
	}
	return id
}

func TestEventSources_PatchAdminAttaches(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "srcadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Venue", "venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/venue/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	src := decodeJSON(t, w)
	if src["url"] != "https://127.0.0.1:9/cal.ics" || src["node_id"] != nodeID {
		t.Errorf("created source: %v", src)
	}
}

func TestEventSources_WebcalURLNormalized(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "webcaladmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Webcal Venue", "webcal-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/webcal-venue/event-sources",
		map[string]string{"url": "webcal://127.0.0.1:9/cal.ics"}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if src := decodeJSON(t, w); src["url"] != "https://127.0.0.1:9/cal.ics" {
		t.Errorf("webcal not normalized to https: %v", src["url"])
	}
}

func TestEventSources_MemberMayNot(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "ownadmin", "member")
	member, memberToken := createTestUser(t, db, "justmember", "member")
	nodeID := createTestNode(t, db, admin.ID, "Guarded", "guarded", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/nodes/guarded/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("member attach: expected 403, got %d", w.Code)
	}

	r = authedRequest("GET", "/api/v1/nodes/guarded/event-sources", nil, memberToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-sources", handler.ListEventSources(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("member list: expected 403, got %d", w.Code)
	}
}

// On unclaimed patches only the instance admin manages sources. A
// trusted contributor's grant delegates the review queue, never standing
// feeds (docs/adr/031).
func TestEventSources_UnclaimedIsInstanceAdminOnly(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, adminToken := createTestUser(t, db, "instadmin", "admin")
	trusted, trustedToken := createTestUser(t, db, "trusty", "member")
	if _, err := db.Exec(`UPDATE users SET trusted_contributor = 1 WHERE id = ?`, trusted.ID); err != nil {
		t.Fatalf("grant trusted: %v", err)
	}

	nodeID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status)
		 VALUES (?, ?, 'Unclaimed Venue', 'unclaimed-venue', '', 'leaf', 'public', 'open', 'unclaimed')`,
		nodeID, instanceAdmin.ID,
	); err != nil {
		t.Fatalf("seed unclaimed node: %v", err)
	}

	r := authedRequest("POST", "/api/v1/nodes/unclaimed-venue/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, trustedToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("trusted contributor attach on unclaimed: expected 403, got %d", w.Code)
	}

	r = authedRequest("POST", "/api/v1/nodes/unclaimed-venue/event-sources",
		map[string]string{"url": "https://127.0.0.1:9/cal.ics"}, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources", handler.CreateEventSource(db), r)
	if w.Code != http.StatusCreated {
		t.Errorf("instance admin attach on unclaimed: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestImportedEvents_ReadOnlyUntilDetached(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "roadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "RO Venue", "ro-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	sourceID := seedEventSource(t, db, nodeID, admin.ID, "https://feeds.example/cal.ics")
	eventID := seedImportedEvent(t, db, nodeID, admin.ID, sourceID, "uid-1", "Imported Show",
		time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))

	// Even the patch admin cannot edit an imported event.
	r := authedRequest("PATCH", "/api/v1/events/"+eventID, map[string]string{"title": "Edited"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "event source") {
		t.Errorf("edit imported: expected 403 naming the source, got %d: %s", w.Code, w.Body.String())
	}

	// Detach: skip-listed and editable from then on.
	r = authedRequest("POST", "/api/v1/events/"+eventID+"/detach", nil, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/events/{id}/detach", handler.DetachEvent(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("detach: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var skips int
	db.QueryRow(`SELECT COUNT(*) FROM event_source_skips WHERE source_id = ? AND uid = 'uid-1'`, sourceID).Scan(&skips)
	if skips != 1 {
		t.Error("detach must skip-list the feed item")
	}

	r = authedRequest("PATCH", "/api/v1/events/"+eventID, map[string]string{"title": "Edited"}, adminToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("edit after detach: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteImportedEvent_SkipListsFirst(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "deladmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Del Venue", "del-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	sourceID := seedEventSource(t, db, nodeID, admin.ID, "https://feeds.example/cal.ics")
	eventID := seedImportedEvent(t, db, nodeID, admin.ID, sourceID, "uid-gone", "Doomed Show",
		time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))

	r := authedRequest("DELETE", "/api/v1/events/"+eventID, nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/events/{id}", handler.DeleteEvent(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var skips, remaining int
	db.QueryRow(`SELECT COUNT(*) FROM event_source_skips WHERE source_id = ? AND uid = 'uid-gone'`, sourceID).Scan(&skips)
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, eventID).Scan(&remaining)
	if skips != 1 || remaining != 0 {
		t.Errorf("delete imported: skips=%d remaining=%d", skips, remaining)
	}
}

// Removing a source keeps past events (detached history) and removes
// future imported ones (docs/adr/031).
func TestDeleteEventSource_KeepsPastRemovesFuture(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "rmadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rm Venue", "rm-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	sourceID := seedEventSource(t, db, nodeID, admin.ID, "https://feeds.example/cal.ics")

	pastID := seedImportedEvent(t, db, nodeID, admin.ID, sourceID, "uid-past", "Past Show",
		time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339))
	futureID := seedImportedEvent(t, db, nodeID, admin.ID, sourceID, "uid-future", "Future Show",
		time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))

	r := authedRequest("DELETE", "/api/v1/nodes/rm-venue/event-sources/"+sourceID, nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/event-sources/{id}", handler.DeleteEventSource(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete source: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var futureCount int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, futureID).Scan(&futureCount)
	if futureCount != 0 {
		t.Error("future imported event survived source removal")
	}
	var pastSourceID *string
	if err := db.QueryRow(`SELECT source_id FROM events WHERE id = ?`, pastID).Scan(&pastSourceID); err != nil {
		t.Fatal("past event was deleted with its source")
	}
	if pastSourceID != nil {
		t.Error("past event should be detached, not still pointing at a deleted source")
	}
}

// Manual sync-now runs synchronously against a real (local) feed.
func TestSyncEventSource_Now(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "syncadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Sync Venue", "sync-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	start := time.Now().Add(48 * time.Hour).UTC().Format("20060102T150405Z")
	feed := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//T//EN\r\nBEGIN:VEVENT\r\nUID:s@t\r\nSUMMARY:Synced Show\r\nDTSTART:" + start + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(feed))
	}))
	defer srv.Close()
	prev := safehttp.SetAllowPrivateAddresses(true)
	defer safehttp.SetAllowPrivateAddresses(prev)

	sourceID := seedEventSource(t, db, nodeID, admin.ID, srv.URL)

	r := authedRequest("POST", "/api/v1/nodes/sync-venue/event-sources/"+sourceID+"/sync", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources/{id}/sync", handler.SyncEventSource(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("sync now: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	src := decodeJSON(t, w)
	if src["status"] != "ok" || src["event_count"].(float64) != 1 {
		t.Errorf("synced source: %v", src)
	}

	// Immediately again: rate-limited per source.
	r = authedRequest("POST", "/api/v1/nodes/sync-venue/event-sources/"+sourceID+"/sync", nil, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/event-sources/{id}/sync", handler.SyncEventSource(db), r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second sync now: expected 429, got %d", w.Code)
	}
}
