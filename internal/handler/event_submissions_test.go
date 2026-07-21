package handler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
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

	unclaimedID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
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
	token, err := auth.CreateSession(db, userID, "127.0.0.1")
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

	unclaimedID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
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

	unclaimedID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
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

	unclaimedID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
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

// The claim transition adopts the calendar: transferOwnership flips the
// node to active, so pending submissions land in the new admins' queue
// and the community-submitted label (derived from node status) vanishes.
func TestClaimMovesQueueToNewAdmins(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	owner, _ := createTestUser(t, db, "founder", "member")
	claimant, claimantToken := createTestUser(t, db, "claimant", "member")
	_, strangerToken := createTestUser(t, db, "stranger", "member")
	_, adminToken := createTestUser(t, db, "siteadmin", "admin")

	nodeID := createTestNode(t, db, owner.ID, "West Art", "west-art", "open")
	makeUnclaimed(t, db, nodeID)

	e, _ := createEventVia(t, db, cfg, strangerToken, eventBody(nodeID, "Basement Show"))

	r0 := authedRequest("POST", "/api/v1/admin/nodes/west-art/assign", map[string]string{"user_id": claimant.ID}, adminToken)
	w0 := serveMux(t, db, "POST", "/api/v1/admin/nodes/{slug}/assign", handler.AdminAssignOwner(db), r0)
	if w0.Code != 200 {
		t.Fatalf("assign owner: code=%d body=%s", w0.Code, w0.Body.String())
	}

	// Gone from the instance admin's queue.
	r := authedRequest("GET", "/api/v1/admin/event-submissions", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/event-submissions", handler.ListAdminEventSubmissions(db), r)
	if bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatal("claimed patch's submission still in instance admin queue")
	}

	// Present in the new patch admin's queue.
	r = authedRequest("GET", "/api/v1/nodes/west-art/event-submissions", nil, claimantToken)
	w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}/event-submissions", handler.ListNodeEventSubmissions(db), r)
	if w.Code != 200 || !bodyContains(w.Body.Bytes(), e.ID) {
		t.Fatalf("new admin queue: code=%d, missing event", w.Code)
	}
}

func bodyContains(body []byte, s string) bool {
	return strings.Contains(string(body), s)
}
