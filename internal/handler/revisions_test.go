package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func TestCreateRevision(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "r_author1", "member")
	nodeID := createTestNode(t, db, author.ID, "Rev Node", "rev-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, author.ID)

	body := map[string]interface{}{
		"title":       "Updated Title",
		"body":        "Updated body text",
		"change_note": "Revised the wording",
	}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/revisions", body, authorToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/revisions", handler.CreateRevision(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["title"] != "Updated Title" {
		t.Errorf("expected title='Updated Title', got %v", result["title"])
	}
	if result["revision_number"].(float64) != 1 {
		t.Errorf("expected revision_number=1, got %v", result["revision_number"])
	}

	// Verify the proposal was updated.
	var title string
	db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&title)
	if title != "Updated Title" {
		t.Errorf("expected proposal title='Updated Title', got %s", title)
	}

	// Verify revision was stored.
	var revCount int
	db.QueryRow("SELECT COUNT(*) FROM proposal_revisions WHERE proposal_id = ?", proposalID).Scan(&revCount)
	if revCount != 1 {
		t.Errorf("expected 1 revision, got %d", revCount)
	}
}

func TestCreateRevision_AuthorOnly(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "r_author2", "member")
	other, otherToken := createTestUser(t, db, "r_other2", "member")
	nodeID := createTestNode(t, db, author.ID, "RevAuth Node", "revauth-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	proposalID := createTestProposal(t, db, nodeID, author.ID)

	body := map[string]interface{}{
		"title":       "Sneaky Update",
		"change_note": "Trying to revise someone else's proposal",
	}

	// Non-author should get 403.
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/revisions", body, otherToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/revisions", handler.CreateRevision(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	// Author should succeed.
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/revisions", body, authorToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/revisions", handler.CreateRevision(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRevisions(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "r_author3", "member")
	nodeID := createTestNode(t, db, author.ID, "ListRev Node", "listrev-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, author.ID)

	// Create 2 revisions.
	for i, note := range []string{"First revision", "Second revision"} {
		body := map[string]interface{}{
			"title":       "Title v" + string(rune('1'+i)),
			"change_note": note,
		}
		r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/revisions", body, authorToken)
		w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/revisions", handler.CreateRevision(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for revision %d, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	// List revisions.
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID+"/revisions", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}/revisions", handler.ListRevisions(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(items))
	}

	// Verify ordering (revision_number ASC).
	first := items[0].(map[string]interface{})
	second := items[1].(map[string]interface{})
	if first["revision_number"].(float64) != 1 {
		t.Errorf("expected first revision_number=1, got %v", first["revision_number"])
	}
	if second["revision_number"].(float64) != 2 {
		t.Errorf("expected second revision_number=2, got %v", second["revision_number"])
	}
	if first["change_note"] != "First revision" {
		t.Errorf("expected first change_note='First revision', got %v", first["change_note"])
	}
	if second["change_note"] != "Second revision" {
		t.Errorf("expected second change_note='Second revision', got %v", second["change_note"])
	}
}
