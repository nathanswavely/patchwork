package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-076: "4 Upcoming Events" over a list of three.
//
// The count was right and the glimpse was capped — that half is a frontend
// fix. What is asserted here is the half a page cannot fix for itself: the
// number and the listing have to be answers to the same question, so that
// "N more" can be arithmetic rather than a guess. The one filter the count
// did not apply was the hosting patch's own gate, so a confirmed link from
// a patch that has since been archived was counted by the profile and
// listed by nothing (docs/adr/034).

// listUpcoming walks GET /api/v1/events for one patch to the end of its
// cursors and returns every title. Paged, because a count compared against
// one page is the bug in the other direction.
func listUpcoming(t *testing.T, db *database.DB, slug string) []string {
	t.Helper()
	var titles []string
	cursor := ""
	for {
		path := "/api/v1/events?node_slug=" + slug + "&limit=2"
		if cursor != "" {
			path += "&after=" + cursor
		}
		r, _ := http.NewRequest("GET", path, nil)
		w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list events: %d — %s", w.Code, w.Body.String())
		}
		var resp struct {
			Items []struct {
				Title string `json:"title"`
			} `json:"items"`
			NextCursor string `json:"next_cursor"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode events: %v", err)
		}
		for _, it := range resp.Items {
			titles = append(titles, it.Title)
		}
		if resp.NextCursor == "" {
			return titles
		}
		cursor = resp.NextCursor
	}
}

func TestUpcomingCountEqualsTheListTheSameRequestReturns(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "agree-owner", "member")
	home := createTestNode(t, db, owner.ID, "Choir", "agree-choir", "open")
	now := time.Now().UTC()

	// Four rehearsals ahead, one behind. The page the flyer's QR code
	// points at is the one that has to add up.
	for i := 1; i <= 4; i++ {
		insertEvent(t, db, home, owner.ID, "Rehearsal", now.Add(time.Duration(i*24)*time.Hour))
	}
	insertEvent(t, db, home, owner.ID, "Last week", now.Add(-72*time.Hour))

	// A confirmed link from a healthy patch is on this calendar.
	live := createTestNode(t, db, owner.ID, "Neighbour", "agree-neighbour", "open")
	shared := insertEvent(t, db, live, owner.ID, "Shared bill", now.Add(120*time.Hour))
	linkEvent(t, db, shared, home, "confirmed", owner.ID)

	// A confirmed link from a patch that has since been archived is on
	// nobody's calendar (docs/adr/034) — and used to be counted anyway.
	gone := createTestNode(t, db, owner.ID, "Folded", "agree-folded", "open")
	if _, err := db.Exec(`UPDATE nodes SET status = 'archived' WHERE id = ?`, gone); err != nil {
		t.Fatalf("archive: %v", err)
	}
	ghost := insertEvent(t, db, gone, owner.ID, "Ghost", now.Add(144*time.Hour))
	linkEvent(t, db, ghost, home, "confirmed", owner.ID)

	// A soft-removed event and a non-public one are on neither.
	removed := insertEvent(t, db, home, owner.ID, "Pulled", now.Add(168*time.Hour))
	if _, err := db.Exec(`UPDATE events SET removed_at = ? WHERE id = ?`,
		now.Format(time.RFC3339), removed); err != nil {
		t.Fatalf("remove: %v", err)
	}
	inside := insertEvent(t, db, home, owner.ID, "Private", now.Add(192*time.Hour))
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, inside); err != nil {
		t.Fatalf("visibility: %v", err)
	}

	listed := listUpcoming(t, db, "agree-choir")
	count := upcomingCount(t, db, "agree-choir")
	if count != len(listed) {
		t.Fatalf("upcoming_event_count = %d but the list returns %d (%v)", count, len(listed), listed)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5 (four rehearsals plus one live shared bill)", count)
	}
	for _, title := range listed {
		if title == "Ghost" {
			t.Errorf("an archived patch's event is on this calendar: %v", listed)
		}
	}
}
