package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// Who appears in a patch's public member list (docs/adr/095). The setting is
// a ladder — everyone, then admins, then nobody — and every test below asks
// the same two questions of a rung: which rows an outsider gets, and whether
// the patch's size is still stated. The second question is the one the ADR
// had to decide explicitly, because the quilt sizes a tile by member count:
// this control withholds identities, never the number.

// rosterFixture builds a patch with one admin, one member, one follower, and
// returns the node id plus session tokens for a member, a follower, an
// unrelated signed-in stranger, and an instance admin holding no role here.
type rosterFixture struct {
	nodeID        string
	memberToken   string
	followerToken string
	strangerToken string
	siteToken     string
}

func newRosterFixture(t *testing.T, db *database.DB, slug string) rosterFixture {
	t.Helper()
	owner, _ := createTestUser(t, db, slug+"-owner", "member")
	member, memberToken := createTestUser(t, db, slug+"-member", "member")
	follower, followerToken := createTestUser(t, db, slug+"-follower", "member")
	_, strangerToken := createTestUser(t, db, slug+"-stranger", "member")
	_, siteToken := createTestUser(t, db, slug+"-site", "admin")

	nodeID := createTestNode(t, db, owner.ID, "Roster "+slug, slug, "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	return rosterFixture{
		nodeID:        nodeID,
		memberToken:   memberToken,
		followerToken: followerToken,
		strangerToken: strangerToken,
		siteToken:     siteToken,
	}
}

func setPublicMemberList(t *testing.T, db *database.DB, nodeID, v string) {
	t.Helper()
	if _, err := db.Exec("UPDATE nodes SET public_member_list = ? WHERE id = ?", v, nodeID); err != nil {
		t.Fatalf("set public_member_list=%s: %v", v, err)
	}
}

// listRoster calls the members endpoint as the holder of token ("" for a
// signed-out visitor) and returns the usernames listed, the stated member
// count, and the setting the payload reports.
func listRoster(t *testing.T, db *database.DB, slug, token string) (names []string, memberCount int, setting string) {
	t.Helper()
	// AuthOptional, exactly as cmd/patchwork/main.go mounts this route: the
	// whole question here is what a signed-out visitor is handed, and
	// AuthRequired would answer 401 to the reader who matters most.
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/members", nil, token)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes/{slug}/members", middleware.AuthOptional(db, handler.ListMembers(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"items"`
		MemberCount      int    `json:"member_count"`
		PublicMemberList string `json:"public_member_list"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, it := range body.Items {
		names = append(names, it.Username)
	}
	return names, body.MemberCount, body.PublicMemberList
}

func rosterLists(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// A patch is born with its roster closed
// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md). This test
// asserted the opposite until then, on docs/adr/095's reasoning that
// 'everyone' was what every existing patch already did. It was, and admins
// consistently believed otherwise — so the default moved to meet the belief
// rather than the history.
func TestPublicMemberListDefaultsToNobody(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosterdefault")
	_ = f

	names, count, setting := listRoster(t, db, "rosterdefault", "")
	if setting != "nobody" {
		t.Errorf("a patch that has never touched the setting should read as nobody, got %q", setting)
	}
	if len(names) != 0 {
		t.Errorf("a closed roster names nobody to a visitor, got %v", names)
	}
	// The count is the one thing the control never withholds (docs/adr/095
	// decision 3): the quilt sizes a patch's tile by it, so a visitor who is
	// told no names is still told how many. Admin and member; the follower
	// has never counted (CONTEXT.md "Member count").
	if count != 2 {
		t.Errorf("expected member_count 2 even with the list closed, got %d", count)
	}
}

// And opening it is an act, which is the other half of the same rule.
func TestPublicMemberListOpensWhenThePatchSaysSo(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosteropened")
	openMemberList(t, db, f.nodeID)

	names, count, setting := listRoster(t, db, "rosteropened", "")
	if setting != "everyone" {
		t.Errorf("setting = %q, want everyone", setting)
	}
	if len(names) != 2 {
		t.Errorf("expected the admin and the member, got %v", names)
	}
	// The follower is absent for the reason it always was (docs/adr/006),
	// not because of anything this setting does.
	if rosterLists(names, "rosteropened-follower") {
		t.Error("follower rows are not public")
	}
	if count != 2 {
		t.Errorf("expected member_count 2, got %d", count)
	}
}

func TestPublicMemberListAdminsOnlyShowsAdmins(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosteradmins")
	setPublicMemberList(t, db, f.nodeID, "admins")

	names, count, setting := listRoster(t, db, "rosteradmins", "")
	if setting != "admins" {
		t.Errorf("expected setting admins, got %q", setting)
	}
	if len(names) != 1 || names[0] != "rosteradmins-owner" {
		t.Errorf("expected the admin alone, got %v", names)
	}
	// Decision 3: the size is still stated. A count gated alongside the
	// listing would say 1, and the patch's quilt tile would say 2.
	if count != 2 {
		t.Errorf("expected member_count to stay 2, got %d", count)
	}
}

func TestPublicMemberListNobodyWithholdsEveryRow(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosternobody")
	setPublicMemberList(t, db, f.nodeID, "nobody")

	names, count, setting := listRoster(t, db, "rosternobody", "")
	if setting != "nobody" {
		t.Errorf("expected setting nobody, got %q", setting)
	}
	if len(names) != 0 {
		t.Errorf("expected no rows, got %v", names)
	}
	if count != 2 {
		t.Errorf("expected member_count to stay 2, got %d", count)
	}
}

// The room always sees the room. This is what makes the setting a publishing
// choice rather than a wall inside the patch.
func TestPublicMemberListDoesNotGateTheRoom(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosterroom")
	setPublicMemberList(t, db, f.nodeID, "nobody")

	for _, c := range []struct {
		who   string
		token string
	}{
		{"a member of the patch", f.memberToken},
		{"an instance admin holding no role here", f.siteToken},
	} {
		names, _, _ := listRoster(t, db, "rosterroom", c.token)
		if !rosterLists(names, "rosterroom-owner") || !rosterLists(names, "rosterroom-member") {
			t.Errorf("%s should see the whole room, got %v", c.who, names)
		}
	}
}

// A follower is an outsider here, and so is any other signed-in stranger.
// This is the one place the setting lands somewhere a reader might not
// expect — docs/adr/095 consequences — so it is pinned rather than left to
// be rediscovered.
func TestPublicMemberListTreatsFollowersAsOutsiders(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosteroutside")
	setPublicMemberList(t, db, f.nodeID, "nobody")

	for _, c := range []struct {
		who   string
		token string
	}{
		{"a follower", f.followerToken},
		{"a signed-in stranger", f.strangerToken},
		{"a signed-out visitor", ""},
	} {
		names, _, setting := listRoster(t, db, "rosteroutside", c.token)
		if len(names) != 0 {
			t.Errorf("%s should get no rows, got %v", c.who, names)
		}
		// They are told why, or the page cannot tell an empty patch from a
		// withheld list.
		if setting != "nobody" {
			t.Errorf("%s should be told the list is withheld, got %q", c.who, setting)
		}
	}
}

// Decision 2: the gate is the member's switch AND the patch's setting. The
// patch can hide somebody who chose to be visible; nothing it sets can
// reveal somebody who chose to hide.
func TestPublicMemberListOnlySubtracts(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rostersubtract")

	// The member hides themselves under docs/adr/006.
	if _, err := db.Exec(
		"UPDATE memberships SET visible = 0 WHERE node_id = ? AND user_id = (SELECT id FROM users WHERE username = 'rostersubtract-member')",
		f.nodeID,
	); err != nil {
		t.Fatalf("hide membership: %v", err)
	}

	for _, setting := range []string{"everyone", "admins", "nobody"} {
		setPublicMemberList(t, db, f.nodeID, setting)
		names, count, _ := listRoster(t, db, "rostersubtract", "")
		if rosterLists(names, "rostersubtract-member") {
			t.Errorf("public_member_list=%s revealed a member who had hidden themselves: %v", setting, names)
		}
		// Subtracted from the list, counted in the total. Decision 3 holds for
		// the member's own switch as well as for this setting: a count names
		// nobody, and the profile head above this page has always stated the
		// ungated number. Counting under the switch here is what used to make
		// the two disagree.
		if count != 2 {
			t.Errorf("public_member_list=%s: expected member_count to stay 2 with a hidden member, got %d", setting, count)
		}
	}

	// And an admin who hid stays hidden at 'admins', which is the rung most
	// likely to be read as "show the admins" rather than "show fewer people".
	if _, err := db.Exec(
		"UPDATE memberships SET visible = 0 WHERE node_id = ? AND role = 'admin'", f.nodeID,
	); err != nil {
		t.Fatalf("hide admin: %v", err)
	}
	setPublicMemberList(t, db, f.nodeID, "admins")
	names, _, _ := listRoster(t, db, "rostersubtract", "")
	if len(names) != 0 {
		t.Errorf("expected a hidden admin to stay hidden at 'admins', got %v", names)
	}
}

// The number is one number. Whatever the setting, and whoever is hiding, the
// members page must state what the profile head states — the head is rendered
// directly above it, and the quilt sizes this patch's tile by the same figure.
// Asserted endpoint-against-endpoint rather than against a literal, because
// the failure being guarded is the two drifting apart.
func TestPublicMemberListCountMatchesTheProfileHead(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rostercount")

	// One more member, hidden under docs/adr/006, so the listing and the count
	// cannot coincide by accident at any rung.
	extra, _ := createTestUser(t, db, "rostercount-hidden", "member")
	hiddenID := createTestMembership(t, db, extra.ID, f.nodeID, "member", "active")
	if _, err := db.Exec("UPDATE memberships SET visible = 0 WHERE id = ?", hiddenID); err != nil {
		t.Fatalf("hide membership: %v", err)
	}

	headCount := func() int {
		t.Helper()
		r := authedRequest("GET", "/api/v1/nodes/rostercount", nil, "")
		w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 from the detail endpoint, got %d: %s", w.Code, w.Body.String())
		}
		var detail struct {
			Node struct {
				MemberCount int `json:"member_count"`
			} `json:"node"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return detail.Node.MemberCount
	}

	for _, setting := range []string{"everyone", "admins", "nobody"} {
		setPublicMemberList(t, db, f.nodeID, setting)
		names, count, _ := listRoster(t, db, "rostercount", "")
		if head := headCount(); count != head {
			t.Errorf("public_member_list=%s: members page says %d, profile head says %d", setting, count, head)
		}
		if count != 3 {
			t.Errorf("public_member_list=%s: expected 3 (admin, member, hidden member), got %d", setting, count)
		}
		if len(names) >= count {
			t.Errorf("public_member_list=%s: expected fewer listed rows than counted people, got %v", setting, names)
		}
	}
}

func TestPublicMemberListWriteIsValidatedAndReadBack(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "rosterwrite-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Roster Write", "rosterwrite", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	patch := func(v interface{}) int {
		r := authedRequest("PATCH", "/api/v1/nodes/rosterwrite",
			map[string]interface{}{"public_member_list": v}, adminToken)
		return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code
	}

	// A bad value is a 400 from the handler, never the CHECK constraint's 500.
	for _, bad := range []interface{}{"members", "public", "", 3} {
		if code := patch(bad); code != http.StatusBadRequest {
			t.Errorf("public_member_list=%v: expected 400, got %d", bad, code)
		}
	}

	if code := patch("nobody"); code != http.StatusOK {
		t.Fatalf("expected 200 setting nobody, got %d", code)
	}

	// It reads back on the detail endpoint, which is where the settings form
	// and the workspace tab row both get it.
	r := authedRequest("GET", "/api/v1/nodes/rosterwrite", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Node struct {
			PublicMemberList string `json:"public_member_list"`
		} `json:"node"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Node.PublicMemberList != "nobody" {
		t.Errorf("expected the detail payload to carry nobody, got %q", detail.Node.PublicMemberList)
	}
}

// A non-admin cannot set it. The setting is a safety lever an admin pulls
// immediately (decision 6), which is only sound while the pullers are admins.
func TestPublicMemberListIsAdminOnly(t *testing.T) {
	db := setupTestDB(t)
	f := newRosterFixture(t, db, "rosterauthz")
	// Opened first so the refusal below has something to fail to change: a
	// PATCH to 'nobody' on a patch already closed proves nothing.
	openMemberList(t, db, f.nodeID)

	r := authedRequest("PATCH", "/api/v1/nodes/rosterauthz",
		map[string]interface{}{"public_member_list": "nobody"}, f.memberToken)
	if code := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code; code != http.StatusForbidden {
		t.Errorf("expected 403 for a member, got %d", code)
	}

	var got string
	db.QueryRow("SELECT public_member_list FROM nodes WHERE id = ?", f.nodeID).Scan(&got)
	if got != "everyone" { // the fixture opened it; a refused PATCH left it alone
		t.Errorf("expected the setting untouched, got %q", got)
	}
}
