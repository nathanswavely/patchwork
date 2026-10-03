package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// F-124. A patch nobody can open, and the person responsible for it was the
// last to know.
//
// A printmaker created a duplicate of her press by accident and archived it.
// A year later she found it in her personal export, still carrying "Council
// election, state: voting, seats: 1, ends: 2025-09-08" — and every door she
// tried answered "Patch not found". She was its admin.
//
// The refusal stays absolute (docs/adr/034): every slug route still declines
// an archived patch, to her as much as to anybody. What these hold is that
// the product stops pretending it was never there, for the one person who
// cannot be protected by the pretence.

func archivedPatch(t *testing.T, db *database.DB, slug string) (nodeID, adminToken, strangerToken string) {
	t.Helper()
	admin, adminToken := createTestUser(t, db, slug+"-admin", "member")
	stranger, strangerToken := createTestUser(t, db, slug+"-stranger", "member")
	_ = stranger
	nodeID = createTestNode(t, db, admin.ID, "Ghost "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	if _, err := db.Exec(
		`UPDATE nodes SET archived_from = status, status = 'archived' WHERE id = ?`, nodeID,
	); err != nil {
		t.Fatalf("archive: %v", err)
	}
	return nodeID, adminToken, strangerToken
}

func getNodeAs(t *testing.T, db *database.DB, slug, token string) (*int, map[string]interface{}) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug, nil, token)
	var w = serveMux(t, db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	if token == "" {
		r = authedRequest("GET", "/api/v1/nodes/"+slug, nil, "")
		w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	}
	code := w.Code
	return &code, decodeJSON(t, w)
}

func TestArchived_ItsOwnAdminIsToldWhatHappened(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken, _ := archivedPatch(t, db, "told-admin")

	code, body := getNodeAs(t, db, "told-admin", adminToken)
	// Still refused: the patch is not servable and docs/adr/034 is not
	// being relaxed. Only the explanation is new.
	if *code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", *code)
	}
	if body["archived"] != true {
		t.Errorf("the patch's own admin was not told it is archived: %v", body)
	}
	if body["name"] != "Ghost told-admin" {
		t.Errorf("name = %v, want the patch's name", body["name"])
	}
}

// A stranger learns exactly what they learned before.
func TestArchived_AStrangerLearnsNothing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, _, strangerToken := archivedPatch(t, db, "told-nobody")

	for _, tok := range []string{"", strangerToken} {
		code, body := getNodeAs(t, db, "told-nobody", tok)
		if *code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", *code)
		}
		if _, present := body["archived"]; present {
			t.Errorf("a caller with no role learned the patch exists: %v", body)
		}
		if _, present := body["name"]; present {
			t.Errorf("a caller with no role learned the patch's name: %v", body)
		}
	}
}

// A slug that never existed is still a slug that never existed.
func TestArchived_AMissingPatchIsStillMissing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken, _ := archivedPatch(t, db, "real-one")

	code, body := getNodeAs(t, db, "never-existed", adminToken)
	if *code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", *code)
	}
	if _, present := body["archived"]; present {
		t.Errorf("a slug with no row behind it reported as archived: %v", body)
	}
}

// And the list she could have found it in.
func TestArchived_ShowsUpInThePatchesSheRuns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, adminToken, strangerToken := archivedPatch(t, db, "in-my-list")

	items := func(token, query string) []interface{} {
		r := authedRequest("GET", "/api/v1/me/nodes"+query, nil, token)
		w := serveMux(t, db, "GET", "/api/v1/me/nodes", handler.ListMyMemberships(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("me/nodes%s: got %d: %s", query, w.Code, w.Body.String())
		}
		out, _ := decodeJSON(t, w)["items"].([]interface{})
		return out
	}

	// The default list is unchanged: no client's idea of "my patches" gains
	// a row it cannot open.
	if got := items(adminToken, ""); len(got) != 0 {
		t.Errorf("an archived patch reached the default list: %v", got)
	}
	archived := items(adminToken, "?status=archived")
	if len(archived) != 1 {
		t.Fatalf("expected one archived patch, got %d", len(archived))
	}
	row := archived[0].(map[string]interface{})
	if row["node_slug"] != "in-my-list" || row["node_status"] != "archived" {
		t.Errorf("unexpected row: %v", row)
	}
	// Somebody who is not its admin answers for nothing.
	if got := items(strangerToken, "?status=archived"); len(got) != 0 {
		t.Errorf("a stranger was handed somebody else's archived patch: %v", got)
	}
}

// Being carried along on a patch somebody else archived is not a thing to
// answer for, so it is not in the list either.
func TestArchived_AMemberIsNotAskedToAnswerForIt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := archivedPatch(t, db, "not-mine")
	member, memberToken := createTestUser(t, db, "not-mine-member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	r := authedRequest("GET", "/api/v1/me/nodes?status=archived", nil, memberToken)
	w := serveMux(t, db, "GET", "/api/v1/me/nodes", handler.ListMyMemberships(db), r)
	out, _ := decodeJSON(t, w)["items"].([]interface{})
	if len(out) != 0 {
		t.Errorf("an ordinary member was shown an archived patch: %v", out)
	}
}
