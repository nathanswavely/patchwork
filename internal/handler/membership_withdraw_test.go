package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Withdrawing an unanswered membership request.
//
// Distinct from leaving, the way WithdrawClaim is distinct from rejection:
// nobody admitted this person, so there is no community to exit. These pin
// that the two verbs stay apart — LeaveNode must keep refusing a pending
// row, or the audit log stops being able to tell a change of mind from a
// departure, and a patch's record shows people leaving who were never in it.

func withdraw(t *testing.T, db *database.DB, slug, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/withdraw", nil, token)
	return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/withdraw", handler.WithdrawMembershipRequest(db), r)
}

func membershipRow(t *testing.T, db *database.DB, userID, nodeID string) (status string, joinMessage *string) {
	t.Helper()
	if err := db.QueryRow(
		"SELECT status, join_message FROM memberships WHERE user_id = ? AND node_id = ?",
		userID, nodeID,
	).Scan(&status, &joinMessage); err != nil {
		t.Fatalf("read membership: %v", err)
	}
	return status, joinMessage
}

func TestWithdrawPendingRequest(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "wd-admin", "member")
	requester, token := createTestUser(t, db, "wd-requester", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Ask through the real join path, so the row under test is the one a
	// person actually produces — including its join_message.
	r := authedRequest("POST", "/api/v1/nodes/wd-approval/join",
		map[string]string{"message": "I run the print shop next door."}, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("join: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if status, msg := membershipRow(t, db, requester.ID, nodeID); status != "pending" || msg == nil {
		t.Fatalf("expected a pending row carrying its message, got %q msg=%v", status, msg)
	}

	if w := withdraw(t, db, "wd-approval", token); w.Code != http.StatusOK {
		t.Fatalf("withdraw: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	status, msg := membershipRow(t, db, requester.ID, nodeID)
	if status != "left" {
		t.Errorf("expected status=left, got %q", status)
	}
	// The intro was written for a request that no longer exists — rejection
	// clears it for the same reason.
	if msg != nil {
		t.Errorf("expected join_message cleared, got %q", *msg)
	}
}

func TestWithdrawLetsThePersonAskAgain(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "wd2-admin", "member")
	requester, token := createTestUser(t, db, "wd2-requester", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd2-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	join := func() int {
		r := authedRequest("POST", "/api/v1/nodes/wd2-approval/join", nil, token)
		return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r).Code
	}

	if code := join(); code != http.StatusCreated {
		t.Fatalf("first join: expected 201, got %d", code)
	}
	// A second ask while one is open is the 409 the UI stops short of.
	if code := join(); code != http.StatusConflict {
		t.Fatalf("second join: expected 409, got %d", code)
	}
	if w := withdraw(t, db, "wd2-approval", token); w.Code != http.StatusOK {
		t.Fatalf("withdraw: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Withdrawing is not a ban: asking again has to work.
	if code := join(); code != http.StatusCreated {
		t.Fatalf("re-join after withdraw: expected 201, got %d", code)
	}
	if status, _ := membershipRow(t, db, requester.ID, nodeID); status != "pending" {
		t.Errorf("expected pending again, got %q", status)
	}
}

func TestWithdrawRefusesWhatIsNotAPendingRequest(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "wd3-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd3-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	cases := []struct {
		name   string
		role   string
		status string
	}{
		{"an active member", "member", "active"},
		{"an active follower", "follower", "active"},
		{"an admin", "admin", "active"},
		{"someone who already left", "member", "left"},
		// No 'banned' case: the memberships CHECK constraint from
		// migration 009 is still status IN ('active','pending','left'),
		// so such a row cannot be created. UpdateMember writes 'banned'
		// anyway and fails — a pre-existing bug, not this route's.
		// Withdraw would refuse it regardless: it accepts 'pending' only.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, token := createTestUser(t, db, "wd3-"+tc.name, "member")
			createTestMembership(t, db, user.ID, nodeID, tc.role, tc.status)

			w := withdraw(t, db, "wd3-approval", token)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			// Above all: it must not become an exit route that skips
			// LeaveNode's only-admin floor.
			if status, _ := membershipRow(t, db, user.ID, nodeID); status != tc.status {
				t.Errorf("row changed: %q -> %q", tc.status, status)
			}
		})
	}
}

func TestWithdrawIsNotAnAdminTool(t *testing.T) {
	// Scoped to the caller's own row. An admin turning a request down is
	// UpdateMember's reject, which is a decision and is logged as one; this
	// route must not give an admin a quieter way to do the same thing.
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "wd4-admin", "member")
	requester, _ := createTestUser(t, db, "wd4-requester", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd4-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, requester.ID, nodeID, "member", "pending")

	if w := withdraw(t, db, "wd4-approval", adminToken); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an admin with no request of their own, got %d: %s", w.Code, w.Body.String())
	}
	if status, _ := membershipRow(t, db, requester.ID, nodeID); status != "pending" {
		t.Errorf("the requester's row was touched: %q", status)
	}
}

func TestWithdrawUnknownPatch(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "wd5-user", "member")
	if w := withdraw(t, db, "no-such-patch", token); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLeaveStillRefusesAPendingRequest(t *testing.T) {
	// The reason withdraw exists. If LeaveNode ever starts accepting a
	// pending row, the two verbs have collapsed and the audit log can no
	// longer tell a change of mind from a departure.
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "wd6-admin", "member")
	requester, token := createTestUser(t, db, "wd6-requester", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd6-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, requester.ID, nodeID, "member", "pending")

	r := authedRequest("POST", "/api/v1/nodes/wd6-approval/leave", nil, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected leave to refuse a pending row with 400, got %d: %s", w.Code, w.Body.String())
	}
	if status, _ := membershipRow(t, db, requester.ID, nodeID); status != "pending" {
		t.Errorf("leave changed a pending row to %q", status)
	}
}

func TestWithdrawIsAudited(t *testing.T) {
	// Its own action, not membership.leave: "changed their mind before
	// anyone answered" and "was here and left" are different facts.
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "wd7-admin", "member")
	requester, token := createTestUser(t, db, "wd7-requester", "member")
	nodeID := createTestNode(t, db, admin.ID, "Approval", "wd7-approval", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, requester.ID, nodeID, "member", "pending")

	if w := withdraw(t, db, "wd7-approval", token); w.Code != http.StatusOK {
		t.Fatalf("withdraw: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE user_id = ? AND action = 'membership.withdraw'",
		requester.ID,
	).Scan(&count); err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 membership.withdraw audit row, got %d", count)
	}
}
