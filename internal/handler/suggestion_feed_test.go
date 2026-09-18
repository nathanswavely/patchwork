package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// A suggestion may carry a feed, and its approval decides who keeps the
// calendar afterwards
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decisions 3, 4, 6 and 8).

// oneFutureEventFeed serves a minimal ICS with a single event still to come,
// and opens the SSRF guard for the loopback address httptest hands out. The
// guard is the reason a test feed needs a switch at all: every outbound fetch
// in this codebase refuses private addresses, which is exactly where a test
// server lives.
func oneFutureEventFeed(t *testing.T) string {
	t.Helper()
	start := time.Now().Add(72 * time.Hour).UTC().Format("20060102T150405Z")
	body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//T//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:suggested@test\r\nSUMMARY:Suggested Show\r\nDTSTART:" + start +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })
	return srv.URL + "/calendar.ics"
}

func suggestedFeed(t *testing.T, db *database.DB, nodeID string) (string, int) {
	t.Helper()
	var url string
	var count int
	db.QueryRow(
		`SELECT COALESCE(suggested_feed_url,''), COALESCE(suggested_feed_count,0) FROM nodes WHERE id = ?`,
		nodeID,
	).Scan(&url, &count)
	return url, count
}

func sourceAddedBy(t *testing.T, db *database.DB, nodeID string) (string, int) {
	t.Helper()
	var n int
	var addedBy string
	db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE node_id = ?`, nodeID).Scan(&n)
	db.QueryRow(`SELECT COALESCE(added_by,'') FROM event_sources WHERE node_id = ? LIMIT 1`, nodeID).Scan(&addedBy)
	return addedBy, n
}

// submitSuggestion posts a suggestion and returns the created node's id.
func submitSuggestion(t *testing.T, db *database.DB, token string, body map[string]interface{}) (*httptest.ResponseRecorder, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/submissions", body, token)
	w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, submissionsCfg(true)), r)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if id, ok := resp["id"].(string); ok {
		return w, id
	}
	if node, ok := resp["node"].(map[string]interface{}); ok {
		if id, ok := node["id"].(string); ok {
			return w, id
		}
	}
	return w, ""
}

func reviewSubmission(t *testing.T, db *database.DB, adminToken, nodeID string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/admin/submissions/"+nodeID, body, adminToken)
	return serveAdminMux(t, db, "PATCH", "/api/v1/admin/submissions/{id}", handler.ReviewSubmission(db), r)
}

// notificationBody returns the body of the one notification of this type the
// person holds, or "" with a failure when the count is not one.
func notificationBody(t *testing.T, db *database.DB, userID string, typ notifications.NotificationType) string {
	t.Helper()
	if n := countNotifications(t, db, userID, typ, 1); n != 1 {
		t.Fatalf("notifications of type %s: got %d, want 1", typ, n)
	}
	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		userID, string(typ)).Scan(&body)
	return body
}

// An address that is not a calendar is refused now rather than never, and
// nothing is written: a suggestion half-made is worse than one refused.
func TestSuggestedFeedThatCannotBeReadIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>we have shows sometimes</body></html>"))
	}))
	defer srv.Close()
	prev := safehttp.SetAllowPrivateAddresses(true)
	defer safehttp.SetAllowPrivateAddresses(prev)

	db := setupTestDB(t)
	_, token := createTestUser(t, db, "suggester", "member")

	w, _ := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Unreadable Venue", "feed_url": srv.URL + "/events",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unreadable feed: got %d %s, want 400", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.Bytes(), "could not read a calendar at that address") {
		t.Errorf("error body: %s", w.Body.String())
	}
	var nodes int
	db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE name = 'Unreadable Venue'`).Scan(&nodes)
	if nodes != 0 {
		t.Errorf("rows created by a refused suggestion: %d, want 0", nodes)
	}

	// A scheme that is not http(s) never reaches the network at all.
	w, _ = submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Odd Scheme", "feed_url": "ftp://files.example/cal.ics",
	})
	if w.Code != http.StatusBadRequest || !bodyContains(w.Body.Bytes(), "feed url must be http(s)") {
		t.Errorf("ftp feed: got %d %s", w.Code, w.Body.String())
	}
}

// A readable feed rides on the listing row until somebody decides about it,
// with the count one fetch found — the number the reviewing admin reads.
func TestSuggestedFeedIsParkedOnTheListing(t *testing.T) {
	feedURL := oneFutureEventFeed(t)
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "suggester", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	w, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Spark Hall", "feed_url": feedURL,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}
	url, count := suggestedFeed(t, db, nodeID)
	if url != feedURL || count != 1 {
		t.Errorf("parked feed: url=%q count=%d, want %q and 1", url, count, feedURL)
	}
	// Nothing syncs a feed onto a patch nobody has approved.
	if _, n := sourceAddedBy(t, db, nodeID); n != 0 {
		t.Errorf("event sources before approval: %d, want 0", n)
	}

	// And the review queue states both, because "attach this feed" is not a
	// decision anybody can make from a URL alone.
	r := authedRequest("GET", "/api/v1/admin/submissions", nil, adminToken)
	w = serveAdminMux(t, db, "GET", "/api/v1/admin/submissions", handler.ListSubmissions(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("queue: got %d %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.Bytes(), `"feed_url":"`+feedURL+`"`) ||
		!bodyContains(w.Body.Bytes(), `"feed_upcoming_count":1`) {
		t.Errorf("queue item: %s", w.Body.String())
	}
}

// Approving with the form's defaults: the suggester keeps the calendar they
// suggested, its feed attaches under their name, and they are told so.
func TestApprovalGrantsTrustAndAttachesTheFeed(t *testing.T) {
	feedURL := oneFutureEventFeed(t)
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	suggester, token := createTestUser(t, db, "suggester", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	_, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Spark Hall", "feed_url": feedURL,
	})
	if w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{"action": "approve"}); w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}

	var grantSource string
	if err := db.QueryRow(
		`SELECT source FROM node_trusted_contributors WHERE user_id = ? AND node_id = ?`,
		suggester.ID, nodeID,
	).Scan(&grantSource); err != nil {
		t.Fatalf("per-patch grant after approval: %v", err)
	}
	if grantSource != "suggestion" {
		t.Errorf("grant source: %q, want %q", grantSource, "suggestion")
	}

	addedBy, n := sourceAddedBy(t, db, nodeID)
	if n != 1 || addedBy != suggester.ID {
		t.Errorf("attached sources: %d added_by=%q, want 1 by the suggester", n, addedBy)
	}
	if url, count := suggestedFeed(t, db, nodeID); url != "" || count != 0 {
		t.Errorf("suggested columns after approval: url=%q count=%d, want cleared", url, count)
	}

	body := notificationBody(t, db, suggester.ID, notifications.SubmissionApproved)
	if !strings.Contains(body, "without review") {
		t.Errorf("approval notification body: %q, want the trust sentence", body)
	}
	if !strings.Contains(body, "feed is attached") {
		t.Errorf("approval notification body: %q, want the feed sentence", body)
	}

	// The grant is audited on the person, because what changed is what they
	// may do.
	var meta string
	if err := db.QueryRow(
		`SELECT metadata FROM audit_log WHERE action = 'trust.granted' AND entity_type = 'user' AND entity_id = ?`,
		suggester.ID,
	).Scan(&meta); err != nil {
		t.Fatalf("trust.granted audit entry: %v", err)
	}
	if !strings.Contains(meta, `"scope":"patch"`) || !strings.Contains(meta, `"source":"suggestion"`) {
		t.Errorf("trust.granted metadata: %s", meta)
	}
}

// The admin may uncheck the trust and keep the feed. Then the vouch is
// theirs: the source is added by the reviewer, and the suggester is told
// plainly that their events still queue.
func TestApprovalMayWithholdTrustAndStillAttach(t *testing.T) {
	feedURL := oneFutureEventFeed(t)
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	suggester, token := createTestUser(t, db, "suggester", "member")
	admin, adminToken := createTestUser(t, db, "siteadmin", "admin")

	_, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Spark Hall", "feed_url": feedURL,
	})
	w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{
		"action": "approve", "grant_trust": false, "attach_feed": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}

	if n := trustGrantCount(t, db, nodeID); n != 0 {
		t.Errorf("grants after an unchecked approval: %d, want 0", n)
	}
	addedBy, n := sourceAddedBy(t, db, nodeID)
	if n != 1 || addedBy != admin.ID {
		t.Errorf("attached sources: %d added_by=%q, want 1 by the reviewer", n, addedBy)
	}

	body := notificationBody(t, db, suggester.ID, notifications.SubmissionApproved)
	if !strings.Contains(body, "go through review") {
		t.Errorf("approval notification body: %q, want the review sentence", body)
	}
	if !strings.Contains(body, "feed is attached") {
		t.Errorf("approval notification body: %q, want the feed sentence", body)
	}
}

// Unchecking the feed leaves the listing with no source at all, and the
// review is still over: the parked URL goes either way.
func TestApprovalMayDeclineTheFeed(t *testing.T) {
	feedURL := oneFutureEventFeed(t)
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "suggester", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	_, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Spark Hall", "feed_url": feedURL,
	})
	w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{
		"action": "approve", "attach_feed": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	if _, n := sourceAddedBy(t, db, nodeID); n != 0 {
		t.Errorf("sources after declining the feed: %d, want 0", n)
	}
	if url, count := suggestedFeed(t, db, nodeID); url != "" || count != 0 {
		t.Errorf("suggested columns after a declined feed: url=%q count=%d, want cleared", url, count)
	}
}

// The note the review form already collected reaches the person it is about,
// instead of only the audit log.
func TestDeclinedSuggestionCarriesTheNote(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	suggester, token := createTestUser(t, db, "suggester", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	_, nodeID := submitSuggestion(t, db, token, map[string]interface{}{"name": "Spark Hall"})
	w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{
		"action": "reject", "note": "This one is already on the quilt as Spark Hall Arts.",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("reject: got %d %s", w.Code, w.Body.String())
	}

	body := notificationBody(t, db, suggester.ID, notifications.SubmissionRejected)
	if !strings.Contains(body, "already on the quilt as Spark Hall Arts") {
		t.Errorf("decline notification body: %q, want the admin's note", body)
	}
}

// A quilt-wide trusted contributor's suggestion is a listing at once
// (decision 4): the feed attaches in the same breath, and there is no queue
// notification, because there is no queue.
func TestTrustedSuggestionPublishesWithItsFeed(t *testing.T) {
	feedURL := oneFutureEventFeed(t)
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	trusted, token := createTestUser(t, db, "helper", "member")
	makeTrusted(t, db, trusted.ID)
	siteAdmin, _ := createTestUser(t, db, "siteadmin", "admin")

	w, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Spark Hall", "feed_url": feedURL,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.Bytes(), `"status":"unclaimed"`) {
		t.Errorf("response: %s, want an unclaimed listing", w.Body.String())
	}
	var status string
	db.QueryRow(`SELECT status FROM nodes WHERE id = ?`, nodeID).Scan(&status)
	if status != "unclaimed" {
		t.Errorf("node status: %q, want unclaimed", status)
	}

	addedBy, n := sourceAddedBy(t, db, nodeID)
	if n != 1 || addedBy != trusted.ID {
		t.Errorf("attached sources: %d added_by=%q, want 1 by the suggester", n, addedBy)
	}
	if url, count := suggestedFeed(t, db, nodeID); url != "" || count != 0 {
		t.Errorf("suggested columns: url=%q count=%d, want cleared", url, count)
	}

	if n := countNotifications(t, db, siteAdmin.ID, notifications.AdminSubmission, 0); n != 0 {
		t.Errorf("site admin queue notifications: %d, want 0 — there is nothing to review", n)
	}

	// Let the background first sync finish before the database closes.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var fetched int
		db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE node_id = ? AND last_fetch_at IS NOT NULL`, nodeID).Scan(&fetched)
		if fetched == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
