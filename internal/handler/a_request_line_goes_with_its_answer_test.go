package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A join request's notification outlived the request (F-045): approving left
// the line in every admin's bell and the badge went on counting it, so an
// admin read three lines, a badge saying two, and a queue saying none.
// Answering a request now takes its line down, for every admin it reached.
//
// And an invitation named nobody on the page that showed it (F-044): the
// membership now records who made it, and both of its readers say so.

// requestLines counts the membership.request notifications one person holds.
func requestLines(t *testing.T, db *database.DB, userID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'membership.request'`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// askToJoin sends a member request and waits until both admins hold its line.
func askToJoin(t *testing.T, db *database.DB, slug, token string, admins ...string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/join", map[string]string{"role": "member"}, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"pending"`) {
		t.Fatalf("request: %d %s", w.Code, w.Body.String())
	}
	for _, a := range admins {
		waitFor(t, "the request line for "+a, func() bool { return requestLines(t, db, a) == 1 })
	}
}

func answerRequest(t *testing.T, db *database.DB, slug, userID, status, token string) {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/nodes/"+slug+"/members/"+userID, map[string]string{"status": status}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("answer %s: %d %s", status, w.Code, w.Body.String())
	}
}

func TestAnsweringARequestTakesItsLineDown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(t *testing.T, db *database.DB, slug, askerID, askerToken, adminToken string)
	}{
		{"approve", func(t *testing.T, db *database.DB, slug, askerID, _, adminToken string) {
			answerRequest(t, db, slug, askerID, "active", adminToken)
		}},
		{"reject", func(t *testing.T, db *database.DB, slug, askerID, _, adminToken string) {
			answerRequest(t, db, slug, askerID, "left", adminToken)
		}},
		{"withdraw", func(t *testing.T, db *database.DB, slug, _, askerToken, _ string) {
			r := authedRequest("POST", "/api/v1/nodes/"+slug+"/withdraw", nil, askerToken)
			w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/withdraw", handler.WithdrawMembershipRequest(db), r)
			if w.Code != http.StatusOK {
				t.Fatalf("withdraw: %d %s", w.Code, w.Body.String())
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			tellingPeople(t, db)
			slug := "omar-" + tc.name
			omar, omarToken := createTestUser(t, db, slug+"-admin", "member")
			lou, _ := createTestUser(t, db, slug+"-coadmin", "member")
			nodeID := createTestNode(t, db, omar.ID, "Bobbin Hall "+tc.name, slug, "approval_required")
			createTestMembership(t, db, omar.ID, nodeID, "admin", "active")
			createTestMembership(t, db, lou.ID, nodeID, "admin", "active")

			// Two people ask; answering one leaves the other's line alone.
			bea, beaToken := createTestUser(t, db, slug+"-bea", "member")
			askToJoin(t, db, slug, beaToken, omar.ID, lou.ID)
			_, clydeToken := createTestUser(t, db, slug+"-clyde", "member")
			r := authedRequest("POST", "/api/v1/nodes/"+slug+"/join", map[string]string{"role": "member"}, clydeToken)
			serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
			waitFor(t, "both lines", func() bool { return requestLines(t, db, omar.ID) == 2 && requestLines(t, db, lou.ID) == 2 })

			tc.answer(t, db, slug, bea.ID, beaToken, omarToken)

			// Both admins, not only the one who answered: the line was about
			// the request, and the request is gone.
			for _, a := range []string{omar.ID, lou.ID} {
				if got := requestLines(t, db, a); got != 1 {
					t.Errorf("after %s, admin %s holds %d request lines, want 1 (Clyde's)", tc.name, a, got)
				}
			}
			var body string
			db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = 'membership.request'`, omar.ID).Scan(&body)
			if strings.Contains(body, bea.DisplayName) {
				t.Errorf("the line left standing is Bea's, whose request was answered: %q", body)
			}
		})
	}
}

// A follower asking to become a member on an approval-required patch had
// asked, not joined, and was announced as "New member joined".
func TestAFollowersRequestIsARequest(t *testing.T) {
	db := setupTestDB(t)
	tellingPeople(t, db)
	admin, adminToken := createTestUser(t, db, "fr-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Follow Room", "follow-room", "approval_required")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	teo, teoToken := createTestUser(t, db, "fr-teo", "member")
	createTestMembership(t, db, teo.ID, nodeID, "follower", "active")

	askToJoin(t, db, "follow-room", teoToken, admin.ID)
	time.Sleep(200 * time.Millisecond)
	var joined int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = 'membership.joined'`, admin.ID).Scan(&joined)
	if joined != 0 {
		t.Errorf("a request was announced as a join (%d joined lines)", joined)
	}

	answerRequest(t, db, "follow-room", teo.ID, "active", adminToken)
	if got := requestLines(t, db, admin.ID); got != 0 {
		t.Errorf("approving the follower's request left %d lines", got)
	}
}

// The rows already in people's bells carry no tie, so the migration takes
// down the ones it can prove were answered and leaves the ones it cannot.
func TestMigration_AnsweredRequestLinesGo(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "mig-admin", "member")
	quiet := createTestNode(t, db, admin.ID, "Quiet", "quiet", "approval_required")
	busy := createTestNode(t, db, admin.ID, "Busy", "busy", "approval_required")
	asker, _ := createTestUser(t, db, "mig-asker", "member")
	pending := createTestMembership(t, db, asker.ID, busy, "member", "pending")
	db.Exec(`UPDATE memberships SET joined_at = '2026-10-10T12:00:00.000Z' WHERE id = ?`, pending)

	seed := func(nodeID, created string) string {
		id := seedNotification(t, db, admin.ID, "membership.request", "/patches/x/settings/members")
		db.Exec(`UPDATE notifications SET node_id = ?, created_at = ? WHERE id = ?`, nodeID, created, id)
		return id
	}
	onQuiet := seed(quiet, "2026-10-11T12:00:00.000Z")   // no request open there at all
	beforeOpen := seed(busy, "2026-10-09T12:00:00.000Z") // older than the open request
	aboutOpen := seed(busy, "2026-10-10T12:00:00.100Z")  // may be the open request's line
	other := seedNotification(t, db, admin.ID, "membership.joined", "/patches/busy/members")

	sql, err := patchwork.MigrationsFS.ReadFile("migrations/20261111T225259_membership_request_notifications.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	// The ALTERs have already run on this database; take the DELETE alone.
	var del string
	for _, stmt := range strings.Split(string(sql), ";") {
		var lines []string
		for _, l := range strings.Split(stmt, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(l), "--") {
				lines = append(lines, l)
			}
		}
		if s := strings.TrimSpace(strings.Join(lines, "\n")); strings.HasPrefix(s, "DELETE FROM notifications") {
			del = s
		}
	}
	if del == "" {
		t.Fatal("migration has no DELETE FROM notifications")
	}
	if _, err := db.Exec(del); err != nil {
		t.Fatalf("run: %v", err)
	}

	exists := func(id string) bool {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE id = ?`, id).Scan(&n)
		return n == 1
	}
	if exists(onQuiet) {
		t.Error("a request line on a patch with no open request survived")
	}
	if exists(beforeOpen) {
		t.Error("a request line older than every open request survived")
	}
	if !exists(aboutOpen) {
		t.Error("a line that may be about the open request was deleted")
	}
	if !exists(other) {
		t.Error("a notification of another type was deleted")
	}
}

func TestAnInvitationNamesWhoMadeIt(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-by")
	if w := invite(t, db, f.slug, f.inviteeName, f.adminToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}

	// The invitee's own list.
	r := authedRequest("GET", "/api/v1/users/me/invitations", nil, f.inviteeToken)
	w := serveMux(t, db, "GET", "/api/v1/users/me/invitations", handler.ListMyInvitations(db), r)
	var mine struct {
		Items []struct {
			InvitedBy struct {
				ID          string `json:"id"`
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
			} `json:"invited_by"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &mine)
	if len(mine.Items) != 1 || mine.Items[0].InvitedBy.ID != f.admin || mine.Items[0].InvitedBy.Username != f.slug+"-admin" {
		t.Fatalf("the invitee's invitation should name the admin who made it, got %s", w.Body.String())
	}

	// The admins' Invited list, on the members payload.
	payload := membersPayload(t, db, "/api/v1/nodes/"+f.slug+"/members", f.adminToken)
	invited, _ := payload["invited"].([]interface{})
	if len(invited) != 1 {
		t.Fatalf("invited = %v", payload["invited"])
	}
	by, _ := invited[0].(map[string]interface{})["invited_by"].(map[string]interface{})
	if by["id"] != f.admin {
		t.Errorf("the admins' invited row should name the inviter, got %v", invited[0])
	}
	// Still the admins' alone (docs/adr/098).
	if _, has := membersPayload(t, db, "/api/v1/nodes/"+f.slug+"/members", f.memberToken)["invited"]; has {
		t.Error("a member was sent the invited list")
	}

	// The column describes the invitation, so answering it clears it.
	if w := answerInvitation(t, db, f.slug, "accept", f.inviteeToken); w.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", w.Code, w.Body.String())
	}
	var after *string
	db.QueryRow(`SELECT invited_by FROM memberships WHERE user_id = ? AND node_id = ?`, f.invitee, f.nodeID).Scan(&after)
	if after != nil {
		t.Errorf("an accepted membership still carries invited_by = %q", *after)
	}
}

// A re-invitation on a row the person had left names the admin asking now.
func TestAReinvitationNamesTheAdminAskingNow(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-again")
	second, secondToken := createTestUser(t, db, "inv-again-second", "member")
	createTestMembership(t, db, second.ID, f.nodeID, "admin", "active")
	createTestMembership(t, db, f.invitee, f.nodeID, "member", "left")

	if w := invite(t, db, f.slug, f.inviteeName, secondToken); w.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	var by string
	db.QueryRow(`SELECT COALESCE(invited_by, '') FROM memberships WHERE user_id = ? AND node_id = ?`, f.invitee, f.nodeID).Scan(&by)
	if by != second.ID {
		t.Errorf("invited_by = %q, want the second admin", by)
	}
}

func TestPersonalExport_AnInvitationCarriesItsInviter(t *testing.T) {
	db := setupTestDB(t)
	f := newInviteFixture(t, db, "inv-export")
	invite(t, db, f.slug, f.inviteeName, f.adminToken)

	w := requestExport(t, db, f.inviteeToken)
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
	var doc map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &doc)
	var found bool
	for _, m := range exportSection(t, doc, "memberships") {
		row := m.(map[string]interface{})
		if row["status"] != "invited" {
			continue
		}
		found = true
		if row["invited_by"] != f.admin || row["invited_by_name"] == nil || row["invited_by_name"] == "" {
			t.Errorf("the exported invitation should name its inviter, got %v", row)
		}
	}
	if !found {
		t.Fatal("the export carries no invited row")
	}
}
