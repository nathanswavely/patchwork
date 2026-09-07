package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
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
	token, err := auth.CreateSession(db, id, "127.0.0.1", "test-agent")
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

// --- Join sheet intro message (docs/adr/040) ---

func TestJoinMessageStoredOnApprovalRequiredRequest(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin17", "member")
	_, userToken := createTestUser(t, db, "joiner17", "member")
	nodeID := createTestNode(t, db, admin.ID, "Message Node", "message-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"message": "  Hi, I'd love to help with sound.  "}
	r := authedRequest("POST", "/api/v1/nodes/message-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "pending" {
		t.Fatalf("expected status=pending, got %v", result["status"])
	}

	var joinerID string
	db.QueryRow("SELECT id FROM users WHERE username = 'joiner17'").Scan(&joinerID)

	var msg sql.NullString
	if err := db.QueryRow("SELECT join_message FROM memberships WHERE user_id = ? AND node_id = ?", joinerID, nodeID).Scan(&msg); err != nil {
		t.Fatalf("query join_message: %v", err)
	}
	if !msg.Valid || msg.String != "Hi, I'd love to help with sound." {
		t.Errorf("expected trimmed join_message, got %+v", msg)
	}

	// Visible to the patch's admins in the pending queue.
	pendingReq := authedRequest("GET", "/api/v1/nodes/message-node/members?status=pending", nil, adminToken)
	pendingW := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), pendingReq)
	if pendingW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", pendingW.Code, pendingW.Body.String())
	}
	pendingResult := decodeJSON(t, pendingW)
	items, ok := pendingResult["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 pending item, got %v", pendingResult["items"])
	}
	item := items[0].(map[string]interface{})
	if item["join_message"] != "Hi, I'd love to help with sound." {
		t.Errorf("expected join_message in pending listing, got %v", item["join_message"])
	}
}

func TestJoinMessageIgnoredForFollower(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin18", "member")
	_, userToken := createTestUser(t, db, "follower18", "member")
	nodeID := createTestNode(t, db, admin.ID, "Follow Message Node", "follow-message-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"role": "follower", "message": "please let me follow"}
	r := authedRequest("POST", "/api/v1/nodes/follow-message-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var followerID string
	db.QueryRow("SELECT id FROM users WHERE username = 'follower18'").Scan(&followerID)

	var msg sql.NullString
	if err := db.QueryRow("SELECT join_message FROM memberships WHERE user_id = ? AND node_id = ?", followerID, nodeID).Scan(&msg); err != nil {
		t.Fatalf("query join_message: %v", err)
	}
	if msg.Valid {
		t.Errorf("expected join_message to be ignored for a follow, got %q", msg.String)
	}
}

func TestJoinMessageIgnoredForOpenJoin(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin19", "member")
	_, userToken := createTestUser(t, db, "opener19", "member")
	nodeID := createTestNode(t, db, admin.ID, "Open Message Node", "open-message-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"message": "excited to join!"}
	r := authedRequest("POST", "/api/v1/nodes/open-message-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "active" {
		t.Fatalf("expected status=active, got %v", result["status"])
	}

	var openerID string
	db.QueryRow("SELECT id FROM users WHERE username = 'opener19'").Scan(&openerID)

	var msg sql.NullString
	if err := db.QueryRow("SELECT join_message FROM memberships WHERE user_id = ? AND node_id = ?", openerID, nodeID).Scan(&msg); err != nil {
		t.Fatalf("query join_message: %v", err)
	}
	if msg.Valid {
		t.Errorf("expected join_message to be ignored for an open join, got %q", msg.String)
	}
}

func TestJoinMessageTooLongRejected(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin20", "member")
	_, userToken := createTestUser(t, db, "joiner20", "member")
	nodeID := createTestNode(t, db, admin.ID, "Long Message Node", "long-message-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"message": strings.Repeat("a", 501)}
	r := authedRequest("POST", "/api/v1/nodes/long-message-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestJoinMessageAbsentFromPublicListing(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin21", "member")
	_, userToken := createTestUser(t, db, "joiner21", "member")
	nodeID := createTestNode(t, db, admin.ID, "Public Listing Node", "public-listing-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"message": "a private note for the admins"}
	r := authedRequest("POST", "/api/v1/nodes/public-listing-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// An anonymous request for the pending queue is silently downgraded to the
	// active/public listing (existing behavior) — join_message must not leak.
	publicReq := authedRequest("GET", "/api/v1/nodes/public-listing-node/members?status=pending", nil, "")
	publicW := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), publicReq)
	if publicW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", publicW.Code, publicW.Body.String())
	}
	if strings.Contains(publicW.Body.String(), "join_message") {
		t.Errorf("join_message must never appear in a public listing, got body: %s", publicW.Body.String())
	}
}

func TestJoinMessageNulledAfterApproval(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin22", "member")
	_, userToken := createTestUser(t, db, "joiner22", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approve Message Node", "approve-message-node", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]string{"message": "would love to get involved"}
	r := authedRequest("POST", "/api/v1/nodes/approve-message-node/join", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var joinerID string
	db.QueryRow("SELECT id FROM users WHERE username = 'joiner22'").Scan(&joinerID)

	var before sql.NullString
	db.QueryRow("SELECT join_message FROM memberships WHERE user_id = ? AND node_id = ?", joinerID, nodeID).Scan(&before)
	if !before.Valid {
		t.Fatalf("expected join_message to be set before approval")
	}

	approveBody := map[string]string{"status": "active"}
	approveReq := authedRequest("PATCH", "/api/v1/nodes/approve-message-node/members/"+joinerID, approveBody, adminToken)
	approveW := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", approveW.Code, approveW.Body.String())
	}

	var after sql.NullString
	db.QueryRow("SELECT join_message FROM memberships WHERE user_id = ? AND node_id = ?", joinerID, nodeID).Scan(&after)
	if after.Valid {
		t.Errorf("expected join_message to be nulled after approval, got %q", after.String)
	}
}

// A ban has to reach the database. Between migrations 004 and 065 the
// memberships CHECK constraint listed only active/pending/left, so the write
// UpdateMember makes failed and the admin got a 500 while the person stayed an
// active member. Assert the row, not just the response code — the handler's
// own JSON reported the status it intended to write.
func TestAdminBansMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin20", "member")
	user, _ := createTestUser(t, db, "banned20", "member")
	nodeID := createTestNode(t, db, admin.ID, "Ban Node", "ban-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	body := map[string]string{"status": "banned"}
	r := authedRequest("PATCH", "/api/v1/nodes/ban-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["status"] != "banned" {
		t.Errorf("expected status=banned in response, got %v", result["status"])
	}

	var status string
	if err := db.QueryRow(
		"SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID,
	).Scan(&status); err != nil {
		t.Fatalf("read membership: %v", err)
	}
	if status != "banned" {
		t.Errorf("expected the row to reach status=banned, got %q", status)
	}
}

// A follower can be removed too — the ban branch takes any active row.
func TestAdminBansFollower(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin21", "member")
	user, _ := createTestUser(t, db, "banned21", "member")
	nodeID := createTestNode(t, db, admin.ID, "Ban Follower Node", "ban-follower-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "follower", "active")

	body := map[string]string{"status": "banned"}
	r := authedRequest("PATCH", "/api/v1/nodes/ban-follower-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&status)
	if status != "banned" {
		t.Errorf("expected the row to reach status=banned, got %q", status)
	}
}

// The point of the ban: the person cannot walk back in through the open door.
func TestBannedUserCannotRejoin(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin22", "member")
	user, userToken := createTestUser(t, db, "banned22", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rejoin Node", "rejoin-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	body := map[string]string{"status": "banned"}
	r := authedRequest("PATCH", "/api/v1/nodes/rejoin-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("ban: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	r = authedRequest("POST", "/api/v1/nodes/rejoin-node/join", nil, userToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("rejoin: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "removed from this community") {
		t.Errorf("expected the removal message, got %s", w.Body.String())
	}

	// Following is refused on the same ground — the ban check precedes the
	// follow branch, so a removed person cannot re-enter as an observer.
	r = authedRequest("POST", "/api/v1/nodes/rejoin-node/join", map[string]string{"role": "follower"}, userToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("refollow: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// Reinstating drops the row to 'left', which is what lets the person choose to
// come back rather than being put back.
func TestAdminReinstatesBannedMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin23", "member")
	user, userToken := createTestUser(t, db, "banned23", "member")
	nodeID := createTestNode(t, db, admin.ID, "Reinstate Node", "reinstate-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	body := map[string]string{"status": "banned"}
	r := authedRequest("PATCH", "/api/v1/nodes/reinstate-node/members/"+user.ID, body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("ban: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body = map[string]string{"status": "left"}
	r = authedRequest("PATCH", "/api/v1/nodes/reinstate-node/members/"+user.ID, body, adminToken)
	w = serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("reinstate: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&status)
	if status != "left" {
		t.Errorf("expected status=left after reinstate, got %q", status)
	}

	// The audit trail distinguishes reinstating from rejecting.
	var action string
	db.QueryRow("SELECT action FROM audit_log WHERE action = 'membership.reinstate'").Scan(&action)
	if action != "membership.reinstate" {
		t.Errorf("expected a membership.reinstate audit event, got %q", action)
	}

	// And the door is open again.
	r = authedRequest("POST", "/api/v1/nodes/reinstate-node/join", nil, userToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("rejoin after reinstate: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// The admin-only ?status=banned filter had nothing to list before 065.
func TestListMembersBannedFilter(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin24", "member")
	user, _ := createTestUser(t, db, "banned24", "member")
	nodeID := createTestNode(t, db, admin.ID, "List Ban Node", "list-ban-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, user.ID, nodeID, "member", "banned")

	r := authedRequest("GET", "/api/v1/nodes/list-ban-node/members?status=banned", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "banned24") {
		t.Errorf("expected the removed member in the list, got %s", w.Body.String())
	}
}

// A CHECK-constrained field takes its value from the request on three paths.
// Before this, each handed the constraint an unrecognized value and turned its
// rejection into a 500 "failed to create node" / "failed to update report" —
// an error that reads like the server broke rather than like the caller sent
// something it does not accept. The constraint is the backstop; it is not the
// error message.
func TestUnrecognizedEnumValuesAre400(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerTok := createTestUser(t, db, "enum1", "member")
	_, adminTok := createTestUser(t, db, "enum2", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Enum", "enum-node", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	if _, err := db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details)
		 VALUES ('enum-report', ?, 'node', ?, 'spam', '')`, owner.ID, nodeID); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	create := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("POST", "/api/v1/nodes", body, ownerTok)
		return serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)
	}
	update := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/nodes/enum-node", body, ownerTok)
		return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	}
	report := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/admin/reports/enum-report", body, adminTok)
		return serveMux(t, db, "PATCH", "/api/v1/admin/reports/{id}", handler.UpdateReport(db), r)
	}

	refused := []struct {
		name string
		got  *httptest.ResponseRecorder
	}{
		{"create with an unknown visibility", create(map[string]interface{}{"name": "A", "visibility": "secret"})},
		{"create with an unknown membership policy", create(map[string]interface{}{"name": "B", "membership_policy": "members_only"})},
		{"update to an unknown visibility", update(map[string]interface{}{"visibility": "secret"})},
		{"report set to an unknown status", report(map[string]interface{}{"status": "closed"})},
	}
	for _, tt := range refused {
		if tt.got.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", tt.name, tt.got.Code, tt.got.Body.String())
		}
	}

	// The refusal explains itself rather than saying the server failed.
	if body := refused[0].got.Body.String(); !strings.Contains(body, "visibility must be one of") {
		t.Errorf("expected the 400 to name the accepted values, got %s", body)
	}

	// Nothing was written on the way to any of those refusals.
	var reportStatus string
	db.QueryRow("SELECT status FROM content_reports WHERE id = 'enum-report'").Scan(&reportStatus)
	if reportStatus != "pending" {
		t.Errorf("a refused update still changed the report: %q", reportStatus)
	}
	var vis string
	db.QueryRow("SELECT visibility FROM nodes WHERE slug = 'enum-node'").Scan(&vis)
	if vis != "public" {
		t.Errorf("a refused update still changed the patch: %q", vis)
	}

	// Every value the constraints do accept still works.
	for _, v := range []string{"public", "private", "unlisted"} {
		if w := update(map[string]interface{}{"visibility": v}); w.Code != http.StatusOK {
			t.Errorf("visibility %q: expected 200, got %d: %s", v, w.Code, w.Body.String())
		}
	}
	for i, p := range []string{"open", "approval_required", "invite_only"} {
		body := map[string]interface{}{"name": fmt.Sprintf("Policy %d", i), "membership_policy": p}
		if w := create(body); w.Code != http.StatusCreated {
			t.Errorf("membership_policy %q: expected 201, got %d: %s", p, w.Code, w.Body.String())
		}
	}
	for _, st := range []string{"pending", "reviewed", "resolved", "dismissed"} {
		if w := report(map[string]interface{}{"status": st}); w.Code != http.StatusOK {
			t.Errorf("report status %q: expected 200, got %d: %s", st, w.Code, w.Body.String())
		}
	}

	// A request that carries an action and no status at all: the admin SPA
	// sends exactly this for remove_image, whose action has no entry in its
	// status map, so the key is dropped from the JSON. Absent is not invalid.
	if w := report(map[string]interface{}{"action": "remove_image"}); w.Code != http.StatusOK {
		t.Errorf("action without a status: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// An unrecognized action used to be ignored in silence: the report was marked
// resolved, the moderation the admin asked for never happened, and the response
// said it had. The patch-side queue in notice_reports.go always refused one;
// this is the instance panel catching up.
func TestUnrecognizedReportActionIs400(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "act1", "member")
	_, adminTok := createTestUser(t, db, "act2", "admin")
	nodeID := createTestNode(t, db, owner.ID, "Act", "act-node", "open")

	report := func(id string, body map[string]interface{}) *httptest.ResponseRecorder {
		if _, err := db.Exec(
			`INSERT OR REPLACE INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details)
			 VALUES (?, ?, 'node', ?, 'spam', '')`, id, owner.ID, nodeID); err != nil {
			t.Fatalf("seed report %s: %v", id, err)
		}
		r := authedRequest("PATCH", "/api/v1/admin/reports/"+id, body, adminTok)
		return serveMux(t, db, "PATCH", "/api/v1/admin/reports/{id}", handler.UpdateReport(db), r)
	}

	w := report("act-bad", map[string]interface{}{"status": "resolved", "action": "banish"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "action must be one of") {
		t.Errorf("expected the 400 to name the accepted actions, got %s", w.Body.String())
	}

	// The status is written before the switch runs, so refusing a bad action
	// anywhere after that write would leave the report resolved by a request
	// that was rejected. It must not move at all.
	var status string
	db.QueryRow("SELECT status FROM content_reports WHERE id = 'act-bad'").Scan(&status)
	if status != "pending" {
		t.Errorf("a refused action still resolved the report: %q", status)
	}

	// Every action the admin panel's menu offers, plus remove_image, which the
	// API serves and that menu does not currently show. dismiss and warn have
	// no case in the switch on purpose; refusing them would break the panel's
	// two most-used buttons, dismiss being its default.
	for i, action := range []string{
		"dismiss", "warn", "remove_content", "reset_appearance", "suspend_user", "remove_image",
	} {
		id := fmt.Sprintf("act-ok-%d", i)
		status := "resolved"
		if action == "dismiss" {
			status = "dismissed"
		}
		if w := report(id, map[string]interface{}{"status": status, "action": action}); w.Code != http.StatusOK {
			t.Errorf("action %q: expected 200, got %d: %s", action, w.Code, w.Body.String())
		}
	}

	// A request with no action at all still just sets the status.
	if w := report("act-none", map[string]interface{}{"status": "reviewed"}); w.Code != http.StatusOK {
		t.Errorf("status without an action: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// "Warn" warned nobody. It set the report to resolved, and the only
// notification the handler sent went to the reporter — so the person whose
// content was reported was never told, and a warning that reaches no one is
// not a remedy. Now it reaches the account behind the reported content.
func TestWarnNotifiesTheReportedUser(t *testing.T) {
	db := setupTestDB(t)
	reporter, _ := createTestUser(t, db, "warn-reporter", "member")
	owner, _ := createTestUser(t, db, "warn-owner", "member")
	_, adminTok := createTestUser(t, db, "warn-admin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Sheep Barn", "sheep-barn", "open")
	var eventID string
	db.QueryRow(`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, visibility)
	             VALUES (?, ?, ?, 'Barn Dance', '', 'Barn', '2026-10-01T00:00:00Z', 'public') RETURNING id`,
		auth.NewUUIDv7(), nodeID, owner.ID).Scan(&eventID)

	warn := func(reportID, entityType, entityID string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details)
			 VALUES (?, ?, ?, ?, 'harassment by warn-reporter', 'details naming the reporter')`,
			reportID, reporter.ID, entityType, entityID); err != nil {
			t.Fatalf("seed %s: %v", reportID, err)
		}
		body := map[string]interface{}{
			"status": "resolved", "action": "warn",
			"resolution_note": "internal note: warn-reporter flagged this again",
		}
		r := authedRequest("PATCH", "/api/v1/admin/reports/"+reportID, body, adminTok)
		w := serveMux(t, db, "PATCH", "/api/v1/admin/reports/{id}", handler.UpdateReport(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("warn %s: expected 200, got %d: %s", reportID, w.Code, w.Body.String())
		}
	}

	// A report about a patch warns its owner and names the patch.
	warn("w-node", "node", nodeID)
	var title, body, link string
	if err := db.QueryRow(
		`SELECT title, body, link FROM notifications WHERE user_id = ? AND type = 'account.warned'`,
		owner.ID).Scan(&title, &body, &link); err != nil {
		t.Fatalf("the reported user was not warned: %v", err)
	}
	if !strings.Contains(title, "Sheep Barn") {
		t.Errorf("the warning does not say what was reported: %q", title)
	}
	if link != "/patches/sheep-barn" {
		t.Errorf("expected a link to the reported patch, got %q", link)
	}

	// It must not carry the reporter's identity, their words, or the admin's
	// note — all of which are written for moderators and can name the person
	// who reported. This is the property that makes reporting safe.
	for _, leak := range []string{"warn-reporter", "harassment", "details naming", "internal note"} {
		if strings.Contains(title+" "+body, leak) {
			t.Errorf("the warning leaks %q to the person reported: %q / %q", leak, title, body)
		}
	}

	// The reporter still hears that their report was reviewed, and hears
	// nothing about the outcome.
	var reporterCount int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'report.resolved'`,
		reporter.ID).Scan(&reporterCount)
	if reporterCount != 1 {
		t.Errorf("expected the reporter to be told their report was reviewed, got %d", reporterCount)
	}
	var reporterWarned int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'account.warned'`,
		reporter.ID).Scan(&reporterWarned)
	if reporterWarned != 0 {
		t.Errorf("the reporter was warned about their own report")
	}

	// A report about an event warns its creator; one about an account warns
	// that account. Same resolution suspend_user uses — the two must agree, or
	// a warning reaches somebody a suspension would not.
	warn("w-event", "event", eventID)
	warn("w-user", "user", owner.ID)
	var warned int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'account.warned'`,
		owner.ID).Scan(&warned)
	if warned != 3 {
		t.Errorf("expected a warning for each of the patch, event and account reports, got %d", warned)
	}

	// A warning changes nothing about the content it is about.
	var removed sql.NullString
	db.QueryRow("SELECT removed_at FROM nodes WHERE id = ?", nodeID).Scan(&removed)
	if removed.Valid && removed.String != "" {
		t.Errorf("warning removed the patch: %q", removed.String)
	}
	var suspended sql.NullString
	db.QueryRow("SELECT suspended_at FROM users WHERE id = ?", owner.ID).Scan(&suspended)
	if suspended.Valid && suspended.String != "" {
		t.Errorf("warning suspended the account: %q", suspended.String)
	}

	// And it is on the record as a moderation act.
	var audits int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'admin.user_warn'`).Scan(&audits)
	if audits != 3 {
		t.Errorf("expected 3 audited warnings, got %d", audits)
	}

	// Dismissing still warns nobody.
	if _, err := db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details)
		 VALUES ('w-dismiss', ?, 'node', ?, 'spam', '')`, reporter.ID, nodeID); err != nil {
		t.Fatal(err)
	}
	r := authedRequest("PATCH", "/api/v1/admin/reports/w-dismiss",
		map[string]interface{}{"status": "dismissed", "action": "dismiss"}, adminTok)
	if w := serveMux(t, db, "PATCH", "/api/v1/admin/reports/{id}", handler.UpdateReport(db), r); w.Code != http.StatusOK {
		t.Fatalf("dismiss: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'account.warned'`,
		owner.ID).Scan(&warned)
	if warned != 3 {
		t.Errorf("dismissing a report warned somebody: now %d warnings", warned)
	}
}

// The notifications list is filtered by category server-side. The mapping used
// to be derived from the category name — "proposals" → "proposal.%" — which
// held only while every category was one prefix with an s on the end.
// Moderation is neither: it spans account. and report., and leaves one
// account. type out deliberately.
func TestNotificationCategoryFilter(t *testing.T) {
	db := setupTestDB(t)
	user, tok := createTestUser(t, db, "notif-cat", "member")

	for _, n := range []struct{ typ, title string }{
		{"account.warned", "warned"},
		{"account.suspended", "suspended"},
		{"account.unsuspended", "restored"},
		{"report.resolved", "report reviewed"},
		{"account.email_changed", "email changed"},
		{"proposal.created", "a proposal"},
		{"membership.banned", "removed"},
		{"notice.posted", "a notice"},
	} {
		if _, err := db.Exec(
			`INSERT INTO notifications (id, user_id, type, title, body, link) VALUES (?, ?, ?, ?, '', '')`,
			auth.NewUUIDv7(), user.ID, n.typ, n.title); err != nil {
			t.Fatalf("seed %s: %v", n.typ, err)
		}
	}

	list := func(category string) (*httptest.ResponseRecorder, []string) {
		t.Helper()
		path := "/api/v1/notifications"
		if category != "" {
			path += "?category=" + category
		}
		r := authedRequest("GET", path, nil, tok)
		w := serveMux(t, db, "GET", "/api/v1/notifications", handler.ListNotifications(db), r)
		var got []string
		if w.Code == http.StatusOK {
			var resp struct {
				Items []struct {
					Type string `json:"type"`
				} `json:"items"`
			}
			json.Unmarshal(w.Body.Bytes(), &resp)
			for _, it := range resp.Items {
				got = append(got, it.Type)
			}
			sort.Strings(got)
		}
		return w, got
	}

	_, moderation := list("moderation")
	want := []string{"account.suspended", "account.unsuspended", "account.warned", "report.resolved"}
	if strings.Join(moderation, " ") != strings.Join(want, " ") {
		t.Errorf("moderation filter\n got: %v\nwant: %v", moderation, want)
	}

	// An admin setting an address (docs/adr/072) is account security, not a
	// moderation outcome. Filing it here would tell somebody they had been
	// moderated when they had not.
	for _, typ := range moderation {
		if typ == "account.email_changed" {
			t.Error("account.email_changed is not a moderation outcome")
		}
	}

	// The categories that already worked still do, and none of them picks up
	// a moderation notice.
	for category, want := range map[string]string{
		"proposals":  "proposal.created",
		"membership": "membership.banned",
	} {
		_, got := list(category)
		if strings.Join(got, " ") != want {
			t.Errorf("%s filter: got %v, want [%s]", category, got, want)
		}
	}

	// Everything is still reachable unfiltered, including the two types that
	// belong to no tab.
	if _, all := list(""); len(all) != 8 {
		t.Errorf("unfiltered list: got %d notifications, want 8 (%v)", len(all), all)
	}

	// A category now contributes several bound parameters where it used to
	// contribute one, and the cursor's binds follow them. Paging within a
	// category is where that ordering would come apart.
	r := authedRequest("GET", "/api/v1/notifications?category=moderation&limit=2", nil, tok)
	w := serveMux(t, db, "GET", "/api/v1/notifications", handler.ListNotifications(db), r)
	var page struct {
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first page: got %d items, cursor %q", len(page.Items), page.NextCursor)
	}
	r = authedRequest("GET", "/api/v1/notifications?category=moderation&limit=2&after="+page.NextCursor, nil, tok)
	w = serveMux(t, db, "GET", "/api/v1/notifications", handler.ListNotifications(db), r)
	var rest struct {
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &rest)
	if len(rest.Items) != 2 {
		t.Errorf("second page of a filtered list: got %d items, want 2", len(rest.Items))
	}
	for _, it := range rest.Items {
		if it.Type == "account.email_changed" || it.Type == "proposal.created" {
			t.Errorf("paging past the first page dropped the category filter: %s", it.Type)
		}
	}

	// A category with no mapping is refused rather than answered with an empty
	// list, which reads as "you have no notifications". The noticeboard is the
	// live example: its types are notice.*, so the old derivation would have
	// served a Noticeboard tab an empty list forever.
	for _, bogus := range []string{"noticeboard", "banana"} {
		w, _ := list(bogus)
		if w.Code != http.StatusBadRequest {
			t.Errorf("category=%s: expected 400, got %d: %s", bogus, w.Code, w.Body.String())
		}
	}
}

// A patch bigger than one page must still report its true size: the header
// states the count as a fact, so it is counted server-side under exactly the
// filter the listing ran, not derived from the page that happened to load.
func TestListMembersCountsWholePatchNotJustThePage(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin60", "member")
	nodeID := createTestNode(t, db, admin.ID, "Big Node", "big-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// 24 more members and 3 followers — past the default limit of 20.
	for i := 0; i < 24; i++ {
		u, _ := createTestUser(t, db, fmt.Sprintf("big%02d", i), "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}
	for i := 0; i < 3; i++ {
		u, _ := createTestUser(t, db, fmt.Sprintf("bigf%02d", i), "member")
		createTestMembership(t, db, u.ID, nodeID, "follower", "active")
	}

	r := authedRequest("GET", "/api/v1/nodes/big-node/members", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)

	items, _ := result["items"].([]interface{})
	if len(items) != 20 {
		t.Errorf("expected one page of 20 rows, got %d", len(items))
	}
	if result["next_cursor"] == "" {
		t.Error("expected a next_cursor: the patch is bigger than one page")
	}
	if got := result["member_count"].(float64); got != 25 {
		t.Errorf("expected member_count=25 (1 admin + 24 members), got %v", got)
	}
	if got := result["follower_count"].(float64); got != 3 {
		t.Errorf("expected follower_count=3, got %v", got)
	}

	// Paging to the end must reach every row.
	seen := len(items)
	cursor, _ := result["next_cursor"].(string)
	for cursor != "" && seen < 100 {
		r = authedRequest("GET", "/api/v1/nodes/big-node/members?after="+cursor, nil, adminToken)
		w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
		page := decodeJSON(t, w)
		items, _ = page["items"].([]interface{})
		seen += len(items)
		cursor, _ = page["next_cursor"].(string)
	}
	if seen != 28 {
		t.Errorf("expected to page through all 28 rows, saw %d", seen)
	}
}

// The totals must match what this viewer's listing can actually show, or the
// header promises rows the list will never hand over: an outsider sees only
// visible member/admin rows (docs/adr/006), so hidden rows and followers are
// outside their count.
func TestListMembersCountsHonourTheViewersVisibility(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "admin61", "member")
	nodeID := createTestNode(t, db, admin.ID, "Quiet Node", "quiet-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	hidden, _ := createTestUser(t, db, "hidden61", "member")
	hiddenID := createTestMembership(t, db, hidden.ID, nodeID, "member", "active")
	if _, err := db.Exec("UPDATE memberships SET visible = 0 WHERE id = ?", hiddenID); err != nil {
		t.Fatalf("hide membership: %v", err)
	}
	follower, _ := createTestUser(t, db, "follow61", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	// The room's own admin counts everybody the listing carries.
	r := authedRequest("GET", "/api/v1/nodes/quiet-node/members", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	inside := decodeJSON(t, w)
	if got := inside["member_count"].(float64); got != 2 {
		t.Errorf("insider: expected member_count=2, got %v", got)
	}
	if got := inside["follower_count"].(float64); got != 1 {
		t.Errorf("insider: expected follower_count=1, got %v", got)
	}

	// An anonymous visitor counts only the rows they can be shown.
	r = authedRequest("GET", "/api/v1/nodes/quiet-node/members", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	outside := decodeJSON(t, w)
	items, _ := outside["items"].([]interface{})
	if got := outside["member_count"].(float64); got != float64(len(items)) || got != 1 {
		t.Errorf("outsider: expected member_count=1 matching %d listed rows, got %v", len(items), got)
	}
	if got := outside["follower_count"].(float64); got != 0 {
		t.Errorf("outsider: expected follower_count=0 (followers are not public), got %v", got)
	}
}

// The offer to share a contact card is a fact about the room, not about the
// page that loaded. A member whose own row sits past the first page must
// still be told they are sharing nothing, and must not be told otherwise.
func TestListMembersSharingFactsSurviveTheFirstPage(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin62", "member")
	nodeID := createTestNode(t, db, admin.ID, "Deep Node", "deep-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// 30 members ahead of our two, so neither lands on the first page.
	for i := 0; i < 30; i++ {
		u, _ := createTestUser(t, db, fmt.Sprintf("deep%02d", i), "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}

	// A sharer and a non-sharer, both far down the list.
	sharer, sharerToken := createTestUser(t, db, "sharer62", "member")
	sharerMID := createTestMembership(t, db, sharer.ID, nodeID, "member", "active")
	if _, err := db.Exec("UPDATE users SET contact_email = 'sharer@example.com' WHERE id = ?", sharer.ID); err != nil {
		t.Fatalf("set card: %v", err)
	}
	if _, err := db.Exec("UPDATE memberships SET share_contact = 1 WHERE id = ?", sharerMID); err != nil {
		t.Fatalf("share card: %v", err)
	}
	quiet, quietToken := createTestUser(t, db, "quiet62", "member")
	createTestMembership(t, db, quiet.ID, nodeID, "member", "active")

	get := func(token string) map[string]interface{} {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/deep-node/members", nil, token)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)
	}

	// Neither of them is on the first page — that is the whole point.
	first := get(quietToken)
	items, _ := first["items"].([]interface{})
	for _, it := range items {
		if u, _ := it.(map[string]interface{})["username"].(string); u == "quiet62" || u == "sharer62" {
			t.Fatalf("test is not exercising paging: %s landed on the first page", u)
		}
	}

	if first["viewer_shares_contact"] != false {
		t.Errorf("a member who shares nothing must be offered the switch, got %v", first["viewer_shares_contact"])
	}
	if first["any_contact_shared"] != true {
		t.Errorf("someone in this room does share a card, got %v", first["any_contact_shared"])
	}

	if got := get(sharerToken)["viewer_shares_contact"]; got != true {
		t.Errorf("a member who already shares must not be offered the switch, got %v", got)
	}

	// An outsider is told neither: there is no room to be offered anything in.
	anon := authedRequest("GET", "/api/v1/nodes/deep-node/members", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), anon)
	out := decodeJSON(t, w)
	if _, present := out["viewer_shares_contact"]; present {
		t.Error("the sharing facts must not be sent to a viewer outside the room")
	}
	if _, present := out["any_contact_shared"]; present {
		t.Error("the sharing facts must not be sent to a viewer outside the room")
	}
}

// An empty card is not a shared card: switching sharing on while the account
// carries nothing to share leaves the row without a card, so the offer stands.
func TestListMembersEmptyCardIsNotSharing(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "admin63", "member")
	nodeID := createTestNode(t, db, admin.ID, "Empty Node", "empty-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	user, token := createTestUser(t, db, "blank63", "member")
	mid := createTestMembership(t, db, user.ID, nodeID, "member", "active")
	if _, err := db.Exec("UPDATE memberships SET share_contact = 1 WHERE id = ?", mid); err != nil {
		t.Fatalf("share card: %v", err)
	}

	r := authedRequest("GET", "/api/v1/nodes/empty-node/members", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	result := decodeJSON(t, w)
	if got := result["viewer_shares_contact"]; got != false {
		t.Errorf("sharing an empty card shares nothing, got %v", got)
	}
	if got := result["any_contact_shared"]; got != false {
		t.Errorf("an empty card must not count as a card in the room, got %v", got)
	}
}
