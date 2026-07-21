package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Issue #46: the patch address must survive creation. The frontend used to
// post "location", which the request struct silently dropped — no error, the
// data just vanished.
func TestCreateNodePersistsAddress(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "addr-creator", "member")

	body := map[string]interface{}{
		"name":    "Address Patch",
		"address": "123 N Queen St, Lancaster, PA",
	}
	r := authedRequest("POST", "/api/v1/nodes", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created struct {
		ID      string `json:"id"`
		Slug    string `json:"slug"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Address != "123 N Queen St, Lancaster, PA" {
		t.Errorf("create response address = %q, want the posted address", created.Address)
	}

	var stored string
	if err := db.QueryRow(`SELECT address FROM nodes WHERE id = ?`, created.ID).Scan(&stored); err != nil {
		t.Fatalf("read back address: %v", err)
	}
	if stored != "123 N Queen St, Lancaster, PA" {
		t.Errorf("stored address = %q, want the posted address", stored)
	}
}

// "location" is the events table's column, never a patch field. Posting it to
// a patch must not quietly write anything.
func TestCreateNodeIgnoresLocationKey(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "addr-legacy", "member")

	body := map[string]interface{}{
		"name":     "Legacy Key Patch",
		"location": "should not be stored",
	}
	r := authedRequest("POST", "/api/v1/nodes", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	}
	json.NewDecoder(w.Body).Decode(&created)
	if created.Address != "" {
		t.Errorf(`"location" must not populate address, got %q`, created.Address)
	}
}

// The update path must accept "address" and return it on the patch record, so
// the settings field both saves and renders (it did neither before).
func TestUpdateNodeAddressRoundTrips(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "addr-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Update Me", "update-me", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/update-me", map[string]interface{}{
		"address": "44 W King St, Lancaster, PA",
	}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stored string
	if err := db.QueryRow(`SELECT address FROM nodes WHERE id = ?`, nodeID).Scan(&stored); err != nil {
		t.Fatalf("read back address: %v", err)
	}
	if stored != "44 W King St, Lancaster, PA" {
		t.Errorf("stored address = %q, want the patched address", stored)
	}

	// And it must come back out on a read, or the settings field renders blank.
	gr := authedRequest("GET", "/api/v1/nodes/update-me", nil, token)
	gw := serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), gr)
	if gw.Code != http.StatusOK {
		t.Fatalf("expected 200 on read, got %d: %s", gw.Code, gw.Body.String())
	}
	var got struct {
		Node struct {
			Address string `json:"address"`
		} `json:"node"`
	}
	if err := json.NewDecoder(gw.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got.Node.Address != "44 W King St, Lancaster, PA" {
		t.Errorf("GET address = %q, want the patched address", got.Node.Address)
	}
}

// A PATCH carrying only "location" has no valid fields — it must 400 rather
// than appear to succeed. This is the error the broken settings field hit.
func TestUpdateNodeRejectsLocationKey(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "addr-reject", "member")
	nodeID := createTestNode(t, db, admin.ID, "Reject Me", "reject-me", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/reject-me", map[string]interface{}{
		"location": "nope",
	}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a location-only patch, got %d: %s", w.Code, w.Body.String())
	}
	if stored := ""; db.QueryRow(`SELECT address FROM nodes WHERE id = ?`, nodeID).Scan(&stored) == nil && stored != "" {
		t.Errorf(`"location" must not have written address, got %q`, stored)
	}
}

// Clearing the address is a legitimate edit — an empty string must be stored,
// not treated as "no valid fields".
func TestUpdateNodeAllowsClearingAddress(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "addr-clear", "member")
	nodeID := createTestNode(t, db, admin.ID, "Clear Me", "clear-me", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	if _, err := db.Exec(`UPDATE nodes SET address = 'somewhere' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("seed address: %v", err)
	}

	r := authedRequest("PATCH", "/api/v1/nodes/clear-me", map[string]interface{}{"address": ""}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stored string
	if err := db.QueryRow(`SELECT address FROM nodes WHERE id = ?`, nodeID).Scan(&stored); err != nil {
		t.Fatalf("read back address: %v", err)
	}
	if stored != "" {
		t.Errorf("expected address cleared, got %q", stored)
	}
}
