package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// The noticeboard (docs/adr/081). Every route is members-only, the check in
// the handler, and the room is exactly the patch's active admins and
// members — not followers, not an instance admin holding no role here.

type room struct {
	db                                         *database.DB
	nodeID, slug                               string
	adminID, memberID, otherID, followerID     string
	adminTok, memberTok, otherTok, followerTok string
	strangerTok, siteAdminTok, elsewhereTok    string
	elsewhereNodeID                            string
}

func newRoom(t *testing.T) *room {
	t.Helper()
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, "nbadmin", "member")
	member, memberTok := createTestUser(t, db, "nbmember", "member")
	other, otherTok := createTestUser(t, db, "nbother", "member")
	follower, followerTok := createTestUser(t, db, "nbfollower", "member")
	_, strangerTok := createTestUser(t, db, "nbstranger", "member")
	_, siteAdminTok := createTestUser(t, db, "nbsiteadmin", "admin")
	elsewhere, elsewhereTok := createTestUser(t, db, "nbelsewhere", "member")

	nodeID := createTestNode(t, db, admin.ID, "Room", "room", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	elsewhereNode := createTestNode(t, db, elsewhere.ID, "Elsewhere", "elsewhere", "open")
	createTestMembership(t, db, elsewhere.ID, elsewhereNode, "admin", "active")

	return &room{db: db, nodeID: nodeID, slug: "room",
		adminID: admin.ID, memberID: member.ID, otherID: other.ID, followerID: follower.ID,
		adminTok: adminTok, memberTok: memberTok, otherTok: otherTok, followerTok: followerTok,
		strangerTok: strangerTok, siteAdminTok: siteAdminTok, elsewhereTok: elsewhereTok,
		elsewhereNodeID: elsewhereNode}
}

func (rm *room) do(t *testing.T, method, pattern, path string, body interface{}, token string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest(method, path, body, token)
	return serveMux(t, rm.db, method, pattern, h, r)
}

func (rm *room) post(t *testing.T, token string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	w := rm.do(t, "POST", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/"+rm.slug+"/notices", body, token, handler.CreateNotice(rm.db))
	if w.Code != http.StatusCreated {
		t.Fatalf("create notice: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

func (rm *room) reply(t *testing.T, token, noticeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return rm.do(t, "POST", "/api/v1/notices/{id}/replies", "/api/v1/notices/"+noticeID+"/replies",
		map[string]string{"body": body}, token, handler.CreateReply(rm.db))
}

func TestNoticeboardIsTheRoomAndNobodyElse(t *testing.T) {
	rm := newRoom(t)
	n := rm.post(t, rm.memberTok, map[string]interface{}{"title": "PA is broken", "body": "who has one?"})
	noticeID := n["id"].(string)

	cases := []struct {
		name  string
		token string
		want  int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"signed-in stranger", rm.strangerTok, http.StatusNotFound},
		{"follower", rm.followerTok, http.StatusNotFound},
		{"instance admin with no role here", rm.siteAdminTok, http.StatusNotFound},
		{"admin of another patch", rm.elsewhereTok, http.StatusNotFound},
		{"member", rm.memberTok, http.StatusOK},
		{"patch admin", rm.adminTok, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name+" list", func(t *testing.T) {
			w := rm.do(t, "GET", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/room/notices", nil, c.token, handler.ListNotices(rm.db))
			if w.Code != c.want {
				t.Errorf("want %d, got %d: %s", c.want, w.Code, w.Body.String())
			}
			if c.want == http.StatusOK && !strings.Contains(w.Body.String(), "PA is broken") {
				t.Errorf("room member should see the notice: %s", w.Body.String())
			}
		})
		t.Run(c.name+" detail", func(t *testing.T) {
			w := rm.do(t, "GET", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID, nil, c.token, handler.GetNotice(rm.db))
			if w.Code != c.want {
				t.Errorf("want %d, got %d: %s", c.want, w.Code, w.Body.String())
			}
		})
		t.Run(c.name+" replies", func(t *testing.T) {
			w := rm.do(t, "GET", "/api/v1/notices/{id}/replies", "/api/v1/notices/"+noticeID+"/replies", nil, c.token, handler.ListReplies(rm.db))
			if w.Code != c.want {
				t.Errorf("want %d, got %d: %s", c.want, w.Code, w.Body.String())
			}
		})
	}
}

func TestNoticePostingPolicy(t *testing.T) {
	rm := newRoom(t)
	body := map[string]interface{}{"title": "Meeting moved", "body": "to Thursday"}

	// Default: members too.
	rm.post(t, rm.memberTok, body)

	// Admins only: a member is refused, an admin is not, and the list says so.
	rm.db.Exec("UPDATE nodes SET notice_posting = 'admins' WHERE id = ?", rm.nodeID)
	w := rm.do(t, "POST", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/room/notices", body, rm.memberTok, handler.CreateNotice(rm.db))
	if w.Code != http.StatusForbidden {
		t.Errorf("member under admins-only: want 403, got %d: %s", w.Code, w.Body.String())
	}
	rm.post(t, rm.adminTok, body)
	w = rm.do(t, "GET", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/room/notices", nil, rm.memberTok, handler.ListNotices(rm.db))
	if res := decodeJSON(t, w); res["may_post"] != false || res["notice_posting"] != "admins" {
		t.Errorf("list should tell a member they may not post: %v", res)
	}

	// Validation: a title is required, and the image reference is checked.
	for _, bad := range []map[string]interface{}{
		{"title": "", "body": "x"},
		{"title": strings.Repeat("t", 141)},
		{"title": "ok", "image_url": "http://insecure.example/x.png", "image_alt": "x"},
		{"title": "ok", "image_url": "https://ok.example/x.png"}, // no alt
	} {
		w := rm.do(t, "POST", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/room/notices", bad, rm.adminTok, handler.CreateNotice(rm.db))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%v: want 400, got %d: %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestRepliesAreFlatAndTheSwitchIsFlippable(t *testing.T) {
	rm := newRoom(t)
	n := rm.post(t, rm.memberTok, map[string]interface{}{"title": "Potluck", "body": "bring a dish"})
	noticeID := n["id"].(string)
	if n["replies_open"] != true {
		t.Fatalf("patch default is replies on: %v", n)
	}

	// Anyone in the room may reply; a follower may not.
	if w := rm.reply(t, rm.otherTok, noticeID, "I'll bring bread"); w.Code != http.StatusCreated {
		t.Fatalf("member reply: want 201, got %d: %s", w.Code, w.Body.String())
	}
	if w := rm.reply(t, rm.followerTok, noticeID, "me too"); w.Code != http.StatusNotFound {
		t.Errorf("follower reply: want 404, got %d", w.Code)
	}

	// The author switches replies off; the box goes, the reply stays.
	w := rm.do(t, "PATCH", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID,
		map[string]interface{}{"replies_open": false}, rm.memberTok, handler.UpdateNotice(rm.db))
	if w.Code != http.StatusOK {
		t.Fatalf("author switch: want 200, got %d: %s", w.Code, w.Body.String())
	}
	if w := rm.reply(t, rm.otherTok, noticeID, "late"); w.Code != http.StatusForbidden {
		t.Errorf("reply after switch off: want 403, got %d", w.Code)
	}
	w = rm.do(t, "GET", "/api/v1/notices/{id}/replies", "/api/v1/notices/"+noticeID+"/replies", nil, rm.otherTok, handler.ListReplies(rm.db))
	if items, _ := decodeJSON(t, w)["items"].([]interface{}); len(items) != 1 {
		t.Errorf("existing replies stay readable after the switch: got %d", len(items))
	}

	// An admin flips it back on; another member cannot; only the author edits.
	w = rm.do(t, "PATCH", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID,
		map[string]interface{}{"replies_open": true}, rm.adminTok, handler.UpdateNotice(rm.db))
	if w.Code != http.StatusOK {
		t.Errorf("admin switch: want 200, got %d: %s", w.Code, w.Body.String())
	}
	w = rm.do(t, "PATCH", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID,
		map[string]interface{}{"replies_open": false}, rm.otherTok, handler.UpdateNotice(rm.db))
	if w.Code != http.StatusForbidden {
		t.Errorf("other member switch: want 403, got %d", w.Code)
	}
	w = rm.do(t, "PATCH", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID,
		map[string]interface{}{"title": "Potluck!"}, rm.adminTok, handler.UpdateNotice(rm.db))
	if w.Code != http.StatusForbidden {
		t.Errorf("admin editing the body: want 403, got %d", w.Code)
	}

	// A reply is removed by its author or an admin, not by another member.
	w = rm.do(t, "GET", "/api/v1/notices/{id}/replies", "/api/v1/notices/"+noticeID+"/replies", nil, rm.otherTok, handler.ListReplies(rm.db))
	replyID := decodeJSON(t, w)["items"].([]interface{})[0].(map[string]interface{})["id"].(string)
	if w := rm.do(t, "DELETE", "/api/v1/replies/{id}", "/api/v1/replies/"+replyID, nil, rm.memberTok, handler.DeleteReply(rm.db)); w.Code != http.StatusForbidden {
		t.Errorf("notice author deleting someone's reply: want 403, got %d", w.Code)
	}
	if w := rm.do(t, "DELETE", "/api/v1/replies/{id}", "/api/v1/replies/"+replyID, nil, rm.adminTok, handler.DeleteReply(rm.db)); w.Code != http.StatusOK {
		t.Errorf("admin deleting a reply: want 200, got %d", w.Code)
	}

	// The notice comes down by its author or an admin, and takes its replies.
	rm.reply(t, rm.otherTok, noticeID, "again")
	if w := rm.do(t, "DELETE", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID, nil, rm.otherTok, handler.DeleteNotice(rm.db)); w.Code != http.StatusForbidden {
		t.Errorf("other member deleting the notice: want 403, got %d", w.Code)
	}
	if w := rm.do(t, "DELETE", "/api/v1/notices/{id}", "/api/v1/notices/"+noticeID, nil, rm.adminTok, handler.DeleteNotice(rm.db)); w.Code != http.StatusOK {
		t.Errorf("admin deleting the notice: want 200, got %d", w.Code)
	}
	var left int
	rm.db.QueryRow("SELECT COUNT(*) FROM notice_replies WHERE notice_id = ?", noticeID).Scan(&left)
	if left != 0 {
		t.Errorf("replies should go with the notice, %d left", left)
	}
}

// A notice is born quiet; a reply reaches participants only.
func TestNoticeboardBellIsQuietByDefault(t *testing.T) {
	rm := newRoom(t)
	handler.SetNotifier(notifications.NewNotifier(rm.db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	quiet := rm.post(t, rm.memberTok, map[string]interface{}{"title": "Quiet", "body": "nobody told"})
	if got := countNotifications(t, rm.db, rm.otherID, notifications.NoticePosted, 0); got != 0 {
		t.Errorf("a notice without Tell members rang the bell %d times", got)
	}
	if quiet["members_told"] != false {
		t.Errorf("members_told should be false: %v", quiet)
	}

	told := rm.post(t, rm.memberTok, map[string]interface{}{"title": "Told", "body": "everyone", "tell_members": true})
	if told["members_told"] != true {
		t.Errorf("members_told should be recorded on the notice: %v", told)
	}
	if got := countNotifications(t, rm.db, rm.otherID, notifications.NoticePosted, 1); got != 1 {
		t.Errorf("Tell members should reach a fellow member: got %d", got)
	}
	if got := countNotifications(t, rm.db, rm.followerID, notifications.NoticePosted, 0); got != 0 {
		t.Errorf("a follower is not in the room: got %d", got)
	}
	if got := countNotifications(t, rm.db, rm.memberID, notifications.NoticePosted, 0); got != 0 {
		t.Errorf("the author is not told about their own notice: got %d", got)
	}

	// Replies: the author hears; a member who never replied does not; once
	// they reply, they are in.
	noticeID := told["id"].(string)
	rm.reply(t, rm.otherTok, noticeID, "first")
	if got := countNotifications(t, rm.db, rm.memberID, notifications.NoticeReply, 1); got != 1 {
		t.Errorf("author should hear the first reply: got %d", got)
	}
	if got := countNotifications(t, rm.db, rm.adminID, notifications.NoticeReply, 0); got != 0 {
		t.Errorf("an admin who never replied is not a participant: got %d", got)
	}
	rm.reply(t, rm.adminTok, noticeID, "second")
	if got := countNotifications(t, rm.db, rm.otherID, notifications.NoticeReply, 1); got != 1 {
		t.Errorf("earlier replier should hear the second reply: got %d", got)
	}
}

// Reports about the room go to the room's admins, never the instance panel.
func TestNoticeReportsGoToThePatch(t *testing.T) {
	rm := newRoom(t)
	handler.SetNotifier(notifications.NewNotifier(rm.db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	n := rm.post(t, rm.otherTok, map[string]interface{}{"title": "Rude", "body": "…"})
	noticeID := n["id"].(string)
	rw := rm.reply(t, rm.otherTok, noticeID, "ruder")
	replyID := decodeJSON(t, rw)["id"].(string)

	report := func(token, typ, id string) *httptest.ResponseRecorder {
		return rm.do(t, "POST", "/api/v1/reports", "/api/v1/reports",
			map[string]interface{}{"entity_type": typ, "entity_id": id, "reason": "harassment"}, token, handler.CreateReport(rm.db))
	}
	// Only someone in the room can report what is in it.
	if w := report(rm.followerTok, "notice", noticeID); w.Code != http.StatusNotFound {
		t.Errorf("follower reporting a notice: want 404, got %d: %s", w.Code, w.Body.String())
	}
	if w := report(rm.memberTok, "notice", noticeID); w.Code != http.StatusCreated {
		t.Fatalf("member reporting a notice: want 201, got %d: %s", w.Code, w.Body.String())
	}
	if w := report(rm.memberTok, "reply", replyID); w.Code != http.StatusCreated {
		t.Fatalf("member reporting a reply: want 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := countNotifications(t, rm.db, rm.adminID, notifications.NoticeReported, 2); got != 2 {
		t.Errorf("patch admin should hear both reports: got %d", got)
	}

	// The instance panel does not list them.
	r := authedRequest("GET", "/api/v1/admin/reports?status=pending", nil, rm.siteAdminTok)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/reports", middleware.AdminRequired(rm.db, handler.ListReports(rm.db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if items, _ := decodeJSON(t, w)["items"].([]interface{}); len(items) != 0 {
		t.Errorf("instance panel must not list room reports: %v", items)
	}

	// The patch queue does — for a patch admin, and not for an instance
	// admin with no role here even though RequireNodeRole would let them by.
	queue := func(token string) *httptest.ResponseRecorder {
		r := authedRequest("GET", "/api/v1/nodes/room/reports", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/nodes/{slug}/reports", middleware.AuthRequired(rm.db, middleware.RequireNodeRole(rm.db, "admin")(handler.ListPatchReports(rm.db))))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := queue(rm.siteAdminTok); w.Code != http.StatusNotFound {
		t.Errorf("instance admin on the patch queue: want 404, got %d", w.Code)
	}
	if w := queue(rm.memberTok); w.Code != http.StatusForbidden {
		t.Errorf("member on the patch queue: want 403, got %d", w.Code)
	}
	w = queue(rm.adminTok)
	if w.Code != http.StatusOK {
		t.Fatalf("patch admin queue: want 200, got %d: %s", w.Code, w.Body.String())
	}
	items, _ := decodeJSON(t, w)["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("queue should hold both reports: %v", items)
	}
	var noticeReport, replyReport string
	for _, it := range items {
		m := it.(map[string]interface{})
		switch m["entity_type"] {
		case "notice":
			noticeReport = m["id"].(string)
			if m["target"] != "Rude" || m["notice_id"] != noticeID {
				t.Errorf("notice report should quote the notice: %v", m)
			}
		case "reply":
			replyReport = m["id"].(string)
			if m["target"] != "ruder" || m["notice_id"] != noticeID {
				t.Errorf("reply report should quote the reply and point at its notice: %v", m)
			}
		}
	}

	resolve := func(token, id string, body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/nodes/room/reports/"+id, body, token)
		mux := http.NewServeMux()
		mux.HandleFunc("PATCH /api/v1/nodes/{slug}/reports/{id}", middleware.AuthRequired(rm.db, middleware.RequireNodeRole(rm.db, "admin")(handler.UpdatePatchReport(rm.db))))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	// Forum tools are refused; the kit's three are not.
	if w := resolve(rm.adminTok, replyReport, map[string]interface{}{"action": "suspend_user"}); w.Code != http.StatusBadRequest {
		t.Errorf("suspend_user: want 400, got %d", w.Code)
	}
	if w := resolve(rm.adminTok, replyReport, map[string]interface{}{"action": "close_replies"}); w.Code != http.StatusOK {
		t.Errorf("close_replies: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var open int
	rm.db.QueryRow("SELECT replies_open FROM notices WHERE id = ?", noticeID).Scan(&open)
	if open != 0 {
		t.Errorf("close_replies on a reply report should switch the notice's replies off")
	}
	if w := resolve(rm.adminTok, noticeReport, map[string]interface{}{"action": "remove"}); w.Code != http.StatusOK {
		t.Errorf("remove: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var left int
	rm.db.QueryRow("SELECT COUNT(*) FROM notices WHERE id = ?", noticeID).Scan(&left)
	if left != 0 {
		t.Errorf("remove should take the notice down")
	}
	if got := countNotifications(t, rm.db, rm.memberID, "report.resolved", 2); got != 2 {
		t.Errorf("reporter should hear both reviews: got %d", got)
	}
}

// The two settings are patch admin's, validated, and echoed on the node.
func TestNoticeboardSettings(t *testing.T) {
	rm := newRoom(t)
	patch := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/nodes/room", body, rm.adminTok)
		mux := http.NewServeMux()
		mux.HandleFunc("PATCH /api/v1/nodes/{slug}", middleware.AuthRequired(rm.db, middleware.RequireNodeRole(rm.db, "admin")(handler.UpdateNode(rm.db))))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := patch(map[string]interface{}{"notice_posting": "everyone"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad posting policy: want 400, got %d: %s", w.Code, w.Body.String())
	}
	if w := patch(map[string]interface{}{"notice_replies_default": "no"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad replies default: want 400, got %d", w.Code)
	}
	w := patch(map[string]interface{}{"notice_posting": "admins", "notice_replies_default": false})
	if w.Code != http.StatusOK {
		t.Fatalf("settings: want 200, got %d: %s", w.Code, w.Body.String())
	}
	if res := decodeJSON(t, w); res["notice_posting"] != "admins" || res["notice_replies_default"] != false {
		t.Errorf("settings should echo on the node: %v", res)
	}
	// A new notice inherits the replies default.
	n := rm.post(t, rm.adminTok, map[string]interface{}{"title": "Read only", "body": "x"})
	if n["replies_open"] != false {
		t.Errorf("new notice should inherit replies_default=false: %v", n)
	}
	// The detail route carries both for the settings page.
	r := authedRequest("GET", "/api/v1/nodes/room", nil, rm.adminTok)
	w = serveOptionalMux(t, rm.db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(rm.db), r)
	node := decodeJSON(t, w)["node"].(map[string]interface{})
	if node["notice_posting"] != "admins" || node["notice_replies_default"] != false {
		t.Errorf("GetNode should carry the settings: %v", node)
	}
}
