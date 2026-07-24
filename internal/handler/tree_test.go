package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func TestNodeTree(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "tree-admin", "member")

	nodeA := createTestNode(t, db, admin.ID, "Patch A", "patch-a", "open")
	nodeB := createTestNode(t, db, admin.ID, "Patch B", "patch-b", "open")

	createTestMembership(t, db, admin.ID, nodeA, "admin", "active")
	createTestMembership(t, db, admin.ID, nodeB, "member", "active")

	// A follower must appear in follower_count but never in member_count —
	// followers are observers, not members.
	fan, _ := createTestUser(t, db, "tree-fan", "member")
	createTestMembership(t, db, fan.ID, nodeA, "follower", "active")

	eventID := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at, visibility) VALUES (?, ?, ?, 'Test Event', 'desc', 'here', '2026-04-01T10:00:00Z', 'public')`,
		eventID, nodeA, admin.ID,
	)

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tree struct {
			ID       string `json:"id"`
			Children []struct {
				ID            string `json:"id"`
				MemberCount   int    `json:"member_count"`
				FollowerCount int    `json:"follower_count"`
				EventCount    int    `json:"event_count"`
			} `json:"children"`
		} `json:"tree"`
		Affinity []struct {
			Source   string `json:"source"`
			Target   string `json:"target"`
			Strength int    `json:"strength"`
		} `json:"affinity"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Tree.ID != "root" {
		t.Errorf("expected root id, got %s", resp.Tree.ID)
	}
	if len(resp.Tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(resp.Tree.Children))
	}

	for _, child := range resp.Tree.Children {
		if child.ID == nodeA {
			if child.MemberCount != 1 {
				t.Errorf("expected member_count=1 for patch A (follower must not count), got %d", child.MemberCount)
			}
			if child.FollowerCount != 1 {
				t.Errorf("expected follower_count=1 for patch A, got %d", child.FollowerCount)
			}
			if child.EventCount != 1 {
				t.Errorf("expected event_count=1 for patch A, got %d", child.EventCount)
			}
		}
	}

	// Admin is member of both patches, so there should be affinity between them.
	if len(resp.Affinity) == 0 {
		t.Error("expected at least one affinity link (shared member)")
	}
	if len(resp.Affinity) > 0 {
		link := resp.Affinity[0]
		if link.Strength <= 0 {
			t.Errorf("expected positive affinity strength, got %d", link.Strength)
		}
	}
}

// TestNodeTreeTagAffinity covers the shared-tag placement term
// (docs/adr/021): declared similarity attracts patches with no people
// overlap, gravitates thin patches toward the biggest patch sharing their
// tags, and never outweighs a single shared member (weight 3).
func TestNodeTreeTagAffinity(t *testing.T) {
	db := setupTestDB(t)
	ownerA, _ := createTestUser(t, db, "tag-owner-a", "member")
	ownerB, _ := createTestUser(t, db, "tag-owner-b", "member")
	ownerC, _ := createTestUser(t, db, "tag-owner-c", "member")

	// Three patches with distinct owners: no shared people anywhere.
	thin := createTestNode(t, db, ownerA.ID, "Thin Band", "thin-band", "open")
	big := createTestNode(t, db, ownerB.ID, "Big Venue", "big-venue", "open")
	small := createTestNode(t, db, ownerC.ID, "Small Band", "small-band", "open")

	// Big Venue has two members; the others have none.
	createTestMembership(t, db, ownerB.ID, big, "admin", "active")
	member2, _ := createTestUser(t, db, "tag-member-2", "member")
	createTestMembership(t, db, member2.ID, big, "member", "active")

	// All three wear 'music'.
	tagID := auth.NewUUIDv7()
	db.Exec(`INSERT INTO tags (id, name, motif) VALUES (?, 'music', 'musicNotes')`, tagID)
	for _, nodeID := range []string{thin, big, small} {
		db.Exec(`INSERT INTO node_tags (node_id, tag_id, position) VALUES (?, ?, 0)`, nodeID, tagID)
	}

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Affinity []struct {
			Source   string  `json:"source"`
			Target   string  `json:"target"`
			Strength float64 `json:"strength"`
		} `json:"affinity"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	strength := func(a, b string) float64 {
		for _, l := range resp.Affinity {
			if (l.Source == a && l.Target == b) || (l.Source == b && l.Target == a) {
				return l.Strength
			}
		}
		return 0
	}

	thinBig := strength(thin, big)
	thinSmall := strength(thin, small)

	if thinBig <= 0 || thinSmall <= 0 {
		t.Fatalf("expected tag links between all music patches, got thin-big=%v thin-small=%v", thinBig, thinSmall)
	}
	// Gravitation: the thin patch is pulled harder toward the big venue
	// than toward the other thin band.
	if thinBig <= thinSmall {
		t.Errorf("expected mass gravitation (thin-big > thin-small), got thin-big=%v thin-small=%v", thinBig, thinSmall)
	}
	// Declared similarity never outweighs one shared human (weight 3).
	for _, l := range resp.Affinity {
		if l.Strength >= 3 {
			t.Errorf("tag link %s-%s strength %v must stay below one shared member (3)", l.Source, l.Target, l.Strength)
		}
	}
}

func TestNodeTreeEmpty(t *testing.T) {
	db := setupTestDB(t)

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tree struct {
			ID       string        `json:"id"`
			Children []interface{} `json:"children"`
		} `json:"tree"`
		Affinity []interface{} `json:"affinity"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Tree.ID != "root" {
		t.Errorf("expected root id")
	}
	if len(resp.Tree.Children) != 0 {
		t.Errorf("expected empty children, got %d", len(resp.Tree.Children))
	}
}
