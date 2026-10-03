package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-126. Seeing your own patch the way a stranger sees it, without signing
// out.
//
// A board member came to check what she would be forwarding to somebody
// outside her organization. The only way to find out was to sign out: "Let
// me look at my own page the way a stranger looks at it without signing
// out, because signing out is how I lost my account this afternoon."
//
// The test that matters is not that the preview looks plausible. It is that
// the preview and the real thing are the same answer: for every surface the
// patch page reads, an admin asking with ?as=visitor gets byte-for-byte
// what a signed-out caller gets. A preview computed from its own idea of
// the rules would be a preview of the wrong product, and would drift the
// first time a gate changed.

// previewFixture is a patch with something to withhold on every surface:
// a closed record, a withheld roster, a members-only charter, a hidden
// membership and a private noticeboard.
func previewFixture(t *testing.T, db *database.DB, slug string) (nodeID, adminToken string) {
	t.Helper()
	admin, adminToken := createTestUser(t, db, slug+"-admin", "member")
	nodeID = createTestNode(t, db, admin.ID, "Preview "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	hidden, _ := createTestUser(t, db, slug+"-hidden", "member")
	createTestMembership(t, db, hidden.ID, nodeID, "member", "active")
	if _, err := db.Exec(
		`UPDATE memberships SET visible = 0 WHERE user_id = ? AND node_id = ?`, hidden.ID, nodeID,
	); err != nil {
		t.Fatalf("hide a membership: %v", err)
	}
	createTestProposal(t, db, nodeID, admin.ID)
	return nodeID, adminToken
}

// sameAsAStranger serves one GET twice — once as the patch's admin asking
// to see it as a visitor, once with no session at all — and requires the
// two answers to match.
func sameAsAStranger(t *testing.T, db *database.DB, pattern, path string, h http.HandlerFunc, adminToken string) {
	t.Helper()
	sep := "?"
	if len(path) > 0 {
		for _, c := range path {
			if c == '?' {
				sep = "&"
				break
			}
		}
	}

	preview := authedRequest("GET", path+sep+"as=visitor", nil, adminToken)
	pw := serveMux(t, db, "GET", pattern, h, preview)

	anon := authedRequest("GET", path, nil, "")
	aw := servePublicMux(t, "GET", pattern, h, anon)

	if pw.Code != aw.Code {
		t.Errorf("%s: preview answered %d, a stranger %d", path, pw.Code, aw.Code)
		return
	}
	if pw.Body.String() != aw.Body.String() {
		t.Errorf("%s: the preview is not what a stranger sees\n preview:  %s\n stranger: %s",
			path, truncate(pw.Body.String()), truncate(aw.Body.String()))
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

func TestPreview_EverySurfaceMatchesAStranger(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken := previewFixture(t, db, "as-a-stranger")

	cases := []struct {
		name    string
		pattern string
		path    string
		h       http.HandlerFunc
	}{
		{"the patch itself", "/api/v1/nodes/{slug}", "/api/v1/nodes/as-a-stranger", handler.GetNode(db)},
		{"the member list", "/api/v1/nodes/{slug}/members", "/api/v1/nodes/as-a-stranger/members", handler.ListMembers(db)},
		{"the governance hub", "/api/v1/nodes/{slug}/governance/overview", "/api/v1/nodes/as-a-stranger/governance/overview", handler.GovernanceOverview(db)},
		{"the proposals list", "/api/v1/nodes/{slug}/proposals", "/api/v1/nodes/as-a-stranger/proposals", handler.ListProposals(db)},
		{"the documents list", "/api/v1/nodes/{slug}/governance", "/api/v1/nodes/as-a-stranger/governance", handler.ListGovernanceDocs(db)},
		{"the record", "/api/v1/nodes/{slug}/governance/record", "/api/v1/nodes/as-a-stranger/governance/record", handler.GovernanceRecord(db)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sameAsAStranger(t, db, c.pattern, c.path, c.h, adminToken)
		})
	}
}

// Without the parameter the admin still sees everything. A preview that
// leaked into ordinary reads would be the worst outcome of the lot.
func TestPreview_TheAdminStillSeesTheirOwnPatch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken := previewFixture(t, db, "still-mine")

	r := authedRequest("GET", "/api/v1/nodes/still-mine", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	body := decodeJSON(t, w)
	if body["is_admin"] != true {
		t.Errorf("the admin lost their own standing without asking to: %v", body)
	}
}

// And with it, the page knows to draw a visitor's chrome.
func TestPreview_TheChromeGoesWithIt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken := previewFixture(t, db, "no-chrome")

	r := authedRequest("GET", "/api/v1/nodes/no-chrome?as=visitor", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	body := decodeJSON(t, w)
	for _, k := range []string{"is_admin", "is_member", "membership_role"} {
		if v, present := body[k]; present && v != false && v != "" {
			t.Errorf("preview still reports %s = %v", k, v)
		}
	}
}

// The parameter may only ever subtract. It is ignored on anything that is
// not a GET, so nothing can act while wearing somebody else's standing.
func TestPreview_IsIgnoredOnAWrite(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, adminToken := previewFixture(t, db, "write-through")

	body := map[string]interface{}{"description": "changed while previewing"}
	r := authedRequest("PATCH", "/api/v1/nodes/write-through?as=visitor", body, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("the parameter changed what a write was allowed to do: %d", w.Code)
	}

	var desc string
	db.QueryRow(`SELECT description FROM nodes WHERE id = ?`, nodeID).Scan(&desc)
	if desc != "changed while previewing" {
		t.Errorf("the write did not land: %q", desc)
	}
}

// A signed-out caller passing it is unchanged by it.
func TestPreview_MeansNothingToAStranger(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	previewFixture(t, db, "already-out")

	plain := authedRequest("GET", "/api/v1/nodes/already-out", nil, "")
	pw := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), plain)
	withParam := authedRequest("GET", "/api/v1/nodes/already-out?as=visitor", nil, "")
	ww := servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), withParam)

	if pw.Body.String() != ww.Body.String() {
		t.Error("the parameter changed what a signed-out caller sees")
	}
}

var _ = httptest.NewRecorder
