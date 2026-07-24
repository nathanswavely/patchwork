package handler_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

func testConfig() *config.Config {
	return &config.Config{
		Instance: config.Instance{
			Name:        "Yaml Quilt",
			Domain:      "quilt.example.com",
			Description: "from yaml",
		},
	}
}

func serveAdmin(db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AdminRequired(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestQuiltRenameOverridesYaml(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	// Rename via PATCH.
	r := authedRequest("PATCH", "/api/v1/admin/settings",
		map[string]string{"name": "Renamed Quilt", "description": "new desc"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("rename: got %d: %s", w.Code, w.Body.String())
	}

	if got := settings.EffectiveName(db, cfg); got != "Renamed Quilt" {
		t.Fatalf("effective name = %q, want Renamed Quilt", got)
	}

	// Public instance endpoint reflects the override and exposes icon_url.
	pub := httptest.NewRequest("GET", "/api/v1/instance", nil)
	pw := httptest.NewRecorder()
	handler.Instance(db, cfg)(pw, pub)
	var resp struct {
		Name    string `json:"name"`
		IconURL string `json:"icon_url"`
	}
	json.NewDecoder(pw.Body).Decode(&resp)
	if resp.Name != "Renamed Quilt" {
		t.Errorf("instance name = %q, want Renamed Quilt", resp.Name)
	}
	if resp.IconURL != "/api/v1/instance/icon" {
		t.Errorf("icon_url = %q", resp.IconURL)
	}

	// Empty name is refused.
	r = authedRequest("PATCH", "/api/v1/admin/settings", map[string]string{"name": "  "}, token)
	w = serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want 400", w.Code)
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadIcon(db *database.DB, token, contentType string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PUT", "/api/v1/admin/settings/icon", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-Patchwork-Request", "true")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	return serveAdmin(db, "PUT", "/api/v1/admin/settings/icon", handler.AdminUploadIcon(db), r)
}

func TestQuiltIconUploadAndServe(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	// Default: no upload → generated SVG block.
	r := httptest.NewRequest("GET", "/api/v1/instance/icon", nil)
	w := httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("default icon: code %d, type %q", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "<svg") {
		t.Fatal("default icon body is not SVG")
	}

	// Non-square upload is refused.
	if w := uploadIcon(db, token, "image/png", encodePNG(t, 128, 64)); w.Code != http.StatusBadRequest {
		t.Errorf("non-square: got %d, want 400", w.Code)
	}
	// Too small is refused.
	if w := uploadIcon(db, token, "image/png", encodePNG(t, 32, 32)); w.Code != http.StatusBadRequest {
		t.Errorf("too small: got %d, want 400", w.Code)
	}
	// Mismatched declared format is refused.
	if w := uploadIcon(db, token, "image/jpeg", encodePNG(t, 128, 128)); w.Code != http.StatusBadRequest {
		t.Errorf("format mismatch: got %d, want 400", w.Code)
	}
	// SVG (or any non-raster type) is refused outright.
	if w := uploadIcon(db, token, "image/svg+xml", []byte("<svg/>")); w.Code != http.StatusBadRequest {
		t.Errorf("svg: got %d, want 400", w.Code)
	}

	// A valid square PNG is accepted.
	if w := uploadIcon(db, token, "image/png", encodePNG(t, 128, 128)); w.Code != http.StatusOK {
		t.Fatalf("valid upload: got %d: %s", w.Code, w.Body.String())
	}

	// A valid square JPEG replaces it.
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, image.NewRGBA(image.Rect(0, 0, 256, 256)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	if w := uploadIcon(db, token, "image/jpeg", jbuf.Bytes()); w.Code != http.StatusOK {
		t.Fatalf("jpeg upload: got %d: %s", w.Code, w.Body.String())
	}

	// Now the public endpoint serves the upload.
	r = httptest.NewRequest("GET", "/api/v1/instance/icon", nil)
	w = httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("served type = %q, want image/jpeg", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("ETag") == "" {
		t.Error("uploaded icon served without ETag")
	}

	// Delete reverts to a default block.
	dr := authedRequest("DELETE", "/api/v1/admin/settings/icon", nil, token)
	if w := serveAdmin(db, "DELETE", "/api/v1/admin/settings/icon", handler.AdminDeleteIcon(db), dr); w.Code != http.StatusOK {
		t.Fatalf("delete icon: got %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/api/v1/instance/icon", nil)
	w = httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("after delete: type = %q, want image/svg+xml", w.Header().Get("Content-Type"))
	}

	// Block preview for the picker.
	r = httptest.NewRequest("GET", "/api/v1/instance/icon?block=pinwheel", nil)
	w = httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("block preview: got %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/api/v1/instance/icon?block=nonsense", nil)
	w = httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown block: got %d, want 404", w.Code)
	}
}

func TestQuiltWipe(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	admin, token := createTestUser(t, db, "boss", "admin")
	nodeID := createTestNode(t, db, admin.ID, "Band", "band", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Wrong confirmation name refuses and deletes nothing.
	r := authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": "wrong"}, token)
	w := serveAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong name: got %d, want 400", w.Code)
	}
	// Nothing deleted: the admin plus the _system sentinel remain.
	var users int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	if users != 2 {
		t.Fatalf("wrong-name wipe changed data: %d users, want 2", users)
	}

	// Correct name wipes everything.
	r = authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": "Yaml Quilt"}, token)
	w = serveAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("wipe: got %d: %s", w.Code, w.Body.String())
	}

	for _, table := range []string{"nodes", "memberships", "sessions", "instance_settings"} {
		var n int
		db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != 0 {
			t.Errorf("table %s still has %d rows after wipe", table, n)
		}
	}

	// Only the re-seeded _system sentinel remains, so the next real account
	// becomes the instance admin again (bootstrap rule).
	var remaining string
	if err := db.QueryRow("SELECT username FROM users").Scan(&remaining); err != nil || remaining != "_system" {
		t.Errorf("after wipe users = %q (err %v), want just _system", remaining, err)
	}

	// Schema and migration history survive.
	var migrations int
	db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrations)
	if migrations == 0 {
		t.Error("schema_migrations was wiped — it must survive")
	}
}
