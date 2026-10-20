package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// #368: the Discussion tab's badge read `comment_count` from the proposal
// payload, which never carried one, so the badge was always 0 and never
// drawn. The count is every comment, replies included, and deleting a
// comment takes its replies with it.
func TestGetProposal_CountsItsDiscussion(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "cc_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Count Node", "count-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	proposalID := createTestProposal(t, db, nodeID, admin.ID)

	commentCount := func() float64 {
		t.Helper()
		r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("get proposal: %d %s", w.Code, w.Body.String())
		}
		n, ok := decodeJSON(t, w)["comment_count"].(float64)
		if !ok {
			t.Fatalf("payload carries no numeric comment_count")
		}
		return n
	}
	post := func(body map[string]interface{}) string {
		t.Helper()
		r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments", body, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r)
		if w.Code != http.StatusCreated {
			t.Fatalf("post comment: %d %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)["id"].(string)
	}

	if got := commentCount(); got != 0 {
		t.Fatalf("a proposal nobody has discussed: comment_count = %v, want 0", got)
	}

	first := post(map[string]interface{}{"body": "Top 1"})
	post(map[string]interface{}{"body": "Top 2"})
	post(map[string]interface{}{"body": "Reply to Top 1", "parent_id": first})
	if got := commentCount(); got != 3 {
		t.Fatalf("two comments and a reply: comment_count = %v, want 3", got)
	}

	r := authedRequest("DELETE", "/api/v1/comments/"+first, nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/comments/{id}", handler.DeleteComment(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete comment: %d %s", w.Code, w.Body.String())
	}
	if got := commentCount(); got != 1 {
		t.Fatalf("after deleting a comment with one reply: comment_count = %v, want 1", got)
	}
}
