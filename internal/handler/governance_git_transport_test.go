package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The git transport hands over a whole bare repository — every charter body,
// its revision history, its diffs — so it is gated on the whole-shelf rule and
// not on the per-document one (docs/adr/110). These tests run the transport the
// way main.go mounts it: governance.GitHTTPHandler over
// handler.GovernanceRepoNodeID, behind AuthOptional.
//
// The REST layer already filters a listing row by row, and governance_public_
// reach_test.go holds it to that. A repo has no such seam: there is no
// half-clone, so there is no visitor who may be handed half of one.

// govRepoPatch creates a patch with a governance repo on disk and both shelves
// stocked — one published doc, one members-only.
func govRepoPatch(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	dir := t.TempDir()
	if err := governance.InitInstanceRepo(dir); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}
	nodeID := createTestNode(t, db, ownerID, name, slug, "invite_only")
	if err := governance.ForkForNode(dir, nodeID, "casual"); err != nil {
		t.Fatalf("fork repo: %v", err)
	}
	insertGovDoc(t, db, nodeID, ownerID, "September Minutes", "public")
	insertGovDoc(t, db, nodeID, ownerID, "Draft Budget", "members")

	governance.SetDataDir(dir)
	t.Cleanup(func() { governance.SetDataDir("") })
	return nodeID
}

// cloneProbe asks for the ref advertisement — the first request a clone makes,
// and the one that decides whether there is anything to take.
func cloneProbe(t *testing.T, db *database.DB, slug, token string) *httptest.ResponseRecorder {
	t.Helper()
	git := governance.GitHTTPHandler(handler.GovernanceRepoNodeID(db))
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance.git/info/refs?service=git-upload-pack", nil, token)
	return serveOptionalAuthMux(t, db, "GET",
		"/api/v1/nodes/{slug}/governance.git/info/refs", git.ServeHTTP, r)
}

func servedRefs(w *httptest.ResponseRecorder) bool {
	return strings.Contains(w.Body.String(), "refs/heads/")
}

func TestGovernanceCloneIsRefusedToEveryoneOutsideTheRoom(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-adr110", "member")
	nodeID := govRepoPatch(t, db, harriet.ID, "Omar Coalition", "omar-coalition")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")

	_, outsiderToken := createTestUser(t, db, "outsider-adr110", "member")
	pending, pendingToken := createTestUser(t, db, "pending-adr110", "member")
	createTestMembership(t, db, pending.ID, nodeID, "member", "pending")

	// A follower is not here: whether one is inside this room is the patch's
	// own answer (docs/adr/050), and TestGovernanceCloneFollowsTheChartersGrant
	// holds the transport to it either way.

	for _, who := range []struct {
		name  string
		token string
	}{
		{"a signed-out visitor", ""},
		{"a signed-in outsider", outsiderToken},
		{"someone whose join request has not been answered", pendingToken},
	} {
		w := cloneProbe(t, db, "omar-coalition", who.token)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", who.name, w.Code)
		}
		if servedRefs(w) {
			t.Errorf("%s: was handed the repo's refs", who.name)
		}
	}
}

func TestGovernanceCloneServesTheRoom(t *testing.T) {
	db := setupTestDB(t)
	harriet, harrietToken := createTestUser(t, db, "harriet-adr110b", "member")
	nodeID := govRepoPatch(t, db, harriet.ID, "Coalition Two", "coalition-two")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")

	bea, beaToken := createTestUser(t, db, "bea-adr110b", "member")
	createTestMembership(t, db, bea.ID, nodeID, "member", "active")
	_, instanceAdminToken := createTestUser(t, db, "iris-adr110b", "admin")

	for _, who := range []struct {
		name  string
		token string
	}{
		{"a patch admin", harrietToken},
		{"a member", beaToken},
		{"an instance admin", instanceAdminToken},
	} {
		w := cloneProbe(t, db, "coalition-two", who.token)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d: %s", who.name, w.Code, w.Body.String())
			continue
		}
		if !servedRefs(w) {
			t.Errorf("%s: got 200 but no refs: %s", who.name, w.Body.String())
		}
	}
}

// A follower is handed the whole shelf exactly when the patch grants it
// (docs/adr/050), and the transport follows that grant rather than keeping a
// second opinion about followers.
func TestGovernanceCloneFollowsTheChartersGrant(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-adr110c", "member")
	nodeID := govRepoPatch(t, db, harriet.ID, "Coalition Three", "coalition-three")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")
	fran, franToken := createTestUser(t, db, "fran-adr110c", "member")
	createTestMembership(t, db, fran.ID, nodeID, "follower", "active")

	setCharters := func(shared string) {
		t.Helper()
		if _, err := db.Exec(
			"UPDATE nodes SET follower_permissions = ? WHERE id = ?",
			"{\"events\":true,\"proposals\":true,\"charters\":"+shared+",\"members\":true}", nodeID,
		); err != nil {
			t.Fatalf("set follower permissions: %v", err)
		}
	}

	setCharters("true")
	if w := cloneProbe(t, db, "coalition-three", franToken); w.Code != http.StatusOK {
		t.Errorf("a follower on a patch that shares its charters: expected 200, got %d", w.Code)
	}

	setCharters("false")
	if w := cloneProbe(t, db, "coalition-three", franToken); w.Code != http.StatusNotFound {
		t.Errorf("a follower on a patch that withholds its charters: expected 404, got %d", w.Code)
	}
}

// A private patch stays off every list while a direct link still opens its
// page. Its governance repo is not the exception — and the refusal must not
// tell an outsider that the slug resolved: one answer for "may not" and for
// "no such patch", or the route is an existence oracle.
func TestGovernanceCloneOfAPrivatePatchIsNotAnOracle(t *testing.T) {
	db := setupTestDB(t)
	harriet, _ := createTestUser(t, db, "harriet-adr110d", "member")
	nodeID := govRepoPatch(t, db, harriet.ID, "Quiet Collective", "quiet-collective")
	createTestMembership(t, db, harriet.ID, nodeID, "admin", "active")
	if _, err := db.Exec("UPDATE nodes SET visibility = 'private' WHERE id = ?", nodeID); err != nil {
		t.Fatalf("make private: %v", err)
	}

	real := cloneProbe(t, db, "quiet-collective", "")
	imagined := cloneProbe(t, db, "no-such-patch-at-all", "")

	if real.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a private patch's repo, got %d", real.Code)
	}
	if real.Code != imagined.Code || real.Body.String() != imagined.Body.String() {
		t.Errorf("a private patch answers differently from one that does not exist:\n  private:  %d %q\n  imagined: %d %q",
			real.Code, real.Body.String(), imagined.Code, imagined.Body.String())
	}
}
