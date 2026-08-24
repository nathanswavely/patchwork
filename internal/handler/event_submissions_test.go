package handler_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

func submissionsCfg(enabled bool) *config.Config {
	return &config.Config{Submissions: config.Submissions{Enabled: enabled}}
}

func makeUnclaimed(t *testing.T, db *database.DB, nodeID string) {
	t.Helper()
	if _, err := db.Exec("UPDATE nodes SET status = 'unclaimed' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("make unclaimed: %v", err)
	}
}

func makeTrusted(t *testing.T, db *database.DB, userID string) {
	t.Helper()
	if _, err := db.Exec("UPDATE users SET trusted_contributor = 1 WHERE id = ?", userID); err != nil {
		t.Fatalf("make trusted: %v", err)
	}
}

func eventBody(nodeID, title string) map[string]interface{} {
	return map[string]interface{}{
		"node_id":   nodeID,
		"title":     title,
		"starts_at": "2027-01-01T20:00:00Z",
	}
}

func createEventVia(t *testing.T, db *database.DB, cfg *config.Config, token string, body map[string]interface{}) (model.Event, int) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/events", body, token)
	w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, cfg), r)
	var e model.Event
	json.Unmarshal(w.Body.Bytes(), &e)
	return e, w.Code
}

func eventStatusInDB(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var s string
	db.QueryRow("SELECT status FROM events WHERE id = ?", id).Scan(&s)
	return s
}

// A non-member's event on an unclaimed patch enters review; a trusted
// contributor's publishes directly; the grant is worthless on active
// patches (docs/adr/026).
func TestCreateEventSubmissionLadder(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	stranger, strangerToken := createTestUser(t, db, "stranger", "member")
	trusted, trustedToken := createTestUser(t, db, "trusty", "member")
	makeTrusted(t, db, trusted.ID)
	_ = stranger

	unclaimedID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, unclaimedID)
	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	// Stranger → unclaimed: pending.
	e, code := createEventVia(t, db, cfg, strangerToken, eventBody(unclaimedID, "Basement Show"))
	if code != 201 || e.Status != "pending_review" {
		t.Fatalf("stranger on unclaimed: code=%d status=%q, want 201 pending_review", code, e.Status)
	}

	// Trusted contributor → unclaimed: direct.
	e2, code := createEventVia(t, db, cfg, trustedToken, eventBody(unclaimedID, "Zine Fair"))
	if code != 201 || e2.Status != "active" {
		t.Fatalf("trusted on unclaimed: code=%d status=%q, want 201 active", code, e2.Status)
	}

	// Trusted contributor → active patch they don't belong to: still a
	// suggestion. The grant only waives the instance admin's own queue.
	e3, code := createEventVia(t, db, cfg, trustedToken, eventBody(activeID, "Open Mic"))
	if code != 201 || e3.Status != "pending_review" {
		t.Fatalf("trusted on active: code=%d status=%q, want 201 pending_review", code, e3.Status)
	}

	// Member → own active patch: direct, unchanged behavior.
	e4, code := createEventVia(t, db, cfg, mustToken(t, db, owner.ID), eventBody(activeID, "Members Night"))
	if code != 201 || e4.Status != "active" {
		t.Fatalf("member on own patch: code=%d status=%q, want 201 active", code, e4.Status)
	}
}

// mustToken creates a fresh session for an existing user.
func mustToken(t *testing.T, db *database.DB, userID string) string {
	t.Helper()
	token, err := auth.CreateSession(db, userID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	return token
}

func TestCreateEventSubmissionGates(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")

	activeID := createTestNode(t, db, owner.ID, "Closed Doors", "closed-doors", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	// Instance switch off: no submissions at all.
	_, code := createEventVia(t, db, submissionsCfg(false), strangerToken, eventBody(activeID, "Nope"))
	if code != 403 {
		t.Fatalf("submissions disabled: code=%d, want 403", code)
	}

	// Patch turned suggestions off: its calendar, its call.
	db.Exec("UPDATE nodes SET accept_event_suggestions = 0 WHERE id = ?", activeID)
	_, code = createEventVia(t, db, submissionsCfg(true), strangerToken, eventBody(activeID, "Still Nope"))
	if code != 403 {
		t.Fatalf("suggestions off: code=%d, want 403", code)
	}
}

// Pending events are invisible everywhere public until approved; approval
// publishes them.
func TestReviewApprovePublishes(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	unclaimedID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, unclaimedID)

	e, _ := createEventVia(t, db, cfg, strangerToken, eventBody(unclaimedID, "Basement Show"))

	// Invisible in the public list.
	r := authedRequest("GET", "/api/v1/events", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("pending event leaked into public list")
	}

	// In the instance admin's queue.
	r = authedRequest("GET", "/api/v1/admin/event-submissions", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/event-submissions", handler.ListAdminEventSubmissions(db), r)
	if w.Code != 200 || !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatalf("admin queue: code=%d, missing event", w.Code)
	}

	// A random member may not review.
	r = authedRequest("PATCH", "/api/v1/events/"+e.ID+"/review", map[string]string{"action": "approve"}, strangerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)
	if w.Code != 403 {
		t.Fatalf("stranger review: code=%d, want 403", w.Code)
	}

	// The instance admin approves — the event publishes.
	r = authedRequest("PATCH", "/api/v1/events/"+e.ID+"/review", map[string]string{"action": "approve"}, adminToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)
	if w.Code != 200 {
		t.Fatalf("approve: code=%d body=%s", w.Code, w.Body.String())
	}
	if s := eventStatusInDB(t, db, e.ID); s != "active" {
		t.Fatalf("after approve status=%q, want active", s)
	}

	r = authedRequest("GET", "/api/v1/events", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("approved event missing from public list")
	}
}

func TestReviewRejectDeletes(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	unclaimedID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, unclaimedID)

	e, _ := createEventVia(t, db, cfg, strangerToken, eventBody(unclaimedID, "Spam Show"))

	r := authedRequest("PATCH", "/api/v1/events/"+e.ID+"/review", map[string]string{"action": "reject", "note": "not a real event"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)
	if w.Code != 200 {
		t.Fatalf("reject: code=%d", w.Code)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", e.ID).Scan(&count)
	if count != 0 {
		t.Fatal("rejected event still in database")
	}
}

// Suggestions to an active patch are that patch's admins' to review —
// never the instance admin's queue.
func TestActivePatchSuggestionReviewedByPatchAdmin(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, ownerToken := createTestUser(t, db, "owner", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	activeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, activeID, "admin", "active")

	e, _ := createEventVia(t, db, cfg, strangerToken, eventBody(activeID, "Touring Band"))
	if e.Status != "pending_review" {
		t.Fatalf("status=%q, want pending_review", e.Status)
	}

	// Not in the instance admin's unclaimed queue.
	r := authedRequest("GET", "/api/v1/admin/event-submissions", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/event-submissions", handler.ListAdminEventSubmissions(db), r)
	if bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("active-patch suggestion leaked into the instance admin queue")
	}

	// In the patch admin's queue.
	r = authedRequest("GET", "/api/v1/nodes/gallery-row/event-submissions", nil, ownerToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-submissions", handler.ListNodeEventSubmissions(db), r)
	if w.Code != 200 || !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatalf("patch queue: code=%d, missing event", w.Code)
	}

	// Patch admin approves.
	r = authedRequest("PATCH", "/api/v1/events/"+e.ID+"/review", map[string]string{"action": "approve"}, ownerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)
	if w.Code != 200 {
		t.Fatalf("patch admin approve: code=%d", w.Code)
	}
	if s := eventStatusInDB(t, db, e.ID); s != "active" {
		t.Fatalf("after approve status=%q, want active", s)
	}
}

// Following grants no write rights. Direct posting is the member/admin
// rung of the ladder (docs/adr/026); a follower's event goes to review
// like a stranger's. This once called userHasMembership, which counts any
// active membership, so a follower published straight to the calendar.
func TestFollowerEventEntersReview(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	follower, followerToken := createTestUser(t, db, "follower", "member")
	member, memberToken := createTestUser(t, db, "member", "member")

	nodeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	if _, err := db.Exec("UPDATE nodes SET accept_event_suggestions = 1 WHERE id = ?", nodeID); err != nil {
		t.Fatalf("open suggestions: %v", err)
	}

	e, code := createEventVia(t, db, cfg, followerToken, eventBody(nodeID, "Follower's Show"))
	if code != 201 || e.Status != "pending_review" {
		t.Fatalf("follower: code=%d status=%q, want 201 pending_review", code, e.Status)
	}

	// The rung above still posts directly — this must not have been fixed
	// by simply refusing everyone.
	m, code := createEventVia(t, db, cfg, memberToken, eventBody(nodeID, "Member's Show"))
	if code != 201 || m.Status != "active" {
		t.Fatalf("member: code=%d status=%q, want 201 active", code, m.Status)
	}
}

// Changes follow the same door: a trusted contributor edits their own
// event directly; an ordinary submitter's edit re-enters review; deleting
// your own event is always free.
func TestEditAndDeleteFollowTheSameDoor(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "owner", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	trusted, trustedToken := createTestUser(t, db, "trusty", "member")
	makeTrusted(t, db, trusted.ID)
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	unclaimedID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, unclaimedID)

	// Trusted contributor's event: direct edit, stays active.
	te, _ := createEventVia(t, db, cfg, trustedToken, eventBody(unclaimedID, "Zine Fair"))
	r := authedRequest("PATCH", "/api/v1/events/"+te.ID, map[string]string{"title": "Zine Fair II"}, trustedToken)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	if w.Code != 200 || eventStatusInDB(t, db, te.ID) != "active" {
		t.Fatalf("trusted edit: code=%d status=%q", w.Code, eventStatusInDB(t, db, te.ID))
	}

	// Ordinary submitter: approved event, then an edit pulls it back into review.
	se, _ := createEventVia(t, db, cfg, strangerToken, eventBody(unclaimedID, "Basement Show"))
	r = authedRequest("PATCH", "/api/v1/events/"+se.ID+"/review", map[string]string{"action": "approve"}, adminToken)
	serveMux(t, db, "PATCH", "/api/v1/events/{id}/review", handler.ReviewEventSubmission(db), r)

	r = authedRequest("PATCH", "/api/v1/events/"+se.ID, map[string]string{"title": "Basement Show (moved)"}, strangerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	if w.Code != 200 {
		t.Fatalf("submitter edit: code=%d body=%s", w.Code, w.Body.String())
	}
	if s := eventStatusInDB(t, db, se.ID); s != "pending_review" {
		t.Fatalf("submitter edit should re-enter review, status=%q", s)
	}

	// A third party may not edit someone else's event.
	r = authedRequest("PATCH", "/api/v1/events/"+te.ID, map[string]string{"title": "Hijacked"}, strangerToken)
	w = serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	if w.Code != 403 {
		t.Fatalf("third-party edit: code=%d, want 403", w.Code)
	}

	// Deleting your own event is always free.
	r = authedRequest("DELETE", "/api/v1/events/"+se.ID, nil, strangerToken)
	w = serveMux(t, db, "DELETE", "/api/v1/events/{id}", handler.DeleteEvent(db), r)
	if w.Code != 200 {
		t.Fatalf("delete own: code=%d", w.Code)
	}
}

// The claim transition adopts the calendar, but only once setup completes
// (docs/adr/039): activateClaimedNode flips the node to active, so pending
// submissions land in the new admins' queue and the community-submitted
// label (derived from node status) vanishes only after setup, not at
// approval or assignment.
func TestClaimMovesQueueToNewAdmins(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "founder", "member")
	claimant, claimantToken := createTestUser(t, db, "claimant", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, nodeID)

	e, _ := createEventVia(t, db, cfg, strangerToken, eventBody(nodeID, "Basement Show"))

	r0 := authedRequest("POST", "/api/v1/admin/nodes/spark-hall/assign", map[string]string{"user_id": claimant.ID}, adminToken)
	w0 := serveMux(t, db, "POST", "/api/v1/admin/nodes/{slug}/assign", handler.AdminAssignOwner(db), r0)
	if w0.Code != 200 {
		t.Fatalf("assign owner: code=%d body=%s", w0.Code, w0.Body.String())
	}

	// Assignment alone opens a setup window; it doesn't activate the patch
	// (docs/adr/039). The queue stays with the instance admin until the
	// assignee actually completes setup.
	r := authedRequest("GET", "/api/v1/admin/event-submissions", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/event-submissions", handler.ListAdminEventSubmissions(db), r)
	if !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("assignment alone moved the queue before setup was completed")
	}

	// The assignee completes setup like any claimant.
	r = authedRequest("GET", "/api/v1/nodes/spark-hall/claims/mine", nil, claimantToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/claims/mine", handler.MyClaim(db, claimCfg(false)), r)
	var mine struct {
		Claim map[string]interface{} `json:"claim"`
	}
	json.Unmarshal(w.Body.Bytes(), &mine)
	if mine.Claim == nil {
		t.Fatalf("assignee has no claim to set up: %s", w.Body.String())
	}
	setupClaimID := mine.Claim["id"].(string)

	r = authedRequest("POST", "/api/v1/claims/"+setupClaimID+"/setup", nil, claimantToken)
	w = serveMux(t, db, "POST", "/api/v1/claims/{id}/setup", handler.SetupClaim(db), r)
	if w.Code != 200 {
		t.Fatalf("setup: code=%d body=%s", w.Code, w.Body.String())
	}

	// Gone from the instance admin's queue.
	r = authedRequest("GET", "/api/v1/admin/event-submissions", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/event-submissions", handler.ListAdminEventSubmissions(db), r)
	if bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("claimed patch's submission still in instance admin queue")
	}

	// Present in the new patch admin's queue.
	r = authedRequest("GET", "/api/v1/nodes/spark-hall/event-submissions", nil, claimantToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-submissions", handler.ListNodeEventSubmissions(db), r)
	if w.Code != 200 || !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatalf("new admin queue: code=%d, missing event", w.Code)
	}
}

func bodyContains(body []byte, s string) bool {
	return strings.Contains(string(body), s)
}

// countNotifications waits for the handler's fire-and-forget notify goroutine
// before counting: it polls up to want, or — when want is zero and there is
// nothing to poll for — lets the goroutine settle first, so a zero is a real
// zero rather than a race.
func countNotifications(t *testing.T, db *database.DB, userID string, typ notifications.NotificationType, want int) int {
	t.Helper()
	if want == 0 {
		time.Sleep(250 * time.Millisecond)
	}
	deadline := time.Now().Add(2 * time.Second)
	var n int
	for {
		db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ?",
			userID, string(typ)).Scan(&n)
		if n >= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A review queue reports its own state, so the admin queue types reach their
// actor too — otherwise a lone site admin's own submission notified nobody at
// all. Every other type still skips the person who caused it.
func TestAdminQueueNotificationsReachTheirActor(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	cfg := submissionsCfg(true)
	soloAdmin, soloAdminToken := createTestUser(t, db, "soloadmin", "admin")
	otherAdmin, _ := createTestUser(t, db, "otheradmin", "admin")
	stranger, strangerToken := createTestUser(t, db, "passerby", "member")

	// A site admin submits a patch: the queue notification reaches them.
	r := authedRequest("POST", "/api/v1/submissions", map[string]string{"name": "Self Submitted"}, soloAdminToken)
	if w := serveMux(t, db, "POST", "/api/v1/submissions", handler.SubmitPatch(db, cfg), r); w.Code != 201 {
		t.Fatalf("admin self-submit: code=%d body=%s", w.Code, w.Body.String())
	}
	if n := countNotifications(t, db, soloAdmin.ID, notifications.AdminSubmission, 1); n != 1 {
		t.Errorf("submitting admin's own queue notification: got %d, want 1", n)
	}
	if n := countNotifications(t, db, otherAdmin.ID, notifications.AdminSubmission, 1); n != 1 {
		t.Errorf("other site admin: got %d, want 1", n)
	}

	// An event submitted to an unclaimed patch routes the same way.
	unclaimedID := createTestNode(t, db, soloAdmin.ID, "Spark Hall", "spark-hall-q", "open")
	makeUnclaimed(t, db, unclaimedID)
	if _, code := createEventVia(t, db, cfg, strangerToken, eventBody(unclaimedID, "Basement Show")); code != 201 {
		t.Fatalf("stranger event submit: code=%d", code)
	}
	if n := countNotifications(t, db, soloAdmin.ID, notifications.AdminEventSubmission, 1); n != 1 {
		t.Errorf("site admin event queue: got %d, want 1", n)
	}

	// A patch-level suggestion still skips its own author, and members hear
	// nothing about events they posted themselves.
	activeID := createTestNode(t, db, stranger.ID, "Gallery Row", "gallery-row-q", "open")
	createTestMembership(t, db, stranger.ID, activeID, "admin", "active")
	if _, code := createEventVia(t, db, cfg, strangerToken, eventBody(activeID, "Members Night")); code != 201 {
		t.Fatalf("member's own event: code=%d", code)
	}
	if n := countNotifications(t, db, stranger.ID, notifications.EventCreated, 0); n != 0 {
		t.Errorf("member notified about their own event: got %d, want 0", n)
	}
}

// patchEventVia sends a PATCH to UpdateEvent as the given session.
func patchEventVia(t *testing.T, db *database.DB, token, eventID string, body map[string]interface{}) (model.Event, int) {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/events/"+eventID, body, token)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	var e model.Event
	json.Unmarshal(w.Body.Bytes(), &e)
	return e, w.Code
}

// The edit door is the create door (docs/adr/026). On an active patch
// CreateEvent publishes for members and admins, so a member who posted an
// event must be able to edit it — UpdateEvent asked for admin and handed a
// member a 403 on their own event a minute after they created it. The reach
// stops there: one member does not rewrite another's event, and a suggestion
// adopted onto the patch keeps no residual rights for its outside submitter.
func TestUpdateEventFollowsTheCreateDoor(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	admin, adminToken := createTestUser(t, db, "voss-admin", "member")
	member, memberToken := createTestUser(t, db, "elena-voss", "member")
	other, otherToken := createTestUser(t, db, "other-member", "member")
	_, strangerToken := createTestUser(t, db, "passing-stranger", "member")

	nodeID := createTestNode(t, db, admin.ID, "Tinker's Damn", "tinkers-damn", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	// The other member needs a membership too, so the only thing separating
	// them from elena on her event is authorship.
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	// A member publishes directly...
	e, code := createEventVia(t, db, cfg, memberToken, eventBody(nodeID, "Basement Set"))
	if code != 201 || e.Status != "active" {
		t.Fatalf("member create: code=%d status=%q, want 201 active", code, e.Status)
	}

	// ...and may edit what they published.
	updated, code := patchEventVia(t, db, memberToken, e.ID, map[string]interface{}{"title": "Basement Set (early)"})
	if code != 200 {
		t.Fatalf("member editing own event: got %d, want 200", code)
	}
	if updated.Title != "Basement Set (early)" {
		t.Fatalf("member edit did not stick: title=%q", updated.Title)
	}
	if got := eventStatusInDB(t, db, e.ID); got != "active" {
		t.Fatalf("member edit changed status to %q, want it left active", got)
	}

	// A fellow member does not get to rewrite it.
	if _, code := patchEventVia(t, db, otherToken, e.ID, map[string]interface{}{"title": "Hijacked"}); code != 403 {
		t.Fatalf("other member editing someone else's event: got %d, want 403", code)
	}

	// A patch admin edits anything on their calendar.
	if _, code := patchEventVia(t, db, adminToken, e.ID, map[string]interface{}{"location": "Back room"}); code != 200 {
		t.Fatalf("patch admin editing a member's event: got %d, want 200", code)
	}

	// An outsider's suggestion is editable while it waits...
	sub, code := createEventVia(t, db, cfg, strangerToken, eventBody(nodeID, "Touring Show"))
	if code != 201 || sub.Status != "pending_review" {
		t.Fatalf("stranger create: code=%d status=%q, want 201 pending_review", code, sub.Status)
	}
	if _, code := patchEventVia(t, db, strangerToken, sub.ID, map[string]interface{}{"title": "Touring Show (moved)"}); code != 200 {
		t.Fatalf("submitter editing own pending submission: got %d, want 200", code)
	}

	// ...and belongs to the patch once adopted.
	if _, err := db.Exec("UPDATE events SET status = 'active' WHERE id = ?", sub.ID); err != nil {
		t.Fatalf("approve submission: %v", err)
	}
	if _, code := patchEventVia(t, db, strangerToken, sub.ID, map[string]interface{}{"title": "Retitled"}); code != 403 {
		t.Fatalf("submitter editing an adopted event: got %d, want 403", code)
	}
}
