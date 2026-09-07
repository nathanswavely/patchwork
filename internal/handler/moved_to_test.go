package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The moved-to pointer (docs/adr/090). Two columns, two owners: a patch's
// admins set the patch's, and a person sets their own.

// patchNode sends a PATCH to /api/v1/nodes/{slug} as token.
func patchNode(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/nodes/"+slug, body, token)
	return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
}

// patchMe sends a PATCH to /api/v1/auth/me as token.
func patchMe(t *testing.T, db *database.DB, token string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/auth/me", body, token)
	return serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
}

func nodeMovedToColumn(t *testing.T, db *database.DB, nodeID string) string {
	t.Helper()
	var moved string
	db.QueryRow("SELECT COALESCE(moved_to,'') FROM nodes WHERE id = ?", nodeID).Scan(&moved)
	return moved
}

// An admin sets the pointer, the read surfaces carry it, and clearing it
// puts the patch back where it was. Clearing matters as much as setting: a
// move called off has to be undoable by the people who called it.
func TestPatchMovedToSetAndClear(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "moving-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	w := patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{
		"moved_to": "  https://new.example/patches/gallery-row  ",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("set moved_to: %d %s", w.Code, w.Body.String())
	}
	if got := nodeMovedToColumn(t, db, nodeID); got != "https://new.example/patches/gallery-row" {
		t.Fatalf("stored moved_to: %q", got)
	}

	// The detail read is what draws the banner.
	r := authedRequest("GET", "/api/v1/nodes/gallery-row", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	var detail struct {
		Node model.Node `json:"node"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Node.MovedTo != "https://new.example/patches/gallery-row" {
		t.Errorf("GetNode moved_to: %q", detail.Node.MovedTo)
	}

	// The tree is what the discovery card reads (docs/adr/074).
	r = authedRequest("GET", "/api/v1/nodes/tree", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if !containsJSONString(w.Body.Bytes(), "https://new.example/patches/gallery-row") {
		t.Errorf("tree did not carry moved_to: %s", w.Body.String())
	}

	// Cleared with an empty string, and the column goes back to NULL rather
	// than to "" so "has not moved" has exactly one representation.
	w = patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{"moved_to": ""})
	if w.Code != http.StatusOK {
		t.Fatalf("clear moved_to: %d %s", w.Code, w.Body.String())
	}
	var isNull bool
	db.QueryRow("SELECT moved_to IS NULL FROM nodes WHERE id = ?", nodeID).Scan(&isNull)
	if !isNull {
		t.Errorf("cleared moved_to should be NULL, got %q", nodeMovedToColumn(t, db, nodeID))
	}
}

// A member is not an admin, and this is a settings field.
func TestPatchMovedToNeedsAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "settled-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	member, memberToken := createTestUser(t, db, "ordinary-member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	w := patchNode(t, db, "the-selvage", memberToken, map[string]interface{}{
		"moved_to": "https://elsewhere.example/patches/the-selvage",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("member setting moved_to: expected 403, got %d", w.Code)
	}
	if got := nodeMovedToColumn(t, db, nodeID); got != "" {
		t.Errorf("member's write landed anyway: %q", got)
	}
}

// The value is rendered as an href, so the scheme rule is event_url's
// (docs/adr/079), plus one clause of its own: a new home somewhere else.
func TestMovedToRejectsBadLinks(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("arts.lancaster.example")
	t.Cleanup(func() { ap.SetDomain("") })

	admin, adminToken := createTestUser(t, db, "scheme-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Scheme Hall", "scheme-hall", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	refused := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"ftp://files.example/patches/scheme-hall",
		"not a link at all",
		// The instance's own origin: a pointer back here is a loop, not a
		// move, and it is the one shape a typo produces most easily.
		"https://arts.lancaster.example/patches/scheme-hall",
		"http://ARTS.LANCASTER.EXAMPLE/patches/scheme-hall",
	}
	for _, bad := range refused {
		w := patchNode(t, db, "scheme-hall", adminToken, map[string]interface{}{"moved_to": bad})
		if w.Code != http.StatusBadRequest {
			t.Errorf("moved_to %q: expected 400, got %d", bad, w.Code)
		}
		if got := nodeMovedToColumn(t, db, nodeID); got != "" {
			t.Errorf("moved_to %q was stored as %q", bad, got)
		}
	}

	// http is allowed on purpose, and so is a quilt on another port.
	for _, ok := range []string{
		"http://plain.example/patches/scheme-hall",
		"https://arts.lancaster.example:8443/patches/scheme-hall",
	} {
		w := patchNode(t, db, "scheme-hall", adminToken, map[string]interface{}{"moved_to": ok})
		if w.Code != http.StatusOK {
			t.Errorf("moved_to %q: expected 200, got %d %s", ok, w.Code, w.Body.String())
		}
	}

	// The person's own field takes the same rule, checked at its own path.
	_, token := createTestUser(t, db, "scheme-person", "member")
	w := patchMe(t, db, token, map[string]interface{}{"moved_to": "javascript:alert(1)"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("profile moved_to javascript: expected 400, got %d", w.Code)
	}
	w = patchMe(t, db, token, map[string]interface{}{"moved_to": "https://arts.lancaster.example/users/scheme-person"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("profile moved_to pointing here: expected 400, got %d", w.Code)
	}
}

// A person's own pointer: set at account settings, read on the public
// profile, cleared the same way a patch's is.
func TestProfileMovedToSetAndClear(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "wanderer", "member")

	w := patchMe(t, db, token, map[string]interface{}{"moved_to": " https://elsewhere.example/users/wanderer "})
	if w.Code != http.StatusOK {
		t.Fatalf("set profile moved_to: %d %s", w.Code, w.Body.String())
	}
	var me model.User
	json.Unmarshal(w.Body.Bytes(), &me)
	if me.MovedTo != "https://elsewhere.example/users/wanderer" {
		t.Errorf("PATCH auth/me response moved_to: %q", me.MovedTo)
	}

	r := authedRequest("GET", "/api/v1/users/wanderer", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/users/{username}", handler.GetUserProfile(db), r)
	profile := map[string]interface{}{}
	json.Unmarshal(w.Body.Bytes(), &profile)
	if profile["moved_to"] != "https://elsewhere.example/users/wanderer" {
		t.Errorf("public profile moved_to: %v", profile["moved_to"])
	}

	w = patchMe(t, db, token, map[string]interface{}{"moved_to": ""})
	if w.Code != http.StatusOK {
		t.Fatalf("clear profile moved_to: %d", w.Code)
	}
	var isNull bool
	db.QueryRow("SELECT moved_to IS NULL FROM users WHERE id = ?", user.ID).Scan(&isNull)
	if !isNull {
		t.Error("cleared profile moved_to should be NULL")
	}
}

// A moved patch is read-mostly (docs/adr/090): it turns away the two acts
// that would start a relationship with a room the community has left, and
// says where to go instead. Everything a member could already do still
// works, because the old home stays a record.
func TestMovedPatchDeclinesJoinAndFollow(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "left-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Old Hall", "old-hall", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec("UPDATE nodes SET moved_to = ? WHERE id = ?", "https://new.example/patches/old-hall", nodeID)

	_, strangerToken := createTestUser(t, db, "newcomer", "member")

	for _, body := range []map[string]interface{}{
		{},
		{"role": "follower"},
	} {
		r := authedRequest("POST", "/api/v1/nodes/old-hall/join", body, strangerToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/join", handler.JoinNode(db), r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("join %v on a moved patch: expected 403, got %d %s", body, w.Code, w.Body.String())
		}
		var refusal map[string]string
		json.Unmarshal(w.Body.Bytes(), &refusal)
		if refusal["moved_to"] != "https://new.example/patches/old-hall" {
			t.Errorf("refusal did not carry the pointer: %v", refusal)
		}
		if refusal["error"] == "" {
			t.Error("refusal carried no message")
		}
	}

	// The admin's own acts are untouched: they can still edit the patch,
	// which is how the pointer gets cleared if the move is called off.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ?", nodeID).Scan(&count)
	if count != 1 {
		t.Errorf("a refused join should leave no membership row: %d rows", count)
	}
}

// An event suggestion from outside is declined the same way, and a member's
// own event still posts.
func TestMovedPatchDeclinesOutsideEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := submissionsCfg(true)
	admin, adminToken := createTestUser(t, db, "moved-venue-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Moved Venue", "moved-venue", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec("UPDATE nodes SET moved_to = ? WHERE id = ?", "https://new.example/patches/moved-venue", nodeID)

	_, strangerToken := createTestUser(t, db, "passerby", "member")
	r := authedRequest("POST", "/api/v1/events", eventBody(nodeID, "Suggested Show"), strangerToken)
	w := serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, cfg), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("outsider's event on a moved patch: expected 403, got %d %s", w.Code, w.Body.String())
	}
	var refusal map[string]string
	json.Unmarshal(w.Body.Bytes(), &refusal)
	if refusal["moved_to"] != "https://new.example/patches/moved-venue" {
		t.Errorf("refusal did not carry the pointer: %v", refusal)
	}

	// The patch's own admin still posts. The old home is a record, and a
	// record can be corrected.
	_, code := createEventVia(t, db, cfg, adminToken, eventBody(nodeID, "Last Show Here"))
	if code != http.StatusCreated {
		t.Fatalf("admin's event on a moved patch: expected 201, got %d", code)
	}
}

// The actor document carries `movedTo` only when there is one, and brings
// the context term with it (docs/adr/090).
func TestActorCarriesMovedTo(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("arts.lancaster.example")
	t.Cleanup(func() { ap.SetDomain("") })

	admin, _ := createTestUser(t, db, "federating-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Federated Hall", "federated-hall", "open")

	fetchActor := func(path, pattern string, h http.HandlerFunc) map[string]interface{} {
		t.Helper()
		r := authedRequest("GET", path, nil, "")
		r.Header.Set("Accept", "application/activity+json")
		w := servePublicMux(t, "GET", pattern, h, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, w.Code)
		}
		doc := map[string]interface{}{}
		json.Unmarshal(w.Body.Bytes(), &doc)
		return doc
	}

	doc := fetchActor("/ap/nodes/"+nodeID, "/ap/nodes/{id}", handler.APNode(db))
	if _, present := doc["movedTo"]; present {
		t.Error("an unmoved patch's actor carried movedTo")
	}
	if _, isString := doc["@context"].(string); !isString {
		t.Errorf("an unmoved actor's @context should stay the plain string, got %T", doc["@context"])
	}

	db.Exec("UPDATE nodes SET moved_to = ? WHERE id = ?", "https://new.example/patches/federated-hall", nodeID)
	doc = fetchActor("/ap/nodes/"+nodeID, "/ap/nodes/{id}", handler.APNode(db))
	if doc["movedTo"] != "https://new.example/patches/federated-hall" {
		t.Errorf("patch actor movedTo: %v", doc["movedTo"])
	}
	ctx, isArray := doc["@context"].([]interface{})
	if !isArray || len(ctx) != 2 {
		t.Fatalf("a moved actor needs the term defined in @context, got %v", doc["@context"])
	}

	person, _ := createTestUser(t, db, "federating-person", "member")
	doc = fetchActor("/ap/users/"+person.ID, "/ap/users/{id}", handler.APUser(db))
	if _, present := doc["movedTo"]; present {
		t.Error("an unmoved person's actor carried movedTo")
	}
	db.Exec("UPDATE users SET moved_to = ? WHERE id = ?", "https://new.example/users/federating-person", person.ID)
	doc = fetchActor("/ap/users/"+person.ID, "/ap/users/{id}", handler.APUser(db))
	if doc["movedTo"] != "https://new.example/users/federating-person" {
		t.Errorf("person actor movedTo: %v", doc["movedTo"])
	}
}

// A tombstone says nothing about where to find the person (docs/adr/086).
func TestDeletingAnAccountClearsMovedTo(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "instance-boss-moved", "admin")
	user, token := createTestUser(t, db, "moving-leaver", "member")
	db.Exec("UPDATE users SET moved_to = ? WHERE id = ?", "https://elsewhere.example/users/moving-leaver", user.ID)

	if w := deleteMe(t, db, token, "moving-leaver"); w.Code != http.StatusOK {
		t.Fatalf("delete account: %d %s", w.Code, w.Body.String())
	}

	var moved *string
	db.QueryRow("SELECT moved_to FROM users WHERE id = ?", user.ID).Scan(&moved)
	if moved != nil {
		t.Errorf("tombstone kept moved_to: %q", *moved)
	}
}

// containsJSONString reports whether an encoded body contains the value as a
// JSON string. Cheaper than modelling the whole tree payload for one field.
func containsJSONString(body []byte, want string) bool {
	quoted, _ := json.Marshal(want)
	return bytes.Contains(body, quoted)
}
