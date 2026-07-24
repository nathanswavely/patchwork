package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// The lining is bible (docs/adr/037): kind='lining', pinned public, title
// immutable, body amendable only by proposal, stale copies auto-updated.

func TestLiningBornPublicAndPristine(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "lin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Lining Node", "lining-node", "open")

	handler.CreateDefaultLining(db, nodeID, admin.ID)

	var kind, visibility, body string
	err := db.QueryRow("SELECT kind, visibility, body FROM governance_docs WHERE node_id = ?", nodeID).Scan(&kind, &visibility, &body)
	if err != nil {
		t.Fatalf("lining row not found: %v", err)
	}
	if kind != "lining" {
		t.Errorf("expected kind=lining, got %q", kind)
	}
	if visibility != "public" {
		t.Errorf("expected lining born public, got %q", visibility)
	}
	if governance.LiningStatus(body) != governance.LiningPristine {
		t.Errorf("expected pristine status for fresh lining, got %q", governance.LiningStatus(body))
	}
}

func TestLiningEnforcement(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "lin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Lining Rules", "lining-rules", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	handler.CreateDefaultLining(db, nodeID, admin.ID)

	var docID string
	db.QueryRow("SELECT id FROM governance_docs WHERE node_id = ? AND kind = 'lining'", nodeID).Scan(&docID)

	for name, body := range map[string]map[string]interface{}{
		"retitle":          {"title": "House Rules"},
		"hide":             {"visibility": "members"},
		"direct body edit": {"body": "our own rules"},
	} {
		r := authedRequest("PUT", "/api/v1/governance/"+docID, body, adminToken)
		w := serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", name, w.Code, w.Body.String())
		}
	}

	// Explicitly restating the pinned visibility is a no-op, not an error.
	r := authedRequest("PUT", "/api/v1/governance/"+docID, map[string]interface{}{"visibility": "public"}, adminToken)
	w := serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("visibility=public restatement: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The lining's title can't be claimed by a new charter either.
	r = authedRequest("POST", "/api/v1/nodes/lining-rules/governance",
		map[string]interface{}{"title": handler.DefaultLiningTitle, "body": "impostor"}, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("reserved title: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLiningStatusClassification(t *testing.T) {
	if got := governance.LiningStatus(governance.CurrentLiningBody()); got != governance.LiningPristine {
		t.Errorf("current text: expected pristine, got %q", got)
	}
	if got := governance.LiningStatus("we do what we want"); got != governance.LiningDiverged {
		t.Errorf("custom text: expected diverged, got %q", got)
	}
}

func TestAutoUpdateLinings(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "lin3", "member")

	// A node with no lining at all (bypassed CreateNode).
	bare := createTestNode(t, db, admin.ID, "Bare Node", "bare-node", "open")

	// A node whose lining diverged: must never be touched.
	diverged := createTestNode(t, db, admin.ID, "Diverged Node", "diverged-node", "open")
	handler.CreateDefaultLining(db, diverged, admin.ID)
	db.Exec("UPDATE governance_docs SET body = 'our own rules', version = 3 WHERE node_id = ? AND kind = 'lining'", diverged)

	created, updated, err := handler.AutoUpdateLinings(db)
	if err != nil {
		t.Fatalf("auto-update: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 created (bare node), got %d", created)
	}
	if updated != 0 {
		t.Errorf("expected 0 updated, got %d", updated)
	}

	var body string
	db.QueryRow("SELECT body FROM governance_docs WHERE node_id = ? AND kind = 'lining'", bare).Scan(&body)
	if governance.LiningStatus(body) != governance.LiningPristine {
		t.Errorf("bare node's created lining should be pristine")
	}

	var divergedBody string
	var divergedVersion int
	db.QueryRow("SELECT body, version FROM governance_docs WHERE node_id = ? AND kind = 'lining'", diverged).Scan(&divergedBody, &divergedVersion)
	if divergedBody != "our own rules" || divergedVersion != 3 {
		t.Errorf("diverged lining was touched: body=%q version=%d", divergedBody, divergedVersion)
	}
}

// Unclaimed patches carry no governance at all (docs/adr/039): they are
// outside lining semantics, not a "missing lining" to backfill.
func TestAutoUpdateLiningsSkipsUnclaimedNodes(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "lin5", "member")

	unclaimed := createTestNode(t, db, admin.ID, "Directory Listing", "directory-listing", "open")
	db.Exec("UPDATE nodes SET status = 'unclaimed' WHERE id = ?", unclaimed)

	created, updated, err := handler.AutoUpdateLinings(db)
	if err != nil {
		t.Fatalf("auto-update: %v", err)
	}
	if created != 0 || updated != 0 {
		t.Errorf("expected no linings created or updated for an unclaimed patch, got created=%d updated=%d", created, updated)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM governance_docs WHERE node_id = ?", unclaimed).Scan(&count)
	if count != 0 {
		t.Errorf("unclaimed patch grew a governance doc: %d rows", count)
	}
}

// One rollout, at most one notification per user: a member of two healed
// patches hears once with the count; a member of one keeps the per-patch
// wording and deep link. The doc updates themselves stay per-patch.
func TestAutoUpdateLiningsOneNotificationPerUser(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	shared, _ := createTestUser(t, db, "lin4shared", "member")
	solo, _ := createTestUser(t, db, "lin4solo", "member")

	nodeA := createTestNode(t, db, shared.ID, "Stale A", "stale-a", "open")
	nodeB := createTestNode(t, db, shared.ID, "Stale B", "stale-b", "open")
	createTestMembership(t, db, shared.ID, nodeA, "admin", "active")
	createTestMembership(t, db, shared.ID, nodeB, "admin", "active")
	createTestMembership(t, db, solo.ID, nodeA, "member", "active")

	staleBody := governance.LegacyLiningBodies()[0]
	for _, nodeID := range []string{nodeA, nodeB} {
		handler.CreateDefaultLining(db, nodeID, shared.ID)
		db.Exec("UPDATE governance_docs SET body = ? WHERE node_id = ? AND kind = 'lining'", staleBody, nodeID)
	}

	_, updated, err := handler.AutoUpdateLinings(db)
	if err != nil {
		t.Fatalf("auto-update: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected 2 updated linings, got %d", updated)
	}

	var sharedCount int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ?",
		shared.ID, string(notifications.LiningUpdated)).Scan(&sharedCount)
	if sharedCount != 1 {
		t.Fatalf("member of 2 healed patches: expected 1 notification, got %d", sharedCount)
	}
	var sharedTitle, sharedBody string
	db.QueryRow("SELECT title, body FROM notifications WHERE user_id = ?", shared.ID).Scan(&sharedTitle, &sharedBody)
	if sharedTitle != "The lining was updated across 2 of your patches" {
		t.Errorf("coalesced title = %q", sharedTitle)
	}
	if !strings.Contains(sharedBody, "Stale A") || !strings.Contains(sharedBody, "Stale B") {
		t.Errorf("coalesced body should name both patches, got %q", sharedBody)
	}

	var soloCount int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ?",
		solo.ID, string(notifications.LiningUpdated)).Scan(&soloCount)
	if soloCount != 1 {
		t.Fatalf("member of 1 healed patch: expected 1 notification, got %d", soloCount)
	}
	var soloTitle, soloLink string
	db.QueryRow("SELECT title, link FROM notifications WHERE user_id = ?", solo.ID).Scan(&soloTitle, &soloLink)
	if soloTitle != "The lining was updated" {
		t.Errorf("single-patch title = %q", soloTitle)
	}
	if !strings.HasPrefix(soloLink, "/patches/stale-a/governance/docs/") {
		t.Errorf("single-patch notification should deep-link to the doc, got %q", soloLink)
	}
}
