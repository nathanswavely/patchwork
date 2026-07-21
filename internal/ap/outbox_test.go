package ap_test

import (
	"io/fs"
	"os"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "patchwork-ap-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestQueueActivity(t *testing.T) {
	db := setupTestDB(t)

	activity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Accept",
		"actor":    "https://example.com/ap/nodes/node-1",
	}
	targetInbox := "https://remote.example/ap/users/remote-user/inbox"

	err := ap.QueueActivity(db, activity, targetInbox)
	if err != nil {
		t.Fatalf("QueueActivity: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue WHERE status = 'pending'").Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending queue entry, got %d", count)
	}

	var storedInbox string
	err = db.QueryRow("SELECT target_inbox FROM ap_outbox_queue WHERE status = 'pending'").Scan(&storedInbox)
	if err != nil {
		t.Fatalf("query target_inbox: %v", err)
	}
	if storedInbox != targetInbox {
		t.Errorf("expected target_inbox=%s, got %s", targetInbox, storedInbox)
	}
}

func TestBroadcastToFollowers(t *testing.T) {
	db := setupTestDB(t)

	nodeID := "test-node-broadcast"

	// Insert 2 ap_followers records with inboxes.
	for i, remote := range []string{"https://remote1.example/ap/users/u1", "https://remote2.example/ap/users/u2"} {
		_, err := db.Exec(
			`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES (?, 'node', ?, ?, ?, 1)`,
			"follower-id-"+string(rune('a'+i)), nodeID, remote, remote+"/inbox",
		)
		if err != nil {
			t.Fatalf("insert follower %d: %v", i, err)
		}
	}

	activity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Create",
		"actor":    "https://example.com/ap/nodes/" + nodeID,
	}

	err := ap.BroadcastToFollowers(db, "node", nodeID, activity)
	if err != nil {
		t.Fatalf("BroadcastToFollowers: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue WHERE status = 'pending'").Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 queued entries, got %d", count)
	}
}

func TestBroadcastToFollowers_NoFollowers(t *testing.T) {
	db := setupTestDB(t)

	activity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Create",
		"actor":    "https://example.com/ap/nodes/empty-node",
	}

	err := ap.BroadcastToFollowers(db, "node", "empty-node", activity)
	if err != nil {
		t.Fatalf("BroadcastToFollowers: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 queue entries, got %d", count)
	}
}

func TestBuildAcceptFollow(t *testing.T) {
	localActorID := "https://example.com/ap/nodes/node-1"
	followActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Follow",
		"actor":    "https://remote.example/ap/users/remote-user",
		"object":   localActorID,
	}

	accept := ap.BuildAcceptFollow(localActorID, followActivity)

	if accept["@context"] != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("expected @context, got %v", accept["@context"])
	}
	if accept["type"] != "Accept" {
		t.Errorf("expected type=Accept, got %v", accept["type"])
	}
	if accept["actor"] != localActorID {
		t.Errorf("expected actor=%s, got %v", localActorID, accept["actor"])
	}
	obj, ok := accept["object"].(map[string]interface{})
	if !ok {
		t.Fatal("expected object to be a map")
	}
	if obj["type"] != "Follow" {
		t.Errorf("expected inner object type=Follow, got %v", obj["type"])
	}
}
