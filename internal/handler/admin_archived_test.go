package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// archiveState reads the columns the archive/restore lifecycle mutates.
func archiveState(t *testing.T, db *database.DB, nodeID string) (status, archivedFrom string) {
	t.Helper()
	var af *string
	if err := db.QueryRow("SELECT status, archived_from FROM nodes WHERE id = ?", nodeID).Scan(&status, &af); err != nil {
		t.Fatalf("node state: %v", err)
	}
	if af != nil {
		archivedFrom = *af
	}
	return status, archivedFrom
}

func restoreNode(t *testing.T, db *database.DB, nodeID, token string) *http.Response {
	t.Helper()
	r := authedRequest("POST", "/api/v1/admin/nodes/"+nodeID+"/restore", nil, token)
	w := serveAdminMux(t, db, "POST", "/api/v1/admin/nodes/{id}/restore", handler.AdminRestoreNode(db), r)
	return w.Result()
}

// Archiving an active patch records where it came from, and restore returns
// it there — the whole lifecycle, patch admin down and instance admin back up.
func TestArchiveThenRestore_ActivePatch(t *testing.T) {
	db := setupTestDB(t)
	patchAdmin, patchToken := createTestUser(t, db, "patch-admin", "member")
	_, adminToken := createTestUser(t, db, "site-admin", "admin")

	nodeID := createTestNode(t, db, patchAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, patchAdmin.ID, nodeID, "admin", "active")

	// Patch admin archives from the danger zone.
	r := authedRequest("DELETE", "/api/v1/nodes/the-selvage", nil, patchToken)
	w := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}", handler.DeleteNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("archive: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if status, from := archiveState(t, db, nodeID); status != "archived" || from != "active" {
		t.Fatalf("after archive: status=%q archived_from=%q, want archived/active", status, from)
	}

	// The patch admin cannot restore — AdminRequired turns them away.
	if resp := restoreNode(t, db, nodeID, patchToken); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("patch admin restore: expected 403, got %d", resp.StatusCode)
	}

	// The instance admin can.
	if resp := restoreNode(t, db, nodeID, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d", resp.StatusCode)
	}
	if status, from := archiveState(t, db, nodeID); status != "active" || from != "" {
		t.Fatalf("after restore: status=%q archived_from=%q, want active/cleared", status, from)
	}

	// Restoring a patch that isn't archived is a conflict, not a no-op.
	if resp := restoreNode(t, db, nodeID, adminToken); resp.StatusCode != http.StatusConflict {
		t.Fatalf("double restore: expected 409, got %d", resp.StatusCode)
	}
}

// An archived unclaimed patch restores to unclaimed, not active — blind
// restore-to-active would strand it claimed with zero admins (docs/adr/034).
func TestRestore_UnclaimedPatchReturnsUnclaimed(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "site-admin", "admin")

	nodeID := createTestNode(t, db, admin.ID, "Ghost Venue", "ghost-venue", "open")
	if _, err := db.Exec("UPDATE nodes SET status = 'unclaimed' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("set unclaimed: %v", err)
	}

	r := authedRequest("DELETE", "/api/v1/nodes/ghost-venue", nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}", handler.DeleteNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("archive: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if status, from := archiveState(t, db, nodeID); status != "archived" || from != "unclaimed" {
		t.Fatalf("after archive: status=%q archived_from=%q, want archived/unclaimed", status, from)
	}

	if resp := restoreNode(t, db, nodeID, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d", resp.StatusCode)
	}
	if status, _ := archiveState(t, db, nodeID); status != "unclaimed" {
		t.Fatalf("after restore: status=%q, want unclaimed", status)
	}
}

// Rows archived before archived_from existed restore by inference: an active
// admin membership means the patch was active, otherwise it was unclaimed.
func TestRestore_LegacyRowsInferPriorStatus(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "legacy-owner", "member")
	_, adminToken := createTestUser(t, db, "site-admin", "admin")

	withAdmins := createTestNode(t, db, owner.ID, "Legacy Band", "legacy-band", "invite_only")
	createTestMembership(t, db, owner.ID, withAdmins, "admin", "active")
	orphan := createTestNode(t, db, owner.ID, "Legacy Listing", "legacy-listing", "open")

	for _, id := range []string{withAdmins, orphan} {
		if _, err := db.Exec("UPDATE nodes SET status = 'archived', archived_from = NULL WHERE id = ?", id); err != nil {
			t.Fatalf("legacy archive: %v", err)
		}
	}

	if resp := restoreNode(t, db, withAdmins, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore with admins: expected 200, got %d", resp.StatusCode)
	}
	if status, _ := archiveState(t, db, withAdmins); status != "active" {
		t.Fatalf("patch with admins restored to %q, want active", status)
	}

	if resp := restoreNode(t, db, orphan, adminToken); resp.StatusCode != http.StatusOK {
		t.Fatalf("restore orphan: expected 200, got %d", resp.StatusCode)
	}
	if status, _ := archiveState(t, db, orphan); status != "unclaimed" {
		t.Fatalf("orphan restored to %q, want unclaimed", status)
	}
}

// The archived list shows admin-archived patches only. Rejected community
// submissions also carry status='archived' but with removed_at set — refuse,
// not archives — and restore refuses them too.
func TestAdminListNodes_ExcludesRejectedSubmissions(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, adminToken := createTestUser(t, db, "site-admin", "admin")

	archived := createTestNode(t, db, owner.ID, "Archived Patch", "archived-patch", "open")
	createTestMembership(t, db, owner.ID, archived, "admin", "active")
	rejected := createTestNode(t, db, owner.ID, "Spam Listing", "spam-listing", "open")
	if _, err := db.Exec("UPDATE nodes SET status = 'archived', archived_from = 'active' WHERE id = ?", archived); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := db.Exec("UPDATE nodes SET status = 'archived', archived_from = 'pending_review', removed_at = '2026-07-01T00:00:00Z' WHERE id = ?", rejected); err != nil {
		t.Fatalf("reject: %v", err)
	}

	r := authedRequest("GET", "/api/v1/admin/nodes?status=archived", nil, adminToken)
	w := serveAdminMux(t, db, "GET", "/api/v1/admin/nodes", handler.AdminListNodes(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, _ := result["nodes"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 archived node, got %d: %v", len(items), items)
	}
	row := items[0].(map[string]interface{})
	if row["id"] != archived || row["restores_to"] != "active" {
		t.Fatalf("unexpected row: %v", row)
	}

	// The list endpoint refuses anything but status=archived for now.
	r = authedRequest("GET", "/api/v1/admin/nodes", nil, adminToken)
	w = serveAdminMux(t, db, "GET", "/api/v1/admin/nodes", handler.AdminListNodes(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no-filter list: expected 400, got %d", w.Code)
	}

	// A rejected submission is not restorable — it reads as not found.
	if resp := restoreNode(t, db, rejected, adminToken); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore rejected submission: expected 404, got %d", resp.StatusCode)
	}
}
