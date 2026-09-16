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

// Suggested tags (docs/adr/114). A patch admin proposes a word, the instance
// admin decides, and until then the word is not vocabulary.

func suggestTag(t *testing.T, db *database.DB, nodeID, userID, name string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO tags (id, name, status, suggested_by) VALUES (?, ?, 'pending', ?)`,
		id, name, userID,
	); err != nil {
		t.Fatalf("suggest tag %s: %v", name, err)
	}
	if _, err := db.Exec(
		`INSERT INTO node_tags (node_id, tag_id, position) VALUES (?, ?, 0)`, nodeID, id,
	); err != nil {
		t.Fatalf("attach suggested tag: %v", err)
	}
	return id
}

func decideSuggestion(t *testing.T, db *database.DB, token, tagID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/admin/tag-suggestions/"+tagID, body, token)
	return serveMux(t, db, "PATCH", "/api/v1/admin/tag-suggestions/{id}", handler.DecideTagSuggestion(db), r)
}

// A pending word is not the vocabulary. The tag list is unauthenticated, so
// this is a disclosure boundary rather than presentation.
func TestSuggestedTagIsNotInThePublicVocabulary(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "sugg-owner", "member")
	node := createTestNode(t, db, owner.ID, "Selvage", "sugg-selvage", "open")

	createTestTag(t, db, "music")
	suggestTag(t, db, node, owner.ID, "zine")

	rec := servePublicMux(t, "GET", "/api/v1/tags", handler.ListTags(db),
		httptest.NewRequest("GET", "/api/v1/tags", nil))

	var tags []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &tags)
	for _, tag := range tags {
		if tag["name"] == "zine" {
			t.Fatal("a suggested tag appeared in the public vocabulary")
		}
	}
	if len(tags) != 1 {
		t.Fatalf("expected only the approved tag, got %d", len(tags))
	}
}

// An unknown name in `tags` stays a 400 so a typo never coins a word. This is
// the reason suggestions travel on their own field.
func TestUnknownNameInTagsIsStillRejected(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "sugg-typo", "member")
	node := createTestNode(t, db, owner.ID, "Typo", "sugg-typo-patch", "open")
	suggestTag(t, db, node, owner.ID, "zine")

	// Even the pending word itself does not resolve as a pick.
	if ids, unknown := handler.ResolveTagIDsForTest(db, []string{"zine"}); unknown != "zine" || ids != nil {
		t.Fatalf("a pending word resolved as vocabulary: ids=%v unknown=%q", ids, unknown)
	}
}

// The failure this design exists to prevent: a settings form that loads
// node.tags and PATCHes it back must not delete the patch's own suggestion.
func TestWholesaleTagReplaceKeepsSuggestions(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "sugg-keep", "member")
	node := createTestNode(t, db, owner.ID, "Keeper", "sugg-keeper", "open")

	musicID := createTestTag(t, db, "music")
	craftID := createTestTag(t, db, "craft")
	attachTestTag(t, db, node, musicID)
	suggestTag(t, db, node, owner.ID, "zine")

	// The form replaces the approved list wholesale, knowing nothing about
	// the suggestion.
	if err := handler.SetNodeTagsForTest(db, node, []string{craftID}); err != nil {
		t.Fatalf("set tags: %v", err)
	}

	var pending int
	db.QueryRow(`SELECT COUNT(*) FROM node_tags nt JOIN tags t ON t.id = nt.tag_id
	             WHERE nt.node_id = ? AND t.status = 'pending'`, node).Scan(&pending)
	if pending != 1 {
		t.Fatalf("a wholesale tags replace destroyed the patch's suggestion (pending=%d)", pending)
	}

	var approved int
	db.QueryRow(`SELECT COUNT(*) FROM node_tags nt JOIN tags t ON t.id = nt.tag_id
	             WHERE nt.node_id = ? AND t.status = 'approved'`, node).Scan(&approved)
	if approved != 1 {
		t.Fatalf("expected the replace to leave exactly the new approved tag, got %d", approved)
	}
}

// Approving publishes the word on every patch wearing it, including one whose
// admin was never asked: one vocabulary, one decision.
func TestApprovingPublishesOnEveryWearingPatch(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "sugg-admin", "admin")
	one, _ := createTestUser(t, db, "sugg-one", "member")
	nodeA := createTestNode(t, db, one.ID, "A", "sugg-a", "open")
	nodeB := createTestNode(t, db, one.ID, "B", "sugg-b", "open")

	tagID := suggestTag(t, db, nodeA, one.ID, "zine")
	db.Exec(`INSERT INTO node_tags (node_id, tag_id, position) VALUES (?, ?, 0)`, nodeB, tagID)

	rec := decideSuggestion(t, db, adminToken, tagID, map[string]any{"action": "approve"})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}

	var status string
	db.QueryRow("SELECT status FROM tags WHERE id = ?", tagID).Scan(&status)
	if status != "approved" {
		t.Fatalf("status = %q", status)
	}
	for _, n := range []string{nodeA, nodeB} {
		var count int
		db.QueryRow(`SELECT COUNT(*) FROM node_tags WHERE node_id = ? AND tag_id = ?`, n, tagID).Scan(&count)
		if count != 1 {
			t.Fatalf("patch %s lost the word on approval", n)
		}
	}
}

// Rejecting spends the word: the row stays so the same suggestion cannot come
// back, and the attachments go because the word does not exist for patches.
func TestRejectingSpendsTheWord(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "rej-admin", "admin")
	owner, _ := createTestUser(t, db, "rej-owner", "member")
	node := createTestNode(t, db, owner.ID, "R", "rej-patch", "open")
	tagID := suggestTag(t, db, node, owner.ID, "zine")

	if rec := decideSuggestion(t, db, adminToken, tagID, map[string]any{"action": "reject"}); rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body.String())
	}

	var status string
	if err := db.QueryRow("SELECT status FROM tags WHERE id = ?", tagID).Scan(&status); err != nil {
		t.Fatalf("the rejected row was deleted, so the name is free again: %v", err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q", status)
	}
	var attached int
	db.QueryRow("SELECT COUNT(*) FROM node_tags WHERE tag_id = ?", tagID).Scan(&attached)
	if attached != 0 {
		t.Fatalf("a rejected word is still on %d patches", attached)
	}

	// Suggesting it again is refused by name, not by a generic conflict.
	_, errMsg := handler.SuggestTagsForNodeForTest(db, node, owner.ID, []string{"zine"})
	if !strings.Contains(errMsg, "declined") || !strings.Contains(errMsg, "zine") {
		t.Fatalf("re-suggestion error did not name the word: %q", errMsg)
	}
}

// The admin's own Add tag form resurrects a declined word, so the vocabulary
// page never disagrees with the UNIQUE constraint.
func TestCreateTagResurrectsARejectedName(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "res-admin", "admin")
	owner, _ := createTestUser(t, db, "res-owner", "member")
	node := createTestNode(t, db, owner.ID, "R", "res-patch", "open")
	tagID := suggestTag(t, db, node, owner.ID, "zine")
	decideSuggestion(t, db, adminToken, tagID, map[string]any{"action": "reject"})

	r := authedRequest("POST", "/api/v1/admin/tags", map[string]any{"name": "zine"}, adminToken)
	rec := serveMux(t, db, "POST", "/api/v1/admin/tags", handler.CreateTag(db), r)
	if rec.Code == http.StatusConflict {
		t.Fatal("Add tag returned 409 for a word absent from the vocabulary page")
	}

	var status string
	db.QueryRow("SELECT status FROM tags WHERE name = ? COLLATE NOCASE", "zine").Scan(&status)
	if status != "approved" {
		t.Fatalf("status after resurrect = %q", status)
	}
}

// Approving with a new name merges onto the existing word rather than
// creating a second row, and does not duplicate the chip on a patch that
// already wore the target.
func TestApproveWithRenameMergesOntoExistingTag(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "mrg-admin", "admin")
	owner, _ := createTestUser(t, db, "mrg-owner", "member")
	node := createTestNode(t, db, owner.ID, "M", "mrg-patch", "open")

	existing := createTestTag(t, db, "zine")
	attachTestTag(t, db, node, existing)
	pendingID := suggestTag(t, db, node, owner.ID, "zines")

	rec := decideSuggestion(t, db, adminToken, pendingID, map[string]any{"action": "approve", "name": "Zine"})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve+rename: %d %s", rec.Code, rec.Body.String())
	}

	var rows int
	db.QueryRow("SELECT COUNT(*) FROM tags WHERE name = ? COLLATE NOCASE", "zine").Scan(&rows)
	if rows != 1 {
		t.Fatalf("merge left %d rows named zine", rows)
	}
	var gone int
	db.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", pendingID).Scan(&gone)
	if gone != 0 {
		t.Fatal("the merged-away suggestion row survived")
	}
	var chips int
	db.QueryRow("SELECT COUNT(*) FROM node_tags WHERE node_id = ? AND tag_id = ?", node, existing).Scan(&chips)
	if chips != 1 {
		t.Fatalf("patch wears the merged tag %d times", chips)
	}
}

// A proposed name that is already approved vocabulary is an ordinary pick, so
// it never reaches the queue.
func TestSuggestingAnExistingWordJustAttachesIt(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "exist-owner", "member")
	node := createTestNode(t, db, owner.ID, "E", "exist-patch", "open")
	musicID := createTestTag(t, db, "music")

	coined, errMsg := handler.SuggestTagsForNodeForTest(db, node, owner.ID, []string{"  Music  "})
	if errMsg != "" {
		t.Fatalf("suggest: %s", errMsg)
	}
	if coined != 0 {
		t.Fatalf("an existing word created %d queue entries", coined)
	}
	var attached int
	db.QueryRow("SELECT COUNT(*) FROM node_tags WHERE node_id = ? AND tag_id = ?", node, musicID).Scan(&attached)
	if attached != 1 {
		t.Fatal("the existing tag was not attached")
	}
}

func TestNormalizeTagName(t *testing.T) {
	cases := map[string]string{
		"  Live Music  ": "live-music",
		"Zine":           "zine",
		"visual   arts":  "visual-arts",
		"café":           "café",
		"tag!!!":         "tag",
		"--spaced--":     "spaced",
		"!!!":            "",
		"":               "",
		strings.Repeat("a", 40): strings.Repeat("a", 32),
	}
	for in, want := range cases {
		if got := handler.NormalizeTagNameForTest(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
