package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func bulkBody(events ...map[string]any) map[string]any {
	return map[string]any{"events": events}
}

func futureISO(h int) string {
	return time.Now().Add(time.Duration(h) * time.Hour).UTC().Format(time.RFC3339)
}

func TestBulkUpload_AdminCreatesAndReuploadSkips(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "bulkadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Season Venue", "season-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := bulkBody(
		map[string]any{"title": "Opening Night", "starts_at": futureISO(24), "location": "Main Stage"},
		map[string]any{"title": "Matinee", "starts_at": futureISO(48), "ends_at": futureISO(51)},
	)
	r := authedRequest("POST", "/api/v1/nodes/season-venue/events/bulk", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	res := decodeJSON(t, w)
	if res["created"].(float64) != 2 || res["skipped"].(float64) != 0 {
		t.Errorf("first upload: %v", res)
	}

	// Re-upload the corrected sheet: one old row, one new — idempotent.
	body = bulkBody(
		map[string]any{"title": "Opening Night", "starts_at": body["events"].([]map[string]any)[0]["starts_at"]},
		map[string]any{"title": "Closing Night", "starts_at": futureISO(72)},
	)
	r = authedRequest("POST", "/api/v1/nodes/season-venue/events/bulk", body, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	res = decodeJSON(t, w)
	if res["created"].(float64) != 1 || res["skipped"].(float64) != 1 {
		t.Errorf("re-upload: %v", res)
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ?`, nodeID).Scan(&n)
	if n != 3 {
		t.Errorf("expected 3 events after both uploads, got %d", n)
	}
}

// Bulk upload is an admin act — members who can post single events
// directly still cannot upload a sheet.
func TestBulkUpload_MemberForbidden(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "bulkowner", "member")
	member, memberToken := createTestUser(t, db, "bulkmember", "member")
	nodeID := createTestNode(t, db, admin.ID, "Member Venue", "member-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	body := bulkBody(map[string]any{"title": "Show", "starts_at": futureISO(24)})
	r := authedRequest("POST", "/api/v1/nodes/member-venue/events/bulk", body, memberToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("member bulk: expected 403, got %d", w.Code)
	}
}

// On unclaimed patches, trusted contributors share the door with the
// instance admin (docs/adr/026 grant extended to the sheet).
func TestBulkUpload_UnclaimedTrustedContributor(t *testing.T) {
	db := setupTestDB(t)
	instanceAdmin, _ := createTestUser(t, db, "bulkinst", "admin")
	trusted, trustedToken := createTestUser(t, db, "bulktrusty", "member")
	_, plainToken := createTestUser(t, db, "bulkplain", "member")
	if _, err := db.Exec(`UPDATE users SET trusted_contributor = 1 WHERE id = ?`, trusted.ID); err != nil {
		t.Fatal(err)
	}
	nodeID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status)
		 VALUES (?, ?, 'Unclaimed Hall', 'unclaimed-hall', '', 'leaf', 'public', 'open', 'unclaimed')`,
		nodeID, instanceAdmin.ID,
	); err != nil {
		t.Fatal(err)
	}

	body := bulkBody(map[string]any{"title": "Community Night", "starts_at": futureISO(24)})
	r := authedRequest("POST", "/api/v1/nodes/unclaimed-hall/events/bulk", body, trustedToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("trusted contributor on unclaimed: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	r = authedRequest("POST", "/api/v1/nodes/unclaimed-hall/events/bulk", body, plainToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusForbidden {
		t.Errorf("plain user on unclaimed: expected 403, got %d", w.Code)
	}
}

// Validation is all-or-nothing with row-indexed errors: nothing is
// created until every row parses.
func TestBulkUpload_AllOrNothingValidation(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "bulkvalid", "member")
	nodeID := createTestNode(t, db, admin.ID, "Valid Venue", "valid-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := bulkBody(
		map[string]any{"title": "Good Row", "starts_at": futureISO(24)},
		map[string]any{"title": "", "starts_at": "not a date"},
	)
	r := authedRequest("POST", "/api/v1/nodes/valid-venue/events/bulk", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	res := decodeJSON(t, w)
	errs, ok := res["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected row errors, got %v", res)
	}
	if errs[0].(map[string]any)["index"].(float64) != 1 {
		t.Errorf("error should name row 1: %v", errs)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE node_id = ?`, nodeID).Scan(&n)
	if n != 0 {
		t.Errorf("invalid batch must create nothing, got %d rows", n)
	}
}

func TestBulkUpload_CapEnforced(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "bulkcap", "member")
	nodeID := createTestNode(t, db, admin.ID, "Cap Venue", "cap-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	events := make([]map[string]any, 201)
	for i := range events {
		events[i] = map[string]any{"title": "E", "starts_at": futureISO(24 + i)}
	}
	r := authedRequest("POST", "/api/v1/nodes/cap-venue/events/bulk", map[string]any{"events": events}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("201 rows: expected 400, got %d", w.Code)
	}
}
