package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-075: "Repeats weekly" was stored, displayed, and never expanded.
//
// One row, one date, and an ICS feed with no RRULE, so a subscriber who
// clicked the calendar link got a single occurrence. The word was the only
// part of a recurring event that existed. The control is withdrawn rather
// than implemented — see validateRecurrence for the trade — so the write
// paths stop taking a word the product cannot keep, and say what to do
// instead. Rows that already carry one keep it; the event page renders it
// with the caveat it always needed.

func TestCreateEventRefusesARecurrenceItCannotKeep(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	owner, token := createTestUser(t, db, "recur-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Choir", "recur-choir", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	for _, word := range []string{"weekly", "daily", "biweekly", "monthly", "FREQ=WEEKLY"} {
		body := map[string]interface{}{
			"node_id": nodeID, "title": "Rehearsal",
			"starts_at": "2027-04-06T19:00:00Z", "recurrence": word,
		}
		r := authedRequest("POST", "/api/v1/events", body, token)
		w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, cfg), r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("POST /events accepted recurrence %q: %d — %s", word, w.Code, w.Body.String())
		}
		// A refusal that does not say what to do instead just moves the
		// problem: she still has four Tuesdays to put on the page.
		if !strings.Contains(w.Body.String(), "each date") ||
			!strings.Contains(w.Body.String(), "event source") {
			t.Errorf("refusal named no alternative: %s", w.Body.String())
		}
	}

	// Without the word, the same event is fine.
	body := map[string]interface{}{
		"node_id": nodeID, "title": "Rehearsal", "starts_at": "2027-04-06T19:00:00Z",
	}
	r := authedRequest("POST", "/api/v1/events", body, token)
	if w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, cfg), r); w.Code != http.StatusCreated {
		t.Fatalf("plain event: got %d — %s", w.Code, w.Body.String())
	}
}

func TestStoredRecurrenceSurvivesAnEditAndCanBeCleared(t *testing.T) {
	db := setupTestDB(t)
	owner, token := createTestUser(t, db, "legacy-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Choir", "legacy-choir", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	id := insertZonedEvent(t, db, nodeID, owner.ID, "Rehearsal", "2027-04-06T19:00:00Z", "", "")
	if _, err := db.Exec(`UPDATE events SET recurrence = 'weekly' WHERE id = ?`, id); err != nil {
		t.Fatalf("seed recurrence: %v", err)
	}

	// An ordinary edit leaves it alone. The word is data a person entered
	// before the product admitted it could not keep it, and deleting it on
	// their behalf during a title change would be its own silent move.
	r := authedRequest("PATCH", "/api/v1/events/"+id, map[string]interface{}{"title": "Rehearsal (moved room)"}, token)
	if w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r); w.Code != http.StatusOK {
		t.Fatalf("edit: got %d — %s", w.Code, w.Body.String())
	}
	var stored string
	db.QueryRow(`SELECT recurrence FROM events WHERE id = ?`, id).Scan(&stored)
	if stored != "weekly" {
		t.Errorf("recurrence = %q after an unrelated edit, want weekly", stored)
	}

	// Sending "" retires it, so the lie is removable.
	r = authedRequest("PATCH", "/api/v1/events/"+id, map[string]interface{}{"recurrence": ""}, token)
	if w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r); w.Code != http.StatusOK {
		t.Fatalf("clear: got %d — %s", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT recurrence FROM events WHERE id = ?`, id).Scan(&stored)
	if stored != "" {
		t.Errorf("recurrence = %q after clearing", stored)
	}

	// Setting a new one is refused on this path too.
	r = authedRequest("PATCH", "/api/v1/events/"+id, map[string]interface{}{"recurrence": "monthly"}, token)
	if w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r); w.Code != http.StatusBadRequest {
		t.Errorf("PATCH accepted a new recurrence: %d", w.Code)
	}
}
