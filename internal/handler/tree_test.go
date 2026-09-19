package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
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

// The tree carries membership_policy because a docked profile's head is
// rendered from this row (docs/adr/094), and the relationship row offers
// the next rung only when the policy is not invite_only. Seeded from a row
// missing the field, the head would offer "Become a member" on an
// invite-only patch while the same profile's page offers nothing — one
// sheet's two heights disagreeing about one patch. Same reason
// docs/adr/090 put moved_to here.
func TestNodeTreeCarriesMembershipPolicy(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "policy-admin", "member")

	open := createTestNode(t, db, admin.ID, "Open Patch", "open-patch", "open")
	closed := createTestNode(t, db, admin.ID, "Band", "band", "invite_only")

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tree struct {
			Children []struct {
				ID               string `json:"id"`
				MembershipPolicy string `json:"membership_policy"`
			} `json:"children"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := map[string]string{}
	for _, child := range resp.Tree.Children {
		got[child.ID] = child.MembershipPolicy
	}
	if got[open] != "open" {
		t.Errorf("expected membership_policy=open, got %q", got[open])
	}
	if got[closed] != "invite_only" {
		t.Errorf("expected membership_policy=invite_only, got %q", got[closed])
	}
}

// treeAffinity fetches the tree and returns the affinity strength between
// two node ids (0 if no link exists).
func treeAffinity(t *testing.T, db *database.DB, a, b string) float64 {
	t.Helper()
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
	for _, l := range resp.Affinity {
		if (l.Source == a && l.Target == b) || (l.Source == b && l.Target == a) {
			return l.Strength
		}
	}
	return 0
}

// A membership the member has hidden (memberships.visible = 0, docs/adr/006)
// must not count toward placement affinity
// (docs/adr/2026-09-18-a-hidden-membership-does-not-place.md): an
// unnormalized public score would otherwise move by a fixed, identifying
// amount when a specific person's hidden row is the only thing that changed.
func TestNodeTreeHiddenMembershipDoesNotAffinity(t *testing.T) {
	db := setupTestDB(t)
	ownerA, _ := createTestUser(t, db, "hidden-owner-a", "member")
	ownerB, _ := createTestUser(t, db, "hidden-owner-b", "member")
	shared, _ := createTestUser(t, db, "hidden-shared-member", "member")

	nodeA := createTestNode(t, db, ownerA.ID, "Hidden A", "hidden-a", "open")
	nodeB := createTestNode(t, db, ownerB.ID, "Hidden B", "hidden-b", "open")

	createTestMembership(t, db, ownerA.ID, nodeA, "admin", "active")
	createTestMembership(t, db, ownerB.ID, nodeB, "admin", "active")

	// Shared member of both patches, hidden on A.
	createTestMembership(t, db, shared.ID, nodeA, "member", "active")
	createTestMembership(t, db, shared.ID, nodeB, "member", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE user_id = ? AND node_id = ?`, shared.ID, nodeA); err != nil {
		t.Fatalf("hide membership: %v", err)
	}

	if got := treeAffinity(t, db, nodeA, nodeB); got != 0 {
		t.Errorf("expected no member-affinity link while the shared membership is hidden on one side, got strength %v", got)
	}

	// Made visible again: the link reappears at full strength (3).
	if _, err := db.Exec(`UPDATE memberships SET visible = 1 WHERE user_id = ? AND node_id = ?`, shared.ID, nodeA); err != nil {
		t.Fatalf("show membership: %v", err)
	}
	if got := treeAffinity(t, db, nodeA, nodeB); got != 3 {
		t.Errorf("expected member-affinity link of strength 3 once both memberships are visible, got %v", got)
	}
}

// Same rule for a shared follower (weight 1): hidden on one side, the
// follower-affinity term must not count it either.
func TestNodeTreeHiddenFollowerDoesNotAffinity(t *testing.T) {
	db := setupTestDB(t)
	ownerA, _ := createTestUser(t, db, "hidden-fol-owner-a", "member")
	ownerB, _ := createTestUser(t, db, "hidden-fol-owner-b", "member")
	fan, _ := createTestUser(t, db, "hidden-fol-fan", "member")

	nodeA := createTestNode(t, db, ownerA.ID, "Hidden Fol A", "hidden-fol-a", "open")
	nodeB := createTestNode(t, db, ownerB.ID, "Hidden Fol B", "hidden-fol-b", "open")

	createTestMembership(t, db, ownerA.ID, nodeA, "admin", "active")
	createTestMembership(t, db, ownerB.ID, nodeB, "admin", "active")

	createTestMembership(t, db, fan.ID, nodeA, "follower", "active")
	createTestMembership(t, db, fan.ID, nodeB, "follower", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE user_id = ? AND node_id = ?`, fan.ID, nodeA); err != nil {
		t.Fatalf("hide follower membership: %v", err)
	}

	if got := treeAffinity(t, db, nodeA, nodeB); got != 0 {
		t.Errorf("expected no follower-affinity link while the shared follower is hidden on one side, got strength %v", got)
	}

	if _, err := db.Exec(`UPDATE memberships SET visible = 1 WHERE user_id = ? AND node_id = ?`, fan.ID, nodeA); err != nil {
		t.Fatalf("show follower membership: %v", err)
	}
	if got := treeAffinity(t, db, nodeA, nodeB); got != 1 {
		t.Errorf("expected follower-affinity link of strength 1 once both memberships are visible, got %v", got)
	}
}

// TestNodeTreeExcludesPrivatePatchAffinity is the regression test for the
// affinity leak: the shared-member query behind the "affinity" array wasn't
// scoped to public/active-or-unclaimed nodes the way the node list itself
// is, and the links it produced were never intersected with the node set
// the response actually returns. A private patch's id (and hence, since ids
// are UUIDv7, its creation time) and its overlap score with a public patch
// could reach an unauthenticated caller through "affinity" even though the
// private patch never appeared in "tree". Asserting the raw body never
// contains the private id covers both surfaces at once.
func TestNodeTreeExcludesPrivatePatchAffinity(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "leak-admin-private", "member")

	publicNode := createTestNode(t, db, admin.ID, "Leak Public", "leak-public", "open")
	privateNode := createTestNode(t, db, admin.ID, "Leak Private", "leak-private", "open")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, privateNode); err != nil {
		t.Fatalf("make node private: %v", err)
	}

	// Same admin on both patches — before the fix this alone was enough to
	// score and serialize a link naming the private patch.
	createTestMembership(t, db, admin.ID, publicNode, "admin", "active")
	createTestMembership(t, db, admin.ID, privateNode, "admin", "active")

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, privateNode) {
		t.Errorf("private patch id %s leaked into the public tree response: %s", privateNode, body)
	}
	if !strings.Contains(body, publicNode) {
		t.Errorf("expected public patch id %s in response: %s", publicNode, body)
	}
}

// TestNodeTreeExcludesRemovedPatchAffinity is the same leak, for a
// soft-deleted patch (nodes.removed_at set) rather than a private one.
func TestNodeTreeExcludesRemovedPatchAffinity(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "leak-admin-removed", "member")

	publicNode := createTestNode(t, db, admin.ID, "Leak Public Two", "leak-public-2", "open")
	removedNode := createTestNode(t, db, admin.ID, "Leak Removed", "leak-removed", "open")
	if _, err := db.Exec(`UPDATE nodes SET removed_at = '2026-01-01T00:00:00Z' WHERE id = ?`, removedNode); err != nil {
		t.Fatalf("mark node removed: %v", err)
	}

	createTestMembership(t, db, admin.ID, publicNode, "admin", "active")
	createTestMembership(t, db, admin.ID, removedNode, "admin", "active")

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, removedNode) {
		t.Errorf("removed patch id %s leaked into the public tree response: %s", removedNode, body)
	}
	if !strings.Contains(body, publicNode) {
		t.Errorf("expected public patch id %s in response: %s", publicNode, body)
	}
}

// TestNodeTreePublicPublicAffinityStillLinks guards against an overcorrection:
// two public, active patches sharing a member must still produce a link.
func TestNodeTreePublicPublicAffinityStillLinks(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "leak-admin-both-public", "member")

	nodeA := createTestNode(t, db, admin.ID, "Both Public A", "both-public-a", "open")
	nodeB := createTestNode(t, db, admin.ID, "Both Public B", "both-public-b", "open")

	createTestMembership(t, db, admin.ID, nodeA, "admin", "active")
	createTestMembership(t, db, admin.ID, nodeB, "member", "active")

	r := httptest.NewRequest("GET", "/api/v1/nodes/tree", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Affinity []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"affinity"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	found := false
	for _, l := range resp.Affinity {
		if (l.Source == nodeA && l.Target == nodeB) || (l.Source == nodeB && l.Target == nodeA) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a public-public affinity link between %s and %s, got %+v", nodeA, nodeB, resp.Affinity)
	}
}
