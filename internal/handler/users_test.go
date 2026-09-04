package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// serveOptionalMux registers the handler behind AuthOptional, matching how
// public-but-viewer-aware routes are mounted in main.go.
func serveOptionalMux(t *testing.T, db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthOptional(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func createPrivateTestNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status) VALUES (?, ?, ?, ?, '', 'leaf', 'private', 'open', 'active')`,
		id, ownerID, name, slug,
	)
	if err != nil {
		t.Fatalf("create private node %s: %v", name, err)
	}
	return id
}

func TestUserProfileShowsOnlyVisiblePublicMemberships(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "profiled", "member")
	owner, _ := createTestUser(t, db, "powner", "member")

	shown := createTestNode(t, db, owner.ID, "Shown Patch", "shown-patch", "open")
	hidden := createTestNode(t, db, owner.ID, "Hidden Patch", "hidden-patch", "open")
	followed := createTestNode(t, db, owner.ID, "Followed Patch", "followed-patch", "open")
	private := createPrivateTestNode(t, db, owner.ID, "Private Patch", "private-patch")

	createTestMembership(t, db, user.ID, shown, "admin", "active")
	hiddenMem := createTestMembership(t, db, user.ID, hidden, "member", "active")
	createTestMembership(t, db, user.ID, followed, "follower", "active")
	createTestMembership(t, db, user.ID, private, "member", "active")
	db.Exec("UPDATE memberships SET visible = 0 WHERE id = ?", hiddenMem)

	db.Exec("UPDATE users SET bio = 'hi', links = '[{\"url\":\"https://example.com\",\"label\":\"Site\"}]' WHERE id = ?", user.ID)

	r := authedRequest("GET", "/api/v1/users/profiled", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/users/{username}", handler.GetUserProfile(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["username"] != "profiled" || result["bio"] != "hi" {
		t.Errorf("unexpected identity fields: %v", result)
	}
	if _, hasEmail := result["email"]; hasEmail {
		t.Error("profile must never expose email")
	}
	links, _ := result["links"].([]interface{})
	if len(links) != 1 {
		t.Errorf("expected 1 link, got %v", result["links"])
	}
	memberships, _ := result["memberships"].([]interface{})
	if len(memberships) != 1 {
		t.Fatalf("expected exactly 1 visible membership, got %d: %v", len(memberships), result["memberships"])
	}
	m := memberships[0].(map[string]interface{})
	if m["node_slug"] != "shown-patch" || m["role"] != "admin" {
		t.Errorf("expected shown-patch admin, got %v", m)
	}
}

func TestUserProfileNotFound(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "suspendedp", "member")
	db.Exec("UPDATE users SET suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?", user.ID)

	for _, username := range []string{"suspendedp", "nosuchuser"} {
		r := authedRequest("GET", "/api/v1/users/"+username, nil, "")
		w := servePublicMux(t, "GET", "/api/v1/users/{username}", handler.GetUserProfile(db), r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", username, w.Code)
		}
	}
}

func TestPublicMemberListHidesHiddenAndFollowers(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "mladmin", "member")
	member, memberToken := createTestUser(t, db, "mlmember", "member")
	hiddenUser, _ := createTestUser(t, db, "mlhidden", "member")
	followerUser, followerToken := createTestUser(t, db, "mlfollower", "member")
	nodeID := createTestNode(t, db, admin.ID, "Vis Node", "vis-node", "open")

	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	hiddenMem := createTestMembership(t, db, hiddenUser.ID, nodeID, "member", "active")
	createTestMembership(t, db, followerUser.ID, nodeID, "follower", "active")
	db.Exec("UPDATE memberships SET visible = 0 WHERE id = ?", hiddenMem)

	cases := []struct {
		name  string
		token string
		want  int
	}{
		{"anonymous gets public view", "", 2},
		{"follower gets public view", followerToken, 2},
		{"fellow member sees all", memberToken, 4},
		{"node admin sees all", adminToken, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := authedRequest("GET", "/api/v1/nodes/vis-node/members", nil, c.token)
			w := serveOptionalMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			items, _ := decodeJSON(t, w)["items"].([]interface{})
			if len(items) != c.want {
				t.Errorf("expected %d members, got %d", c.want, len(items))
			}
		})
	}
}

func TestUpdateMembershipVisibility(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "vtadmin", "member")
	user, userToken := createTestUser(t, db, "vtuser", "member")
	nodeID := createTestNode(t, db, admin.ID, "Toggle Node", "toggle-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	body := map[string]bool{"visible": false}
	r := authedRequest("PATCH", "/api/v1/users/me/memberships/"+nodeID, body, userToken)
	w := serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var visible int
	db.QueryRow("SELECT visible FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&visible)
	if visible != 0 {
		t.Errorf("expected visible=0 after toggle, got %d", visible)
	}

	// The hidden membership leaves the public profile too.
	pr := authedRequest("GET", "/api/v1/users/vtuser", nil, "")
	pw := servePublicMux(t, "GET", "/api/v1/users/{username}", handler.GetUserProfile(db), pr)
	memberships, _ := decodeJSON(t, pw)["memberships"].([]interface{})
	if len(memberships) != 0 {
		t.Errorf("expected no visible memberships, got %v", memberships)
	}

	// Toggling a membership you don't have is a 404.
	r = authedRequest("PATCH", "/api/v1/users/me/memberships/"+auth.NewUUIDv7(), body, userToken)
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown membership, got %d", w.Code)
	}
}

func TestUpdateMeLinks(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "linker", "member")

	body := map[string]interface{}{
		"links": []map[string]string{{"url": "https://example.com", "label": "Site"}},
	}
	r := authedRequest("PATCH", "/api/v1/auth/me", body, token)
	w := serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	links, _ := decodeJSON(t, w)["links"].([]interface{})
	if len(links) != 1 {
		t.Fatalf("expected 1 link in response, got %v", links)
	}

	r = authedRequest("GET", "/api/v1/auth/me", nil, token)
	w = serveMux(t, db, "GET", "/api/v1/auth/me", handler.Me(db), r)
	links, _ = decodeJSON(t, w)["links"].([]interface{})
	if len(links) != 1 {
		t.Errorf("expected 1 link from GET me, got %v", links)
	}
}

// The contact card (docs/adr/080) is shown to the people in the room — a
// patch's own active admins and members — and to nobody else: not the
// public member list, not followers, not an instance admin who holds no
// role in the patch, not another patch the person is in but didn't share
// with, and never the public profile.
func TestContactCardIsSharedPatchByPatch(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "ccowner", "member")
	sharer, sharerToken := createTestUser(t, db, "ccsharer", "member")
	fellow, fellowToken := createTestUser(t, db, "ccfellow", "member")
	follower, followerToken := createTestUser(t, db, "ccfollower", "member")
	elsewhere, elsewhereToken := createTestUser(t, db, "ccelsewhere", "member")
	_, siteAdminToken := createTestUser(t, db, "ccsiteadmin", "admin")

	shared := createTestNode(t, db, owner.ID, "Shared Room", "shared-room", "open")
	other := createTestNode(t, db, owner.ID, "Other Room", "other-room", "open")
	createTestMembership(t, db, owner.ID, shared, "admin", "active")
	createTestMembership(t, db, owner.ID, other, "admin", "active")
	createTestMembership(t, db, sharer.ID, shared, "member", "active")
	createTestMembership(t, db, sharer.ID, other, "member", "active")
	createTestMembership(t, db, fellow.ID, shared, "member", "active")
	createTestMembership(t, db, follower.ID, shared, "follower", "active")
	createTestMembership(t, db, elsewhere.ID, other, "member", "active")

	// The sharer fills in a card...
	r := authedRequest("PATCH", "/api/v1/auth/me", map[string]interface{}{
		"contact_card": map[string]string{"phone": " +1 717 555 0100 ", "email": "reach@example.com", "note": "Signal preferred"},
	}, sharerToken)
	w := serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("update card: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	card, _ := decodeJSON(t, w)["contact_card"].(map[string]interface{})
	if card["phone"] != "+1 717 555 0100" || card["email"] != "reach@example.com" || card["note"] != "Signal preferred" {
		t.Fatalf("card not stored as trimmed: %v", card)
	}

	// ...and shares it with one patch of the two.
	r = authedRequest("PATCH", "/api/v1/users/me/memberships/"+shared, map[string]bool{"share_contact": true}, sharerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("share: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if res := decodeJSON(t, w); res["share_contact"] != true || res["visible"] != true {
		t.Errorf("switch response should carry both switches: %v", res)
	}

	contactOf := func(t *testing.T, slug, token string) interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/"+slug+"/members", nil, token)
		w := serveOptionalMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list members: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		items, _ := decodeJSON(t, w)["items"].([]interface{})
		for _, it := range items {
			m := it.(map[string]interface{})
			if m["username"] == "ccsharer" {
				return m["contact"]
			}
		}
		t.Fatalf("sharer missing from %s listing", slug)
		return nil
	}

	cases := []struct {
		name  string
		slug  string
		token string
		want  bool
	}{
		{"anonymous never", "shared-room", "", false},
		{"follower is not in the room", "shared-room", followerToken, false},
		{"instance admin with no role here", "shared-room", siteAdminToken, false},
		{"fellow member of the shared patch", "shared-room", fellowToken, true},
		{"the sharer sees their own card", "shared-room", sharerToken, true},
		{"fellow member of the other patch", "other-room", elsewhereToken, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := contactOf(t, c.slug, c.token)
			if (got != nil) != c.want {
				t.Errorf("want contact present=%v, got %v", c.want, got)
			}
			if c.want {
				card := got.(map[string]interface{})
				if card["phone"] != "+1 717 555 0100" || card["note"] != "Signal preferred" {
					t.Errorf("wrong card: %v", card)
				}
			}
		})
	}

	// The public profile never carries it, whoever asks.
	pr := authedRequest("GET", "/api/v1/users/ccsharer", nil, "")
	pw := servePublicMux(t, "GET", "/api/v1/users/{username}", handler.GetUserProfile(db), pr)
	if body := pw.Body.String(); strings.Contains(body, "555 0100") || strings.Contains(body, "contact") {
		t.Errorf("profile leaks the contact card: %s", body)
	}

	// A follower cannot share: there is no room for a follower to be in.
	r = authedRequest("PATCH", "/api/v1/users/me/memberships/"+shared, map[string]bool{"share_contact": true}, followerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("follower sharing: expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// Switching off takes effect at once; the empty body is refused.
	r = authedRequest("PATCH", "/api/v1/users/me/memberships/"+shared, map[string]bool{"share_contact": false}, sharerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("unshare: expected 200, got %d", w.Code)
	}
	if got := contactOf(t, "shared-room", fellowToken); got != nil {
		t.Errorf("card still shown after unsharing: %v", got)
	}
	r = authedRequest("PATCH", "/api/v1/users/me/memberships/"+shared, map[string]string{}, sharerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembership(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty switch body: expected 400, got %d", w.Code)
	}
}

func TestContactCardValidation(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "ccvalid", "member")

	bad := []map[string]string{
		{"phone": strings.Repeat("1", 61)},
		{"email": "not-an-address"},
		{"email": "two words@example.com"},
		{"note": strings.Repeat("n", 201)},
	}
	for _, card := range bad {
		r := authedRequest("PATCH", "/api/v1/auth/me", map[string]interface{}{"contact_card": card}, token)
		w := serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("card %v: expected 400, got %d: %s", card, w.Code, w.Body.String())
		}
	}

	// A card is replaced whole: sending only a phone clears the rest.
	for _, card := range []map[string]string{
		{"phone": "555", "email": "a@b.example", "note": "hi"},
		{"phone": "555"},
	} {
		r := authedRequest("PATCH", "/api/v1/auth/me", map[string]interface{}{"contact_card": card}, token)
		w := serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("card %v: expected 200, got %d: %s", card, w.Code, w.Body.String())
		}
	}
	r := authedRequest("GET", "/api/v1/auth/me", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/auth/me", handler.Me(db), r)
	got, _ := decodeJSON(t, w)["contact_card"].(map[string]interface{})
	if got["phone"] != "555" || got["email"] != "" || got["note"] != "" {
		t.Errorf("card should be replaced whole, got %v", got)
	}
}
