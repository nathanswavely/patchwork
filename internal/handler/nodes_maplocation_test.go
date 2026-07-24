package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Issue #4: a patch admin sets a map location by placing a marker. The update
// path must accept latitude/longitude, store them, and reject out-of-range or
// non-admin writes. An explicit null clears the position (off the map).

func TestUpdateNodeSetsMapLocation(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "loc-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Place Me", "place-me", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/place-me", map[string]interface{}{
		"latitude":  40.0379,
		"longitude": -76.3055,
	}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var lat, lng float64
	if err := db.QueryRow(`SELECT latitude, longitude FROM nodes WHERE id = ?`, nodeID).Scan(&lat, &lng); err != nil {
		t.Fatalf("read back coords: %v", err)
	}
	if lat != 40.0379 || lng != -76.3055 {
		t.Errorf("stored coords = (%v, %v), want (40.0379, -76.3055)", lat, lng)
	}
}

func TestUpdateNodeClearsMapLocation(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "loc-clear-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Clear Loc", "clear-loc", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	if _, err := db.Exec(`UPDATE nodes SET latitude = 40.0, longitude = -76.0 WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("seed coords: %v", err)
	}

	// Explicit nulls clear the position — the patch leaves the map.
	r := authedRequest("PATCH", "/api/v1/nodes/clear-loc", map[string]interface{}{
		"latitude":  nil,
		"longitude": nil,
	}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var lat, lng *float64
	if err := db.QueryRow(`SELECT latitude, longitude FROM nodes WHERE id = ?`, nodeID).Scan(&lat, &lng); err != nil {
		t.Fatalf("read back coords: %v", err)
	}
	if lat != nil || lng != nil {
		t.Errorf("expected coords cleared to NULL, got (%v, %v)", lat, lng)
	}
}

func TestUpdateNodeRejectsOutOfRangeCoords(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "loc-range-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Range Me", "range-me", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"latitude too high", map[string]interface{}{"latitude": 91.0, "longitude": 0.0}},
		{"latitude too low", map[string]interface{}{"latitude": -90.5, "longitude": 0.0}},
		{"longitude too high", map[string]interface{}{"latitude": 0.0, "longitude": 181.0}},
		{"longitude too low", map[string]interface{}{"latitude": 0.0, "longitude": -180.1}},
		{"latitude not a number", map[string]interface{}{"latitude": "north"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := authedRequest("PATCH", "/api/v1/nodes/range-me", tc.body, token)
			w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	// None of the rejected requests may have written anything.
	var lat, lng *float64
	if err := db.QueryRow(`SELECT latitude, longitude FROM nodes WHERE id = ?`, nodeID).Scan(&lat, &lng); err != nil {
		t.Fatalf("read back coords: %v", err)
	}
	if lat != nil || lng != nil {
		t.Errorf("out-of-range writes must not persist, got (%v, %v)", lat, lng)
	}
}

func TestUpdateNodeMapLocationRejectsNonAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "loc-owner", "member")
	nodeID := createTestNode(t, db, admin.ID, "Guarded", "guarded", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// A plain member of the same patch may not place its marker.
	member, memberToken := createTestUser(t, db, "loc-member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/guarded", map[string]interface{}{
		"latitude":  40.0,
		"longitude": -76.0,
	}, memberToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d: %s", w.Code, w.Body.String())
	}

	var lat, lng *float64
	if err := db.QueryRow(`SELECT latitude, longitude FROM nodes WHERE id = ?`, nodeID).Scan(&lat, &lng); err != nil {
		t.Fatalf("read back coords: %v", err)
	}
	if lat != nil || lng != nil {
		t.Errorf("non-admin write must not persist, got (%v, %v)", lat, lng)
	}
}
