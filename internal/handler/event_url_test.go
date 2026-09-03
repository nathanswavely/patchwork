package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The event's own page out on the web (docs/adr/079): stored on create,
// returned on read, and refused when it isn't a link a browser can open.
func TestCreateEventWithEventURL(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	admin, adminToken := createTestUser(t, db, "linkadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Link Venue", "link-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := eventBody(nodeID, "Ticketed Show")
	body["event_url"] = "  https://elcapitan.example/shows/march-14  "
	e, code := createEventVia(t, db, cfg, adminToken, body)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	// Trimmed on the way in, and echoed by the create response — the form
	// reads its own result back.
	if e.EventURL != "https://elcapitan.example/shows/march-14" {
		t.Fatalf("event_url on create response: %q", e.EventURL)
	}

	var stored string
	db.QueryRow(`SELECT event_url FROM events WHERE id = ?`, e.ID).Scan(&stored)
	if stored != "https://elcapitan.example/shows/march-14" {
		t.Fatalf("event_url in db: %q", stored)
	}

	// And it comes back on the detail read, which is the surface that
	// renders it.
	r := authedRequest("GET", "/api/v1/events/"+e.ID, nil, "")
	w := servePublicMux(t, "GET", "/api/v1/events/{id}", handler.GetEvent(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("get event: %d", w.Code)
	}
	var got model.Event
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.EventURL != "https://elcapitan.example/shows/march-14" {
		t.Errorf("event_url on GET: %q", got.EventURL)
	}
}

// http is allowed on purpose and the exotic schemes are not: this value is
// rendered as an href, and half the small venues in a scene are still on
// plain http (docs/adr/079).
func TestCreateEventURLSchemes(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	admin, adminToken := createTestUser(t, db, "schemeadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Scheme Venue", "scheme-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"https", "https://venue.example/show", http.StatusCreated},
		{"plain http", "http://venue.example/show", http.StatusCreated},
		{"empty is fine", "", http.StatusCreated},
		{"javascript", "javascript:alert(1)", http.StatusBadRequest},
		{"data", "data:text/html,<script>alert(1)</script>", http.StatusBadRequest},
		{"bare words", "ask at the door", http.StatusBadRequest},
		{"scheme only", "https://", http.StatusBadRequest},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := eventBody(nodeID, "Show "+tc.name)
			body["starts_at"] = daysOut(10 + i)
			body["event_url"] = tc.url
			_, code := createEventVia(t, db, cfg, adminToken, body)
			if code != tc.want {
				t.Fatalf("%q: expected %d, got %d", tc.url, tc.want, code)
			}
		})
	}
}

// The link is judged alone — unlike the image pair, it has no second half
// to be valid against — and sending "" clears it.
func TestUpdateEventURL(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "patchadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Patch Venue", "patch-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	eventID := seedEvent(t, db, nodeID, admin.ID, "Editable Show", daysOut(5))

	patch := func(v interface{}) int {
		r := authedRequest("PATCH", "/api/v1/events/"+eventID, map[string]interface{}{"event_url": v}, adminToken)
		return serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r).Code
	}
	stored := func() string {
		var s string
		db.QueryRow(`SELECT event_url FROM events WHERE id = ?`, eventID).Scan(&s)
		return s
	}

	if code := patch("https://venue.example/tickets"); code != http.StatusOK {
		t.Fatalf("set link: %d", code)
	}
	if stored() != "https://venue.example/tickets" {
		t.Fatalf("after set: %q", stored())
	}

	if code := patch("javascript:alert(1)"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for javascript:, got %d", code)
	}
	if stored() != "https://venue.example/tickets" {
		t.Fatalf("a refused edit changed the stored value: %q", stored())
	}

	if code := patch("  "); code != http.StatusOK {
		t.Fatalf("clear link: %d", code)
	}
	if stored() != "" {
		t.Fatalf("expected cleared, got %q", stored())
	}
}

// ICS spends its one URL property on the Patchwork permalink, so the
// event's own page rides in DESCRIPTION where every client renders it
// (docs/adr/079). RSS does the same, for the same reason.
func TestFeedsCarryEventURL(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "feedlinkadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Linked Venue", "linked-venue", "open")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	withLink := seedEvent(t, db, nodeID, admin.ID, "Linked Show", future)
	if _, err := db.Exec(
		`UPDATE events SET event_url = ?, description = 'Doors at 7' WHERE id = ?`,
		"https://elcapitan.example/shows/linked", withLink,
	); err != nil {
		t.Fatal(err)
	}
	// A second event with no link must not grow a stray separator.
	seedEvent(t, db, nodeID, admin.ID, "Plain Show", future)

	r := authedRequest("GET", "/api/v1/nodes/linked-venue/events.ics", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("ics: %d", w.Code)
	}
	// ICS folds long lines; unfold before looking for a URL.
	ics := strings.ReplaceAll(w.Body.String(), "\r\n ", "")
	if !strings.Contains(ics, "elcapitan.example/shows/linked") {
		t.Errorf("ICS description is missing the event's own page:\n%s", ics)
	}
	if !strings.Contains(ics, "https://quilt.test/events/") {
		t.Errorf("ICS URL property is no longer the Patchwork permalink:\n%s", ics)
	}

	r = authedRequest("GET", "/api/v1/nodes/linked-venue/events.rss", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.rss", handler.NodeRSSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("rss: %d", w.Code)
	}
	rss := w.Body.String()
	if !strings.Contains(rss, "elcapitan.example/shows/linked") {
		t.Errorf("RSS description is missing the event's own page:\n%s", rss)
	}
	if !strings.Contains(rss, "quilt.test/events/") {
		t.Errorf("RSS link is no longer the Patchwork permalink:\n%s", rss)
	}
}

// Bulk upload is the spreadsheet door and season sheets have a link
// column; a bad one fails its row rather than being dropped silently.
func TestBulkUploadCarriesEventURL(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "bulklinkadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Bulk Venue", "bulk-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	post := func(rows []map[string]interface{}) int {
		r := authedRequest("POST", "/api/v1/nodes/bulk-venue/events/bulk",
			map[string]interface{}{"events": rows}, adminToken)
		return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk",
			handler.BulkCreateEvents(db), r).Code
	}

	if code := post([]map[string]interface{}{{
		"title":     "Opening Night",
		"starts_at": daysOut(20),
		"event_url": "https://venue.example/opening",
	}}); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("bulk upload: %d", code)
	}
	var stored string
	db.QueryRow(`SELECT event_url FROM events WHERE title = 'Opening Night'`).Scan(&stored)
	if stored != "https://venue.example/opening" {
		t.Fatalf("bulk event_url: %q", stored)
	}

	if code := post([]map[string]interface{}{{
		"title":     "Bad Link Night",
		"starts_at": daysOut(21),
		"event_url": "javascript:alert(1)",
	}}); code != http.StatusBadRequest {
		t.Fatalf("expected the row to fail validation, got %d", code)
	}
}
