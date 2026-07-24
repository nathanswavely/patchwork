package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func createTestTag(t *testing.T, db *database.DB, name string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO tags (id, name) VALUES (?, ?)`, id, name); err != nil {
		t.Fatalf("create tag %s: %v", name, err)
	}
	return id
}

func attachTestTag(t *testing.T, db *database.DB, nodeID, tagID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO node_tags (node_id, tag_id) VALUES (?, ?)`, nodeID, tagID); err != nil {
		t.Fatalf("attach tag: %v", err)
	}
}

func TestListTagsNodeCounts(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "tags-admin", "member")

	nodeA := createTestNode(t, db, admin.ID, "Patch A", "tags-patch-a", "open")
	nodeB := createTestNode(t, db, admin.ID, "Patch B", "tags-patch-b", "open")

	// A private patch and a removed patch must not count — the tags
	// endpoint is public and follows the public tree's visibility.
	private := createTestNode(t, db, admin.ID, "Private", "tags-private", "open")
	db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, private)
	removed := createTestNode(t, db, admin.ID, "Removed", "tags-removed", "open")
	db.Exec(`UPDATE nodes SET removed_at = '2026-01-01T00:00:00Z' WHERE id = ?`, removed)

	music := createTestTag(t, db, "music")
	craft := createTestTag(t, db, "craft")
	createTestTag(t, db, "unworn")

	attachTestTag(t, db, nodeA, music)
	attachTestTag(t, db, nodeB, music)
	attachTestTag(t, db, private, music)
	attachTestTag(t, db, removed, music)
	attachTestTag(t, db, nodeA, craft)

	r := httptest.NewRequest("GET", "/api/v1/tags", nil)
	w := servePublicMux(t, "GET", "/api/v1/tags", handler.ListTags(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tags []struct {
		Name      string `json:"name"`
		NodeCount int    `json:"node_count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&tags); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	counts := map[string]int{}
	for _, tag := range tags {
		counts[tag.Name] = tag.NodeCount
	}
	if counts["music"] != 2 {
		t.Errorf("expected music node_count=2 (private and removed patches excluded), got %d", counts["music"])
	}
	if counts["craft"] != 1 {
		t.Errorf("expected craft node_count=1, got %d", counts["craft"])
	}
	if got, ok := counts["unworn"]; !ok || got != 0 {
		t.Errorf("expected unworn tag present with node_count=0, got %d (present=%v)", got, ok)
	}

	// Order stays alphabetical — the sidebar filter list depends on it.
	for i := 1; i < len(tags); i++ {
		if tags[i-1].Name > tags[i].Name {
			t.Errorf("tags not sorted by name: %s before %s", tags[i-1].Name, tags[i].Name)
		}
	}
}
