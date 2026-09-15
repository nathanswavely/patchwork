package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A document published to everyone is reachable by everyone (docs/adr/036),
// and no other gate may stand in front of it.
//
// The shape here is F-052's, twice found by the governance simulation: a
// coalition on the Minimal template — which ships
// `follower_permissions.charters: false` — deliberately published its
// September minutes, and a signed-out visitor could not find them. The flag
// that hid them gates one thing and one thing only: whether a *follower* is
// handed this patch's members-only charters. It is a grant on top of the
// public shelf, never a lid over it.

// minimalPatch builds a patch configured the way the Minimal template
// configures one: nothing offered to followers.
func minimalPatch(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "invite_only")
	if _, err := db.Exec(
		`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
		`{"events":true,"proposals":false,"charters":false,"members":false}`, nodeID,
	); err != nil {
		t.Fatalf("set follower permissions: %v", err)
	}
	return nodeID
}

func insertGovDoc(t *testing.T, db *database.DB, nodeID, authorID, title, visibility string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, visibility, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		id, nodeID, title, "What the meeting decided.", visibility, authorID,
	); err != nil {
		t.Fatalf("insert doc %s: %v", title, err)
	}
	return id
}

// listGovDocs runs the list endpoint as whoever the token names (an empty
// token is signed out) and returns the titles handed over plus the
// published-only signal.
func listGovDocs(t *testing.T, db *database.DB, slug, token string) ([]string, bool) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance", nil, token)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes/{slug}/governance", handler.ListGovernanceDocs(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list docs as %q: expected 200, got %d: %s", token, w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %v", result["items"])
	}
	var titles []string
	for _, it := range items {
		titles = append(titles, it.(map[string]interface{})["title"].(string))
	}
	publishedOnly, ok := result["published_only"].(bool)
	if !ok {
		t.Fatalf("expected published_only bool, got %v", result["published_only"])
	}
	return titles, publishedOnly
}

func hasDocTitle(titles []string, want string) bool {
	for _, got := range titles {
		if got == want {
			return true
		}
	}
	return false
}

func TestPublishedDocReachesASignedOutVisitorOnAMinimalPatch(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-f052", "member")
	nodeID := minimalPatch(t, db, harriet.ID, "Omar Coalition", "omar-coalition")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")
	insertGovDoc(t, db, nodeID, harriet.ID, "September Minutes", "public")
	insertGovDoc(t, db, nodeID, harriet.ID, "Draft Budget", "members")

	titles, publishedOnly := listGovDocs(t, db, "omar-coalition", "")

	if !hasDocTitle(titles, "September Minutes") {
		t.Errorf("a signed-out visitor could not reach the published minutes; got %v", titles)
	}
	// Invisible and uncounted: the withheld doc contributes no row and no
	// number a visitor could subtract to learn it exists (docs/adr/036).
	if hasDocTitle(titles, "Draft Budget") {
		t.Errorf("a members-only doc reached a signed-out visitor; got %v", titles)
	}
	if len(titles) != 1 {
		t.Errorf("expected exactly the published doc, got %v", titles)
	}
	if !publishedOnly {
		t.Error("published_only should be true for a signed-out visitor")
	}
}

func TestMemberSeesBothShelvesOnAMinimalPatch(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-f052b", "member")
	bea, beaToken := createTestUser(t, db, "bea-f052b", "member")
	nodeID := minimalPatch(t, db, harriet.ID, "Coalition Two", "coalition-two")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")
	createTestMembership(t, db, bea.ID, nodeID, "member", "active")
	insertGovDoc(t, db, nodeID, harriet.ID, "September Minutes", "public")
	insertGovDoc(t, db, nodeID, harriet.ID, "Draft Budget", "members")

	titles, publishedOnly := listGovDocs(t, db, "coalition-two", beaToken)

	if !hasDocTitle(titles, "September Minutes") || !hasDocTitle(titles, "Draft Budget") {
		t.Errorf("a member should see both shelves; got %v", titles)
	}
	if publishedOnly {
		t.Error("published_only should be false for a member: they are shown everything")
	}
}

// The flag still means what it always meant, and only that: it is the
// follower's key to the members-only shelf (docs/adr/050).
func TestCharterFlagStillGatesTheMembersOnlyShelfForFollowers(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner-f052c", "member")
	follower, followerToken := createTestUser(t, db, "follower-f052c", "member")

	shut := minimalPatch(t, db, owner.ID, "Shut Shelf", "shut-shelf")
	insertGovDoc(t, db, shut, owner.ID, "September Minutes", "public")
	insertGovDoc(t, db, shut, owner.ID, "Draft Budget", "members")
	createTestMembership(t, db, follower.ID, shut, "follower", "active")

	open := createTestNode(t, db, owner.ID, "Open Shelf", "open-shelf", "open")
	if _, err := db.Exec(`UPDATE nodes SET follower_permissions = ? WHERE id = ?`,
		`{"events":true,"proposals":true,"charters":true,"members":true}`, open); err != nil {
		t.Fatalf("set follower permissions: %v", err)
	}
	insertGovDoc(t, db, open, owner.ID, "September Minutes", "public")
	insertGovDoc(t, db, open, owner.ID, "Draft Budget", "members")
	createTestMembership(t, db, follower.ID, open, "follower", "active")

	shutTitles, shutPublishedOnly := listGovDocs(t, db, "shut-shelf", followerToken)
	// The half that must not regress: a follower is never shown less than a
	// passer-by, so the published doc reaches them either way.
	if !hasDocTitle(shutTitles, "September Minutes") {
		t.Errorf("charters:false hid a published doc from a follower; got %v", shutTitles)
	}
	if hasDocTitle(shutTitles, "Draft Budget") {
		t.Errorf("charters:false should still withhold the members-only shelf; got %v", shutTitles)
	}
	if !shutPublishedOnly {
		t.Error("published_only should be true for a follower without the charters grant")
	}

	openTitles, openPublishedOnly := listGovDocs(t, db, "open-shelf", followerToken)
	if !hasDocTitle(openTitles, "Draft Budget") {
		t.Errorf("charters:true should hand a follower the members-only shelf; got %v", openTitles)
	}
	if openPublishedOnly {
		t.Error("published_only should be false for a follower holding the charters grant")
	}
}

// Which kind of empty an empty list is — and the signal that says so must be
// about the viewer, never about the documents, or it becomes the disclosure
// per-document visibility exists to prevent.
func TestEmptyListSaysWhichKindOfEmptyWithoutDisclosingWhatIsHidden(t *testing.T) {
	db := setupTestDB(t)
	owner, ownerToken := createTestUser(t, db, "owner-f052d", "member")

	bare := minimalPatch(t, db, owner.ID, "Nothing Here", "nothing-here")
	createTestMembership(t, db, owner.ID, bare, "admin", "active")

	hiding := minimalPatch(t, db, owner.ID, "Members Only", "members-only-patch")
	createTestMembership(t, db, owner.ID, hiding, "admin", "active")
	insertGovDoc(t, db, hiding, owner.ID, "Draft Budget", "members")
	insertGovDoc(t, db, hiding, owner.ID, "House Rules", "members")

	bareTitles, barePublishedOnly := listGovDocs(t, db, "nothing-here", "")
	hidingTitles, hidingPublishedOnly := listGovDocs(t, db, "members-only-patch", "")

	// Both empty to a visitor, and the signal is the same either way: the
	// patch with two withheld documents is indistinguishable from the patch
	// with none. That is the no-leak property, and it is what makes the
	// signal safe to print.
	if len(bareTitles) != 0 || len(hidingTitles) != 0 {
		t.Fatalf("expected both listings empty to a visitor; got %v and %v", bareTitles, hidingTitles)
	}
	if !barePublishedOnly || !hidingPublishedOnly {
		t.Error("a signed-out visitor is always reading the published-only shelf")
	}
	if barePublishedOnly != hidingPublishedOnly {
		t.Error("published_only varied with what was hidden — that discloses the hidden rows")
	}

	// The same emptiness, read by someone shown everything, is the other
	// sentence: nothing has been recorded at all.
	adminTitles, adminPublishedOnly := listGovDocs(t, db, "nothing-here", ownerToken)
	if len(adminTitles) != 0 {
		t.Fatalf("expected an empty listing, got %v", adminTitles)
	}
	if adminPublishedOnly {
		t.Error("published_only should be false for an admin: their empty is the real empty")
	}
}

// An instance admin holds no membership here and still reads everything —
// unchanged, and asserted so the narrowing above cannot quietly take it away.
func TestInstanceAdminStillReadsEveryShelf(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "owner-f052e", "member")
	_, adminToken := createTestUser(t, db, "instance-admin-f052e", "admin")
	nodeID := minimalPatch(t, db, owner.ID, "Coalition Three", "coalition-three")
	insertGovDoc(t, db, nodeID, owner.ID, "September Minutes", "public")
	insertGovDoc(t, db, nodeID, owner.ID, "Draft Budget", "members")

	titles, publishedOnly := listGovDocs(t, db, "coalition-three", adminToken)
	if len(titles) != 2 {
		t.Errorf("an instance admin should read both shelves; got %v", titles)
	}
	if publishedOnly {
		t.Error("published_only should be false for an instance admin")
	}
}

// The document itself, not just the listing: a signed-out visitor opening the
// published minutes gets them, and the withheld one 404s rather than 403s.
func TestSignedOutVisitorOpensThePublishedDocAndNotTheOther(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-f052f", "member")
	nodeID := minimalPatch(t, db, harriet.ID, "Coalition Four", "coalition-four")
	publicID := insertGovDoc(t, db, nodeID, harriet.ID, "September Minutes", "public")
	hiddenID := insertGovDoc(t, db, nodeID, harriet.ID, "Draft Budget", "members")

	r := httptest.NewRequest("GET", "/api/v1/governance/"+publicID, nil)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected the published doc to open for a visitor, got %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest("GET", "/api/v1/governance/"+hiddenID, nil)
	w = serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a members-only doc, got %d: %s", w.Code, w.Body.String())
	}
}
