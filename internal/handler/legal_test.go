package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

func getLegal(t *testing.T, db *database.DB, doc string) map[string]interface{} {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/legal/{doc}", handler.LegalDoc(db, testConfig()))
	r := httptest.NewRequest("GET", "/api/v1/legal/"+doc, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET legal/%s: got %d: %s", doc, w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return resp
}

func TestLegalDefaultsServeWithQuiltName(t *testing.T) {
	db := setupTestDB(t)

	for doc, wantTitle := range map[string]string{
		"privacy": "Privacy Policy",
		"terms":   "User Agreement",
	} {
		resp := getLegal(t, db, doc)
		if resp["title"] != wantTitle {
			t.Errorf("%s title = %q, want %q", doc, resp["title"], wantTitle)
		}
		if resp["customized"] != false {
			t.Errorf("%s: fresh instance should serve the default", doc)
		}
		md, _ := resp["markdown"].(string)
		if !strings.Contains(md, "Yaml Quilt") {
			t.Errorf("%s: default should carry the effective instance name", doc)
		}
		if strings.Contains(md, "{quilt_name}") {
			t.Errorf("%s: placeholder left unsubstituted", doc)
		}
	}
}

func TestLegalDefaultHonorsRename(t *testing.T) {
	db := setupTestDB(t)
	if err := settings.Set(db, settings.KeyName, "Renamed Quilt"); err != nil {
		t.Fatal(err)
	}
	md, _ := getLegal(t, db, "privacy")["markdown"].(string)
	if !strings.Contains(md, "Renamed Quilt") || strings.Contains(md, "Yaml Quilt") {
		t.Fatalf("default doc should render the DB-overridden name")
	}
}

func TestLegalUnknownDoc404(t *testing.T) {
	db := setupTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/legal/{doc}", handler.LegalDoc(db, testConfig()))
	r := httptest.NewRequest("GET", "/api/v1/legal/imprint", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown doc: got %d, want 404", w.Code)
	}
}

func TestLegalCustomizeAndReset(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")

	// Customize the terms.
	r := authedRequest("PUT", "/api/v1/admin/legal/terms",
		map[string]string{"markdown": "## House Rules\nBe kind."}, token)
	w := serveAdmin(db, "PUT", "/api/v1/admin/legal/{doc}", handler.AdminUpdateLegal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: got %d: %s", w.Code, w.Body.String())
	}

	resp := getLegal(t, db, "terms")
	if resp["customized"] != true {
		t.Fatalf("terms should report customized after PUT")
	}
	if md, _ := resp["markdown"].(string); md != "## House Rules\nBe kind." {
		t.Fatalf("custom markdown not served back: %q", md)
	}
	if at, _ := resp["updated_at"].(string); at == "" {
		t.Fatalf("customized doc should carry updated_at")
	}
	// The other document is untouched.
	if getLegal(t, db, "privacy")["customized"] != false {
		t.Fatalf("privacy should still be the default")
	}

	// Reset restores the shipped default.
	r = authedRequest("DELETE", "/api/v1/admin/legal/terms", nil, token)
	w = serveAdmin(db, "DELETE", "/api/v1/admin/legal/{doc}", handler.AdminResetLegal(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE: got %d: %s", w.Code, w.Body.String())
	}
	resp = getLegal(t, db, "terms")
	if resp["customized"] != false {
		t.Fatalf("terms should be back to default after reset")
	}
	if md, _ := resp["markdown"].(string); !strings.Contains(md, "Yaml Quilt") {
		t.Fatalf("reset should serve the rendered default again")
	}
}

func TestLegalRejectsEmptyCustomDoc(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")

	r := authedRequest("PUT", "/api/v1/admin/legal/privacy",
		map[string]string{"markdown": "   "}, token)
	w := serveAdmin(db, "PUT", "/api/v1/admin/legal/{doc}", handler.AdminUpdateLegal(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty doc: got %d, want 400", w.Code)
	}
}

func TestLegalAdminEndpointsRequireAdmin(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "plain", "member")

	r := authedRequest("PUT", "/api/v1/admin/legal/privacy",
		map[string]string{"markdown": "mine now"}, token)
	w := serveAdmin(db, "PUT", "/api/v1/admin/legal/{doc}", handler.AdminUpdateLegal(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin PUT: got %d, want 403", w.Code)
	}
}

func TestAdminGetLegalIncludesDefaults(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "boss", "admin")

	if err := settings.Set(db, settings.KeyLegalPrivacy, "custom privacy"); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("GET", "/api/v1/admin/legal", nil, token)
	w := serveAdmin(db, "GET", "/api/v1/admin/legal", handler.AdminGetLegal(db, testConfig()), r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET admin/legal: got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Docs []struct {
			Doc             string `json:"doc"`
			Markdown        string `json:"markdown"`
			Customized      bool   `json:"customized"`
			DefaultMarkdown string `json:"default_markdown"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(resp.Docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(resp.Docs))
	}
	for _, d := range resp.Docs {
		if d.Doc == "privacy" {
			if !d.Customized || d.Markdown != "custom privacy" {
				t.Errorf("privacy should be customized")
			}
			if !strings.Contains(d.DefaultMarkdown, "Yaml Quilt") {
				t.Errorf("default_markdown should still carry the rendered default")
			}
		}
	}
}
