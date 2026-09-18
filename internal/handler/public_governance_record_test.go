package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// serveOptional mounts a handler the way main.go mounts every read gated
// here: AuthOptional, so a signed-out caller is a viewer with no session
// rather than a 401. serveMux would answer 401 before the gate ran, which
// tests the middleware instead of the rule.
func serveOptional(t *testing.T, db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthOptional(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// The default should match the assumption
// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
//
// Patch admins believed their member list was not public. It was, and their
// proposals were world-readable with no control over them at all. These tests
// pin the two halves of the correction: a patch is born closed on both
// counts, and opening either one is an act.

type recordFixture struct {
	nodeID        string
	slug          string
	proposalID    string
	adminToken    string
	memberToken   string
	followerToken string
	siteToken     string
}

func newRecordFixture(t *testing.T, db *database.DB, slug string) recordFixture {
	t.Helper()
	admin, adminToken := createTestUser(t, db, slug+"_admin", "member")
	member, memberToken := createTestUser(t, db, slug+"_member", "member")
	follower, followerToken := createTestUser(t, db, slug+"_follower", "member")
	_, siteToken := createTestUser(t, db, slug+"_site", "admin")

	nodeID := createTestNode(t, db, admin.ID, "Record "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	proposalID := createTestProposal(t, db, nodeID, admin.ID)

	return recordFixture{
		nodeID: nodeID, slug: slug, proposalID: proposalID,
		adminToken: adminToken, memberToken: memberToken,
		followerToken: followerToken, siteToken: siteToken,
	}
}

// listRecordProposals asks for a patch's proposals as the holder of token
// ("" for signed out) and hands back the item count and the setting the
// payload states.
func listRecordProposals(t *testing.T, db *database.DB, slug, token string) (int, string) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/proposals", nil, token)
	w := serveOptional(t, db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("proposals list returned %d, want 200: %s", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, _ := body["items"].([]interface{})
	setting, _ := body["public_governance_record"].(string)
	return len(items), setting
}

func TestGovernanceRecord_APatchIsBornClosed(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "bornclosed", "member")
	_ = owner

	r := authedRequest("POST", "/api/v1/nodes", map[string]interface{}{
		"name": "Born Closed", "membership_policy": "open",
	}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create node returned %d, want 201: %s", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	slug, _ := created["slug"].(string)

	var list, record string
	db.QueryRow(
		"SELECT public_member_list, public_governance_record FROM nodes WHERE slug = ?", slug,
	).Scan(&list, &record)

	// Both, because the pair is the point: an admin meets one closed door
	// rather than finding out later that only half of it was shut.
	if list != "nobody" {
		t.Errorf("public_member_list = %q, want nobody — a new patch is not enumerable", list)
	}
	if record != "nobody" {
		t.Errorf("public_governance_record = %q, want nobody", record)
	}
}

func TestGovernanceRecord_CreationCanOpenItDeliberately(t *testing.T) {
	db := setupTestDB(t)
	_, ownerToken := createTestUser(t, db, "bornopen", "member")

	r := authedRequest("POST", "/api/v1/nodes", map[string]interface{}{
		"name": "Born Open", "membership_policy": "open",
		"public_member_list": "everyone", "public_governance_record": "everyone",
	}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create node returned %d, want 201: %s", w.Code, w.Body.String())
	}
	slug, _ := decodeJSON(t, w)["slug"].(string)

	var list, record string
	db.QueryRow(
		"SELECT public_member_list, public_governance_record FROM nodes WHERE slug = ?", slug,
	).Scan(&list, &record)
	if list != "everyone" || record != "everyone" {
		t.Errorf("create took %q/%q, want everyone/everyone — a patch that governs in the open says so at the door", list, record)
	}
}

func TestGovernanceRecord_CreationRejectsANonsenseValue(t *testing.T) {
	db := setupTestDB(t)
	_, ownerToken := createTestUser(t, db, "bornbogus", "member")

	// 'admins' is its sibling's middle rung and deliberately not a value here:
	// a nomination names its subject in its own title, so there is nothing
	// coherent between open and closed for a record.
	r := authedRequest("POST", "/api/v1/nodes", map[string]interface{}{
		"name": "Born Bogus", "public_governance_record": "admins",
	}, ownerToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes", handler.CreateNode(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("create with public_governance_record=admins returned %d, want 400", w.Code)
	}
}

// Who is inside the room, and who reads the patch's public answer. A follower
// is an outsider here for the same reason they are for the member list
// (docs/adr/095): following needs nobody's approval, so counting one as an
// insider would let any signed-in stranger admit themselves by clicking
// Follow.
func TestGovernanceRecord_ClosedIsTheRoomsAlone(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "recclosed")

	for _, c := range []struct {
		who   string
		token string
		want  int
	}{
		{"signed out", "", 0},
		{"a follower", f.followerToken, 0},
		{"a member", f.memberToken, 1},
		{"an admin", f.adminToken, 1},
		// An instance admin sees every room already, everywhere.
		{"an instance admin", f.siteToken, 1},
	} {
		got, setting := listRecordProposals(t, db, f.slug, c.token)
		if got != c.want {
			t.Errorf("%s saw %d proposals, want %d", c.who, got, c.want)
		}
		if c.want == 0 && setting != "nobody" {
			// The whole reason this is a 200 and not a 404: an empty array
			// alone cannot tell "nothing decided yet" from "withheld", and
			// those two want opposite copy.
			t.Errorf("%s got an empty list without being told why (setting %q)", c.who, setting)
		}
	}
}

func TestGovernanceRecord_OpeningItLetsAStrangerRead(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "recopened")
	openGovernanceRecord(t, db, f.nodeID)

	got, setting := listRecordProposals(t, db, f.slug, "")
	if got != 1 {
		t.Errorf("signed out saw %d proposals on an open record, want 1", got)
	}
	if setting != "everyone" {
		t.Errorf("setting = %q, want everyone — one field answers the question either way", setting)
	}
}

// A single proposal by id is a 404 rather than the list's 200: without the
// listing there is no legitimate way to be holding the id, and it matches how
// a members-only charter answers.
func TestGovernanceRecord_AProposalByIdIs404ToOutsiders(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "recbyid")

	for _, c := range []struct {
		who   string
		token string
		want  int
	}{
		{"signed out", "", http.StatusNotFound},
		{"a follower", f.followerToken, http.StatusNotFound},
		{"a member", f.memberToken, http.StatusOK},
		{"an admin", f.adminToken, http.StatusOK},
	} {
		r := authedRequest("GET", "/api/v1/proposals/"+f.proposalID, nil, c.token)
		w := serveOptional(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), r)
		if w.Code != c.want {
			t.Errorf("%s reading the proposal got %d, want %d", c.who, w.Code, c.want)
		}
	}
}

// The thread under a proposal follows the proposal. This route was mounted
// bare — no auth wrapper at all — which is how a discussion nobody meant to
// publish became world-readable with every commenter named.
func TestGovernanceRecord_CommentsFollowTheProposal(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "reccomments")

	r := authedRequest("GET", "/api/v1/proposals/"+f.proposalID+"/comments", nil, "")
	w := serveOptional(t, db, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("anonymous comment read on a closed record got %d, want 404", w.Code)
	}

	r = authedRequest("GET", "/api/v1/proposals/"+f.proposalID+"/comments", nil, f.memberToken)
	w = serveOptional(t, db, "GET", "/api/v1/proposals/{id}/comments", handler.ListComments(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("member comment read got %d, want 200", w.Code)
	}
}

// Setting it is the patch's, not any member's — the same authorisation the
// member list takes.
func TestGovernanceRecord_OnlyAdminsMoveIt(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "recauthz")

	r := authedRequest("PATCH", "/api/v1/nodes/"+f.slug,
		map[string]interface{}{"public_governance_record": "everyone"}, f.memberToken)
	if code := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code; code != http.StatusForbidden {
		t.Errorf("member PATCH returned %d, want 403", code)
	}

	var got string
	db.QueryRow("SELECT public_governance_record FROM nodes WHERE id = ?", f.nodeID).Scan(&got)
	if got != "nobody" {
		t.Errorf("a refused PATCH still moved the setting to %q", got)
	}

	r = authedRequest("PATCH", "/api/v1/nodes/"+f.slug,
		map[string]interface{}{"public_governance_record": "everyone"}, f.adminToken)
	if code := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code; code != http.StatusOK {
		t.Errorf("admin PATCH returned %d, want 200", code)
	}
}

// A membership must not travel as a fact, or as an inference. docs/adr/095
// decision 7 said "nothing federates" because memberships never ride an actor
// document — true of the actor, false in effect: you cannot vote on a patch
// unless you are a member of it, so a gv:Vote naming an actor and a node is a
// membership assertion in all but name.
//
// Without this the setting is a REST-only fiction: the record closed to a
// browser and delivered in full to every remote follower, in copies that
// never come back.
func TestGovernanceRecord_ClosedRecordDoesNotFederate(t *testing.T) {
	db := setupTestDB(t)

	ap.SetDomain("closed.test.example.com")
	t.Cleanup(func() { ap.SetDomain("") })

	admin, adminToken := createTestUser(t, db, "fed_closed_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Closed Fed", "closed-fed", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	// Deliberately no openGovernanceRecord: this patch is what the product
	// now creates.

	followerID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted) VALUES (?, 'node', ?, 'https://remote.example.com/ap/users/remote2', 'https://remote.example.com/inbox', 1)`,
		followerID, nodeID,
	); err != nil {
		t.Fatalf("insert ap_follower: %v", err)
	}

	r := authedRequest("POST", "/api/v1/nodes/closed-fed/proposals", map[string]interface{}{
		"title": "Not For The Wire", "body": "Ours.", "proposal_type": "action", "duration_hours": 48,
	}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create proposal returned %d: %s", w.Code, w.Body.String())
	}

	// The broadcast is a goroutine, same as the positive case asserts.
	time.Sleep(150 * time.Millisecond)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&count)
	if count != 0 {
		var activityJSON string
		db.QueryRow("SELECT activity_json FROM ap_outbox_queue LIMIT 1").Scan(&activityJSON)
		t.Errorf("a closed patch queued %d activities; first: %s", count, activityJSON)
	}
}

// The governance overview names the patch's council, which is a public member
// list by another route. It ran no gate at all until this ADR: not the
// patch's own public_member_list (docs/adr/095), not the member's visible
// switch (docs/adr/006). A patch that had taken its roster down published its
// admins here anyway.
//
// Found by opening the page on a closed patch in the browser, which is the
// only place it shows — the frontend suite asserts source text and could not
// have seen it.
func TestGovernanceOverview_CouncilFollowsTheRoster(t *testing.T) {
	db := setupTestDB(t)
	f := newRecordFixture(t, db, "ovcouncil")

	overviewAdmins := func(token string) int {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/"+f.slug+"/governance/overview", nil, token)
		w := serveOptional(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("overview returned %d: %s", w.Code, w.Body.String())
		}
		admins, _ := decodeJSON(t, w)["admins"].([]interface{})
		return len(admins)
	}

	// The fixture's patch is closed, which is what the product now creates.
	if n := overviewAdmins(""); n != 0 {
		t.Errorf("a closed patch named %d admins to a stranger, want 0", n)
	}
	if n := overviewAdmins(f.followerToken); n != 0 {
		t.Errorf("a closed patch named %d admins to a follower, want 0", n)
	}
	// The room always sees its own council.
	if n := overviewAdmins(f.memberToken); n != 1 {
		t.Errorf("a member saw %d admins, want 1", n)
	}

	// 'admins' and 'everyone' agree here, because admins are what it lists.
	for _, setting := range []string{"admins", "everyone"} {
		if _, err := db.Exec(
			`UPDATE nodes SET public_member_list = ? WHERE id = ?`, setting, f.nodeID,
		); err != nil {
			t.Fatalf("set roster: %v", err)
		}
		if n := overviewAdmins(""); n != 1 {
			t.Errorf("at %q a stranger saw %d admins, want 1", setting, n)
		}
	}

	// And the member's own switch subtracts on top, which is the one
	// direction no patch setting may overrule (docs/adr/006).
	if _, err := db.Exec(
		`UPDATE memberships SET visible = 0 WHERE node_id = ? AND role = 'admin'`, f.nodeID,
	); err != nil {
		t.Fatalf("hide admin: %v", err)
	}
	if n := overviewAdmins(""); n != 0 {
		t.Errorf("an admin who hid was named to a stranger anyway (%d shown)", n)
	}
	if n := overviewAdmins(f.memberToken); n != 1 {
		t.Errorf("the room stopped seeing a hidden admin (%d shown), want 1", n)
	}
}
