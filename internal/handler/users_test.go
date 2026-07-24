package handler_test

import (
	"net/http"
	"net/http/httptest"
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
	w := serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembershipVisibility(db), r)
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
	w = serveMux(t, db, "PATCH", "/api/v1/users/me/memberships/{nodeId}", handler.UpdateMyMembershipVisibility(db), r)
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
