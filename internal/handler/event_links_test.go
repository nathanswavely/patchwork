package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

func linksCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Instance.Domain = "quilt.test"
	return cfg
}

// insertActiveEvent creates a published public event directly.
func insertActiveEvent(t *testing.T, db *database.DB, nodeID, createdBy, title string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility, status)
		 VALUES (?, ?, ?, ?, '2027-06-01T20:00:00Z', 'public', 'active')`,
		id, nodeID, createdBy, title,
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	return id
}

func requestLink(t *testing.T, db *database.DB, token, eventID, target, absorb string) (*httptest.ResponseRecorder, model.EventLink) {
	t.Helper()
	body := map[string]interface{}{"target": target}
	if absorb != "" {
		body["absorb_event_id"] = absorb
	}
	r := authedRequest("POST", "/api/v1/events/"+eventID+"/links", body, token)
	w := serveMux(t, db, "POST", "/api/v1/events/{id}/links", handler.CreateEventLink(db, linksCfg()), r)
	var l model.EventLink
	json.Unmarshal(w.Body.Bytes(), &l)
	return w, l
}

func confirmLink(t *testing.T, db *database.DB, token, eventID, nodeID, absorb string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]interface{}{}
	if absorb != "" {
		body["absorb_event_id"] = absorb
	}
	r := authedRequest("POST", "/api/v1/events/"+eventID+"/links/"+nodeID+"/confirm", body, token)
	return serveMux(t, db, "POST", "/api/v1/events/{id}/links/{nodeId}/confirm", handler.ConfirmEventLink(db), r)
}

func linkStatusInDB(t *testing.T, db *database.DB, eventID, nodeID string) string {
	t.Helper()
	var s string
	db.QueryRow(`SELECT status FROM event_links WHERE event_id = ? AND node_id = ?`, eventID, nodeID).Scan(&s)
	return s
}

// getEventPublic fetches an event through GetEvent with optional auth.
func getEventPublic(t *testing.T, db *database.DB, eventID, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/events/"+eventID, nil, token)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/events/{id}", middleware.AuthOptional(db, handler.GetEvent(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	var out map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

// The core handshake: the linked side requests, the owner side confirms,
// and only then does the event surface on the linked patch (docs/adr/032).
func TestEventLinkHandshake(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Friday Gig")

	// Band admin requests onto the venue's event → pending.
	w, l := requestLink(t, db, bandToken, eventID, "cool-band", "")
	if w.Code != 201 || l.Status != "pending" || l.InitiatedBy != "linked" {
		t.Fatalf("request: code=%d status=%q initiated_by=%q, want 201 pending linked", w.Code, l.Status, l.InitiatedBy)
	}

	// Pending links are invisible to the public.
	pub := getEventPublic(t, db, eventID, "")
	if links, _ := pub["links"].([]interface{}); len(links) != 0 {
		t.Fatalf("public view shows pending link: %v", links)
	}
	// But the acting admins see them.
	own := getEventPublic(t, db, eventID, venueToken)
	if links, _ := own["links"].([]interface{}); len(links) != 1 {
		t.Fatalf("owner admin can't see pending link: %v", own["links"])
	}

	// The requesting side can't confirm its own request.
	if w := confirmLink(t, db, bandToken, eventID, bandID, ""); w.Code != 403 {
		t.Fatalf("self-confirm: code=%d, want 403", w.Code)
	}

	// The owner side confirms.
	if w := confirmLink(t, db, venueToken, eventID, bandID, ""); w.Code != 200 {
		t.Fatalf("confirm: code=%d body=%s", w.Code, w.Body.String())
	}
	if s := linkStatusInDB(t, db, eventID, bandID); s != "confirmed" {
		t.Fatalf("link status=%q, want confirmed", s)
	}

	// The event now lists on the band's calendar.
	r := authedRequest("GET", "/api/v1/events?node_id="+bandID, nil, "")
	lw := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if !strings.Contains(lw.Body.String(), "Friday Gig") {
		t.Fatalf("linked event missing from band listing: %s", lw.Body.String())
	}

	// Either side severs unilaterally — the band walks away.
	dr := authedRequest("DELETE", "/api/v1/events/"+eventID+"/links/"+bandID, nil, bandToken)
	dw := serveMux(t, db, "DELETE", "/api/v1/events/{id}/links/{nodeId}", handler.RemoveEventLink(db), dr)
	if dw.Code != 200 {
		t.Fatalf("remove: code=%d", dw.Code)
	}
	r2 := authedRequest("GET", "/api/v1/events?node_id="+bandID, nil, "")
	lw2 := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r2)
	if strings.Contains(lw2.Body.String(), "Friday Gig") {
		t.Fatalf("removed link still lists on band calendar")
	}
}

// One person adminning both sides completes the handshake instantly.
func TestEventLinkInstantConfirm(t *testing.T) {
	db := setupTestDB(t)
	booker, token := createTestUser(t, db, "booker", "member")
	venueID := createTestNode(t, db, booker.ID, "Warehouse", "warehouse", "open")
	createTestMembership(t, db, booker.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, booker.ID, "House Band", "house-band", "invite_only")
	createTestMembership(t, db, booker.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, booker.ID, "Residency Night")
	w, l := requestLink(t, db, token, eventID, "house-band", "")
	if w.Code != 201 || l.Status != "confirmed" {
		t.Fatalf("both-sides admin: code=%d status=%q, want 201 confirmed", w.Code, l.Status)
	}
}

// Speaking for a patch is admin territory: members and strangers can't
// enter the handshake from either side.
func TestEventLinkForbidden(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, _ := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, _ := createTestUser(t, db, "band-admin", "member")
	member, memberToken := createTestUser(t, db, "just-member", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")
	createTestMembership(t, db, member.ID, bandID, "member", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Friday Gig")
	if w, _ := requestLink(t, db, memberToken, eventID, "cool-band", ""); w.Code != 403 {
		t.Fatalf("band member request: code=%d, want 403", w.Code)
	}
}

// Owner-initiated: the venue tags the band, the band's admin confirms
// and may absorb a duplicate of their own at confirm time.
func TestEventLinkOwnerInitiatedAbsorb(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Friday Gig")

	// The band's own imported duplicate of the same gig.
	sourceID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', 'https://tour.example/cal.ics', ?)`,
		sourceID, bandID, bandAdmin.ID,
	); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	dupID := insertActiveEvent(t, db, bandID, bandAdmin.ID, "Friday Gig (tour cal)")
	db.Exec(`UPDATE events SET source_id = ?, source_uid = 'gig-1', source_occurrence = '' WHERE id = ?`, sourceID, dupID)

	// Venue proposes the link.
	w, l := requestLink(t, db, venueToken, eventID, "cool-band", "")
	if w.Code != 201 || l.Status != "pending" || l.InitiatedBy != "owner" {
		t.Fatalf("owner propose: code=%d status=%q initiated_by=%q", w.Code, l.Status, l.InitiatedBy)
	}

	// The owner side can't attach an absorb target — that's the linked
	// patch's call.
	if w := confirmLink(t, db, venueToken, eventID, bandID, dupID); w.Code != 403 {
		t.Fatalf("owner-side confirm: code=%d, want 403", w.Code)
	}

	// Band admin confirms, absorbing their duplicate.
	if w := confirmLink(t, db, bandToken, eventID, bandID, dupID); w.Code != 200 {
		t.Fatalf("confirm+absorb: code=%d body=%s", w.Code, w.Body.String())
	}

	// The duplicate is gone and skip-listed so the sync can't resurrect it.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, dupID).Scan(&n)
	if n != 0 {
		t.Fatalf("duplicate survived absorb")
	}
	db.QueryRow(`SELECT COUNT(*) FROM event_source_skips WHERE source_id = ? AND uid = 'gig-1'`, sourceID).Scan(&n)
	if n != 1 {
		t.Fatalf("absorbed import not skip-listed")
	}
}

// A pasted remote patch URL becomes a display-only cross-quilt mention;
// a local patch URL routes into the real handshake (docs/adr/032).
func TestEventLinkMentionVsLocalURL(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Friday Gig")

	// Remote URL → mention, owner-side only.
	w, _ := requestLink(t, db, venueToken, eventID, "https://arts.elsewhere.example/patches/touring-band", "")
	if w.Code != 201 {
		t.Fatalf("mention: code=%d body=%s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM event_mentions WHERE event_id = ? AND host = 'arts.elsewhere.example' AND slug = 'touring-band'`, eventID).Scan(&n)
	if n != 1 {
		t.Fatalf("mention row missing")
	}
	// A non-owner admin can't decorate someone else's event page.
	if w, _ := requestLink(t, db, bandToken, eventID, "https://other.example/patches/whoever", ""); w.Code != 403 {
		t.Fatalf("non-owner mention: code=%d, want 403", w.Code)
	}

	// Local URL → the real handshake, never a consent-free mention.
	w2, l := requestLink(t, db, venueToken, eventID, "https://quilt.test/patches/cool-band", "")
	if w2.Code != 201 || l.Status != "pending" {
		t.Fatalf("local URL: code=%d status=%q, want 201 pending", w2.Code, l.Status)
	}
	db.QueryRow(`SELECT COUNT(*) FROM event_mentions WHERE event_id = ?`, eventID).Scan(&n)
	if n != 1 {
		t.Fatalf("local URL minted a mention")
	}
}

// When the confirming side is an unclaimed patch, the instance admin
// holds that calendar in trust (docs/adr/031) — nobody else can confirm.
func TestEventLinkUnclaimedOwnerSide(t *testing.T) {
	db := setupTestDB(t)
	siteAdmin, siteToken := createTestUser(t, db, "site-admin", "admin")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, siteAdmin.ID, "Old Mill", "old-mill", "open")
	makeUnclaimed(t, db, venueID)
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, siteAdmin.ID, "Mill Show")

	w, l := requestLink(t, db, bandToken, eventID, "cool-band", "")
	if w.Code != 201 || l.Status != "pending" {
		t.Fatalf("request onto unclaimed: code=%d status=%q", w.Code, l.Status)
	}
	if w := confirmLink(t, db, bandToken, eventID, bandID, ""); w.Code != 403 {
		t.Fatalf("band self-confirm on unclaimed owner: code=%d, want 403", w.Code)
	}
	if w := confirmLink(t, db, siteToken, eventID, bandID, ""); w.Code != 200 {
		t.Fatalf("site admin confirm: code=%d", w.Code)
	}
}

// Only active events are linkable — a pending submission can't
// accumulate links while it might still be rejected.
func TestEventLinkOnlyActiveEvents(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, _ := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")
	_ = bandID

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Maybe Show")
	db.Exec(`UPDATE events SET status = 'pending_review' WHERE id = ?`, eventID)

	if w, _ := requestLink(t, db, bandToken, eventID, "cool-band", ""); w.Code != 409 {
		t.Fatalf("link pending event: code=%d, want 409", w.Code)
	}
}

// The patch ICS feed carries confirmed links and nothing pending; the
// owner deleting the event takes the link with it (CASCADE).
func TestEventLinkFeedAndCascade(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Cool Band", "cool-band", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Friday Gig")
	requestLink(t, db, bandToken, eventID, "cool-band", "")

	feed := func() string {
		r := authedRequest("GET", "/api/v1/nodes/cool-band/events.ics", nil, "")
		w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, linksCfg()), r)
		return w.Body.String()
	}
	if strings.Contains(feed(), "Friday Gig") {
		t.Fatalf("pending link leaked into band ICS feed")
	}
	confirmLink(t, db, venueToken, eventID, bandID, "")
	if !strings.Contains(feed(), "Friday Gig") {
		t.Fatalf("confirmed link missing from band ICS feed")
	}

	// Owner deletes the event: the link rides the event row.
	db.Exec(`DELETE FROM events WHERE id = ?`, eventID)
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM event_links WHERE event_id = ?`, eventID).Scan(&n)
	if n != 0 {
		t.Fatalf("link outlived its event")
	}
	if strings.Contains(feed(), "Friday Gig") {
		t.Fatalf("deleted event still in band feed")
	}
}
