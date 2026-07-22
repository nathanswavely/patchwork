package handler_test

import (
	"net/http"
	"testing"

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
