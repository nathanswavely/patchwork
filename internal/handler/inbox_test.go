package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func apInboxRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/activity+json")
	return r
}

func TestAPNodeInbox_Follow(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	owner, _ := createTestUser(t, db, "inboxowner1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Inbox Patch", "inbox-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	remoteActor := "https://remote.example/ap/users/remote-user"
	followActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Follow",
		"actor":    remoteActor,
		"object":   fmt.Sprintf("https://%s/ap/nodes/%s", ap.GetDomain(), nodeID),
	}

	r := apInboxRequest(t, "POST", "/ap/nodes/"+nodeID+"/inbox", followActivity)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	// Verify ap_followers record was created.
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = ? AND remote_actor_id = ?",
		nodeID, remoteActor,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query ap_followers: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 ap_followers record, got %d", count)
	}

	// Verify an Accept activity was queued in ap_outbox_queue.
	var queueCount int
	err = db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue WHERE status = 'pending'").Scan(&queueCount)
	if err != nil {
		t.Fatalf("query ap_outbox_queue: %v", err)
	}
	if queueCount != 1 {
		t.Errorf("expected 1 queued activity, got %d", queueCount)
	}

	// Verify the queued activity is an Accept.
	var activityJSON string
	err = db.QueryRow("SELECT activity_json FROM ap_outbox_queue WHERE status = 'pending'").Scan(&activityJSON)
	if err != nil {
		t.Fatalf("query activity_json: %v", err)
	}
	var queued map[string]interface{}
	if err := json.Unmarshal([]byte(activityJSON), &queued); err != nil {
		t.Fatalf("unmarshal queued activity: %v", err)
	}
	if queued["type"] != "Accept" {
		t.Errorf("expected queued activity type=Accept, got %v", queued["type"])
	}
}

func TestAPNodeInbox_Follow_NodeNotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	followActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Follow",
		"actor":    "https://remote.example/ap/users/remote-user",
		"object":   "https://test.example.com/ap/nodes/nonexistent-node-id",
	}

	r := apInboxRequest(t, "POST", "/ap/nodes/nonexistent-node-id/inbox", followActivity)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPNodeInbox_UndoFollow(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	owner, _ := createTestUser(t, db, "inboxowner3", "member")
	nodeID := createTestNode(t, db, owner.ID, "Undo Patch", "undo-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	remoteActor := "https://remote.example/ap/users/undo-user"

	// Insert a follower record first.
	followerID := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES (?, 'node', ?, ?, ?, 1)`,
		followerID, nodeID, remoteActor, remoteActor+"/inbox",
	)
	if err != nil {
		t.Fatalf("insert ap_follower: %v", err)
	}

	// Verify the record exists.
	var beforeCount int
	db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = ? AND remote_actor_id = ?", nodeID, remoteActor).Scan(&beforeCount)
	if beforeCount != 1 {
		t.Fatalf("expected 1 follower record before undo, got %d", beforeCount)
	}

	undoActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Undo",
		"actor":    remoteActor,
		"object": map[string]interface{}{
			"type":   "Follow",
			"actor":  remoteActor,
			"object": fmt.Sprintf("https://%s/ap/nodes/%s", ap.GetDomain(), nodeID),
		},
	}

	r := apInboxRequest(t, "POST", "/ap/nodes/"+nodeID+"/inbox", undoActivity)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the ap_followers record was deleted.
	var afterCount int
	db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = ? AND remote_actor_id = ?", nodeID, remoteActor).Scan(&afterCount)
	if afterCount != 0 {
		t.Errorf("expected 0 follower records after undo, got %d", afterCount)
	}
}

func TestAPNodeInbox_UnknownActivity(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	owner, _ := createTestUser(t, db, "inboxowner4", "member")
	nodeID := createTestNode(t, db, owner.ID, "Unknown Patch", "unknown-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	likeActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Like",
		"actor":    "https://remote.example/ap/users/liker",
		"object":   fmt.Sprintf("https://%s/ap/nodes/%s", ap.GetDomain(), nodeID),
	}

	r := apInboxRequest(t, "POST", "/ap/nodes/"+nodeID+"/inbox", likeActivity)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPNodeInbox_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")
	defer handler.SetRequireSignature(handler.SetRequireSignature(false))

	owner, _ := createTestUser(t, db, "inboxowner5", "member")
	nodeID := createTestNode(t, db, owner.ID, "Invalid Patch", "invalid-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	r := httptest.NewRequest("POST", "/ap/nodes/"+nodeID+"/inbox", bytes.NewReader([]byte("not json at all")))
	r.Header.Set("Content-Type", "application/activity+json")
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
