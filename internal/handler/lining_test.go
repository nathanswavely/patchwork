package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
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
