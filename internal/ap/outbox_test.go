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

// insertBroadcastNode inserts a live node (and its owner) so the broadcast
// gate in BroadcastToFollowers has a real row to check.
func insertBroadcastNode(t *testing.T, db *database.DB, nodeID, visibility string) {
	t.Helper()
	ownerID := nodeID + "-owner"
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role) VALUES (?, ?, ?, 'member')`,
		ownerID, ownerID, ownerID,
	); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status) VALUES (?, ?, ?, ?, '', 'leaf', ?, 'open', 'active')`,
		nodeID, ownerID, nodeID, nodeID, visibility,
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
}

func TestBroadcastToFollowers(t *testing.T) {
	db := setupTestDB(t)

	nodeID := "test-node-broadcast"
	insertBroadcastNode(t, db, nodeID, "public")

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

	insertBroadcastNode(t, db, "empty-node", "public")

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

// TestBroadcastToFollowers_NonPublicNodeSkips: federation is public-only
// (docs/adr/024). A node that went private keeps its follower rows — flipping
// back to public resumes delivery — but broadcasts nothing while private.
func TestBroadcastToFollowers_NonPublicNodeSkips(t *testing.T) {
	db := setupTestDB(t)

	nodeID := "private-node-broadcast"
	insertBroadcastNode(t, db, nodeID, "private")

	// A follower acquired while the node was still public.
	if _, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES ('stale-follower', 'node', ?, 'https://remote1.example/ap/users/u1', 'https://remote1.example/ap/users/u1/inbox', 1)`,
		nodeID,
	); err != nil {
		t.Fatalf("insert follower: %v", err)
	}

	activity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Create",
		"actor":    "https://example.com/ap/nodes/" + nodeID,
	}

	if err := ap.BroadcastToFollowers(db, "node", nodeID, activity); err != nil {
		t.Fatalf("BroadcastToFollowers: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected no deliveries queued for a private node, got %d", count)
	}

	// Back to public: the surviving follower rows deliver again.
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'public' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("set public: %v", err)
	}
	if err := ap.BroadcastToFollowers(db, "node", nodeID, activity); err != nil {
		t.Fatalf("BroadcastToFollowers after flip back: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected delivery to resume after flipping back to public, got %d queued", count)
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
