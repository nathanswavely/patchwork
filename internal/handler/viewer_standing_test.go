package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// The quilt's cards draw their relationship chip from viewer_standing on the
// list payload. They used to read it from the first page of me/nodes, so a
// person in more than twenty patches saw "Follow" on the ones they followed
// last (internal/handler/viewer_standing.go).

type standingFixture struct {
	db    *database.DB
	token string
	// slug → the standing the viewer should be told, "" for none.
	want map[string]string
}

func setupStanding(t *testing.T) standingFixture {
	t.Helper()
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "standing-owner", "member")
	viewer, token := createTestUser(t, db, "standing-viewer", "member")

	want := map[string]string{}
	place := func(slug, role, status, standing string) {
		id := createTestNode(t, db, owner.ID, slug, slug, "open")
		if role != "" {
			createTestMembership(t, db, viewer.ID, id, role, status)
		}
		want[slug] = standing
	}
	place("runs-it", "admin", "active", "admin")
	place("in-it", "member", "active", "member")
	place("follows-it", "follower", "active", "follower")
	// A pending row carries role='member', which is what it would become.
	place("asked-in", "member", "pending", "pending")
	place("was-asked", "member", "invited", "invited")
	place("left-it", "member", "left", "")
	place("banned-from", "follower", "banned", "")
	place("strangers", "", "", "")

	// Hidden from everyone else, never from its holder (docs/adr/006).
	hiddenID := createTestNode(t, db, owner.ID, "hidden-mine", "hidden-mine", "open")
	mid := createTestMembership(t, db, viewer.ID, hiddenID, "follower", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE id = ?`, mid); err != nil {
		t.Fatalf("hide membership: %v", err)
	}
	want["hidden-mine"] = "follower"

	return standingFixture{db: db, token: token, want: want}
}

func standingMux(db *database.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes/tree", middleware.AuthOptional(db, handler.NodeTree(db)))
	mux.HandleFunc("GET /api/v1/nodes", middleware.AuthOptional(db, handler.ListNodes(db)))
	return mux
}

// treeStandings and listStandings read slug → viewer_standing, nil where
// the key is absent, so "omitted" can be told from "empty".
func treeStandings(t *testing.T, db *database.DB, path, token string) map[string]*string {
	t.Helper()
	w := httptest.NewRecorder()
	standingMux(db).ServeHTTP(w, authedRequest("GET", path, nil, token))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	var resp struct {
		Tree struct {
			Children []map[string]interface{} `json:"children"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return standingsOf(resp.Tree.Children)
}

func listStandings(t *testing.T, db *database.DB, path, token string) map[string]*string {
	t.Helper()
	w := httptest.NewRecorder()
	standingMux(db).ServeHTTP(w, authedRequest("GET", path, nil, token))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return standingsOf(resp.Items)
}

func standingsOf(rows []map[string]interface{}) map[string]*string {
	out := map[string]*string{}
	for _, row := range rows {
		slug, _ := row["slug"].(string)
		if v, ok := row["viewer_standing"]; ok {
			s, _ := v.(string)
			out[slug] = &s
		} else {
			out[slug] = nil
		}
	}
	return out
}

func checkStandings(t *testing.T, label string, got map[string]*string, want map[string]string) {
	t.Helper()
	for slug, w := range want {
		g, listed := got[slug]
		if !listed {
			t.Errorf("%s: %s is not in the payload", label, slug)
			continue
		}
		switch {
		case w == "" && g != nil:
			t.Errorf("%s: %s should carry no viewer_standing, got %q", label, slug, *g)
		case w != "" && g == nil:
			t.Errorf("%s: %s should be %q, got no viewer_standing", label, slug, w)
		case w != "" && *g != w:
			t.Errorf("%s: %s should be %q, got %q", label, slug, w, *g)
		}
	}
}

func TestTreeStatesTheViewersStanding(t *testing.T) {
	f := setupStanding(t)
	checkStandings(t, "tree", treeStandings(t, f.db, "/api/v1/nodes/tree", f.token), f.want)
}

func TestNodeListStatesTheViewersStanding(t *testing.T) {
	f := setupStanding(t)
	checkStandings(t, "nodes", listStandings(t, f.db, "/api/v1/nodes?limit=100", f.token), f.want)
}

// scope=my joins memberships a second time with its own placeholder; the
// standing join's argument has to bind ahead of it.
func TestMyScopeStatesTheViewersStanding(t *testing.T) {
	f := setupStanding(t)
	mine := map[string]string{
		"runs-it": "admin", "in-it": "member", "follows-it": "follower", "hidden-mine": "follower",
	}
	for label, got := range map[string]map[string]*string{
		"tree scope=my":  treeStandings(t, f.db, "/api/v1/nodes/tree?scope=my", f.token),
		"nodes scope=my": listStandings(t, f.db, "/api/v1/nodes?scope=my&limit=100", f.token),
	} {
		checkStandings(t, label, got, mine)
		if len(got) != len(mine) {
			t.Errorf("%s: expected only the viewer's %d active patches, got %d", label, len(mine), len(got))
		}
	}
}

func TestAnonymousCallerHasNoStanding(t *testing.T) {
	f := setupStanding(t)
	none := map[string]string{}
	for slug := range f.want {
		none[slug] = ""
	}
	checkStandings(t, "anonymous tree", treeStandings(t, f.db, "/api/v1/nodes/tree", ""), none)
	checkStandings(t, "anonymous nodes", listStandings(t, f.db, "/api/v1/nodes?limit=100", ""), none)
}

// The event form asks me/nodes for the patches it can post to by role,
// rather than filtering every relationship on the client. A pending row
// carries role='member' and must not answer a request for members.
func TestMyMembershipsFiltersByRole(t *testing.T) {
	f := setupStanding(t)
	get := func(path string) (int, map[string]string) {
		w := serveMux(t, f.db, "GET", "/api/v1/me/nodes", handler.ListMyMemberships(f.db), authedRequest("GET", path, nil, f.token))
		var out struct {
			Items []struct {
				Slug string `json:"node_slug"`
				Role string `json:"role"`
			} `json:"items"`
		}
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		got := map[string]string{}
		for _, it := range out.Items {
			got[it.Slug] = it.Role
		}
		return w.Code, got
	}

	code, got := get("/api/v1/me/nodes?role=admin,member")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	want := map[string]string{"runs-it": "admin", "in-it": "member"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for slug, role := range want {
		if got[slug] != role {
			t.Errorf("%s: expected %q, got %q", slug, role, got[slug])
		}
	}

	if _, got := get("/api/v1/me/nodes?role=follower"); len(got) != 2 || got["follows-it"] != "follower" || got["hidden-mine"] != "follower" {
		t.Errorf("role=follower: expected the two active follows, got %v", got)
	}

	if code, _ := get("/api/v1/me/nodes?role=pending"); code != http.StatusBadRequest {
		t.Errorf("an unknown role should be a 400, got %d", code)
	}
}
