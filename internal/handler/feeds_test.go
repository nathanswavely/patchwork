package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func feedTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Instance.Domain = "quilt.test"
	cfg.Instance.Name = "Test Quilt"
	return cfg
}

func TestNodeICSFeed_PublicEventsOnly(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "icsadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Feed Venue", "feed-venue", "open")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	seedEvent(t, db, nodeID, admin.ID, "Public Show", future)
	// A private event and a pending submission must never leave.
	privateID := seedEvent(t, db, nodeID, admin.ID, "Private Show", future)
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, privateID); err != nil {
		t.Fatal(err)
	}
	pendingID := seedEvent(t, db, nodeID, admin.ID, "Pending Show", future)
	if _, err := db.Exec(`UPDATE events SET status = 'pending_review' WHERE id = ?`, pendingID); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("GET", "/api/v1/nodes/feed-venue/events.ics", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "Public Show") {
		t.Errorf("feed body missing content:\n%s", body)
	}
	if strings.Contains(body, "Private Show") || strings.Contains(body, "Pending Show") {
		t.Errorf("non-public content leaked into the feed:\n%s", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Errorf("content type: %s", ct)
	}

	// Conditional GET: same content, one 304.
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on feed response")
	}
	r = authedRequest("GET", "/api/v1/nodes/feed-venue/events.ics", nil, "")
	r.Header.Set("If-None-Match", etag)
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), r)
	if w.Code != http.StatusNotModified {
		t.Errorf("conditional GET: expected 304, got %d", w.Code)
	}
}

// A patch with no events must still serve a valid (empty) calendar —
// go-ical refuses componentless VCALENDARs, and the silent error path
// produced a zero-byte 200 that calendar apps treat as a broken feed.
func TestNodeICSFeed_EmptyCalendarIsValid(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "emptyadmin", "member")
	createTestNode(t, db, admin.ID, "Quiet Venue", "quiet-venue", "open")

	r := authedRequest("GET", "/api/v1/nodes/quiet-venue/events.ics", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "END:VCALENDAR") {
		t.Errorf("empty feed must still be a calendar, got %d bytes: %q", len(body), body)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("empty feed lost its ETag")
	}
}

func TestNodeICSFeed_PrivateNodeInvisible(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "privadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Hidden Venue", "hidden-venue", "open")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, nodeID); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("GET", "/api/v1/nodes/hidden-venue/events.ics", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("private node feed: expected 404, got %d", w.Code)
	}
}

func TestNodeRSSFeed(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "rssadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "RSS Venue", "rss-venue", "open")
	seedEvent(t, db, nodeID, admin.ID, "RSS Show", time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))

	r := authedRequest("GET", "/api/v1/nodes/rss-venue/events.rss", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.rss", handler.NodeRSSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<rss") || !strings.Contains(body, "RSS Show") ||
		!strings.Contains(body, "https://quilt.test/patches/rss-venue/events/") {
		t.Errorf("rss body:\n%s", body)
	}
}

// Archiving sets nodes.status='archived' but leaves removed_at NULL and
// memberships/events active; the personal feed must gate on node status
// like the public feeds do, or archived patches keep haunting calendars.
func TestPersonalFeed_ArchivedPatchExcluded(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "archadmin", "member")
	person, personToken := createTestUser(t, db, "archperson", "member")
	nodeID := createTestNode(t, db, admin.ID, "Archived Band", "archived-band", "open")
	createTestMembership(t, db, person.ID, nodeID, "member", "active")
	seedEvent(t, db, nodeID, admin.ID, "Ghost Show", time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))

	if _, err := db.Exec(`UPDATE nodes SET status = 'archived' WHERE id = ?`, nodeID); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("POST", "/api/v1/users/me/feed-secret", nil, personToken)
	w := serveMux(t, db, "POST", "/api/v1/users/me/feed-secret", handler.GenerateFeedSecret(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("generate: %d", w.Code)
	}
	feedURL, _ := decodeJSON(t, w)["url"].(string)
	secret := strings.TrimSuffix(strings.TrimPrefix(feedURL, "https://quilt.test/api/v1/feeds/"), "/events.ics")

	r = authedRequest("GET", "/api/v1/feeds/"+secret+"/events.ics", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/feeds/{secret}/events.ics", handler.PersonalICSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("personal feed: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Ghost Show") {
		t.Error("archived patch's event leaked into the personal feed")
	}
}

func TestPersonalFeed_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "pfadmin", "member")
	person, personToken := createTestUser(t, db, "pfperson", "member")

	// Person is a member of one patch (sees its private events) and a
	// follower of another (public events only).
	memberNode := createTestNode(t, db, admin.ID, "My Band", "my-band", "open")
	createTestMembership(t, db, person.ID, memberNode, "member", "active")
	followedNode := createTestNode(t, db, admin.ID, "Followed Venue", "followed-venue", "open")
	createTestMembership(t, db, person.ID, followedNode, "follower", "active")
	strangerNode := createTestNode(t, db, admin.ID, "Stranger Patch", "stranger-patch", "open")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	seedEvent(t, db, memberNode, admin.ID, "Band Practice Public", future)
	privateID := seedEvent(t, db, memberNode, admin.ID, "Band Practice Private", future)
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, privateID); err != nil {
		t.Fatal(err)
	}
	seedEvent(t, db, followedNode, admin.ID, "Venue Show", future)
	followedPrivateID := seedEvent(t, db, followedNode, admin.ID, "Venue Members Meeting", future)
	if _, err := db.Exec(`UPDATE events SET visibility = 'private' WHERE id = ?`, followedPrivateID); err != nil {
		t.Fatal(err)
	}
	seedEvent(t, db, strangerNode, admin.ID, "Stranger Event", future)

	// No secret yet.
	r := authedRequest("GET", "/api/v1/users/me/feed-secret", nil, personToken)
	w := serveMux(t, db, "GET", "/api/v1/users/me/feed-secret", handler.FeedSecretStatus(db), r)
	if w.Code != http.StatusOK || decodeJSON(t, w)["enabled"] != false {
		t.Fatalf("initial status: %d %s", w.Code, w.Body.String())
	}

	// Generate: URL comes back once.
	r = authedRequest("POST", "/api/v1/users/me/feed-secret", nil, personToken)
	w = serveMux(t, db, "POST", "/api/v1/users/me/feed-secret", handler.GenerateFeedSecret(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("generate: %d %s", w.Code, w.Body.String())
	}
	feedURL, _ := decodeJSON(t, w)["url"].(string)
	prefix := "https://quilt.test/api/v1/feeds/"
	if !strings.HasPrefix(feedURL, prefix) || !strings.HasSuffix(feedURL, "/events.ics") {
		t.Fatalf("feed url shape: %s", feedURL)
	}
	secret := strings.TrimSuffix(strings.TrimPrefix(feedURL, prefix), "/events.ics")

	// The feed sees membership-appropriate events and nothing else.
	r = authedRequest("GET", "/api/v1/feeds/"+secret+"/events.ics", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/feeds/{secret}/events.ics", handler.PersonalICSFeed(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("personal feed: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"Band Practice Public", "Band Practice Private", "Venue Show"} {
		if !strings.Contains(body, want) {
			t.Errorf("personal feed missing %q", want)
		}
	}
	for _, reject := range []string{"Venue Members Meeting", "Stranger Event"} {
		if strings.Contains(body, reject) {
			t.Errorf("personal feed leaked %q", reject)
		}
	}

	// A wrong secret is a 404, not an empty calendar.
	r = authedRequest("GET", "/api/v1/feeds/"+strings.Repeat("0", 64)+"/events.ics", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/feeds/{secret}/events.ics", handler.PersonalICSFeed(db, cfg), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("wrong secret: expected 404, got %d", w.Code)
	}

	// Regenerating revokes the old URL.
	r = authedRequest("POST", "/api/v1/users/me/feed-secret", nil, personToken)
	w = serveMux(t, db, "POST", "/api/v1/users/me/feed-secret", handler.GenerateFeedSecret(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("regenerate: %d", w.Code)
	}
	r = authedRequest("GET", "/api/v1/feeds/"+secret+"/events.ics", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/feeds/{secret}/events.ics", handler.PersonalICSFeed(db, cfg), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("old secret after regenerate: expected 404, got %d", w.Code)
	}

	// Disable ends the feed entirely.
	r = authedRequest("DELETE", "/api/v1/users/me/feed-secret", nil, personToken)
	w = serveMux(t, db, "DELETE", "/api/v1/users/me/feed-secret", handler.DeleteFeedSecret(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("disable: %d", w.Code)
	}
	r = authedRequest("GET", "/api/v1/users/me/feed-secret", nil, personToken)
	w = serveMux(t, db, "GET", "/api/v1/users/me/feed-secret", handler.FeedSecretStatus(db), r)
	if decodeJSON(t, w)["enabled"] != false {
		t.Error("feed still enabled after delete")
	}
}
