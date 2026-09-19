package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The verified atproto handle on the patch page (docs/adr/062, amended).
// `nodes.did` is written by exactly one path — a claim that passed the
// bidirectional check — and read by exactly one: the patch's own detail
// response, which is where the handle is shown.

func TestGetNodeCarriesTheVerifiedDID(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "tellus-admin", "member")
	nodeID := createTestNode(t, db, owner.ID, "Tellus", "tellus", "open")

	// Unverified is the default, and it must not reach the page as an
	// empty handle: `omitempty` keeps the key out entirely, so the profile
	// has nothing to render rather than an `@` with no domain after it.
	r := authedRequest("GET", "/api/v1/nodes/tellus", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("GetNode: %d %s", w.Code, w.Body.String())
	}
	var raw struct {
		Node map[string]interface{} `json:"node"`
	}
	json.Unmarshal(w.Body.Bytes(), &raw)
	if _, ok := raw.Node["did"]; ok {
		t.Errorf("a patch that verified no handle sent a did key: %v", raw.Node["did"])
	}

	// Set the way a verified claim sets it (internal/handler/claims.go).
	if _, err := db.Exec("UPDATE nodes SET did = ? WHERE id = ?", "did:web:tellus.example", nodeID); err != nil {
		t.Fatalf("set did: %v", err)
	}

	r = authedRequest("GET", "/api/v1/nodes/tellus", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	var detail struct {
		Node model.Node `json:"node"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Node.DID != "did:web:tellus.example" {
		t.Errorf("GetNode did: %q", detail.Node.DID)
	}
	// The rest of the row still scans — a column inserted into a positional
	// SELECT is the mistake this guards.
	if detail.Node.Name != "Tellus" || detail.Node.Slug != "tellus" {
		t.Errorf("scan drifted: name=%q slug=%q", detail.Node.Name, detail.Node.Slug)
	}
}

// The handle is shown, not editable. A patch admin can rewrite most of the
// profile from settings; the one fact on it that was proved rather than
// typed must not be reachable that way, or the proof means nothing.
func TestPatchCannotSetItsOwnDID(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "did-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Selvage", "selvage", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	w := patchNode(t, db, "selvage", adminToken, map[string]interface{}{
		"did":         "did:web:not-mine.example",
		"description": "still writable",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH node: %d %s", w.Code, w.Body.String())
	}
	var stored string
	db.QueryRow("SELECT COALESCE(did,'') FROM nodes WHERE id = ?", nodeID).Scan(&stored)
	if stored != "" {
		t.Errorf("PATCH wrote a did: %q", stored)
	}
	var desc string
	db.QueryRow("SELECT description FROM nodes WHERE id = ?", nodeID).Scan(&desc)
	if desc != "still writable" {
		t.Errorf("the same PATCH should still have applied its legal field, got %q", desc)
	}
}
