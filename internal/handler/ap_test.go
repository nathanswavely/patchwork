package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func apRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Accept", "application/activity+json")
	return r
}

func createTestEvent(t *testing.T, db *database.DB, nodeID, createdBy, title string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility) VALUES (?, ?, ?, ?, '', '', '2025-06-01T12:00:00Z', '', 'public')`,
		id, nodeID, createdBy, title,
	)
	if err != nil {
		t.Fatalf("create event %s: %v", title, err)
	}
	return id
}

func TestAPNode_ReturnsOrganization(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "apowner1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Test Patch", "test-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	r := apRequest("GET", "/ap/nodes/"+nodeID)
	w := servePublicMux(t, "GET", "/ap/nodes/{id}", handler.APNode(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["type"] != "Organization" {
		t.Errorf("expected type=Organization, got %v", result["type"])
	}
	if id, ok := result["id"].(string); !ok || !strings.Contains(id, nodeID) {
		t.Errorf("expected id to contain node ID %s, got %v", nodeID, result["id"])
	}
	if result["name"] != "Test Patch" {
		t.Errorf("expected name=Test Patch, got %v", result["name"])
	}
	if result["preferredUsername"] != "test-patch" {
		t.Errorf("expected preferredUsername=test-patch, got %v", result["preferredUsername"])
	}
	for _, field := range []string{"inbox", "outbox", "followers"} {
		val, ok := result[field].(string)
		if !ok || val == "" {
			t.Errorf("expected non-empty %s URL, got %v", field, result[field])
		}
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/activity+json" {
		t.Errorf("expected Content-Type application/activity+json, got %s", ct)
	}
}

func TestAPNode_RedirectsWithoutAcceptHeader(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "apowner2", "member")
	nodeID := createTestNode(t, db, owner.ID, "Redirect Patch", "redirect-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	r := httptest.NewRequest("GET", "/ap/nodes/"+nodeID, nil)
	// No Accept: application/activity+json header set.
	w := servePublicMux(t, "GET", "/ap/nodes/{id}", handler.APNode(db), r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/patches/redirect-patch") {
		t.Errorf("expected redirect to /patches/redirect-patch, got %s", loc)
	}
}

func TestAPNode_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := apRequest("GET", "/ap/nodes/nonexistent-id-12345")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}", handler.APNode(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPUser_ReturnsPerson(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	user, _ := createTestUser(t, db, "apuser1", "member")

	r := apRequest("GET", "/ap/users/"+user.ID)
	w := servePublicMux(t, "GET", "/ap/users/{id}", handler.APUser(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["type"] != "Person" {
		t.Errorf("expected type=Person, got %v", result["type"])
	}
	if result["name"] != "apuser1" {
		t.Errorf("expected name=apuser1, got %v", result["name"])
	}
	if result["preferredUsername"] != "apuser1" {
		t.Errorf("expected preferredUsername=apuser1, got %v", result["preferredUsername"])
	}
	if id, ok := result["id"].(string); !ok || !strings.Contains(id, user.ID) {
		t.Errorf("expected id to contain user ID %s, got %v", user.ID, result["id"])
	}
	for _, field := range []string{"inbox", "outbox", "followers", "following"} {
		val, ok := result[field].(string)
		if !ok || val == "" {
			t.Errorf("expected non-empty %s URL, got %v", field, result[field])
		}
	}
}

func TestAPUser_ReturnsPerson_WithPublicKey(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	user, _ := createTestUser(t, db, "apuserkey", "member")
	// Set a public key on the user.
	_, err := db.Exec("UPDATE users SET public_key = ? WHERE id = ?", "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----", user.ID)
	if err != nil {
		t.Fatalf("set public key: %v", err)
	}

	r := apRequest("GET", "/ap/users/"+user.ID)
	w := servePublicMux(t, "GET", "/ap/users/{id}", handler.APUser(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	pk, ok := result["publicKey"].(map[string]interface{})
	if !ok {
		t.Fatal("expected publicKey object in response")
	}
	if pk["publicKeyPem"] == nil || pk["publicKeyPem"] == "" {
		t.Error("expected publicKeyPem in publicKey object")
	}
	if pk["owner"] == nil || pk["owner"] == "" {
		t.Error("expected owner in publicKey object")
	}
}

func TestAPEvent_ReturnsEvent(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "eventowner1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Event Patch", "event-patch", "open")
	eventID := createTestEvent(t, db, nodeID, owner.ID, "Summer Concert")

	r := apRequest("GET", "/ap/events/"+eventID)
	w := servePublicMux(t, "GET", "/ap/events/{id}", handler.APEvent(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["type"] != "Event" {
		t.Errorf("expected type=Event, got %v", result["type"])
	}
	if result["name"] != "Summer Concert" {
		t.Errorf("expected name=Summer Concert, got %v", result["name"])
	}
	if id, ok := result["id"].(string); !ok || !strings.Contains(id, eventID) {
		t.Errorf("expected id to contain event ID %s, got %v", eventID, result["id"])
	}
}

func TestAPEvent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := apRequest("GET", "/ap/events/nonexistent-event-id")
	w := servePublicMux(t, "GET", "/ap/events/{id}", handler.APEvent(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPNodeOutbox_ReturnsCollection(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "outboxowner1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Outbox Patch", "outbox-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestEvent(t, db, nodeID, owner.ID, "Outbox Event 1")
	createTestEvent(t, db, nodeID, owner.ID, "Outbox Event 2")

	r := apRequest("GET", "/ap/nodes/"+nodeID+"/outbox")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/outbox", handler.APNodeOutbox(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["type"] != "OrderedCollection" {
		t.Errorf("expected type=OrderedCollection, got %v", result["type"])
	}
	totalItems, ok := result["totalItems"].(float64)
	if !ok {
		t.Fatal("expected totalItems to be a number")
	}
	if int(totalItems) != 2 {
		t.Errorf("expected totalItems=2, got %v", totalItems)
	}
	items, ok := result["orderedItems"].([]interface{})
	if !ok {
		t.Fatal("expected orderedItems array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 ordered items, got %d", len(items))
	}
}

func TestAPNodeOutbox_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := apRequest("GET", "/ap/nodes/nonexistent-node/outbox")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/outbox", handler.APNodeOutbox(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPNodeFollowers_ReturnsCollection(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "followowner1", "member")
	follower, _ := createTestUser(t, db, "follower1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Follow Patch", "follow-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	r := apRequest("GET", "/ap/nodes/"+nodeID+"/followers")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/followers", handler.APNodeFollowers(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["type"] != "Collection" {
		t.Errorf("expected type=Collection, got %v", result["type"])
	}
	totalItems, ok := result["totalItems"].(float64)
	if !ok {
		t.Fatal("expected totalItems to be a number")
	}
	if int(totalItems) < 1 {
		t.Errorf("expected totalItems >= 1, got %v", totalItems)
	}
}

func TestAPNodeFollowers_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := apRequest("GET", "/ap/nodes/nonexistent-node/followers")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/followers", handler.APNodeFollowers(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPEvent_PrivatePatch404: federation is public-only (docs/adr/024) — an
// event hosted by a private patch has no addressable AP object.
func TestAPEvent_PrivatePatch404(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "apprivowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Private AP Patch", "private-ap-patch", "open")
	eventID := createTestEvent(t, db, nodeID, owner.ID, "Hidden Concert")
	if _, err := db.Exec("UPDATE nodes SET visibility = 'private' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("set private: %v", err)
	}

	r := apRequest("GET", "/ap/events/"+eventID)
	w := servePublicMux(t, "GET", "/ap/events/{id}", handler.APEvent(db), r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for private patch's event, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPNodeOutbox_PrivatePatch404: a private patch serves no public outbox.
func TestAPNodeOutbox_PrivatePatch404(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "apprivoutbox", "member")
	nodeID := createTestNode(t, db, owner.ID, "Private Outbox Patch", "private-outbox-patch", "open")
	createTestEvent(t, db, nodeID, owner.ID, "Hidden Event")
	if _, err := db.Exec("UPDATE nodes SET visibility = 'private' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("set private: %v", err)
	}

	r := apRequest("GET", "/ap/nodes/"+nodeID+"/outbox")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/outbox", handler.APNodeOutbox(db), r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for private patch's outbox, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAPNodeOutbox_OmitsNonPublicEvents: only public events federate — the
// outbox applies the same visibility rule broadcastEventCreate does.
func TestAPNodeOutbox_OmitsNonPublicEvents(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "apmixedoutbox", "member")
	nodeID := createTestNode(t, db, owner.ID, "Mixed Outbox Patch", "mixed-outbox-patch", "open")
	createTestEvent(t, db, nodeID, owner.ID, "Public Show")
	unlistedID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, recurrence, visibility)
		 VALUES (?, ?, ?, 'Members Only', '', '', '2025-06-01T12:00:00Z', '', 'unlisted')`,
		unlistedID, nodeID, owner.ID,
	); err != nil {
		t.Fatalf("insert unlisted event: %v", err)
	}

	r := apRequest("GET", "/ap/nodes/"+nodeID+"/outbox")
	w := servePublicMux(t, "GET", "/ap/nodes/{id}/outbox", handler.APNodeOutbox(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if total, _ := result["totalItems"].(float64); int(total) != 1 {
		t.Fatalf("expected only the public event in the outbox, got totalItems=%v", result["totalItems"])
	}
}
