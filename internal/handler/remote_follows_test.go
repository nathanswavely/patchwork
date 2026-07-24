package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// Cross-quilt following (docs/adr/024).

// authedJSONRequest is authedRequest for raw JSON string bodies —
// authedRequest marshals its body, which would double-encode a string.
func authedJSONRequest(method, path, body, token string) *http.Request {
	if body == "" {
		return authedRequest(method, path, nil, token)
	}
	return authedRequest(method, path, json.RawMessage(body), token)
}

func federatedCfg() *config.Config {
	return &config.Config{Federation: config.Federation{Enabled: true}}
}

func stubRemoteActor(t *testing.T) {
	t.Helper()
	prev := ap.SetActorFetcher(func(ctx context.Context, actorID string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: actorID, Inbox: actorID + "/inbox"}, nil
	})
	t.Cleanup(func() { ap.SetActorFetcher(prev); ap.ClearActorCache() })
}

// waitFor polls until check returns true or the deadline passes. The AP
// relay runs in a goroutine behind the HTTP response, so tests poll.
func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func countRows(db *database.DB, query string, args ...interface{}) int {
	var n int
	db.QueryRow(query, args...).Scan(&n)
	return n
}

func TestRemoteFollow_CreateListDelete(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "weaver", "member")

	body := `{"quilt_url":"https://other.example","node_ap_id":"https://other.example/ap/nodes/abc","node_slug":"gallery-row","node_name":"Gallery Row","snapshot":{"appearance":{"palette":"protest"}}}`
	r := authedJSONRequest("POST", "/api/v1/users/me/remote-follows", body, token)
	w := serveMux(t, db, "POST", "/api/v1/users/me/remote-follows", handler.CreateRemoteFollow(db, &config.Config{}), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["node_slug"] != "gallery-row" {
		t.Errorf("node_slug = %v", resp["node_slug"])
	}
	snapshot, _ := resp["snapshot"].(map[string]interface{})
	if snapshot == nil || snapshot["appearance"] == nil {
		t.Errorf("snapshot not round-tripped: %v", resp["snapshot"])
	}

	// Idempotent: re-follow refreshes rather than duplicating.
	r = authedJSONRequest("POST", "/api/v1/users/me/remote-follows", body, token)
	w = serveMux(t, db, "POST", "/api/v1/users/me/remote-follows", handler.CreateRemoteFollow(db, &config.Config{}), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("re-follow: got %d", w.Code)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM remote_follows"); n != 1 {
		t.Fatalf("expected 1 follow row, got %d", n)
	}

	r = authedJSONRequest("GET", "/api/v1/users/me/remote-follows", "", token)
	w = serveMux(t, db, "GET", "/api/v1/users/me/remote-follows", handler.ListRemoteFollows(db), r)
	list := decodeJSON(t, w)
	follows, _ := list["remote_follows"].([]interface{})
	if len(follows) != 1 {
		t.Fatalf("expected 1 follow in list, got %d", len(follows))
	}

	followID := follows[0].(map[string]interface{})["id"].(string)
	r = authedJSONRequest("DELETE", "/api/v1/users/me/remote-follows/"+followID, "", token)
	w = serveMux(t, db, "DELETE", "/api/v1/users/me/remote-follows/{id}", handler.DeleteRemoteFollow(db, &config.Config{}), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", w.Code)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM remote_follows"); n != 0 {
		t.Fatalf("expected 0 follow rows after delete, got %d", n)
	}
}

func TestRemoteFollow_RejectsMismatchedHost(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "weaver", "member")

	body := `{"quilt_url":"https://other.example","node_ap_id":"https://evil.example/ap/nodes/abc","node_slug":"x"}`
	r := authedJSONRequest("POST", "/api/v1/users/me/remote-follows", body, token)
	w := serveMux(t, db, "POST", "/api/v1/users/me/remote-follows", handler.CreateRemoteFollow(db, &config.Config{}), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched host, got %d", w.Code)
	}
}

func TestRemoteFollow_RelaysInstanceFollowOnce(t *testing.T) {
	db := setupTestDB(t)
	stubRemoteActor(t)
	ap.SetDomain("home.example")
	if err := ap.EnsureInstanceActor(db, "home.example"); err != nil {
		t.Fatalf("ensure instance actor: %v", err)
	}
	cfg := federatedCfg()

	_, token1 := createTestUser(t, db, "weaver", "member")
	_, token2 := createTestUser(t, db, "stitcher", "member")

	nodeAPID := "https://other.example/ap/nodes/abc"
	body := fmt.Sprintf(`{"quilt_url":"https://other.example","node_ap_id":"%s","node_slug":"gallery-row"}`, nodeAPID)

	r := authedJSONRequest("POST", "/api/v1/users/me/remote-follows", body, token1)
	w := serveMux(t, db, "POST", "/api/v1/users/me/remote-follows", handler.CreateRemoteFollow(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("follow 1: got %d: %s", w.Code, w.Body.String())
	}

	// One outbound Follow queued from the instance actor.
	waitFor(t, "ap_following row", func() bool {
		return countRows(db, "SELECT COUNT(*) FROM ap_following WHERE remote_actor_id = ?", nodeAPID) == 1
	})
	waitFor(t, "queued Follow", func() bool {
		return countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%"type":"Follow"%'`) == 1
	})
	var queuedActor int
	queuedActor = countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%/ap/instance%'`)
	if queuedActor != 1 {
		t.Fatalf("Follow should be sent by the instance actor, found %d matching rows", queuedActor)
	}

	// Second local follower: no second Follow relayed.
	r = authedJSONRequest("POST", "/api/v1/users/me/remote-follows", body, token2)
	w = serveMux(t, db, "POST", "/api/v1/users/me/remote-follows", handler.CreateRemoteFollow(db, cfg), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("follow 2: got %d", w.Code)
	}
	time.Sleep(150 * time.Millisecond) // give a wrong second relay time to appear
	if n := countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%"type":"Follow"%'`); n != 1 {
		t.Fatalf("expected exactly 1 queued Follow after second follower, got %d", n)
	}

	// First unfollow: still one local follower left, no Undo.
	var follow1ID string
	db.QueryRow("SELECT rf.id FROM remote_follows rf JOIN users u ON u.id = rf.user_id AND u.username = 'weaver'").Scan(&follow1ID)
	r = authedJSONRequest("DELETE", "/api/v1/users/me/remote-follows/"+follow1ID, "", token1)
	w = serveMux(t, db, "DELETE", "/api/v1/users/me/remote-follows/{id}", handler.DeleteRemoteFollow(db, cfg), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("unfollow 1: got %d", w.Code)
	}
	time.Sleep(150 * time.Millisecond)
	if n := countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%"type":"Undo"%'`); n != 0 {
		t.Fatalf("Undo queued while a follower remains")
	}

	// Last unfollow: Undo(Follow) relayed, ap_following row removed.
	var follow2ID string
	db.QueryRow("SELECT rf.id FROM remote_follows rf JOIN users u ON u.id = rf.user_id AND u.username = 'stitcher'").Scan(&follow2ID)
	r = authedJSONRequest("DELETE", "/api/v1/users/me/remote-follows/"+follow2ID, "", token2)
	w = serveMux(t, db, "DELETE", "/api/v1/users/me/remote-follows/{id}", handler.DeleteRemoteFollow(db, cfg), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("unfollow 2: got %d", w.Code)
	}
	waitFor(t, "queued Undo", func() bool {
		return countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%"type":"Undo"%'`) == 1
	})
	waitFor(t, "ap_following cleanup", func() bool {
		return countRows(db, "SELECT COUNT(*) FROM ap_following WHERE remote_actor_id = ?", nodeAPID) == 0
	})
}

func TestInstanceInbox_AcceptMarksFollowing(t *testing.T) {
	db := setupTestDB(t)
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))
	ap.SetDomain("home.example")

	db.Exec("INSERT INTO ap_following (id, remote_actor_id, remote_inbox) VALUES ('f1', 'https://other.example/ap/nodes/abc', 'https://other.example/inbox')")

	body := `{"type":"Accept","actor":"https://other.example/ap/nodes/abc","object":{"type":"Follow","actor":"https://home.example/ap/instance"}}`
	r := authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	w := servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("accept: got %d: %s", w.Code, w.Body.String())
	}
	if n := countRows(db, "SELECT COUNT(*) FROM ap_following WHERE accepted = 1"); n != 1 {
		t.Fatalf("follow not marked accepted")
	}
}

func TestInstanceInbox_CreateFansOutNotifications(t *testing.T) {
	db := setupTestDB(t)
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	u1, _ := createTestUser(t, db, "weaver", "member")
	u2, _ := createTestUser(t, db, "stitcher", "member")
	u3, _ := createTestUser(t, db, "bystander", "member")

	nodeAPID := "https://other.example/ap/nodes/abc"
	for i, u := range []string{u1.ID, u2.ID} {
		db.Exec(
			"INSERT INTO remote_follows (id, user_id, quilt_url, node_ap_id, node_slug, node_name) VALUES (?, ?, 'https://other.example', ?, 'gallery-row', 'Gallery Row')",
			fmt.Sprintf("rf%d", i), u, nodeAPID,
		)
	}

	body := `{"type":"Create","actor":"https://other.example/ap/nodes/abc","object":{"type":"Event","name":"Print Swap Night","startTime":"2026-08-01T19:00:00Z"}}`
	r := authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	w := servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create: got %d: %s", w.Code, w.Body.String())
	}

	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE type = 'remote.event'"); n != 2 {
		t.Fatalf("expected 2 remote.event notifications, got %d", n)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE user_id = ?", u3.ID); n != 0 {
		t.Fatalf("non-follower notified")
	}
	var link string
	db.QueryRow("SELECT link FROM notifications WHERE user_id = ?", u1.ID).Scan(&link)
	if link != "/quilts/other.example/patches/gallery-row" {
		t.Errorf("link = %q", link)
	}

	// Redelivery of the same activity must not duplicate notifications.
	r = authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	w = servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("redelivery: got %d", w.Code)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE type = 'remote.event'"); n != 2 {
		t.Fatalf("redelivery duplicated notifications: %d", n)
	}
}

func TestNeighborQuilts_AdminCRUDAndPublicExposure(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "quiltkeeper", "admin")
	cfg := &config.Config{}

	r := authedJSONRequest("POST", "/api/v1/admin/neighbor-quilts", `{"url":"https://discgolf.example/","name":"Disc Golf"}`, adminToken)
	w := serveAdminMux(t, db, "POST", "/api/v1/admin/neighbor-quilts", handler.AdminAddNeighborQuilt(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: got %d: %s", w.Code, w.Body.String())
	}
	added := decodeJSON(t, w)
	if added["url"] != "https://discgolf.example" {
		t.Errorf("url not normalized to origin: %v", added["url"])
	}

	// Public instance document lists neighbors for everyone.
	r = authedJSONRequest("GET", "/api/v1/instance", "", "")
	w = servePublicMux(t, "GET", "/api/v1/instance", handler.Instance(db, cfg), r)
	inst := decodeJSON(t, w)
	neighbors, _ := inst["neighbor_quilts"].([]interface{})
	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor quilt on instance doc, got %d", len(neighbors))
	}

	id := added["id"].(string)
	r = authedJSONRequest("DELETE", "/api/v1/admin/neighbor-quilts/"+id, "", adminToken)
	w = serveAdminMux(t, db, "DELETE", "/api/v1/admin/neighbor-quilts/{id}", handler.AdminDeleteNeighborQuilt(db), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", w.Code)
	}
}

func TestUserQuilts_CRUD(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "weaver", "member")

	r := authedJSONRequest("POST", "/api/v1/users/me/quilts", `{"url":"https://discgolf.example/some/path","name":"Disc Golf"}`, token)
	w := serveMux(t, db, "POST", "/api/v1/users/me/quilts", handler.AddUserQuilt(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: got %d: %s", w.Code, w.Body.String())
	}
	added := decodeJSON(t, w)
	if added["url"] != "https://discgolf.example" {
		t.Errorf("url not normalized: %v", added["url"])
	}

	r = authedJSONRequest("GET", "/api/v1/users/me/quilts", "", token)
	w = serveMux(t, db, "GET", "/api/v1/users/me/quilts", handler.ListUserQuilts(db), r)
	list := decodeJSON(t, w)
	quilts, _ := list["quilts"].([]interface{})
	if len(quilts) != 1 {
		t.Fatalf("expected 1 quilt, got %d", len(quilts))
	}

	id := added["id"].(string)
	r = authedJSONRequest("DELETE", "/api/v1/users/me/quilts/"+id, "", token)
	w = serveMux(t, db, "DELETE", "/api/v1/users/me/quilts/{id}", handler.DeleteUserQuilt(db), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", w.Code)
	}
}

func TestCreateEvent_BroadcastsToAPFollowers(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("home.example")
	user, token := createTestUser(t, db, "weaver", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	db.Exec(
		"INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES ('f1', 'node', ?, 'https://other.example/ap/instance', 'https://other.example/ap/instance/inbox', 1)",
		nodeID,
	)

	body := fmt.Sprintf(`{"node_id":"%s","title":"Print Swap Night","starts_at":"2026-08-01T19:00:00Z"}`, nodeID)
	r := authedJSONRequest("POST", "/api/v1/events", body, token)
	w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, submissionsCfg(true)), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create event: got %d: %s", w.Code, w.Body.String())
	}

	waitFor(t, "queued event Create", func() bool {
		return countRows(db, `SELECT COUNT(*) FROM ap_outbox_queue WHERE activity_json LIKE '%"type":"Create"%' AND activity_json LIKE '%Print Swap Night%'`) == 1
	})
}

// serveAdminMux wraps a handler in AdminRequired the way main.go registers
// admin routes.
func serveAdminMux(t *testing.T, db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AdminRequired(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestCheckActorBinding(t *testing.T) {
	activity := map[string]interface{}{"actor": "https://other.example/ap/nodes/abc"}
	if err := handler.CheckActorBinding("https://other.example/ap/nodes/abc", activity); err != nil {
		t.Errorf("matching actor rejected: %v", err)
	}
	if err := handler.CheckActorBinding("https://evil.example/ap/users/mallory", activity); err == nil {
		t.Error("spoofed actor accepted: signature by one actor must not deliver activities in another's name")
	}
	// Empty verified actor = verification disabled (tests) — binding is a no-op.
	if err := handler.CheckActorBinding("", activity); err != nil {
		t.Errorf("binding should be skipped when verification is off: %v", err)
	}
}

func TestInstanceInbox_DistinctPinsDistinctNotifications(t *testing.T) {
	db := setupTestDB(t)
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	u, _ := createTestUser(t, db, "weaver", "member")
	nodeAPID := "https://other.example/ap/nodes/abc"
	db.Exec(
		"INSERT INTO remote_follows (id, user_id, quilt_url, node_ap_id, node_slug, node_name) VALUES ('rf1', ?, 'https://other.example', ?, 'gallery-row', 'Gallery Row')",
		u.ID, nodeAPID,
	)

	// Two different events with the SAME name must both notify.
	for _, objID := range []string{"https://other.example/ap/events/ev-1", "https://other.example/ap/events/ev-2"} {
		body := fmt.Sprintf(`{"type":"Create","actor":"%s","object":{"type":"Event","id":"%s","name":"Open Studio"}}`, nodeAPID, objID)
		r := authedJSONRequest("POST", "/ap/instance/inbox", body, "")
		w := servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
		if w.Code != http.StatusAccepted {
			t.Fatalf("create %s: got %d", objID, w.Code)
		}
	}
	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE type = 'remote.event'"); n != 2 {
		t.Fatalf("two distinct events should make two notifications, got %d", n)
	}

	// Redelivery of one of them must still dedup.
	body := fmt.Sprintf(`{"type":"Create","actor":"%s","object":{"type":"Event","id":"https://other.example/ap/events/ev-1","name":"Open Studio"}}`, nodeAPID)
	r := authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE type = 'remote.event'"); n != 2 {
		t.Fatalf("redelivery duplicated: %d", n)
	}

	// An Update to the same event is news with its own title, not a dup.
	body = fmt.Sprintf(`{"type":"Update","actor":"%s","object":{"type":"Event","id":"https://other.example/ap/events/ev-1","name":"Open Studio"}}`, nodeAPID)
	r = authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if n := countRows(db, "SELECT COUNT(*) FROM notifications WHERE title LIKE 'Updated event:%'"); n != 1 {
		t.Fatalf("expected 1 updated-event notification, got %d", n)
	}
}

func TestInstanceInbox_AcceptRequiresOurFollow(t *testing.T) {
	db := setupTestDB(t)
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))
	ap.SetDomain("home.example")

	db.Exec("INSERT INTO ap_following (id, remote_actor_id, remote_inbox) VALUES ('f1', 'https://other.example/ap/nodes/abc', 'https://other.example/inbox')")

	// An Accept that doesn't wrap our Follow must not flip accepted.
	body := `{"type":"Accept","actor":"https://other.example/ap/nodes/abc","object":{"type":"Like"}}`
	r := authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	w := servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("accept: got %d", w.Code)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM ap_following WHERE accepted = 1"); n != 0 {
		t.Fatal("Accept without our Follow flipped accepted")
	}

	// A proper Accept wrapping our instance actor's Follow does.
	body = `{"type":"Accept","actor":"https://other.example/ap/nodes/abc","object":{"type":"Follow","actor":"https://home.example/ap/instance"}}`
	r = authedJSONRequest("POST", "/ap/instance/inbox", body, "")
	servePublicMux(t, "POST", "/ap/instance/inbox", handler.APInstanceInbox(db), r)
	if n := countRows(db, "SELECT COUNT(*) FROM ap_following WHERE accepted = 1"); n != 1 {
		t.Fatal("valid Accept did not flip accepted")
	}
}
