package handler_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// setupTestDB creates a temporary SQLite DB with all migrations applied.
func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "patchwork-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// createTestUser inserts a user and creates a session, returning the user and session token.
func createTestUser(t *testing.T, db *database.DB, username, role string) (*model.User, string) {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role) VALUES (?, ?, ?, ?)`,
		id, username, username, role,
	)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	token, err := auth.CreateSession(db, id, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session for %s: %v", username, err)
	}
	return &model.User{ID: id, Username: username, DisplayName: username, Role: role}, token
}

func createTestNode(t *testing.T, db *database.DB, ownerID, name, slug, policy string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status) VALUES (?, ?, ?, ?, '', 'leaf', 'public', ?, 'active')`,
		id, ownerID, name, slug, policy,
	)
	if err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	return id
}

func createTestMembership(t *testing.T, db *database.DB, userID, nodeID, role, status string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, ?, ?)`,
		id, userID, nodeID, role, status,
	)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	return id
}

// authedRequest creates an HTTP request with session cookie and CSRF header set.
func authedRequest(method, path string, body interface{}, token string) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, bodyReader)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Patchwork-Request", "true")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	return r
}

// serveMux registers the handler with auth middleware and serves the request.
func serveMux(t *testing.T, db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthRequired(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// servePublicMux registers the handler without auth middleware.
func servePublicMux(t *testing.T, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, h)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return result
}

func TestJoinOpenNode(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin1", "member")
	_, userToken := createTestUser(t, db, "joiner1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Open Node", "open-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/open-node/join", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "active" {
		t.Errorf("expected status=active, got %v", result["status"])
	}
}

func TestJoinApprovalRequiredNode(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin2", "member")
	_, userToken := createTestUser(t, db, "joiner2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval Node", "approval-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/approval-node/join", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "pending" {
		t.Errorf("expected status=pending, got %v", result["status"])
	}
}

func TestJoinInviteOnlyNode(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin3", "member")
	_, userToken := createTestUser(t, db, "joiner3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Invite Node", "invite-node", "invite_only")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/invite-node/join", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminApprovesPendingMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin4", "member")
	user, _ := createTestUser(t, db, "pending4", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approve Node", "approve-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "pending")

	body := map[string]string{"status": "active"}
	r := authedRequest("PATCH", "/api/v1/nodes/approve-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "active" {
		t.Errorf("expected status=active, got %v", result["status"])
	}
}

func TestAdminRejectsPendingMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin5", "member")
	user, _ := createTestUser(t, db, "pending5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Reject Node", "reject-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "pending")

	body := map[string]string{"status": "left"}
	r := authedRequest("PATCH", "/api/v1/nodes/reject-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "left" {
		t.Errorf("expected status=left, got %v", result["status"])
	}
}

func TestRoleChange(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin6", "member")
	user, _ := createTestUser(t, db, "member6", "member")
	nodeID := createTestNode(t, db, admin.ID, "Role Node", "role-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	// member -> follower
	body := map[string]string{"role": "follower"}
	r := authedRequest("PATCH", "/api/v1/nodes/role-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["role"] != "follower" {
		t.Errorf("expected role=follower, got %v", result["role"])
	}

	// follower -> admin
	body = map[string]string{"role": "admin"}
	r = authedRequest("PATCH", "/api/v1/nodes/role-node/members/"+user.ID, body, adminToken)
	w = serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result = decodeJSON(t, w)
	if result["role"] != "admin" {
		t.Errorf("expected role=admin, got %v", result["role"])
	}
}

func TestNonAdminCannotChangeRoles(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin7", "member")
	_, memberToken := createTestUser(t, db, "member7", "member")
	target, _ := createTestUser(t, db, "target7", "member")
	nodeID := createTestNode(t, db, admin.ID, "Perms Node", "perms-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, target.ID, nodeID, "member", "active")

	body := map[string]string{"role": "moderator"}
	r := authedRequest("PATCH", "/api/v1/nodes/perms-node/members/"+target.ID, body, memberToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLeaveNode(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin8", "member")
	user, userToken := createTestUser(t, db, "leaver8", "member")
	nodeID := createTestNode(t, db, admin.ID, "Leave Node", "leave-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/nodes/leave-node/leave", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&status)
	if status != "left" {
		t.Errorf("expected status=left, got %s", status)
	}
}

func TestCannotLeaveAsLastAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin9", "member")
	nodeID := createTestNode(t, db, admin.ID, "Solo Node", "solo-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("POST", "/api/v1/nodes/solo-node/leave", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreatorGetsAdminOnNodeCreation(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "creator10", "member")

	body := map[string]string{
		"name":              "Created Node",
		"membership_policy": "approval_required",
	}
	r := authedRequest("POST", "/api/v1/nodes", body, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	nodeID, ok := node["id"].(string)
	if !ok || nodeID == "" {
		t.Fatal("expected node ID in response")
	}

	var role, status string
	err := db.QueryRow("SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&role, &status)
	if err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if role != "admin" {
		t.Errorf("expected role=admin, got %s", role)
	}
	if status != "active" {
		t.Errorf("expected status=active, got %s", status)
	}

	var policy string
	db.QueryRow("SELECT membership_policy FROM nodes WHERE id = ?", nodeID).Scan(&policy)
	if policy != "approval_required" {
		t.Errorf("expected membership_policy=approval_required, got %s", policy)
	}
}

func TestDuplicateJoinReturnsConflict(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin11", "member")
	user, userToken := createTestUser(t, db, "dup11", "member")
	nodeID := createTestNode(t, db, admin.ID, "Dup Node", "dup-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/nodes/dup-node/join", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListMembers(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin12", "member")
	_, _ = createTestUser(t, db, "member12", "member")
	nodeID := createTestNode(t, db, admin.ID, "List Node", "list-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Look up member12's ID from DB since createTestUser returns it.
	var member12ID string
	db.QueryRow("SELECT id FROM users WHERE username = 'member12'").Scan(&member12ID)
	createTestMembership(t, db, member12ID, nodeID, "member", "active")

	r := authedRequest("GET", "/api/v1/nodes/list-node/members", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 members, got %d", len(items))
	}
}

func TestListMyMemberships(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "me13", "member")
	owner, _ := createTestUser(t, db, "owner13", "member")
	nodeID1 := createTestNode(t, db, owner.ID, "My Node 1", "my-node-1", "open")
	nodeID2 := createTestNode(t, db, owner.ID, "My Node 2", "my-node-2", "open")
	createTestMembership(t, db, user.ID, nodeID1, "member", "active")
	createTestMembership(t, db, user.ID, nodeID2, "admin", "active")

	r := authedRequest("GET", "/api/v1/users/me/memberships", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/me/memberships", middleware.AuthRequired(db, handler.ListMyMemberships(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 memberships, got %d", len(items))
	}
}

func TestRejoinAfterLeaving(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin14", "member")
	user, userToken := createTestUser(t, db, "rejoiner14", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rejoin Node", "rejoin-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "left")

	r := authedRequest("POST", "/api/v1/nodes/rejoin-node/join", nil, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "active" {
		t.Errorf("expected status=active, got %v", result["status"])
	}
}

func TestCannotDemoteLastAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin15", "member")
	nodeID := createTestNode(t, db, admin.ID, "Demote Node", "demote-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"role": "member"}
	r := authedRequest("PATCH", "/api/v1/nodes/demote-node/members/"+admin.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSiteAdminCanUpdateMembers(t *testing.T) {
	db := setupTestDB(t)
	_, siteAdminToken := createTestUser(t, db, "siteadmin16", "admin")
	nodeOwner, _ := createTestUser(t, db, "owner16", "member")
	target, _ := createTestUser(t, db, "target16", "member")
	nodeID := createTestNode(t, db, nodeOwner.ID, "SiteAdmin Node", "siteadmin-node", "open")
	createTestMembership(t, db, nodeOwner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, target.ID, nodeID, "member", "active")

	body := map[string]string{"role": "follower"}
	r := authedRequest("PATCH", "/api/v1/nodes/siteadmin-node/members/"+target.ID, body, siteAdminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["role"] != "follower" {
		t.Errorf("expected role=follower, got %v", result["role"])
	}
}
