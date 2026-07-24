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

func TestCreateGovernanceDoc(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gadmin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gov Node", "gov-node", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]interface{}{
		"title": "Test Lining",
		"body":  "This is a test governance document.",
	}
	r := authedRequest("POST", "/api/v1/nodes/gov-node/governance", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["title"] != "Test Lining" {
		t.Errorf("expected title=Test Lining, got %v", result["title"])
	}
	if result["version"].(float64) != 1 {
		t.Errorf("expected version=1, got %v", result["version"])
	}
}

func TestListGovernanceDocs(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gadmin2", "member")
	nodeID := createTestNode(t, db, admin.ID, "List Gov", "list-gov", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create a doc. Published, so an anonymous list sees it (docs/adr/036).
	body := map[string]interface{}{"title": "Doc 1", "body": "Content", "visibility": "public"}
	r := authedRequest("POST", "/api/v1/nodes/list-gov/governance", body, adminToken)
	serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)

	// List.
	r = httptest.NewRequest("GET", "/api/v1/nodes/list-gov/governance", nil)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes/{slug}/governance", handler.ListGovernanceDocs(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 1 {
		t.Errorf("expected 1 doc, got %d", len(items))
	}
}

func TestUpdateGovernanceDocVersionIncrements(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gadmin3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Update Gov", "update-gov", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create a doc.
	body := map[string]interface{}{"title": "Versioned", "body": "v1 content"}
	r := authedRequest("POST", "/api/v1/nodes/update-gov/governance", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)
	createResult := decodeJSON(t, w)
	docID := createResult["id"].(string)

	// Update.
	updateBody := map[string]interface{}{"title": "Versioned Updated", "body": "v2 content"}
	r = authedRequest("PUT", "/api/v1/governance/"+docID, updateBody, adminToken)
	w = serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["version"].(float64) != 2 {
		t.Errorf("expected version=2, got %v", result["version"])
	}
	if result["title"] != "Versioned Updated" {
		t.Errorf("expected title=Versioned Updated, got %v", result["title"])
	}

	// Update again.
	updateBody = map[string]interface{}{"title": "Versioned Updated Again", "body": "v3 content"}
	r = authedRequest("PUT", "/api/v1/governance/"+docID, updateBody, adminToken)
	w = serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result = decodeJSON(t, w)
	if result["version"].(float64) != 3 {
		t.Errorf("expected version=3, got %v", result["version"])
	}
}

func TestDefaultLiningOnNodeCreation(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "gcreator4", "member")

	body := map[string]string{
		"name": "Lining Auto Node",
	}
	r := authedRequest("POST", "/api/v1/nodes", body, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var node map[string]interface{}
	json.NewDecoder(w.Body).Decode(&node)
	nodeID := node["id"].(string)

	// Check that a governance doc was auto-created.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM governance_docs WHERE node_id = ?", nodeID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 governance doc, got %d", count)
	}

	var title, docBody string
	db.QueryRow("SELECT title, body FROM governance_docs WHERE node_id = ?", nodeID).Scan(&title, &docBody)
	if title != handler.DefaultLiningTitle {
		t.Errorf("expected title=%q, got %q", handler.DefaultLiningTitle, title)
	}
	if docBody != handler.DefaultLiningBody {
		t.Errorf("expected default lining body, got %q", docBody)
	}

	_ = user
}

func TestGetGovernanceDoc(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gadmin5", "member")
	nodeID := createTestNode(t, db, admin.ID, "Get Gov", "get-gov", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Create a doc, published for public reading.
	body := map[string]interface{}{"title": "Get Test", "body": "Some content", "visibility": "public"}
	r := authedRequest("POST", "/api/v1/nodes/get-gov/governance", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)
	createResult := decodeJSON(t, w)
	docID := createResult["id"].(string)

	// Get.
	r = httptest.NewRequest("GET", "/api/v1/governance/"+docID, nil)
	w = serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["title"] != "Get Test" {
		t.Errorf("expected title=Get Test, got %v", result["title"])
	}
	if result["body"] != "Some content" {
		t.Errorf("expected body=Some content, got %v", result["body"])
	}
}

// --- Per-document visibility (docs/adr/036) ---

// createGovDoc posts a governance doc and returns its id.
func createGovDoc(t *testing.T, db *database.DB, slug, token, title, visibility string) string {
	t.Helper()
	body := map[string]interface{}{"title": title, "body": "secret rules"}
	if visibility != "" {
		body["visibility"] = visibility
	}
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/governance", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create doc: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)["id"].(string)
}

func TestGovernanceDocDefaultsToMembersOnly(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gvis1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Vis Default", "vis-default", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	docID := createGovDoc(t, db, "vis-default", adminToken, "House Rules", "")

	var visibility string
	db.QueryRow("SELECT visibility FROM governance_docs WHERE id = ?", docID).Scan(&visibility)
	if visibility != "members" {
		t.Errorf("expected new doc to be members-only, got %q", visibility)
	}

	// Anonymous list omits it, and the doc itself reads as not found.
	r := httptest.NewRequest("GET", "/api/v1/nodes/vis-default/governance", nil)
	w := serveOptionalAuthMux(t, db, "GET", "/api/v1/nodes/{slug}/governance", handler.ListGovernanceDocs(db), r)
	if items := decodeJSON(t, w)["items"].([]interface{}); len(items) != 0 {
		t.Errorf("expected anonymous list to be empty, got %d docs", len(items))
	}

	r = httptest.NewRequest("GET", "/api/v1/governance/"+docID, nil)
	w = serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for anonymous read of members-only doc, got %d", w.Code)
	}

	// The patch's own admin still reads it.
	r = authedRequest("GET", "/api/v1/governance/"+docID, nil, adminToken)
	w = serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("expected admin to read members-only doc, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGovernanceDocVisibilityFlip(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gvis2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Vis Flip", "vis-flip", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	docID := createGovDoc(t, db, "vis-flip", adminToken, "Charter", "")

	r := authedRequest("PUT", "/api/v1/governance/"+docID, map[string]interface{}{"visibility": "public"}, adminToken)
	w := serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodeJSON(t, w)
	if result["visibility"] != "public" {
		t.Errorf("expected visibility=public, got %v", result["visibility"])
	}
	// Publishing is not an amendment — the text didn't change, so neither did
	// the version.
	if result["version"].(float64) != 1 {
		t.Errorf("expected version to stay 1 on a visibility flip, got %v", result["version"])
	}

	// Now anonymous readers see it.
	r = httptest.NewRequest("GET", "/api/v1/governance/"+docID, nil)
	w = serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 after publishing, got %d", w.Code)
	}

	// A bad value is refused rather than silently coerced.
	r = authedRequest("PUT", "/api/v1/governance/"+docID, map[string]interface{}{"visibility": "everyone"}, adminToken)
	w = serveMux(t, db, "PUT", "/api/v1/governance/{id}", handler.UpdateGovernanceDoc(db), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid visibility, got %d", w.Code)
	}
}

func TestGovernanceDocVisibilityForNonMembers(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "gvis3", "member")
	nodeID := createTestNode(t, db, admin.ID, "Vis Roles", "vis-roles", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	member, memberToken := createTestUser(t, db, "gvis3member", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	outsider, outsiderToken := createTestUser(t, db, "gvis3outsider", "member")
	_ = outsider

	docID := createGovDoc(t, db, "vis-roles", adminToken, "Members Only", "")

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"member", memberToken, http.StatusOK},
		{"signed-in outsider", outsiderToken, http.StatusNotFound},
	} {
		r := authedRequest("GET", "/api/v1/governance/"+docID, nil, tc.token)
		w := serveOptionalAuthMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
		if w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d", tc.name, tc.want, w.Code)
		}
	}
}
