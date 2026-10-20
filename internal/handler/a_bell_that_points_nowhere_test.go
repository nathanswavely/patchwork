package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// A founder archived a duplicate of her press and went on being told about
// its election: two bell lines, each opening onto "Patch not found", counted
// in the badge (F-033). A notification pointing into an archived patch is now
// hidden from the list and the count, and left alone by mark-all-read, so a
// restore (docs/adr/034) brings it back exactly as it was.
func TestNotifications_AnArchivedPatchLeavesTheBell(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "bell-reader", "member")
	press := createTestNode(t, db, user.ID, "Press", "press", "open")
	createTestNode(t, db, user.ID, "Press Two", "press-two", "open")

	handler.CreateNotification(db, user.ID, "proposal.new", "Nominations are open in Press", "", "/patches/press/governance/abc")
	handler.CreateNotification(db, user.ID, "membership.joined", "New member joined Press", "", "/patches/press")
	handler.CreateNotification(db, user.ID, "membership.request", "Request for Press", "", "/patches/press/members?status=pending")
	// A neighbour whose slug starts with the archived one must not be caught.
	handler.CreateNotification(db, user.ID, "membership.joined", "New member joined Press Two", "", "/patches/press-two/members")
	handler.CreateNotification(db, user.ID, "remote.event", "Something elsewhere", "", "/events/xyz")

	list := func() []string {
		r := authedRequest("GET", "/api/v1/notifications", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/notifications", middleware.AuthRequired(db, handler.ListNotifications(db)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		items, _ := decodeJSON(t, w)["items"].([]interface{})
		var titles []string
		for _, it := range items {
			m, _ := it.(map[string]interface{})
			title, _ := m["title"].(string)
			titles = append(titles, title)
		}
		return titles
	}
	count := func() float64 {
		r := authedRequest("GET", "/api/v1/notifications/count", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/notifications/count", middleware.AuthRequired(db, handler.NotificationCount(db)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		n, _ := decodeJSON(t, w)["unread"].(float64)
		return n
	}

	if got := len(list()); got != 5 {
		t.Fatalf("before archiving every line shows, got %d", got)
	}

	if _, err := db.Exec(`UPDATE nodes SET archived_from = status, status = 'archived' WHERE id = ?`, press); err != nil {
		t.Fatalf("archive: %v", err)
	}

	titles := list()
	if len(titles) != 2 {
		t.Fatalf("an archived patch's three lines should leave the bell and the other two stay, got %v", titles)
	}
	for _, title := range titles {
		if title != "New member joined Press Two" && title != "Something elsewhere" {
			t.Errorf("unexpected line left in the bell: %q", title)
		}
	}
	if got := count(); got != 2 {
		t.Errorf("the badge must count what the bell shows, got %v", got)
	}

	// Mark all read reads what the person can see, and nothing hidden.
	r := authedRequest("POST", "/api/v1/notifications/read-all", nil, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/notifications/read-all", middleware.AuthRequired(db, handler.MarkAllNotificationsRead(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("read-all: %d %s", w.Code, w.Body.String())
	}

	// Restore is silent (docs/adr/034): the lines come back as they were,
	// unread, because nobody ever saw them.
	if _, err := db.Exec(`UPDATE nodes SET status = archived_from, archived_from = NULL WHERE id = ?`, press); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := len(list()); got != 5 {
		t.Fatalf("a restore brings every line back, got %d", got)
	}
	if got := count(); got != 3 {
		t.Errorf("the three hidden lines come back unread, got %v unread", got)
	}
}
