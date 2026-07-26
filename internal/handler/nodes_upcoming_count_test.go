package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The patch profile states an upcoming-events figure. It used to render the
// length of the capped page of rows it had already fetched, so a venue with
// forty shows advertised five (CONTEXT.md "Upcoming events"). The count is
// the server's, and it counts the same events the list under it shows.

func insertEvent(t *testing.T, db *database.DB, nodeID, ownerID, title string, startsAt time.Time, opts ...func(id string)) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at,
		 recurrence, visibility, status)
		 VALUES (?, ?, ?, ?, '', '', ?, '', 'public', 'active')`,
		id, nodeID, ownerID, title, startsAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert event %s: %v", title, err)
	}
	for _, o := range opts {
		o(id)
	}
	return id
}

func upcomingCount(t *testing.T, db *database.DB, slug string) int {
	t.Helper()
	// Public read: the profile is what a stranger with a flyer's QR code
	// lands on (docs/adr/042), so the count is asserted unauthenticated.
	r, _ := http.NewRequest("GET", "/api/v1/nodes/"+slug, nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("get node %s: %d — %s", slug, w.Code, w.Body.String())
	}
	var resp struct {
		Node struct {
			UpcomingEventCount int `json:"upcoming_event_count"`
		} `json:"node"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode node response: %v", err)
	}
	return resp.Node.UpcomingEventCount
}

func TestUpcomingEventCountExcludesThePast(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "count-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "The Selvage", "the-selvage", "open")

	now := time.Now().UTC()
	insertEvent(t, db, nodeID, owner.ID, "Last spring's show", now.Add(-90*24*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Yesterday", now.Add(-24*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Tonight", now.Add(2*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Next week", now.Add(7*24*time.Hour))

	// Four events on the calendar, two of them still ahead. A patch idle
	// since spring must not read as busy — this is why the tree endpoint's
	// all-time event_count was not reused.
	if got := upcomingCount(t, db, "the-selvage"); got != 2 {
		t.Errorf("upcoming_event_count = %d, want 2", got)
	}
}

func TestUpcomingEventCountIsNotCappedByAnyPageSize(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "busy-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Busy Venue", "busy-venue", "open")

	now := time.Now().UTC()
	for i := 0; i < 40; i++ {
		insertEvent(t, db, nodeID, owner.ID, "Show", now.Add(time.Duration(i+1)*24*time.Hour))
	}

	// The bug in one assertion: the profile fetches three rows, and the
	// count must not be three.
	if got := upcomingCount(t, db, "busy-venue"); got != 40 {
		t.Errorf("upcoming_event_count = %d, want 40 — a capped page of rows is not the count", got)
	}
}

func TestUpcomingEventCountSkipsRemovedAndPrivateEvents(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "mod-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Moderated", "moderated", "open")

	now := time.Now().UTC()
	insertEvent(t, db, nodeID, owner.ID, "Counted", now.Add(48*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Soft-removed", now.Add(49*time.Hour), func(id string) {
		if _, err := db.Exec(`UPDATE events SET removed_at = ? WHERE id = ?`, now.Format(time.RFC3339), id); err != nil {
			t.Fatalf("soft-remove: %v", err)
		}
	})
	insertEvent(t, db, nodeID, owner.ID, "Not public", now.Add(50*time.Hour), func(id string) {
		if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, id); err != nil {
			t.Fatalf("set visibility: %v", err)
		}
	})

	// The same gates ListEvents applies, so the number never promises a row
	// the list below it will not show.
	if got := upcomingCount(t, db, "moderated"); got != 1 {
		t.Errorf("upcoming_event_count = %d, want 1", got)
	}
}

func TestUpcomingEventCountIncludesConfirmedEventLinks(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "link-owner", "member")
	hostID := createTestNode(t, db, owner.ID, "Host", "host-patch", "open")
	guestID := createTestNode(t, db, owner.ID, "Guest", "guest-patch", "open")

	now := time.Now().UTC()
	confirmed := insertEvent(t, db, hostID, owner.ID, "Shared bill", now.Add(72*time.Hour))
	pending := insertEvent(t, db, hostID, owner.ID, "Proposed bill", now.Add(96*time.Hour))

	for id, status := range map[string]string{confirmed: "confirmed", pending: "pending"} {
		if _, err := db.Exec(
			`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by)
			 VALUES (?, ?, ?, ?, 'owner', ?)`,
			auth.NewUUIDv7(), id, guestID, status, owner.ID,
		); err != nil {
			t.Fatalf("insert event_link (%s): %v", status, err)
		}
	}

	// A patch's calendar is its own events plus confirmed links
	// (docs/adr/032); a link awaiting consent is not on the bill yet.
	if got := upcomingCount(t, db, "guest-patch"); got != 1 {
		t.Errorf("guest upcoming_event_count = %d, want 1 (confirmed link only)", got)
	}
	if got := upcomingCount(t, db, "host-patch"); got != 2 {
		t.Errorf("host upcoming_event_count = %d, want 2 (both its own)", got)
	}
}
