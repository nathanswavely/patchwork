package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// Account deletion (docs/adr/086). Every test here is really one question:
// did the person go away without taking the record with them?

// serveSudoUser mounts a handler behind the gate chain the route uses:
// session, then step-up. No AdminRequired — this is a person's own account.
func serveSudoUser(db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthRequired(db, middleware.SudoRequired(db, h)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// deleteMe runs a full, step-up-satisfied deletion for username.
func deleteMe(t *testing.T, db *database.DB, token, username string) *httptest.ResponseRecorder {
	t.Helper()
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	r := authedRequest("DELETE", "/api/v1/users/me", map[string]string{"confirm_username": username}, token)
	return serveSudoUser(db, "DELETE", "/api/v1/users/me", handler.DeleteMyAccount(db, testConfig()), r)
}

// A patch with two admins, so leaving it is not the thing under test.
func nodeWithSpareAdmin(t *testing.T, db *database.DB, ownerID, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, slug, slug, "open")
	spare, _ := createTestUser(t, db, slug+"-spare-admin", "member")
	createTestMembership(t, db, spare.ID, nodeID, "admin", "active")
	return nodeID
}

// The happy path, in one test because these facts only mean anything
// together: the person is gone, and everything they did is still there.
func TestDeleteAccountErasesPersonAndKeepsActs(t *testing.T) {
	db := setupTestDB(t)
	// An instance admin so the last-admin floor is not what we are measuring.
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "leaver", "member")
	nodeID := nodeWithSpareAdmin(t, db, user.ID, "the-selvage")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	db.Exec(`UPDATE users SET email = ?, bio = ?, avatar_url = ?, contact_phone = ?, links = ? WHERE id = ?`,
		"leaver@example.com", "I organize things", "https://example.com/me.png", "555-0100",
		`[{"url":"https://example.com","label":"site"}]`, user.ID)

	// An act of record: a proposal, and a vote on it.
	proposalID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status) VALUES (?, ?, ?, ?, '', 'open')`,
		proposalID, nodeID, user.ID, "Buy a new PA",
	); err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	voteID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, 'approve')`,
		voteID, proposalID, user.ID,
	); err != nil {
		t.Fatalf("insert vote: %v", err)
	}

	if w := deleteMe(t, db, token, "leaver"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d, want 200: %s", w.Code, w.Body.String())
	}

	// The row survives — that is the whole design — with nothing in it.
	var (
		email       sql.NullString
		deletedAt   sql.NullString
		username    string
		displayName string
		bio         string
		avatar      string
		phone       string
		links       sql.NullString
		role        string
	)
	err := db.QueryRow(
		`SELECT email, deleted_at, username, display_name, bio, avatar_url, contact_phone, links, role
		 FROM users WHERE id = ?`, user.ID,
	).Scan(&email, &deletedAt, &username, &displayName, &bio, &avatar, &phone, &links, &role)
	if err != nil {
		t.Fatalf("the tombstone must survive so the record does: %v", err)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Error("deleted_at not set")
	}
	if email.Valid {
		t.Errorf("email survived deletion: %q", email.String)
	}
	for label, got := range map[string]string{
		"display_name": displayName, "bio": bio, "avatar_url": avatar, "contact_phone": phone,
	} {
		if got != "" {
			t.Errorf("%s survived deletion: %q", label, got)
		}
	}
	if links.String != "[]" {
		t.Errorf("links survived deletion: %q", links.String)
	}
	if role != "member" {
		t.Errorf("role = %q, want member: a tombstone holds no instance role", role)
	}
	// Retired, not freed.
	if username != "leaver" {
		t.Errorf("username = %q, want it kept so the handle can never be reissued", username)
	}

	// Rows that were the person alone.
	for _, q := range []string{
		`SELECT COUNT(*) FROM sessions WHERE user_id = ?`,
		`SELECT COUNT(*) FROM memberships WHERE user_id = ?`,
		`SELECT COUNT(*) FROM notification_preferences WHERE user_id = ?`,
		`SELECT COUNT(*) FROM credentials WHERE user_id = ?`,
	} {
		var n int
		db.QueryRow(q, user.ID).Scan(&n)
		if n != 0 {
			t.Errorf("%s returned %d, want 0", q, n)
		}
	}

	// The acts.
	var proposals, votes int
	db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE author_id = ?`, user.ID).Scan(&proposals)
	db.QueryRow(`SELECT COUNT(*) FROM votes WHERE user_id = ?`, user.ID).Scan(&votes)
	if proposals != 1 {
		t.Errorf("proposal count = %d, want 1: a proposal must keep its proposer", proposals)
	}
	if votes != 1 {
		t.Errorf("vote count = %d, want 1: a vote must keep its voter", votes)
	}

	// And the audit trail says it happened.
	var audits int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'user.deleted' AND entity_id = ?`, user.ID).Scan(&audits)
	if audits != 1 {
		t.Errorf("user.deleted audit entries = %d, want 1", audits)
	}
}

// The proposal is still readable and the author renders as the neutral
// label, not as the retired handle. This is the substitution that stops a
// frontend leaking a name it was never sent.
func TestDeletedAuthorRendersAsDeletedAccount(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "ghostwriter", "member")
	db.Exec(`UPDATE users SET display_name = 'Very Real Name' WHERE id = ?`, user.ID)
	nodeID := nodeWithSpareAdmin(t, db, user.ID, "gallery-row")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	proposalID := auth.NewUUIDv7()
	db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status) VALUES (?, ?, ?, 'Repaint the hall', '', 'open')`,
		proposalID, nodeID, user.ID)

	viewer, viewerToken := createTestUser(t, db, "still-here", "member")
	createTestMembership(t, db, viewer.ID, nodeID, "member", "active")

	if w := deleteMe(t, db, token, "ghostwriter"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", w.Code, w.Body.String())
	}

	r := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, viewerToken)
	w := serveAuthed(db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("proposal by a deleted author returned %d, want it still readable: %s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := body["author_name"]; got != handler.DeletedAccountName {
		t.Errorf("author_name = %v, want %q", got, handler.DeletedAccountName)
	}
	if raw := w.Body.String(); strings.Contains(raw, "Very Real Name") || strings.Contains(raw, "ghostwriter") {
		t.Errorf("the response leaked the deleted person's identity: %s", raw)
	}
}

// The floor from LeaveNode, applied to every patch at once. The refusal has
// to name the patches, because "hand it over first" is useless without
// saying which.
func TestDeleteAccountRefusedAsSolePatchAdmin(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "only-admin", "member")
	nodeID := createTestNode(t, db, user.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	w := deleteMe(t, db, token, "only-admin")
	if w.Code != http.StatusConflict {
		t.Fatalf("delete as sole admin returned %d, want 409: %s", w.Code, w.Body.String())
	}

	var body struct {
		Code    string `json:"code"`
		Patches []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"patches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "sole_admin" {
		t.Errorf("code = %q, want sole_admin", body.Code)
	}
	if len(body.Patches) != 1 || body.Patches[0].Slug != "the-selvage" || body.Patches[0].Name != "The Selvage" {
		t.Errorf("patches = %+v, want the one patch named", body.Patches)
	}

	// A refused deletion must not have deleted anything.
	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Error("the account was erased despite the refusal")
	}
	var memberships int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE user_id = ?`, user.ID).Scan(&memberships)
	if memberships != 1 {
		t.Errorf("memberships = %d, want 1: a refused deletion changes nothing", memberships)
	}
}

// A suspended co-admin is not an admin present: the count asks whether
// somebody can run the patch tomorrow.
func TestSuspendedCoAdminDoesNotUnblockDeletion(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "last-standing", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")

	other, _ := createTestUser(t, db, "benched", "member")
	createTestMembership(t, db, other.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE users SET suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, other.ID)

	if w := deleteMe(t, db, token, "last-standing"); w.Code != http.StatusConflict {
		t.Fatalf("returned %d, want 409: a suspended admin cannot run the patch: %s", w.Code, w.Body.String())
	}
}

// The invariant bootstrap.go establishes when it makes the first account an
// admin: an instance always has one.
func TestDeleteAccountRefusedAsLastInstanceAdmin(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "sole-boss", "admin")
	createTestUser(t, db, "a-member", "member")

	w := deleteMe(t, db, token, "sole-boss")
	if w.Code != http.StatusConflict {
		t.Fatalf("returned %d, want 409: %s", w.Code, w.Body.String())
	}
	if code := bodyCode(t, w); code != "last_instance_admin" {
		t.Errorf("code = %q, want last_instance_admin", code)
	}

	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Error("the last instance admin was erased anyway")
	}
}

// With a second admin present the floor lifts.
func TestLastInstanceAdminFloorLiftsWithASecondAdmin(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "one-boss", "admin")
	createTestUser(t, db, "two-boss", "admin")

	if w := deleteMe(t, db, token, "one-boss"); w.Code != http.StatusOK {
		t.Fatalf("returned %d, want 200: %s", w.Code, w.Body.String())
	}
	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt)
	if !deletedAt.Valid {
		t.Error("deletion reported success without erasing the account")
	}
}

// A valid cookie is proof of identity, not of presence (docs/adr/017).
func TestDeleteAccountRejectsSessionWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")
	user, token := createTestUser(t, db, "hasty", "member")

	r := authedRequest("DELETE", "/api/v1/users/me", map[string]string{"confirm_username": "hasty"}, token)
	w := serveSudoUser(db, "DELETE", "/api/v1/users/me", handler.DeleteMyAccount(db, testConfig()), r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("returned %d, want 403: %s", w.Code, w.Body.String())
	}
	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Fatal("a gate that returns 403 after doing the work is not a gate")
	}
}

// Typing the wrong username is not a confirmation.
func TestDeleteAccountNeedsTheTypedUsername(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")
	user, token := createTestUser(t, db, "careful", "member")
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}

	r := authedRequest("DELETE", "/api/v1/users/me", map[string]string{"confirm_username": "carefull"}, token)
	w := serveSudoUser(db, "DELETE", "/api/v1/users/me", handler.DeleteMyAccount(db, testConfig()), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("returned %d, want 400: %s", w.Code, w.Body.String())
	}
	var deletedAt sql.NullString
	db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, user.ID).Scan(&deletedAt)
	if deletedAt.Valid {
		t.Fatal("erased on a mistyped confirmation")
	}
}

// The profile stops existing, even though the row does not.
func TestDeletedProfileIsNotFound(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")
	_, token := createTestUser(t, db, "vanishing", "member")

	if w := deleteMe(t, db, token, "vanishing"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", w.Code, w.Body.String())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/{username}", handler.GetUserProfile(db))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/users/vanishing", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("profile of a deleted account returned %d, want 404: %s", w.Code, w.Body.String())
	}
}

// The handle is retired, not freed. Freeing it would let a stranger inherit
// every link that pointed at somebody else.
func TestDeletedUsernameCannotBeReregistered(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")
	_, token := createTestUser(t, db, "taken-forever", "member")

	if w := deleteMe(t, db, token, "taken-forever"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", w.Code, w.Body.String())
	}

	if err := auth.CheckUsernameAvailable(db, "taken-forever"); err == nil {
		t.Fatal("a deleted account's username came back up for grabs")
	}
}

// The session dies with the account, so the cookie in the deleting request
// is worthless the moment it returns.
func TestDeletionRevokesSessions(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")
	_, token := createTestUser(t, db, "signed-out", "member")

	if w := deleteMe(t, db, token, "signed-out"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", w.Code, w.Body.String())
	}

	u, err := auth.ValidateSession(db, token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if u != nil {
		t.Fatal("the session outlived the account")
	}
}

// A tombstone is not an account to administer.
func TestDeletedAccountIsNotInTheAdminUserList(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "instance-boss", "admin")
	_, token := createTestUser(t, db, "listed-once", "member")

	if w := deleteMe(t, db, token, "listed-once"); w.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", w.Code, w.Body.String())
	}

	r := authedRequest("GET", "/api/v1/admin/users", nil, adminToken)
	w := serveAdmin(db, "GET", "/api/v1/admin/users", handler.ListUsers(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list users returned %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "listed-once") {
		t.Errorf("the admin user list still shows a tombstone: %s", w.Body.String())
	}
}

// A maintainer patch with a named successor is not a blocker: leaving is
// the handover (docs/adr/051), and account deletion is leaving.
func TestSolePatchAdminWithSuccessorCanDelete(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "maintainer", "member")
	nodeID := createTestNode(t, db, user.ID, "One Person Band", "one-person-band", "invite_only")
	createTestMembership(t, db, user.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"leadership_model":"maintainer"}`, nodeID)

	heir, _ := createTestUser(t, db, "heir", "member")
	createTestMembership(t, db, heir.ID, nodeID, "member", "active")
	db.Exec(`UPDATE nodes SET designated_successor_id = ? WHERE id = ?`, heir.ID, nodeID)

	if w := deleteMe(t, db, token, "maintainer"); w.Code != http.StatusOK {
		t.Fatalf("returned %d, want 200: %s", w.Code, w.Body.String())
	}

	var role string
	db.QueryRow(`SELECT role FROM memberships WHERE user_id = ? AND node_id = ?`, heir.ID, nodeID).Scan(&role)
	if role != "admin" {
		t.Errorf("successor role = %q, want admin: the patch must not be left unadministered", role)
	}
	var successor sql.NullString
	db.QueryRow(`SELECT designated_successor_id FROM nodes WHERE id = ?`, nodeID).Scan(&successor)
	if successor.Valid && successor.String == user.ID {
		t.Error("the node still names the deleted account as its successor")
	}
}

// Pointers the schema declared ON DELETE SET NULL never fire, because
// nothing is deleted. The handler carries out that intent by hand.
func TestDeletionVacatesSeatsAndSuccessorPointers(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss", "admin")

	user, token := createTestUser(t, db, "seat-holder", "member")
	nodeID := nodeWithSpareAdmin(t, db, user.ID, "the-council")
	createTestMembership(t, db, user.ID, nodeID, "member", "active")

	seatID := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO seats (id, node_id, holder_id) VALUES (?, ?, ?)`, seatID, nodeID, user.ID); err != nil {
		t.Fatalf("insert seat: %v", err)
	}

	// Another patch names them successor; that designation cannot survive.
	otherNode := nodeWithSpareAdmin(t, db, user.ID, "elsewhere")
	createTestMembership(t, db, user.ID, otherNode, "member", "active")
	db.Exec(`UPDATE nodes SET designated_successor_id = ? WHERE id = ?`, user.ID, otherNode)

	if w := deleteMe(t, db, token, "seat-holder"); w.Code != http.StatusOK {
		t.Fatalf("returned %d: %s", w.Code, w.Body.String())
	}

	var holder sql.NullString
	db.QueryRow(`SELECT holder_id FROM seats WHERE id = ?`, seatID).Scan(&holder)
	if holder.Valid {
		t.Errorf("seat still held by a deleted account: %q", holder.String)
	}
	var successor sql.NullString
	db.QueryRow(`SELECT designated_successor_id FROM nodes WHERE id = ?`, otherNode).Scan(&successor)
	if successor.Valid {
		t.Errorf("a patch still names a deleted account as its successor: %q", successor.String)
	}
}
