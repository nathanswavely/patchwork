package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// A patch can say it is a nonprofit (#448): its admins at creation and in
// settings, and whoever suggests a listing. It is a column rather than a tag
// because tags place patches and draw motifs (docs/adr/021), and how a group
// is constituted says nothing about who it is like.

func nodeNonprofitColumn(t *testing.T, db *database.DB, nodeID string) bool {
	t.Helper()
	var v bool
	if err := db.QueryRow(`SELECT nonprofit FROM nodes WHERE id = ?`, nodeID).Scan(&v); err != nil {
		t.Fatalf("read nonprofit: %v", err)
	}
	return v
}

func TestNonprofit_SetAtCreation(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "npcreator", "member")

	r := authedRequest("POST", "/api/v1/nodes", map[string]interface{}{"name": "Food Bank", "nonprofit": true}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}
	var n model.Node
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if !n.Nonprofit {
		t.Errorf("create response nonprofit = false: %s", w.Body.String())
	}
	if !nodeNonprofitColumn(t, db, n.ID) {
		t.Error("stored nonprofit = false, want true")
	}
}

// A patch admin sets it in settings, the detail and the tree both say it, and
// an unrelated edit leaves it alone — the settings form spreads what it
// loaded, and a field the PATCH reset on omission would silently clear.
func TestNonprofit_AdminSetsItAndItSurvivesOtherEdits(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "npadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	if nodeNonprofitColumn(t, db, nodeID) {
		t.Fatal("a new patch should not start as a nonprofit")
	}

	w := patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{"nonprofit": true})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: got %d %s", w.Code, w.Body.String())
	}
	var updated model.Node
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if !updated.Nonprofit {
		t.Errorf("update response nonprofit = false: %s", w.Body.String())
	}

	w = patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{"description": "A gallery."})
	if w.Code != http.StatusOK {
		t.Fatalf("unrelated patch: got %d %s", w.Code, w.Body.String())
	}
	if !nodeNonprofitColumn(t, db, nodeID) {
		t.Error("an edit that did not mention nonprofit cleared it")
	}

	r := authedRequest("GET", "/api/v1/nodes/gallery-row", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/{slug}", handler.GetNode(db), r)
	var detail struct {
		Node model.Node `json:"node"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if !detail.Node.Nonprofit {
		t.Errorf("GetNode nonprofit = false: %s", w.Body.String())
	}

	r = authedRequest("GET", "/api/v1/nodes/tree", nil, "")
	w = servePublicMux(t, "GET", "/api/v1/nodes/tree", handler.NodeTree(db), r)
	if !bodyContains(w.Body.Bytes(), `"nonprofit":true`) {
		t.Errorf("tree did not carry nonprofit: %s", w.Body.String())
	}

	w = patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{"nonprofit": false})
	if w.Code != http.StatusOK {
		t.Fatalf("clear: got %d %s", w.Code, w.Body.String())
	}
	if nodeNonprofitColumn(t, db, nodeID) {
		t.Error("nonprofit: false did not clear it")
	}
}

func TestNonprofit_RejectsANonBool(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "npbool", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	w := patchNode(t, db, "gallery-row", adminToken, map[string]interface{}{"nonprofit": "yes"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("a string nonprofit: got %d, want 400", w.Code)
	}
}

// Only the patch's admins say what the patch is: a member cannot.
func TestNonprofit_MemberCannotSetIt(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "npown", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "npmember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	w := patchNode(t, db, "gallery-row", memberToken, map[string]interface{}{"nonprofit": true})
	if w.Code != http.StatusForbidden {
		t.Errorf("member patch: got %d, want 403", w.Code)
	}
	if nodeNonprofitColumn(t, db, nodeID) {
		t.Error("a member's request set nonprofit")
	}
}

// A suggester's answer reaches the review queue, and stands through an
// approval that does not mention it.
func TestNonprofit_SuggestionCarriesItThroughReview(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "npsuggester", "member")
	_, adminToken := createTestUser(t, db, "npsiteadmin", "admin")

	w, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Community Fridge", "nonprofit": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("submit: got %d %s", w.Code, w.Body.String())
	}
	if !nodeNonprofitColumn(t, db, nodeID) {
		t.Fatal("the suggestion's nonprofit answer was not stored")
	}

	r := authedRequest("GET", "/api/v1/admin/submissions", nil, adminToken)
	w = serveAdminMux(t, db, "GET", "/api/v1/admin/submissions", handler.ListSubmissions(db), r)
	if !bodyContains(w.Body.Bytes(), `"nonprofit":true`) {
		t.Errorf("queue item does not show nonprofit: %s", w.Body.String())
	}

	if w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{"action": "approve"}); w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	if !nodeNonprofitColumn(t, db, nodeID) {
		t.Error("approval without a correction cleared nonprofit")
	}
}

// The reviewer may correct it at approval, as they may the tags.
func TestNonprofit_ReviewerCorrectsIt(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "npsuggester2", "member")
	_, adminToken := createTestUser(t, db, "npsiteadmin2", "admin")

	_, nodeID := submitSuggestion(t, db, token, map[string]interface{}{
		"name": "Corner Bar", "nonprofit": true,
	})
	w := reviewSubmission(t, db, adminToken, nodeID, map[string]interface{}{"action": "approve", "nonprofit": false})
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d %s", w.Code, w.Body.String())
	}
	if nodeNonprofitColumn(t, db, nodeID) {
		t.Error("the reviewer's correction did not land")
	}
}
