package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// `follower_permissions.proposals` had no effect anywhere in Go. The workspace
// hid the tab and the API took the comment anyway, so the one setting a patch
// has for saying "our proposals are not a follower matter" said it only to the
// SPA (docs/adr/050).
func TestFollowerProposalParticipation(t *testing.T) {
	comment := func(t *testing.T, db *database.DB, proposalID, token string) int {
		t.Helper()
		r := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/comments",
			map[string]string{"body": "A thought."}, token)
		return serveMux(t, db, "POST", "/api/v1/proposals/{id}/comments", handler.CreateComment(db), r).Code
	}

	t.Run("a follower comments when the patch includes them", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "fp_admin1", "member")
		follower, followerToken := createTestUser(t, db, "fp_follower1", "member")
		nodeID := createTestNode(t, db, admin.ID, "Open Talk", "open-talk", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
		proposalID := openProposal(t, db, nodeID, admin.ID, "Something to discuss")

		if code := comment(t, db, proposalID, followerToken); code != http.StatusCreated {
			t.Errorf("follower comment returned %d, want 201 — the default includes followers", code)
		}
	})

	t.Run("a follower is refused when the patch has switched proposals off", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "fp_admin2", "member")
		follower, followerToken := createTestUser(t, db, "fp_follower2", "member")
		nodeID := createTestNode(t, db, admin.ID, "Members Only Talk", "members-talk", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
			`{"events":true,"proposals":false,"charters":true,"members":true}`, nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
		proposalID := openProposal(t, db, nodeID, admin.ID, "Members deliberate this")

		if code := comment(t, db, proposalID, followerToken); code != http.StatusForbidden {
			t.Errorf("follower comment returned %d, want 403", code)
		}
	})

	t.Run("members and admins are unaffected by the follower setting", func(t *testing.T) {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "fp_admin3", "member")
		member, memberToken := createTestUser(t, db, "fp_member3", "member")
		nodeID := createTestNode(t, db, admin.ID, "Members Talk", "members-talk3", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		createTestMembership(t, db, member.ID, nodeID, "member", "active")
		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
			`{"events":true,"proposals":false,"charters":true,"members":true}`, nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
		proposalID := openProposal(t, db, nodeID, admin.ID, "Ours to discuss")

		if code := comment(t, db, proposalID, memberToken); code != http.StatusCreated {
			t.Errorf("member comment returned %d, want 201", code)
		}
		if code := comment(t, db, proposalID, adminToken); code != http.StatusCreated {
			t.Errorf("admin comment returned %d, want 201", code)
		}
	})

	t.Run("reading is untouched, because it was never private", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "fp_admin4", "member")
		nodeID := createTestNode(t, db, admin.ID, "Public Read", "public-read", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
			`{"events":true,"proposals":false,"charters":true,"members":true}`, nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
		proposalID := openProposal(t, db, nodeID, admin.ID, "Visible to anyone")

		// Signed out entirely: the thread is a public read and stays one.
		// Gating participation is not a claim that the data is hidden.
		r := authedRequest("GET", "/api/v1/proposals/"+proposalID+"/comments", nil, "")
		w := servePublicMux(t, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
		if w.Code != http.StatusOK {
			t.Errorf("anonymous comment read returned %d, want 200", w.Code)
		}
	})
}
