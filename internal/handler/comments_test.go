package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func createTestProposal(t *testing.T, db *database.DB, nodeID, authorID string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours) VALUES (?, ?, ?, 'Test Proposal', 'Body', 'open', 'action', 72)`,
		id, nodeID, authorID,
	)
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	return id
}

func TestCreateComment(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "c_admin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Comment Node", "comment-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create a proposal to comment on.
	proposalID := createTestProposal(t, db, nodeID, admin.ID)

	body := map[string]interface{}{
		"body": "This is a test comment",
	}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["body"] != "This is a test comment" {
		t.Errorf("expected body='This is a test comment', got %v", result["body"])
	}
	if result["id"] == nil || result["id"] == "" {
		t.Error("expected id to be set")
	}
	if result["author_name"] == nil || result["author_name"] == "" {
		t.Error("expected author_name to be set")
	}
}

func TestCreateComment_Threaded(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "c_admin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Thread Node", "thread-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, admin.ID)

	// Create a top-level comment.
	body := map[string]interface{}{"body": "Parent comment"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for parent, got %d: %s", w.Code, w.Body.String())
	}
	parentResult := decodeJSON(t, w)
	parentID := parentResult["id"].(string)

	// Reply to the parent.
	body = map[string]interface{}{
		"body":      "Reply comment",
		"parent_id": parentID,
	}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for reply, got %d: %s", w.Code, w.Body.String())
	}
	replyResult := decodeJSON(t, w)
	if replyResult["parent_id"] != parentID {
		t.Errorf("expected parent_id=%s, got %v", parentID, replyResult["parent_id"])
	}
}

func TestListComments_Threaded(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "c_admin3", "member")
	nodeID := createTestNode(t, db, admin.ID, "ListThread Node", "listthread-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, admin.ID)

	// Create 2 top-level comments.
	for _, text := range []string{"Top 1", "Top 2"} {
		body := map[string]interface{}{"body": text}
		r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Get the first comment's ID for a reply.
	r := authedRequest("GET", "/api/v1/proposals/"+proposalID+"/comments", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	listResult := decodeJSON(t, w)
	items := listResult["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 top-level comments, got %d", len(items))
	}
	firstID := items[0].(map[string]interface{})["id"].(string)

	// Create a reply to the first comment.
	body := map[string]interface{}{"body": "Reply to Top 1", "parent_id": firstID}
	r = authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List again and verify tree structure.
	r = authedRequest("GET", "/api/v1/proposals/"+proposalID+"/comments", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	listResult = decodeJSON(t, w)
	items = listResult["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 top-level comments in tree, got %d", len(items))
	}

	firstItem := items[0].(map[string]interface{})
	replies := firstItem["replies"].([]interface{})
	if len(replies) != 1 {
		t.Errorf("expected 1 reply on first comment, got %d", len(replies))
	}
	if replies[0].(map[string]interface{})["body"] != "Reply to Top 1" {
		t.Errorf("unexpected reply body: %v", replies[0].(map[string]interface{})["body"])
	}
}

func TestUpdateComment_AuthorOnly(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "c_author4", "member")
	other, otherToken := createTestUser(t, db, "c_other4", "member")
	nodeID := createTestNode(t, db, author.ID, "EditComment Node", "editcomment-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	proposalID := createTestProposal(t, db, nodeID, author.ID)

	// Author creates a comment.
	body := map[string]interface{}{"body": "Original text"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, authorToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	commentID := decodeJSON(t, w)["id"].(string)

	// Author updates it -- should succeed.
	updateBody := map[string]interface{}{"body": "Updated text"}
	r = authedRequest("PATCH", "/api/v1/comments/"+commentID, updateBody, authorToken)
	w = serveMux(t, db, "PATCH", "/api/v1/comments/{id}", handler.UpdateComment(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["body"] != "Updated text" {
		t.Errorf("expected body='Updated text', got %v", result["body"])
	}

	// Other user tries to update -- should get 403.
	r = authedRequest("PATCH", "/api/v1/comments/"+commentID, updateBody, otherToken)
	w = serveMux(t, db, "PATCH", "/api/v1/comments/{id}", handler.UpdateComment(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteComment(t *testing.T) {
	db := setupTestDB(t)
	author, authorToken := createTestUser(t, db, "c_author5", "member")
	nodeID := createTestNode(t, db, author.ID, "DelComment Node", "delcomment-node", "open")
	createTestMembership(t, db, author.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, author.ID)

	// Create a comment.
	body := map[string]interface{}{"body": "To be deleted"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, authorToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	commentID := decodeJSON(t, w)["id"].(string)

	// Delete it.
	r = authedRequest("DELETE", "/api/v1/comments/"+commentID, nil, authorToken)
	w = serveMux(t, db, "DELETE", "/api/v1/comments/{id}", handler.DeleteComment(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM proposal_comments WHERE id = ?", commentID).Scan(&count)
	if count != 0 {
		t.Errorf("expected comment to be deleted, but found %d", count)
	}
}

func TestAddReaction(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "c_react6", "member")
	nodeID := createTestNode(t, db, user.ID, "React Node", "react-node", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, user.ID)

	// Create a comment to react to.
	body := map[string]interface{}{"body": "React to me"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	commentID := decodeJSON(t, w)["id"].(string)

	// Add a reaction.
	reactBody := map[string]interface{}{"emoji": "\xf0\x9f\x91\x8d"} // thumbs up
	r = authedRequest("POST", "/api/v1/comments/"+commentID+"/reactions", reactBody, userToken)
	w = serveMux(t, db, "POST", "/api/v1/comments/{id}/reactions", handler.AddReaction(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify reaction exists in DB.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM comment_reactions WHERE comment_id = ?", commentID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 reaction, got %d", count)
	}
}

func TestRemoveReaction(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "c_react7", "member")
	nodeID := createTestNode(t, db, user.ID, "UnReact Node", "unreact-node", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, user.ID)

	// Create comment.
	body := map[string]interface{}{"body": "Unreact me"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	commentID := decodeJSON(t, w)["id"].(string)

	// Add a reaction.
	emoji := "\xf0\x9f\x91\x8d" // thumbs up
	reactBody := map[string]interface{}{"emoji": emoji}
	r = authedRequest("POST", "/api/v1/comments/"+commentID+"/reactions", reactBody, userToken)
	w = serveMux(t, db, "POST", "/api/v1/comments/{id}/reactions", handler.AddReaction(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Remove the reaction.
	r = authedRequest("DELETE", "/api/v1/comments/"+commentID+"/reactions/"+emoji, nil, userToken)
	w = serveMux(t, db, "DELETE", "/api/v1/comments/{id}/reactions/{emoji}", handler.RemoveReaction(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify reaction is gone.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM comment_reactions WHERE comment_id = ?", commentID).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 reactions, got %d", count)
	}
}

func TestAddReaction_InvalidEmoji(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "c_react8", "member")
	nodeID := createTestNode(t, db, user.ID, "BadReact Node", "badreact-node", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	proposalID := createTestProposal(t, db, nodeID, user.ID)

	// Create comment.
	body := map[string]interface{}{"body": "Bad react"}
	r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, userToken)
	w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	commentID := decodeJSON(t, w)["id"].(string)

	// Try to add an invalid emoji.
	reactBody := map[string]interface{}{"emoji": "X"}
	r = authedRequest("POST", "/api/v1/comments/"+commentID+"/reactions", reactBody, userToken)
	w = serveMux(t, db, "POST", "/api/v1/comments/{id}/reactions", handler.AddReaction(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
