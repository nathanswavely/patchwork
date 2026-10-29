package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func getAIPage(t *testing.T, db *database.DB) map[string]interface{} {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ai", handler.AIPage(db, testConfig()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/ai", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET ai: got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return resp
}

// The project part is rendered from the code it describes: no placeholder
// survives, and the crawler list is the one the middleware enforces.
func TestAIPageRendersFromTheCode(t *testing.T) {
	db := setupTestDB(t)
	resp := getAIPage(t, db)

	md, _ := resp["project_markdown"].(string)
	for _, ph := range []string{"{quilt_name}", "{outbound}", "{ai_crawlers}"} {
		if strings.Contains(md, ph) {
			t.Errorf("placeholder %s left unsubstituted", ph)
		}
	}
	if !strings.Contains(md, "Yaml Quilt") {
		t.Errorf("project part should carry the effective instance name")
	}
	for _, name := range middleware.AICrawlerNames() {
		if !strings.Contains(md, name) {
			t.Errorf("crawler %s is refused by the site but missing from the page", name)
		}
	}

	if resp["stewards_markdown"] != "" {
		t.Errorf("a fresh quilt's stewards have written nothing, got %q", resp["stewards_markdown"])
	}
}

func TestAIStewardsWriteAndRemove(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")

	r := authedRequest("PUT", "/api/v1/admin/ai-page",
		map[string]string{"markdown": "  Label what a model made.  "}, token)
	w := serveAdmin(db, "PUT", "/api/v1/admin/ai-page", handler.AdminUpdateAIStewards(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: got %d: %s", w.Code, w.Body.String())
	}
	resp := getAIPage(t, db)
	if resp["stewards_markdown"] != "Label what a model made." {
		t.Fatalf("stewards' part not served back: %q", resp["stewards_markdown"])
	}
	if at, _ := resp["stewards_updated_at"].(string); at == "" {
		t.Fatalf("stewards' part should carry updated_at")
	}

	r = authedRequest("DELETE", "/api/v1/admin/ai-page", nil, token)
	w = serveAdmin(db, "DELETE", "/api/v1/admin/ai-page", handler.AdminResetAIStewards(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE: got %d: %s", w.Code, w.Body.String())
	}
	if getAIPage(t, db)["stewards_markdown"] != "" {
		t.Fatalf("stewards' part should be gone after DELETE")
	}
}

func TestAIStewardsRejectsEmptyAndNonAdmins(t *testing.T) {
	db := setupTestDB(t)
	_, admin := createTestUser(t, db, "boss", "admin")
	_, member := createTestUser(t, db, "plain", "member")

	r := authedRequest("PUT", "/api/v1/admin/ai-page", map[string]string{"markdown": "   "}, admin)
	if w := serveAdmin(db, "PUT", "/api/v1/admin/ai-page", handler.AdminUpdateAIStewards(db), r); w.Code != http.StatusBadRequest {
		t.Errorf("empty text: got %d, want 400", w.Code)
	}

	r = authedRequest("PUT", "/api/v1/admin/ai-page", map[string]string{"markdown": "mine now"}, member)
	if w := serveAdmin(db, "PUT", "/api/v1/admin/ai-page", handler.AdminUpdateAIStewards(db), r); w.Code != http.StatusForbidden {
		t.Errorf("non-admin PUT: got %d, want 403", w.Code)
	}
}
