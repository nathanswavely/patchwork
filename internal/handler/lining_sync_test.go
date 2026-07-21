package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// TestApplyAmendment_SyncsLiningToDB covers docs/adr/011 end to end: the DB
// governance_docs row is the canonical lining, git is its mirror, and
// applying an amendment must land the merged content back in the DB row —
// otherwise the hub never shows what the community voted in.
func TestApplyAmendment_SyncsLiningToDB(t *testing.T) {
	db := setupTestDB(t)

	// Real git repos for this test: the write-back reads merged content
	// from git HEAD. Restore the package-global dataDir afterward so other
	// tests keep their no-git best-effort behavior.
	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })
	if err := governance.InitInstanceRepo(tmp); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}

	admin, adminToken := createTestUser(t, db, "liningadmin", "member")

	// Create the node through the handler so both halves of the lining
	// identity are born: the governance_docs row and the forked
	// community-standards.md.
	r := authedRequest("POST", "/api/v1/nodes", map[string]string{"name": "Lining Sync Node"}, adminToken)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create node: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	nodeID := node["id"].(string)

	// One identity at birth: same title-derived filename, same body verbatim.
	var title, body string
	var version int
	if err := db.QueryRow(
		`SELECT title, body, version FROM governance_docs WHERE node_id = ?`, nodeID,
	).Scan(&title, &body, &version); err != nil {
		t.Fatalf("default lining row missing: %v", err)
	}
	if title != handler.DefaultLiningTitle {
		t.Fatalf("expected default lining title %q, got %q", handler.DefaultLiningTitle, title)
	}
	gitBody, err := governance.GetDocument(tmp, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("forked community-standards.md missing: %v", err)
	}
	if gitBody != body {
		t.Errorf("DB body and git file must match at birth (adr/011); DB %d bytes, git %d bytes", len(body), len(gitBody))
	}

	// Amend: a branch carrying new content, an approved amendment proposal
	// targeting the lining's file, then the admin applies it.
	amended := body + "\n\n## Land Acknowledgment\n\nWe gather on Susquehannock land.\n"
	if _, err := governance.CreateBranch(tmp, nodeID, "amendment-test", "community-standards.md", amended,
		"Tester", "tester@example.com", "Proposed: land acknowledgment"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	propID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type, target_doc, proposed_branch, proposed_body, duration_hours)
		 VALUES (?, ?, ?, 'Add land acknowledgment', 'Recognize the Susquehannock people in our lining.', 'approved', 'approved', 'amendment', 'community-standards.md', 'amendment-test', ?, 168)`,
		propID, nodeID, admin.ID, amended,
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}

	r = authedRequest("POST", "/api/v1/proposals/"+propID+"/apply", nil, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/apply", handler.ApplyProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("apply: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The canonical store reflects the merge: amended body, bumped version,
	// title (the filename identity) untouched.
	var newTitle, newBody string
	var newVersion int
	db.QueryRow(
		`SELECT title, body, version FROM governance_docs WHERE node_id = ?`, nodeID,
	).Scan(&newTitle, &newBody, &newVersion)
	if newBody != amended {
		t.Errorf("governance_docs body not synced from merged amendment; got %d bytes, want %d", len(newBody), len(amended))
	}
	if newVersion != version+1 {
		t.Errorf("expected version %d after sync, got %d", version+1, newVersion)
	}
	if newTitle != title {
		t.Errorf("title must not change on body amendment (filename identity); got %q", newTitle)
	}

	// And git agrees, as the mirror should.
	mergedGit, err := governance.GetDocument(tmp, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	if mergedGit != newBody {
		t.Errorf("git file and DB body diverged after apply")
	}
}
