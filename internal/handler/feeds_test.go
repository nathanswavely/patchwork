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
	// The URL a subscriber's calendar app opens has to be the event's real
	// address — /events/{id}, not the patch-scoped path the SPA never routed
	// (issue #56). Unfolded first, since ICS wraps long lines.
	unfolded := strings.ReplaceAll(body, "\r\n ", "")
	if !strings.Contains(unfolded, "https://quilt.test/events/") {
		t.Errorf("ICS URL property is not the canonical event URL:\n%s", body)
	}
	if strings.Contains(unfolded, "/patches/feed-venue/events/") {
		t.Errorf("ICS URL property still uses the unrouted patch-scoped path:\n%s", body)
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
	if !strings.Contains(body, "<rss") || !strings.Contains(body, "RSS Show") {
		t.Errorf("rss body:\n%s", body)
	}
	// Item links must be the event's real address. This used to assert
	// /patches/{slug}/events/{id}, a path the SPA never routed — feed readers
	// followed it to the home quilt (issue #56).
	if !strings.Contains(body, "https://quilt.test/events/") {
		t.Errorf("rss item link is not the canonical event URL:\n%s", body)
	}
	if strings.Contains(body, "/patches/rss-venue/events/") {
		t.Errorf("rss item link still uses the unrouted patch-scoped event path:\n%s", body)
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

// TestEventICS_OneEventForOneNight covers docs/adr/090 decision 5: the
// single-event download that replaces the notification Patchwork no longer
// sends. It must carry the same UID the patch feed gives the event, so a
// person who downloads tonight's show and later subscribes to the venue
// ends up with one entry rather than two.
func TestEventICS_OneEventForOneNight(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, _ := createTestUser(t, db, "icsoneadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "One Night Venue", "one-night-venue", "open")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	eventID := seedEvent(t, db, nodeID, admin.ID, "Basement Show", future)

	r := authedRequest("GET", "/api/v1/events/"+eventID+"/event.ics", nil, "")
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/events/{id}/event.ics", handler.EventICS(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("public event: code=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Errorf("content type: got %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="basement-show.ics"`) {
		t.Errorf("content disposition: got %q", cd)
	}
	if !strings.Contains(body, "SUMMARY:Basement Show") {
		t.Errorf("missing summary:\n%s", body)
	}
	// One event, not a calendar's worth.
	if n := strings.Count(body, "BEGIN:VEVENT"); n != 1 {
		t.Errorf("VEVENT count: got %d, want 1", n)
	}

	// The UID must match what the patch's own feed emits for the same
	// event, or a later subscription duplicates the entry.
	fr := authedRequest("GET", "/api/v1/nodes/one-night-venue/events.ics", nil, "")
	fw := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(db, cfg), fr)
	uid := "UID:" + eventID + "@quilt.test"
	if !strings.Contains(body, uid) || !strings.Contains(fw.Body.String(), uid) {
		t.Errorf("UID %q must appear in both the single event and the patch feed", uid)
	}
}

// A non-public event is a file only its patch's members can take away.
// The events table spells that 'private' or 'unlisted'; ListEvents admits
// either only for a member or admin of the event's own patch, and this
// endpoint follows it rather than GetEvent, which gates neither.
func TestEventICS_NonPublicEventNeedsTheRoom(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, adminToken := createTestUser(t, db, "icsroomadmin", "member")
	member, memberToken := createTestUser(t, db, "icsroommember", "member")
	follower, followerToken := createTestUser(t, db, "icsroomfollower", "member")
	_, outsiderToken := createTestUser(t, db, "icsroomoutsider", "member")
	nodeID := createTestNode(t, db, admin.ID, "Closed Room", "closed-room", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	eventID := seedEvent(t, db, nodeID, admin.ID, "House Meeting", future)

	for _, vis := range []string{"private", "unlisted"} {
		if _, err := db.Exec(`UPDATE events SET visibility = ? WHERE id = ?`, vis, eventID); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			who, token string
			want       int
		}{
			{"a member", memberToken, http.StatusOK},
			{"an admin", adminToken, http.StatusOK},
			{"a follower", followerToken, http.StatusNotFound},
			{"an outsider", outsiderToken, http.StatusNotFound},
			{"nobody", "", http.StatusNotFound},
		} {
			r := authedRequest("GET", "/api/v1/events/"+eventID+"/event.ics", nil, tc.token)
			w := serveOptionalAuthMux(t, db, "GET", "/api/v1/events/{id}/event.ics", handler.EventICS(db, cfg), r)
			if w.Code != tc.want {
				t.Errorf("%s on a %s event: code=%d, want %d", tc.who, vis, w.Code, tc.want)
			}
		}
	}
}

// A pending submission has no calendar file, matching every other surface
// that treats it as not yet existing (docs/adr/026).
func TestEventICS_PendingSubmissionHasNoFile(t *testing.T) {
	db := setupTestDB(t)
	cfg := feedTestConfig()
	admin, adminToken := createTestUser(t, db, "icspendadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Queue Venue", "queue-venue", "open")

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	eventID := seedEvent(t, db, nodeID, admin.ID, "Unreviewed Show", future)
	if _, err := db.Exec(`UPDATE events SET status = 'pending_review' WHERE id = ?`, eventID); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("GET", "/api/v1/events/"+eventID+"/event.ics", nil, adminToken)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/events/{id}/event.ics", handler.EventICS(db, cfg), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("pending event: code=%d, want 404", w.Code)
	}
}
