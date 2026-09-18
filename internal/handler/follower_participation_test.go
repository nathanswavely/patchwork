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

	// The follower key does not decide whether the thread can be read — the
	// patch's record setting does
	// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md). These
	// two cases are one pair on purpose: docs/adr/095 says the
	// follower_permissions family is workspace tidiness and "must not be
	// merged" with a privacy control, so `proposals: false` is held constant
	// across both and the record setting is the only thing that moves.
	//
	// This replaces a case named "reading is untouched, because it was never
	// private", whose premise the ADR overturned.
	t.Run("the follower key still does not gate the read", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "fp_admin4", "member")
		nodeID := createTestNode(t, db, admin.ID, "Public Read", "public-read", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		openGovernanceRecord(t, db, nodeID)
		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
			`{"events":true,"proposals":false,"charters":true,"members":true}`, nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
		proposalID := openProposal(t, db, nodeID, admin.ID, "Visible to anyone")

		// Signed out entirely, on a patch that publishes its record: the
		// thread reads, `proposals: false` notwithstanding. Gating
		// participation is still not a claim that the data is hidden.
		r := authedRequest("GET", "/api/v1/proposals/"+proposalID+"/comments", nil, "")
		w := servePublicMux(t, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
		if w.Code != http.StatusOK {
			t.Errorf("anonymous comment read returned %d, want 200", w.Code)
		}
	})

	t.Run("a closed record does gate the read", func(t *testing.T) {
		db := setupTestDB(t)
		admin, _ := createTestUser(t, db, "fp_admin5", "member")
		nodeID := createTestNode(t, db, admin.ID, "Closed Read", "closed-read", "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		// No openGovernanceRecord: a patch is born closed, which is the point.
		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
			`{"events":true,"proposals":false,"charters":true,"members":true}`, nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
		proposalID := openProposal(t, db, nodeID, admin.ID, "Ours alone")

		// 404 rather than an empty 200: the thread is reached from a proposal
		// that already answered 404, so an empty list here would only ever be
		// read by somebody who guessed the id.
		r := authedRequest("GET", "/api/v1/proposals/"+proposalID+"/comments", nil, "")
		w := servePublicMux(t, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
		if w.Code != http.StatusNotFound {
			t.Errorf("anonymous comment read on a closed record returned %d, want 404", w.Code)
		}
	})
}
