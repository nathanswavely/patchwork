package handler_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func TestCreateReport(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "reporter1", "member")
	nodeID := createTestNode(t, db, user.ID, "Reported Node", "reported-node", "open")

	body := map[string]interface{}{
		"entity_type": "node",
		"entity_id":   nodeID,
		"reason":      "spam content",
		"details":     "this is spam",
	}
	r := authedRequest("POST", "/api/v1/reports", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/reports", handler.CreateReport(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["id"] == nil || result["id"] == "" {
		t.Fatal("expected report ID in response")
	}
}

func TestCreateReport_InvalidTarget(t *testing.T) {
	db := setupTestDB(t)
	_, userToken := createTestUser(t, db, "reporter2", "member")

	body := map[string]interface{}{
		"entity_type": "node",
		"entity_id":   "nonexistent-id",
		"reason":      "test",
	}
	r := authedRequest("POST", "/api/v1/reports", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/reports", handler.CreateReport(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminListAndResolveReport(t *testing.T) {
	db := setupTestDB(t)
	reporter, _ := createTestUser(t, db, "reporter3", "member")
	_, adminToken := createTestUser(t, db, "admin-rpt1", "admin")
	nodeID := createTestNode(t, db, reporter.ID, "Rpt Node", "rpt-node", "open")

	// Create a report directly.
	reportID := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details) VALUES (?, ?, 'node', ?, 'spam', 'details')`,
		reportID, reporter.ID, nodeID,
	)

	// List reports as admin.
	r := authedRequest("GET", "/api/v1/admin/reports?status=pending", nil, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/reports", middleware.AdminRequired(db, handler.ListReports(db)))
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
	if len(items) != 1 {
		t.Fatalf("expected 1 report, got %d", len(items))
	}
	first := items[0].(map[string]interface{})
	if first["reporter_name"] == nil || first["reporter_name"] == "" {
		t.Fatal("expected reporter_name to be populated")
	}

	// Resolve the report.
	resolveBody := map[string]interface{}{
		"status":          "resolved",
		"resolution_note": "resolved it",
		"action":          "dismiss",
	}
	r = authedRequest("PATCH", "/api/v1/admin/reports/"+reportID, resolveBody, adminToken)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("PATCH /api/v1/admin/reports/{id}", middleware.AdminRequired(db, handler.UpdateReport(db)))
	w = httptest.NewRecorder()
	mux2.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify notification was created for reporter.
	var notifCount int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'report.resolved'", reporter.ID).Scan(&notifCount)
	if notifCount == 0 {
		t.Fatal("expected notification for reporter")
	}
}

func TestSuspendUserViaReport(t *testing.T) {
	db := setupTestDB(t)
	reporter, _ := createTestUser(t, db, "reporter4", "member")
	target, _ := createTestUser(t, db, "bad-user1", "member")
	_, adminToken := createTestUser(t, db, "admin-rpt2", "admin")

	// Create a report against the user.
	reportID := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details) VALUES (?, ?, 'user', ?, 'harassment', '')`,
		reportID, reporter.ID, target.ID,
	)

	// Resolve with suspend_user action.
	resolveBody := map[string]interface{}{
		"status":          "resolved",
		"resolution_note": "suspended for harassment",
		"action":          "suspend_user",
	}
	r := authedRequest("PATCH", "/api/v1/admin/reports/"+reportID, resolveBody, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/admin/reports/{id}", middleware.AdminRequired(db, handler.UpdateReport(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user is suspended.
	var suspendedAt sql.NullString
	db.QueryRow("SELECT suspended_at FROM users WHERE id = ?", target.ID).Scan(&suspendedAt)
	if !suspendedAt.Valid {
		t.Fatal("expected user to be suspended")
	}
}

// The moderation response to an offensive tile is proportionate: reset the
// patch's appearance to hash-assigned ("the quilt decides now") without
// touching the patch itself (docs/adr/029).
func TestResetAppearanceViaReport(t *testing.T) {
	db := setupTestDB(t)
	reporter, _ := createTestUser(t, db, "reporter5", "member")
	owner, _ := createTestUser(t, db, "tile-owner1", "member")
	_, adminToken := createTestUser(t, db, "admin-rpt3", "admin")
	nodeID := createTestNode(t, db, owner.ID, "Loud Tile", "loud-tile", "open")

	db.Exec(
		`UPDATE nodes SET appearance = '{"block":{"grid":1,"seams":[[0,0,4,4]]},"bundle":["#EC341C"]}' WHERE id = ?`,
		nodeID,
	)

	reportID := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details) VALUES (?, ?, 'node', ?, 'offensive tile', '')`,
		reportID, reporter.ID, nodeID,
	)

	resolveBody := map[string]interface{}{
		"status":          "resolved",
		"resolution_note": "tile reset",
		"action":          "reset_appearance",
	}
	r := authedRequest("PATCH", "/api/v1/admin/reports/"+reportID, resolveBody, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/admin/reports/{id}", middleware.AdminRequired(db, handler.UpdateReport(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Appearance is NULL again; the patch itself is untouched.
	var appearance sql.NullString
	var removedAt sql.NullString
	db.QueryRow("SELECT appearance, removed_at FROM nodes WHERE id = ?", nodeID).Scan(&appearance, &removedAt)
	if appearance.Valid {
		t.Fatalf("expected appearance to be reset to NULL, got %q", appearance.String)
	}
	if removedAt.Valid {
		t.Fatal("reset_appearance must not remove the patch")
	}

	// The action is audit-logged.
	var auditCount int
	db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE action = 'admin.node_update' AND entity_id = ?", nodeID,
	).Scan(&auditCount)
	if auditCount == 0 {
		t.Fatal("expected an audit log entry for the appearance reset")
	}
}

// A suspended ordinary member keeps read access — they can still browse the
// patches they belong to. Only mutation is cut off. See TestSuspendedAdmin
// LosesAdminReads for why admin routes are deliberately stricter.
func TestSuspendedUserCannotMutate(t *testing.T) {
	db := setupTestDB(t)
	_, userToken := createTestUser(t, db, "suspend-test1", "member")

	// Suspend the user.
	db.Exec("UPDATE users SET suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE username = 'suspend-test1'")

	// Try to create a node (POST) -- should fail.
	body := map[string]interface{}{
		"name": "test-node",
	}
	r := authedRequest("POST", "/api/v1/nodes", body, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for suspended user POST, got %d: %s", w.Code, w.Body.String())
	}

	// GET should be allowed.
	r = authedRequest("GET", "/api/v1/nodes", nil, userToken)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("GET /api/v1/nodes", middleware.AuthRequired(db, handler.ListNodes(db)))
	w = httptest.NewRecorder()
	mux2.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for suspended user GET, got %d: %s", w.Code, w.Body.String())
	}
}

// A suspended admin loses admin reads, unlike an ordinary suspended member.
// The routes behind AdminRequired serve the instance export, the user list, and
// the audit log — suspension has to cut those immediately.
func TestSuspendedAdminLosesAdminReads(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "suspend-admin1", "admin")

	db.Exec("UPDATE users SET suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE username = 'suspend-admin1'")

	r := authedRequest("GET", "/api/v1/admin/users", nil, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/users", middleware.AdminRequired(db, handler.ListUsers(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for suspended admin GET, got %d: %s", w.Code, w.Body.String())
	}
}

// Suspending a user revokes their live sessions, so the suspension takes effect
// on the next request rather than whenever a 30-day cookie lapses.
func TestSuspensionRevokesSessions(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "suspender", "admin")
	target, targetToken := createTestUser(t, db, "suspend-target", "member")

	body := map[string]interface{}{"suspended_at": "now"}
	r := authedRequest("PATCH", "/api/v1/admin/users/"+target.ID, body, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", middleware.AdminRequired(db, handler.UpdateUser(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from suspend, got %d: %s", w.Code, w.Body.String())
	}

	var n int
	db.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id = ?", target.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("expected suspended user's sessions to be revoked, %d remain", n)
	}

	// The revoked cookie must no longer authenticate.
	user, err := auth.ValidateSession(db, targetToken)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if user != nil {
		t.Fatal("expected revoked session token to stop validating")
	}
}
