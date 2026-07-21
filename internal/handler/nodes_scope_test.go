package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// serveOptionalAuthMux registers a handler behind AuthOptional, matching how
// GET /api/v1/nodes is mounted in main.go.
func serveOptionalAuthMux(t *testing.T, db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthOptional(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

type listNodesResp struct {
	Items []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Address string `json:"address"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func decodeListNodes(t *testing.T, w *httptest.ResponseRecorder) listNodesResp {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp listNodesResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func listNodeIDs(resp listNodesResp) map[string]bool {
	ids := make(map[string]bool, len(resp.Items))
	for _, it := range resp.Items {
		ids[it.ID] = true
	}
	return ids
}

// TestListNodesScopeMy covers the core of issue #45: ?scope=my must narrow the
// listing to the caller's own patches, counting followers as belonging.
func TestListNodesScopeMy(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "scope-user", "member")
	other, _ := createTestUser(t, db, "scope-other", "member")

	mine := createTestNode(t, db, other.ID, "Mine", "mine", "open")
	joined := createTestNode(t, db, other.ID, "Joined", "joined", "open")
	followed := createTestNode(t, db, other.ID, "Followed", "followed", "open")
	stranger := createTestNode(t, db, other.ID, "Stranger", "stranger", "open")

	createTestMembership(t, db, user.ID, mine, "admin", "active")
	createTestMembership(t, db, user.ID, joined, "member", "active")
	createTestMembership(t, db, user.ID, followed, "follower", "active")
	// Someone else's membership must not pull a patch into my quilt.
	createTestMembership(t, db, other.ID, stranger, "admin", "active")

	r := authedRequest("GET", "/api/v1/nodes?scope=my&limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	ids := listNodeIDs(decodeListNodes(t, w))

	if len(ids) != 3 {
		t.Fatalf("expected 3 patches in my quilt, got %d", len(ids))
	}
	for name, id := range map[string]string{"admin": mine, "member": joined, "follower": followed} {
		if !ids[id] {
			t.Errorf("expected %s patch in my quilt, missing", name)
		}
	}
	if ids[stranger] {
		t.Error("stranger's patch must not appear in my quilt")
	}
}

// A follow is a real relationship: My Quilt must not quietly become
// "memberships only" and drop followed patches off the map.
func TestListNodesScopeMyIncludesFollowOnly(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "follow-only", "member")
	owner, _ := createTestUser(t, db, "follow-owner", "member")

	followed := createTestNode(t, db, owner.ID, "Followed Only", "followed-only", "open")
	createTestMembership(t, db, user.ID, followed, "follower", "active")

	r := authedRequest("GET", "/api/v1/nodes?scope=my&limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	resp := decodeListNodes(t, w)

	if len(resp.Items) != 1 || resp.Items[0].ID != followed {
		t.Fatalf("expected the followed patch, got %+v", resp.Items)
	}
}

// Inactive memberships (pending, removed) are not belonging.
func TestListNodesScopeMyIgnoresInactiveMembership(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "scope-inactive", "member")
	owner, _ := createTestUser(t, db, "scope-inactive-owner", "member")

	pending := createTestNode(t, db, owner.ID, "Pending", "pending", "approval_required")
	createTestMembership(t, db, user.ID, pending, "member", "pending")

	r := authedRequest("GET", "/api/v1/nodes?scope=my&limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	resp := decodeListNodes(t, w)

	if len(resp.Items) != 0 {
		t.Fatalf("expected no patches for a pending membership, got %d", len(resp.Items))
	}
}

// My Quilt shows private patches the caller belongs to — the map has to agree
// with the quilt, and every row is one their own membership reaches.
func TestListNodesScopeMyIncludesOwnPrivatePatches(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "scope-private", "member")
	owner, _ := createTestUser(t, db, "scope-private-owner", "member")

	priv := createTestNode(t, db, owner.ID, "Private", "private-patch", "invite_only")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, priv); err != nil {
		t.Fatalf("set private: %v", err)
	}
	createTestMembership(t, db, user.ID, priv, "member", "active")

	// Someone else's private patch stays invisible.
	otherPriv := createTestNode(t, db, owner.ID, "Other Private", "other-private", "invite_only")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, otherPriv); err != nil {
		t.Fatalf("set private: %v", err)
	}

	r := authedRequest("GET", "/api/v1/nodes?scope=my&limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	ids := listNodeIDs(decodeListNodes(t, w))

	if !ids[priv] {
		t.Error("expected own private patch in my quilt")
	}
	if ids[otherPriv] {
		t.Error("another patch's privacy must not leak through scope=my")
	}
}

// The unscoped listing stays public-only regardless of who is asking.
func TestListNodesDefaultScopeStaysPublicOnly(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "scope-default", "member")
	owner, _ := createTestUser(t, db, "scope-default-owner", "member")

	pub := createTestNode(t, db, owner.ID, "Public", "public-patch", "open")
	priv := createTestNode(t, db, owner.ID, "Private", "private-default", "invite_only")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, priv); err != nil {
		t.Fatalf("set private: %v", err)
	}
	createTestMembership(t, db, user.ID, priv, "member", "active")

	r := authedRequest("GET", "/api/v1/nodes?limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	ids := listNodeIDs(decodeListNodes(t, w))

	if !ids[pub] {
		t.Error("expected the public patch in the default listing")
	}
	if ids[priv] {
		t.Error("default listing must stay public-only even for a member")
	}
}

// scope=my without a session returns nothing — never the whole instance.
func TestListNodesScopeMyAnonymousReturnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "anon-owner", "member")
	createTestNode(t, db, owner.ID, "Public", "anon-public", "open")

	r := httptest.NewRequest("GET", "/api/v1/nodes?scope=my&limit=100", nil)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	resp := decodeListNodes(t, w)

	if len(resp.Items) != 0 {
		t.Fatalf("anonymous scope=my must return nothing, got %d patches", len(resp.Items))
	}
}

// scope=my composes with the other filters rather than replacing them.
func TestListNodesScopeMyWithTagFilter(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "scope-tag", "member")
	owner, _ := createTestUser(t, db, "scope-tag-owner", "member")

	tagged := createTestNode(t, db, owner.ID, "Tagged", "tagged-patch", "open")
	untagged := createTestNode(t, db, owner.ID, "Untagged", "untagged-patch", "open")
	createTestMembership(t, db, user.ID, tagged, "member", "active")
	createTestMembership(t, db, user.ID, untagged, "member", "active")

	tagID := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO tags (id, name) VALUES (?, 'music')`, tagID); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO node_tags (node_id, tag_id) VALUES (?, ?)`, tagged, tagID); err != nil {
		t.Fatalf("tag node: %v", err)
	}
	_ = untagged

	r := authedRequest("GET", "/api/v1/nodes?scope=my&tag=music&limit=100", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes", handler.ListNodes(db), r)
	resp := decodeListNodes(t, w)

	if len(resp.Items) != 1 || resp.Items[0].ID != tagged {
		t.Fatalf("expected only the tagged patch from my quilt, got %+v", resp.Items)
	}
}
