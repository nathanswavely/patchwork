package handler_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// serveContactProfile registers the profile handler behind AuthOptional, the
// way main.go does — the profile's contact section is the one part of it that
// depends on the caller (docs/adr/083).
func serveContactProfile(t *testing.T, db *database.DB, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/{username}", middleware.AuthOptional(db, handler.GetUserProfile(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// profileContactValues returns the values a caller can read on a profile.
func profileContactValues(t *testing.T, db *database.DB, username, token string) []string {
	t.Helper()
	r := authedRequest("GET", "/api/v1/users/"+username, nil, token)
	w := serveContactProfile(t, db, r)
	if w.Code != http.StatusOK {
		t.Fatalf("profile %s: expected 200, got %d: %s", username, w.Code, w.Body.String())
	}
	raw, _ := decodeJSON(t, w)["contact"].([]interface{})
	values := []string{}
	for _, it := range raw {
		if m, ok := it.(map[string]interface{}); ok {
			if v, ok := m["value"].(string); ok {
				values = append(values, v)
			}
		}
	}
	return values
}

func TestContactItemsSharedOnePatchAtATime(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "ciowner", "member")
	sharer, sharerToken := createTestUser(t, db, "cisharer", "member")
	fellow, fellowToken := createTestUser(t, db, "cifellow", "member")
	elsewhere, elsewhereToken := createTestUser(t, db, "cielsewhere", "member")
	follower, followerToken := createTestUser(t, db, "cifollower", "member")
	_, strangerToken := createTestUser(t, db, "cistranger", "member")

	roomA := createTestNode(t, db, owner.ID, "Room A", "room-a", "open")
	roomB := createTestNode(t, db, owner.ID, "Room B", "room-b", "open")
	createTestMembership(t, db, owner.ID, roomA, "admin", "active")
	createTestMembership(t, db, owner.ID, roomB, "admin", "active")
	createTestMembership(t, db, sharer.ID, roomA, "member", "active")
	createTestMembership(t, db, fellow.ID, roomA, "member", "active")
	createTestMembership(t, db, follower.ID, roomA, "follower", "active")
	createTestMembership(t, db, elsewhere.ID, roomB, "member", "active")

	// Two items on the account. Neither is shared by being created.
	newItem := func(kind, value string) string {
		t.Helper()
		r := authedRequest("POST", "/api/v1/users/me/contact-items",
			map[string]string{"kind": kind, "value": value}, sharerToken)
		w := serveMux(t, db, "POST", "/api/v1/users/me/contact-items", handler.CreateMyContactItem(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("create %s: expected 200, got %d: %s", kind, w.Code, w.Body.String())
		}
		id, _ := decodeJSON(t, w)["id"].(string)
		if id == "" {
			t.Fatalf("create %s returned no id", kind)
		}
		return id
	}
	phoneID := newItem("phone", " +1 717 555 0199 ")
	newItem("email", "reach@example.com")

	if got := profileContactValues(t, db, "cisharer", fellowToken); len(got) != 0 {
		t.Errorf("a new item is shared with nobody, got %v", got)
	}

	putShares := func(slug, token string, ids []string) *httptest.ResponseRecorder {
		t.Helper()
		r := authedRequest("PUT", "/api/v1/nodes/"+slug+"/contact-shares",
			map[string][]string{"item_ids": ids}, token)
		return serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/contact-shares",
			handler.PutMyContactSharesForNode(db), r)
	}

	// The phone goes to Room A. The email goes nowhere.
	if w := putShares("room-a", sharerToken, []string{phoneID}); w.Code != http.StatusOK {
		t.Fatalf("share phone: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Per-item, per-patch: the room sees the phone and not the email.
	got := profileContactValues(t, db, "cisharer", fellowToken)
	if len(got) != 1 || got[0] != "+1 717 555 0199" {
		t.Errorf("fellow member should see the phone alone, got %v", got)
	}

	// Everyone outside that room sees nothing — including a follower of it,
	// who is not in the room, and a member of a patch the sharer is also in
	// but shared nothing with.
	for _, c := range []struct{ who, token string }{
		{"a member of another patch", elsewhereToken},
		{"a follower of the room", followerToken},
		{"a stranger", strangerToken},
		{"an anonymous visitor", ""},
	} {
		if got := profileContactValues(t, db, "cisharer", c.token); len(got) != 0 {
			t.Errorf("%s should see nothing, got %v", c.who, got)
		}
	}

	// The owner's own listing counts the rooms, which nobody else is told.
	r := authedRequest("GET", "/api/v1/users/me/contact-items", nil, sharerToken)
	w := serveMux(t, db, "GET", "/api/v1/users/me/contact-items", handler.ListMyContactItems(db), r)
	items, _ := decodeJSON(t, w)["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first, _ := items[0].(map[string]interface{})
	if first["shared_with"] != float64(1) {
		t.Errorf("phone should report one patch, got %v", first["shared_with"])
	}

	// A follower has no room to share into.
	if w := putShares("room-a", followerToken, []string{}); w.Code != http.StatusForbidden {
		t.Errorf("follower sharing: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	// Nor can anyone share an item that is not theirs.
	if w := putShares("room-a", fellowToken, []string{phoneID}); w.Code != http.StatusForbidden {
		t.Errorf("sharing someone else's item: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	// Unshare-everywhere is the one item-first write, and it only subtracts.
	ur := authedRequest("DELETE", "/api/v1/users/me/contact-items/"+phoneID+"/shares", nil, sharerToken)
	uw := serveMux(t, db, "DELETE", "/api/v1/users/me/contact-items/{id}/shares",
		handler.UnshareMyContactItemEverywhere(db), ur)
	if uw.Code != http.StatusOK {
		t.Fatalf("unshare all: expected 200, got %d: %s", uw.Code, uw.Body.String())
	}
	if got := profileContactValues(t, db, "cisharer", fellowToken); len(got) != 0 {
		t.Errorf("unshared item still readable: %v", got)
	}

	// Leaving drops the shares outright, so rejoining discloses nothing.
	if w := putShares("room-a", sharerToken, []string{phoneID}); w.Code != http.StatusOK {
		t.Fatalf("re-share: expected 200, got %d", w.Code)
	}
	lr := authedRequest("POST", "/api/v1/nodes/room-a/leave", nil, sharerToken)
	lw := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), lr)
	if lw.Code != http.StatusOK {
		t.Fatalf("leave: expected 200, got %d: %s", lw.Code, lw.Body.String())
	}
	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM contact_item_shares WHERE item_id = ?`, phoneID).Scan(&remaining)
	if remaining != 0 {
		t.Errorf("leaving left %d share rows behind", remaining)
	}
}

// TestDeleteAccountErasesTheContactCard guards the intersection of two
// decisions that never met: docs/adr/086 keeps the users row as a tombstone,
// so `contact_items.user_id ON DELETE CASCADE` never fires, and the card
// would outlive the person it belongs to.
//
// Deleting the memberships already ends every disclosure — both surfaces
// require an active member/admin row — but a phone number left in the table
// is not erased, and a card is the person rather than an act.
func TestDeleteAccountErasesTheContactCard(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "ci-boss", "admin")
	user, token := createTestUser(t, db, "ci-leaver", "member")
	nodeID := nodeWithSpareAdmin(t, db, user.ID, "ci-room")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/users/me/contact-items",
		map[string]string{"kind": "phone", "value": "+1 717 555 0166"}, token)
	w := serveMux(t, db, "POST", "/api/v1/users/me/contact-items", handler.CreateMyContactItem(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("create item: %d %s", w.Code, w.Body.String())
	}
	itemID, _ := decodeJSON(t, w)["id"].(string)

	sr := authedRequest("PUT", "/api/v1/nodes/ci-room/contact-shares",
		map[string][]string{"item_ids": {itemID}}, token)
	if sw := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/contact-shares",
		handler.PutMyContactSharesForNode(db), sr); sw.Code != http.StatusOK {
		t.Fatalf("share item: %d %s", sw.Code, sw.Body.String())
	}

	if dw := deleteMe(t, db, token, "ci-leaver"); dw.Code != http.StatusOK {
		t.Fatalf("delete returned %d, want 200: %s", dw.Code, dw.Body.String())
	}

	for _, q := range []string{
		`SELECT COUNT(*) FROM contact_items WHERE user_id = ?`,
		`SELECT COUNT(*) FROM contact_item_shares WHERE item_id IN (SELECT id FROM contact_items WHERE user_id = ?)`,
	} {
		var n int
		db.QueryRow(q, user.ID).Scan(&n)
		if n != 0 {
			t.Errorf("%s returned %d, want 0 — the card outlived the person", q, n)
		}
	}
	// The tombstone itself must still be there; this test must not pass by
	// having deleted the row the whole design keeps.
	var deletedAt sql.NullString
	if err := db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("tombstone missing: %v", err)
	}
	if !deletedAt.Valid {
		t.Error("deleted_at not set")
	}
}
