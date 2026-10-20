package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Membership notices named the patch and nobody else, so an admin read seven
// identical "New member joined" lines and had to open the page to learn who
// (F-044); the request line opened a roster that could not show the request
// (F-043); and a role set by hand told the person nothing (F-041).

// noticeFor waits for the newest notification of a type to one person, since
// delivery is asynchronous.
func noticeFor(t *testing.T, db *database.DB, userID, notifType string) (title, body, link string) {
	t.Helper()
	waitFor(t, notifType+" for "+userID, func() bool {
		err := db.QueryRow(`SELECT title, COALESCE(body,''), link FROM notifications
		                    WHERE user_id = ? AND type = ? ORDER BY created_at DESC LIMIT 1`,
			userID, notifType).Scan(&title, &body, &link)
		return err == nil
	})
	return title, body, link
}

func TestMembershipNotices_SayWhoJoinedAndWhoAsked(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, _ := createTestUser(t, db, "who-admin", "member")

	openID := createTestNode(t, db, admin.ID, "Open Room", "open-room", "open")
	createTestMembership(t, db, admin.ID, openID, "admin", "active")
	joiner, joinerToken := createTestUser(t, db, "kofi-mensah", "member")
	r := authedRequest("POST", "/api/v1/nodes/open-room/join", map[string]string{"role": "member"}, joinerToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r); w.Code >= 300 {
		t.Fatalf("join: %d %s", w.Code, w.Body.String())
	}
	if _, body, _ := noticeFor(t, db, admin.ID, "membership.joined"); body != joiner.DisplayName+" joined." {
		t.Errorf("the joined notice should say who, got body %q", body)
	}

	gatedID := createTestNode(t, db, admin.ID, "Gated Room", "gated-room", "approval_required")
	createTestMembership(t, db, admin.ID, gatedID, "admin", "active")
	asker, askerToken := createTestUser(t, db, "harriet-boone", "member")
	r = authedRequest("POST", "/api/v1/nodes/gated-room/join", map[string]string{"role": "member"}, askerToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r); w.Code >= 300 {
		t.Fatalf("request: %d %s", w.Code, w.Body.String())
	}
	_, body, link := noticeFor(t, db, admin.ID, "membership.request")
	if body != asker.DisplayName+" asked to join." {
		t.Errorf("the request notice should say who, got body %q", body)
	}
	if link != "/patches/gated-room/settings/members" {
		t.Errorf("the request notice should open the queue it asks the admin to answer, got %q", link)
	}
}

func TestMembershipNotices_AnInvitationSaysWhoAsked(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "june-okafor", "member")
	nodeID := createTestNode(t, db, admin.ID, "Tin Roof", "tin-roof", "invite_only")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	teo, _ := createTestUser(t, db, "teo-marsh", "member")

	r := authedRequest("POST", "/api/v1/nodes/tin-roof/invitations", map[string]string{"username": teo.Username}, adminToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/invitations", handler.InviteMember(db), r); w.Code >= 300 {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	if _, body, _ := noticeFor(t, db, teo.ID, "membership.invited"); body != admin.DisplayName+" invited you." {
		t.Errorf("the invitation should say who asked, got body %q", body)
	}
}

func TestRoleChange_TellsThePersonWhoseRoleItIs(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "priya-n", "member")
	nodeID := createTestNode(t, db, admin.ID, "Bobbin Hall", "bobbin-hall", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	tam, _ := createTestUser(t, db, "tam-nguyen", "member")
	createTestMembership(t, db, tam.ID, nodeID, "member", "active")

	setRole := func(role string) {
		t.Helper()
		r := authedRequest("PATCH", "/api/v1/nodes/bobbin-hall/members/"+tam.ID, map[string]string{"role": role}, adminToken)
		w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("set role %s: %d %s", role, w.Code, w.Body.String())
		}
	}
	roleNotices := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'membership.role_changed'`, tam.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	setRole("admin")
	title, body, _ := noticeFor(t, db, tam.ID, "membership.role_changed")
	if title != "You are now an admin of Bobbin Hall" {
		t.Errorf("promotion title: got %q", title)
	}
	if !strings.HasPrefix(body, admin.DisplayName+" made you an admin.") || !strings.Contains(body, "settings") {
		t.Errorf("a promotion should say who and what the role lets them do, got %q", body)
	}

	setRole("member")
	waitFor(t, "the demotion notice", func() bool { return roleNotices() == 2 })
	var demoted string
	if err := db.QueryRow(`SELECT title FROM notifications WHERE user_id = ? AND type = 'membership.role_changed'
	                       ORDER BY created_at DESC, id DESC LIMIT 1`, tam.ID).Scan(&demoted); err != nil {
		t.Fatalf("read: %v", err)
	}
	if demoted != "You are no longer an admin of Bobbin Hall" {
		t.Errorf("demotion title: got %q", demoted)
	}

	// Setting the role it already has tells nobody anything. Delivery is
	// asynchronous, so give a stray notice time to land before counting.
	setRole("member")
	time.Sleep(300 * time.Millisecond)
	if got := roleNotices(); got != 2 {
		t.Errorf("an unchanged role should notify nobody, got %d notices", got)
	}
}

// Rows already in people's bells carry the old link; the migration rewrites
// them in place, by suffix, so a patch slugged "members" is not spliced at its
// own slug.
func TestMigration_JoinRequestLinksOpenTheQueue(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "linkadmin", "member")
	old := seedNotification(t, db, user.ID, "membership.request", "/patches/gallery-row/members?status=pending")
	tricky := seedNotification(t, db, user.ID, "membership.request", "/patches/members/members?status=pending")
	roster := seedNotification(t, db, user.ID, "membership.joined", "/patches/gallery-row/members")

	sql, err := patchwork.MigrationsFS.ReadFile("migrations/20261020T013027_join_request_links.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(sql)); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	if got := linkOf(t, db, old); got != "/patches/gallery-row/settings/members" {
		t.Errorf("old request link: got %q", got)
	}
	if got := linkOf(t, db, tricky); got != "/patches/members/settings/members" {
		t.Errorf("a patch slugged members: got %q", got)
	}
	if got := linkOf(t, db, roster); got != "/patches/gallery-row/members" {
		t.Errorf("the roster link must be untouched: got %q", got)
	}
}
