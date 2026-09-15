package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Membership invitations (docs/adr/098).
//
// An invite-only patch had no door. An admin now invites by username and
// the person accepts or declines; until they answer the row is 'invited',
// which is not membership anywhere — these pin both halves: the door
// exists, and walking up to it is not the same as being inside.

// inviteFixture is an invite-only patch with one admin, one member, one
// follower, plus a stranger, an instance admin with no role here, and the
// person who will be invited.
type inviteFixture struct {
	nodeID        string
	slug          string
	admin         string
	adminToken    string
	memberToken   string
	followerToken string
	strangerToken string
	siteToken     string
	invitee       string
	inviteeName   string
	inviteeToken  string
}

func newInviteFixture(t *testing.T, db *database.DB, slug string) inviteFixture {
	t.Helper()
	admin, adminToken := createTestUser(t, db, slug+"-admin", "member")
	member, memberToken := createTestUser(t, db, slug+"-member", "member")
	follower, followerToken := createTestUser(t, db, slug+"-follower", "member")
	_, strangerToken := createTestUser(t, db, slug+"-stranger", "member")
	_, siteToken := createTestUser(t, db, slug+"-site", "admin")
	invitee, inviteeToken := createTestUser(t, db, slug+"-invitee", "member")

	nodeID := createTestNode(t, db, admin.ID, "Choir "+slug, slug, "invite_only")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	return inviteFixture{
		nodeID: nodeID, slug: slug,
		admin: admin.ID, adminToken: adminToken,
		memberToken: memberToken, followerToken: followerToken,
		strangerToken: strangerToken, siteToken: siteToken,
		invitee: invitee.ID, inviteeName: invitee.Username, inviteeToken: inviteeToken,
	}
}

func invite(t *testing.T, db *database.DB, slug, username, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/invitations", map[string]string{"username": username}, token)
	return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/invitations", handler.InviteMember(db), r)
}

func answerInvitation(t *testing.T, db *database.DB, slug, verb, token string) *httptest.ResponseRecorder {
	t.Helper()
	h := handler.AcceptInvitation(db)
	if verb == "decline" {
		h = handler.DeclineInvitation(db)
	}
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/invitations/"+verb, nil, token)
	return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/invitations/"+verb, h, r)
}

func rescind(t *testing.T, db *database.DB, slug, userID, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("DELETE", "/api/v1/nodes/"+slug+"/invitations/"+userID, nil, token)
	return serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/invitations/{userId}", handler.RescindInvitation(db), r)
}

// membersPayload calls the members endpoint the way main.go mounts it and
// returns the whole payload, so a test can ask about the `invited` key.
func membersPayload(t *testing.T, db *database.DB, path, token string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", path, nil, token)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes/{slug}/members", middleware.AuthOptional(db, handler.ListMembers(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("members: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func auditCount(t *testing.T, db *database.DB, userID, action string) int {
	t.Helper()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE user_id = ? AND action = ?", userID, action).Scan(&n)
	return n
}

func TestInviteByAdminCreatesAnInvitedRowAndTellsThePerson(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	f := newInviteFixture(t, db, "inv-basic")

	// With the @ on, as people paste handles.
	w := invite(t, db, f.slug, "@"+f.inviteeName, f.adminToken)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var status, role string
	if err := db.QueryRow("SELECT status, role FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&status, &role); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "invited" || role != "member" {
		t.Errorf("expected invited/member, got %s/%s", status, role)
	}
	if n := auditCount(t, db, f.admin, "membership.invite"); n != 1 {
		t.Errorf("expected 1 membership.invite audit row, got %d", n)
	}
	if n := countNotifications(t, db, f.invitee, notifications.MembershipInvited, 1); n != 1 {
		t.Errorf("invitee's bell: got %d membership.invited, want 1", n)
	}
	var link, title string
	db.QueryRow("SELECT link, title FROM notifications WHERE user_id = ? AND type = ?", f.invitee, string(notifications.MembershipInvited)).Scan(&link, &title)
	if link != "/patches/"+f.slug {
		t.Errorf("the notice should open the patch page, got link %q", link)
	}
	if title != "You're invited to join Choir "+f.slug {
		t.Errorf("unexpected title %q", title)
	}
}

// The room rule (docs/adr/081): an active admin of this patch, and nobody
// else — not a member, not a follower, not a stranger, and not an instance
// admin holding no role here. One 404 for all of them.
func TestInviteIsThePatchAdminsAlone(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-room")
	for name, token := range map[string]string{
		"a member":          f.memberToken,
		"a follower":        f.followerToken,
		"a stranger":        f.strangerToken,
		"an instance admin": f.siteToken,
	} {
		t.Run(name, func(t *testing.T) {
			w := invite(t, db, f.slug, f.inviteeName, token)
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
			}
			var n int
			db.QueryRow("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&n)
			if n != 0 {
				t.Errorf("a row was written by %s", name)
			}
		})
	}
	if w := invite(t, db, "no-such-patch", f.inviteeName, f.adminToken); w.Code != http.StatusNotFound {
		t.Errorf("unknown patch: expected 404, got %d", w.Code)
	}
}

func TestInviteRefusesWhatIsAlreadyThere(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-dup")

	if w := invite(t, db, f.slug, "nobody-by-that-name", f.adminToken); w.Code != http.StatusNotFound {
		t.Errorf("unknown username: expected 404, got %d", w.Code)
	}
	if w := invite(t, db, f.slug, "", f.adminToken); w.Code != http.StatusBadRequest {
		t.Errorf("blank username: expected 400, got %d", w.Code)
	}
	if w := invite(t, db, f.slug, f.slug+"-member", f.adminToken); w.Code != http.StatusConflict {
		t.Errorf("an active member: expected 409, got %d", w.Code)
	}

	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("first invite: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusConflict {
		t.Errorf("second invite: expected 409, got %d", w.Code)
	}

	pending, _ := createTestUser(t, db, "inv-dup-pending", "member")
	createTestMembership(t, db, pending.ID, f.nodeID, "member", "pending")
	if w := invite(t, db, f.slug, pending.Username, f.adminToken); w.Code != http.StatusConflict {
		t.Errorf("a requester: expected 409, got %d", w.Code)
	}
	banned, _ := createTestUser(t, db, "inv-dup-banned", "member")
	createTestMembership(t, db, banned.ID, f.nodeID, "member", "banned")
	if w := invite(t, db, f.slug, banned.Username, f.adminToken); w.Code != http.StatusConflict {
		t.Errorf("a removed person: expected 409, got %d", w.Code)
	}
	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", banned.ID, f.nodeID).Scan(&status)
	if status != "banned" {
		t.Errorf("an invitation must not be a quiet un-ban, row is now %q", status)
	}
}

// Someone who left may be asked back: the same row, re-invited.
func TestInviteReusesALeftRow(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-left")
	left, _ := createTestUser(t, db, "inv-left-person", "member")
	rowID := createTestMembership(t, db, left.ID, f.nodeID, "admin", "left")

	if w := invite(t, db, f.slug, left.Username, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var id, status, role string
	db.QueryRow("SELECT id, status, role FROM memberships WHERE user_id = ? AND node_id = ?", left.ID, f.nodeID).Scan(&id, &status, &role)
	if id != rowID || status != "invited" || role != "member" {
		t.Errorf("expected the same row as invited/member, got id=%s (was %s) %s/%s", id, rowID, status, role)
	}
}

func TestAcceptMakesAMemberAndTellsTheAdmins(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	f := newInviteFixture(t, db, "inv-accept")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	var invitedAt string
	db.QueryRow("SELECT joined_at FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&invitedAt)
	// Push the stamp back so the reset on accept is observable.
	db.Exec("UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID)

	w := answerInvitation(t, db, f.slug, "accept", f.inviteeToken)
	if w.Code != http.StatusOK {
		t.Fatalf("accept: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var status, role, joinedAt string
	db.QueryRow("SELECT status, role, joined_at FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&status, &role, &joinedAt)
	if status != "active" || role != "member" {
		t.Errorf("expected active/member, got %s/%s", status, role)
	}
	if joinedAt == "2020-01-01T00:00:00.000Z" {
		t.Error("joined_at was not reset on accept: it would date the membership from the invitation")
	}
	if n := auditCount(t, db, f.invitee, "membership.accept_invite"); n != 1 {
		t.Errorf("expected 1 membership.accept_invite audit row, got %d", n)
	}
	if n := countNotifications(t, db, f.admin, notifications.MembershipJoined, 1); n != 1 {
		t.Errorf("admin's bell: got %d membership.joined, want 1", n)
	}
	// Answered once. A second accept has nothing to accept.
	if w := answerInvitation(t, db, f.slug, "accept", f.inviteeToken); w.Code != http.StatusBadRequest {
		t.Errorf("second accept: expected 400, got %d", w.Code)
	}
}

func TestDeclineDeletesTheRow(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-decline")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	if w := answerInvitation(t, db, f.slug, "decline", f.inviteeToken); w.Code != http.StatusOK {
		t.Fatalf("decline: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&n)
	if n != 0 {
		t.Errorf("expected the row gone, found %d", n)
	}
	if n := auditCount(t, db, f.invitee, "membership.decline_invite"); n != 1 {
		t.Errorf("expected 1 membership.decline_invite audit row, got %d", n)
	}
	// And they can be asked again.
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Errorf("re-invite after decline: expected 201, got %d", w.Code)
	}
}

// Accept and decline are the invitee's own row: an admin, a member and a
// stranger all have no invitation to answer here.
func TestAnswerIsTheInviteesAlone(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-own")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	for name, token := range map[string]string{"the admin": f.adminToken, "a member": f.memberToken, "a stranger": f.strangerToken} {
		for _, verb := range []string{"accept", "decline"} {
			if w := answerInvitation(t, db, f.slug, verb, token); w.Code != http.StatusBadRequest {
				t.Errorf("%s %s: expected 400, got %d", name, verb, w.Code)
			}
		}
	}
	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&status)
	if status != "invited" {
		t.Errorf("the invitee's row was touched: %q", status)
	}
}

// A person pressing the ordinary button is accepting — on an invite-only
// patch, where that button is otherwise a 403.
func TestJoinWithAnInvitedRowAccepts(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-join")

	join := func(body interface{}) *httptest.ResponseRecorder {
		r := authedRequest("POST", "/api/v1/nodes/"+f.slug+"/join", body, f.inviteeToken)
		return serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	}
	if w := join(nil); w.Code != http.StatusForbidden {
		t.Fatalf("uninvited join on invite_only: expected 403, got %d", w.Code)
	}
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	// Following would quietly overwrite the invitation with less.
	if w := join(map[string]string{"role": "follower"}); w.Code != http.StatusConflict {
		t.Errorf("follow while invited: expected 409, got %d", w.Code)
	}
	w := join(nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("join while invited: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["status"] != "active" {
		t.Errorf("expected status active in the response, got %q", out["status"])
	}
	var status string
	db.QueryRow("SELECT status FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&status)
	if status != "active" {
		t.Errorf("expected active, got %q", status)
	}
	if n := auditCount(t, db, f.invitee, "membership.accept_invite"); n != 1 {
		t.Errorf("join-as-accept must audit as an accept, got %d", n)
	}
}

// Withdraw takes a pending row only (docs/adr/088), and leave an active one.
// An invitation is neither: the row is the admin's ask, not the person's.
func TestWithdrawAndLeaveRefuseAnInvitedRow(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-wd")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	if w := withdraw(t, db, f.slug, f.inviteeToken); w.Code != http.StatusBadRequest {
		t.Errorf("withdraw: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	r := authedRequest("POST", "/api/v1/nodes/"+f.slug+"/leave", nil, f.inviteeToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/leave", handler.LeaveNode(db), r); w.Code != http.StatusBadRequest {
		t.Errorf("leave: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if status, _ := membershipRow(t, db, f.invitee, f.nodeID); status != "invited" {
		t.Errorf("row changed to %q", status)
	}
}

// The Invited list is for the patch's admins. Everyone else — the member
// list, the counts, an instance admin without a role — sees nothing of it.
func TestMembersListingShowsInvitedToPatchAdminsOnly(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-list")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	path := "/api/v1/nodes/" + f.slug + "/members"

	adminView := membersPayload(t, db, path, f.adminToken)
	invited, ok := adminView["invited"].([]interface{})
	if !ok || len(invited) != 1 {
		t.Fatalf("admin should see one invited person, got %v", adminView["invited"])
	}
	row := invited[0].(map[string]interface{})
	if row["username"] != f.inviteeName || row["user_id"] != f.invitee || row["invited_at"] == "" {
		t.Errorf("invited row is incomplete: %v", row)
	}

	for name, token := range map[string]string{
		"a member":             f.memberToken,
		"a follower":           f.followerToken,
		"a stranger":           f.strangerToken,
		"an instance admin":    f.siteToken,
		"a signed-out visitor": "",
	} {
		view := membersPayload(t, db, path, token)
		if _, has := view["invited"]; has {
			t.Errorf("%s was handed the invited list", name)
		}
		for _, it := range view["items"].([]interface{}) {
			if it.(map[string]interface{})["username"] == f.inviteeName {
				t.Errorf("%s saw the invitee among the members", name)
			}
		}
	}

	// Counts: two members (admin + member), unchanged by the invitation.
	if c := adminView["member_count"].(float64); c != 2 {
		t.Errorf("member_count counted the invitation: got %v, want 2", c)
	}
	// Asking for the invited rows as a page of members lists members.
	for _, token := range []string{f.strangerToken, f.adminToken} {
		view := membersPayload(t, db, path+"?status=invited", token)
		for _, it := range view["items"].([]interface{}) {
			if it.(map[string]interface{})["status"] != "active" {
				t.Errorf("?status=invited served a non-active row: %v", it)
			}
		}
	}
}

// Every other membership surface already pins status = 'active'; these are
// the ones a person would check first.
func TestInvitedIsNotMembershipAnywhere(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-nowhere")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}

	// me/nodes: not among my patches.
	for _, m := range myNodes(t, db, f.inviteeToken) {
		if m["node_slug"] == f.slug {
			t.Errorf("me/nodes lists the invitation as a membership: %v", m)
		}
	}
	// users/me/invitations: exactly there.
	r := authedRequest("GET", "/api/v1/users/me/invitations", nil, f.inviteeToken)
	w := serveMux(t, db, "GET", "/api/v1/users/me/invitations", handler.ListMyInvitations(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("invitations: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var inv struct {
		Items []map[string]interface{} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &inv)
	if len(inv.Items) != 1 || inv.Items[0]["node_slug"] != f.slug || inv.Items[0]["status"] != "invited" {
		t.Errorf("expected one invitation for %s, got %v", f.slug, inv.Items)
	}

	// The node payload: no standing, no ban.
	r = authedRequest("GET", "/api/v1/nodes/"+f.slug, nil, f.inviteeToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	var node map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &node)
	if _, has := node["membership_role"]; has {
		t.Error("nodes/{slug} claims a role for an invited row")
	}
	if node["is_member"] == true || node["is_banned"] == true {
		t.Errorf("nodes/{slug} misreads an invited row: %v %v", node["is_member"], node["is_banned"])
	}

	// Member count on the node payload (nested under "node").
	inner, _ := node["node"].(map[string]interface{})
	if c, _ := inner["member_count"].(float64); c != 2 {
		t.Errorf("node member_count counted the invitation: got %v, want 2", c)
	}
}

// An admin cannot answer for the person: not by approving the row, and not
// by setting the role it will carry once they do.
func TestAdminCannotAnswerAnInvitation(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-consent")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	patch := func(body interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/nodes/"+f.slug+"/members/"+f.invitee, body, f.adminToken)
		return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	}
	if w := patch(map[string]string{"status": "active"}); w.Code != http.StatusBadRequest {
		t.Errorf("approve an invited row: expected 400, got %d", w.Code)
	}
	if w := patch(map[string]string{"role": "admin"}); w.Code != http.StatusBadRequest {
		t.Errorf("promote an invited row: expected 400, got %d", w.Code)
	}
	var status, role string
	db.QueryRow("SELECT status, role FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&status, &role)
	if status != "invited" || role != "member" {
		t.Errorf("row changed: %s/%s", status, role)
	}
}

func TestRescindTakesBackAnInvitationAndNothingElse(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-rescind")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	for name, token := range map[string]string{"a member": f.memberToken, "an instance admin": f.siteToken, "the invitee": f.inviteeToken} {
		if w := rescind(t, db, f.slug, f.invitee, token); w.Code != http.StatusNotFound {
			t.Errorf("%s rescinding: expected 404, got %d", name, w.Code)
		}
	}
	// Never a quieter way to remove a member.
	memberID := ""
	db.QueryRow("SELECT user_id FROM memberships WHERE node_id = ? AND role = 'member' AND status = 'active'", f.nodeID).Scan(&memberID)
	if w := rescind(t, db, f.slug, memberID, f.adminToken); w.Code != http.StatusNotFound {
		t.Errorf("rescind on an active member: expected 404, got %d", w.Code)
	}
	if status, _ := membershipRow(t, db, memberID, f.nodeID); status != "active" {
		t.Errorf("the member's row was touched: %q", status)
	}

	if w := rescind(t, db, f.slug, f.invitee, f.adminToken); w.Code != http.StatusOK {
		t.Fatalf("rescind: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ?", f.invitee, f.nodeID).Scan(&n)
	if n != 0 {
		t.Errorf("expected the row gone, found %d", n)
	}
	if n := auditCount(t, db, f.admin, "membership.rescind_invite"); n != 1 {
		t.Errorf("expected 1 membership.rescind_invite audit row, got %d", n)
	}
	// Answering a rescinded invitation: nothing to answer.
	if w := answerInvitation(t, db, f.slug, "accept", f.inviteeToken); w.Code != http.StatusBadRequest {
		t.Errorf("accept after rescind: expected 400, got %d", w.Code)
	}
}
