package handler_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestContactItemsInTheMembersRoom is the Members-room half of the predicate,
// and carries forward the viewer matrix docs/adr/080's test established —
// including the case that is the whole point of the gate being narrower than
// `insider`: an instance admin holding no role in the patch.
func TestContactItemsInTheMembersRoom(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "mrowner", "member")
	sharer, sharerToken := createTestUser(t, db, "mrsharer", "member")
	fellow, fellowToken := createTestUser(t, db, "mrfellow", "member")
	follower, followerToken := createTestUser(t, db, "mrfollower", "member")
	elsewhere, elsewhereToken := createTestUser(t, db, "mrelsewhere", "member")
	_, siteAdminToken := createTestUser(t, db, "mrsiteadmin", "admin")

	shared := createTestNode(t, db, owner.ID, "Shared Room", "mr-shared", "open")
	other := createTestNode(t, db, owner.ID, "Other Room", "mr-other", "open")
	createTestMembership(t, db, owner.ID, shared, "admin", "active")
	createTestMembership(t, db, owner.ID, other, "admin", "active")
	createTestMembership(t, db, sharer.ID, shared, "member", "active")
	createTestMembership(t, db, sharer.ID, other, "member", "active")
	createTestMembership(t, db, fellow.ID, shared, "member", "active")
	createTestMembership(t, db, follower.ID, shared, "follower", "active")
	createTestMembership(t, db, elsewhere.ID, other, "member", "active")

	r := authedRequest("POST", "/api/v1/users/me/contact-items",
		map[string]string{"kind": "phone", "value": "+1 717 555 0100", "label": "Signal preferred"}, sharerToken)
	w := serveMux(t, db, "POST", "/api/v1/users/me/contact-items", handler.CreateMyContactItem(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("create item: %d %s", w.Code, w.Body.String())
	}
	itemID, _ := decodeJSON(t, w)["id"].(string)

	// Shared with one patch of the two the sharer belongs to.
	sr := authedRequest("PUT", "/api/v1/nodes/mr-shared/contact-shares",
		map[string][]string{"item_ids": {itemID}}, sharerToken)
	if sw := serveMux(t, db, "PUT", "/api/v1/nodes/{slug}/contact-shares",
		handler.PutMyContactSharesForNode(db), sr); sw.Code != http.StatusOK {
		t.Fatalf("share: %d %s", sw.Code, sw.Body.String())
	}

	contactOf := func(t *testing.T, slug, token string) []interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/"+slug+"/members", nil, token)
		w := serveOptionalMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list members %s: %d %s", slug, w.Code, w.Body.String())
		}
		items, _ := decodeJSON(t, w)["items"].([]interface{})
		for _, it := range items {
			m := it.(map[string]interface{})
			if m["username"] == "mrsharer" {
				got, _ := m["contact"].([]interface{})
				return got
			}
		}
		t.Fatalf("sharer missing from %s listing", slug)
		return nil
	}

	for _, c := range []struct {
		name  string
		slug  string
		token string
		want  bool
	}{
		{"anonymous never", "mr-shared", "", false},
		{"follower is not in the room", "mr-shared", followerToken, false},
		{"instance admin with no role here", "mr-shared", siteAdminToken, false},
		{"fellow member of the shared patch", "mr-shared", fellowToken, true},
		{"the sharer sees their own item", "mr-shared", sharerToken, true},
		{"fellow member of the other patch", "mr-other", elsewhereToken, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := contactOf(t, c.slug, c.token)
			if (len(got) > 0) != c.want {
				t.Fatalf("want contact present=%v, got %v", c.want, got)
			}
			if c.want {
				item := got[0].(map[string]interface{})
				if item["value"] != "+1 717 555 0100" || item["kind"] != "phone" || item["label"] != "Signal preferred" {
					t.Errorf("wrong item: %v", item)
				}
			}
		})
	}
}

// TestContactItemValidation replaces docs/adr/080's whole-card validation.
// The rule that matters is the last one: a value is checked against the kind
// in force *after* the patch, so retyping a note as an email has to meet the
// email rule rather than the one it arrived under.
func TestContactItemValidation(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "civalid", "member")

	for _, c := range []struct {
		name string
		body map[string]string
	}{
		{"unknown kind", map[string]string{"kind": "carrier-pigeon", "value": "x"}},
		{"empty value", map[string]string{"kind": "phone", "value": "   "}},
		{"phone too long", map[string]string{"kind": "phone", "value": strings.Repeat("1", 61)}},
		{"email without @", map[string]string{"kind": "email", "value": "not-an-address"}},
		{"email with a space", map[string]string{"kind": "email", "value": "two words@example.com"}},
		{"note too long", map[string]string{"kind": "note", "value": strings.Repeat("n", 201)}},
		{"handle too long", map[string]string{"kind": "handle", "value": strings.Repeat("h", 101)}},
		{"label too long", map[string]string{"kind": "phone", "value": "+1 717 555 0100", "label": strings.Repeat("l", 41)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := authedRequest("POST", "/api/v1/users/me/contact-items", c.body, token)
			w := serveMux(t, db, "POST", "/api/v1/users/me/contact-items", handler.CreateMyContactItem(db), r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	// A valid note, then retyped as an email: the new kind's rule applies.
	r := authedRequest("POST", "/api/v1/users/me/contact-items",
		map[string]string{"kind": "note", "value": "ask at the bar"}, token)
	w := serveMux(t, db, "POST", "/api/v1/users/me/contact-items", handler.CreateMyContactItem(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("create note: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeJSON(t, w)["id"].(string)

	pr := authedRequest("PATCH", "/api/v1/users/me/contact-items/"+id,
		map[string]string{"kind": "email"}, token)
	pw := serveMux(t, db, "PATCH", "/api/v1/users/me/contact-items/{id}", handler.UpdateMyContactItem(db), pr)
	if pw.Code != http.StatusBadRequest {
		t.Errorf("retyping a note as an email must meet the email rule: got %d %s", pw.Code, pw.Body.String())
	}
}
